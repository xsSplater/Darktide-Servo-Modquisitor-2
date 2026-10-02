//go:build windows

// Servo-Modquisitor-2/win_native.go

package main

import "syscall"

// Все Windows DLL и proc-указатели, разделяемые между файлами. Если
// объявлять их по разным файлам — Go упадёт с «redeclared in this
// block», потому что все файлы пакета main компилируются вместе.
var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	// user32.dll
	procSetForegroundWindow      = user32.NewProc("SetForegroundWindow")
	procShowWindow               = user32.NewProc("ShowWindow")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput        = user32.NewProc("AttachThreadInput")
	procMessageBoxW              = user32.NewProc("MessageBoxW")
	procFindWindowW              = user32.NewProc("FindWindowW")
	procIsZoomed                 = user32.NewProc("IsZoomed")

	// kernel32.dll
	procGetCurrentThreadId       = kernel32.NewProc("GetCurrentThreadId")
	procCreateMutexW             = kernel32.NewProc("CreateMutexW")
	procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW          = kernel32.NewProc("Process32FirstW")
	procProcess32NextW           = kernel32.NewProc("Process32NextW")
)
