// ui_dialogs.go
package main

import (
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type dialogButton struct {
	Text     string
	Callback func()
}

func (app *App) newModalDialog(dType DialogType, title, message string, buttons []dialogButton) {
	fyne.Do(func() {
		var gradImg *canvas.Image
		switch dType {
		case DialogTypeInfo:
			gradImg = canvas.NewImageFromImage(app.makeCRTGradient(DialogGradientWidth, DialogGradientHeight))
		default:
			gradImg = canvas.NewImageFromImage(app.makeRedCRTGradient(DialogGradientWidth, DialogGradientHeight))
		}
		gradImg.FillMode = canvas.ImageFillStretch

		titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		headerContainer := container.NewStack(gradImg, container.NewCenter(titleLabel))

		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord
		msgScroll := container.NewScroll(msgLabel)
		msgScroll.SetMinSize(fyne.NewSize(DialogMinWidth-40, DialogMinHeight-80))

		var popUp *widget.PopUp
		var btnObjects []fyne.CanvasObject
		for _, b := range buttons {
			btn := NewCustomButton(b.Text, func() {
				popUp.Hide()
				if b.Callback != nil {
					b.Callback()
				}
			})
			spacer := canvas.NewRectangle(color.Transparent)
			spacer.SetMinSize(fyne.NewSize(DialogButtonMinWidth, DialogButtonHeight))
			btnObjects = append(btnObjects, container.NewStack(spacer, btn))
		}

		content := container.NewVBox(
			headerContainer,
			msgScroll,
			container.NewCenter(container.NewHBox(btnObjects...)),
		)
		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
		popUp.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
		popUp.Show()
	})
}

func (app *App) showInfoDialog(title, message string) {
	app.newModalDialog(DialogTypeInfo, title, message, []dialogButton{
		{Text: app.messages["btn_ok"], Callback: nil},
	})
}

func (app *App) showChoiceDialog(parent fyne.Window, title, message string, options ...string) int {
	resultChan := make(chan int, 1)
	buttons := make([]dialogButton, len(options))
	for i, opt := range options {
		idx := i
		buttons[i] = dialogButton{
			Text:     opt,
			Callback: func() { resultChan <- idx },
		}
	}
	app.newModalDialog(DialogTypeWarn, title, message, buttons)
	return <-resultChan
}

func (app *App) showConfirmDialog(title, message, confirmKey, cancelKey string, callback func(bool)) {
	app.newModalDialog(DialogTypeWarn, title, message, []dialogButton{
		{Text: app.messages[confirmKey], Callback: func() { callback(true) }},
		{Text: app.messages[cancelKey], Callback: func() { callback(false) }},
	})
}

func (app *App) showChoiceDialogAsync(parent fyne.Window, title, message string, callback func(int), options ...string) {
	fyne.Do(func() {
		var buttons []*CustomButton
		popUp := widget.NewModalPopUp(nil, parent.Canvas())
		var btnObjects []fyne.CanvasObject
		for i, opt := range options {
			idx := i
			btn := NewCustomButton(opt, func() {
				popUp.Hide()
				if callback != nil {
					callback(idx)
				}
			})
			buttons = append(buttons, btn)
			btnObjects = append(btnObjects, btn)
		}

		gradHeader := canvas.NewImageFromImage(app.makeRedCRTGradient(DialogGradientWidth, DialogGradientHeight))
		gradHeader.FillMode = canvas.ImageFillStretch
		titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		headerContainer := container.NewStack(gradHeader, container.NewCenter(titleLabel))

		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord
		msgScroll := container.NewScroll(msgLabel)
		msgScroll.SetMinSize(fyne.NewSize(666, 250))
		content := container.NewVBox(
			headerContainer,
			msgScroll,
			container.NewCenter(container.NewHBox(btnObjects...)),
		)
		popUp.Content = content
		popUp.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
		popUp.Show()
	})
}

func (app *App) requestNexusAPIKey() {
	if app.cfg.NexusAPIKey != "" {
		return
	}
	fyne.Do(func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder(app.messages["nexus_api_key_placeholder"])
		var dlg dialog.Dialog
		content := container.NewVBox(
			widget.NewLabel(app.messages["nexus_api_key_label"]),
			entry,
			widget.NewButton(app.messages["btn_save"], func() {
				app.cfg.NexusAPIKey = entry.Text
				saveConfig(app.cfg)
				app.appendLog(app.messages["nexus_api_key_saved"])
				dlg.Hide()
			}),
		)
		dlg = dialog.NewCustom(app.messages["nexus_api_key_title"], app.messages["btn_cancel"], content, app.mainWindow)
		dlg.Show()
	})
}

