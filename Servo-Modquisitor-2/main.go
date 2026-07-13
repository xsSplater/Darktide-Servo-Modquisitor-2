// main.go
package main

import (
	"bufio"
	"embed"
	"fmt"
	"net"
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

	if isAlreadyRunning() {
		showAlreadyRunningDialog()
		os.Exit(0)
	}

	myApp := app.NewWithID(AppID)
	cfg := loadConfig()

	application := NewApp(cfg, myApp)

	// Открываем лог (путь может быть невалидным, но logPath будет создан позже)
	logPath := filepath.Join(cfg.ModsPath, FileNameLog)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err == nil {
		if info, err := f.Stat(); err == nil && info.Size() > MaxLogFileSize {
			f.Close()
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

	// Создаём главное окно
	application.mainWindow = myApp.NewWindow(application.messages["app_title_long"])
	ApplyWindowSettings(application.mainWindow)
	application.mainWindow.SetMaster()

	iconData, _ := embeddedFiles.ReadFile(AppIcon)
	if iconData != nil {
		icon := fyne.NewStaticResource("icon", iconData)
		application.mainWindow.SetIcon(icon)
	}

	// Строим UI (пока с пустыми данными)
	application.buildUI()

	// Устанавливаем заголовок и меню
	application.mainWindow.SetTitle(application.getTitle() + " v" + AppVersion)
	application.mainWindow.SetMainMenu(application.buildMainMenu())

	// Обработчик закрытия окна
	application.mainWindow.SetOnClosed(func() {
		// Закрываем слушатель nxm, чтобы освободить порт
		if application.nxmListener != nil {
			application.nxmListener.Close()
		}

		if application.orderDirty {
			dialog.ShowConfirm(
				application.messages["window_error_title"],
				application.messages["unsaved_changes_question"],
				func(ok bool) {
					if ok {
						application.saveCurrentOrder()
						application.appendLog(application.messages["order_saved_on_exit"])
					}
					// Сохраняем размеры окна перед выходом
					size := application.mainWindow.Canvas().Size()
					application.cfg.WindowWidth = int(size.Width)
					application.cfg.WindowHeight = int(size.Height)
					application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
					saveConfig(application.cfg)
					// Закрываем окно и завершаем процесс
					application.closeApp()
				},
				application.mainWindow,
			)
			return
		}

		// Если изменений нет - просто выходим
		size := application.mainWindow.Canvas().Size()
		application.cfg.WindowWidth = int(size.Width)
		application.cfg.WindowHeight = int(size.Height)
		application.cfg.WindowMaximized = isWindowMaximized(application.mainWindow.Title())
		saveConfig(application.cfg)
		application.closeApp()
	})

	// Обработчик Drag&Drop
	application.mainWindow.SetOnDropped(func(pos fyne.Position, uris []fyne.URI) {
		application.handleDrop(uris)
	})

	// Показываем окно (оно уже отображается, но данные ещё не загружены)
	application.mainWindow.Show()

	// Восстанавливаем размеры окна из конфига (если есть)
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

	// Запускаем фоновую горутину для инициализации путей и загрузки данных
	go func() {
		// 1. Определяем корень игры и папку mods (в фоне, диалоги внутри используют fyne.Do)
		application.initializePaths()

		// 2. После того как пути определены, загружаем все данные и обновляем UI
		application.loadDataAfterInit()

		// 3. Регистрируем обработчик nxm (если ещё не зарегистрирован)
		if exePath, err := os.Executable(); err == nil {
			registerNXMProtocol(exePath)
		}

		// 4. Запускаем слушатель nxm-ссылок (если ещё не запущен)
		if application.nxmListener == nil {
			listener, err := net.Listen(NXMProtocol, NXMAddress)
			if err == nil {
				application.nxmListener = listener
				go func() {
					for {
						if application.nxmListener == nil {
							return
						}
						conn, err := application.nxmListener.Accept()
						if err != nil {
							return
						}
						link, _ := bufio.NewReader(conn).ReadString('\n')
						conn.Close()
						fyne.Do(func() {
							application.handleNXMLink(strings.TrimSpace(link))
						})
					}
				}()
			}
		}
	}()

	// Запускаем главный цикл событий
	application.mainWindow.ShowAndRun()
}
