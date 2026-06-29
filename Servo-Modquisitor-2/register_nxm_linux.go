//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func registerNXMProtocol(exePath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	desktopDir := filepath.Join(homeDir, ".local", "share", "applications")
	if err := os.MkdirAll(desktopDir, 0755); err != nil {
		return err
	}

	desktopFile := filepath.Join(desktopDir, "servo-modquisitor.desktop")

	content := fmt.Sprintf(`[Desktop Entry]
Name=Servo-Modquisitor
Comment=Darktide mod manager
Exec=%s --nxm %%u
Type=Application
Terminal=false
MimeType=x-scheme-handler/nxm;
`, exePath)

	if err := os.WriteFile(desktopFile, []byte(content), 0644); err != nil {
		return err
	}

	// Регистрируем MIME-тип
	_ = exec.Command("xdg-mime", "default", "servo-modquisitor.desktop", "x-scheme-handler/nxm").Run()

	// Обновляем базу desktop-файлов
	_ = exec.Command("update-desktop-database", desktopDir).Run()

	return nil
}
