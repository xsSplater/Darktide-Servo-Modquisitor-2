package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

//go:embed lang/messages.json assets/CRT_BlackBG.jpg assets/Yellow_BG.jpg assets/Yellow_BG_button.jpg assets/Yellow_BG_col.jpg assets/icon.png
var embeddedFiles embed.FS

func main() {
	myApp := app.NewWithID("com.xssplater.servo-modquisitor")

	cfg := loadConfig()
	cfg.ModsPath, _ = os.Getwd()

	application := NewApp(cfg, myApp)
	logPath := filepath.Join(cfg.ModsPath, "app.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err == nil {
		application.logFile = f
		application.appendLog(application.messages["log_started"])
	} else {
		application.appendLog(fmt.Sprintf(application.messages["log_could_not_open_log"], err))
	}
	application.mainWindow = myApp.NewWindow(application.messages["app_title_long"])
	ApplyWindowSettings(application.mainWindow)
	application.mainWindow.SetMaster()

	iconData, _ := embeddedFiles.ReadFile("assets/icon.png")
	if iconData != nil {
		icon := fyne.NewStaticResource("icon", iconData)
		application.mainWindow.SetIcon(icon)
	}

	if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
		application.mainWindow.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))
	} else {
		application.mainWindow.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
	}

	if cfg.WindowMaximized {
		go func() {
			time.Sleep(WindowMaximizeDelay)
			maximizeWindowByTitle(application.mainWindow.Title())
		}()
	}

	checks.SetLanguage(cfg.Language)
	checks.InitGlobals(
		func(text string) { application.appendLog(text) },
		&application.messages,
		func(parent fyne.Window, header, msg string, opts ...string) int {
			return application.showChoiceDialog(parent, header, msg, opts...)
		},
		func(link string) {
			fyne.Do(func() { u, _ := url.Parse(link); myApp.OpenURL(u) })
		},
		cfg.ModsPath,
	)

	sorter.SetFolderExistsFunc(checks.FolderExists)
	sorter.SetListModFoldersFunc(checks.ListModFolders)
	sorter.SetLogFunc(func(text string) { application.appendLog(text) })
	sorter.SetSortMessages(application.messages["sort_ru_warning"], application.messages["sort_en_warning"])
	sorter.SetLogMessages(application.messages["log_create_mlot"], application.messages["log_mlot_created"])

	if err := checks.LoadExternalLists(FileNameMandatoryRules); err != nil {
		application.appendLog(application.messages["log_warn_moid_not_found"])
	} else {
		application.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
		saveConfig(application.cfg)
		application.appendLog(application.messages["log_succ_moid_found"])
	}
	sorter.SetMandatoryOrder(checks.MandatoryOrder)
	sorter.SetDependencies(convertDeps(checks.Dependencies))

	var sorterRules []sorter.LoadOrderRule
	for _, r := range checks.LoadOrderRules {
		sorterRules = append(sorterRules, sorter.LoadOrderRule{Mod: r.Mod, Before: r.Before})
	}
	sorter.SetLoadOrderRules(sorterRules)

	if err := application.loadModDatabase(FileNameModDatabase); err != nil {
		application.modDatabase = []checks.ModDBEntry{}
		application.appendLog(application.messages["log_mod_db_missing"])
		application.cfg.LastModDatabaseVersion = ""
	}
	checks.SetModDatabase(application.modDatabase)

	sorter.LoadSortOrders()

	SetLauncherMessages(
		application.messages["launcher_ver_unknown"],
		application.messages["launcher_exe_not_found"],
		application.messages["launcher_root_not_found"],
	)
	SetLinuxLauncherMessages(
		application.messages["linux_wine_not_found"],
		application.messages["linux_xbox_not_supported"],
	)
	application.launchGameFunc = launchGame

	application.syncModsEnabledState()
	application.refreshModList()
	application.buildUI()

	if !cfg.InitialSetupDone {
		application.performFirstRunSetup()
	}
	application.updateToggleButtonText(application.btnToggle)

	application.mainWindow.SetTitle(application.getTitle() + " v" + AppVersion)
	application.mainWindow.SetMainMenu(application.buildMainMenu())

	application.mainWindow.SetOnClosed(func() {
		if application.orderDirty {
			dialog.ShowConfirm(
				application.messages["window_error_title"],
				application.messages["unsaved_changes_question"],
				func(ok bool) {
					if ok {
						application.saveCurrentOrder()
						application.appendLog(application.messages["order_saved_on_exit"])
					}
					size := application.mainWindow.Canvas().Size()
					application.cfg.WindowWidth = int(size.Width)
					application.cfg.WindowHeight = int(size.Height)
					application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
					saveConfig(application.cfg)
					application.mainWindow.Close()
				},
				application.mainWindow,
			)
			return
		}
		size := application.mainWindow.Canvas().Size()
		application.cfg.WindowWidth = int(size.Width)
		application.cfg.WindowHeight = int(size.Height)
		application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
		saveConfig(application.cfg)
	})

	application.mainWindow.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		application.handleDrop(uris)
	})

	go application.ensureSortFiles()

	if application.cfg.UpdateCheckFrequency != "never" && application.shouldCheckUpdates() {
		go application.checkForUpdates()
	}

	go application.blinkCheckSortIfNeeded()

	application.mainWindow.ShowAndRun()
}
