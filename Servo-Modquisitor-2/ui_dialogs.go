// ui_dialogs.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// showInfoDialog показывает информационный диалог с кнопкой OK.
func (app *App) showInfoDialog(title, message string) {
	dialog.ShowInformation(title, message, app.mainWindow)
}

// showChoiceDialog показывает диалог выбора с произвольным количеством кнопок.
// Результат возвращается через callback. Диалог автоматически закрывается при нажатии любой кнопки.
func (app *App) showChoiceDialog(parent fyne.Window, title, message string, callback func(int), options ...string) {

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
}

// showConfirmDialog показывает диалог подтверждения с двумя кнопками (Да/Нет).
func (app *App) showConfirmDialog(title, message string, onConfirm func()) {
	dialog.ShowConfirm(title, message, func(ok bool) {
		if ok && onConfirm != nil {
			onConfirm()
		}
	}, app.mainWindow)
}

// showChoiceDialogSync - синхронная версия для фоновых горутин.
// Блокирует вызывающую горутину до выбора пользователя.
func (app *App) showChoiceDialogSync(parent fyne.Window, title, message string, options ...string) int {
	resultChan := make(chan int, 1)
	app.showChoiceDialog(parent, title, message, func(choice int) {
		resultChan <- choice
	}, options...)
	return <-resultChan
}

// showChoiceDialogAsync - устаревшая, оставлена для совместимости.
// Используйте showChoiceDialog с callback.
func (app *App) showChoiceDialogAsync(parent fyne.Window, title, message string, callback func(int), options ...string) {
	app.showChoiceDialog(parent, title, message, callback, options...)
}

// applyTooltip - навешивает тултип на кастомную кнопку.
func (app *App) applyTooltip(btn *CustomButton, tipKey string) {
	tip := app.messages[tipKey]
	if tip == "" {
		return
	}
	btn.OnMouseIn = func() {
		app.tooltipStatus.Show(tip)
	}
	btn.OnMouseMoved = func(*desktop.MouseEvent) {
		app.tooltipStatus.HideAfterDelay()
	}
	btn.OnMouseOut = func() {
		app.tooltipStatus.HideAfterDelay()
	}
}

// --- Основной диалог скачивания (для обычных модов) ---
func (app *App) showDownloadDialog(downloadURL, filename string, modName string, fileInfo *FileInfo, modID string) {

	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.messages["confirm_download_title"],
		fmt.Sprintf(app.messages["confirm_download_text"], modName, displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startDownload(downloadURL, filename, modName, fileInfo, modID)
		},
		app.messages["btn_yes"],
		app.messages["btn_no"],
	)
}

// startDownload - выполняет скачивание и установку после подтверждения.
func (app *App) startDownload(downloadURL, filename, modName string, fileInfo *FileInfo, modID string) {
	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], modName))

	bar := widget.NewProgressBar()
	bar.SetValue(0)
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)

	ctx, cancel := context.WithCancel(context.Background())
	dlg.SetOnClosed(func() {
		cancel()
	})
	dlg.Show()

	go func() {
		saveFilename := filename
		if fileInfo != nil && fileInfo.FileName != "" {
			saveFilename = fileInfo.FileName
		}
		safeFilename, err := sanitizeFilename(saveFilename)
		if err != nil {
			app.appendLog(fmt.Sprintf("Invalid filename: %v", err))
			return
		}
		dest := filepath.Join(app.cfg.ModsPath, safeFilename)

		err = app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				if err == context.Canceled {
					app.appendLog(app.messages["download_cancelled"])
				} else {
					app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				}
				os.Remove(dest)
				return
			}
			info, e := os.Stat(dest)
			if e != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_not_found"], e))
				return
			}
			app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_size"], float64(info.Size())/1024/1024))

			if info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], safeFilename, info.Size()))
				os.Remove(dest)
				return
			}

			app.appendLog(app.messages["download_complete"])
			// Передаём modName для удаления старой папки при обновлении
			installedName, installedVersion, err := app.InstallModFromArchive(dest, false, fileInfo.Version, modName)
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_install_failed"], err))
			} else {
				os.Remove(dest)
				if modID != "" && installedName != "" {
					cacheKey := modID + ":" + installedName
					app.cacheModVersion(cacheKey, installedName, installedVersion, fileInfo.UploadedTimestamp, "nexus")
				}
				if installedName != "" {
					app.selectAndScrollToMod(installedName)
				}
				if modID != "" {
					mid, err := strconv.Atoi(modID)
					if err == nil {
						var fn string
						if fileInfo != nil {
							fn = fileInfo.FileName
						}
						go app.autoAddModToDatabase(mid, installedName, fn)
					}
				}
			}
		})
	}()
}

