// Servo-Modquisitor-2/app_window_registry.go
package main

import "fyne.io/fyne/v2"

// acquireWindow проверяет, открыто ли уже окно с указанным ID.
//
//   - Если открыто — поднимает его на передний план и возвращает false.
//   - Если нет — возвращает true, вызывающий создаёт новое окно и
//     обязан зарегистрировать его через registerWindow.
//
// Типичный паттерн:
//
//	if !app.acquireWindow("aml_config") {
//		return
//	}
//	win := app.myApp.NewWindow(...)
//	app.registerWindow("aml_config", win)
//	// ... остальной код
//
// Вызывается из UI-потока (обработчик меню, OnTapped), поэтому
// гонок между проверкой и вставкой в map быть не может.
func (app *App) acquireWindow(id string) bool {
	app.openWindowsMu.Lock()
	win, ok := app.openWindows[id]
	app.openWindowsMu.Unlock()

	if ok && win != nil {
		bringWindowToFront(win)
		return false
	}
	return true
}

// registerWindow сохраняет окно в реестре и вешает SetOnClosed,
// который автоматически удаляет запись при закрытии. Это гарантирует,
// что повторный вызов showXxxWindow создаст новое окно, а не поднимет
// уже уничтоженное.
func (app *App) registerWindow(id string, win fyne.Window) {
	app.openWindowsMu.Lock()
	app.openWindows[id] = win
	app.openWindowsMu.Unlock()

	win.SetOnClosed(func() {
		app.openWindowsMu.Lock()
		delete(app.openWindows, id)
		app.openWindowsMu.Unlock()
	})
}

// forgetWindow принудительно удаляет запись из реестра. Нужна только
// в редком случае: когда окно закрывается, а сразу за этим в том же
// стеке открывается новое с тем же ID (как в reload-кнопке
// редактора правил). В обычной жизни хватает SetOnClosed.
func (app *App) forgetWindow(id string) {
	app.openWindowsMu.Lock()
	delete(app.openWindows, id)
	app.openWindowsMu.Unlock()
}
