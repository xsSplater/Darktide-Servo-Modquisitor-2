// Servo-Modquisitor-2/ui_dialogs.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"context"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

// ModUpdateChoice представляет один мод в диалоге выбора обновлений.
type ModUpdateChoice struct {
	Mod       *checks.ModInfo
	FileInfo  *FileInfo
	Changelog string
	Selected  bool
}

// showInfoDialog показывает информационный диалог с кнопкой OK.
func (app *App) showInfoDialog(title, message string) {
	fyne.Do(func() {
		app.bringToFront()
		dialog.ShowInformation(title, message, app.mainWindow)
	})
}

// dialogMaxHeight — жёсткий потолок высоты модального диалога.
// Ограничивает рост попапа, когда сообщение содержит длинный список
// (например, десятки устаревших модов). Без него контент уходит
// за нижний край экрана, а кнопки становятся недоступными. 700px —
// разумный компромисс: типичный ноутбук 1366×768 даёт ~700px
// рабочей высоты, диалог с 3 кнопками помещается целиком, а на
// большом мониторе видно больше строк без скролла.
const dialogMaxHeight float32 = 700

func (app *App) showChoiceDialog(parent fyne.Window, title, message string, callback func(int), options ...string) {
	// Все операции с UI - только в главном потоке
	fyne.Do(func() {
		app.bringToFront()
		// Объявляем popUp ПЕРЕД созданием кнопок
		var popUp *widget.PopUp

		// Фабрика кнопок. Вызывается дважды: для однорядной и
		// двухрядной раскладки. Fyne не позволяет переиспользовать
		// виджет в двух разных контейнерах одновременно, поэтому
		// кнопки создаются заново — это диалог, лишняя работа
		// незаметна.
		// Добавляем переменную для хранения первой кнопки
		var firstBtn fyne.Focusable

		makeButtons := func() []fyne.CanvasObject {
			btns := make([]fyne.CanvasObject, len(options))
			for i, opt := range options {
				idx := i
				btn := NewCustomIconButton(opt, app.getIconForButton(opt), func() {
					if popUp != nil {
						popUp.Hide()
					}
					if callback != nil {
						callback(idx)
					}
				})
				if i == 0 {
					firstBtn = btn // Запоминаем первую кнопку (обычно "Да/ОК")
				}
				btns[i] = btn
			}
			return btns
		}

		titleLabel := helpers.BoldCenterLabel(title)
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord

		msgScroll := container.NewVScroll(msgLabel)
		msgScroll.SetMinSize(fyne.NewSize(0, estimateMessageHeight(message)))

		// Функция сборки контента с заданной раскладкой кнопок.
		// Возвращает контейнер и ссылку на свою первую кнопку.
		buildContent := func(oneRow bool) (*fyne.Container, fyne.Focusable) {
			btns := makeButtons()
			var firstBtn fyne.Focusable
			if len(btns) > 0 {
				if btn, ok := btns[0].(fyne.Focusable); ok {
					firstBtn = btn
				}
			}

			var btnArea fyne.CanvasObject
			if oneRow || len(btns) <= 2 {
				btnArea = container.NewCenter(container.NewHBox(btns...))
			} else {
				mid := (len(btns) + 1) / 2
				row1 := container.NewCenter(container.NewHBox(btns[:mid]...))
				row2 := container.NewCenter(container.NewHBox(btns[mid:]...))
				btnArea = container.NewVBox(row1, row2)
			}
			content := container.NewBorder(
				container.NewVBox(titleLabel, widget.NewSeparator()),
				container.NewVBox(widget.NewSeparator(), btnArea),
				nil, nil,
				msgScroll,
			)
			return content, firstBtn
		}

		// Первая попытка — одна строка.
		content, firstBtn := buildContent(true)
		padded := container.NewPadded(content)

		if parent != nil && parent.Canvas() != nil && len(options) > 2 {
			ps := parent.Canvas().Size()
			if padded.MinSize().Width > ps.Width*0.9 {
				content2, firstBtn2 := buildContent(false)
				padded2 := container.NewPadded(content2)
				if padded2.MinSize().Width < padded.MinSize().Width {
					padded = padded2
					firstBtn = firstBtn2
				}
			}
		}

		// Размер = MinSize(padded) + небольшой запас на округления
		// шрифта. Запас маленький: основную работу делает NewPadded.
		sz := padded.MinSize()
		w := sz.Width + 8
		h := sz.Height + 8

		if w < DialogMinWidth500 {
			w = DialogMinWidth500
		}
		if h < DialogMinHeight200 {
			h = DialogMinHeight200
		}
		if h > dialogMaxHeight {
			h = dialogMaxHeight
		}
		if parent != nil && parent.Canvas() != nil {
			ps := parent.Canvas().Size()
			if maxW := ps.Width * 0.9; w > maxW {
				w = maxW
			}
			if maxH := ps.Height * 0.9; h > maxH {
				h = maxH
			}
		}

		popUp = widget.NewModalPopUp(padded, parent.Canvas())
		popUp.Resize(fyne.NewSize(w, h))
		popUp.Show()

		// Устанавливаем фокус на актуальную первую кнопку
		if firstBtn != nil && parent != nil && parent.Canvas() != nil {
			parent.Canvas().Focus(firstBtn)
		}
	})
}

