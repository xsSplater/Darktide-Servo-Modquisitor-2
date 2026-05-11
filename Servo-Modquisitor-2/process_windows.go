//go:build windows

package main

import (
    "syscall"
    "unsafe"
)

var (
    kernel32 = syscall.NewLazyDLL("kernel32.dll")
    procCreateToolhelp32Snapshot = kernel32.NewProc("CreateToolhelp32Snapshot")
    procProcess32First           = kernel32.NewProc("Process32FirstW")
    procProcess32Next            = kernel32.NewProc("Process32NextW")
    procCloseHandle              = kernel32.NewProc("CloseHandle")
)

const (
    TH32CS_SNAPPROCESS = 0x00000002
)

type PROCESSENTRY32 struct {
    DwSize              uint32
    CntUsage            uint32
    Th32ProcessID       uint32
    Th32DefaultHeapID   uintptr
    Th32ModuleID        uint32
    CntThreads          uint32
    Th32ParentProcessID uint32
    PcPriClassBase      int32
    DwFlags             uint32
    SzExeFile           [syscall.MAX_PATH]uint16
}

// isDarktideRunning возвращает true, если процесс Darktide.exe уже запущен.
func isDarktideRunning() bool {
    snapshot, _, _ := procCreateToolhelp32Snapshot.Call(TH32CS_SNAPPROCESS, 0)
    if snapshot == 0 {
        return false
    }
    defer procCloseHandle.Call(snapshot)

    var pe PROCESSENTRY32
    pe.DwSize = uint32(unsafe.Sizeof(pe))
    ret, _, _ := procProcess32First.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
    if ret == 0 {
        return false
    }
    for {
        name := syscall.UTF16ToString(pe.SzExeFile[:])
        if name == "Darktide.exe" {
            return true
        }
        ret, _, _ = procProcess32Next.Call(snapshot, uintptr(unsafe.Pointer(&pe)))
        if ret == 0 {
            break
        }
    }
    return false
}
