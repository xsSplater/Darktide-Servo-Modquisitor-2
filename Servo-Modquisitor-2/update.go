// update.go
package main

import (
	"net/url"
	"os"
	"path/filepath"
	"time"
)

// shouldCheckUpdates определяет, нужно ли проверять обновления при старте.
func (app *App) shouldCheckUpdates() bool {
	if app.cfg.UpdateCheckFrequency == "never" {
		return false
	}
	if app.cfg.UpdateCheckFrequency == "every_start" {
		return true
	}
	if app.cfg.LastUpdateCheck == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, app.cfg.LastUpdateCheck)
	if err != nil {
		return true
	}
	now := time.Now()
	switch app.cfg.UpdateCheckFrequency {
	case "weekly":
		return now.Sub(last) >= 7*24*time.Hour
	case "monthly":
		return now.After(last.AddDate(0, 1, 0))
	case "yearly":
		return now.After(last.AddDate(1, 0, 0))
	}
	return false
}

// checkForProgramUpdate - открывает страницу мода на Nexus (ручное обновление).
func (app *App) checkForProgramUpdate() {
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog(app.messages["open_download_page"])
}

// initiateProgramUpdate - открывает страницу мода и предлагает скачать программу через Mod manager download.
func (app *App) initiateProgramUpdate() {
	app.appendLog("Opening Nexus mod page for program update...")
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog("Please click 'Mod manager download' on the 'Servo Modquisitor 2.zip' file.")
}

// initiateSortFilesUpdate - открывает страницу мода и предлагает скачать файлы сортировки через Mod manager download.
func (app *App) initiateSortFilesUpdate() {
	app.appendLog("Opening Nexus mod page for sorting files update...")
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog("Please click 'Mod manager download' on the 'Mod DB And Sorting Rules.zip' file.")
}

// ensureSortFiles - вызывается при старте, если файлы отсутствуют.
// Теперь открывает страницу мода и просит пользователя скачать их вручную.
func (app *App) ensureSortFiles() {
	missing := false
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameMandatoryRules)); os.IsNotExist(err) {
		missing = true
	}
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameModDatabase)); os.IsNotExist(err) {
		missing = true
	}
	if !missing {
		return
	}

	app.appendLog(app.messages["sort_files_missing_short"])

	if app.cfg.SkipSortFilesPrompt {
		app.appendLog(app.messages["download_skip_forever"])
		return
	}

	choice := app.showChoiceDialog(app.mainWindow,
		app.messages["sort_files_missing"],
		"Sorting files are missing. Would you like to open the Nexus mod page to download them manually?",
		app.messages["yes"],
		app.messages["skip"],
		app.messages["download_skip_forever"],
	)
	switch choice {
	case 0:
		u, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/139")
		_ = app.myApp.OpenURL(u)
		app.appendLog("Please download 'Mod DB And Sorting Rules.zip' and install it manually via drag-and-drop or 'Install Mod' button.")
	case 2:
		app.cfg.SkipSortFilesPrompt = true
		saveConfig(app.cfg)
		fallthrough
	case 1:
		app.appendLog(app.messages["download_skipped"])
	}
}
