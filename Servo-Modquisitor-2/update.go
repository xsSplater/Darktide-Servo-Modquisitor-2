// Servo-Modquisitor-2/update.go
package main

import (
	"Servo-Modquisitor/checks"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/fyne/v2"
)

// initiateSortFilesUpdate - открывает страницу мода и предлагает скачать файлы сортировки
func (app *App) initiateSortFilesUpdate() {
	app.appendLog(app.msg("log_open_nexus_page"))
	u, _ := url.Parse(ServoMQModPage)
	fyne.Do(func() {
		_ = app.myApp.OpenURL(u)
	})
}

// ensureSortFiles - вызывается при старте, если файлы отсутствуют.
func (app *App) ensureSortFiles() {
	// Проверяем наличие файлов в глобальной папке (рядом с программой)
	globalDir := checks.GlobalDataDir()
	missing := false
	if _, err := os.Stat(filepath.Join(globalDir, FileNameMandatoryRules)); os.IsNotExist(err) {
		missing = true
	}
	if _, err := os.Stat(filepath.Join(globalDir, FileNameModDatabase)); os.IsNotExist(err) {
		missing = true
	}
	if !missing {
		return
	}

	app.appendLog(app.msg("sort_files_missing_short"))

	// Чтение настройки под мьютексом
	app.cfgMutex.RLock()
	skipPrompt := app.cfg.SkipSortFilesPrompt
	app.cfgMutex.RUnlock()

	if skipPrompt {
		app.appendLog(app.msg("download_skip_forever"))
		return
	}

	// Асинхронный диалог - не блокирует поток
	app.showChoiceDialog(
		app.mainWindow,
		app.msg("sort_files_missing"),
		app.msg("sort_files_missing_open_page"),
		func(choice int) {
			switch choice {
			case 0:
				u, _ := url.Parse(ServoMQModPage)
				_ = app.myApp.OpenURL(u)
				app.appendLog(app.msg("please_download_mod_db_install"))
			case 2:
				app.cfgMutex.Lock()
				app.cfg.SkipSortFilesPrompt = true
				app.cfgMutex.Unlock()
				saveConfig(app.cfg)
				fallthrough
			case 1:
				app.appendLog(app.msg("download_skipped"))
			}
		},
		app.msg("btn_yes"),
		app.msg("skip"),
		app.msg("download_skip_forever"),
	)
}

func getProgramArchivePattern() string {
	switch runtime.GOOS {
	case "windows":
		return "Servo Modquisitor 2 Windows"
	case "linux":
		return "Servo Modquisitor 2 Linux"
	default:
		return "Servo Modquisitor 2"
	}
}

// checkSpecialUpdates проверяет наличие новых версий программы и файлов сортировки (мод 139).
func (app *App) checkSpecialUpdates() {
	// Проверяем, авторизован ли пользователь
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("log_spec_update_not_logged"))
		return
	}

	// Проверка программы
	programFileInfo, err := app.getLatestFileInfoForMod(139, getProgramArchivePattern())
	if err != nil {
		app.logNexusError(err, "Program", app.msg("program_update_unavailable"))
	} else if programFileInfo != nil {
		if saved, ok := app.getCachedVersion(NexusCacheKeyProgram); ok {
			// сравниваем, но не обновляем кэш
			if compareVersions(programFileInfo.Version, saved.Version) > 0 {
				app.appendLog(fmt.Sprintf(app.msg("log_new_program_version_available"),
					programFileInfo.Version, saved.Version))
			}
		} else {
			// Просто пропускаем, не создаём запись
			app.appendLog(app.msg("log_program_not_cached"))
		}
	}

	// Проверка файлов сортировки
	rulesFileInfo, err := app.getLatestFileInfoForMod(139, "Mod DB And Sorting Rules")
	if err != nil {
		app.logNexusError(err, "Rules", app.msg("rules_update_unavailable"))
	} else if rulesFileInfo != nil {
		if saved, ok := app.getCachedVersion(NexusCacheKeyRules); ok {
			if compareVersions(rulesFileInfo.Version, saved.Version) > 0 {
				app.appendLog(fmt.Sprintf(app.msg("log_new_sorting_files_available"),
					rulesFileInfo.Version, saved.Version))
			}
		} else {
			// Просто пропускаем, не создаём запись
			app.appendLog(app.msg("log_sorting_rules_not_cached"))
		}
	}

	// Обновляем время последней проверки под мьютексом
	app.cfgMutex.Lock()
	app.cfg.LastUpdateCheck = time.Now().Format(time.RFC3339)
	app.cfgMutex.Unlock()
	app.saveConfigSafe()
}
