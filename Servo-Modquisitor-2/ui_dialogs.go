// Servo-Modquisitor-2/ui_dialogs.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
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
	// fyne.Do(func() {
	dialog.ShowInformation(title, message, app.mainWindow)
	// })
}

// showChoiceDialog показывает диалог выбора с произвольным количеством кнопок.
// Результат возвращается через callback. Диалог автоматически закрывается при нажатии любой кнопки.
func (app *App) showChoiceDialog(parent fyne.Window, title, message string, callback func(int), options ...string) {
	// Все операции с UI - только в главном потоке
	fyne.Do(func() {
		// Объявляем popUp ПЕРЕД созданием кнопок
		var popUp *widget.PopUp

		var btnObjects []fyne.CanvasObject
		for i, opt := range options {
			idx := i
			btn := widget.NewButton(opt, func() {
				if popUp != nil {
					popUp.Hide()
				}
				if callback != nil {
					callback(idx)
				}
			})
			btnObjects = append(btnObjects, btn)
		}

		titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord

		// Центрируем кнопки
		btnContainer := container.NewCenter(container.NewHBox(btnObjects...))

		content := container.NewVBox(
			titleLabel,
			widget.NewSeparator(),
			msgLabel,
			widget.NewSeparator(),
			btnContainer,
		)

		popUp = widget.NewModalPopUp(content, parent.Canvas())
		popUp.Resize(fyne.NewSize(DialogMinWidth500, DialogMinHeight200))
		popUp.Show()
	})
}

// showConfirmDialog показывает диалог подтверждения с двумя кнопками (Да/Нет).
func (app *App) showConfirmDialog(title, message string, onConfirm func()) {
	fyne.Do(func() {
		dialog.ShowConfirm(title, message, func(ok bool) {
			if ok && onConfirm != nil {
				onConfirm()
			}
		}, app.mainWindow)
	})
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

// Основной диалог скачивания (для обычных модов)
func (app *App) showDownloadDialog(downloadURL, filename string, modName string, fileInfo *FileInfo, modID string) {

	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), modName, displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startDownload(downloadURL, filename, modName, fileInfo, modID)
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

// startDownload - выполняет скачивание и установку после подтверждения.
// Длительная работа вынесена в горутину, UI-поток только для dlg.Hide()
// и прокрутки к установленному моду. Иначе вложенный fyne.Do даёт deadlock.
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

	bar := widget.NewProgressBar()
	bar.SetValue(0)
	lbl := widget.NewLabel(fmt.Sprintf(app.msg("downloading"), filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.msg("download_title"), app.msg("btn_cancel"), content, app.mainWindow)

	ctx, cancel := context.WithCancel(context.Background())
	dlg.SetOnClosed(func() {
		cancel()
	})
	dlg.Show()

	// Забираем ModsPath заранее, чтобы не трогать cfg из горутины
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
		fyne.Do(func() { dlg.Hide() })
		return
	}
	dest := filepath.Join(modsPath, safeFilename)

	// Вся тяжёлая работа — в фоне. UI-поток свободен.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				app.appendLogToFile(fmt.Sprintf("PANIC in startDownload goroutine: %v", r))
				app.appendLogToFile(fmt.Sprintf("Stack: %s", debug.Stack()))
			}
		}()

		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: starting download to %s", dest))

		err := app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		fyne.Do(func() { dlg.Hide() })

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
		installedName, installedVersion, err := app.InstallModFromArchive(dest, false, knownVersion, modName)
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

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Loader", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Loader", fileInfo, "19:base", app.installDMLFromArchive, app.msg("installing_dml"), app.msg("dml_updated"))
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

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Framework", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Framework", fileInfo, "8:dmf", app.installDMLFromArchive, app.msg("installing_dmf"), app.msg("log_dmf_updated_succ"))
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

	app.showChoiceDialog(
		app.mainWindow,
		app.msg("confirm_download_title"),
		fmt.Sprintf(app.msg("confirm_download_text"), "Darktide Mod Autopatcher", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Autopatcher", fileInfo, "709:autopatch", app.installAutopatcherFromArchive, app.msg("installing_autopatcher"), app.msg("autopatcher_updated"))
		},
		app.msg("btn_yes"),
		app.msg("btn_no"),
	)
}