// showConfirmDialog показывает диалог подтверждения с двумя кнопками (Да/Нет).
func (app *App) showConfirmDialog(title, message string, onConfirm func()) {
	app.showChoiceDialog(
		app.mainWindow,
		title,
		message,
		func(choice int) {
			if choice == 0 && onConfirm != nil {
				onConfirm()
			}
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

// showChoiceDialogSync - синхронная версия для фоновых горутин.
// Блокирует вызывающую горутину до выбора пользователя.
//
// ВАЖНО: вызывать ТОЛЬКО из фоновой горутины. Вызов из UI-потока
// приведёт к deadlock: UI-поток будет ждать канал, а fyne.Do внутри
// showChoiceDialog не сможет выполниться, потому что UI-поток занят.
// Для защиты от случайного misuse используется таймаут: если за
// 5 минут ответа не последовало, функция возвращает -1 и логирует
// предупреждение (утечка горутины, но UI остаётся живым).
func (app *App) showChoiceDialogSync(parent fyne.Window, title, message string, options ...string) int {
	resultChan := make(chan int, 1)
	app.showChoiceDialog(parent, title, message, func(choice int) {
		resultChan <- choice
	}, options...)

	select {
	case choice := <-resultChan:
		return choice
	case <-time.After(5 * time.Minute):
		app.appendLogToFile(fmt.Sprintf(
			"showChoiceDialogSync: timeout waiting for user choice (title=%q). "+
				"Возможно, вызов из UI-потока — это deadlock. Возвращён -1.", title))
		return -1
	}
}

// showChoiceDialogAsync - устаревшая, оставлена для совместимости.
// Используйте showChoiceDialog с callback.
func (app *App) showChoiceDialogAsync(parent fyne.Window, title, message string, callback func(int), options ...string) {
	app.showChoiceDialog(parent, title, message, callback, options...)
}

// showChoiceDialogRows — вариант showChoiceDialog, в котором кнопки
// разложены по строкам. Индексы в callback — плоские: сначала все
// кнопки первой строки, потом второй, слева-направо. Логика вызывающего
// кода, построенная на едином actions-массиве, остаётся верной.
func (app *App) showChoiceDialogRows(parent fyne.Window, title, message string, callback func(int), rows ...[]string) {
	fyne.Do(func() {
		app.bringToFront()
		var popUp *widget.PopUp

		flatIdx := 0
		var rowContainers []fyne.CanvasObject
		for _, rowOpts := range rows {
			if len(rowOpts) == 0 {
				continue
			}
			rowObjs := make([]fyne.CanvasObject, 0, len(rowOpts))
			for _, opt := range rowOpts {
				idx := flatIdx
				flatIdx++
				btn := NewCustomIconButton(opt, app.getIconForButton(opt), func() {
					if popUp != nil {
						popUp.Hide()
					}
					if callback != nil {
						callback(idx)
					}
				})
				rowObjs = append(rowObjs, btn)
			}
			rowContainers = append(rowContainers,
				container.NewCenter(container.NewHBox(rowObjs...)))
		}

		titleLabel := helpers.BoldCenterLabel(title)
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord

		// Скролл вокруг сообщения — та же причина, что и в
		// showChoiceDialog: длинный список пар «X требует Y»
		// иначе уводит кнопки за границу экрана.
		msgScroll := container.NewVScroll(msgLabel)

		// Высота сообщения — по числу визуальных строк, как
		// в showChoiceDialog.
		msgScroll.SetMinSize(fyne.NewSize(0, estimateMessageHeight(message)))

		// Кнопки идут под сообщением, одним VBox'ом — потому что
		// здесь их произвольное число строк.
		buttonsColumn := container.NewVBox()
		for _, rc := range rowContainers {
			buttonsColumn.Add(rc)
		}

		content := container.NewBorder(
			container.NewVBox(titleLabel, widget.NewSeparator()),
			container.NewVBox(widget.NewSeparator(), buttonsColumn),
			nil, nil,
			msgScroll,
		)

		// Обёртка в Padded — та же причина, что и в showChoiceDialog:
		// widget.NewModalPopUp не даёт внутренних отступов, кнопки
		// прилипают к краям.
		padded := container.NewPadded(content)

		sz := padded.MinSize()
		w := sz.Width + 8
		h := sz.Height + 8
		if w < DialogMinWidth500 {
			w = DialogMinWidth500
		}
		if h < DialogMinHeight200 {
			h = DialogMinHeight200
		}
		if h > dialogMaxHeight {
			h = dialogMaxHeight
		}
		if parent != nil && parent.Canvas() != nil {
			ps := parent.Canvas().Size()
			// 100% вместо 90%: у этой функции кнопки уже разложены
			// по строкам вызывающей стороной (CheckDependencies
			// передаёт две строки). Возможность занять чуть больше
			// места позволяет уместить строку целиком, вместо того
			// чтобы обрезать её край.
			if w > ps.Width {
				w = ps.Width
			}
			if maxH := ps.Height * 0.9; h > maxH {
				h = maxH
			}
		}

		popUp = widget.NewModalPopUp(padded, parent.Canvas())
		popUp.Resize(fyne.NewSize(w, h))
		popUp.Show()
	})
}

// showChoiceDialogRowsSync — синхронный вариант showChoiceDialogRows
// для фоновых горутин. По контракту совпадает с showChoiceDialogSync:
// блокирует вызывающую горутину, при таймауте возвращает -1.
func (app *App) showChoiceDialogRowsSync(parent fyne.Window, title, message string, rows ...[]string) int {
	resultChan := make(chan int, 1)
	app.showChoiceDialogRows(parent, title, message, func(choice int) {
		resultChan <- choice
	}, rows...)

	select {
	case choice := <-resultChan:
		return choice
	case <-time.After(5 * time.Minute):
		app.appendLogToFile(fmt.Sprintf(
			"showChoiceDialogRowsSync: timeout waiting for user choice (title=%q)", title))
		return -1
	}
}

// Основной диалог скачивания (для обычных модов)
func (app *App) showDownloadDialog(downloadURL, filename string, modName string, fileInfo *FileInfo, modID string) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.cfgMutex.RLock()
	autoConfirm := app.cfg.AutoConfirmDownloadEnabled()
	app.cfgMutex.RUnlock()

	if autoConfirm {
		// Пользователь явно разрешил пропускать подтверждение.
		// Consent уже дан им на сайте Nexus при клике «Mod Manager Download».
		app.startDownload(downloadURL, filename, modName, fileInfo, modID)
		return
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), modName, displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			// callback выполняется в UI-потоке. startDownload внутри делает
			// fyne.Do + ожидает закрытия канала — синхронный вызов из UI
			// заклинил бы поток навсегда. Уводим в горутину.
			go app.startDownload(downloadURL, filename, modName, fileInfo, modID)
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

// startDownload - выполняет скачивание и установку после подтверждения.
// Длительная работа вынесена в горутину, UI-поток только для dlg.Hide()
// и прокрутки к установленному моду. Иначе вложенный fyne.Do даёт deadlock.
//
// Создание виджетов (bar, dlg) обёрнуто в fyne.Do и синхронизировано
// через ready-канал: fyne.Do асинхронный, без барьера горутина
// скачивания стартанула бы раньше, чем bar/dlg будут присвоены, и
// DownloadFileWithProgress упал бы на nil-pointer при bar.SetValue.
func (app *App) startDownload(downloadURL, filename, modName string, fileInfo *FileInfo, modID string) {
	app.appendLog(fmt.Sprintf(app.msg("log_downloading_mod"), modName))

	// fileInfo может быть nil, если nxm-ссылка пришла без доступных
	// метаданных файла (например, getFileInfoByID упал по сети/403).
	// Запоминаем известную версию/timestamp один раз — это не только
	// защита от nil-deref, но и единственный snapshot: дальше в горутине
	// fileInfo может быть изменён другим вызовом (в теории).
	var knownVersion string
	var knownTimestamp int64
	var fallbackFileName string
	if fileInfo != nil {
		knownVersion = fileInfo.Version
		knownTimestamp = fileInfo.UploadedTimestamp
		fallbackFileName = fileInfo.FileName
	}

	bar, ctx, hideDialog, ready := app.downloadProgressDialog(
		app.msg("download_title"), filename,
	)
	<-ready

	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	saveFilename := filename
	if fallbackFileName != "" {
		saveFilename = fallbackFileName
	}
	safeFilename, err := sanitizeFilename(saveFilename)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Invalid filename: %v", err))
		hideDialog()
		return
	}
	dest := filepath.Join(modsPath, safeFilename)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				app.appendLogToFile(fmt.Sprintf("PANIC in startDownload goroutine: %v", r))
				app.appendLogToFile(fmt.Sprintf("Stack: %s", debug.Stack()))
			}
		}()

		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: starting download to %s", dest))
		err := app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		hideDialog()

		if err != nil {
			if err == context.Canceled {
				app.appendLog(app.msg("download_cancelled"))
			} else {
				app.appendLog(fmt.Sprintf(app.msg("download_failed"), err))
			}
			os.Remove(dest)
			return
		}

		app.appendLogToFile("DownloadFileWithProgress: download completed successfully")

		info, e := os.Stat(dest)
		if e != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_downloaded_file_not_found"), e))
			return
		}
		app.appendLog(fmt.Sprintf(app.msg("log_downloaded_file_size"), float64(info.Size())/1024/1024))

		if info.Size() < 100 {
			app.appendLog(fmt.Sprintf(app.msg("log_error_file_too_small"), safeFilename, info.Size()))
			os.Remove(dest)
			return
		}

		app.appendLogToFile(fmt.Sprintf("InstallModFromArchive: starting installation for %s", modName))

		// knownVersion может быть "", тогда InstallModFromArchive попробует
		// вытащить версию из имени файла или спросить у пользователя.
		installedName, installedVersion, err := app.InstallModFromArchive(dest, true, knownVersion, modName)
		app.appendLogToFile(fmt.Sprintf("InstallModFromArchive: finished with err=%v, installedName=%s", err, installedName))
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_install_failed"), err))
			return
		}

		os.Remove(dest)
		if modID != "" && installedName != "" {
			cacheKey := modID + ":" + installedName
			// cacheModVersion сам ничего не пишет, если version == "".
			// Если InstallModFromArchive вернул "unknown", предпочтём его,
			// но если вернул пусто — попробуем knownVersion (не пусто ли).
			ver := installedVersion
			if ver == "" {
				ver = knownVersion
			}
			app.cacheModVersion(cacheKey, installedName, ver, knownTimestamp, "nexus", 0)
		}
		if installedName != "" {
			app.selectAndScrollToMod(installedName)
		}
		if modID != "" {
			mid, err := strconv.Atoi(modID)
			if err == nil {
				// fallbackFileName уже безопасен при fileInfo == nil
				go app.autoAddModToDatabase(mid, installedName, fallbackFileName)
			}
		}
	}()
}

