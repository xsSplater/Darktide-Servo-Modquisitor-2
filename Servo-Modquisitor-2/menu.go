// menu.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"fmt"
	"net/url"

	"fyne.io/fyne/v2"
)

func (app *App) buildMainMenu() *fyne.MainMenu {
	langEng := fyne.NewMenuItem(app.messages["menu_lang_en"], func() { app.changeLanguage("en") })
	langRu := fyne.NewMenuItem(app.messages["menu_lang_ru"], func() { app.changeLanguage("ru") })
	langMenu := fyne.NewMenuItem(app.messages["menu_language"], nil)
	langMenu.ChildMenu = fyne.NewMenu("", langEng, langRu)

	themeDark := fyne.NewMenuItem(app.messages["menu_theme_dark"], func() {
		app.cfg.Theme = "dark"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
		app.refreshThemeColors()
	})
	themeLight := fyne.NewMenuItem(app.messages["menu_theme_light"], func() {
		app.cfg.Theme = "light"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
		app.refreshThemeColors()
	})
	themeMenu := fyne.NewMenuItem(app.messages["menu_theme"], nil)
	themeMenu.ChildMenu = fyne.NewMenu("", themeDark, themeLight)

	dateYYYY := fyne.NewMenuItem(app.messages["menu_date_format_yyyy_mm_dd"], func() {
		app.cfg.DateFormat = "yyyy-mm-dd"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateMMDD := fyne.NewMenuItem(app.messages["menu_date_format_mm_dd_yyyy"], func() {
		app.cfg.DateFormat = "mm-dd-yyyy"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateDDMM := fyne.NewMenuItem(app.messages["menu_date_format_dd_mm_yyyy"], func() {
		app.cfg.DateFormat = "dd-mm-yyyy"
		saveConfig(app.cfg)
		app.refreshModList()
	})
	dateMenu := fyne.NewMenuItem(app.messages["menu_date_format"], nil)
	dateMenu.ChildMenu = fyne.NewMenu("", dateYYYY, dateMMDD, dateDDMM)

	// Force English Mod Names
	forceEnglishLabel := app.messages["setting_force_english_mod_names"]
	if app.cfg.ForceEnglishModNames {
		forceEnglishLabel = "✅ " + forceEnglishLabel
	} else {
		forceEnglishLabel = "❌ " + forceEnglishLabel
	}
	forceEnglishItem := fyne.NewMenuItem(forceEnglishLabel, func() {
		app.cfg.ForceEnglishModNames = !app.cfg.ForceEnglishModNames
		saveConfig(app.cfg)
		app.refreshModList()
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})
	showListAfterSortLabel := app.messages["menu_show_list_after_sort"]
	if app.cfg.ShowModListAfterSort {
		showListAfterSortLabel = "✅ " + showListAfterSortLabel
	} else {
		showListAfterSortLabel = "❌ " + showListAfterSortLabel
	}
	showListAfterSortItem := fyne.NewMenuItem(showListAfterSortLabel, func() {
		app.cfg.ShowModListAfterSort = !app.cfg.ShowModListAfterSort
		saveConfig(app.cfg)
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	// --- Меню Nexus ---
	// Пункт входа/выхода
	oauthActionLabel := app.messages["menu_nexus_login"]
	if app.cfg.OAuthAccessToken != "" {
		oauthActionLabel = app.messages["menu_nexus_logout"]
	}
	oauthActionItem := fyne.NewMenuItem(oauthActionLabel, func() {
		if app.cfg.OAuthAccessToken != "" {
			app.logoutOAuth()
		} else {
			app.startOAuthFlow()
		}
	})

	nexusAPIKeyItem := fyne.NewMenuItem(app.messages["menu_nexus_api_key"], func() {
		app.showNexusAPIKeyDialog()
	})

	nexusMenu := fyne.NewMenu(app.messages["menu_nexus"],
		oauthActionItem,
		nexusAPIKeyItem,
	)

	// ---- Меню Обновления ----
	freqNames := map[string]string{
		"every_start": app.messages["freq_every_start"],
		"weekly":      app.messages["freq_weekly"],
		"monthly":     app.messages["freq_monthly"],
		"yearly":      app.messages["freq_yearly"],
		"never":       app.messages["freq_never"],
	}
	freqItems := make([]*fyne.MenuItem, 0, len(freqNames))
	for freq, name := range freqNames {
		freqCopy := freq
		freqItems = append(freqItems, fyne.NewMenuItem(name, func() {
			app.cfg.UpdateCheckFrequency = freqCopy
			saveConfig(app.cfg)
			app.appendLog(fmt.Sprintf(app.messages["log_update_check_frequency_set"], name))
		}))
	}
	periodicSub := fyne.NewMenuItem(app.messages["menu_periodic_check"], nil)
	periodicSub.ChildMenu = fyne.NewMenu("", freqItems...)

	updateProgram := fyne.NewMenuItem(app.messages["menu_update_program"], func() {
		go app.checkForProgramUpdate() // существующая функция открытия Nexus
	})
	updateProgramAuto := fyne.NewMenuItem(app.messages["menu_update_program_auto"], func() {
		go app.updateProgramFromGitHub()
	})
	updateSortFiles := fyne.NewMenuItem(app.messages["menu_update_sort_files"], func() {
		go app.updateSortFiles()
	})

	updatesMenu := fyne.NewMenu(app.messages["menu_updates"],
		updateProgram,
		fyne.NewMenuItemSeparator(),
		updateProgramAuto,
		updateSortFiles,
		fyne.NewMenuItemSeparator(),
		periodicSub,
	)

	// Контакты
	contactGitHub := fyne.NewMenuItem(app.messages["menu_github"], func() {
		u, _ := url.Parse(GitHubRepoSMQ)
		_ = app.myApp.OpenURL(u)
	})
	contactDiscordMy := fyne.NewMenuItem(app.messages["menu_discord_my"], func() {
		u, _ := url.Parse(DiscordDTMy)
		_ = app.myApp.OpenURL(u)
	})
	contactDiscord := fyne.NewMenuItem(app.messages["menu_discord_dtmoddrs"], func() {
		u, _ := url.Parse(DiscordDTModders)
		_ = app.myApp.OpenURL(u)
	})
	contactMenu := fyne.NewMenu(app.messages["menu_contact"], contactGitHub, contactDiscord, contactDiscordMy)

	// Show DMLoader and DMFramework
	showSystemLabel := app.messages["setting_show_system_mods"]
	if app.cfg.ShowSystemMods {
		showSystemLabel = "✅ " + showSystemLabel
	} else {
		showSystemLabel = "❌ " + showSystemLabel
	}
	showSystemItem := fyne.NewMenuItem(showSystemLabel, func() {
		app.cfg.ShowSystemMods = !app.cfg.ShowSystemMods
		saveConfig(app.cfg)
		if app.systemModsTableContainer != nil {
			if app.cfg.ShowSystemMods {
				app.systemModsTableContainer.Show()
			} else {
				app.systemModsTableContainer.Hide()
			}
		}
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	// Настройки
	settingsMenu := fyne.NewMenu(app.messages["menu_settings"],
		langMenu,
		themeMenu,
		dateMenu,
		fyne.NewMenuItemSeparator(),
		forceEnglishItem,
		showListAfterSortItem,
		showSystemItem,
		fyne.NewMenuItemSeparator(),
	)

	return fyne.NewMainMenu(settingsMenu, nexusMenu, updatesMenu, contactMenu)
}

func (app *App) changeLanguage(lang string) {
	if err := app.loadLanguage(lang); err != nil {
		return
	}
	app.cfg.Language = lang
	checks.SetLanguage(lang)
	saveConfig(app.cfg)
	app.mainWindow.SetTitle(app.messages["app_title_long"])
	app.mainWindow.SetMainMenu(app.buildMainMenu())
	sorter.SetSortMessages(app.messages["sort_ru_warning"], app.messages["sort_en_warning"])
	sorter.SetLogMessages(app.messages["log_create_mlot"], app.messages["log_mlot_created"])

	// Проверки на nil для виджетов, которые создаются в buildUI
	if app.searchEntry != nil {
		app.searchEntry.SetPlaceHolder(app.messages["search_placeholder"])
	}
	if app.filterSelect != nil {
		app.filterSelect.Options = []string{
			app.messages["filter_all"], app.messages["filter_active"], app.messages["filter_inactive"],
			app.messages["filter_obsolete"], app.messages["filter_conflict"],
		}
		app.filterSelect.SetSelected(app.messages["filter_all"])
		app.filterSelect.Refresh()
	}
	if app.btnToggle != nil {
		app.updateToggleButtonText(app.btnToggle)
	}
	app.refreshModList() // filterModList внутри имеет проверку counterLabel != nil

	// Текстовые метки
	if app.filterLabel != nil {
		app.filterLabel.SetText(app.messages["filter_label"])
	}
	// Кнопки, которые были в правой панели и теперь в верхней
	if app.btnSaveOrder != nil {
		app.btnSaveOrder.SetText(app.messages["btn_save_order"])
	}
	if app.btnSortChecks != nil {
		app.btnSortChecks.SetText(app.messages["btn_sort_checks"])
	}
	if app.btnRefresh != nil {
		app.btnRefresh.SetText(app.messages["btn_refresh"])
	}
	if app.btnInstall != nil {
		app.btnInstall.SetText(app.messages["btn_install"])
	}
	if app.btnRemove != nil {
		app.btnRemove.SetText(app.messages["btn_remove"])
	}
	if app.btnUp != nil {
		app.btnUp.SetText(app.messages["btn_up"])
	}
	if app.btnDown != nil {
		app.btnDown.SetText(app.messages["btn_down"])
	}
	// Новые кнопки быстрого перемещения
	if app.moveToTopBtn != nil {
		app.moveToTopBtn.SetText(app.messages["btn_move_to_top"])
	}
	if app.moveToBottomBtn != nil {
		app.moveToBottomBtn.SetText(app.messages["btn_move_to_bottom"])
	}
	if app.moveToEntry != nil {
		app.moveToEntry.SetPlaceHolder("##") // неизменяемое
	}
	if app.moveLabel != nil {
		app.moveLabel.SetText(app.messages["lbl_move_to"])
	}
	// Кнопки выделения
	if app.selectAllBtn != nil {
		app.selectAllBtn.SetText(app.messages["btn_select_all"])
	}
	if app.deselectAllBtn != nil {
		app.deselectAllBtn.SetText(app.messages["btn_deselect_all"])
	}
	if app.enableSelectedBtn != nil {
		app.enableSelectedBtn.SetText(app.messages["btn_enable_selected"])
	}
	if app.disableSelectedBtn != nil {
		app.disableSelectedBtn.SetText(app.messages["btn_disable_selected"])
	}
	// Кнопки массового включения/выключения
	if app.enableAllBtn != nil {
		app.enableAllBtn.SetText(app.messages["btn_enable_all_mods"])
	}
	if app.disableAllBtn != nil {
		app.disableAllBtn.SetText(app.messages["btn_disable_all_mods"])
	}
	// Кнопки удаления модов
	if app.removeAllBtn != nil {
		app.removeAllBtn.SetText(app.messages["btn_remove_all_mods"])
	}
	if app.removeSelectedBtn != nil {
		app.removeSelectedBtn.SetText(app.messages["btn_remove_selected"])
	}
	// Кнопки запуска
	app.updateLaunchButtonTexts()
	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.manageBtn != nil {
		app.manageBtn.SetText(app.messages["btn_manage_mods"])
	}
	// добавить:
	if app.btnCheckUpdates != nil {
		app.btnCheckUpdates.SetText(app.messages["btn_check_updates"])
	}
	if app.btnUpdateMod != nil {
		app.btnUpdateMod.SetText(app.messages["btn_update_mod"])
	}
	if app.btnUpdateAll != nil {
		app.btnUpdateAll.SetText(app.messages["btn_update_all"])
	}

	// Обновляем заголовок консоли
	if app.logHeaderText != nil {
		app.logHeaderText.Text = app.messages["log_start0"]
		app.logHeaderText.Refresh()
	}

	app.applyTooltip(app.removeSelectedBtn, "btn_remove_selected_tooltip")
	app.applyTooltip(app.removeAllBtn, "btn_remove_all_tooltip")
	app.applyTooltip(app.btnCheckUpdates, "btn_check_updates_tooltip")
	app.applyTooltip(app.btnUpdateMod, "btn_update_mod_tooltip")
	app.applyTooltip(app.btnUpdateAll, "btn_update_all_tooltip")

	app.reapplyTooltips()
	app.updateDescriptionForMod(app.selectedModName)
}

func (app *App) updateLaunchButtonTexts() {
	if app.btnLaunchNormal != nil {
		app.btnLaunchNormal.SetText(app.messages["btn_launch_game"])
	}
	if app.btnLaunchNoLauncher != nil {
		app.btnLaunchNoLauncher.SetText(app.messages["btn_launch_nolauncher_long"])
	}
}

func (app *App) reapplyTooltips() {
	app.applyTooltip(app.btnSaveOrder, "btn_save_order_tooltip")
	app.applyTooltip(app.btnRefresh, "btn_refresh_tooltip")
	app.applyTooltip(app.btnInstall, "btn_install_tooltip")
	app.applyTooltip(app.btnRemove, "btn_remove_tooltip")
	app.applyTooltip(app.btnUp, "btn_up_tooltip")
	app.applyTooltip(app.btnDown, "btn_down_tooltip")
	app.applyTooltip(app.btnSortChecks, "btn_sort_checks_tooltip")
	app.applyTooltip(app.btnToggle, "btn_toggle_tooltip")
	app.applyTooltip(app.btnLaunchNormal, "btn_launch_game_tooltip")
	app.applyTooltip(app.btnLaunchNoLauncher, "btn_launch_nolauncher_long_tooltip")
	app.applyTooltip(app.moveToTopBtn, "btn_move_to_top_tooltip")
	app.applyTooltip(app.moveToBottomBtn, "btn_move_to_bottom_tooltip")
	app.applyTooltip(app.selectAllBtn, "btn_select_all_tooltip")
	app.applyTooltip(app.deselectAllBtn, "btn_deselect_all_tooltip")
	app.applyTooltip(app.enableSelectedBtn, "btn_enable_selected_tooltip")
	app.applyTooltip(app.disableSelectedBtn, "btn_disable_selected_tooltip")
	app.applyTooltip(app.enableAllBtn, "btn_enable_all_tooltip")
	app.applyTooltip(app.disableAllBtn, "btn_disable_all_tooltip")
	app.applyTooltip(app.manageBtn, "btn_manage_mods_tooltip")
	app.applyTooltip(app.removeAllBtn, "btn_remove_all_tooltip")
	app.applyTooltip(app.removeSelectedBtn, "btn_remove_selected_tooltip")
}
