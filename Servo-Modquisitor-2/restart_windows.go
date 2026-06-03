//go:build windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func replaceAndRestart(oldPath, newPath string) {
	backupPath := oldPath + ".old"

	// 1. Удаляем старый бэкап
	os.Remove(backupPath)

	// 2. Переименовываем текущий exe в backup
	err := os.Rename(oldPath, backupPath)
	if err != nil {
		os.Stderr.WriteString("Failed to rename running executable: " + err.Error() + "\n")
		return
	}

	// 3. Ставим новый exe на место старого
	err = os.Rename(newPath, oldPath)
	if err != nil {
		// Откатываем
		os.Rename(backupPath, oldPath)
		os.Stderr.WriteString("Failed to install new executable: " + err.Error() + "\n")
		return
	}

	// 4. Запускаем новый процесс с флагом --updated, чтобы обойти проверку isAlreadyRunning
	cmd := exec.Command(oldPath, "--updated")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = cmd.Start()
	if err != nil {
		// Откатываем
		os.Rename(oldPath, newPath)
		os.Rename(backupPath, oldPath)
		os.Stderr.WriteString("Failed to start new executable: " + err.Error() + "\n")
		return
	}

	time.Sleep(500 * time.Millisecond)
	os.Exit(0)
}
