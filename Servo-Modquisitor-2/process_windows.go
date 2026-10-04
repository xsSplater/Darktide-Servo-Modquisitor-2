//go:build windows

// Servo-Modquisitor-2/process_windows.go

package main

import (
	"strings"
	"syscall"
	"unsafe"
)

const (
	TH32CS_SNAPPROCESS   = 0x00000002
	ERROR_ALREADY_EXISTS = 183
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
	// Без "Global\\" — мьютекс в namespace текущей сессии.
	// Это то, что нужно для single-instance: у каждой сессии
	// свой экземпляр программы.
	mutexName, _ := syscall.UTF16PtrFromString("Servo-Modquisitor-Mutex")
	ret, _, err := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(mutexName)))
	if ret == 0 {
		// Не смогли создать мьютекс. Раньше возвращали false —
		// запускали второй инстанс. Безопаснее считать, что уже
		// запущено: пользователь скорее перезапустит, чем
		// получит дубль.
		return true
	}
	if errno, ok := err.(syscall.Errno); ok && errno == ERROR_ALREADY_EXISTS {
		return true
	}
	return false
}

func showAlreadyRunningDialog() {
	const title = "Servo-Modquisitor"
	const text = "Servo-Modquisitor is already running.\n\nPlease close the other instance before starting a new one."

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	textPtr, _ := syscall.UTF16PtrFromString(text)

	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
		0x00040030, // MB_ICONINFORMATION | MB_OK | MB_TOPMOST
	)
}

// isProcessRunning сообщает, есть ли в системе процесс с таким именем
// исполняемого файла (регистр не важен, имя с ".exe").
func isProcessRunning(name string) bool {
	snapshot, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
	if snapshot == 0 {
		return false
	}
	defer syscall.CloseHandle(syscall.Handle(snapshot))

	var pe PROCESSENTRY32
	pe.dwSize = uint32(unsafe.Sizeof(pe))

	ret, _, _ := procProcess32FirstW.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
	for ret != 0 {
		if strings.EqualFold(syscall.UTF16ToString(pe.szExeFile[:]), name) {
			return true
		}
		ret, _, _ = procProcess32NextW.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
	}
	return false
}

func isDarktideRunning() bool {
	return isProcessRunning("Darktide.exe")
}

// isSteamRunning сообщает, запущен ли клиент Steam. Проверяем оба
// процесса: основной "steam.exe" и его helper "steamwebhelper.exe".
func isSteamRunning() bool {
	return isProcessRunning("steam.exe") || isProcessRunning("steamwebhelper.exe")
}

// isSteamClientReady сообщает, что клиент Steam полностью поднялся.
// Основной steam.exe появляется моментально (это bootstrap), а
// steamwebhelper.exe — только когда клиент готов принимать URL и
// обслуживать SteamAPI.
func isSteamClientReady() bool {
	return isProcessRunning("steamwebhelper.exe")
}
