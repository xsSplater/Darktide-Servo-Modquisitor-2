//go:build linux

// Servo-Modquisitor-2/process_linux.go

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/unix"
)

// lockFileHandle держим открытым до выхода процесса: ядро снимает
// flock автоматически при завершении (в т.ч. по крашу).
var lockFileHandle *os.File

// isAlreadyRunning — single-instance через flock(LOCK_EX|LOCK_NB).
//
// Файл НЕ удаляем после открытия: имя /tmp/servo-modquisitor.lock —
// просто якорь. Состояние «занято» живёт в ядре (flock), а не в ФС.
// При завершении процесса ядро снимает лок само — файл остаётся, но
// следующий запуск спокойно получит на нём эксклюзивный flock.
//
// Раньше здесь был O_CREATE|O_EXCL + os.Remove — модель «существование
// файла = занято». Она ломалась: удаление файла освобождало имя,
// следующий процесс создавал его заново и работал параллельно.
// Плюс краш оставлял файл навсегда → программа не запускалась.
func isAlreadyRunning() bool {
	lockFile := "/tmp/servo-modquisitor.lock"

	f, err := os.OpenFile(lockFile, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		// Не смогли открыть даже для чтения — что-то не так,
		// безопаснее считать, что уже запущено.
		return true
	}

	// LOCK_EX | LOCK_NB: эксклюзивный, не блокирующий.
	// EWOULDBLOCK → лок уже держит другой процесс.
	err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		f.Close()
		return true
	}

	// Лок наш. Файл НЕ удаляем — иначе следующий процесс
	// создаст новый по тому же пути и получит лок без конфликта.
	// Пишем PID для отладки.
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	fmt.Fprintf(f, "%d", os.Getpid())

	lockFileHandle = f
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