// startSystemDownload - общая логика скачивания для системных модов.
func (app *App) startSystemDownload(downloadURL, filename, displayName string, fileInfo *FileInfo, cacheKey string, installFunc func(string) error, logInstalling, logSuccess string) {
	app.appendLog(fmt.Sprintf(app.msg("log_downloading_mod"), displayName))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.msg("downloading"), filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.msg("download_title"), app.msg("btn_cancel"), content, app.mainWindow)

	ctx, cancel := context.WithCancel(context.Background())
	dlg.SetOnClosed(func() {
		cancel()
	})
	dlg.Show()

	// Копируем ModsPath под мьютексом перед запуском горутины
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	go func() {
		safeFilename, err := sanitizeFilename(filename)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("Invalid filename: %v", err))
			return
		}
		dest := filepath.Join(modsPath, safeFilename)

		err = app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
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
				app.appendLog(fmt.Sprintf(app.msg("log_error_file_too_small"), info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(logInstalling)
			if err := installFunc(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.msg("log_install_failed"), err))
			} else {
				if fileInfo != nil {
					app.setCachedVersion(cacheKey, ModVersionInfo{
						Timestamp:   fileInfo.UploadedTimestamp,
						Version:     fileInfo.Version,
						Folder:      displayName,
						Source:      "nexus",
						InstalledAt: time.Now().Unix(),
					})
					app.saveNexusVersionCache()
				}
				app.appendLog(logSuccess)
			}
			os.Remove(dest)
		})
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
			fyne.Do(func() {
				app.showDMLDownloadDialog(directURL, filename, fileInfo)
			})
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
			fyne.Do(func() {
				app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
			})
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
		fyne.Do(func() {
			app.showDownloadDialog(directURL, filename, modName, fileInfo, modID)
		})
	}()
}

// Остальные функции (showEditVersionDialog)

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
			widget.NewButton(app.msg("btn_save"), func() {
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
			widget.NewButton(app.msg("btn_cancel"), func() {
				popUp.Hide()
			}),
		),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(400, 200))
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
	var closeDialogFunc func()

	fyne.Do(func() {
		dlg = dialog.NewCustom(title, app.msg("btn_cancel"), content, app.mainWindow)
		dlg.Resize(fyne.NewSize(400, 120))

		dlg.SetOnClosed(func() {
			once.Do(func() {
				close(cancelChan)
			})
		})

		dlg.Show()
	})

	closeDialogFunc = func() {
		if dlg != nil {
			fyne.Do(func() {
				dlg.Hide()
			})
		}
	}

	return bar, label, cancelChan, closeDialogFunc
}

