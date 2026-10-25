//go:build linux

// Servo-Modquisitor-2/window_state_linux.go

package main

import (
	"os/exec"
)

// maximizeWindowByTitle пытается развернуть окно на весь экран средствами ОС.
func maximizeWindowByTitle(title string) {
	// Пробуем wmctrl (наиболее надежный способ для большинства WM)
	if err := exec.Command("wmctrl", "-r", title, "-b", "add,maximized_vert,maximized_horz").Run(); err == nil {
		return
	}
	// Если wmctrl нет, пробуем xdotool (менее надежно, но часто работает)
	_ = exec.Command("xdotool", "search", "--name", title, "windowsize", "100%", "100%").Run()
}

// isWindowMaximized на Linux без прямых X11-биндингов проверить сложно,
// поэтому возвращаем false, чтобы не мешать сохранению текущих размеров.
func isWindowMaximized(_ string) bool {
	// На Linux состояние «развёрнуто» через WM-независимый API
	// не читается. Возвращаем false — при следующем старте
	// окно откроется в сохранённом размере.
	return false
}
