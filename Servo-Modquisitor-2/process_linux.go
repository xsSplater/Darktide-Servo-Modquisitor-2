//go:build linux

// process_linux.go
package main

import (
	"os"
	"os/exec"
	"strings"
)

func isAlreadyRunning() bool {
	lockFile := "/tmp/servo-modquisitor.lock"
	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return true
	}
	defer f.Close()
	return false
}

func showAlreadyRunningDialog() {
	// На Linux показываем сообщение в терминал, так как GUI ещё не запущен
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
