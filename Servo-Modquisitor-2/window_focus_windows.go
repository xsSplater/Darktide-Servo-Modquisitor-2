//go:build windows
// Servo-Modquisitor-2/window_focus_windows.go

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

const swRestore = 9 // SW_RESTORE — восстановить и активировать

// bringWindowToFront поднимает окно поверх других приложений на Windows.
//
// Чистый RequestFocus() работает плохо: с Windows 7 действует
// foreground-lock — SetForegroundWindow отклоняется, если текущее
// foreground-окно принадлежит другому процессу. Обходим через
// AttachThreadInput к потоку текущего foreground-окна: пока потоки
// «склеены», Windows считает вызов легитимным.
func bringWindowToFront(w fyne.Window) {
	if w == nil {
		return
	}
	w.RequestFocus()

	native, ok := w.(driver.NativeWindow)
	if !ok {
		return
	}
	native.RunNative(func(ctx any) {
		wctx, ok := ctx.(*driver.WindowsWindowContext)
		if !ok || wctx.HWND == 0 {
			return
		}
		hwnd := uintptr(wctx.HWND)

		// Если окно свёрнуто — восстановим.
		procShowWindow.Call(hwnd, swRestore)

		fgHwnd, _, _ := procGetForegroundWindow.Call()
		ourThreadID, _, _ := procGetCurrentThreadId.Call()

		if fgHwnd != 0 {
			fgThreadID, _, _ := procGetWindowThreadProcessId.Call(fgHwnd, 0)
			if fgThreadID != 0 && fgThreadID != ourThreadID {
				procAttachThreadInput.Call(ourThreadID, fgThreadID, 1)
				procSetForegroundWindow.Call(hwnd)
				procAttachThreadInput.Call(ourThreadID, fgThreadID, 0)
				return
			}
		}
		procSetForegroundWindow.Call(hwnd)
	})
}
