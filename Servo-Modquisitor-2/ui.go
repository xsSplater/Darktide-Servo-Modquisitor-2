package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/config"
	"fmt"
	"image/color"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

func (app *App) buildUI() {
	app.logWindow = widget.NewMultiLineEntry()
	app.logWindow.Disable()
	app.logWindow.Wrapping = fyne.TextWrapWord
	crtData, _ := embeddedFiles.ReadFile(config.ConsoleBackgroundImage)
	if crtData != nil {
		crtImg := canvas.NewImageFromResource(fyne.NewStaticResource("CRT_BlackBG", crtData))
		crtImg.FillMode = canvas.ImageFillStretch
		grad := canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
		grad.Translucency = config.ConsoleGradientOpacity
		app.logContainer = container.NewStack(crtImg, grad, app.logWindow)
	} else {
		grad := canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
		app.logContainer = container.NewStack(grad, app.logWindow)
	}
	app.consoleScroll = container.NewScroll(app.logContainer)
	app.consoleScroll.SetMinSize(fyne.NewSize(config.ConsoleWidth, config.ConsoleHeight))

	app.searchEntry = widget.NewEntry()
	app.searchEntry.SetPlaceHolder(app.messages["search_placeholder"])
	app.filterSelect = widget.NewSelect([]string{
		app.messages["filter_all"], app.messages["filter_active"], app.messages["filter_inactive"],
		app.messages["filter_obsolete"], app.messages["filter_conflict"],
	}, nil)
	app.filterSelect.SetSelected(app.messages["filter_all"])
	app.searchEntry.OnChanged = func(s string) { app.filterModList() }
	app.filterSelect.OnChanged = func(s string) { app.filterModList() }
	app.filterLabel = widget.NewLabel(app.messages["filter_label"])
	filterPanel := container.NewBorder(nil, nil,
		container.NewHBox(app.filterLabel, app.filterSelect),
		nil,
		app.searchEntry,
	)

	app.modTable = widget.NewTable(
		func() (int, int) { return len(app.displayedMods), config.TableColumnCount },
		func() fyne.CanvasObject { return container.NewStack(widget.NewLabel("")) },
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row >= len(app.displayedMods) {
				return
			}
			mod := app.displayedMods[id.Row]
			cont := cell.(*fyne.Container)
			cont.Objects = nil
			switch id.Col {
			case 0:
				check := widget.NewCheck("", nil)
				check.SetChecked(mod.Active)
				check.OnChanged = func(b bool) { app.toggleModActive(mod.Name, b) }
				cont.Add(check)
			case 1:
				num := widget.NewLabel(fmt.Sprintf("%2d.", id.Row+1))
				cont.Add(num)
			case 2:
				display := mod.DisplayName
				if display == "" {
					display = mod.Name
				}
				cont.Add(widget.NewLabel(display))
			case 3:
				dateStr := app.formatDate(mod.ModTime, app.cfg.DateFormat)
				cont.Add(widget.NewLabel(dateStr))
			case 4:
				var statusStr string
				var clr color.Color = color.White
				switch {
				case mod.Broken:
					statusStr = app.messages["desc_broken"]
					clr = color.NRGBA{R: 255, G: 80, B: 80, A: 255}
				case mod.Incompatible:
					statusStr = app.messages["desc_conflict"]
					clr = color.NRGBA{R: 255, G: 140, B: 0, A: 255}
				case mod.Obsolete:
					statusStr = app.messages["desc_obsolete"]
					clr = color.NRGBA{R: 180, G: 180, B: 0, A: 255}
				case mod.Mandatory:
					statusStr = app.messages["status_mandatory"]
					clr = color.NRGBA{R: 0, G: 180, B: 0, A: 255}
				default:
					if mod.Active {
						statusStr = app.messages["status_active"]
						clr = color.NRGBA{R: 100, G: 200, B: 100, A: 255}
					} else {
						statusStr = app.messages["status_inactive"]
						clr = color.NRGBA{R: 140, G: 140, B: 140, A: 255}
					}
				}
				statusText := canvas.NewText(statusStr, clr)
				cont.Add(statusText)
			case 5:
				noteLabel := widget.NewLabel(mod.Note)
				noteLabel.Wrapping = fyne.TextWrapWord
				cont.Add(noteLabel)
			}
		},
	)
	config.ApplyTableColumnWidths(app.modTable)

	app.modTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.displayedMods) {
			app.selectedModName = app.displayedMods[id.Row].Name
			app.updateDescriptionForMod(app.selectedModName)
		}
	}

	app.modListTitle = widget.NewLabelWithStyle(app.messages["mod_list_title"], fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	leftPanel := container.NewBorder(
		container.NewVBox(app.modListTitle, filterPanel),
		nil, nil, nil,
		app.modTable,
	)

	app.descTitle = widget.NewLabel(app.messages["select_mod"])
	app.descTitle.TextStyle = fyne.TextStyle{Bold: true}
	app.descAuthor = widget.NewLabel("—")
	app.descBody = widget.NewLabel(app.messages["desc_placeholder"])
	app.descBody.Wrapping = fyne.TextWrapWord
	app.descURL = widget.NewHyperlink("", nil)
	app.descURL.Alignment = fyne.TextAlignLeading

	app.btnSaveOrder = widget.NewButton(app.messages["btn_save_order"], func() {
		if app.orderDirty {
			app.saveCurrentOrder()
			app.orderDirty = false
			app.appendLog(app.messages["log_order_saved"])
		} else {
			app.appendLog(app.messages["log_order_unchanged"])
		}
	})
	app.btnSortChecks = widget.NewButton(app.messages["btn_sort_checks"], func() { go app.runAllChecks() })
	app.btnSortChecks.Importance = widget.WarningImportance
	app.btnRefresh = widget.NewButton(app.messages["btn_refresh"], func() {
		app.refreshModList()
		app.appendLog(app.messages["log_list_refreshed"])
	})
	app.btnInstall = widget.NewButton(app.messages["btn_install"], func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				defer reader.Close()
				path := reader.URI().Path()
				if strings.HasSuffix(strings.ToLower(path), ".zip") {
					err := app.extractArchive(path)
					if err != nil {
						app.appendLog(fmt.Sprintf(app.messages["log_extract_error"], err))
					} else {
						checks.AutoFixMalformed()
						app.refreshModList()
						app.appendLog(fmt.Sprintf(app.messages["log_installed"], filepath.Base(path)))
					}
				} else {
					app.appendLog(app.messages["log_zip_only"])
				}
			}
		}, app.mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip", ".rar", ".7z"}))
		fd.Resize(fyne.NewSize(config.FileDialogWidth, config.FileDialogHeight))
		fd.Show()
	})
	app.btnInstallFolder = widget.NewButton(app.messages["btn_install_folder"], func() {
		fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err == nil && uri != nil {
				path := uri.Path()
				app.copyFolder(path, filepath.Join(app.cfg.ModsPath, filepath.Base(path)))
				checks.AutoFixMalformed()
				app.refreshModList()
				app.appendLog(fmt.Sprintf(app.messages["log_installed_folder"], filepath.Base(path)))
			}
		}, app.mainWindow)
		fd.Resize(fyne.NewSize(config.FileDialogWidth, config.FileDialogHeight))
		fd.Show()
	})
	app.btnRemove = widget.NewButton(app.messages["btn_remove"], func() {
		if app.selectedModName == "" {
			return
		}
		modName := app.selectedModName
		mod := app.findModByName(modName)
		if mod == nil {
			return
		}
		dialog.ShowConfirm(app.messages["confirm_delete_title"],
			fmt.Sprintf(app.messages["confirm_delete_text"], mod.Name),
			func(ok bool) {
				if ok {
					checks.RemoveMod(modName)
					app.removeFromAllMods(modName)
					app.refreshModList()
					app.appendLog(fmt.Sprintf(app.messages["log_deleted"], modName))
				}
			},
			app.mainWindow,
		)
	})
	app.btnToggle = widget.NewButton("", func() { app.toggleGlobalMods() })
	app.updateToggleButtonText(app.btnToggle)

	app.btnUp = widget.NewButton(app.messages["btn_up"], func() { app.moveSelected(-1) })
	app.btnUp.Disable()
	app.btnDown = widget.NewButton(app.messages["btn_down"], func() { app.moveSelected(1) })
	app.btnDown.Disable()

	app.btnExport = widget.NewButton(app.messages["btn_export"], func() {
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err == nil && writer != nil {
				defer writer.Close()
				entries := app.buildLoadOrderEntries()
				checks.WriteLoadOrder(entries)
				src, _ := os.Open(filepath.Join(app.cfg.ModsPath, "mod_load_order.txt"))
				if src != nil {
					io.Copy(writer, src)
					src.Close()
				}
				app.appendLog(app.messages["log_exported"])
			}
		}, app.mainWindow)
	})
	app.btnImport = widget.NewButton(app.messages["btn_import"], func() {
		dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				defer reader.Close()
				data, _ := io.ReadAll(reader)
				os.WriteFile(filepath.Join(app.cfg.ModsPath, "mod_load_order.txt"), data, 0644)
				app.refreshModList()
				app.appendLog(app.messages["log_imported"])
			}
		}, app.mainWindow)
	})

	topRight := container.NewVBox(
		app.btnToggle,
		container.NewHBox(app.btnSortChecks, app.btnExport, app.btnImport),
		container.NewHBox(app.btnUp, app.btnDown, app.btnSaveOrder),
		container.NewHBox(app.btnRefresh, app.btnInstall, app.btnRemove),
		widget.NewSeparator(),
		widget.NewSeparator(),
		app.descTitle,
		app.descAuthor,
		app.descURL,
	)
	descScroll := container.NewScroll(app.descBody)
	descScroll.SetMinSize(fyne.NewSize(config.DescScrollMinWidth, config.DescScrollMinHeight))
	rightPanel := container.NewBorder(
		topRight,
		nil, nil, nil,
		descScroll,
	)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = config.SplitOffset
	content := container.NewBorder(nil, app.consoleScroll, nil, nil, split)
	app.mainWindow.SetContent(content)

	app.appendCenteredLog(app.messages["log_start0"] + "\n")
	app.filterModList()
}