// Специальные диалоги для системных модов
func (app *App) showDMLDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.cfgMutex.RLock()
	autoConfirm := app.cfg.AutoConfirmDownloadEnabled()
	app.cfgMutex.RUnlock()

	run := func() {
		app.startSystemDownload(downloadURL, filename, "Darktide Mod Loader", fileInfo, "19:base", app.installDMLFromArchive, app.msg("installing_dml"), app.msg("dml_updated"))
	}

	if autoConfirm {
		run()
		return
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Loader", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			// Тот же случай, что и в showDownloadDialog: run() → startSystemDownload
			// с fyne.Do + ожиданием. Из UI-потока это deadlock.
			go run()
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

func (app *App) showDMFDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.cfgMutex.RLock()
	autoConfirm := app.cfg.AutoConfirmDownloadEnabled()
	app.cfgMutex.RUnlock()

	run := func() {
		app.startSystemDownload(downloadURL, filename, "Darktide Mod Framework", fileInfo, "8:dmf", app.installDMLFromArchive, app.msg("installing_dmf"), app.msg("log_dmf_updated_succ"))
	}

	if autoConfirm {
		// app.appendLog(fmt.Sprintf(app.msg("log_downloading_mod"), "Darktide Mod Framework"))
		run()
		return
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Framework", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			// Тот же случай, что и в showDownloadDialog: run() → startSystemDownload
			// с fyne.Do + ожиданием. Из UI-потока это deadlock.
			go run()
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

func (app *App) showAutopatcherDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.cfgMutex.RLock()
	autoConfirm := app.cfg.AutoConfirmDownloadEnabled()
	app.cfgMutex.RUnlock()

	run := func() {
		app.startSystemDownload(downloadURL, filename, "Darktide Mod Autopatcher", fileInfo, "709:autopatch", app.installAutopatcherFromArchive, app.msg("installing_autopatcher"), app.msg("autopatcher_updated"))
	}

	if autoConfirm {
		run()
		return
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Autopatcher", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			// Тот же случай, что и в showDownloadDialog: run() → startSystemDownload
			// с fyne.Do + ожиданием. Из UI-потока это deadlock.
			go run()
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

// startSystemDownload - общая логика скачивания для системных модов.
//
// Как и startDownload, использует ready-барьер: fyne.Do асинхронный,
// и без синхронизации горутина скачивания стартанула бы раньше, чем
// bar/dlg будут присвоены — DownloadFileWithProgress упал бы на
// nil-pointer при bar.SetValue.
func (app *App) startSystemDownload(downloadURL, filename, displayName string, fileInfo *FileInfo, cacheKey string, installFunc func(string) error, logInstalling, logSuccess string) {
	app.appendLog(fmt.Sprintf(app.msg("log_downloading_mod"), displayName))

	bar, ctx, hideDialog, ready := app.downloadProgressDialog(
		app.msg("download_title"), filename,
	)
	<-ready

	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	go func() {
		safeFilename, err := sanitizeFilename(filename)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("Invalid filename: %v", err))
			hideDialog()
			return
		}
		dest := filepath.Join(modsPath, safeFilename)

		err = app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		hideDialog()
		if err != nil {
			if err == context.Canceled {
				app.appendLog(app.msg("download_cancelled"))
			} else {
				app.appendLog(fmt.Sprintf(app.msg("download_failed"), err))
			}
			return
		}
		info, e := os.Stat(dest)
		if e == nil && info.Size() < 100 {
			app.appendLog(fmt.Sprintf(app.msg("log_error_file_too_small"), safeFilename, info.Size()))
			os.Remove(dest)
			return
		}
		app.appendLog(logInstalling)
		if err := installFunc(dest); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_install_failed"), err))
		} else {
			if fileInfo != nil {
				folderName := displayName
				parts := strings.Split(cacheKey, ":")
				if len(parts) == 2 {
					folderName = parts[1]
				}
				modID, _ := strconv.Atoi(parts[0])
				app.setCachedVersion(cacheKey, ModVersionInfo{
					Timestamp:   fileInfo.UploadedTimestamp,
					Version:     fileInfo.Version,
					Folder:      folderName,
					Source:      "nexus",
					InstalledAt: time.Now().Unix(),
				})
				select {
				case app.batchInstallChan <- BatchSignal{Folder: folderName, NexusModID: modID}:
				default:
				}
			}
			app.appendLog(logSuccess)
		}
		os.Remove(dest)
	}()
}

// Функции обновления системных модов (вызываются из кнопок)

func (app *App) updateDML() {
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}
	const dmlModID = 19
	app.appendLog(fmt.Sprintf(app.msg("looking_for_latest_file"), dmlModID))
	fileInfo, err := app.getLatestFileInfo(dmlModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_latest_file_id"), err))
		return
	}
	cacheKey := "19:base"
	saved, exists := app.getCachedVersion(cacheKey)
	// Проверяем только актуальность по дате, игнорируем source
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.msg("already_latest"), "DML", fileInfo.Version))
		return
	}
	// Если был ручной, сообщим об этом (опционально)
	if exists && saved.Source == "manual" {
		app.appendLogToFile("DML was installed manually, but you chose to update - overwriting with Nexus version.")
	}
	directURL, filename, err := app.getPremiumDownloadURL("19", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
		return
	}
	app.showDMLDownloadDialog(directURL, filename, fileInfo)
}

func (app *App) updateDMF() {
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}
	const dmfModID = 8
	app.appendLog(fmt.Sprintf(app.msg("looking_for_latest_file"), dmfModID))
	fileInfo, err := app.getLatestFileInfo(dmfModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_latest_file_id"), err))
		return
	}
	cacheKey := "8:dmf"
	saved, exists := app.getCachedVersion(cacheKey)
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.msg("already_latest"), "DMF", fileInfo.Version))
		return
	}
	if exists && saved.Source == "manual" {
		app.appendLogToFile("DMF was installed manually, but you chose to update - overwriting with Nexus version.")
	}
	directURL, filename, err := app.getPremiumDownloadURL("8", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
		return
	}
	app.showDMFDownloadDialog(directURL, filename, fileInfo)
}

func (app *App) updateAutopatcher() {
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}
	const autopatchModID = 709
	app.appendLog(fmt.Sprintf(app.msg("looking_for_latest_file"), autopatchModID))
	fileInfo, err := app.getLatestFileInfo(autopatchModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_latest_file_id"), err))
		return
	}
	cacheKey := "709:autopatch"
	saved, exists := app.getCachedVersion(cacheKey)
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.msg("already_latest"), "Autopatcher", fileInfo.Version))
		return
	}
	if exists && saved.Source == "manual" {
		app.appendLogToFile("Autopatcher was installed manually, but you chose to update - overwriting with Nexus version.")
	}
	directURL, filename, err := app.getPremiumDownloadURL("709", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
		return
	}
	app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
}

