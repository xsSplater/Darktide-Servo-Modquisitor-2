//go:build linux

// register_nxm_linux.go
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func registerNXMProtocol(exePath string) error {
	homeDir, _ := os.UserHomeDir()
	desktopFile := filepath.Join(homeDir, ".local", "share", "applications", "servo-modquisitor.desktop")

	// Формируем содержимое .desktop файла
	content := fmt.Sprintf(`[Desktop Entry]
Name=Servo-Modquisitor
Comment=Darktide mod manager
Exec=%s --nxm %%u
Type=Application
Terminal=false
MimeType=x-scheme-handler/nxm;
`, exePath)

	if err := os.WriteFile(desktopFile, []byte(content), 0755); err != nil {
		return err
	}

	// Регистрируем MIME-тип
	exec.Command("xdg-mime", "default", "servo-modquisitor.desktop", "x-scheme-handler/nxm").Run()

	return nil
}
