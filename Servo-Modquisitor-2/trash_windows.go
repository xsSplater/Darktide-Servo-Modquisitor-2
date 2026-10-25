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
//   - работает в разы быстрее при массовом удалении.
//
// ВНИМАНИЕ: Мы намеренно НЕ используем IFileOperationProgressSink
// (Advise/Unadvise), так как это вызывает дедлок в Windows при удалении
// большого количества файлов из фонового потока. Флаг fofxRecycleOnDelete
// надежно гарантирует, что файлы уйдут в Корзину, а не будут стерты навсегда.

var (
	modole32   = windows.NewLazySystemDLL("ole32.dll")
	modshell32 = windows.NewLazySystemDLL("shell32.dll")

	procCoCreateInstance            = modole32.NewProc("CoCreateInstance")
	procSHCreateItemFromParsingName = modshell32.NewProc("SHCreateItemFromParsingName")
)

var (
	clsidFileOperation = windows.GUID{Data1: 0x3ad05575, Data2: 0x8857, Data3: 0x4850, Data4: [8]byte{0x92, 0x77, 0x11, 0xb8, 0x5b, 0xdb, 0x8e, 0x09}}
	iidFileOperation   = windows.GUID{Data1: 0x947aab5f, Data2: 0x0a5c, Data3: 0x4c13, Data4: [8]byte{0xb4, 0xd6, 0x4b, 0xf7, 0x83, 0x6f, 0xc9, 0xf8}}
	iidShellItem       = windows.GUID{Data1: 0x43826d1e, Data2: 0xe718, Data3: 0x42ee, Data4: [8]byte{0xbc, 0x55, 0xa1, 0xe2, 0x61, 0xc3, 0x7b, 0xfe}}
)

const (
	clsctxAll = 0x17

	// Без окон и вопросов; удалять только в Корзину.
	fofAllowUndo        = 0x40
	fofNoUI             = 0x4 | 0x10 | 0x200 | 0x400
	fofxRecycleOnDelete = 0x00080000

	hrOK          = 0
	hrFalse       = 1
	hrChangedMode = 0x80010106
)

const (
	vtRelease           = 2
	vtSetOperationFlags = 5
	vtDeleteItem        = 18
	vtPerformOperations = 21
)

var trashMu sync.Mutex

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

	// Выполняем. Без колбэков это не заблокирует поток навсегда.
	hr, _, _ = syscall.SyscallN(method(op, vtPerformOperations), uintptr(op))
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
		return fmt.Errorf("trash: не удалось переместить %d из %d файлов", len(stillThere), len(paths))
	}
	return nil
}