// Processing nxm links
func (app *App) handleNXMLink(nxmURL string) {
	if app.nxm.IsDuplicate(nxmURL) {
		app.appendLog(app.msg("nxm_already_processing"))
		return
	}
	app.bringToFront()

	// Проверяем, не является ли это коллекцией
	if link, ok := parseCollectionLink(nxmURL); ok {
		go app.installNexusCollection(link.Slug, link.Rev)
		return
	}

	u, err := url.Parse(nxmURL)
	if err != nil {
		app.appendLogToFile(app.msg("log_invalid_nxm_link"))
		return
	}

	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	var modID, fileID string
	for i := 0; i < len(segments)-1; i++ {
		if segments[i] == "mods" && i+1 < len(segments) {
			modID = segments[i+1]
		}
		if segments[i] == "files" && i+1 < len(segments) {
			fileID = segments[i+1]
		}
	}
	if modID == "" || fileID == "" {
		app.appendLogToFile(app.msg("log_invalid_nxm_link"))
		return
	}

	key := u.Query().Get("key")
	expires := u.Query().Get("expires")

	if modID == "19" {
		go func() {
			var fileInfo *FileInfo
			if mid, _ := strconv.Atoi(modID); mid > 0 {
				if fi, err := app.getLatestFileInfo(mid); err == nil {
					fileInfo = fi
				}
			}
			var directURL, filename string
			var err error
			if key != "" && expires != "" {
				// Free method - pass both parameters
				directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
			} else if key != "" && expires == "" {
				// Incomplete link - missing expires
				err = fmt.Errorf("incomplete nxm link: missing expires")
			} else {
				// Premium method (requires OAuth token and premium account)
				directURL, filename, err = app.getPremiumDownloadURL(modID, fileID)
			}
			if err != nil {
				app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
				return
			}
			// Вызываем напрямую из фоновой горутины: showDMLDownloadDialog
			// сам делает fyne.Do там, где надо (appendLog, showChoiceDialog),
			// а startSystemDownload ждёт UI-барьер — если обернуть снаружи,
			// UI-поток окажется занят и барьер никогда не закроется.
			app.showDMLDownloadDialog(directURL, filename, fileInfo)
		}()
		return
	}

	if modID == "709" {
		go func() {
			var fileInfo *FileInfo
			if mid, _ := strconv.Atoi(modID); mid > 0 {
				if fi, err := app.getLatestFileInfo(mid); err == nil {
					fileInfo = fi
				}
			}
			var directURL, filename string
			var err error
			if key != "" && expires != "" {
				// Free method - pass both parameters
				directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
			} else if key != "" && expires == "" {
				// Incomplete link - missing expires
				err = fmt.Errorf("incomplete nxm link: missing expires")
			} else {
				// Premium method (requires OAuth token and premium account)
				directURL, filename, err = app.getPremiumDownloadURL(modID, fileID)
			}
			if err != nil {
				app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
				return
			}
			app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
		}()
		return
	}

	go func() {
		var directURL, filename string
		var err error
		if key != "" && expires != "" {
			// Free method - pass both parameters
			directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
		} else if key != "" && expires == "" {
			// Incomplete link - missing expires
			err = fmt.Errorf("incomplete nxm link: missing expires")
		} else {
			// Premium method (requires OAuth token and premium account)
			directURL, filename, err = app.getPremiumDownloadURL(modID, fileID)
		}
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
			return
		}
		mid, _ := strconv.Atoi(modID)
		var fileInfo *FileInfo
		if mid > 0 && fileID != "" {
			if fi, err := app.getFileInfoByID(modID, fileID); err == nil {
				fileInfo = fi
			}
		}
		modName := "Mod " + modID
		if mid > 0 {
			if info, err := app.FetchNexusModInfo(mid, app.getAuthToken()); err == nil {
				modName = info.Name
			}
		}
		app.showDownloadDialog(directURL, filename, modName, fileInfo, modID)
	}()
}

func (app *App) showEditVersionDialog(mod *checks.ModInfo) {
	var cacheKey string
	switch mod.Name {
	case "dmf":
		cacheKey = "8:dmf"
	case "base":
		cacheKey = "19:base"
	case "autopatch":
		cacheKey = "709:autopatch"
	default:
		if mod.URL != "" {
			modID := helpers.ExtractModIDFromURL(mod.URL)
			if modID != 0 {
				cacheKey = fmt.Sprintf("%d:%s", modID, mod.Name)
			}
		}
	}
	if cacheKey == "" {
		app.appendLogToFile(app.msg("log_cannot_determine_cache_key"))
		return
	}

	currentVersion := ""
	if info, ok := app.getCachedVersion(cacheKey); ok {
		currentVersion = info.Version
	}

	entry := widget.NewEntry()
	entry.SetText(currentVersion)
	entry.SetPlaceHolder(app.msg("placeholder_mod_version"))

	var popUp *widget.PopUp

	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf(app.msg("edit_version_current"), mod.DisplayName, currentVersion)),
		entry,
		container.NewHBox(
			NewCustomIconButton(app.msg("btn_save"), app.loadIconResource("save", "assets/buttons/save.png"), func() {
				newVersion := strings.TrimSpace(entry.Text)
				if newVersion == "" {
					app.appendLog(app.msg("log_cannot_version_empty"))
					return
				}
				app.setCachedVersion(cacheKey, ModVersionInfo{
					Timestamp: time.Now().Unix(),
					Version:   newVersion,
					Folder:    mod.Name,
					Source:    "manual",
				})
				app.saveNexusVersionCache()
				app.appendLog(fmt.Sprintf(app.msg("log_version_for_updated_to"), mod.DisplayName, newVersion))
				popUp.Hide()
				app.updateDescriptionForMod(mod.Name)
			}),
			NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
				popUp.Hide()
			}),
		),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(WindowWidth400, WindowHeight120))
	popUp.Show()
}

// showProgressDialog создаёт модальный диалог с заголовком, сообщением и прогресс-баром.
// Возвращает прогресс-бар, метку, канал отмены и функцию закрытия диалога.
func (app *App) showProgressDialog(title, message string) (*widget.ProgressBar, *widget.Label, <-chan struct{}, func()) {
	bar := widget.NewProgressBar()
	bar.SetValue(0)

	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(label, bar)

	var dlg *dialog.CustomDialog
	cancelChan := make(chan struct{})
	var once sync.Once
	ready := make(chan struct{})

	fyne.Do(func() {
		app.bringToFront()
		dlg = dialog.NewCustom(title, app.msg("btn_cancel"), content, app.mainWindow)
		dlg.Resize(fyne.NewSize(400, 120))

		dlg.SetOnClosed(func() {
			once.Do(func() {
				close(cancelChan)
			})
		})

		dlg.Show()
		close(ready)
	})
	<-ready

	closeDialogFunc := func() {
		if dlg != nil {
			fyne.Do(func() {
				dlg.Hide()
			})
		}
	}

	return bar, label, cancelChan, closeDialogFunc
}

// stripHTML удаляет HTML-теги и преобразует <br> и <li> в переносы строк.
var htmlTagStripRe = regexp.MustCompile(`<[^>]*>`)

func stripHTML(html string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</li>", "\n", "<li>", "- ",
	)
	text := replacer.Replace(html)
	text = htmlTagStripRe.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// showCreateProfileDialog показывает диалог создания профиля
func (app *App) showCreateProfileDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(app.msg("profile_name_placeholder"))

	// Чекбокс "Копировать из текущего"
	copyCheck := widget.NewCheck(app.msg("profile_copy_from_current"), nil)
	copyCheck.SetChecked(true)

	// Предупреждение о создании чистого профиля
	warningLabel := widget.NewLabel(app.msg("profile_clean_warning"))
	warningLabel.Wrapping = fyne.TextWrapWord
	warningLabel.Importance = widget.WarningImportance
	warningLabel.Hide() // Скрыто по умолчанию

	copyCheck.OnChanged = func(b bool) {
		if !b {
			warningLabel.Show()
		} else {
			warningLabel.Hide()
		}
	}

	var popUp *widget.PopUp

	// Кнопки с центрированием
	btnCreate := NewCustomIconButton(app.msg("btn_create"), app.loadIconResource("add", "assets/buttons/add.png"), func() {
		name := strings.TrimSpace(entry.Text)
		if name == "" {
			app.showInfoDialog(app.msg("error_title"), app.msg("profile_name_empty"))
			return
		}
		copyFrom := ""
		if copyCheck.Checked {
			app.cfgMutex.RLock()
			copyFrom = app.cfg.ActiveProfile
			app.cfgMutex.RUnlock()
		}
		popUp.Hide()

		go func() {
			// Синхронизируем активный профиль, чтобы копия была свежей.
			if copyFrom != "" {
				if err := app.syncActiveProfileFromGame(); err != nil {
					app.appendLogToFile(fmt.Sprintf("createProfile: sync failed: %v", err))
					// продолжаем — в профиле останется последняя синхронизация
				}
			}
			if err := app.createProfile(name, copyFrom); err != nil {
				fyne.Do(func() {
					app.appendLogToFile(fmt.Sprintf("Failed to create profile: %v", err))
					app.showInfoDialog(app.msg("error_title"), app.msg("profile_create_failed"))
				})
				return
			}
			fyne.Do(func() {
				app.switchProfile(name)
			})
		}()
	})

	btnCancel := NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
		popUp.Hide()
	})
	btnContainer := container.NewCenter(
		container.NewHBox(btnCreate, btnCancel),
	)

	content := container.NewVBox(
		helpers.BoldCenterLabel(app.msg("profile_create_title")),
		widget.NewLabel(app.msg("profile_create_message")),
		entry,
		copyCheck,
		warningLabel,
		widget.NewSeparator(),
		btnContainer,
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(WindowWidth400, WindowHeight200))
	popUp.Show()
	app.mainWindow.Canvas().Focus(entry)
}

