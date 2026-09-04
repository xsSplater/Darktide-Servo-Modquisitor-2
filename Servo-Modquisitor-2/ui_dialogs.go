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
	"regexp"
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

// Основной диалог скачивания (для обычных модов)
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
					app.cacheModVersion(cacheKey, installedName, installedVersion, fileInfo.UploadedTimestamp, "nexus", 0)
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

// Специальные диалоги для системных модов

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
	// Проверяем только актуальность по дате, игнорируем source
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DML", fileInfo.Version))
		return
	}
	// Если был ручной, сообщим об этом (опционально)
	if exists && saved.Source == "manual" {
		app.appendLog("DML was installed manually, but you chose to update - overwriting with Nexus version.")
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
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DMF", fileInfo.Version))
		return
	}
	if exists && saved.Source == "manual" {
		app.appendLog("DMF was installed manually, but you chose to update - overwriting with Nexus version.")
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
	if exists && saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "Autopatcher", fileInfo.Version))
		return
	}
	if exists && saved.Source == "manual" {
		app.appendLog("Autopatcher was installed manually, but you chose to update - overwriting with Nexus version.")
	}
	directURL, filename, err := app.getPremiumDownloadURL("709", fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
}

// Processing nxm links

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

	var dlg *dialog.CustomDialog
	cancelChan := make(chan struct{})
	var once sync.Once
	var closeDialogFunc func()

	fyne.Do(func() {
		dlg = dialog.NewCustom(title, app.messages["btn_cancel"], content, app.mainWindow)
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
		// app.appendLog("[DEBUG] showUpdateChoiceDialog: mainWindow or Canvas is nil")
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
		if info, ok := app.nexusVersionCache[cacheKey]; ok {
			currentVersion = info.Version
		}
		titleText := fmt.Sprintf("%s  %s → %s", displayName, currentVersion, update.FileInfo.Version)
		titleLabel := widget.NewLabel(titleText)
		titleLabel.TextStyle = fyne.TextStyle{Bold: true}

		// Список изменений (изначально скрыт)
		changelogText := update.Changelog
		if changelogText == "" {
			changelogText = app.messages["changelog_unavailable"]
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
		btnState.btn = widget.NewButton(app.messages["btn_show_changelog"], func() {
			btnState.expanded = !btnState.expanded
			if btnState.expanded {
				changelogContainer.Show()
				btnState.btn.SetText(app.messages["btn_hide_changelog"])
			} else {
				changelogContainer.Hide()
				btnState.btn.SetText(app.messages["btn_show_changelog"])
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
		fmt.Sprintf(app.messages["update_available_x"], len(updates)),
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	confirmBtn := widget.NewButton(app.messages["btn_update_mods"], func() {
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
	cancelBtn := widget.NewButton(app.messages["btn_cancel"], func() {
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
	entry.SetPlaceHolder(app.messages["profile_name_placeholder"])

	// Чекбокс "Копировать из текущего"
	copyCheck := widget.NewCheck(app.messages["profile_copy_from_current"], nil)
	copyCheck.SetChecked(true)

	var popUp *widget.PopUp

	// Кнопки с центрированием
	btnCreate := widget.NewButton(app.messages["btn_create"], func() {
		name := strings.TrimSpace(entry.Text)
		if name == "" {
			app.showInfoDialog(app.messages["error_title"], app.messages["profile_name_empty"])
			return
		}
		copyFrom := ""
		if copyCheck.Checked {
			copyFrom = app.cfg.ActiveProfile
		}
		if err := app.createProfile(name, copyFrom); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create profile: %v", err))
			app.showInfoDialog(app.messages["error_title"], app.messages["profile_create_failed"])
			return
		}
		app.switchProfile(name)
		popUp.Hide()
	})

	btnCancel := widget.NewButton(app.messages["btn_cancel"], func() {
		popUp.Hide()
	})

	btnContainer := container.NewCenter(
		container.NewHBox(btnCreate, btnCancel),
	)

	content := container.NewVBox(
		widget.NewLabelWithStyle(app.messages["profile_create_title"], fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(app.messages["profile_create_message"]),
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
	if app.cfg.ActiveProfile == "Default" {
		app.appendLog(app.messages["profile_cannot_rename_default"])
		return
	}
	entry := widget.NewEntry()
	entry.SetText(app.cfg.ActiveProfile)
	entry.SetPlaceHolder(app.messages["profile_name_placeholder"])

	var popUp *widget.PopUp

	btnRename := widget.NewButton(app.messages["btn_rename"], func() {
		newName := strings.TrimSpace(entry.Text)
		if newName == "" {
			app.showInfoDialog(app.messages["error_title"], app.messages["profile_name_empty"])
			return
		}
		if err := app.renameProfile(app.cfg.ActiveProfile, newName); err != nil {
			app.appendLog(fmt.Sprintf("Failed to rename profile: %v", err))
			app.showInfoDialog(app.messages["error_title"], app.messages["profile_rename_failed"])
			return
		}
		app.refreshProfileList()
		popUp.Hide()
	})

	btnCancel := widget.NewButton(app.messages["btn_cancel"], func() {
		popUp.Hide()
	})

	btnContainer := container.NewCenter(container.NewHBox(btnRename, btnCancel))

	content := container.NewVBox(
		widget.NewLabelWithStyle(app.messages["profile_rename_title"], fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
		widget.NewLabel(fmt.Sprintf(app.messages["profile_rename_message"], app.cfg.ActiveProfile)),
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
	if app.cfg.ActiveProfile == "Default" {
		app.showInfoDialog(app.messages["error_title"], app.messages["profile_cannot_delete_default"])
		return
	}

	nameToDelete := app.cfg.ActiveProfile

	// Собираем другие нестандартные профили (исключая Default и удаляемый)
	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLog(fmt.Sprintf("Failed to read profiles: %v", err))
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
		app.messages["profile_delete_title"],
		fmt.Sprintf(app.messages["profile_delete_message"], nameToDelete),
		func() {
			// Переключаемся на target
			app.switchProfile(target)
			// Удаляем старый
			if err := app.deleteProfile(nameToDelete); err != nil {
				app.appendLog(fmt.Sprintf("Failed to delete profile: %v", err))
				app.showInfoDialog(app.messages["error_title"], app.messages["profile_delete_failed"])
				return
			}
			app.refreshProfileList()
			app.appendLog(fmt.Sprintf("Deleted profile: %s", nameToDelete))
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
				app.appendLog(fmt.Sprintf("Failed to create temp directory: %v", err))
				return
			}
			defer os.RemoveAll(tmpDir)

			if err := app.extractZipArchive(srcPath, tmpDir); err != nil {
				app.appendLog(fmt.Sprintf("Failed to extract zip archive: %v", err))
				return
			}

			// Ожидаем, что внутри архива будет одна папка (имя профиля)
			entries, err := os.ReadDir(tmpDir)
			if err != nil || len(entries) != 1 || !entries[0].IsDir() {
				app.appendLog("Archive must contain exactly one folder (the profile name)")
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
			app.appendLog(app.messages["profile_import_no_mods"])
			return
		}

		// Определяем имя нового профиля
		baseName := filepath.Base(profileFolder)
		if app.profileExists(baseName) {
			baseName = baseName + "_imported"
		}

		if err := app.createProfile(baseName, ""); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create profile: %v", err))
			return
		}

		dstPath := app.profilePath(baseName)
		if err := copyPath(profileFolder, dstPath); err != nil {
			app.appendLog(fmt.Sprintf("Failed to import profile: %v", err))
			return
		}
		cleanupModsFolder(filepath.Join(dstPath, "mods"))
		app.appendLog(fmt.Sprintf("Profile imported: %s", baseName))
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
		profileName := app.cfg.ActiveProfile
		zipPath := filepath.Join(dstDir, profileName+".zip")

		// Создаём временную папку для подготовки архива
		tmpDir, err := os.MkdirTemp("", "profile-export-")
		if err != nil {
			app.appendLog(fmt.Sprintf("Failed to create temp directory: %v", err))
			return
		}
		defer os.RemoveAll(tmpDir)

		// Копируем содержимое профиля во временную папку
		srcPath := app.activeProfilePath()
		tmpProfileDir := filepath.Join(tmpDir, profileName)
		if err := copyPath(srcPath, tmpProfileDir); err != nil {
			app.appendLog(fmt.Sprintf("Failed to copy profile to temp: %v", err))
			return
		}

		// Архивируем временную папку
		if err := app.createZipArchive(tmpProfileDir, zipPath); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create zip archive: %v", err))
			return
		}

		app.appendLog(fmt.Sprintf("Profile exported to: %s", zipPath))
	}, app.mainWindow)
	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}
