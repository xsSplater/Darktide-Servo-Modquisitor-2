//go:build linux
// Servo-Modquisitor-2/window_focus_linux.go

package main

import "fyne.io/fyne/v2"

// bringWindowToFront поднимает окно и забирает фокус. На Linux
// RequestFocus() обычно достаточно — оконные менеджеры позволяют
// приложению выйти на передний план.
func bringWindowToFront(w fyne.Window) {
	if w == nil {
		return
	}
	w.RequestFocus()
}