// showUpdateChoiceDialog показывает диалог со списком обновлений и чекбоксами.
// Результат отправляется в канал resultChan.
func (app *App) showUpdateChoiceDialog(updates []*ModUpdateChoice, resultChan chan<- struct {
	indices []int
	ok      bool
}) {
	if len(updates) == 0 {
		resultChan <- struct {
			indices []int
			ok      bool
		}{nil, false}
		return
	}
	if app.mainWindow == nil || app.mainWindow.Canvas() == nil {
		resultChan <- struct {
			indices []int
			ok      bool
		}{nil, false}
		return
	}

	var popUp *widget.PopUp
	var items []fyne.CanvasObject

	for _, update := range updates {
		// Чекбокс выбора мода
		check := widget.NewCheck("", func(checked bool) {
			update.Selected = checked
		})
		check.SetChecked(update.Selected)

		// Название мода и версии
		displayName := update.Mod.DisplayName
		if displayName == "" {
			displayName = update.Mod.Name
		}
		cacheKey := fmt.Sprintf("%d:%s", helpers.ExtractModIDFromURL(update.Mod.URL), update.Mod.Name)
		currentVersion := "?"
		if info, ok := app.getCachedVersion(cacheKey); ok {
			currentVersion = info.Version
		}
		titleText := fmt.Sprintf("%s  %s → %s", displayName, currentVersion, update.FileInfo.Version)
		titleLabel := widget.NewLabel(titleText)
		titleLabel.TextStyle = fyne.TextStyle{Bold: true}

		// Список изменений (изначально скрыт)
		changelogText := update.Changelog
		if changelogText == "" {
			changelogText = app.msg("changelog_unavailable")
		} else {
			changelogText = stripHTML(changelogText)
		}
		changelogLabel := widget.NewLabel(changelogText)
		changelogLabel.Wrapping = fyne.TextWrapWord

		changelogContainer := container.NewVBox(changelogLabel)
		changelogContainer.Hide()

		// Кнопка-спойлер с явным состоянием
		btnState := struct {
			expanded bool
			btn      *widget.Button
		}{}
		btnState.btn = widget.NewButton(app.msg("btn_show_changelog"), func() {
			btnState.expanded = !btnState.expanded
			if btnState.expanded {
				changelogContainer.Show()
				btnState.btn.SetText(app.msg("btn_hide_changelog"))
			} else {
				changelogContainer.Hide()
				btnState.btn.SetText(app.msg("btn_show_changelog"))
			}
		})

		// Строка: чекбокс + название + кнопка
		row := container.NewHBox(check, titleLabel, btnState.btn)

		// Весь элемент (строка + список изменений + разделитель)
		item := container.NewVBox(row, changelogContainer, widget.NewSeparator())
		items = append(items, item)
	}

	listContainer := container.NewVBox(items...)
	scroll := container.NewVScroll(listContainer)
	scroll.SetMinSize(fyne.NewSize(600, 400))

	titleLabel := widget.NewLabelWithStyle(
		fmt.Sprintf(app.msg("update_available_x"), len(updates)),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	confirmBtn := widget.NewButton(app.msg("btn_update_mods"), func() {
		if popUp != nil {
			popUp.Hide()
		}
		var idxs []int
		for i, u := range updates {
			if u.Selected {
				idxs = append(idxs, i)
			}
		}
		resultChan <- struct {
			indices []int
			ok      bool
		}{idxs, true}
	})
	cancelBtn := widget.NewButton(app.msg("btn_cancel"), func() {
		if popUp != nil {
			popUp.Hide()
		}
		resultChan <- struct {
			indices []int
			ok      bool
		}{nil, false}
	})

	btnContainer := container.NewHBox(
		container.NewCenter(confirmBtn),
		container.NewCenter(cancelBtn),
	)

	content := container.NewBorder(
		container.NewVBox(titleLabel, widget.NewSeparator()),
		btnContainer,
		nil, nil,
		scroll,
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(700, 500))
	popUp.Show()
}

// stripHTML удаляет HTML-теги и преобразует <br> и <li> в переносы строк.
func stripHTML(html string) string {
	replacer := strings.NewReplacer(
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
		"</li>", "\n",
		"<li>", "- ",
	)
	text := replacer.Replace(html)
	re := regexp.MustCompile(`<[^>]*>`)
	text = re.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// showCreateProfileDialog показывает диалог создания профиля
func (app *App) showCreateProfileDialog() {
	entry := widget.NewEntry()
	entry.SetPlaceHolder(app.msg("profile_name_placeholder"))

	// Чекбокс "Копировать из текущего"
	copyCheck := widget.NewCheck(app.msg("profile_copy_from_current"), nil)
	copyCheck.SetChecked(true)

	var popUp *widget.PopUp

	// Кнопки с центрированием
	btnCreate := widget.NewButton(app.msg("btn_create"), func() {
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
		if err := app.createProfile(name, copyFrom); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create profile: %v", err))
			app.showInfoDialog(app.msg("error_title"), app.msg("profile_create_failed"))
			return
		}
		app.switchProfile(name)
		popUp.Hide()
	})

	btnCancel := widget.NewButton(app.msg("btn_cancel"), func() {
		popUp.Hide()
	})

	btnContainer := container.NewCenter(
		container.NewHBox(btnCreate, btnCancel),
	)

	content := container.NewVBox(
		widget.NewLabelWithStyle(app.msg("profile_create_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(app.msg("profile_create_message")),
		entry,
		copyCheck,
		widget.NewSeparator(),
		btnContainer,
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(400, 250))
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

	btnRename := widget.NewButton(app.msg("btn_rename"), func() {
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

	btnCancel := widget.NewButton(app.msg("btn_cancel"), func() {
		popUp.Hide()
	})

	btnContainer := container.NewCenter(container.NewHBox(btnRename, btnCancel))

	content := container.NewVBox(
		widget.NewLabelWithStyle(app.msg("profile_rename_title"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(fmt.Sprintf(app.msg("profile_rename_message"), currentProfile)),
		entry,
		widget.NewSeparator(),
		btnContainer,
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(400, 200))
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
			// Удаление запускаем ТОЛЬКО после того, как переключение
			// профиля действительно завершилось. Иначе deleteProfile
			// видит cfg.ActiveProfile = nameToDelete и отклоняет удаление
			// («cannot delete active profile»), а параллельно с этим
			// switchProfile ещё копирует моды — файловая гонка.
			app.switchProfileAsync(target, func() {
				if err := app.deleteProfile(nameToDelete); err != nil {
					app.appendLogToFile(fmt.Sprintf("Failed to delete profile: %v", err))
					app.showInfoDialog(app.msg("error_title"), app.msg("profile_delete_failed"))
					return
				}
				app.refreshProfileList()
				app.appendLog(fmt.Sprintf(app.msg("profile_deleted"), nameToDelete))
			})
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

		if err := app.createProfile(baseName, ""); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create profile: %v", err))
			return
		}

		dstPath := app.profilePath(baseName)
		if err := copyPath(profileFolder, dstPath); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to import profile: %v", err))
			return
		}
		cleanupModsFolder(filepath.Join(dstPath, "mods"))
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

		app.cfgMutex.RLock()
		profileName := app.cfg.ActiveProfile
		app.cfgMutex.RUnlock()

		zipPath := filepath.Join(dstDir, profileName+".zip")

		// Создаём временную папку для подготовки архива
		tmpDir, err := os.MkdirTemp("", "profile-export-")
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create temp directory: %v", err))
			return
		}
		defer os.RemoveAll(tmpDir)

		// Копируем содержимое профиля во временную папку
		srcPath := app.activeProfilePath()
		tmpProfileDir := filepath.Join(tmpDir, profileName)
		if err := copyPath(srcPath, tmpProfileDir); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy profile to temp: %v", err))
			return
		}

		// Архивируем временную папку
		if err := app.createZipArchive(tmpProfileDir, zipPath); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create zip archive: %v", err))
			return
		}

		app.appendLog(fmt.Sprintf(app.msg("profile_exported"), zipPath))
	}, app.mainWindow)
	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}

// fileInfoSnapshot безопасно извлекает поля из возможно-nil *FileInfo.
// Возвращает (version, uploadedTimestamp, fileName). Для nil-аргумента
// все три значения — zero-value, что позволяет вызывающей стороне
// не думать про nil-проверки на каждом шаге.
func fileInfoSnapshot(fi *FileInfo) (version string, timestamp int64, fileName string) {
	if fi == nil {
		return "", 0, ""
	}
	return fi.Version, fi.UploadedTimestamp, fi.FileName
}
