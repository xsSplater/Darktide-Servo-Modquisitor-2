//go:build linux

// restart_linux.go
package main

import (
	"os"
	"os/exec"
)

func replaceAndRestart(oldPath, newPath string) {
	backupPath := oldPath + ".bak"

	os.Remove(backupPath)
	err := os.Rename(oldPath, backupPath)
	if err != nil {
		os.Stderr.WriteString("Failed to backup: " + err.Error() + "\n")
		return
	}

	os.Chmod(newPath, 0755)
	err = os.Rename(newPath, oldPath)
	if err != nil {
		os.Rename(backupPath, oldPath)
		os.Stderr.WriteString("Failed to install: " + err.Error() + "\n")
		return
	}

	cmd := exec.Command(oldPath, "--updated")
	err = cmd.Start()
	if err != nil {
		os.Rename(oldPath, newPath)
		os.Rename(backupPath, oldPath)
		os.Stderr.WriteString("Failed to launch: " + err.Error() + "\n")
		return
	}

	os.Exit(0)
}
