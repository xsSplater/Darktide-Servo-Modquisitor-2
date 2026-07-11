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
	// --- Меню выбора языка ---
	// Список всех поддерживаемых языков
	languageCodes := []string{
		"en", "ru", "zh-hans", "zh-hant", "de", "fr", "ja", "ko", "it", "pl", "es", "pt-BR",
	}
	var langItems []*fyne.MenuItem
	for _, code := range languageCodes {
		// Ключ сообщения для названия языка (например "menu_lang_en")
		msgKey := "menu_lang_" + code
		label := app.messages[msgKey]
		if label == "" {
			label = code // fallback на код, если перевод отсутствует
		}
		if app.cfg.Language == code {
			label = "✅ " + label
		} else {
			label = "❌ " + label
		}
		codeCopy := code
		item := fyne.NewMenuItem(label, func() {
			app.changeLanguage(codeCopy)
		})
		langItems = append(langItems, item)
	}
	langMenu := fyne.NewMenuItem(app.messages["menu_language"], nil)
	langMenu.ChildMenu = fyne.NewMenu("", langItems...)

	// Создаём пункты меню с маркерами
	themeDark := fyne.NewMenuItem("", func() {
		app.cfg.Theme = "dark"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
		app.refreshThemeColors()
		app.mainWindow.SetMainMenu(app.buildMainMenu()) // <-- обновляем меню
	})
	themeLight := fyne.NewMenuItem("", func() {
		app.cfg.Theme = "light"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
		app.refreshThemeColors()
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})
	themeHighContrast := fyne.NewMenuItem("", func() {
		app.cfg.Theme = "highcontrast"
		saveConfig(app.cfg)
		app.myApp.Settings().SetTheme(&themes.HighContrastTheme{})
		app.refreshThemeColors()
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	// Устанавливаем текст с маркером
	markTheme := func(label string, current string, target string) string {
		if current == target {
			return "✅ " + label
		}
		return "❌ " + label
	}
	themeDark.Label = markTheme(app.messages["menu_theme_dark"], app.cfg.Theme, "dark")
	themeLight.Label = markTheme(app.messages["menu_theme_light"], app.cfg.Theme, "light")
	themeHighContrast.Label = markTheme(app.messages["menu_theme_highcontrast"], app.cfg.Theme, "highcontrast")
	themeMenu := fyne.NewMenuItem(app.messages["menu_theme"], nil)
	themeMenu.ChildMenu = fyne.NewMenu("", themeDark, themeLight, themeHighContrast)

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

	nexusMenu := fyne.NewMenu(app.messages["menu_nexus"],
		oauthActionItem,
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

	openSMQNexusPage := fyne.NewMenuItem(app.messages["menu_open_smq_page"], func() {
		go app.initiateSortFilesUpdate()
	})

	updatesMenu := fyne.NewMenu(app.messages["menu_updates"],
		openSMQNexusPage,
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

	// Guides
	guidesItem := fyne.NewMenuItem(app.messages["menu_guides_video_hti"], func() {
		u, _ := url.Parse(YouTubeGuideURL)
		_ = app.myApp.OpenURL(u)
	})
	guidesMenu := fyne.NewMenu(app.messages["menu_guides"], guidesItem)

	// Меню поддержки (Donate)
	donateBoosty := fyne.NewMenuItem(app.messages["menu_boosty"], func() {
		u, _ := url.Parse(DonateBoostyURL)
		_ = app.myApp.OpenURL(u)
	})
	donateDonationAlerts := fyne.NewMenuItem(app.messages["menu_donationalerts"], func() {
		u, _ := url.Parse(DonateDonationAlertsURL)
		_ = app.myApp.OpenURL(u)
	})
	donateSteamGift := fyne.NewMenuItem(app.messages["menu_steam_gift"], func() {
		u, _ := url.Parse("https://steamcommunity.com/id/xssplater/")
		_ = app.myApp.OpenURL(u)
	})
	donateCard := fyne.NewMenuItem(app.messages["menu_card"], func() {
		app.myApp.Clipboard().SetContent(DonateCardNumber)
		app.tooltipStatus.Show(app.messages["card_copied_tip"])
		app.tooltipStatus.HideAfterDelay()
	})

	donateMenu := fyne.NewMenu(app.messages["menu_donate"],
		donateBoosty,
		donateDonationAlerts,
		donateSteamGift,
		donateCard,
	)

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
	// Строка меню
	return fyne.NewMainMenu(settingsMenu, nexusMenu, updatesMenu, contactMenu, guidesMenu, donateMenu)
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
	if app.btnAMLConfig != nil { // AML
		app.btnAMLConfig.SetText(app.messages["btn_aml_config"])
		app.applyTooltip(app.btnAMLConfig, "btn_aml_config_tooltip")
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
		app.applyTooltip(app.removeAllBtn, "btn_remove_all_tooltip")
	}
	if app.removeSelectedBtn != nil {
		app.removeSelectedBtn.SetText(app.messages["btn_remove_selected"])
		app.applyTooltip(app.removeSelectedBtn, "btn_remove_selected_tooltip")
	}
	// Кнопки запуска
	app.updateLaunchButtonTexts()
	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.manageBtn != nil {
		app.manageBtn.SetText(app.messages["btn_manage_mods"])
	}
	if app.btnCheckUpdates != nil {
		app.btnCheckUpdates.SetText(app.messages["btn_check_updates"])
		app.applyTooltip(app.btnCheckUpdates, "btn_check_updates_tooltip")
	}
	if app.btnUpdateMod != nil {
		app.btnUpdateMod.SetText(app.messages["btn_update_mod"])
		app.applyTooltip(app.btnUpdateMod, "btn_update_mod_tooltip")
	}
	if app.btnUpdateAll != nil {
		app.btnUpdateAll.SetText(app.messages["btn_update_all"])
		app.applyTooltip(app.btnUpdateAll, "btn_update_all_tooltip")
	}

	if app.btnEditVersion != nil {
		app.btnEditVersion.SetText(app.messages["btn_edit_version"])
		app.applyTooltip(app.btnEditVersion, "btn_edit_version_tooltip")
	}

	// Обновляем заголовок консоли
	if app.logHeaderText != nil {
		app.logHeaderText.Text = app.messages["log_start0"]
		app.logHeaderText.Refresh()
	}

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
	app.applyTooltip(app.btnAMLConfig, "btn_aml_config_tooltip")
}
