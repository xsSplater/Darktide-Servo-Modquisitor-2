// update.go
package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// initiateSortFilesUpdate - открывает страницу мода и предлагает скачать файлы сортировки
func (app *App) initiateSortFilesUpdate() {
	app.appendLog(app.messages["log_open_nexus_page"])
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
}

// ensureSortFiles - вызывается при старте, если файлы отсутствуют.
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

	// Асинхронный диалог - не блокирует поток
	app.showChoiceDialog(
		app.mainWindow,
		app.messages["sort_files_missing"],
		app.messages["sort_files_missing_open_page"],
		func(choice int) {
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
		},
		app.messages["btn_yes"],
		app.messages["skip"],
		app.messages["download_skip_forever"],
	)
}

// compareVersions сравнивает две семантические версии (например, "1.9.0" и "1.9.5").
// Возвращает -1, если v1 < v2; 0, если равны; 1, если v1 > v2.
func compareVersions(v1, v2 string) int {
	// Разбиваем на части по точкам
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	for i := 0; i < maxLen; i++ {
		var n1, n2 int
		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
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
		if saved, ok := app.getCachedVersion(NexusCacheKeyProgram); ok {
			// сравниваем, но не обновляем кэш
			if compareVersions(programFileInfo.Version, saved.Version) > 0 {
				app.appendLog(fmt.Sprintf(app.messages["log_new_program_version_available"],
					programFileInfo.Version, saved.Version))
			}
		} else {
			// Просто пропускаем, не создаём запись
			app.appendLog(app.messages["log_program_not_cached"])
		}
	}

	// Проверка файлов сортировки
	rulesFileInfo, err := app.getLatestFileInfoForMod(139, "Mod DB And Sorting Rules")
	if err != nil {
		app.logNexusError(err, "Rules", app.messages["rules_update_unavailable"])
	} else if rulesFileInfo != nil {
		if saved, ok := app.getCachedVersion(NexusCacheKeyRules); ok {
			if compareVersions(rulesFileInfo.Version, saved.Version) > 0 {
				app.appendLog(fmt.Sprintf(app.messages["log_new_sorting_files_available"],
					rulesFileInfo.Version, saved.Version))
			}
		} else {
			// Просто пропускаем, не создаём запись
			app.appendLog(app.messages["log_sorting_rules_not_cached"])
		}
	}

	// Обновляем время последней проверки
	app.cfg.LastUpdateCheck = time.Now().Format(time.RFC3339)
	saveConfig(app.cfg)
}
