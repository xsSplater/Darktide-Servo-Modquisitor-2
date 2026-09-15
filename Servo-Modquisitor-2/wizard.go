// Servo-Modquisitor-2/wizard.go
package main

import (
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// runWizard запускает мастер установки модов.
// Если force == true, мастер показывается принудительно, игнорируя флаги и проверки установки DML/DMF.
func (app *App) runWizard(force bool) {
	if !force {
		if app.cfg.FirstRunWizardDisabled {
			return
		}
		if app.cfg.WizardHelpInstalled {
			return
		}
		// Если DML и DMF уже установлены, мастер не нужен
		if app.checkDMLInstalled() && app.checkDMFInstalled() {
			app.cfgMutex.Lock()
			app.cfg.WizardHelpInstalled = true
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			return
		}
	}

	// Запускаем мастер (первый шаг)
	result := make(chan bool, 1)
	fyne.DoAndWait(func() {
		app.showWelcomeStep(result)
	})
	// Результат можно игнорировать
}

// promptRunWizard показывает диалог с предложением запустить мастер установки.
func (app *App) promptRunWizard() {
	app.showChoiceDialog(
		app.mainWindow,
		app.msg("wizard_no_dmlf"),
		app.msg("wizard_no_dmlf_desc"),
		func(choice int) {
			if choice == 0 {
				go app.runWizard(true) // асинхронно, чтобы избежать deadlock
			}
		},
		app.msg("wizard_no_dmlf_btn_yes"),
		app.msg("wizard_btn_no"),
	)
}

// checkDMLInstalled проверяет наличие папки base в mods
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

// checkDMFInstalled проверяет наличие папки dmf в mods
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

// checkAutopatcherInstalled проверяет наличие autopatcher в корне игры
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

// showWelcomeStep показывает приветственное окно мастера.
func (app *App) showWelcomeStep(result chan bool) {
	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("wizard_first_run"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_first_run_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	btnYes := widget.NewButton(app.msg("wizard_first_run_btn_yes"), func() {
		popUp.Hide()
		app.showNexusStep(result)
	})
	btnNo := widget.NewButton(app.msg("wizard_btn_no"),
		func() {
			popUp.Hide()
			result <- false
		})
	btnNever := widget.NewButton(app.msg("wizard_first_run_btn_dont_show"), func() {
		popUp.Hide()
		app.cfgMutex.Lock()
		app.cfg.FirstRunWizardDisabled = true
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
		result <- false
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnYes, btnNo, btnNever)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(500, 300))
	popUp.Show()
}

// showNexusStep показывает шаг входа в Nexus.
func (app *App) showNexusStep(result chan bool) {
	if app.isLoggedIn() {
		app.showDMLStep(result)
		return
	}

	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("menu_nexus_login"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_nexus_login_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	statusLabel := widget.NewLabel("")
	statusLabel.Hide()

	btnLogin := widget.NewButton(app.msg("menu_nexus")+" 🔗", func() {
		statusLabel.SetText(app.msg("log_open_nexus_page"))
		statusLabel.Show()
		go app.startOAuthFlow()
		go func() {
			for i := 0; i < 30; i++ {
				time.Sleep(500 * time.Millisecond)
				if app.isLoggedIn() {
					fyne.Do(func() {
						statusLabel.SetText(app.msg("oauth_success_title"))
						statusLabel.Importance = widget.SuccessImportance
						time.AfterFunc(1500*time.Millisecond, func() {
							popUp.Hide()
							app.showDMLStep(result)
						})
					})
					return
				}
			}
			fyne.Do(func() {
				statusLabel.SetText(app.msg("wizard_nexus_login_fail"))
				statusLabel.Importance = widget.WarningImportance
			})
		}()
	})

	btnManual := widget.NewButton(app.msg("wizard_nexus_btn_manual"), func() {
		popUp.Hide()
		app.showDMLStep(result)
	})

	btnClose := widget.NewButton(app.msg("wizard_btn_close"), func() {
		popUp.Hide()
		result <- false
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		statusLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnLogin, btnManual, btnClose)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(550, 320))
	popUp.Show()
}

// showDMLStep показывает шаг установки DML.
func (app *App) showDMLStep(result chan bool) {
	if app.checkDMLInstalled() {
		app.showDMFStep(result)
		return
	}

	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("wizard_install_dml"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_install_dml_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	var instructionLabel *widget.Label
	if app.isLoggedIn() {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_mmd"))
	} else {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_man"))
	}
	instructionLabel.Wrapping = fyne.TextWrapWord

	statusLabel := widget.NewLabel(app.msg("wizard_install_waiting_dml"))
	statusLabel.Hide()
	statusLabel.Importance = widget.WarningImportance

	btnOpen := widget.NewButton(app.msg("btn_open_dml_page"), func() {
		u, _ := url.Parse(DarktideModDML)
		_ = app.myApp.OpenURL(u)
		statusLabel.SetText(app.msg("wizard_install_waiting_dml"))
		statusLabel.Show()
		go app.waitForDML(statusLabel, popUp, result)
	})

	btnClose := widget.NewButton(app.msg("wizard_btn_close"), func() {
		popUp.Hide()
		result <- false
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		instructionLabel,
		statusLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnOpen, btnClose)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(550, 350))
	popUp.Show()
}

// waitForDML периодически проверяет наличие папки base.
func (app *App) waitForDML(statusLabel *widget.Label, popUp *widget.PopUp, result chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if app.checkDMLInstalled() {
			app.refreshAfterSystemInstall()
			fyne.Do(func() {
				statusLabel.SetText(app.msg("wizard_install_dml_succ"))
				statusLabel.Importance = widget.SuccessImportance
				time.AfterFunc(1500*time.Millisecond, func() {
					popUp.Hide()
					app.showDMFStep(result)
				})
			})
			return
		}
	}
}

// showDMFStep показывает шаг установки DMF.
func (app *App) showDMFStep(result chan bool) {
	if app.checkDMFInstalled() {
		app.showAutopatcherStep(result)
		return
	}

	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("wizard_install_dmf"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_install_dmf_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	var instructionLabel *widget.Label
	if app.isLoggedIn() {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_mmd"))
	} else {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_man"))
	}
	instructionLabel.Wrapping = fyne.TextWrapWord

	statusLabel := widget.NewLabel(app.msg("wizard_install_waiting_dmf"))
	statusLabel.Hide()
	statusLabel.Importance = widget.WarningImportance

	btnOpen := widget.NewButton(app.msg("btn_open_dmf_page"), func() {
		u, _ := url.Parse(DarktideModDMF)
		_ = app.myApp.OpenURL(u)
		statusLabel.SetText(app.msg("wizard_install_waiting_dmf"))
		statusLabel.Show()
		go app.waitForDMF(statusLabel, popUp, result)
	})

	btnClose := widget.NewButton(app.msg("wizard_btn_close"), func() {
		popUp.Hide()
		result <- false
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		instructionLabel,
		statusLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnOpen, btnClose)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(550, 350))
	popUp.Show()
}

// waitForDMF периодически проверяет наличие папки dmf.
func (app *App) waitForDMF(statusLabel *widget.Label, popUp *widget.PopUp, result chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if app.checkDMFInstalled() {
			fyne.Do(func() {
				statusLabel.SetText(app.msg("wizard_install_dmf_succ"))
				statusLabel.Importance = widget.SuccessImportance
				time.AfterFunc(1500*time.Millisecond, func() {
					popUp.Hide()
					app.showAutopatcherStep(result)
				})
			})
			return
		}
	}
}

// showAutopatcherStep показывает шаг установки Autopatcher.
func (app *App) showAutopatcherStep(result chan bool) {
	if app.checkAutopatcherInstalled() {
		app.showCongratulationsStep(result)
		return
	}

	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("wizard_install_dtap"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_install_dtap_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	var instructionLabel *widget.Label
	if app.isLoggedIn() {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_mmd"))
	} else {
		instructionLabel = widget.NewLabel(app.msg("wizard_install_dml_instr_man"))
	}
	instructionLabel.Wrapping = fyne.TextWrapWord

	statusLabel := widget.NewLabel(app.msg("wizard_install_waiting_dtap"))
	statusLabel.Hide()
	statusLabel.Importance = widget.WarningImportance

	btnOpen := widget.NewButton(app.msg("btn_open_dtap_page"), func() {
		u, _ := url.Parse(DarktideAPPage)
		_ = app.myApp.OpenURL(u)
		statusLabel.SetText(app.msg("wizard_install_waiting_dtap"))
		statusLabel.Show()
		go app.waitForAutopatcher(statusLabel, popUp, result)
	})

	btnSkip := widget.NewButton(app.msg("skip"), func() {
		popUp.Hide()
		app.showCongratulationsStep(result)
	})

	btnClose := widget.NewButton(app.msg("wizard_btn_close"), func() {
		popUp.Hide()
		result <- false
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		instructionLabel,
		statusLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnOpen, btnSkip, btnClose)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(550, 380))
	popUp.Show()
}

// waitForAutopatcher периодически проверяет наличие autopatcher.
func (app *App) waitForAutopatcher(statusLabel *widget.Label, popUp *widget.PopUp, result chan bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if app.checkAutopatcherInstalled() {
			fyne.Do(func() {
				statusLabel.SetText(app.msg("wizard_install_dtap_succ"))
				statusLabel.Importance = widget.SuccessImportance
				time.AfterFunc(1500*time.Millisecond, func() {
					popUp.Hide()
					app.showCongratulationsStep(result)
				})
			})
			return
		}
	}
}

// showCongratulationsStep показывает финальный экран мастера.
func (app *App) showCongratulationsStep(result chan bool) {
	var popUp *widget.PopUp
	titleLabel := widget.NewLabelWithStyle(app.msg("wizard_succ"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	msgLabel := widget.NewLabel(app.msg("wizard_succ_desc"))
	msgLabel.Wrapping = fyne.TextWrapWord

	btnTopMods := widget.NewButton(app.msg("wizard_succ_btn_best_mods"), func() {
		u, _ := url.Parse("https://www.nexusmods.com/games/warhammer40kdarktide/mods?sort=endorsements")
		_ = app.myApp.OpenURL(u)
	})

	btnFinish := widget.NewButton(app.msg("wizard_btn_close"), func() {
		popUp.Hide()
		app.cfgMutex.Lock()
		app.cfg.WizardHelpInstalled = true
		app.cfgMutex.Unlock()
		app.saveConfigSafe()
		app.refreshAfterSystemInstall()
		result <- true
	})

	content := container.NewVBox(
		titleLabel,
		widget.NewSeparator(),
		msgLabel,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(btnTopMods, btnFinish)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(600, 400))
	popUp.Show()
}

// refreshAfterSystemInstall вызывается после успешной установки любого
// системного компонента (DML/DMF/Autopatcher). Пересчитывает patcherType
// по факту наличия файлов на диске, обновляет состояние mods-enabled и
// перестраивает список модов — чтобы пользователь увидел autopatch в
// таблице без перезапуска программы.
//
// Порядок блокировок: gameRootMutex берётся первым, потом modsMutex (внутри
// refreshModList), потом cfgMutex (внутри syncModsEnabledState).
func (app *App) refreshAfterSystemInstall() {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		return
	}
	// Пересчитываем patcherType — autopatch мог только что появиться.
	app.setGameState(gameRoot, detectPatcherTypeWithRoot(gameRoot))
	// Обновляем флаг глобального режима модов под новый patcher.
	app.syncModsEnabledState()
	// Перестраиваем UI в главном потоке: список модов, кнопка toggle.
	fyne.Do(func() {
		app.refreshModList()
		app.updateToggleButtonText(app.btnToggle)
	})
}
