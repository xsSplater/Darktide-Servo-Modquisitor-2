// Servo-Modquisitor-2/main.go
package main

import (
	"Servo-Modquisitor/themes"
	"embed"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
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

//go:embed assets/patch.bin

// All button icons live under assets/buttons/ — glob covers the whole set.
//go:embed assets/buttons/*.png

var embeddedFiles embed.FS

// bundlePatchBin заполняется в main() из embeddedFiles.
// Раньше переменная была объявлена в bundle_patch.go, но никогда
// не инициализировалась — патч заменял 84 байта на пустоту, что
// привело бы к порче bundle_database.data. Теперь читаем файл
// из embed явно.
var bundlePatchBin []byte

func main() {
	// Загружаем patch.bin из embed — иначе PatchBundle заменит
	// 84 байта на пустой слайс и испортит bundle_database.data.
	var err error
	bundlePatchBin, err = embeddedFiles.ReadFile("assets/patch.bin")
	if err != nil || len(bundlePatchBin) == 0 {
		log.Fatalf("failed to load assets/patch.bin: %v (len=%d)", err, len(bundlePatchBin))
	}

	// Ищем nxm-ссылку в аргументах. Поддерживаем оба формата:
	//   servo-modquisitor-2 --nxm "nxm://..."
	//   servo-modquisitor-2 "nxm://..."       ← .desktop с %u на Linux
	args := os.Args[1:]
	var nxmURL string
	for i := 0; i < len(args); i++ {
		if args[i] == NXMCommLine && i+1 < len(args) {
			nxmURL = args[i+1]
			break
		}
	}
	if nxmURL == "" {
		for _, a := range args {
			if strings.HasPrefix(a, "nxm://") {
				nxmURL = a
				break
			}
		}
	}

	if nxmURL != "" {
		// Оптимизация: если запущен живой инстанс — перекинуть URL и выйти.
		// Это НЕ замена single-instance check: isAlreadyRunning() ниже
		// сработает, даже если TCP не прошёл.
		if conn, err := net.Dial(NXMProtocol, NXMAddress); err == nil {
			fmt.Fprintln(conn, nxmURL)
			conn.Close()
			os.Exit(0)
		}
		// Не выходим: TCP — best-effort. Проверку single-instance
		// пройдём общей веткой ниже.
	}

	// Ярлык быстрого запуска: --play. Обрабатываем ДО проверки
	// single-instance, чтобы не мешать пользователю, когда менеджер
	// уже открыт — просто тихо запускаем игру и выходим.
	if len(os.Args) > 1 && os.Args[1] == QuickLaunchCommLine {
		runQuickLaunch()
		return
	}

	// Single-instance — единственный источник истины, независимо от nxmURL.
	// TCP выше — только best-effort оптимизация «перекинуть URL живому
	// инстансу и выйти, не открывая второе окно».
	if isAlreadyRunning() {
		showAlreadyRunningDialog()
		os.Exit(0)
	}

	// Создаём приложение
	myApp := app.NewWithID(AppID)
	cfg := loadConfig()
	application := NewApp(cfg, myApp)
	application.pendingNXMURL = nxmURL

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

		// Синхронизируем активный профиль из game/mods, чтобы правки,
		// сделанные в течение сессии, не потерялись при следующем
		// переключении профиля.
		if !isDarktideRunning() {
			if err := application.syncActiveProfileFromGame(); err != nil {
				application.appendLogToFile(fmt.Sprintf("onClose sync: %v", err))
			}
		}

		saveWindowState := func() {
			if application.mainWindow == nil || application.mainWindow.Canvas() == nil {
				return
			}
			size := application.mainWindow.Canvas().Size()
			application.cfgMutex.Lock()
			application.cfg.WindowWidth = int(size.Width)
			application.cfg.WindowHeight = int(size.Height)
			application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
			application.cfgMutex.Unlock()
			application.saveConfigSafe()
		}

		if application.orderDirty.Load() {
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

	// Запускаем фоновую горутину для инициализации путей и загрузки данных.
	// Мастер запускается последним — только когда пути выбраны, профили
	// готовы и UI построен. Иначе пользователь видит стопку модалок
	// (выбор пути + мастер + AML-предупреждение одновременно).
	go func() {
		defer func() {
			if r := recover(); r != nil {
				application.appendLogToFile(fmt.Sprintf("PANIC in background initialization: %v", r))
				fyne.Do(func() {
					dialog.ShowError(fmt.Errorf("Critical error during initialization: %v", r), application.mainWindow)
				})
			}
		}()

		// 0. Если язык ещё не выбран (первый запуск или пустой cfg.Language) - показываем пикер ДО всех остальных диалогов. Иначе пользователь увидит англоязычные окна выбора пути и wizard'а, и лишь потом сможет сменить язык в настройках.
		application.showLanguagePickerIfFirstRun()

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

		// 5.5. Проверяем расхождение game/mods с активным профилем.
		// Если пользователь раньше правил моды вне программы — предложим
		// сохранить их в профиль.
		application.checkProfileSyncOnStart()

		// 6. Регистрируем nxm
		if exePath, err := os.Executable(); err == nil {
			registerNXMProtocol(exePath)
		}

		// 7. Запускаем слушатель
		application.nxm.Start()

		// 7.5. Если URL пришёл при старте — обрабатываем после того,
		// как listener готов и пути загружены. Проверяем, что окно
		// действительно готово, иначе handleNXMLink может дёрнуть
		// диалоги раньше, чем UI построен.
		if application.pendingNXMURL != "" {
			url := application.pendingNXMURL
			application.pendingNXMURL = ""
			fyne.Do(func() {
				application.handleNXMLink(url)
			})
		}
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