func (app *App) appendLog(text string) {
	if app.logWindow == nil {
		if app.logFile != nil {
			fmt.Fprintln(app.logFile, time.Now().Format("15:04:05"), text)
		}
		return
	}
	fyne.Do(func() {
		app.logWindow.SetText(app.logWindow.Text + text + "\n")
		app.logContainer.Refresh()
		if app.consoleScroll != nil {
			app.consoleScroll.ScrollToBottom()
		}
	})
	if app.logFile != nil {
		fmt.Fprintln(app.logFile, time.Now().Format("15:04:05"), text)
	}
}

func (app *App) appendCenteredLog(text string) {
	width := app.logWindow.Size().Width
	if width <= 0 {
		width = float32(config.LogDefaultWidth)
	}
	const charWidth = 4
	maxChars := int(float64(width) / config.LogCharWidth)
	if maxChars < len(text) {
		maxChars = len(text)
	}
	padLeft := (maxChars - len(text)) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	app.appendLog(strings.Repeat(" ", padLeft) + text)
}

func (app *App) showChoiceDialog(parent fyne.Window, title, message string, options ...string) int {
	resultChan := make(chan int, 1)
	fyne.DoAndWait(func() {
		var buttons []fyne.CanvasObject
		for i, opt := range options {
			idx := i
			btn := widget.NewButton(opt, func() {
				resultChan <- idx
			})
			buttons = append(buttons, btn)
		}
		gradHeader := canvas.NewImageFromImage(app.makeRedCRTGradient(600, 50))
		gradHeader.FillMode = canvas.ImageFillStretch
		titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		headerContainer := container.NewStack(gradHeader, container.NewCenter(titleLabel))

		content := container.NewVBox(
			headerContainer,
			widget.NewLabel(message),
			container.NewCenter(container.NewHBox(buttons...)),
		)
		popUp := widget.NewModalPopUp(content, parent.Canvas())
		popUp.Resize(fyne.NewSize(config.DialogMinWidth, config.DialogMinHeight))
		popUp.Show()
		go func() {
			idx := <-resultChan
			fyne.Do(func() { popUp.Hide() })
			resultChan <- idx
		}()
	})
	return <-resultChan
}

