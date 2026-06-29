// main.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"bufio"
	"embed"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

//go:embed lang/messages.json assets/CRT_BlackBG.jpg assets/Yellow_BG.jpg assets/Yellow_BG_button.jpg assets/Yellow_BG_col.jpg assets/icon.png assets/mechanicus.png
var embeddedFiles embed.FS

var pendingNXMURL string

func main() {
	// Проверяем, не передали ли нам nxm-ссылку при запуске
	if len(os.Args) > 1 && os.Args[1] == NXMCommLine && len(os.Args) > 2 {
		nxmURL := os.Args[2]
		conn, err := net.Dial(NXMProtocol, NXMAddress)
		if err == nil {
			fmt.Fprintln(conn, nxmURL)
			conn.Close()
			os.Exit(0)
		}
		// Если не удалось соединиться - сохраняем ссылку для отложенной обработки
		pendingNXMURL = nxmURL
	}

	// Если запущены с флагом --updated, пропускаем проверку isAlreadyRunning
	if len(os.Args) > 1 && os.Args[1] == "--updated" {
		// ничего не делаем, продолжаем запуск
	} else {
		if isAlreadyRunning() {
			showAlreadyRunningDialog()
			os.Exit(0)
		}
	}

	myApp := app.NewWithID(AppID)
	cfg := loadConfig()
	// cfg.ModsPath, _ = os.Getwd()
	exePath, _ := os.Executable()
	cfg.ModsPath = filepath.Dir(exePath)

	application := NewApp(cfg, myApp)
	logPath := filepath.Join(cfg.ModsPath, FileNameLog)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err == nil {
		// Проверяем размер файла. Если больше 240 Кб - удаляем, чтобы не засорять систему.
		if info, err := f.Stat(); err == nil && info.Size() > MaxLogFileSize {
			f.Close()
			// Удаляем старый файл и создаём новый пустой
			os.Remove(logPath)
			f, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				application.appendLog(fmt.Sprintf(application.messages["log_failed_to_recreate_log"], err))
				application.logFile = nil
			} else {
				application.logFile = f
				application.appendLog(application.messages["log_failed_to_recreate_log"])
				application.appendLog(application.messages["log_started"])
			}
		} else {
			application.logFile = f
			application.appendLog(application.messages["log_started"])
		}
	} else {
		application.appendLog(fmt.Sprintf(application.messages["log_could_not_open_log"], err))
		application.logFile = nil
	}
	application.mainWindow = myApp.NewWindow(application.messages["app_title_long"])
	ApplyWindowSettings(application.mainWindow)
	application.mainWindow.SetMaster()

	iconData, _ := embeddedFiles.ReadFile(AppIcon)
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
		func(modName string) bool {
			return application.isModActive(modName)
		},
		func() { fyne.Do(application.refreshModList) },
	)

	sorter.SetFolderExistsFunc(checks.FolderExists)
	sorter.SetListModFoldersFunc(checks.ListModFolders)
	sorter.SetLogFunc(func(text string) { application.appendLog(text) })
	sorter.SetSortMessages(application.messages["sort_ru_warning"], application.messages["sort_en_warning"])
	sorter.SetHeaderFunc(checks.WriteLoadOrderHeader)
	sorter.SetLoadOrderOutputPath(filepath.Join(cfg.ModsPath, FileNameLoadOrder))
	sorter.SetLogMessages(application.messages["log_create_mlot"], application.messages["log_mlot_created"])

	if err := checks.LoadExternalLists(FileNameMandatoryRules); err != nil {
		application.appendLog(application.messages["log_warn_moid_not_found"] + ": " + err.Error())
	} else {
		application.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
		saveConfig(application.cfg)
		application.appendLog(application.messages["log_succ_moid_found"])
	}
	sorter.SetMandatoryOrder(checks.MandatoryOrder)
	sorter.SetDependencies(convertDeps(checks.Dependencies))

	var sorterRules []sorter.LoadOrderRule
	for _, r := range checks.LoadOrderRules {
		sorterRules = append(sorterRules, sorter.LoadOrderRule{
			Before: r.Before,
			After:  r.After,
		})
	}
	sorter.SetLoadOrderRules(sorterRules)

	if err := application.loadModDatabase(FileNameModDatabase); err != nil {
		application.modDatabase = []checks.ModDBEntry{}
		application.appendLog(application.messages["log_mod_db_missing"] + ": " + err.Error())
		application.cfg.LastModDatabaseVersion = ""
	}
	checks.SetModDatabase(application.modDatabase)

	// Синхронизируем кэш версий с локальными файлами
	application.syncVersionCache()

	// Пишем в лог версию проги и основных файлов
	if application.logFile != nil {
		fmt.Fprintf(application.logFile, "Program version: %s\n", AppVersion)
		fmt.Fprintf(application.logFile, "mandatory_obsolete_incompatible_dependencies.json version: %s\n", checks.GetExternalVersion())
		fmt.Fprintf(application.logFile, "mod_database.json version: %s\n", application.cfg.LastModDatabaseVersion)
	}

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
	application.buildUI()
	application.refreshModList()

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

	// Периодическая проверка обновлений программы и файлов сортировки
	if application.cfg.UpdateCheckFrequency != "never" && application.shouldCheckUpdates() {
		go application.checkSpecialUpdates()
	}

	// Проверка на AML и предупреждение пользователя (асинхронно)
	application.amlDetected = checks.IsAMLInstalled(cfg.ModsPath)
	if application.amlDetected && !cfg.SuppressAMLWarning {
		application.showChoiceDialogAsync(
			application.mainWindow,
			application.messages["aml_detected_title"],
			application.messages["aml_detected_warning"],
			func(choice int) {
				switch choice {
				case 0:
					if u, err := url.Parse(DarktideModDML); err == nil {
						application.myApp.OpenURL(u)
					}
				case 2:
					cfg.SuppressAMLWarning = true
					saveConfig(cfg)
				}
			},
			application.messages["btn_open_dml_page"],
			application.messages["btn_continue"],
			application.messages["btn_dont_show_again"],
		)
	}

	// Регистрируем программу как обработчик nxm:// ссылок
	if exePath, err := os.Executable(); err == nil {
		registerNXMProtocol(exePath)
	}

	// Запускаем слушатель для межпроцессного взаимодействия (nxm-ссылки)
	application.nxmListener, err = net.Listen(NXMProtocol, NXMAddress)
	if err == nil {
		go func() {
			for {
				if application.nxmListener == nil {
					return // слушатель был остановлен, горутина больше не нужна
				}
				conn, err := application.nxmListener.Accept()
				if err != nil {
					// слушатель закрыт - выходим
					return
				}
				// Читаем одну строку - nxm-ссылку
				link, _ := bufio.NewReader(conn).ReadString('\n')
				conn.Close()
				// Передаём в главный поток
				fyne.Do(func() {
					application.handleNXMLink(strings.TrimSpace(link))
				})
			}
		}()
		defer application.nxmListener.Close()
	}
	// Проверка наличия DML/DMF сразу после запуска
	go func() {
		time.Sleep(500 * time.Millisecond) // даём окну отрисоваться
		fyne.Do(func() {
			if !checks.FolderExists("base") {
				application.appendLog(application.messages["log_warn_base_missing"])
				dialog.ShowInformation(
					application.messages["window_error_title"],
					application.messages["missing_base_dml"],
					application.mainWindow,
				)
			}
			if !checks.FolderExists("dmf") {
				application.appendLog(application.messages["dmf_missing"])
				dialog.ShowInformation(
					application.messages["window_error_title"],
					application.messages["missing_dmf_dmf"],
					application.mainWindow,
				)
			}
		})
	}()

	// Если есть отложенная ссылка - обрабатываем её после инициализации UI
	if pendingNXMURL != "" {
		fyne.Do(func() {
			application.handleNXMLink(pendingNXMURL)
		})
	}
	application.mainWindow.ShowAndRun()
}