// --- Специальные диалоги для системных модов ---

func (app *App) showDMLDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.messages["confirm_download_title"],
		fmt.Sprintf(app.messages["confirm_download_text"], "Darktide Mod Loader", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Loader", fileInfo, "19:base", app.installDMLFromArchive, app.messages["installing_dml"], app.messages["dml_updated"])
		},
		app.messages["btn_yes"],
		app.messages["btn_no"],
	)
}

func (app *App) showDMFDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.messages["confirm_download_title"],
		fmt.Sprintf(app.messages["confirm_download_text"], "Darktide Mod Framework", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Framework", fileInfo, "8:dmf", app.installDMLFromArchive, app.messages["installing_dmf"], app.messages["log_dmf_updated_succ"])
		},
		app.messages["btn_yes"],
		app.messages["btn_no"],
	)
}

func (app *App) showAutopatcherDownloadDialog(downloadURL, filename string, fileInfo *FileInfo) {
	displayFilename := filename
	if fileInfo != nil && fileInfo.FileName != "" {
		displayFilename = fileInfo.FileName
	}

	app.showChoiceDialog(
		app.mainWindow,
		app.messages["confirm_download_title"],
		fmt.Sprintf(app.messages["confirm_download_text"], "Darktide Mod Autopatcher", displayFilename),
		func(choice int) {
			if choice != 0 {
				return
			}
			app.startSystemDownload(downloadURL, filename, "Darktide Mod Autopatcher", fileInfo, "709:autopatch", app.installAutopatcherFromArchive, app.messages["installing_autopatcher"], app.messages["autopatcher_updated"])
		},
		app.messages["btn_yes"],
		app.messages["btn_no"],
	)
}

// startSystemDownload - общая логика скачивания для системных модов.
func (app *App) startSystemDownload(downloadURL, filename, displayName string, fileInfo *FileInfo, cacheKey string, installFunc func(string) error, logInstalling, logSuccess string) {
	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], displayName))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)

	ctx, cancel := context.WithCancel(context.Background())
	dlg.SetOnClosed(func() {
		cancel()
	})
	dlg.Show()

	go func() {
		safeFilename, err := sanitizeFilename(filename)
		if err != nil {
			app.appendLog(fmt.Sprintf("Invalid filename: %v", err))
			return
		}
		dest := filepath.Join(app.cfg.ModsPath, safeFilename)

		err = app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				if err == context.Canceled {
					app.appendLog(app.messages["download_cancelled"])
				} else {
					app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				}
				return
			}
			info, e := os.Stat(dest)
			if e == nil && info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(logInstalling)
			if err := installFunc(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_install_failed"], err))
			} else {
				if fileInfo != nil {
					app.setCachedVersion(cacheKey, ModVersionInfo{
						Timestamp: fileInfo.UploadedTimestamp,
						Version:   fileInfo.Version,
						Folder:    displayName,
						Source:    "nexus",
					})
					app.saveNexusVersionCache()
				}
				app.appendLog(logSuccess)
			}
			os.Remove(dest)
		})
	}()
}

// --- Функции обновления системных модов (вызываются из кнопок) ---

func (app *App) updateDML() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	const dmlModID = 19
	app.appendLog(fmt.Sprintf(app.messages["looking_for_latest_file"], dmlModID))
	fileInfo, err := app.getLatestFileInfo(dmlModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}
	cacheKey := "19:base"
	saved, exists := app.getCachedVersion(cacheKey)
	if exists && saved.Source == "manual" {
		app.appendLog(app.messages["log_dml_installed_manual"])
		return
	}
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DML", fileInfo.Version))
		return
	}
	directURL, filename, err := app.getPremiumDownloadURL("19", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showDMLDownloadDialog(directURL, filename, fileInfo)
}

