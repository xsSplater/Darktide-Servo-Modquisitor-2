//go:build windows

// process_windows.go
package main

import (
	"syscall"
	"unsafe"
)

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	createMutex      = kernel32.NewProc("CreateMutexW")
	createToolhelp32 = kernel32.NewProc("CreateToolhelp32Snapshot")
	process32First   = kernel32.NewProc("Process32FirstW")
	process32Next    = kernel32.NewProc("Process32NextW")
	user32           = syscall.NewLazyDLL("user32.dll")
	messageBox       = user32.NewProc("MessageBoxW")
)

const (
	TH32CS_SNAPPROCESS = 0x00000002
)

type PROCESSENTRY32 struct {
	dwSize              uint32
	cntUsage            uint32
	th32ProcessID       uint32
	th32DefaultHeapID   uintptr
	th32ModuleID        uint32
	cntThreads          uint32
	th32ParentProcessID uint32
	pcPriClassBase      int32
	dwFlags             uint32
	szExeFile           [260]uint16
}

func isAlreadyRunning() bool {
	mutexName, _ := syscall.UTF16PtrFromString("Global\\Servo-Modquisitor-Mutex")
	ret, _, err := createMutex.Call(0, 1, uintptr(unsafe.Pointer(mutexName)))
	if ret == 0 {
		return false
	}
	if err != nil && err.(syscall.Errno) == syscall.ERROR_ALREADY_EXISTS {
		return true
	}
	return false
}

func showAlreadyRunningDialog() {
	const title = "Servo-Modquisitor"
	const text = "Servo-Modquisitor is already running.\n\nPlease close the other instance before starting a new one."

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)

	messageBox.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		0x00040030, // MB_ICONINFORMATION | MB_OK | MB_TOPMOST
	)
}

func isDarktideRunning() bool {
	snapshot, _, _ := createToolhelp32.Call(TH32CS_SNAPPROCESS, 0)
	if snapshot == 0 {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))

	var pe PROCESSENTRY32
	pe.dwSize = uint32(unsafe.Sizeof(pe))

	ret, _, _ := process32First.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		name := syscall.UTF16ToString(pe.szExeFile[:])
		if name == "Darktide.exe" {
			return true
		}
		ret, _, _ = process32Next.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
	}
	return false
}
