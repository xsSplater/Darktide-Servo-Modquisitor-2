// Servo-Modquisitor-2/wizard.go
package main

import (
	"Servo-Modquisitor/checks"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
)

// runWizard запускает мастер установки компонентов.
// force == true — показать мастер принудительно, игнорируя флаги.

func (app *App) runWizard(force bool) {
	if !force {
		if app.cfg.FirstRunWizardDisabled {
			return
		}
		if app.cfg.WizardHelpInstalled {
			return
		}
		// Если DML и DMF уже установлены, мастер не нужен.
		if app.checkDMLInstalled() && app.checkDMFInstalled() {
			app.cfgMutex.Lock()
			app.cfg.WizardHelpInstalled = true
			app.cfgMutex.Unlock()
			app.saveConfigSafe()
			return
		}
	}

	// ─── ПРОВЕРКА АВТОРИЗАЦИИ ──────────────────────────────────────
	// Предлагаем войти в аккаунт сразу, чтобы не заставлять пользователя
	// делать это в середине процесса установки.
	if !app.isLoggedIn() {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.msg("wizard_mi"),
			app.msg("wizard_nexus_login_desc"),
			app.msg("menu_nexus_login"), // Кнопка "Войти"
			app.msg("btn_continue"),     // Кнопка "Продолжить без входа"
		)
		if choice == 0 {
			// Запускаем OAuth-флоу. Он откроет браузер.
			app.startOAuthFlow()
			// Закрываем мастер. Пользователь сможет запустить его снова
			// из меню "Гайды", когда успешно авторизуется.
			return
		}
	}
	// ───────────────────────────────────────────────────────────────

	// Заголовок с защитой от пустой строки
	title := app.msg("wizard_mi")
	if title == "" {
		title = "Setup Wizard"
	}

	// Вспомогательная функция для получения данных из базы
	getMeta := func(folder, defaultAuthor, defaultCategory string) (string, string, string) {
		pic, author, cat := "", defaultAuthor, defaultCategory
		if db := checks.GetModDBEntry(folder); db != nil {
			pic = db.NexusPictureURL
			if db.Author != "" {
				author = db.Author
			}
			if db.Category != "" {
				cat = db.Category
			}
		}
		return pic, author, cat
	}

	picDML, authorDML, catDML := getMeta("base", "Darktide Modders", "Core")
	picDMF, authorDMF, catDMF := getMeta("dmf", "Darktide Modders", "Core")
	picAuto, authorAuto, catAuto := getMeta("autopatch", "Darktide Modders", "Tools")

	items := []*BatchItem{
		{
			ID:          "base",
			Name:        "Darktide Mod Loader",
			Description: "Основной загрузчик модов. Без него игра не загрузит ни один мод.",
			NexusModID:  19,
			Selected:    !app.checkDMLInstalled(),
			Status:      BatchPending,
			PictureURL:  picDML,
			Author:      authorDML,
			Category:    catDML,
		},
		{
			ID:          "dmf",
			Name:        "Darktide Mod Framework",
			Description: "Библиотека, необходимая большинству модов.",
			NexusModID:  8,
			Selected:    !app.checkDMFInstalled(),
			Status:      BatchPending,
			PictureURL:  picDMF,
			Author:      authorDMF,
			Category:    catDMF,
		},
		{
			ID:          "autopatch",
			Name:        "Darktide Mod Autopatcher",
			Description: "Автоматически патчит bundle_database.data при запуске игры.",
			NexusModID:  709,
			Selected:    !app.checkAutopatcherInstalled(),
			Status:      BatchPending,
			PictureURL:  picAuto,
			Author:      authorAuto,
			Category:    catAuto,
		},
	}

	// Подтягиваем FileID для Premium
	if app.getAuthToken() != "" {
		for _, item := range items {
			if fi, err := app.getLatestFileInfo(item.NexusModID); err == nil {
				item.FileID = fi.ID
				item.Version = fi.Version
			}
		}
	}

	fyne.Do(func() {
		app.showBatchDialog(title, items, BatchModeInstall)
	})
}

// promptRunWizard показывает диалог с предложением запустить мастер.
func (app *App) promptRunWizard() {
	app.showChoiceDialog(
		app.mainWindow,
		app.msg("wizard_no_dmlf"),
		app.msg("wizard_no_dmlf_desc"),
		func(choice int) {
			if choice == 0 {
				go app.runWizard(true)
			}
		},
		app.msg("wizard_no_dmlf_btn_yes"),
		app.msg("wizard_btn_no"),
	)
}

// checkDMLInstalled проверяет наличие папки base в mods.
func (app *App) checkDMLInstalled() bool {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	if modsPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(modsPath, "base"))
	return err == nil
}

// checkDMFInstalled проверяет наличие папки dmf в mods.
func (app *App) checkDMFInstalled() bool {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	if modsPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(modsPath, "dmf"))
	return err == nil
}

// checkAutopatcherInstalled проверяет наличие autopatcher в корне игры.
func (app *App) checkAutopatcherInstalled() bool {
	app.cfgMutex.RLock()
	gameRoot := app.cfg.GameRoot
	app.cfgMutex.RUnlock()
	if gameRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(gameRoot, "binaries", "plugins", "_dt_mod_autopatch.dll"))
	return err == nil
}

// refreshAfterSystemInstall вызывается после установки системного компонента.
// Пересчитывает patcherType и перестраивает UI.
func (app *App) refreshAfterSystemInstall() {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		return
	}
	app.setGameState(gameRoot, detectPatcherTypeWithRoot(gameRoot))
	app.syncModsEnabledState()
	fyne.Do(func() {
		app.refreshModList()
		app.updateToggleButtonText(app.btnToggle)
	})
}