// showRenameProfileDialog показывает диалог переименования профиля
func (app *App) showRenameProfileDialog() {
	app.cfgMutex.RLock()
	currentProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()

	if currentProfile == "Default" {
		app.appendLog(app.msg("profile_cannot_rename_default"))
		return
	}
	entry := widget.NewEntry()
	entry.SetText(currentProfile)
	entry.SetPlaceHolder(app.msg("profile_name_placeholder"))

	var popUp *widget.PopUp

	btnRename := NewCustomIconButton(app.msg("btn_rename"), app.loadIconResource("edit_version", "assets/buttons/edit_version.png"), func() {
		newName := strings.TrimSpace(entry.Text)
		if newName == "" {
			app.showInfoDialog(app.msg("error_title"), app.msg("profile_name_empty"))
			return
		}
		if err := app.renameProfile(currentProfile, newName); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to rename profile: %v", err))
			app.showInfoDialog(app.msg("error_title"), app.msg("profile_rename_failed"))
			return
		}
		app.refreshProfileList()
		popUp.Hide()
	})

	btnCancel := NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
		popUp.Hide()
	})

	btnContainer := container.NewCenter(container.NewHBox(btnRename, btnCancel))

	content := container.NewVBox(
		helpers.BoldCenterLabel(app.msg("profile_rename_title")),
		widget.NewLabel(fmt.Sprintf(app.msg("profile_rename_message"), currentProfile)),
		entry,
		widget.NewSeparator(),
		btnContainer,
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(WindowWidth400, WindowHeight160))
	popUp.Show()
	app.mainWindow.Canvas().Focus(entry)
}

// showDeleteProfileDialog показывает диалог удаления профиля
func (app *App) showDeleteProfileDialog() {
	app.cfgMutex.RLock()
	currentProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()

	if currentProfile == "Default" {
		app.showInfoDialog(app.msg("error_title"), app.msg("profile_cannot_delete_default"))
		return
	}

	nameToDelete := currentProfile

	// Собираем другие нестандартные профили (исключая Default и удаляемый)
	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to read profiles: %v", err))
		return
	}
	var otherProfiles []string
	for _, e := range entries {
		if e.IsDir() && e.Name() != nameToDelete && e.Name() != "Default" {
			otherProfiles = append(otherProfiles, e.Name())
		}
	}

	// Если других нет - переключимся на Default (он всегда существует)
	target := "Default"
	if len(otherProfiles) > 0 {
		target = otherProfiles[0]
	}

	app.showConfirmDialog(
		app.msg("profile_delete_title"),
		fmt.Sprintf(app.msg("profile_delete_message"), nameToDelete),
		func() {
			go func() {
				// 1. Переключиться на target и дождаться завершения.
				done := make(chan struct{})
				fyne.Do(func() {
					app.switchProfileAsync(target, func() { close(done) })
				})
				<-done

				// 2. Бэкап перед удалением.
				if _, err := app.CreateProfileBackup(nameToDelete); err != nil {
					fyne.Do(func() {
						app.appendLogToFile(fmt.Sprintf("deleteProfile: backup failed: %v", err))
						app.showInfoDialog(app.msg("error_title"), app.msg("profile_backup_failed"))
					})
					return
				}

				// 3. Удаление.
				if err := app.deleteProfile(nameToDelete); err != nil {
					fyne.Do(func() {
						app.appendLogToFile(fmt.Sprintf("Failed to delete profile: %v", err))
						app.showInfoDialog(app.msg("error_title"), app.msg("profile_delete_failed"))
					})
					return
				}

				fyne.Do(func() {
					app.refreshProfileList()
					app.appendLog(fmt.Sprintf(app.msg("profile_deleted"), nameToDelete))
				})
			}()
		},
	)
}

// showImportProfileDialog показывает диалог импорта профиля
func (app *App) showImportProfileDialog() {
	// Используем FileOpen, чтобы можно было выбрать zip или папку
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		srcPath := filepath.FromSlash(reader.URI().Path())
		ext := strings.ToLower(filepath.Ext(srcPath))
		var profileFolder string
		var tmpDir string

		if ext == ".zip" {
			// Это архив - распаковываем во временную папку
			var err error
			tmpDir, err = os.MkdirTemp("", "profile-import-")
			if err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to create temp directory: %v", err))
				return
			}
			defer os.RemoveAll(tmpDir)

			if err := app.extractZipArchive(srcPath, tmpDir); err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to extract zip archive: %v", err))
				return
			}

			// Ожидаем, что внутри архива будет одна папка (имя профиля)
			entries, err := os.ReadDir(tmpDir)
			if err != nil || len(entries) != 1 || !entries[0].IsDir() {
				app.appendLogToFile(app.msg("profile_import_one_folder"))
				return
			}
			profileFolder = filepath.Join(tmpDir, entries[0].Name())
		} else {
			// Это папка
			profileFolder = srcPath
		}

		// Проверяем, что внутри есть папка mods
		modsPath := filepath.Join(profileFolder, "mods")
		if _, err := os.Stat(modsPath); os.IsNotExist(err) {
			app.appendLog(app.msg("profile_import_no_mods"))
			return
		}

		// Определяем имя нового профиля
		baseName := filepath.Base(profileFolder)
		if app.profileExists(baseName) {
			baseName = baseName + "_imported"
		}

		// Используем логику restore, она правильно разложит файлы
		if err := app.restoreProfileFromBackup(srcPath, baseName); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to import profile: %v", err))
			return
		}

		app.appendLog(fmt.Sprintf(app.msg("profile_imported"), baseName))
		app.refreshProfileList()
		app.switchProfile(baseName)

	}, app.mainWindow)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip"})) // опционально
	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}

// showExportProfileDialog показывает диалог экспорта профиля
func (app *App) showExportProfileDialog() {
	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		dstDir := filepath.FromSlash(uri.Path())

		go func() {
			// Обязательный синк перед экспортом.
			if err := app.syncActiveProfileFromGame(); err != nil {
				app.appendLogToFile(fmt.Sprintf("exportProfile: sync failed: %v", err))
				fyne.Do(func() {
					app.showInfoDialog(app.msg("error_title"), err.Error())
				})
				return
			}

			app.cfgMutex.RLock()
			profileName := app.cfg.ActiveProfile
			app.cfgMutex.RUnlock()

			zipPath := filepath.Join(dstDir, profileName+".zip")

			// Используем логику полноценного бэкапа (с системными файлами)
			tempBackupPath, err := app.CreateProfileBackup(profileName)
			if err != nil {
				app.appendLogToFile(fmt.Sprintf("Export profile: failed to create backup: %v", err))
				return
			}

			// Копируем готовый zip в выбранную пользователем папку
			if err := copyFile(tempBackupPath, zipPath); err != nil {
				app.appendLogToFile(fmt.Sprintf("Export profile: failed to copy zip: %v", err))
				return
			}
			// Временный архив (созданный в папке бэкапов) удаляем, чтобы не засорять
			_ = os.Remove(tempBackupPath)

			fyne.Do(func() {
				app.appendLog(fmt.Sprintf(app.msg("profile_exported"), zipPath))
			})
		}()
	}, app.mainWindow)
	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}

// showInfiniteProgressDialog — модальный диалог с инфинитным прогрессом.
//
// cancellable == true  — показывает кнопку «Отмена», закрытие крестиком
//
//	и Escape закрывают окно и сигналят в cancelChan.
//
// cancellable == false — кнопки нет, окно не закрывается ничем, кроме
//
//	вызова closeDialog. Применяется для операций,
//	которые опасно прерывать (смена профиля,
//	закрытие программы).
func (app *App) showInfiniteProgressDialog(title, message string, cancellable bool) (<-chan struct{}, func()) {
	bar := widget.NewProgressBarInfinite()
	bar.Start()

	label := widget.NewLabel(message)
	label.Wrapping = fyne.TextWrapWord

	content := container.NewVBox(label, bar)

	var dlg *dialog.CustomDialog
	cancelChan := make(chan struct{})
	var once sync.Once
	ready := make(chan struct{})

	fyne.Do(func() {
		app.bringToFront()

		if cancellable {
			// С кнопкой отмены
			dlg = dialog.NewCustom(title, app.msg("btn_cancel"), content, app.mainWindow)
			dlg.SetOnClosed(func() {
				once.Do(func() { close(cancelChan) })
			})
		} else {
			// Совсем без кнопок (чистый прогресс-бар)
			dlg = dialog.NewCustomWithoutButtons(title, content, app.mainWindow)
		}

		dlg.Resize(fyne.NewSize(400, 120))
		dlg.Show()
		close(ready)
	})
	<-ready

	return cancelChan, func() {
		if dlg != nil {
			fyne.Do(func() {
				dlg.Hide()
			})
		}
	}
}

