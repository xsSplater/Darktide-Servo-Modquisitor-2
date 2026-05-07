package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/config"
	"Servo-Modquisitor/sorter"
	"embed"
	"net/url"
	"os"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

//go:embed lang/messages.json assets/CRT_BlackBG.jpg
var embeddedFiles embed.FS

func main() {
	myApp := app.NewWithID("com.xssplater.servo-modquisitor")
	cfg := loadConfig()
	cfg.ModsPath, _ = os.Getwd()

	application := NewApp(cfg, myApp)
	application.mainWindow = myApp.NewWindow(application.messages["app_title_long"])
	config.ApplyWindowSettings(application.mainWindow)
	application.mainWindow.SetMaster()

	if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
		application.mainWindow.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))
	} else {
		application.mainWindow.Resize(fyne.NewSize(config.MainWindowWidth, config.MainWindowHeight))
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

	if err := checks.LoadExternalLists("mandatory_obsolete_incompatible_dependencies.json"); err != nil {
		application.appendLog(application.messages["log_warn_moid_not_found"])
	} else {
		application.appendLog(application.messages["log_succ_moid_found"])
	}
	sorter.SetMandatoryOrder(checks.MandatoryOrder)
	sorter.SetDependencies(convertDeps(checks.Dependencies))

	var sorterRules []sorter.LoadOrderRule
	for _, r := range checks.LoadOrderRules {
		sorterRules = append(sorterRules, sorter.LoadOrderRule{Mod: r.Mod, Before: r.Before})
	}
	sorter.SetLoadOrderRules(sorterRules)

	if err := application.loadModDatabase("mod_database.json"); err != nil {
		application.modDatabase = []checks.ModDBEntry{}
		application.appendLog(application.messages["log_mod_db_missing"])
	}
	checks.SetModDatabase(application.modDatabase)

	sorter.LoadSortOrders()

	SetLauncherMessages(
		application.messages["launcher_ver_unknown"],
		application.messages["launcher_exe_not_found"],
		application.messages["launcher_root_not_found"],
	)
	application.launchGameFunc = launchGame

	application.syncModsEnabledState()
	application.refreshModList()
	application.buildUI()

	if !cfg.InitialSetupDone {
		application.performFirstRunSetup()
	}
	application.updateToggleButtonText(application.btnToggle)

	application.mainWindow.SetTitle(application.getTitle())
	application.mainWindow.SetMainMenu(application.buildMainMenu())

	application.mainWindow.SetOnClosed(func() {
		size := application.mainWindow.Canvas().Size()
		application.cfg.WindowWidth = int(size.Width)
		application.cfg.WindowHeight = int(size.Height)
		saveConfig(application.cfg)
	})

	application.mainWindow.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		application.handleDrop(uris)
	})

	application.mainWindow.ShowAndRun()
}
