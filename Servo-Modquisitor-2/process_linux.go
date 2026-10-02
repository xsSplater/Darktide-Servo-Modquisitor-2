//go:build linux

// Servo-Modquisitor-2/process_linux.go

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
	cmd := exec.Command("pgrep", "-f", "Darktide.exe")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// isSteamRunning сообщает, запущен ли клиент Steam. Покрывает обычную
// установку (deb/rpm — процесс называется "steam") и Flatpak (снаружи
// процесса "steam" не видно, но видно bwrap/flatpak с путём
// com.valvesoftware.Steam).
func isSteamRunning() bool {
	if err := exec.Command("pgrep", "-x", "steam").Run(); err == nil {
		return true
	}
	// Flatpak: ищем любой процесс с "com.valvesoftware.Steam" в командной строке.
	// Ложные срабатывания маловероятны, а если сработает на запущенной игре —
	// это тоже значит, что Steam работает.
	if err := exec.Command("pgrep", "-f", "com.valvesoftware.Steam").Run(); err == nil {
		return true
	}
	return false
}

// isSteamClientReady сообщает, что клиент Steam полностью поднялся.
// На Linux steamwebhelper называется steamwebhelper (нативная версия)
// или com.valvesoftware.Steam... (Flatpak). Плюс — признак готовности:
// процесс висит больше секунды.
func isSteamClientReady() bool {
	if err := exec.Command("pgrep", "-x", "steamwebhelper").Run(); err == nil {
		return true
	}
	// Flatpak: steamwebhelper внутри sandbox
	if err := exec.Command("pgrep", "-f", "steamwebhelper").Run(); err == nil {
		return true
	}
	return false
}