// createActiveProfileBackupFromUI — ручное создание бэкапа активного
// профиля (из меню).
func (app *App) createActiveProfileBackupFromUI() {
	go func() {
		app.cfgMutex.RLock()
		active := app.cfg.ActiveProfile
		app.cfgMutex.RUnlock()

		_, closeDialog := app.showInfiniteProgressDialog(
			app.msg("backup_creating_title"),
			app.msg("backup_creating_message"),
			false,
		)
		defer closeDialog()

		// Синк перед бэкапом, чтобы архив был свежим.
		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLogToFile(fmt.Sprintf("backup: sync failed: %v", err))
		}

		zipPath, err := app.CreateProfileBackup(active)
		fyne.Do(func() {
			if err != nil {
				app.appendLog(fmt.Sprintf(app.msg("backup_failed"), err.Error()))
				return
			}
			app.appendLog(fmt.Sprintf(app.msg("backup_created"), filepath.Base(zipPath)))
		})
	}()
}

// showRestoreBackupDialog — выбор zip-файла из бэкапов.
func (app *App) showRestoreBackupDialog() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		zipPath := filepath.FromSlash(reader.URI().Path())

		go func() {
			// Имя целевого профиля: убираем ts_ префикс, добавляем _restored.
			base := strings.TrimSuffix(filepath.Base(zipPath), ".zip")
			if idx := strings.Index(base, "_"); idx > 0 {
				if _, perr := time.Parse("20060102_150405", base[:idx]); perr == nil {
					base = base[idx+1:]
				}
			}
			profileName := base + "_restored"
			if app.profileExists(profileName) {
				profileName = fmt.Sprintf("%s_%d", profileName, time.Now().Unix())
			}

			_, closeDialog := app.showInfiniteProgressDialog(
				app.msg("backup_restoring_title"),
				app.msg("backup_restoring_message"),
				false,
			)
			defer closeDialog()

			if err := app.restoreProfileFromBackup(zipPath, profileName); err != nil {
				fyne.Do(func() {
					app.appendLogToFile(fmt.Sprintf("restore backup: %v", err))
					app.showInfoDialog(app.msg("error_title"), err.Error())
				})
				return
			}

			fyne.Do(func() {
				app.refreshProfileList()
				app.appendLog(fmt.Sprintf(app.msg("backup_restored"), profileName))
				// Сразу переключаемся на восстановленный профиль,
				// чтобы пользователь увидел сборку и системные файлы подхватились.
				app.switchProfile(profileName)
			})
		}()
	}, app.mainWindow)

	fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip"}))

	if dir := app.backupsDir(); dir != "" {
		uri := storage.NewFileURI(dir)
		if lister, err := storage.ListerForURI(uri); err == nil {
			fd.SetLocation(lister)
		} else {
			app.appendLogToFile(fmt.Sprintf("showRestoreBackupDialog: can't list backups dir: %v", err))
		}
	}

	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}

// openBackupsFolder — открыть папку с бэкапами в файловом менеджере.
func (app *App) openBackupsFolder() {
	app.openPathInFileManager(app.backupsDir())
}

// showEditUserNoteDialog открывает модальное окно редактирования
// пользовательской заметки к моду.
func (app *App) showEditUserNoteDialog(modName, displayName string) {
	current := app.userNotes.Get(modName)

	entry := widget.NewMultiLineEntry()
	entry.SetText(current)
	entry.SetPlaceHolder(app.msg("user_note_placeholder"))
	entry.Wrapping = fyne.TextWrapWord

	var popUp *widget.PopUp

	// Иконка
	saveIcon := app.loadIconResource("save", "assets/buttons/save.png")
	// Создаём кнопку через кастомную функцию
	saveBtn := NewCustomIconButton(app.msg("btn_save"), saveIcon, func() {
		app.userNotes.Set(modName, entry.Text)
		if err := app.userNotes.Save(); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to save user_notes.json: %v", err))
			app.showInfoDialog(app.msg("error_title"), err.Error())
		} else {
			app.appendLogToFile(fmt.Sprintf("User note saved for %s", modName))
		}
		popUp.Hide()

		// refreshModList() ниже перечитывает mod_load_order.txt с диска
		// и заменяет app.allMods целиком. Если у пользователя были
		// несохранённые переключения Active — они живут только в памяти
		// и пропали бы молча. Сохраняем порядок первым, чтобы пересборка
		// читала уже актуальное состояние.
		if app.orderDirty.Load() {
			app.saveCurrentOrder()
			app.orderDirty.Store(false)
			app.stopBlinkSaveButton()
			app.updateTableBorder()
		}

		app.refreshModList()
		app.updateDescriptionForMod(modName)
	})

	clearBtn := NewCustomIconButton(app.msg("btn_clear"), app.loadIconResource("clear", "assets/buttons/backsp.png"), func() {
		entry.SetText("")
	})

	cancelBtn := NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
		popUp.Hide()
	})
	title := fmt.Sprintf(app.msg("user_note_title"), displayName)
	if displayName == "" {
		title = fmt.Sprintf(app.msg("user_note_title"), modName)
	}

	content := container.NewVBox(
		helpers.BoldCenterLabel(title),
		widget.NewSeparator(),
		entry,
		widget.NewSeparator(),
		container.NewCenter(container.NewHBox(saveBtn, clearBtn, cancelBtn)),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(WindowWidth550, WindowHeight170))
	popUp.Show()
	app.mainWindow.Canvas().Focus(entry)
}

// estimateMessageHeight возвращает высоту в пикселях для сообщения
// диалога, оценённую по числу визуальных строк.
//
// Точную высоту посчитать нельзя без вызова layout'а с финальной
// шириной (TextWrapWord переносит по фактическому размеру). Но
// грубой оценки достаточно: диалог всё равно имеет жёсткий
// потолок по высоте и скролл, если текста больше.
//
// Алгоритм:
//  1. Разбиваем по "\n" — это явные строки.
//  2. Каждую длинную строку делим на «псевдостроки» по ~50
//     символов. Это типичное число символов, влезающих в
//     диалог шириной 500px при TextSize=14. Для кириллицы
//     ~45, для латиницы ~55 — берём среднее.
//  3. Умножаем на ~22px (высота строки в Fyne при дефолтной
//     теме) + padding скролла.
//  4. Ограничиваем диапазоном [80, 400] пикселей.
//
// Примеры:
//
//	"Удалить мод X?"           → 80px  (одна строка, попадает в минимум)
//	"Список (10 модов)"        → ~240px
//	"Список (40 модов)"        → 400px (потолок, дальше скролл)
func estimateMessageHeight(message string) float32 {
	const (
		charsPerLine = 50
		lineHeightPx = 22
		minHeightPx  = 80
		maxHeightPx  = 400
	)

	visualLines := 0
	for _, line := range strings.Split(message, "\n") {
		if line == "" {
			// Пустая строка — тоже занимает одну визуальную.
			visualLines++
			continue
		}
		// (len + charsPerLine - 1) / charsPerLine — целочисленное
		// округление вверх без math.Ceil.
		visualLines += (len(line) + charsPerLine - 1) / charsPerLine
	}
	if visualLines == 0 {
		visualLines = 1
	}

	h := float32(visualLines * lineHeightPx)
	if h < minHeightPx {
		return minHeightPx
	}
	if h > maxHeightPx {
		return maxHeightPx
	}
	return h
}