func (app *App) applyTooltip(btn *CustomButton, tipKey string) {
	tip := ""
	if tipKey != "" {
		tip = app.messages[tipKey]
	}
	btn.OnMouseIn = func() {
		if tip != "" {
			app.tooltipStatus.Show(tip)
		}
	}
	btn.OnMouseMoved = func(*desktop.MouseEvent) {
		app.tooltipStatus.HideAfterDelay()
	}
	btn.OnMouseOut = func() {
		app.tooltipStatus.HideAfterDelay()
	}
}

func (app *App) showNexusAPIKeyDialog() {
	fyne.Do(func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder(app.messages["nexus_api_key_placeholder"])
		entry.SetText(app.cfg.NexusAPIKey)

		clearBtn := NewCustomButton("✕", func() {
			entry.SetText("")
		})
		clearBtn.Importance = widget.DangerImportance
		clearBtn.Hide()
		entry.OnChanged = func(s string) {
			if s != "" {
				clearBtn.Show()
			} else {
				clearBtn.Hide()
			}
		}
		entryBox := container.NewBorder(nil, nil, nil, clearBtn, entry)

		var dlg dialog.Dialog
		var deleteBtn *CustomButton
		if app.cfg.NexusAPIKey != "" {
			deleteBtn = NewCustomButton(app.messages["btn_delete_api_key"], func() {
				app.cfg.NexusAPIKey = ""
				saveConfig(app.cfg)
				app.appendLog(app.messages["nexus_api_key_deleted"])
				dlg.Hide()
			})
		}

		var btns []fyne.CanvasObject
		saveBtn := widget.NewButton(app.messages["btn_save_api"], func() {
			app.cfg.NexusAPIKey = entry.Text
			saveConfig(app.cfg)
			app.appendLog(app.messages["nexus_api_key_saved"])
			dlg.Hide()
		})
		btns = append(btns, saveBtn)
		if deleteBtn != nil {
			btns = append(btns, deleteBtn)
		}

		content := container.NewVBox(
			widget.NewLabel(app.messages["nexus_api_key_label"]),
			entryBox,
			container.NewHBox(btns...),
		)
		dlg = dialog.NewCustom(app.messages["nexus_api_key_title"], app.messages["btn_cancel"], content, app.mainWindow)
		dlg.Show()
	})
}

func (app *App) showDownloadDialog(url, filename, modName string, fileInfo *FileInfo, modID string) {
	if modName != "" {
		app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], modName))
	}
	bar := widget.NewProgressBar()
	bar.SetValue(0)
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)
	dlg.Show()
	go func() {
		dest := filepath.Join(app.cfg.ModsPath, filename)
		ext := strings.ToLower(filepath.Ext(dest))
		knownExts := map[string]bool{".zip": true, ".rar": true, ".7z": true}
		if !knownExts[ext] {
			newDest := dest + ".zip"
			if err := os.Rename(dest, newDest); err == nil {
				dest = newDest
			}
		}
		err := app.DownloadFileWithProgress(url, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				return
			}
			info, e := os.Stat(dest)
			if e != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_not_found"], e))
				return
			}
			app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_size"], float64(info.Size())/1024/1024))

			// Проверка на битый архив (размер менее 100 байт)
			if info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], filename, info.Size()))
				os.Remove(dest)
				return
			}

			app.appendLog(app.messages["download_complete"])
			installedName, installedVersion, err := app.InstallModFromArchive(dest, false, fileInfo.Version)
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_install_failed"], err))
			} else {
				os.Remove(dest)
				if modID != "" {
					// Сохраняем версию в кэш (используем installedVersion, которую мог ввести пользователь)
					app.cacheModVersion(modID, installedName, installedVersion, fileInfo.UploadedTimestamp)
				}
				if installedName != "" {
					app.selectModByName(installedName)
				}
			}
		})
	}()
}

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

	modIDStr := fmt.Sprintf("%d", dmlModID)
	var saved ModVersionInfo
	if info, exists := app.nexusVersionCache[modIDStr]; exists {
		saved = info
	}
	if saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DML", fileInfo.Version))
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	fyne.Do(func() {
		app.showDMLDownloadDialog(directURL, filename, fileInfo)
	})
}

