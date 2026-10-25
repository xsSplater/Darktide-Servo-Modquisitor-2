//go:build windows

// Servo-Modquisitor-2/trash_windows.go
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Реализация Корзины через COM IFileOperation. В отличие от
// SHFileOperationW (бэкенд wastebasket), этот API:
//   - умеет пакетную обработку (один PerformOperations на N файлов);
//   - позволяет отменить удаление через PreDeleteItem, если
//     оболочка собирается стереть файл насовсем (Корзина недоступна);
//   - работает в разы быстрее при массовом удалении.
//
// Логика адаптирована из github.com/zemidala/modvault/fsx.
// Проверьте лицензию источника, если копируете.

var (
	modole32   = windows.NewLazySystemDLL("ole32.dll")
	modshell32 = windows.NewLazySystemDLL("shell32.dll")

	procCoCreateInstance            = modole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = modshell32.NewProc("SHCreateItemFromParsingName")
)

var (
	clsidFileOperation = windows.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
	iidFileOperation   = windows.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
	iidProgressSink    = windows.GUID{Data1: 0x04b0f1a7, Data2: 0x9490, Data3: 0x44bc, Data4: [8]byte{0x96, 0xe1, 0x42, 0x96, 0xa3, 0x12, 0x52, 0xe2}}
	iidShellItem       = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
	iidUnknown         = windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}
)

const (
	clsctxAll = 0x17

	// Без окон и вопросов; удалять только в Корзину.
	fofAllowUndo        = 0x40
	fofNoUI             = 0x4 | 0x10 | 0x200 | 0x400
	fofxRecycleOnDelete = 0x00080000
	// ВАЖНО: НЕ ставим fofxEarlyFailure. В batch-режиме пусть каждый
	// мод удаляется независимо — один занятый файл не должен
	// блокировать остальные 99.

	tsfDeleteRecycleIfPossible = 0x80

	hrOK               = 0
	hrFalse            = 1
	hrNoInterface      = 0x80004002
	hrAbort            = 0x80004004
	hrChangedMode      = 0x80010106
	hrSharingViolation = 0x80070020
)

const (
	vtRelease = 2

	vtAdvise                  = 3
	vtUnadvise                = 4
	vtSetOperationFlags       = 5
	vtDeleteItem              = 18
	vtPerformOperations       = 21
	vtGetAnyOperationsAborted = 22

	sinkMethods      = 19
	sinkPreDeleteIdx = 11
)

type deleteSink struct {
	vtbl *[sinkMethods]uintptr
}

var (
	trashMu  sync.Mutex
	sinkOnce sync.Once
	sink     deleteSink
	sinkNuke bool
)

func initSink() {
	ok := windows.NewCallback(func(this uintptr) uintptr { return hrOK })
	ref := windows.NewCallback(func(this uintptr) uintptr { return 1 })

	var vtbl [sinkMethods]uintptr
	for i := range vtbl {
		vtbl[i] = ok
	}
	vtbl[0] = windows.NewCallback(sinkQueryInterface)
	vtbl[1] = ref
	vtbl[2] = ref
	vtbl[sinkPreDeleteIdx] = windows.NewCallback(sinkPreDelete)
	sink.vtbl = &vtbl
}

func sinkQueryInterface(this *deleteSink, iid *windows.GUID, out **deleteSink) uintptr {
	if *iid == iidUnknown || *iid == iidProgressSink {
		*out = this
		return hrOK
	}
	*out = nil
	return hrNoInterface
}

// sinkPreDelete вызывается оболочкой перед удалением каждого элемента.
// Если флаг tsfDeleteRecycleIfPossible отсутствует — значит, Корзина
// для этого элемента недоступна, и оболочка собирается стереть его
// насовсем. Мы такого не хотим — отменяем всю операцию.
func sinkPreDelete(this, flags, item uintptr) uintptr {
	if flags&tsfDeleteRecycleIfPossible == 0 {
		sinkNuke = true
		return hrAbort
	}
	return hrOK
}

func failed(hr uintptr) bool {
	return int32(hr) < 0
}

func method(obj unsafe.Pointer, index int) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	return *(*uintptr)(unsafe.Add(vtbl, uintptr(index)*unsafe.Sizeof(uintptr(0))))
}

func release(obj unsafe.Pointer) {
	syscall.SyscallN(method(obj, vtRelease), uintptr(obj))
}

// trashPaths перемещает все paths в Корзину одной операцией IFileOperation.
func trashPaths(paths []string) error {
	trashMu.Lock()
	defer trashMu.Unlock()
	sinkOnce.Do(initSink)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE)
	var errno syscall.Errno
	switch {
	case err == nil, errors.As(err, &errno) && uint32(errno) == hrFalse:
		defer windows.CoUninitialize()
	case errors.As(err, &errno) && uint32(errno) == hrChangedMode:
		// Уже инициализирован в другом режиме — нам подходит.
	default:
		return fmt.Errorf("trash: CoInitializeEx: %w", err)
	}

	var op unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxAll,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&op)))
	if failed(hr) {
		return fmt.Errorf("trash: CoCreateInstance: HRESULT 0x%08X", uint32(hr))
	}
	defer release(op)

	hr, _, _ = syscall.SyscallN(method(op, vtSetOperationFlags), uintptr(op),
		fofAllowUndo|fofNoUI|fofxRecycleOnDelete)
	if failed(hr) {
		return fmt.Errorf("trash: SetOperationFlags: HRESULT 0x%08X", uint32(hr))
	}

	var cookie uint32
	hr, _, _ = syscall.SyscallN(method(op, vtAdvise), uintptr(op),
		uintptr(unsafe.Pointer(&sink)), uintptr(unsafe.Pointer(&cookie)))
	if failed(hr) {
		return fmt.Errorf("trash: Advise: HRESULT 0x%08X", uint32(hr))
	}
	defer func() {
		syscall.SyscallN(method(op, vtUnadvise), uintptr(op), uintptr(cookie))
	}()

	// Ставим все файлы в очередь.
	for _, path := range paths {
		path16, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		var item unsafe.Pointer
		hr, _, _ := procSHCreateItemFromParsingName.Call(
			uintptr(unsafe.Pointer(path16)), 0,
			uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
		if failed(hr) {
			// Оболочка не понимает этот путь (>260 символов или
			// спецсимволы). Пропускаем — остальные всё равно попробуем.
			continue
		}
		hr, _, _ = syscall.SyscallN(method(op, vtDeleteItem), uintptr(op), uintptr(item), 0)
		release(item)
		if failed(hr) {
			continue
		}
	}

	sinkNuke = false
	hr, _, _ = syscall.SyscallN(method(op, vtPerformOperations), uintptr(op))

	if sinkNuke {
		return errors.New("trash: отменено — Корзина недоступна для одного из файлов, " +
			"оболочка собиралась удалить его насовсем")
	}
	if failed(hr) {
		return fmt.Errorf("trash: PerformOperations: HRESULT 0x%08X", uint32(hr))
	}

	// Проверяем, что все файлы действительно исчезли.
	var stillThere []string
	for _, path := range paths {
		if _, err := os.Lstat(path); err == nil {
			stillThere = append(stillThere, path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if len(stillThere) > 0 {
		return fmt.Errorf("trash: не удалось переместить %d из %d файлов (заняты?)",
			len(stillThere), len(paths))
	}
	return nil
}