// showEnlargedImage показывает картинку в модальном окне на 80% экрана.
func (app *App) showEnlargedImage(imgRes fyne.Resource) {
	if imgRes == nil {
		return
	}

	fyne.Do(func() {
		app.bringToFront()

		img := canvas.NewImageFromResource(imgRes)
		img.FillMode = canvas.ImageFillContain

		var popUp *widget.PopUp

		// Кнопка закрытия
		closeBtn := NewCustomIconButton(app.msg("theme_editor_btn_close"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
			popUp.Hide()
		})
		closeBtn.Importance = widget.HighImportance

		const padding float32 = 5

		topSpacer := canvas.NewRectangle(color.Transparent)
		topSpacer.SetMinSize(fyne.NewSize(0, padding))

		bottomSpacer := canvas.NewRectangle(color.Transparent)
		bottomSpacer.SetMinSize(fyne.NewSize(0, padding))

		leftSpacer := canvas.NewRectangle(color.Transparent)
		leftSpacer.SetMinSize(fyne.NewSize(padding, 0))

		rightSpacer := canvas.NewRectangle(color.Transparent)
		rightSpacer.SetMinSize(fyne.NewSize(padding, 0))

		// Создаем копию заголовка для попапа
		popupTitle := canvas.NewText(app.descTitle.Text, app.descTitle.Color)
		popupTitle.TextSize = app.descTitle.TextSize
		popupTitle.TextStyle = app.descTitle.TextStyle
		popupTitle.Alignment = fyne.TextAlignCenter

		// Верхняя часть
		topArea := container.NewVBox(
			topSpacer,
			container.NewCenter(popupTitle),
			widget.NewSeparator(),
		)

		// Нижняя часть
		bottomArea := container.NewVBox(
			widget.NewSeparator(),
			container.NewCenter(closeBtn),
			bottomSpacer,
		)

		// Собираем контент с отступами со всех сторон
		content := container.NewBorder(
			topArea,
			bottomArea,
			leftSpacer,
			rightSpacer,
			img,
		)

		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())

		winSize := app.mainWindow.Canvas().Size()
		popUp.Resize(fyne.NewSize(winSize.Width*0.8, winSize.Height*0.8))
		popUp.Show()
	})
}

// === УСТАНОВКА КОЛЛЕКЦИИ С НЕКСУС ===
func (app *App) installNexusCollection(slug string, rev int) {
	col, err := app.fetchNexusCollection(slug, rev)
	if err != nil {
		app.appendLog(fmt.Sprintf("Failed to fetch collection: %v", err))
		return
	}

	app.cfgMutex.RLock()
	lang := app.cfg.Language
	app.cfgMutex.RUnlock()

	// Создаём карту NexusID -> ModDBEntry для быстрого поиска
	dbMap := make(map[int]checks.ModDBEntry)
	for _, entry := range checks.GetModDBList() {
		if entry.URL != "" {
			if id := helpers.ExtractModIDFromURL(entry.URL); id != 0 {
				dbMap[id] = entry
			}
		}
	}

	var items []*BatchItem

	// ─── ШАГ 1: ПРИНУДИТЕЛЬНО ДОБАВЛЯЕМ DML И DMF ───────────────────
	// Создаем элементы для DML и DMF. Ставим Selected: true, чтобы они
	// точно установились, даже если мы переключаемся на новый (пустой) профиль.
	dmlItem := &BatchItem{
		ID:          "base",
		NexusModID:  19,
		Name:        "Darktide Mod Loader",
		Description: "Required for all mods. (Added automatically)",
		Selected:    true,
		Status:      BatchPending,
	}
	dmfItem := &BatchItem{
		ID:          "dmf",
		NexusModID:  8,
		Name:        "Darktide Mod Framework",
		Description: "Required for most mods. (Added automatically)",
		Selected:    true,
		Status:      BatchPending,
	}

	// Если пользователь Premium, подтягиваем актуальные FileID для них,
	// чтобы скачивание прошло автоматически.
	if app.getAuthToken() != "" {
		if fi, err := app.getLatestFileInfo(19); err == nil {
			dmlItem.FileID = fi.ID
			dmlItem.Version = fi.Version
		}
		if fi, err := app.getLatestFileInfo(8); err == nil {
			dmfItem.FileID = fi.ID
			dmfItem.Version = fi.Version
		}
	}

	// Строго заданный порядок: сначала DML, затем DMF.
	items = append(items, dmlItem, dmfItem)

	// ─── ШАГ 2: ДОБАВЛЯЕМ ОСТАЛЬНЫЕ МОДЫ ────────────────────────────
	for _, m := range col.Mods {
		// Пропускаем DML и DMF, если автор их добавил в коллекцию,
		// так как мы уже поставили их в самое начало.
		if m.ModID == 19 || m.ModID == 8 {
			continue
		}

		author, category, desc, picURL := "Unknown", "Unknown", "", ""
		modName := m.Name

		if db, ok := dbMap[m.ModID]; ok {
			author = db.Author
			category = db.Category
			if locName := checks.PickLocalized(db.Name, lang); locName != "" {
				modName = locName
			}
			desc = checks.PickLocalized(db.Description, lang)
			if desc == "" {
				desc = checks.PickLocalized(db.Description, "en")
			}
			picURL = db.NexusPictureURL
		}

		items = append(items, &BatchItem{
			ID:          fmt.Sprintf("mod_%d", m.ModID),
			NexusModID:  m.ModID,
			FileID:      m.FileID,
			Name:        modName,
			Version:     m.Version,
			Selected:    !m.Optional,
			Author:      author,
			Category:    category,
			Description: desc,
			PictureURL:  picURL,
		})
	}

	// ─── ШАГ 3: ПРОВЕРКА НА AML ─────────────────────────────────────
	var amlItem *BatchItem
	for _, item := range items {
		if item.NexusModID == 246 || strings.Contains(strings.ToLower(item.Name), "auto mod loading") {
			amlItem = item
			break
		}
	}

	if amlItem != nil {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.msg("aml_detected_title"),
			app.msg("aml_detected_collections"),
			app.msg("btn_yes"), app.msg("btn_no"),
		)
		if choice == 0 {
			amlItem.Selected = false
		}
	}

	// ─── ШАГ 4: ДИАЛОГ ВЫБОРА ПРОФИЛЯ ───────────────────────────────
	title := app.msg("install_collection")
	msg := fmt.Sprintf(app.msg("install_collection_desc"), col.Name, col.Author, len(items))

	fyne.Do(func() {
		app.showChoiceDialog(
			app.mainWindow,
			title,
			msg,
			func(choice int) {
				switch choice {
				case 0:
					go app.startCollectionInstall(col, items, true)
				case 1:
					go app.startCollectionInstall(col, items, false)
				}
			},
			app.msg("install_collection_create_profile"),
			app.msg("install_collection_current_profile"),
			app.msg("btn_cancel"),
		)
	})
}

func (app *App) startCollectionInstall(col *NexusCollection, items []*BatchItem, createProfile bool) {
	if createProfile {
		name := sanitizeProfileName(col.Name)
		if app.profileExists(name) {
			name = fmt.Sprintf("%s_%d", name, time.Now().Unix())
		}

		app.appendLogToFile(fmt.Sprintf("Collection: creating profile %q", name))

		if err := app.createProfile(name, ""); err != nil {
			app.appendLogToFile(fmt.Sprintf("Collection: failed to create profile: %v", err))
			fyne.Do(func() {
				app.showInfoDialog("Error", fmt.Sprintf("Failed to create profile: %v", err))
			})
			return
		}

		done := make(chan struct{})
		fyne.Do(func() {
			app.switchProfileAsync(name, func() { close(done) })
		})
		<-done
	}

	fyne.Do(func() {
		app.showBatchDialog(
			fmt.Sprintf("Collection «%s» (rev %d)", col.Name, col.Revision),
			items, BatchModeInstall,
		)
	})
}

func sanitizeProfileName(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	name = replacer.Replace(name)
	name = strings.TrimSpace(name)
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "Collection"
	}
	return name
}

