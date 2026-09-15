// Servo-Modquisitor-2/main.go
package main

import (
	"Servo-Modquisitor/themes"
	"embed"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/tooltip"
)

// Localization + app-level images.
//go:embed lang/messages.json
//go:embed assets/*.jpg
//go:embed assets/*.png

// All button icons live under assets/buttons/ — glob covers the whole set.
//go:embed assets/buttons/*.png

var embeddedFiles embed.FS

func main() {
	// Проверяем, не передали ли нам nxm-ссылку при запуске
	if len(os.Args) > 1 && os.Args[1] == NXMCommLine && len(os.Args) > 2 {
		nxmURL := os.Args[2]
		// Пытаемся подключиться к уже запущенному экземпляру
		conn, err := net.Dial(NXMProtocol, NXMAddress)
		if err == nil {
			fmt.Fprintln(conn, nxmURL)
			conn.Close()
			os.Exit(0)
		}
		// Если не удалось - это первый экземпляр, продолжаем обычный запуск
	}

	// Проверяем, не запущен ли уже другой экземпляр (использует системный диалог, не требует Fyne)
	if isAlreadyRunning() {
		showAlreadyRunningDialog()
		os.Exit(0)
	}

	// Создаём приложение
	myApp := app.NewWithID(AppID)
	cfg := loadConfig()
	application := NewApp(cfg, myApp)

	// Очищаем временные папки от предыдущих запусков
	cleanProgramTempDirs()

	// Открываем лог
	exePath, _ := os.Executable()
	globalDir := filepath.Dir(exePath)
	logPath := filepath.Join(globalDir, FileNameLog)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err == nil {
		if info, err := f.Stat(); err == nil && info.Size() > MaxLogFileSize {
			f.Close()
			os.Remove(logPath)
			f, err = os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
			if err != nil {
				application.appendLogToFile(fmt.Sprintf(application.msg("log_failed_to_recreate_log"), err))
				application.logFile = nil
			} else {
				application.logFile = f
				application.appendLogToFile(application.msg("log_failed_to_recreate_log"))
				application.appendLogToFile(application.msg("log_started"))
			}
		} else {
			application.logFile = f
			application.appendLogToFile(application.msg("log_started"))
		}
	} else {
		application.appendLogToFile(fmt.Sprintf(application.msg("log_could_not_open_log"), err))
		application.logFile = nil
	}

	// Создаём главное окно
	application.mainWindow = myApp.NewWindow(application.msg("app_title_long"))
	ApplyWindowSettings(application.mainWindow)
	application.mainWindow.SetMaster()

	iconData, _ := embeddedFiles.ReadFile(AppIcon)
	if iconData != nil {
		icon := fyne.NewStaticResource("icon", iconData)
		application.mainWindow.SetIcon(icon)
	}

	// Строим UI, устанавливаем заголовок и меню
	application.buildUI()
	application.mainWindow.SetTitle(application.getTitle() + " v" + AppVersion)
	application.mainWindow.SetMainMenu(application.buildMainMenu())

	// Обработчик закрытия окна
	application.mainWindow.SetOnClosed(func() {
		// Закрываем слушатель nxm, чтобы освободить порт
		application.nxm.Stop()

		saveWindowState := func() {
			size := application.mainWindow.Canvas().Size()
			application.cfgMutex.Lock()
			application.cfg.WindowWidth = int(size.Width)
			application.cfg.WindowHeight = int(size.Height)
			application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
			application.cfgMutex.Unlock()
			application.saveConfigSafe()
		}

		if application.orderDirty {
			dialog.ShowConfirm(
				application.msg("window_error_title"),
				application.msg("unsaved_changes_question"),
				func(ok bool) {
					if ok {
						application.saveCurrentOrder()
						application.appendLogToFile(application.msg("order_saved_on_exit"))
					}
					saveWindowState()
					application.closeApp()
				},
				application.mainWindow,
			)
			return
		}

		saveWindowState()
		application.closeApp()
	})

	// Обработчик Drag&Drop
	application.mainWindow.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		application.handleDrop(uris)
	})

	// Сначала показываем окно, потом восстанавливаем размеры (как в старом коде)
	application.mainWindow.Show()
	application.updateTooltipStyle()

	// Восстанавливаем размеры окна из конфига (если есть)
	if cfg.WindowWidth > 0 && cfg.WindowHeight > 0 {
		application.mainWindow.Resize(fyne.NewSize(float32(cfg.WindowWidth), float32(cfg.WindowHeight)))
	} else {
		application.mainWindow.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
	}
	if cfg.WindowMaximized {
		go func() {
			time.Sleep(WindowMaximizeDelay)
			fyne.Do(func() {
				maximizeWindowByTitle(application.mainWindow.Title())
			})
		}()
	}

	// Запускаем мастер установки (если нужно)
	go func() {
		time.Sleep(300 * time.Millisecond)
		application.runWizard(false)
	}()

	// Запускаем фоновую горутину для инициализации путей и загрузки данных
	go func() {
		defer func() {
			if r := recover(); r != nil {
				application.appendLogToFile(fmt.Sprintf("PANIC in background initialization: %v", r))
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("Critical error during initialization: %v", r), application.mainWindow)
				})
			}
		}()

		// 1. Определяем корень игры и папку mods
		application.initializePaths()

		// 2. Устанавливаем путь к глобальным данным
		application.setGlobalDataDir()

		// 3. Миграция глобальных файлов (если они ещё в папке mods)
		application.migrateGlobalFilesFromMods()

		// 4. Загружаем данные (базы, кэш, списки модов)
		application.loadDataAfterInit()

		// 5. Инициализируем профили
		application.initProfiles()

		// 6. Регистрируем nxm
		if exePath, err := os.Executable(); err == nil {
			registerNXMProtocol(exePath)
		}

		// 7. Запускаем слушатель
		application.nxm.Start()
	}()

	// Запускаем главный цикл событий
	application.mainWindow.ShowAndRun()
}

// updateTooltipStyle обновляет стиль тултипов в соответствии с текущей темой.
func (app *App) updateTooltipStyle() {
	if app.mainWindow == nil {
		return
	}
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	// Фон тултипа - используем цвет оверлея (обычно тёмный/светлый)
	bg := th.Color(theme.ColorNameMenuBackground, variant)
	// Рамка - используем цвет обводки CRT-консоли (у вас он меняется в темах)
	border := th.Color(themes.ColorCRTScreenStroke, variant)

	style := tooltip.Style{
		Background:  bg,
		BorderColor: border,
		BorderWidth: 1,
		FontBold:    true,
	}

	if c, ok := app.mainWindow.Canvas().(interface{ SetTooltipStyle(tooltip.Style) }); ok {
		c.SetTooltipStyle(style)
	}
}
