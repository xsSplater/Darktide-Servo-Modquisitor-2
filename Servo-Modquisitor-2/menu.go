// Servo-Modquisitor-2/menu.go
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
	// Use a single `cfg` snapshot for the entire function
	// otherwise, numerous reads of `app.cfg.*` will contend with background writes (such as `syncVersionCache`, etc.).
	app.cfgMutex.RLock()
	currentLang := app.cfg.Language
	currentTheme := app.cfg.Theme
	currentDateFormat := app.cfg.DateFormat
	currentUpdateFreq := app.cfg.UpdateCheckFrequency
	forceEnglish := app.cfg.ForceEnglishModNames
	showListAfterSort := app.cfg.ShowModListAfterSort
	showSystemMods := app.cfg.ShowSystemMods
	app.cfgMutex.RUnlock()

	// loadMenuIcon — Loading icons for menu items.
	loadMenuIcon := func(filename string) fyne.Resource {
		path := "assets/buttons/" + filename
		data, err := embeddedFiles.ReadFile(path)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("menu icon %q not found: %v", path, err))
			return nil
		}
		return fyne.NewStaticResource(filename, data)
	}

	// !!!!! Иконка-заглушка для пунктов, у которых пока нет своей !!!!!
	// wrenchIcon := loadMenuIcon("wrench.png")

	// SETTINGS MENU - Submenu "Language selection"
	languageCodes := []string{"de", "en", "es", "fr", "it", "ja", "ko", "pl", "pt-BR", "ru", "zh-hans", "zh-hant"}
	checkedIcon := loadMenuIcon("checked_box.png")
	uncheckedIcon := loadMenuIcon("unchecked_box.png")

	var langItems []*fyne.MenuItem
	for _, code := range languageCodes {
		msgKey := "menu_lang_" + code
		label := app.msg(msgKey)
		if label == "" {
			label = code
		}
		codeCopy := code
		item := fyne.NewMenuItem(label, func() {
			app.changeLanguage(codeCopy)
		})
		item.Checked = currentLang == code
		item.CheckedIcon = checkedIcon
		item.UncheckedIcon = uncheckedIcon
		langItems = append(langItems, item)
	}
	langMenu := fyne.NewMenuItemWithIcon(
		app.msg("menu_language"),
		loadMenuIcon("language.png"),
		nil)
	langMenu.ChildMenu = fyne.NewMenu("", langItems...)

	// SETTINGS MENU - Submenu Theme
	// Dark theme
	themeDark := fyne.NewMenuItemWithIcon(
		app.msg("menu_theme_dark"),
		loadMenuIcon("moon.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.Theme = "dark"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	themeDark.Checked = currentTheme == "dark"
	themeDark.CheckedIcon = checkedIcon
	themeDark.UncheckedIcon = uncheckedIcon
	// Light theme
	themeLight := fyne.NewMenuItemWithIcon(
		app.msg("menu_theme_light"),
		loadMenuIcon("sun.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.Theme = "light"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	themeLight.Checked = currentTheme == "light"
	themeLight.CheckedIcon = checkedIcon
	themeLight.UncheckedIcon = uncheckedIcon
	// High-contrast theme
	themeHighContrast := fyne.NewMenuItemWithIcon(
		app.msg("menu_theme_highcontrast"),
		loadMenuIcon("highnoon.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.Theme = "highcontrast"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.myApp.Settings().SetTheme(&themes.HighContrastTheme{})
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	themeHighContrast.Checked = currentTheme == "highcontrast"
	themeHighContrast.CheckedIcon = checkedIcon
	themeHighContrast.UncheckedIcon = uncheckedIcon
	// Custom theme
	themeCustomize := fyne.NewMenuItemWithIcon(
		app.msg("menu_theme_custom"),
		loadMenuIcon("palette.png"),
		func() {
			app.showThemeEditor()
		})
	themeCustomize.Checked = currentTheme == "custom"
	themeCustomize.CheckedIcon = checkedIcon
	themeCustomize.UncheckedIcon = uncheckedIcon

	themeMenu := fyne.NewMenuItemWithIcon(
		app.msg("menu_theme"),
		loadMenuIcon("themes.png"),
		nil)
	themeMenu.ChildMenu = fyne.NewMenu("", themeDark, themeLight, themeHighContrast, fyne.NewMenuItemSeparator(), themeCustomize)

	// SETTINGS MENU - Submenu Date format
	// YYYY-MM-DD
	dateYYYY := fyne.NewMenuItemWithIcon(
		app.msg("menu_date_format_yyyy_mm_dd"),
		loadMenuIcon("date_sort.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.DateFormat = "yyyy-mm-dd"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.refreshModList()
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	dateYYYY.Checked = currentDateFormat == "yyyy-mm-dd"
	dateYYYY.CheckedIcon = checkedIcon
	dateYYYY.UncheckedIcon = uncheckedIcon
	// MM-DD-YYYY
	dateMMDD := fyne.NewMenuItemWithIcon(
		app.msg("menu_date_format_mm_dd_yyyy"),
		loadMenuIcon("date_sort.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.DateFormat = "mm-dd-yyyy"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.refreshModList()
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	dateMMDD.Checked = currentDateFormat == "mm-dd-yyyy"
	dateMMDD.CheckedIcon = checkedIcon
	dateMMDD.UncheckedIcon = uncheckedIcon
	// DD-MM-YYYY
	dateDDMM := fyne.NewMenuItemWithIcon(
		app.msg("menu_date_format_dd_mm_yyyy"),
		loadMenuIcon("date_sort.png"),
		func() {
			app.cfgMutex.Lock()
			app.cfg.DateFormat = "dd-mm-yyyy"
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.refreshModList()
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
	dateDDMM.Checked = currentDateFormat == "dd-mm-yyyy"
	dateDDMM.CheckedIcon = checkedIcon
	dateDDMM.UncheckedIcon = uncheckedIcon

	dateMenu := fyne.NewMenuItemWithIcon(
		app.msg("menu_date_format"),
		loadMenuIcon("date_sort.png"),
		nil)
	dateMenu.ChildMenu = fyne.NewMenu("", dateYYYY, dateMMDD, dateDDMM)

	// SETTINGS MENU - Submenu Checking for program updates
	type freqItem struct {
		key   string
		label string
	}
	freqOrder := []freqItem{
		{"every_start", app.msg("freq_every_start")},
		{"weekly", app.msg("freq_weekly")},
		{"monthly", app.msg("freq_monthly")},
		{"yearly", app.msg("freq_yearly")},
		{"never", app.msg("freq_never")},
	}
	freqItems := make([]*fyne.MenuItem, 0, len(freqOrder))
	for _, fi := range freqOrder {
		freqCopy := fi.key
		name := fi.label
		item := fyne.NewMenuItem(name, func() {
			app.cfgMutex.Lock()
			app.cfg.UpdateCheckFrequency = freqCopy
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
			app.appendLog(fmt.Sprintf(app.msg("log_update_check_frequency_set"), name))
			app.mainWindow.SetMainMenu(app.buildMainMenu())
		})
		item.Checked = currentUpdateFreq == fi.key
		item.CheckedIcon = checkedIcon
		item.UncheckedIcon = uncheckedIcon
		freqItems = append(freqItems, item)
	}
	periodicSub := fyne.NewMenuItemWithIcon(
		app.msg("menu_periodic_check"),
		loadMenuIcon("check_updates_blue_time.png"),
		nil)
	periodicSub.ChildMenu = fyne.NewMenu("", freqItems...)

	// SETTINGS MENU - Force English Mod Names.
	forceEnglishItem := fyne.NewMenuItem(app.msg("setting_force_english_mod_names"), func() {
		app.cfgMutex.Lock()
		app.cfg.ForceEnglishModNames = !app.cfg.ForceEnglishModNames
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
		app.refreshModList()
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})
	forceEnglishItem.Checked = forceEnglish
	forceEnglishItem.CheckedIcon = checkedIcon
	forceEnglishItem.UncheckedIcon = uncheckedIcon

	// SETTINGS MENU - Show list after sorting.
	showListAfterSortItem := fyne.NewMenuItem(app.msg("menu_show_list_after_sort"), func() {
		app.cfgMutex.Lock()
		app.cfg.ShowModListAfterSort = !app.cfg.ShowModListAfterSort
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})
	showListAfterSortItem.Checked = showListAfterSort
	showListAfterSortItem.CheckedIcon = checkedIcon
	showListAfterSortItem.UncheckedIcon = uncheckedIcon

	quitItem := fyne.NewMenuItemWithIcon(
		app.msg("menu_quit"),
		loadMenuIcon("exit.png"),
		func() { app.closeApp() })
	quitItem.Danger = true
	quitItem.IsQuit = true

	// LOGIN TO NEXUS MENU
	loggedIn := app.isLoggedIn()
	oauthActionLabel := app.msg("menu_nexus_login")
	oauthActionIcon := loadMenuIcon("enter.png")
	nexusMenuIcon := loadMenuIcon("disconnect_red_int.png")
	if loggedIn {
		oauthActionLabel = app.msg("menu_nexus_logout")
		oauthActionIcon = loadMenuIcon("exit.png")
		nexusMenuIcon = loadMenuIcon("connect_int.png")
	}
	oauthActionItem := fyne.NewMenuItemWithIcon(oauthActionLabel, oauthActionIcon, func() {
		if app.isLoggedIn() {
			app.logoutOAuth()
		} else {
			app.startOAuthFlow()
		}
	})
	nexusMenu := fyne.NewMenuWithIcon(app.msg("menu_nexus"), nexusMenuIcon,
		oauthActionItem,
	)

	// SETTINGS MENU AML AND USER_SETTINGS.CONFIG
	amlUSConfMenu := fyne.NewMenuWithIcon(
		app.msg("menu_aml_usconf"),
		loadMenuIcon("clockcog.png"),
		fyne.NewMenuItemWithIcon(
			app.msg("btn_aml_config"),
			loadMenuIcon("tasks.png"),
			func() { app.showAMLConfigWindow() }),
		fyne.NewMenuItemSeparator(),

		fyne.NewMenuItemWithIcon(
			app.msg("menu_game_settings_editor"),
			loadMenuIcon("cogs.png"),
			func() { app.showGameSettingsEditor() }),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_backup_settings"),
			loadMenuIcon("export.png"),
			func() { app.createSettingsBackup() }),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_restore_settings"),
			loadMenuIcon("import.png"),
			func() { app.showRestoreSettingsDialog() }),
	)

	// PROFILES MENU
	profileMenu := fyne.NewMenuWithIcon(
		app.msg("menu_profiles"),
		loadMenuIcon("profiles.png"),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_profiles_create"),
			loadMenuIcon("profile_add.png"),
			func() { app.showCreateProfileDialog() }),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_profiles_rename"),
			loadMenuIcon("profile_edit.png"),
			func() { app.showRenameProfileDialog() }),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_profiles_delete"),
			loadMenuIcon("profile_del.png"),
			func() { app.showDeleteProfileDialog() }),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_profiles_import"),
			loadMenuIcon("profile_import.png"),
			func() { app.showImportProfileDialog() }),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_profiles_export"),
			loadMenuIcon("profile_export.png"),
			func() { app.showExportProfileDialog() }),
	)

	// GUIDES MENU
	guidesMenu := fyne.NewMenuWithIcon(
		app.msg("menu_guides"),
		loadMenuIcon("infobook.png"),
		fyne.NewMenuItemWithIcon(
			app.msg("wizard_mi"),
			loadMenuIcon("wizard.png"),
			func() {
				go app.runWizard(true)
			}),
		fyne.NewMenuItemSeparator(),

		fyne.NewMenuItemWithIcon(
			app.msg("menu_guides_video_profiles"),
			loadMenuIcon("video.png"),
			func() {
				u, _ := url.Parse(YouTubeGuideProfil)
				_ = app.myApp.OpenURL(u)
			}),
		fyne.NewMenuItemWithIcon(
			app.msg("menu_guides_video_hti"),
			loadMenuIcon("video.png"),
			func() {
				u, _ := url.Parse(YouTubeGuideURLOld)
				_ = app.myApp.OpenURL(u)
			}),
	)

	// FEEDBACK MENU
	openSMQNexusPage := fyne.NewMenuItemWithIcon(
		app.msg("menu_open_smq_page"),
		loadMenuIcon("nexus_orange.png"),
		func() {
			go app.initiateSortFilesUpdate()
		})
	contactGitHub := fyne.NewMenuItemWithIcon(
		app.msg("menu_github"),
		loadMenuIcon("repo.png"),
		func() {
			u, _ := url.Parse(GitHubRepoSMQ)
			_ = app.myApp.OpenURL(u)
		})
	contactDiscord := fyne.NewMenuItemWithIcon(
		app.msg("menu_discord_dtmoddrs"),
		loadMenuIcon("discord.png"),
		func() {
			u, _ := url.Parse(DiscordDTModders)
			_ = app.myApp.OpenURL(u)
		})
	contactDiscordMy := fyne.NewMenuItemWithIcon(
		app.msg("menu_discord_my"),
		loadMenuIcon("discord_xss.png"),
		func() {
			u, _ := url.Parse(DiscordDTMy)
			_ = app.myApp.OpenURL(u)
		})

	contactMenu := fyne.NewMenuWithIcon(
		app.msg("menu_contact"),
		loadMenuIcon("feedback.png"),
		openSMQNexusPage,
		contactGitHub,
		contactDiscord,
		contactDiscordMy,
	)

	// DONATE MENU
	donateBoosty := fyne.NewMenuItemWithIcon(
		app.msg("menu_boosty"),
		loadMenuIcon("boosty.png"),
		func() {
			u, _ := url.Parse(DonateBoostyURL)
			_ = app.myApp.OpenURL(u)
		})
	donateDonationAlerts := fyne.NewMenuItemWithIcon(
		app.msg("menu_donationalerts"),
		loadMenuIcon("donation_alerts.png"),
		func() {
			u, _ := url.Parse(DonateDonationAlertsURL)
			_ = app.myApp.OpenURL(u)
		})
	donateSteamGift := fyne.NewMenuItemWithIcon(
		app.msg("menu_steam_gift"),
		loadMenuIcon("steam.png"),
		func() {
			u, _ := url.Parse("https://steamcommunity.com/id/xssplater/")
			_ = app.myApp.OpenURL(u)
		})
	donateCard := fyne.NewMenuItemWithIcon(
		app.msg("menu_card"),
		loadMenuIcon("card.png"),
		func() {
			app.myApp.Clipboard().SetContent(DonateCardNumber)
		})

	donateMenu := fyne.NewMenuWithIcon(app.msg("menu_donate"), loadMenuIcon("hearth_red.png"),
		donateBoosty,
		donateDonationAlerts,
		donateSteamGift,
		donateCard,
	)

	// Show DMLoader and DMFramework — тоже настоящая галочка.
	showSystemItem := fyne.NewMenuItem(app.msg("setting_show_system_mods"), func() {
		app.cfgMutex.Lock()
		app.cfg.ShowSystemMods = !app.cfg.ShowSystemMods
		newShow := app.cfg.ShowSystemMods
		app.cfgMutex.Unlock()
		app.saveConfigSafe()
		if app.systemModsTableContainer != nil {
			if newShow {
				app.systemModsTableContainer.Show()
			} else {
				app.systemModsTableContainer.Hide()
			}
		}
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})
	showSystemItem.Checked = showSystemMods
	showSystemItem.CheckedIcon = checkedIcon
	showSystemItem.UncheckedIcon = uncheckedIcon

	// SETTINGS MENU
	settingsMenu := fyne.NewMenuWithIcon(app.msg("menu_settings"), loadMenuIcon("cog.png"),
		langMenu,
		themeMenu,
		dateMenu,
		periodicSub,
		fyne.NewMenuItemSeparator(),
		forceEnglishItem,
		showListAfterSortItem,
		showSystemItem,
		fyne.NewMenuItemSeparator(),
		quitItem,
	)

	// Right Align == false, Left Align == true.
	nexusMenu.RightAlign = false
	profileMenu.RightAlign = false
	amlUSConfMenu.RightAlign = false
	contactMenu.RightAlign = true
	guidesMenu.RightAlign = true
	donateMenu.RightAlign = true

	// Menu items layout
	return fyne.NewMainMenu(
		// Left Align
		settingsMenu,
		fyne.NewMenuSeparator(),
		nexusMenu,
		fyne.NewMenuSeparator(),
		amlUSConfMenu,
		fyne.NewMenuSeparator(),
		profileMenu,

		// Right Align
		guidesMenu,
		fyne.NewMenuSeparator(),
		contactMenu,
		fyne.NewMenuSeparator(),
		donateMenu,
	)
}

func (app *App) changeLanguage(lang string) {
	if err := app.loadLanguage(lang); err != nil {
		return
	}
	app.cfgMutex.Lock()
	app.cfg.Language = lang
	app.cfgMutex.Unlock()
	checks.SetLanguage(lang)
	saveConfig(app.cfg)
	app.mainWindow.SetTitle(app.msg("app_title_long"))
	app.mainWindow.SetMainMenu(app.buildMainMenu())
	sorter.SetSortMessages(app.msg("sort_ru_warning"), app.msg("sort_en_warning"))
	sorter.SetLogMessages(app.msg("log_create_mlot"), app.msg("log_mlot_created"))

	// Проверки на nil для виджетов, которые создаются в buildUI
	if app.searchEntry != nil {
		app.searchEntry.SetPlaceHolder(app.msg("search_placeholder"))
	}
	if app.filterSelect != nil {
		app.filterSelect.Options = []string{
			app.msg("filter_all"), app.msg("filter_active"), app.msg("filter_inactive"),
			app.msg("filter_obsolete"), app.msg("filter_conflict"),
		}
		app.filterSelect.SetSelected(app.msg("filter_all"))
		app.filterSelect.Refresh()
	}
	app.refreshModList() // filterModList внутри имеет проверку counterLabel != nil

	// Текстовые метки
	if app.filterLabel != nil {
		app.filterLabel.SetText(app.msg("filter_label"))
	}
	if app.btnAMLConfig != nil { // AML
		app.btnAMLConfig.SetText(app.msg("btn_aml_config"))
		app.btnAMLConfig.SetToolTip(app.msg("btn_aml_config_tooltip"))
	}
	// Новые кнопки быстрого перемещения
	if app.moveToEntry != nil {
		app.moveToEntry.SetPlaceHolder("##") // неизменяемое
	}
	if app.moveLabel != nil {
		app.moveLabel.SetText(app.msg("lbl_move_to"))
	}
	// Кнопки удаления модов
	if app.btnRemoveAll != nil {
		app.btnRemoveAll.SetToolTip(app.msg("btn_remove_all_tooltip"))
	}
	if app.btnRemoveSelected != nil {
		app.btnRemoveSelected.SetToolTip(app.msg("btn_remove_selected_tooltip"))
	}
	// Кнопки запуска
	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.btnCheckUpdates != nil {
		app.btnCheckUpdates.SetToolTip(app.msg("btn_check_updates_tooltip"))
	}
	if app.btnUpdateMod != nil {
		app.btnUpdateMod.SetToolTip(app.msg("btn_update_mod_premium_only"))
	}
	if app.btnUpdateSelected != nil {
		app.btnUpdateSelected.SetToolTip(app.msg("btn_update_selected_tooltip"))
	}
	if app.btnUpdateAll != nil {
		app.btnUpdateAll.SetToolTip(app.msg("btn_update_all_tooltip"))
	}

	if app.btnEditVersion != nil {
		app.btnEditVersion.SetToolTip(app.msg("btn_edit_version_tooltip"))
	}

	// Обновляем заголовок консоли
	if app.logHeaderText != nil {
		app.logHeaderText.Text = app.msg("log_start0")
		app.logHeaderText.Refresh()
	}

	if app.profileLabel != nil {
		app.profileLabel.SetText(app.msg("profile_label"))
	}

	app.updateDescriptionForMod(app.selectedModName)

	app.refreshProfileList()

	// Обновляем тултипы для всех кнопок
	app.btnSaveOrder.SetToolTip(app.msg("btn_save_order_tooltip"))
	app.btnRefresh.SetToolTip(app.msg("btn_refresh_tooltip"))
	app.btnInstall.SetToolTip(app.msg("btn_install_tooltip"))
	app.btnRemove.SetToolTip(app.msg("btn_remove_tooltip"))
	app.btnUp.SetToolTip(app.msg("btn_up_tooltip"))
	app.btnDown.SetToolTip(app.msg("btn_down_tooltip"))
	app.btnSortChecks.SetToolTip(app.msg("btn_sort_checks_tooltip"))
	app.btnToggle.SetToolTip(app.msg("btn_toggle_tooltip"))
	app.btnLaunchNormal.SetToolTip(app.msg("btn_launch_game_tooltip"))
	app.btnLaunchNoLauncher.SetToolTip(app.msg("btn_launch_nolauncher_long_tooltip"))
	app.moveToTopBtn.SetToolTip(app.msg("btn_move_to_top_tooltip"))
	app.moveToBottomBtn.SetToolTip(app.msg("btn_move_to_bottom_tooltip"))
	app.selectAllBtn.SetToolTip(app.msg("btn_select_all_tooltip"))
	app.deselectAllBtn.SetToolTip(app.msg("btn_deselect_all_tooltip"))
	app.enableSelectedBtn.SetToolTip(app.msg("btn_enable_selected_tooltip"))
	app.disableSelectedBtn.SetToolTip(app.msg("btn_disable_selected_tooltip"))
	app.enableAllBtn.SetToolTip(app.msg("btn_enable_all_tooltip"))
	app.disableAllBtn.SetToolTip(app.msg("btn_disable_all_tooltip"))
	app.manageBtn.SetToolTip(app.msg("btn_manage_mods_tooltip"))
	app.btnRemoveAll.SetToolTip(app.msg("btn_remove_all_tooltip"))
	app.btnRemoveSelected.SetToolTip(app.msg("btn_remove_selected_tooltip"))
	app.btnAMLConfig.SetToolTip(app.msg("btn_aml_config_tooltip"))
	app.btnUpdateSelected.SetToolTip(app.msg("btn_update_selected_tooltip"))
	app.btnCheckUpdates.SetToolTip(app.msg("btn_check_updates_tooltip"))
	app.btnUpdateAll.SetToolTip(app.msg("btn_update_all_tooltip"))
	app.btnEditVersion.SetToolTip(app.msg("btn_edit_version_tooltip"))
	app.openFolderBtn.SetToolTip(app.msg("open_mod_folder_tooltip"))
}