// confirmSteamLoggedIn показывает модальный диалог и блокирует
// вызывающую горутину до тех пор, пока пользователь не подтвердит,
// что залогинился в Steam (или не отменит запуск).
//
// Нужен потому, что определить программно момент «Steam готов
// принимать игру» невозможно: steamwebhelper поднимается ещё на
// экране выбора аккаунта, а до логина SteamAPI не работает.
// Вызывать ТОЛЬКО из фоновой горутины.
func (app *App) confirmSteamLoggedIn() bool {
	resultChan := make(chan bool, 1)

	fyne.Do(func() {
		app.bringToFront()

		var popUp *widget.PopUp

		okBtn := NewCustomIconButton(app.msg("btn_continue"), app.loadIconResource("ok", "assets/buttons/checked_box.png"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- true
		})
		cancelBtn := NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- false
		})

		msg := widget.NewLabel(app.msg("steam_login_prompt"))
		msg.Wrapping = fyne.TextWrapWord

		content := container.NewVBox(
			helpers.BoldCenterLabel(app.msg("steam_not_running_title")),
			widget.NewSeparator(),
			msg,
			widget.NewSeparator(),
			container.NewCenter(container.NewHBox(okBtn, cancelBtn)),
		)

		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
		popUp.Resize(fyne.NewSize(450, 220))
		popUp.Show()
	})

	return <-resultChan
}

// showLanguagePickerIfFirstRun показывает выбор языка при самом первом
// запуске. Вызывается ПЕРЕД initializePaths/loadDataAfterInit, чтобы
// все последующие диалоги (пути, wizard, AML-предупреждение) были уже
// на выбранном языке.
//
// Если InitialSetupDone == true — no-op: язык уже выбран ранее.
//
// Блокирует вызывающую горутину до выбора пользователя. Вызывать
// ТОЛЬКО из фоновой горутины (внутри fyne.DoAndWait + ожидание канала).
func (app *App) showLanguagePickerIfFirstRun() {
	app.cfgMutex.RLock()
	done := app.cfg.InitialSetupDone
	current := app.cfg.Language
	app.cfgMutex.RUnlock()

	// Показываем пикер, если:
	//   - setup ещё не проходили (свежий install), ИЛИ
	//   - cfg.Language пустой — это означает, что язык так и не выбрали
	//     (например, старый config.json без поля language, или запись
	//     повреждена). Тогда fallback на английский даёт неверный UI,
	//     хотя пользователь мог быть русскоязычным.
	if done && current != "" {
		return
	}

	// Названия языков — эндонимы (на самом языке), не переводятся.
	// Первый — English, чтобы англоязычный пользователь мог сразу ОК.
	languages := []struct {
		code  string
		label string
	}{
		{"en", app.msg("menu_lang_en")},
		{"ru", app.msg("menu_lang_ru")},
		{"de", app.msg("menu_lang_de")},
		{"es", app.msg("menu_lang_es")},
		{"fr", app.msg("menu_lang_fr")},
		{"it", app.msg("menu_lang_it")},
		{"ja", app.msg("menu_lang_ja")},
		{"ko", app.msg("menu_lang_ko")},
		{"pl", app.msg("menu_lang_pl")},
		{"pt-br", app.msg("menu_lang_pt-BR")},
		{"zh-hans", app.msg("menu_lang_zh-hans")},
		{"zh-hant", app.msg("menu_lang_zh-hant")},
	}

	// Индекс текущего языка в списке. На первом запуске cfg.Language
	// ещё "en" (defaultConfig), реального выбора пользователя не было.
	// Поэтому пытаемся предвыбрать системную локаль: русский пользователь сразу увидит «Русский» и нажмёт только OK.
	//
	// Если система не поддерживается (fi-FI, tr-TR, …) — оставляем
	// текущий (en) или ранее сохранённый выбор.
	curIdx := 0
	for i, l := range languages {
		if l.code == current {
			curIdx = i
			break
		}
	}
	if sysCode := mapSystemLocaleToSupported(systemLocale()); sysCode != "" {
		for i, l := range languages {
			if l.code == sysCode {
				curIdx = i
				break
			}
		}
	}

	labels := make([]string, len(languages))
	for i, l := range languages {
		labels[i] = l.label
	}

	chosen := make(chan string, 1)

	fyne.DoAndWait(func() {
		app.bringToFront()

		sel := widget.NewSelect(labels, nil)
		sel.SetSelectedIndex(curIdx)

		var popUp *widget.PopUp
		okBtn := widget.NewButton(app.msg("btn_ok"), func() {
			idx := sel.SelectedIndex()
			if idx < 0 || idx >= len(languages) {
				idx = curIdx
			}
			if popUp != nil {
				popUp.Hide()
			}
			chosen <- languages[idx].code
		})

		content := container.NewVBox(
			helpers.BoldCenterLabel(app.msg("menu_language")),
			widget.NewSeparator(),
			widget.NewLabel(app.msg("choose_lang")),
			sel,
			widget.NewSeparator(),
			container.NewCenter(okBtn),
		)

		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
		popUp.Resize(fyne.NewSize(420, 240))
		popUp.Show()
	})

	code := <-chosen

	if err := app.loadLanguage(code); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to load language %q: %v", code, err))
	}
	app.cfgMutex.Lock()
	app.cfg.Language = code
	app.cfgMutex.Unlock()
	app.saveConfigSafe()

	// Обновляем заголовок окна и меню на новый язык. Само окно уже
	// построено — просто перекрашиваем тексты.
	fyne.Do(func() {
		app.mainWindow.SetTitle(app.getTitle() + " v" + AppVersion)
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	app.appendLogToFile(fmt.Sprintf("First-run language selected: %s", code))
}

// showProblemsDialog открывает батч-диалог для проблемных модов.
// Вызывается из runAllChecksInternal.
func (app *App) showProblemsDialog(items []*BatchItem) {
	if len(items) == 0 {
		return
	}
	fyne.Do(func() {
		app.showBatchDialog(
			app.msg("problems_title"), // «Найдены проблемы»
			items,
			BatchModeDelete,
		)
	})
}

// ─────────────────────────────────────────────────────────────────────
// Восстановление из бэкапа
// ─────────────────────────────────────────────────────────────────────

func (app *App) showRestoreSettingsDialog() {
	settingsPath := app.getUserSettingsPath()
	configDir := filepath.Dir(configFilePath())
	backupDir := filepath.Join(configDir, "backups", "user_settings")

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_no_backups"))
		return
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_read_backup_dir_failed"), err))
		return
	}
	var backupFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".config") {
			backupFiles = append(backupFiles, e.Name())
		}
	}
	if len(backupFiles) == 0 {
		app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_no_backup_files"))
		return
	}
	sort.Slice(backupFiles, func(i, j int) bool {
		return backupFiles[i] > backupFiles[j]
	})

	selected := ""
	list := widget.NewList(
		func() int { return len(backupFiles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(backupFiles[id])
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		selected = backupFiles[id]
	}

	// widget.List в VBox схлопывается до одной строки — VBox выдаёт
	// каждому ребёнку ровно его MinSize(). Спайсер поднимает MinSize
	// Stack-контейнера до нужной высоты, а сам List внутри Stack
	// растягивается на всю эту высоту.
	// Одна строка ~37-40px, 6 строк ≈ 240px.
	listSpacer := canvas.NewRectangle(color.Transparent)
	listSpacer.SetMinSize(fyne.NewSize(0, 240))
	listBox := container.NewStack(listSpacer, list)

	var popUp *widget.PopUp
	content := container.NewVBox(
		widget.NewLabel(app.msg("usettings_select_backup")),
		listBox,
		container.NewHBox(
			widget.NewButton(app.msg("usettings_restore_btn"), func() {
				if selected == "" {
					app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_file"))
					return
				}
				popUp.Hide()
				dialog.ShowConfirm(app.msg("usettings_confirm_title"), fmt.Sprintf(app.msg("usettings_confirm_restore"), selected), func(ok bool) {
					if !ok {
						return
					}
					app.createSettingsBackup()
					src := filepath.Join(backupDir, selected)
					data, err := os.ReadFile(src)
					if err != nil {
						app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_read_backup_failed"), err))
						return
					}
					if err := os.WriteFile(settingsPath, data, 0644); err != nil {
						app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_restore_failed"), err))
						return
					}
					app.appendLog(fmt.Sprintf(app.msg("usettings_restored"), selected))
				}, app.mainWindow)
			}),
			widget.NewButton(app.msg("btn_cancel"), func() { popUp.Hide() }),
		),
	)
	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(WindowWidth400, 300))
	popUp.Show()
}
