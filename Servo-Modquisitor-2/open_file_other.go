//go:build !windows

// open_file_other.go
package main

import "os/exec"

// openFileWithDefaultApp открывает файл приложением по умолчанию (Linux/macOS).
func openFileWithDefaultApp(path string) error {
	// xdg-open на Linux, open на macOS — оба сработают
	return exec.Command("xdg-open", path).Start()
}