func (app *App) showDMLDownloadDialog(url, filename string, fileInfo *FileInfo) {
	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], "Darktide Mod Loader"))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading_dml"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)
	dlg.Show()
	go func() {
		dest := filepath.Join(app.cfg.ModsPath, filename)
		err := app.DownloadFileWithProgress(url, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				return
			}
			info, e := os.Stat(dest)
			if e == nil && info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(app.messages["installing_dml"])
			if err := app.installDMLFromArchive(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["dml_install_failed"], err))
			} else {
				if fileInfo != nil {
					app.nexusVersionCache["19"] = ModVersionInfo{
						Timestamp: fileInfo.UploadedTimestamp,
						Version:   fileInfo.Version,
						Folder:    "Darktide Mod Loader",
					}
					app.saveNexusVersionCache()
				}
				app.appendLog(app.messages["dml_updated"])
			}
			os.Remove(dest)
		})
	}()
}

func (app *App) handleNXMLink(nxmURL string) {
	// Защита от двойного клика
	now := time.Now()
	if nxmURL == app.lastNxmURL && now.Sub(app.lastNxmTime) < 5*time.Second {
		app.appendLog(app.messages["nxm_already_processing"])
		return
	}
	app.lastNxmURL = nxmURL
	app.lastNxmTime = now
	u, err := url.Parse(nxmURL)
	if err != nil {
		// app.appendLog(app.messages["log_invalid_nmx_link"])
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
		// app.appendLog(app.messages["log_invalid_nmx_link"])
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
			if key != "" {
				app.appendLog(app.messages["download_free_method"])
				directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
			} else {
				app.appendLog(app.messages["download_premium_method"])
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
			if key != "" {
				app.appendLog(app.messages["download_free_method"])
				directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
			} else {
				app.appendLog(app.messages["download_premium_method"])
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
		if key != "" {
			directURL, filename, err = app.getFreeDownloadURL(modID, fileID, key, expires)
		} else {
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

func (app *App) showAutopatcherDownloadDialog(url, filename string, fileInfo *FileInfo) {
	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], "Darktide Mod Autopatcher"))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)
	dlg.Show()
	go func() {
		dest := filepath.Join(app.cfg.ModsPath, filename)
		err := app.DownloadFileWithProgress(url, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				return
			}
			info, e := os.Stat(dest)
			if e == nil && info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(app.messages["installing_autopatcher"])
			if err := app.installAutopatcherFromArchive(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["dml_install_failed"], err))
			} else {
				if fileInfo != nil {
					app.nexusVersionCache["709"] = ModVersionInfo{
						Timestamp: fileInfo.UploadedTimestamp,
						Version:   fileInfo.Version,
						Folder:    "Darktide Autopatch",
					}
					app.saveNexusVersionCache()
				}
				app.appendLog(app.messages["autopatcher_updated"])
			}
			os.Remove(dest)
		})
	}()
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

	modIDStr := fmt.Sprintf("%d", dmfModID)
	var saved ModVersionInfo
	if info, exists := app.nexusVersionCache[modIDStr]; exists {
		saved = info
	}
	if saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DMF", fileInfo.Version))
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	fyne.Do(func() {
		app.showDMFDownloadDialog(directURL, filename, fileInfo)
	})
}

func (app *App) showDMFDownloadDialog(url, filename string, fileInfo *FileInfo) {
	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], "Darktide Mod Framework"))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading_dml"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)
	dlg.Show()
	go func() {
		dest := filepath.Join(app.cfg.ModsPath, filename)
		err := app.DownloadFileWithProgress(url, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				return
			}
			info, e := os.Stat(dest)
			if e == nil && info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(app.messages["installing_dml"]) // текст "Installing Darktide Mod Loader..." - можно оставить или заменить
			if err := app.installDMLFromArchive(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["dml_install_failed"], err))
			} else {
				if fileInfo != nil {
					app.nexusVersionCache["8"] = ModVersionInfo{
						Timestamp: fileInfo.UploadedTimestamp,
						Version:   fileInfo.Version,
						Folder:    "Darktide Mod Framework",
					}
					app.saveNexusVersionCache()
				}
				app.appendLog(app.messages["log_dmf_updated_succ"])
			}
			os.Remove(dest)
		})
	}()
}