func (app *App) updateDMF() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	const dmfModID = 8
	app.appendLog(fmt.Sprintf(app.messages["looking_for_latest_file"], dmfModID))
	fileInfo, err := app.getLatestFileInfo(dmfModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}
	cacheKey := "8:dmf"
	saved, exists := app.getCachedVersion(cacheKey)
	if exists && saved.Source == "manual" {
		app.appendLog(app.messages["log_dmf_installed_manual"])
		return
	}
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DMF", fileInfo.Version))
		return
	}
	directURL, filename, err := app.getPremiumDownloadURL("8", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showDMFDownloadDialog(directURL, filename, fileInfo)
}

func (app *App) updateAutopatcher() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	const autopatchModID = 709
	app.appendLog(fmt.Sprintf(app.messages["looking_for_latest_file"], autopatchModID))
	fileInfo, err := app.getLatestFileInfo(autopatchModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}
	cacheKey := "709:autopatch"
	saved, exists := app.getCachedVersion(cacheKey)
	if exists && saved.Source == "manual" {
		app.appendLog(app.messages["log_autopatcher_manual"])
		return
	}
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "Autopatcher", fileInfo.Version))
		return
	}
	directURL, filename, err := app.getPremiumDownloadURL("709", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
}

// --- Processing nxm links ---

func (app *App) handleNXMLink(nxmURL string) {
	now := time.Now()
	if nxmURL == app.lastNxmURL && now.Sub(app.lastNxmTime) < 5*time.Second {
		app.appendLog(app.messages["nxm_already_processing"])
		return
	}
	app.lastNxmURL = nxmURL
	app.lastNxmTime = now

	u, err := url.Parse(nxmURL)
	if err != nil {
		app.appendLog(app.messages["log_invalid_nxm_link"])
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
		app.appendLog(app.messages["log_invalid_nxm_link"])
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
				app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
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
				app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
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
			app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
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

// --- Остальные функции (showEditVersionDialog) ---

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
		app.appendLog(app.messages["log_cannot_determine_cache_key"])
		return
	}

	currentVersion := ""
	if info, ok := app.getCachedVersion(cacheKey); ok {
		currentVersion = info.Version
	}

	entry := widget.NewEntry()
	entry.SetText(currentVersion)
	entry.SetPlaceHolder(app.messages["placeholder_mod_version"])

	var popUp *widget.PopUp

	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf(app.messages["edit_version_current"], mod.DisplayName, currentVersion)),
		entry,
		container.NewHBox(
			widget.NewButton(app.messages["btn_save"], func() {
				newVersion := strings.TrimSpace(entry.Text)
				if newVersion == "" {
					app.appendLog(app.messages["log_cannot_version_empty"])
					return
				}
				app.setCachedVersion(cacheKey, ModVersionInfo{
					Timestamp: time.Now().Unix(),
					Version:   newVersion,
					Folder:    mod.Name,
					Source:    "manual",
				})
				app.saveNexusVersionCache()
				app.appendLog(fmt.Sprintf(app.messages["log_version_for_updated_to"], mod.DisplayName, newVersion))
				popUp.Hide()
				app.updateDescriptionForMod(mod.Name)
			}),
			widget.NewButton(app.messages["btn_cancel"], func() {
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

	// ВСЕ ОПЕРАЦИИ С UI В ГЛАВНОМ ПОТОКЕ
	var dlg *dialog.CustomDialog
	var cancelChan chan struct{}
	var once sync.Once
	var closeDialogFunc func()

	fyne.Do(func() {
		dlg = dialog.NewCustom(title, app.messages["btn_cancel"], content, app.mainWindow)
		dlg.Resize(fyne.NewSize(400, 120))

		cancelChan = make(chan struct{})
		dlg.SetOnClosed(func() {
			once.Do(func() {
				close(cancelChan)
			})
		})

		closeDialogFunc = func() {
			dlg.Hide()
		}

		dlg.Show()
	})

	return bar, label, cancelChan, closeDialogFunc
}
