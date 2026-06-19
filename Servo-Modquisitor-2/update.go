// update.go
package main

import (
	"fmt"
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
	app.appendLog(app.messages["log_open_nexus_page"])
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog(app.messages["log_please_click_smq_zip"])
}

// initiateSortFilesUpdate - открывает страницу мода и предлагает скачать файлы сортировки через Mod manager download.
func (app *App) initiateSortFilesUpdate() {
	app.appendLog(app.messages["log_open_nexus_page"])
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog(app.messages["log_please_click_sort_zip"])
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
		app.messages["sort_files_missing_open_page"],
		app.messages["yes"],
		app.messages["skip"],
		app.messages["download_skip_forever"],
	)
	switch choice {
	case 0:
		u, _ := url.Parse(ServoMQModPage)
		_ = app.myApp.OpenURL(u)
		app.appendLog(app.messages["please_download_mod_db_install"])
	case 2:
		app.cfg.SkipSortFilesPrompt = true
		saveConfig(app.cfg)
		fallthrough
	case 1:
		app.appendLog(app.messages["download_skipped"])
	}
}

// checkSpecialUpdates проверяет наличие новых версий программы и файлов сортировки (мод 139).
func (app *App) checkSpecialUpdates() {
	// Проверяем, авторизован ли пользователь
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["log_spec_update_not_logged"])
		return
	}

	// Проверка программы
	programFileInfo, err := app.getLatestFileInfoForMod(139, "Servo Modquisitor 2")
	if err != nil {
		app.logNexusError(err, "Program", app.messages["program_update_unavailable"])
	} else if programFileInfo != nil {
		if saved, ok := app.nexusVersionCache[NexusCacheKeyProgram]; ok {
			if programFileInfo.UploadedTimestamp > saved.Timestamp {
				app.appendLog(fmt.Sprintf(app.messages["log_new_program_version_available"],
					programFileInfo.Version, saved.Version))
			}
		} else {
			// Первая проверка — сохраняем информацию
			app.nexusVersionCache[NexusCacheKeyProgram] = ModVersionInfo{
				Timestamp: programFileInfo.UploadedTimestamp,
				Version:   programFileInfo.Version,
				Folder:    "Program",
			}
			app.saveNexusVersionCache()
			app.appendLog(app.messages["log_version_cached_program"] + programFileInfo.Version)
		}
	}

	// Проверка файлов сортировки
	rulesFileInfo, err := app.getLatestFileInfoForMod(139, "Mod DB And Sorting Rules")
	if err != nil {
		app.logNexusError(err, "Rules", app.messages["rules_update_unavailable"])
	} else if rulesFileInfo != nil {
		if saved, ok := app.nexusVersionCache[NexusCacheKeyRules]; ok {
			if rulesFileInfo.UploadedTimestamp > saved.Timestamp {
				app.appendLog(fmt.Sprintf(app.messages["log_new_sorting_files_available"],
					rulesFileInfo.Version, saved.Version))
			}
		} else {
			app.nexusVersionCache[NexusCacheKeyRules] = ModVersionInfo{
				Timestamp: rulesFileInfo.UploadedTimestamp,
				Version:   rulesFileInfo.Version,
				Folder:    "Sorting Rules",
			}
			app.saveNexusVersionCache()
			app.appendLog(app.messages["log_version_cached_sort"] + rulesFileInfo.Version)
		}
	}

	// Обновляем время последней проверки
	app.cfg.LastUpdateCheck = time.Now().Format(time.RFC3339)
	saveConfig(app.cfg)
}