func (app *App) updateDescriptionForMod(name string) {
	if name == "" {
		app.descTitle.SetText(app.messages["select_mod"])
		app.descAuthor.SetText("—")
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
		app.descBody.SetText(app.messages["desc_placeholder"])
		return
	}
	mod := app.findModByName(name)
	if mod == nil {
		return
	}
	display := mod.DisplayName
	if display == "" {
		display = mod.Name
	}
	app.descTitle.SetText(display)

	author := mod.Author
	if author == "" {
		author = app.messages["author_unknown"]
	}
	app.descAuthor.SetText(fmt.Sprintf(app.messages["author_format"], author, mod.ModTime.Format("02-01-2006")))

	body := mod.Description
	if mod.Incompatible {
		body += "\n" + app.messages["desc_conflict"]
	}
	if mod.Obsolete {
		body += "\n" + app.messages["desc_obsolete"]
	}
	if mod.Broken {
		body += "\n" + app.messages["desc_broken"]
	}
	app.descBody.SetText(body)

	if mod.URL != "" {
		u, err := url.Parse(mod.URL)
		if err == nil {
			app.descURL.SetURL(u)
			app.descURL.SetText(app.messages["mod_url_label"])
		} else {
			app.descURL.SetURL(nil)
			app.descURL.SetText("")
		}
	} else {
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
	}
}

func (app *App) updateToggleButtonText(btn *widget.Button) {
	switch app.patcherType {
	case PatcherAutoPatch:
		if isModsEnabledAutoPatch() {
			btn.SetText(app.messages["btn_disable_mods"])
		} else {
			btn.SetText(app.messages["btn_enable_mods"])
		}
	case PatcherLegacy:
		if app.cfg.ModsGloballyEnabled {
			btn.SetText(app.messages["btn_disable_mods"])
		} else {
			btn.SetText(app.messages["btn_enable_mods"])
		}
	default:
		btn.SetText(app.messages["btn_no_patcher"])
		btn.Disable()
	}
}

func (app *App) updateUpDownButtons() {
	if app.selectedModName == "" {
		app.btnUp.Disable()
		app.btnDown.Disable()
		return
	}
	idx := -1
	for i, m := range app.allMods {
		if m.Name == app.selectedModName {
			idx = i
			break
		}
	}
	app.btnUp.Enable()
	app.btnDown.Enable()
	if idx <= 0 {
		app.btnUp.Disable()
	}
	if idx >= len(app.allMods)-1 {
		app.btnDown.Disable()
	}
}
