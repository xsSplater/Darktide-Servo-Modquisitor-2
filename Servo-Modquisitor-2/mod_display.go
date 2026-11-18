// Servo-Modquisitor-2/mod_display.go
package main

import "Servo-Modquisitor/checks"

// localizedModName возвращает локализованное имя для записи БД с
// учётом настройки ForceEnglishModNames. Возвращает fallback, если
// перевода нет.
func (app *App) localizedModName(entry *checks.ModDBEntry, fallback string) string {
	if entry == nil {
		return fallback
	}
	app.cfgMutex.RLock()
	lang := app.cfg.Language
	forceEn := app.cfg.ForceEnglishModNames
	app.cfgMutex.RUnlock()
	if forceEn {
		lang = "en"
	}
	if name := checks.PickLocalized(entry.Name, lang); name != "" {
		return name
	}
	return fallback
}

// getModDisplayName — локализованное имя по имени папки.
// Единая точка для UI; заменяет прямые PickLocalized в редакторах.
func (app *App) getModDisplayName(folder string) string {
	return app.localizedModName(checks.GetModDBEntry(folder), folder)
}
