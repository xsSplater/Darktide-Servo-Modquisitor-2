//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32					= syscall.NewLazyDLL("user32.dll")
	procFindWindowW			= user32.NewProc("FindWindowW")
	procShowWindow			= user32.NewProc("ShowWindow")
	procIsZoomed			= user32.NewProc("IsZoomed")
)

const (
	SW_MAXIMIZE = 3
)

// maximizeWindowByTitle пытается найти окно по заголовку и максимизировать его.
func maximizeWindowByTitle(title string) {
	hwnd := findWindow(title)
	if hwnd != 0 {
		procShowWindow.Call(uintptr(hwnd), SW_MAXIMIZE)
	}
}

// isWindowMaximized возвращает true, если окно сейчас развёрнуто.
func isWindowMaximized(title string) bool {
	hwnd := findWindow(title)
	if hwnd == 0 {
		return false
	}
	ret, _, _ := procIsZoomed.Call(uintptr(hwnd))
	return ret != 0
}

// findWindow ищет окно верхнего уровня по точному заголовку.
func findWindow(title string) syscall.Handle {
	lpWindowName, _ := syscall.UTF16PtrFromString(title)
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(lpWindowName)))
	return syscall.Handle(hwnd)
}
