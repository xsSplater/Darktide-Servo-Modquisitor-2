//go:build windows

// open_file_windows.go
package main

import "os/exec"

// openFileWithDefaultApp открывает файл приложением по умолчанию (Windows).
func openFileWithDefaultApp(path string) error {
	return exec.Command("cmd", "/c", "start", "", path).Start()
}
