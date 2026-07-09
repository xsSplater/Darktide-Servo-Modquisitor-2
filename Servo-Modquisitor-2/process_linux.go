//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func isAlreadyRunning() bool {
	lockFile := "/tmp/servo-modquisitor.lock"
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		// Файл уже существует - другой процесс держит его (или старый висит)
		return true
	}
	defer f.Close()

	// Удаляем файл из файловой системы, но оставляем открытый дескриптор.
	// При завершении процесса дескриптор закроется, и файл исчезнет окончательно.
	os.Remove(lockFile)

	// Записываем PID (необязательно, но полезно для отладки)
	fmt.Fprintf(f, "%d", os.Getpid())

	return false
}

func showAlreadyRunningDialog() {
	os.Stderr.WriteString("Servo-Modquisitor is already running.\nPlease close the other instance before starting a new one.\n")
}

func isDarktideRunning() bool {
	cmd := exec.Command("pgrep", "Darktide.exe")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}
