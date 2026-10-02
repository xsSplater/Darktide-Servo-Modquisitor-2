//go:build linux

// Servo-Modquisitor-2/trash_linux.go
package main

import (
	"fmt"
	"os/exec"
)

// moveToTrash пытается положить путь в системную корзину Linux:
// сначала gio trash (glib2), затем trash-put (trash-cli).
func moveToTrash(path string) error {
	if err := exec.Command("gio", "trash", "--", path).Run(); err == nil {
		return nil
	}
	if err := exec.Command("trash-put", "--", path).Run(); err == nil {
		return nil
	}
	return fmt.Errorf("no working trash command (install gio or trash-cli)")
}
