// ui_build.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func createTableRow(height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(1, height))
	lbl := widget.NewLabel("")
	return container.NewStack(spacer, lbl)
}

// VBoxWithSpacing — вертикальный layout с заданным отступом между элементами.
type VBoxWithSpacing struct {
	Spacing float32
}

func (v VBoxWithSpacing) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	// Вычисляем общую высоту всех объектов
	var totalHeight float32
	for _, obj := range objects {
		totalHeight += obj.MinSize().Height
	}
	totalHeight += v.Spacing * float32(len(objects)-1)

	// Если общая высота меньше размера контейнера — центрируем по вертикали
	y := (size.Height - totalHeight) / 2
	if y < 0 {
		y = 0
	}
	for _, obj := range objects {
		minSize := obj.MinSize()
		obj.Resize(fyne.NewSize(size.Width, minSize.Height))
		obj.Move(fyne.NewPos(0, y))
		y += minSize.Height + v.Spacing
	}
}

func (v VBoxWithSpacing) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	var maxWidth float32
	var totalHeight float32
	for _, obj := range objects {
		min := obj.MinSize()
		if min.Width > maxWidth {
			maxWidth = min.Width
		}
		totalHeight += min.Height
	}
	totalHeight += v.Spacing * float32(len(objects)-1)
	return fyne.NewSize(maxWidth, totalHeight)
}

func (app *App) buildUI() {
	// app.fixHubHotkeyMenus()
	// Лог
	app.logWindow = widget.NewRichText(
		&widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForegroundOnWarning,
				TextStyle: fyne.TextStyle{},
			},
		},
	)
	app.logWindow.Wrapping = fyne.TextWrapWord

	crtData, _ := embeddedFiles.ReadFile(ConsoleBackgroundImage)
	var crtImg *canvas.Image
	var grad *canvas.Image
	if crtData != nil {
		crtImg = canvas.NewImageFromResource(fyne.NewStaticResource("CRT_BlackBG", crtData))
		crtImg.FillMode = canvas.ImageFillStretch
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
		grad.Translucency = ConsoleGradientOpacity
	} else {
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
	}

	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	app.screenBgRect = canvas.NewRectangle(th.Color(themes.ColorCRTScreenFill, variant))
	app.screenBgRect.CornerRadius = 22
	app.screenBgRect.StrokeWidth = 2
	app.screenBgRect.StrokeColor = th.Color(themes.ColorCRTScreenStroke, variant)
	screenBg := app.screenBgRect

	app.logHeaderText = canvas.NewText("", th.Color(themes.ColorConsoleText, variant))
	app.logHeaderText.TextStyle = fyne.TextStyle{Bold: true}
	app.logHeaderText.Alignment = fyne.TextAlignCenter
	app.logHeaderText.TextSize = theme.TextSize()

	logStack := container.NewStack()
	if crtImg != nil {
		logStack.Add(crtImg)
	}
	logStack.Add(grad)
	logStack.Add(screenBg)
	logStack.Add(container.NewPadded(app.logWindow))

	app.headerBoxBgRect = canvas.NewRectangle(th.Color(themes.ColorCRTHeaderBg, variant))
	headerBox := container.NewStack(
		app.headerBoxBgRect,
		container.NewCenter(app.logHeaderText),
	)

	logPanel := container.NewBorder(headerBox, nil, nil, nil, logStack)

	app.consoleScroll = container.NewScroll(logPanel)
	app.consoleScroll.SetMinSize(fyne.NewSize(ConsoleWidth, ConsoleHeight))

	// Поиск и фильтр
	app.searchEntry = widget.NewEntry()
	app.searchEntry.SetPlaceHolder(app.messages["search_placeholder"])

	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(SearchMinWidth, 1))
	searchEntryBox := container.NewStack(searchSpacer, app.searchEntry)

	app.searchClearBtn = NewCustomButton("✕", func() {
		app.searchEntry.SetText("")
	})
	app.searchClearBtn.Importance = widget.DangerImportance
	app.searchClearBtn.Hide()

	app.searchEntry.OnChanged = func(s string) {
		if s != "" {
			app.searchClearBtn.Show()
		} else {
			app.searchClearBtn.Hide()
		}
		app.filterModList()
	}

	searchBar := container.NewBorder(nil, nil, nil, app.searchClearBtn, searchEntryBox)

	app.filterSelect = widget.NewSelect(app.filterOptions(), nil)
	app.filterSelect.SetSelected(app.messages["filter_all"])
	app.filterSelect.OnChanged = func(s string) { app.filterModList() }
	// Увеличиваем ширину выпадающего списка фильтра
	filterSpacer := canvas.NewRectangle(color.Transparent)
	filterSpacer.SetMinSize(fyne.NewSize(FilterMinWidth, 1))
	filterSelectWithSize := container.NewStack(filterSpacer, app.filterSelect)

	// Кнопки быстрого перемещения
	upImgData, err := embeddedFiles.ReadFile("assets/buttons/up.png")
	if err != nil {
		app.appendLog("Could not load up icon: " + err.Error())
	}
	upRes := fyne.NewStaticResource("up", upImgData)

	downImgData, err := embeddedFiles.ReadFile("assets/buttons/down.png")
	if err != nil {
		app.appendLog("Could not load down icon: " + err.Error())
	}
	downRes := fyne.NewStaticResource("down", downImgData)

	topImgData, err := embeddedFiles.ReadFile("assets/buttons/top.png")
	if err != nil {
		app.appendLog("Could not load top icon: " + err.Error())
	}
	topRes := fyne.NewStaticResource("top", topImgData)

	bottomImgData, err := embeddedFiles.ReadFile("assets/buttons/bottom.png")
	if err != nil {
		app.appendLog("Could not load bottom icon: " + err.Error())
	}
	bottomRes := fyne.NewStaticResource("bottom", bottomImgData)

	// Иконка для Remove Selected
	selTrashImgData, err := embeddedFiles.ReadFile("assets/buttons/trashcan_red_sel.png")
	if err != nil {
		app.appendLog("Could not load selected trash icon: " + err.Error())
	}
	selTrashRes := fyne.NewStaticResource("trash_sel", selTrashImgData)

	ImgPngData, err := embeddedFiles.ReadFile("assets/buttons/trashcan_red.png")
	if err != nil {
		app.appendLog("Could not load trash icon: " + err.Error())
	}
	trashRes := fyne.NewStaticResource("trash", ImgPngData)

	// Иконка для Remove All (красная корзина с крестиком)
	trashXImgData, err := embeddedFiles.ReadFile("assets/buttons/trashcan_red_x.png")
	if err != nil {
		app.appendLog("Could not load trash_x icon: " + err.Error())
	}
	trashXRes := fyne.NewStaticResource("trash_x", trashXImgData)

	app.moveToTopBtn = NewIconButton(topRes, func() { app.moveSelectedToTop() })
	app.moveToTopBtn.SetToolTip(app.messages["btn_move_to_top_tooltip"])

	app.moveToBottomBtn = NewIconButton(bottomRes, func() { app.moveSelectedToBottom() })
	app.moveToBottomBtn.SetToolTip(app.messages["btn_move_to_bottom_tooltip"])

	app.moveToEntry = widget.NewEntry()
	app.moveToEntry.SetPlaceHolder(app.messages["col_number"])
	app.moveToEntry.OnSubmitted = func(text string) { app.moveSelectedToPosition() }
	app.moveLabel = widget.NewLabel(app.messages["lbl_move_to"])

	// Кнопки выделения и массовых операций
	// Иконки для Select All / Deselect All
	selectAllImgData, err := embeddedFiles.ReadFile("assets/buttons/select_all.png")
	if err != nil {
		app.appendLog("Could not load select all icon: " + err.Error())
	}
	selectAllRes := fyne.NewStaticResource("select_all", selectAllImgData)
	app.selectAllBtn = NewIconButton(selectAllRes, func() { app.selectAllMods(true) })
	app.selectAllBtn.SetToolTip(app.messages["btn_select_all_tooltip"])

	selectAllDeImgData, err := embeddedFiles.ReadFile("assets/buttons/select_all_de.png")
	if err != nil {
		app.appendLog("Could not load select all de icon: " + err.Error())
	}
	selectAllDeRes := fyne.NewStaticResource("select_all_de", selectAllDeImgData)
	app.deselectAllBtn = NewIconButton(selectAllDeRes, func() { app.selectAllMods(false) })
	app.deselectAllBtn.SetToolTip(app.messages["btn_deselect_all_tooltip"])

	app.btnRemoveAll = NewIconButton(trashXRes, func() {
		app.showConfirmDialog(
			app.messages["confirm_remove_all_title"],
			app.messages["confirm_remove_all_text"],
			func() {
				app.removeAllMods()
			},
		)
	})
	app.btnRemoveAll.SetToolTip(app.messages["btn_remove_all_tooltip"])

	app.btnRemoveSelected = NewIconButton(selTrashRes, func() {
		sel := app.selectedMods()
		if len(sel) == 0 {
			app.appendLog(app.messages["no_mods_selected"])
			return
		}
		app.showConfirmDialog(
			app.messages["confirm_remove_selected_title"],
			fmt.Sprintf(app.messages["confirm_remove_selected_text"], len(sel)),
			func() {
				app.removeSelectedMods()
			},
		)
	})
	app.btnRemoveSelected.SetToolTip(app.messages["btn_remove_selected_tooltip"])

	editVersionImgData, err := embeddedFiles.ReadFile("assets/buttons/edit_version.png")
	if err != nil {
		app.appendLog("Could not load edit_version icon: " + err.Error())
	}
	editVersionRes := fyne.NewStaticResource("edit_version", editVersionImgData)
	app.btnEditVersion = NewIconButton(editVersionRes, func() {
		if app.selectedModName == "" {
			return
		}
		mod := app.findModByName(app.selectedModName)
		if mod == nil {
			return
		}
		app.showEditVersionDialog(mod)
	})
	app.btnEditVersion.SetToolTip(app.messages["btn_edit_version_tooltip"])

	// Основные кнопки
	app.btnUp = NewIconButton(upRes, func() { app.moveSelected(-1) })
	app.btnUp.SetToolTip(app.messages["btn_up_tooltip"])

	app.btnDown = NewIconButton(downRes, func() { app.moveSelected(1) })
	app.btnDown.SetToolTip(app.messages["btn_down_tooltip"])

	// Save List
	saveImgData, err := embeddedFiles.ReadFile("assets/buttons/save.png")
	if err != nil {
		app.appendLog("Could not load save icon: " + err.Error())
	}
	saveRes := fyne.NewStaticResource("save", saveImgData)

	app.btnSaveOrder = NewIconButton(saveRes, func() {
		if app.orderDirty {
			app.saveCurrentOrder()
			app.orderDirty = false
			app.refreshModList()
			app.appendLog(app.messages["log_order_saved"])
			app.stopBlinkSaveButton()
			app.updateTableBorder()
		} else {
			app.appendLog(app.messages["log_order_unchanged"])
		}
	})
	app.btnSaveOrder.SetToolTip(app.messages["btn_save_order_tooltip"])

	// Refresh List
	refreshImgData, err := embeddedFiles.ReadFile("assets/buttons/refresh.png")
	if err != nil {
		app.appendLog("Could not load refresh icon: " + err.Error())
	}
	refreshRes := fyne.NewStaticResource("refresh", refreshImgData)

	app.btnRefresh = NewIconButton(refreshRes, func() {
		go func() {
			if app.orderDirty {
				choice := app.showChoiceDialogSync(app.mainWindow,
					app.messages["warning_title"],
					app.messages["refresh_discard_changes"],
					app.messages["btn_save_and_refresh"],
					app.messages["btn_cancel"],
					app.messages["btn_refresh_anyway"],
				)
				fyne.Do(func() {
					switch choice {
					case 0:
						app.saveCurrentOrder()
						app.orderDirty = false
						app.stopBlinkSaveButton()
						app.updateTableBorder()
						app.appendLog(app.messages["log_order_saved"])
						app.refreshModList()
						app.appendLog(app.messages["log_list_refreshed"])
					case 1:
						// Отмена
					case 2:
						app.orderDirty = false
						app.stopBlinkSaveButton()
						app.updateTableBorder()
						app.refreshModList()
						app.appendLog(app.messages["log_list_refreshed"])
					}
				})
			} else {
				fyne.Do(func() {
					app.refreshModList()
					app.appendLog(app.messages["log_list_refreshed"])
				})
			}
		}()
	})
	app.btnRefresh.SetToolTip(app.messages["btn_refresh_tooltip"])

	// Toggle Mods - иконки on.png / off_red.png
	onImgData, err := embeddedFiles.ReadFile("assets/buttons/on.png")
	if err != nil {
		app.appendLog("Could not load on icon: " + err.Error())
	}
	app.toggleOnIcon = fyne.NewStaticResource("on", onImgData)

	offImgData, err := embeddedFiles.ReadFile("assets/buttons/off_red.png")
	if err != nil {
		app.appendLog("Could not load off icon: " + err.Error())
	}
	app.toggleOffIcon = fyne.NewStaticResource("off", offImgData)

	app.btnToggle = NewIconButton(app.toggleOnIcon, func() { app.toggleGlobalMods() })
	app.btnToggle.SetToolTip(app.messages["btn_toggle_tooltip"])
	app.updateToggleButtonText(app.btnToggle)

	// Кнопка управления модами и панель
	cogImgData, err := embeddedFiles.ReadFile("assets/buttons/cog_check.png")
	if err != nil {
		app.appendLog("Could not load cog icon: " + err.Error())
	}
	cogRes := fyne.NewStaticResource("cog", cogImgData)

	app.manageBtn = NewIconButton(cogRes, func() {
		if app.managePanel.Visible() {
			app.managePanel.Hide()
			app.showSelectColumn = false
			app.headerTable.SetColumnWidth(0, 0)
			app.modTable.SetColumnWidth(0, 0)
		} else {
			app.managePanel.Show()
			app.showSelectColumn = true
			app.headerTable.SetColumnWidth(0, ColSelectWidth)
			app.modTable.SetColumnWidth(0, ColSelectWidth)
		}
		app.headerTable.Refresh()
		app.modTable.Refresh()
		app.managePanel.Refresh()
	})
	app.manageBtn.SetToolTip(app.messages["btn_manage_mods_tooltip"])

	// Загрузка иконки обновления
	updateSelImgData, err := embeddedFiles.ReadFile("assets/buttons/update_selected_blue_p.png")
	if err != nil {
		app.appendLog("Could not load update icon: " + err.Error())
	}
	updateSelRes := fyne.NewStaticResource("update", updateSelImgData)

	app.btnUpdateSelected = NewIconButton(updateSelRes, func() {
		go app.updateSelectedMods()
	})
	app.btnUpdateSelected.SetToolTip(app.messages["btn_update_selected_tooltip"])

	app.btnAMLConfig = NewCustomButton(app.messages["btn_aml_config"], func() { app.showAMLConfigWindow() })
	app.btnAMLConfig.SetToolTip(app.messages["btn_aml_config_tooltip"])

	if btnImgData, _ := embeddedFiles.ReadFile(ButtonBackgroundImage); btnImgData != nil {
		img := canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG_button", btnImgData))
		img.FillMode = canvas.ImageFillStretch
		img.Translucency = 0.8
		app.manageBtn.SetBackgroundImage(img)
	}
	if colImgData, _ := embeddedFiles.ReadFile(ColBackgroundImage); colImgData != nil {
		app.selectColumnBgRes = fyne.NewStaticResource("Yellow_BG_col", colImgData)
	}

	// Иконки для Enable Selected / Disable Selected
	checkedBoxImgData, err := embeddedFiles.ReadFile("assets/buttons/checked_box.png")
	if err != nil {
		app.appendLog("Could not load checked_box icon: " + err.Error())
	}
	checkedBoxRes := fyne.NewStaticResource("checked_box", checkedBoxImgData)
	app.enableSelectedBtn = NewIconButton(checkedBoxRes, func() { app.setSelectedActive(true) })
	app.enableSelectedBtn.SetToolTip(app.messages["btn_enable_selected_tooltip"])

	checkedBoxUnImgData, err := embeddedFiles.ReadFile("assets/buttons/checked_box_un.png")
	if err != nil {
		app.appendLog("Could not load checked_box_un icon: " + err.Error())
	}
	checkedBoxUnRes := fyne.NewStaticResource("checked_box_un", checkedBoxUnImgData)
	app.disableSelectedBtn = NewIconButton(checkedBoxUnRes, func() { app.setSelectedActive(false) })
	app.disableSelectedBtn.SetToolTip(app.messages["btn_disable_selected_tooltip"])

	enableAllImgData, err := embeddedFiles.ReadFile("assets/buttons/enable_all.png")
	if err != nil {
		app.appendLog("Could not load enable_all icon: " + err.Error())
	}
	enableAllRes := fyne.NewStaticResource("enable_all", enableAllImgData)
	app.enableAllBtn = NewIconButton(enableAllRes, func() { app.setAllModsActive(true) })
	app.enableAllBtn.SetToolTip(app.messages["btn_enable_all_tooltip"])

	disableAllImgData, err := embeddedFiles.ReadFile("assets/buttons/disable_all.png")
	if err != nil {
		app.appendLog("Could not load disable_all icon: " + err.Error())
	}
	disableAllRes := fyne.NewStaticResource("disable_all", disableAllImgData)
	app.disableAllBtn = NewIconButton(disableAllRes, func() { app.setAllModsActive(false) })
	app.disableAllBtn.SetToolTip(app.messages["btn_disable_all_tooltip"])

	// Кнопки Управления модами
	singleRow := container.NewHBox(
		app.moveLabel,
		app.moveToEntry,
		widget.NewSeparator(),
		app.btnUp,
		app.btnDown,
		app.moveToTopBtn,
		app.moveToBottomBtn,
		widget.NewSeparator(),
		app.selectAllBtn,
		app.deselectAllBtn,
		widget.NewSeparator(),
		app.enableSelectedBtn,
		app.disableSelectedBtn,
		app.enableAllBtn,
		app.disableAllBtn,
		widget.NewSeparator(),
		app.btnEditVersion,
		widget.NewSeparator(),
		app.btnUpdateSelected,
		widget.NewSeparator(),
		app.btnRemoveSelected,
		app.btnRemoveAll,
	)

	yellowData, _ := embeddedFiles.ReadFile(HeaderBackgroundImage)
	var yellowBg *canvas.Image
	if yellowData != nil {
		yellowBg = canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG", yellowData))
		yellowBg.FillMode = canvas.ImageFillStretch
		yellowBg.Translucency = 0.9
	}

	app.managePanelBgRect = canvas.NewRectangle(th.Color(themes.ColorManagePanelBg, variant))
	panelContent := container.NewVBox(singleRow)
	if yellowBg != nil {
		app.managePanel = container.NewStack(app.managePanelBgRect, yellowBg, panelContent)
	} else {
		app.managePanel = container.NewStack(app.managePanelBgRect, panelContent)
	}
	app.managePanel.Hide()

	// Install Mod - иконка add.png
	addImgData, err := embeddedFiles.ReadFile("assets/buttons/add.png")
	if err != nil {
		app.appendLog("Could not load add icon: " + err.Error())
	}
	addRes := fyne.NewStaticResource("add", addImgData)

	app.btnInstall = NewIconButton(addRes, func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err == nil && reader != nil {
				defer reader.Close()
				path := reader.URI().Path()
				if strings.HasSuffix(strings.ToLower(path), ".zip") {
					go func(p string) {
						installedName, _, err := app.InstallModFromArchive(p, true, "", "")
						fyne.Do(func() {
							if err != nil {
								app.appendLog(fmt.Sprintf(app.messages["log_extract_error"], err))
								return
							}
							checks.AutoFixMalformed()
							app.refreshModList()
							app.selectAndScrollToMod(installedName)
							app.appendLog(fmt.Sprintf(app.messages["log_installed"], filepath.Base(p)))
						})
					}(path)
				} else {
					app.appendLog(app.messages["log_zip_only"])
				}
			}
		}, app.mainWindow)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip", ".rar", ".7z"}))
		fd.Show()
		fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
	})
	app.btnInstall.SetToolTip(app.messages["btn_install_tooltip"])

	// Auto-Sort - иконка sort.png
	autosortImgData, err := embeddedFiles.ReadFile("assets/buttons/sort.png")
	if err != nil {
		app.appendLog("Could not load autosort icon: " + err.Error())
	}
	autosortRes := fyne.NewStaticResource("autosort", autosortImgData)

	app.btnSortChecks = NewIconButton(autosortRes, func() { go app.runAllChecks() })
	app.btnSortChecks.SetIconSize(32) // увеличенный размер
	app.btnSortChecks.SetToolTip(app.messages["btn_sort_checks_tooltip"])

	if app.amlDetected {
		app.btnSaveOrder.SetToolTip(app.messages["aml_save_warning_tooltip"])
		app.btnSortChecks.SetToolTip(app.messages["aml_sort_warning_tooltip"])
	}

	// Check updates
	checkUpdatesImgData, err := embeddedFiles.ReadFile("assets/buttons/check_updates_blue.png")
	if err != nil {
		app.appendLog("Could not load check updates icon: " + err.Error())
	}
	checkUpdatesRes := fyne.NewStaticResource("check_updates", checkUpdatesImgData)

	app.btnCheckUpdates = NewIconButton(checkUpdatesRes, func() {
		go app.checkNexusUpdates()
	})
	app.btnCheckUpdates.SetToolTip(app.messages["btn_check_updates_tooltip"])

	// Update All Mods
	updateAllImgData, err := embeddedFiles.ReadFile("assets/buttons/update_all_mods_blue_p.png")
	if err != nil {
		app.appendLog("Could not load update all icon: " + err.Error())
	}
	updateAllRes := fyne.NewStaticResource("update_all", updateAllImgData)

	app.btnUpdateAll = NewIconButton(updateAllRes, func() {
		go app.updateAllModsFromNexus()
	})
	app.btnUpdateAll.SetToolTip(app.messages["btn_update_all_premium_only"])

	playImgData, err := embeddedFiles.ReadFile("assets/buttons/play.png")
	if err != nil {
		app.appendLog("Could not load play icon: " + err.Error())
	}
	playRes := fyne.NewStaticResource("play", playImgData)

	gameVer := detectGameVersion(app.gameRoot)
	if gameVer == VersionUnknown {
		app.btnLaunchNormal.Hide()
		app.btnLaunchNoLauncher.Hide()
	}

	app.btnLaunchNormal = NewIconButton(playRes, func() {
		go func() {
			if isDarktideRunning() {
				app.appendLog(app.messages["game_already_running"])
				return
			}
			ver := detectGameVersion(app.gameRoot)
			err := app.launchGameFunc(ver, app.gameRoot, false)
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["launch_error"], err))
			}
		}()
	})
	app.btnLaunchNormal.SetToolTip(app.messages["btn_launch_game_tooltip"])

	playFastImgData, err := embeddedFiles.ReadFile("assets/buttons/play_fast.png")
	if err != nil {
		app.appendLog("Could not load play_fast icon: " + err.Error())
	}
	playFastRes := fyne.NewStaticResource("play_fast", playFastImgData)

	app.btnLaunchNoLauncher = NewIconButton(playFastRes, func() {
		app.btnLaunchNoLauncher.SetIconSize(32)
		go func() {
			if isDarktideRunning() {
				app.appendLog(app.messages["game_already_running"])
				return
			}
			ver := detectGameVersion(app.gameRoot)
			err := app.launchGameFunc(ver, app.gameRoot, true)
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["launch_error"], err))
			}
		}()
	})
	app.btnLaunchNoLauncher.SetToolTip(app.messages["btn_launch_nolauncher_long_tooltip"])

	// Верхняя панель
	app.topPanelBgRect = canvas.NewRectangle(th.Color(themes.ColorTopPanelBg, variant))
	topPanelContent := container.NewHBox(
		app.btnInstall,
		app.btnRefresh,
		app.btnSaveOrder,
		widget.NewSeparator(),
		app.btnSortChecks,
		widget.NewSeparator(),
		app.manageBtn,
		widget.NewSeparator(),
		filterSelectWithSize,
		searchBar,
		widget.NewSeparator(),
		app.btnCheckUpdates,
		app.btnUpdateAll,
		widget.NewSeparator(),
		app.btnToggle,
		widget.NewSeparator(),
		app.btnLaunchNormal,
		app.btnLaunchNoLauncher,
	)
	topPanelWithBg := container.NewStack(app.topPanelBgRect, topPanelContent)

	// Таблица заголовков
	headerCreateCell := func() fyne.CanvasObject {
		return container.NewStack(
			canvas.NewRectangle(color.Transparent),
			widget.NewLabel(""),
		)
	}
	headerUpdateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		bg := canvas.NewRectangle(th.Color(themes.ColorTableHeaderBg, variant))
		cont.Add(bg)
		label := widget.NewLabel("")
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Alignment = fyne.TextAlignCenter
		switch id.Col {
		case 0:
			if app.showSelectColumn {
				label.SetText(" ")
			} else {
				label.SetText("")
			}
		case 1:
			label.SetText(app.messages["col_checkbox"])
		case 2:
			label.SetText(app.messages["col_number"])
		case 3:
			label.SetText(app.messages["col_name"])
		case 4:
			label.SetText(app.messages["col_date"])
		case 5:
			label.SetText(app.messages["col_status"])
		case 6:
			label.SetText(app.messages["col_note"])
		}
		cont.Add(label)
	}
	app.headerTable = widget.NewTable(
		func() (int, int) { return 1, TableColumnCount },
		headerCreateCell,
		headerUpdateCell,
	)
	ApplyTableColumnWidths(app.headerTable)
	app.headerTable.SetColumnWidth(0, 0)
	app.headerTable.OnSelected = nil

	// Таблица с DML и DMF
	systemUpdateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.systemMods) {
			return
		}
		mod := &app.systemMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		bgColor := th.Color(themes.ColorSystemTableBg, variant)
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
		case 0:
			cont.Add(widget.NewLabel(""))
		case 1:
			cont.Add(widget.NewLabel(""))
		case 2:
			cont.Add(widget.NewLabel(""))
		case 3:
			display := mod.DisplayName
			if display == "" {
				display = mod.Name
			}
			nameLabel := widget.NewLabel(display)
			nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			cont.Add(nameLabel)
		case 4:
			dateStr := app.formatDate(mod.ModTime, app.cfg.DateFormat)
			cont.Add(widget.NewLabel(dateStr))
		case 5:
			var subStatusText string
			var subStatusColor color.Color

			// Основной статус - "framework"
			mainStatusText := app.messages["status_system"]
			mainStatusColor := th.Color(themes.ColorStatusSystem, variant)

			// Дополнительный статус
			switch {
			case mod.MissingFolder:
				subStatusText = app.messages["status_missing_folder"]
				subStatusColor = th.Color(themes.ColorStatusMissing, variant)
			case mod.VortexDeployed:
				subStatusText = app.messages["status_vortex"]
				subStatusColor = th.Color(themes.ColorStatusVortex, variant)
			case mod.IsSymlink:
				subStatusText = app.messages["status_symlink"]
				subStatusColor = th.Color(themes.ColorStatusSymlink, variant)
			case mod.Source == "manual":
				subStatusText = app.messages["status_manual"]
				subStatusColor = th.Color(themes.ColorStatusManual, variant)
			case mod.Source == "nexus":
				subStatusText = app.messages["status_nexus"]
				subStatusColor = th.Color(themes.ColorStatusNexus, variant)
			default:
				subStatusText = ""
			}

			mainLabel := canvas.NewText(mainStatusText, mainStatusColor)
			mainLabel.TextSize = StatusFontSize + 2
			mainLabel.Alignment = fyne.TextAlignCenter
			mainLabel.TextStyle = fyne.TextStyle{Bold: true}

			subLabel := canvas.NewText(subStatusText, subStatusColor)
			subLabel.TextSize = StatusFontSize
			subLabel.Alignment = fyne.TextAlignCenter

			if subStatusText == "" {
				cont.Add(mainLabel)
			} else {
				// Используем кастомный layout с точным отступом
				statusBox := container.NewWithoutLayout(mainLabel, subLabel)
				statusBox.Layout = &VBoxWithSpacing{Spacing: StatusRowSpacing}
				cont.Add(statusBox)
			}
		case 6:
			noteLabel := widget.NewLabel(mod.Note)
			noteLabel.Wrapping = fyne.TextWrapWord
			cont.Add(noteLabel)
		}
	}
	app.systemModsTable = widget.NewTable(
		func() (int, int) { return len(app.systemMods), TableColumnCount },
		func() fyne.CanvasObject { return createTableRow(TableRowHeight) },
		systemUpdateCell,
	)
	ApplyTableColumnWidths(app.systemModsTable)
	app.systemModsTable.SetColumnWidth(0, 0)

	app.systemModsTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.systemMods) {
			mod := &app.systemMods[id.Row]
			app.selectedModName = mod.Name
			app.selectedModIndex.Store(-1)
			app.updateDescriptionForMod(mod.Name)
			app.scheduleEnrich(mod)
			app.updateUpDownButtons()
			app.systemModsTable.Refresh()
			app.modTable.UnselectAll()
		}
	}

	sysHeight := float32(SystemTableHeight)
	sysSpacer := canvas.NewRectangle(color.Transparent)
	sysSpacer.SetMinSize(fyne.NewSize(1, sysHeight))
	systemTableContainer := container.NewStack(sysSpacer, app.systemModsTable)
	if !app.cfg.ShowSystemMods {
		systemTableContainer.Hide()
	}
	app.systemModsTableContainer = systemTableContainer

	// Основная таблица модов
	updateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.displayedMods) {
			return
		}
		mod := &app.displayedMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		th := app.myApp.Settings().Theme()
		variant := app.myApp.Settings().ThemeVariant()
		var bgColor color.Color = color.Transparent
		baseBG := th.Color(themes.ColorTableRowEven, variant)
		if id.Row%2 == 1 {
			baseBG = th.Color(themes.ColorTableRowOdd, variant)
		}
		if id.Row == int(app.selectedModIndex.Load()) {
			bgColor = th.Color(themes.ColorTableRowSelected, variant)
		} else if mod.HasUpdate {
			bgColor = th.Color(themes.ColorTableHasUpdateMod, variant)
		} else if mod.Obsolete {
			bgColor = th.Color(themes.ColorTableObsoleteMod, variant)
		} else if mod.MissingFolder {
			bgColor = th.Color(themes.ColorTableMissingFolder, variant)
		} else if mod.Incompatible {
			bgColor = th.Color(themes.ColorTableRowConflict, variant)
		} else if mod.IsSymlink {
			bgColor = th.Color(themes.ColorStatusSymlinkBg, variant)
		} else {
			bgColor = baseBG
		}
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
		case 0:
			if app.showSelectColumn && !mod.IsSystem {
				cellBg := canvas.NewRectangle(th.Color(themes.ColorButtonShadow, variant))
				bgStack := []fyne.CanvasObject{}
				if app.selectColumnBgRes != nil {
					img := canvas.NewImageFromResource(app.selectColumnBgRes)
					img.FillMode = canvas.ImageFillStretch
					img.Translucency = 0.8
					bgStack = append(bgStack, img)
				} else {
					bgStack = append(bgStack, cellBg)
				}

				check := widget.NewCheck("", nil)
				check.SetChecked(mod.Selected)
				check.OnChanged = func(b bool) {
					mod.Selected = b
					if orig := app.findModByName(mod.Name); orig != nil {
						orig.Selected = b
					}
					if b {
						app.modTable.Select(widget.TableCellID{Row: id.Row, Col: 0})
					} else {
						if app.selectedModName == mod.Name {
							var newSelRow int = -1
							for i, dm := range app.displayedMods {
								if dm.Selected && dm.Name != mod.Name {
									newSelRow = i
									break
								}
							}
							if newSelRow >= 0 {
								app.modTable.Select(widget.TableCellID{Row: newSelRow, Col: 0})
							} else {
								app.modTable.UnselectAll()
								app.selectedModName = ""
								app.selectedModIndex.Store(-1)
								app.updateDescriptionForMod("")
								app.updateUpDownButtons()
							}
						}
					}
					app.modTable.Refresh()
				}
				bgStack = append(bgStack, check)
				cont.Add(container.NewStack(bgStack...))
			}
		case 1:
			if !mod.IsSystem {
				check := widget.NewCheck("", nil)
				check.SetChecked(mod.Active)
				if mod.MissingFolder {
					check.Disable() // блокируем, если папки нет
				}
				check.OnChanged = func(b bool) {
					app.toggleModActive(mod.Name, b)
					app.modTable.Select(widget.TableCellID{Row: id.Row, Col: 0})
				}
				cont.Add(check)
			}
		case 2:
			if mod.IsSystem {
				cont.Add(widget.NewLabel(""))
			} else {
				numText := canvas.NewText(fmt.Sprintf("%2d", id.Row+1), th.Color(theme.ColorNameForeground, variant))
				numText.Alignment = fyne.TextAlignCenter
				cont.Add(numText)
			}
		case 3:
			display := mod.DisplayName
			if display == "" {
				display = mod.Name
			}
			nameLabel := widget.NewLabel(display)
			if id.Row == int(app.selectedModIndex.Load()) {
				nameLabel.TextStyle = fyne.TextStyle{Bold: true}
			}
			cont.Add(nameLabel)
		case 4:
			dateStr := app.formatDate(mod.ModTime, app.cfg.DateFormat)
			dateText := canvas.NewText(dateStr, th.Color(theme.ColorNameForeground, variant))
			dateText.Alignment = fyne.TextAlignCenter
			cont.Add(dateText)
		case 5:
			var mainStatusText string
			var mainStatusColor color.Color
			var subStatusText string
			var subStatusColor color.Color

			// Основной статус (active/inactive)
			if mod.Active {
				mainStatusText = app.messages["status_active"]
				mainStatusColor = th.Color(themes.ColorStatusActive, variant)
			} else {
				mainStatusText = app.messages["status_inactive"]
				mainStatusColor = th.Color(themes.ColorStatusInactive, variant)
			}

			if mod.HasUpdate {
				subStatusText = app.messages["status_update_available"]
				subStatusColor = th.Color(theme.ColorNamePrimary, variant)
			} else {
				// Дополнительный статус
				switch {
				case mod.MissingFolder:
					subStatusText = app.messages["status_missing_folder"]
					subStatusColor = th.Color(themes.ColorStatusMissing, variant)
				case mod.VortexDeployed:
					subStatusText = app.messages["status_vortex"]
					subStatusColor = th.Color(themes.ColorStatusVortex, variant)
				case mod.IsSymlink:
					subStatusText = app.messages["status_symlink"]
					subStatusColor = th.Color(themes.ColorStatusSymlink, variant)
				case mod.IsSystem:
					subStatusText = app.messages["status_system"]
					subStatusColor = th.Color(themes.ColorStatusSystem, variant)
				case mod.Broken:
					subStatusText = app.messages["desc_broken"]
					subStatusColor = th.Color(themes.ColorStatusBroken, variant)
				case mod.Incompatible:
					subStatusText = app.messages["desc_conflict"]
					subStatusColor = th.Color(themes.ColorStatusConflict, variant)
				case mod.Obsolete:
					subStatusText = app.messages["desc_obsolete"]
					subStatusColor = th.Color(themes.ColorStatusObsolete, variant)
				case mod.Mandatory && mod.Active:
					subStatusText = app.messages["status_mandatory"]
					subStatusColor = th.Color(themes.ColorStatusMandatory, variant)
				case mod.Source == "manual":
					subStatusText = app.messages["status_manual"]
					subStatusColor = th.Color(themes.ColorStatusManual, variant)
				case mod.Source == "nexus":
					subStatusText = app.messages["status_nexus"]
					subStatusColor = th.Color(themes.ColorStatusNexus, variant)
				default:
					subStatusText = ""
				}
			}

			// Создаём вертикальный контейнер
			mainLabel := canvas.NewText(mainStatusText, mainStatusColor)
			mainLabel.TextSize = StatusFontSize + 2
			mainLabel.Alignment = fyne.TextAlignCenter
			mainLabel.TextStyle = fyne.TextStyle{Bold: true}

			subLabel := canvas.NewText(subStatusText, subStatusColor)
			subLabel.TextSize = StatusFontSize
			subLabel.Alignment = fyne.TextAlignCenter

			if subStatusText == "" {
				cont.Add(mainLabel)
			} else {
				// Используем кастомный layout с точным отступом
				statusBox := container.NewWithoutLayout(mainLabel, subLabel)
				statusBox.Layout = &VBoxWithSpacing{Spacing: StatusRowSpacing}
				cont.Add(statusBox)
			}
		case 6:
			noteLabel := widget.NewLabel(mod.Note)
			noteLabel.Wrapping = fyne.TextWrapOff
			noteScroll := container.NewScroll(noteLabel)
			noteScroll.SetMinSize(fyne.NewSize(0, 35))
			cont.Add(noteScroll)
		}
	}

	app.modTable = widget.NewTable(
		func() (int, int) { return len(app.displayedMods), TableColumnCount },
		func() fyne.CanvasObject { return createTableRow(TableRowHeight) },
		updateCell,
	)
	ApplyTableColumnWidths(app.modTable)
	app.modTable.SetColumnWidth(0, 0)

	app.modTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.displayedMods) {
			app.selectedModName = app.displayedMods[id.Row].Name
			app.selectedModIndex.Store(int32(id.Row))
			app.updateDescriptionForMod(app.selectedModName)
			app.scheduleEnrich(&app.displayedMods[id.Row])
			app.updateUpDownButtons()
			app.modTable.Refresh()
		}
	}

	// Рамка таблицы
	app.tableBorder = canvas.NewRectangle(color.Transparent)
	app.tableBorder.StrokeWidth = 2
	app.tableBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
	app.tableBorder.FillColor = color.Transparent
	app.tableBorder.Hide()
	// Фоновое изображение таблицы
	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	var mechBg *canvas.Image
	if mechData != nil {
		mechBg = canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechBg.FillMode = canvas.ImageFillContain // ImageFillStretch
		mechBg.Translucency = TableBackgroundOpacity
	}

	if mechBg != nil {
		app.tableBorderContainer = container.NewStack(mechBg, app.modTable, app.tableBorder)
	} else {
		app.tableBorderContainer = container.NewStack(app.modTable, app.tableBorder)
	}

	// Нижняя панель
	app.counterLabel = widget.NewLabel("")
	app.profileLabel = widget.NewLabel(app.messages["profile_label"])
	app.profileLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Выпадающий список профилей
	app.profileSelect = widget.NewSelect([]string{}, func(s string) {
		if s != app.cfg.ActiveProfile {
			app.switchProfile(s)
		}
	})
	app.profileSelect.PlaceHolder = app.messages["profile_select_placeholder"]

	bottomContent := container.NewHBox(
		app.counterLabel,
		layout.NewSpacer(),
		app.profileLabel,
		app.profileSelect,
	)

	bottomPanel := container.NewBorder(
		nil, nil, nil, nil,
		bottomContent,
	)

	// Левая панель
	modsArea := container.NewBorder(
		container.NewVBox(
			topPanelWithBg,
			app.managePanel,
			app.headerTable,
		),
		nil, nil, nil,
		container.NewBorder(
			container.NewVBox(systemTableContainer),
			nil, nil, nil,
			app.tableBorderContainer,
		),
	)

	leftPanel := container.NewBorder(
		nil,
		bottomPanel,
		nil, nil,
		modsArea,
	)

	// Описание в карточке
	app.descTitle = canvas.NewText(app.messages["select_mod"], th.Color(theme.ColorNameForeground, variant))
	app.descTitle.TextSize = theme.TextSize() + 2
	app.descTitle.TextStyle = fyne.TextStyle{Bold: true}

	// Кнопка открытия папки мода
	// Загрузка иконки папки
	folderImgData, err := embeddedFiles.ReadFile("assets/buttons/folder_open.png")
	if err != nil {
		app.appendLog("Could not load folder icon: " + err.Error())
	}
	folderRes := fyne.NewStaticResource("folder", folderImgData)

	app.openFolderBtn = NewIconButton(folderRes, func() {
		if app.selectedModName == "" {
			return
		}
		mod := app.findModByName(app.selectedModName)
		if mod == nil || mod.MissingFolder {
			return
		}
		modPath := filepath.Join(app.cfg.ModsPath, mod.Name)
		if _, err := os.Stat(modPath); err == nil {
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "windows":
				cmd = exec.Command("explorer", modPath)
			case "linux":
				cmd = exec.Command("xdg-open", modPath)
			case "darwin":
				cmd = exec.Command("open", modPath)
			default:
				u, _ := url.Parse("file://" + filepath.ToSlash(modPath))
				_ = app.myApp.OpenURL(u)
				return
			}
			if cmd != nil {
				cmd.Start()
			}
		}
	})
	app.openFolderBtn.Importance = widget.MediumImportance
	app.openFolderBtn.SetToolTip(app.messages["open_mod_folder_tooltip"])

	app.descAuthor = widget.NewLabel("-")
	app.descInstalled = widget.NewLabel("")
	app.descBody = widget.NewLabel(app.messages["desc_placeholder"])
	app.descBody.Wrapping = fyne.TextWrapWord
	app.descURL = widget.NewHyperlink("", nil)

	th, variant = app.myApp.Settings().Theme(), app.myApp.Settings().ThemeVariant()
	app.descCardBgRect = canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	app.descCardBgRect.CornerRadius = 12
	app.descCardBgRect.StrokeWidth = 0.5
	app.descCardBgRect.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)
	descCardBg := app.descCardBgRect

	app.githubLink = widget.NewHyperlink("", nil)
	app.githubLink.Alignment = fyne.TextAlignLeading

	app.descLocalVersion = widget.NewLabel("")
	app.descLatestVersion = widget.NewLabel("")
	app.descLastUpdated = widget.NewLabel("")
	app.descOriginalUpload = widget.NewLabel("")
	app.descConflict = widget.NewLabel("")
	app.descConflict.Wrapping = fyne.TextWrapWord
	app.descConflict.Hide()

	app.btnRemove = NewIconButton(trashRes, func() {
		if app.selectedModName == "" {
			return
		}
		modName := app.selectedModName
		mod := app.findModByName(modName)
		if mod == nil || mod.IsSystem {
			app.appendLog(app.messages["log_cannot_delete_system"])
			return
		}

		var nextModName string
		for i, m := range app.displayedMods {
			if m.Name == modName {
				if i+1 < len(app.displayedMods) {
					nextModName = app.displayedMods[i+1].Name
				} else if i-1 >= 0 {
					nextModName = app.displayedMods[i-1].Name
				}
				break
			}
		}

		app.showConfirmDialog(
			app.messages["confirm_delete_title"],
			fmt.Sprintf(app.messages["confirm_delete_text"], mod.Name),
			func() {
				checks.RemoveMod(modName)
				app.removeModFromCache(modName)
				oldIndex, _ := app.removeModFromData(modName)

				app.updateModCounter()
				app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
				app.modTable.Refresh()
				app.updateTableBorder()
				app.appendLog(fmt.Sprintf(app.messages["log_deleted"], modName))

				app.saveCurrentOrder()
				app.syncProfileFromGame()
				app.orderDirty = false
				app.updateTableBorder()

				// Восстановление выделения
				if nextModName != "" {
					for i, m := range app.displayedMods {
						if m.Name == nextModName {
							app.modTable.Select(widget.TableCellID{Row: i, Col: 0})
							app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
							break
						}
					}
				} else if len(app.displayedMods) > 0 {
					newIndex := oldIndex
					if newIndex >= len(app.displayedMods) {
						newIndex = len(app.displayedMods) - 1
					}
					if newIndex >= 0 {
						app.modTable.Select(widget.TableCellID{Row: newIndex, Col: 0})
						app.modTable.ScrollTo(widget.TableCellID{Row: newIndex, Col: 0})
					}
				} else {
					app.selectedModName = ""
					app.selectedModIndex.Store(-1)
					app.updateDescriptionForMod("")
					app.updateUpDownButtons()
				}
			},
		)
	})
	app.btnRemove.SetToolTip(app.messages["btn_remove_tooltip"])

	// Загрузка иконки обновления
	updateImgData, err := embeddedFiles.ReadFile("assets/buttons/upd_download_blue_p.png")
	if err != nil {
		app.appendLog("Could not load update icon: " + err.Error())
	}
	updateRes := fyne.NewStaticResource("update", updateImgData)

	app.btnUpdateMod = NewIconButton(updateRes, func() {
		if app.selectedModName == "" {
			return
		}
		mod := app.findModByName(app.selectedModName)
		if mod == nil {
			return
		}
		if mod.URL == "" {
			app.appendLog(app.messages["update_no_url"])
			return
		}
		// Все длительные операции - в фоновой горутине
		go func() {
			if mod.Name == "base" {
				app.updateDML()
				return
			}
			if mod.Name == "dmf" {
				app.updateDMF()
				return
			}
			if mod.Name == "autopatch" {
				app.updateAutopatcher()
				return
			}
			if mod.IsSystem {
				app.appendLog(app.messages["log_cannot_update_system"])
				return
			}
			app.updateModFromNexus(mod, false)
		}()
	})
	app.btnUpdateMod.SetToolTip(app.messages["btn_update_mod_premium_only"])

	// Инициализация контейнера для дополнительного содержимого (спойлер со списком обновлений)
	app.descExtraContainer = container.NewVBox()

	// Отступ шириной 30px
	leftPadding := canvas.NewRectangle(color.Transparent)
	leftPadding.SetMinSize(fyne.NewSize(30, 1))

	// Строка: название слева, кнопки справа
	headerRow := container.NewHBox(
		leftPadding,
		app.descTitle,
		layout.NewSpacer(),
		widget.NewSeparator(),
		app.btnUpdateMod,
		app.openFolderBtn,
		app.btnRemove,
	)

	titleSep := canvas.NewRectangle(th.Color(themes.ColorCRTScreenStroke, variant))
	titleSep.SetMinSize(fyne.NewSize(0, 3))

	descHeader := container.NewBorder(
		nil, nil, nil, nil,
		container.NewVBox(
			headerRow,
			titleSep,
			app.descAuthor,
			widget.NewSeparator(),
			container.NewHBox(widget.NewLabel(""), app.descURL, widget.NewLabel("  "), app.githubLink),
			widget.NewSeparator(),
			container.NewHBox(widget.NewLabel(""), app.descLocalVersion),
			widget.NewSeparator(),
			app.descConflict,
		),
	)
	// Отступ перед спойлером
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, 20))

	descCardContent := container.NewVBox(
		descHeader,
		widget.NewSeparator(),
		app.descBody,
		spacer,
		app.descExtraContainer,
	)

	descCardScroll := container.NewScroll(descCardContent)
	descCardScroll.SetMinSize(fyne.NewSize(DescScrollMinWidth, DescScrollMinHeight))
	descCard := container.NewStack(
		descCardBg,
		container.NewPadded(descCardScroll),
	)

	app.descCardContent = descCardContent
	app.descCardScroll = descCardScroll

	rightContent := container.NewVSplit(descCard, app.consoleScroll)
	rightContent.Offset = 0.65
	rightPanel := container.NewBorder(nil, nil, nil, nil, rightContent)
	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = SplitOffset
	content := container.NewBorder(nil, nil, nil, nil, split)
	app.mainWindow.SetContent(content)

	app.appendCenteredLog(app.messages["log_start0"])
	app.filterModList()
	app.updateTableBorder()
}

func (app *App) refreshThemeColors() {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	if app.screenBgRect != nil {
		app.screenBgRect.FillColor = th.Color(themes.ColorCRTScreenFill, variant)
		app.screenBgRect.StrokeColor = th.Color(themes.ColorCRTScreenStroke, variant)
		app.screenBgRect.Refresh()
	}
	if app.headerBoxBgRect != nil {
		app.headerBoxBgRect.FillColor = th.Color(themes.ColorCRTHeaderBg, variant)
		app.headerBoxBgRect.Refresh()
	}
	if app.logHeaderText != nil {
		app.logHeaderText.Color = th.Color(themes.ColorConsoleText, variant)
		app.logHeaderText.Refresh()
	}
	if app.tipBgRect != nil {
		app.tipBgRect.FillColor = th.Color(themes.ColorTipBg, variant)
		app.tipBgRect.Refresh()
	}
	if app.topPanelBgRect != nil {
		app.topPanelBgRect.FillColor = th.Color(themes.ColorTopPanelBg, variant)
		app.topPanelBgRect.Refresh()
	}
	if app.managePanelBgRect != nil {
		app.managePanelBgRect.FillColor = th.Color(themes.ColorManagePanelBg, variant)
		app.managePanelBgRect.Refresh()
	}
	if app.descCardBgRect != nil {
		app.descCardBgRect.FillColor = th.Color(themes.ColorDescCardBg, variant)
		app.descCardBgRect.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)
		app.descCardBgRect.Refresh()
	}
	if app.tableBorder != nil {
		app.tableBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
		app.tableBorder.Refresh()
	}

	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.systemModsTable != nil {
		app.systemModsTable.Refresh()
	}
	if app.modTable != nil {
		app.modTable.Refresh()
	}

	// Принудительное обновление таблиц (изменение ширины колонки 0 заставляет пересоздать ячейки)
	for _, tbl := range []*widget.Table{app.headerTable, app.systemModsTable, app.modTable} {
		if tbl == nil {
			continue
		}
		// Сохраняем текущую ширину колонки 0 (если GetColumnWidth не работает, используем константу)
		// Вместо GetColumnWidth просто устанавливаем ширину в 1, затем в 0, чтобы вызвать перерисовку
		tbl.SetColumnWidth(0, 1)
		tbl.SetColumnWidth(0, 0)
		tbl.Refresh()
	}

	// Обновляем стиль тултипа
	app.updateTooltipStyle()

	for _, btn := range []*CustomButton{
		app.btnSaveOrder, app.btnRefresh, app.btnInstall, app.btnRemove,
		app.btnUp, app.btnDown, app.btnSortChecks, app.btnToggle,
		app.btnLaunchNormal, app.btnLaunchNoLauncher,
		app.moveToTopBtn, app.moveToBottomBtn, app.btnAMLConfig,
		app.selectAllBtn, app.deselectAllBtn, app.enableSelectedBtn,
		app.disableSelectedBtn, app.enableAllBtn, app.disableAllBtn, app.btnEditVersion,
		app.manageBtn, app.searchClearBtn, app.btnRemoveAll, app.btnRemoveSelected,
	} {
		if btn != nil {
			btn.Refresh()
		}
	}
}

func (app *App) appendLog(text string) {
	if app.logWindow == nil {
		if app.logFile != nil {
			fmt.Fprintln(app.logFile, time.Now().Format(LogTimeFormat), text)
		}
		return
	}
	fyne.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				if app.logFile != nil {
					fmt.Fprintf(app.logFile, "PANIC in appendLog: %v\n", r)
				}
			}
		}()
		seg := &widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: themes.ColorConsoleText,
				TextStyle: fyne.TextStyle{},
			},
			Text: text,
		}
		app.logWindow.Segments = append(app.logWindow.Segments, seg)
		app.logWindow.Refresh()
		if app.consoleScroll != nil {
			app.consoleScroll.ScrollToBottom()
		}
	})
	if app.logFile != nil {
		fmt.Fprintln(app.logFile, time.Now().Format(LogTimeFormat), text)
	}
}

func (app *App) appendCenteredLog(text string) {
	fyne.Do(func() {
		if app.logHeaderText != nil {
			app.logHeaderText.Text = text
			app.logHeaderText.Refresh()
		}
	})
}

func (app *App) updateDescriptionForMod(name string) {
	// Если мод не выбран — очищаем описание
	if name == "" {
		app.descTitle.Text = app.messages["select_mod"]
		app.descTitle.Refresh()
		app.descAuthor.SetText("-")
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
		app.descBody.SetText(app.messages["desc_placeholder"])
		if app.descExtraContainer != nil {
			app.descExtraContainer.Objects = nil
			app.descExtraContainer.Refresh()
		}
		// Обновляем контейнеры, чтобы пересчитать layout
		if app.descCardContent != nil {
			app.descCardContent.Refresh()
		}
		if app.descCardScroll != nil {
			app.descCardScroll.Refresh()
		}
		return
	}

	mod := app.findModByName(name)
	if mod == nil {
		return
	}

	// Обновляем состояние кнопки открытия папки
	if app.openFolderBtn != nil {
		if mod.MissingFolder || mod.Name == "" {
			app.openFolderBtn.Disable()
		} else {
			app.openFolderBtn.Enable()
		}
	}

	// --- Название мода ---
	display := mod.DisplayName
	if display == "" {
		display = mod.Name
	}
	app.descTitle.Text = display
	app.descTitle.Refresh()

	// --- Автор с датой Original Upload (если есть) ---
	author := mod.Author
	if author == "" {
		author = app.messages["author_unknown"]
	}
	authorText := fmt.Sprintf(app.messages["author_label"], author)
	if !mod.OriginalUpload.IsZero() {
		authorText += fmt.Sprintf("          %s: %s", app.messages["original_upload_label"], app.formatDate(mod.OriginalUpload, app.cfg.DateFormat))
	}
	app.descAuthor.SetText(authorText)

	// --- Дата установки (Installed) ---
	app.descInstalled.SetText(fmt.Sprintf(app.messages["installed_label"], app.formatDate(mod.ModTime, app.cfg.DateFormat)))

	// --- Локальная версия с датой Last Updated ---
	if app.descLocalVersion != nil {
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

		if cacheKey != "" {
			if info, ok := app.getCachedVersion(cacheKey); ok && info.Version != "" {
				// Начинаем с локальной версии
				localText := fmt.Sprintf(app.messages["nexus_local_version_label"], info.Version)

				// Добавляем Latest (если есть)
				if latest, ok := app.getLatestVersion(cacheKey); ok {
					localText += "          " + fmt.Sprintf(app.messages["nexus_latest_version_label"], latest)
				}

				// Добавляем Last Updated (если есть)
				if !mod.LastUpdated.IsZero() {
					localText += fmt.Sprintf("          %s: %s", app.messages["last_updated_label"], app.formatDate(mod.LastUpdated, app.cfg.DateFormat))
				}

				app.descLocalVersion.SetText(localText)
			} else {
				app.descLocalVersion.SetText(app.messages["nexus_local_version_unknown"])
			}
		} else {
			app.descLocalVersion.SetText("")
		}
	}

	// --- Last Updated (отдельная строка) ---
	if app.descLastUpdated != nil {
		if !mod.LastUpdated.IsZero() {
			app.descLastUpdated.SetText(fmt.Sprintf("Last updated: %s", app.formatDate(mod.LastUpdated, app.cfg.DateFormat)))
		} else {
			app.descLastUpdated.SetText("")
		}
	}

	// --- Original Upload (отдельная строка) ---
	if app.descOriginalUpload != nil {
		if !mod.OriginalUpload.IsZero() {
			app.descOriginalUpload.SetText(fmt.Sprintf("Original upload: %s", app.formatDate(mod.OriginalUpload, app.cfg.DateFormat)))
		} else {
			app.descOriginalUpload.SetText("")
		}
	}

	// --- Последняя версия (Latest) — без даты (она уже есть в локальной) ---
	if app.descLatestVersion != nil {
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

		if cacheKey != "" {
			if latest, ok := app.getLatestVersion(cacheKey); ok {
				app.descLatestVersion.SetText(fmt.Sprintf(app.messages["nexus_latest_version_label"], latest))
			} else {
				app.descLatestVersion.SetText(app.messages["nexus_latest_version_unknown"])
			}
		} else {
			app.descLatestVersion.SetText("")
		}
	}

	// --- Описание ---
	desc := strings.TrimSpace(mod.Description)
	if mod.MissingFolder {
		desc = app.messages["desc_missing"] + desc
	}
	if desc == "" || desc == "{" || desc == "}" || desc == "[]" || desc == "()" {
		desc = app.messages["desc_placeholder"]
	}
	app.descBody.SetText(desc)

	// --- Ссылка на мод (Nexus) ---
	if mod.URL != "" {
		if u, err := url.Parse(mod.URL); err == nil {
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

	// --- Ссылка на GitHub (если есть) ---
	if app.githubLink != nil {
		if mod.GitHubURL != "" {
			if u, err := url.Parse(mod.GitHubURL); err == nil {
				app.githubLink.SetURL(u)
				app.githubLink.SetText(app.messages["source_code_url"])
			} else {
				app.githubLink.SetURL(nil)
				app.githubLink.SetText("")
			}
		} else {
			app.githubLink.SetURL(nil)
			app.githubLink.SetText("")
		}
	}

	// --- Конфликты ---
	if mod.Incompatible {
		for _, pair := range checks.IncompatiblePairs {
			if pair.Mod1 == mod.Name || pair.Mod2 == mod.Name {
				other := pair.Mod1
				if other == mod.Name {
					other = pair.Mod2
				}
				if !checks.FolderExists(other) {
					continue
				}
				if desc := checks.GetIncompatibleDesc(pair.Mod1, pair.Mod2); desc != "" {
					app.descConflict.SetText(desc)
				} else {
					app.descConflict.SetText("")
				}
				app.descConflict.Show()
				break
			}
		}
	} else {
		app.descConflict.Hide()
		app.descConflict.SetText("")
	}

	// --- Спойлер со списком изменений (changelog) ---
	if app.descExtraContainer != nil {
		app.descExtraContainer.Objects = nil
		if mod.URL != "" {
			modID := helpers.ExtractModIDFromURL(mod.URL)
			if modID != 0 {
				key := fmt.Sprintf("%d:%s", modID, mod.Name)
				app.changelogMutex.RLock()
				savedText, hasText := app.changelogTexts[key]
				expanded, hasExpanded := app.changelogExpanded[key]
				app.changelogMutex.RUnlock()

				changelogLabel := widget.NewLabel(app.messages["downloading_changelog"])
				changelogLabel.Wrapping = fyne.TextWrapWord
				changelogContainer := container.NewVBox(changelogLabel)
				changelogContainer.Hide()

				if hasText && savedText != "" && savedText != app.messages["downloading_changelog"] {
					changelogLabel.SetText(savedText)
				}
				if hasExpanded && expanded {
					changelogContainer.Show()
				}

				btnState := struct {
					expanded bool
					btn      *widget.Button
				}{}
				btnState.btn = widget.NewButton(app.messages["btn_show_changelog"], func() {
					if !btnState.expanded {
						if changelogLabel.Text == app.messages["downloading_changelog"] {
							go func() {
								fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
								if err != nil {
									fyne.Do(func() {
										changelogLabel.SetText(app.messages["changelog_load_failed"])
									})
									return
								}
								changelog, err := app.FetchChangelog(modID, fileInfo.ID)
								clean := app.messages["changelog_unavailable"]
								if err == nil && changelog != "" {
									clean = stripHTML(changelog)
								}
								fyne.Do(func() {
									app.changelogMutex.Lock()
									app.changelogTexts[key] = clean
									app.changelogMutex.Unlock()
									changelogLabel.SetText(clean)
								})
							}()
						}
						btnState.expanded = true
						changelogContainer.Show()
						btnState.btn.SetText(app.messages["btn_hide_changelog"])
						app.changelogMutex.Lock()
						app.changelogExpanded[key] = true
						app.changelogMutex.Unlock()
					} else {
						btnState.expanded = false
						changelogContainer.Hide()
						btnState.btn.SetText(app.messages["btn_show_changelog"])
						app.changelogMutex.Lock()
						app.changelogExpanded[key] = false
						app.changelogMutex.Unlock()
					}
				})

				if hasExpanded && expanded {
					btnState.btn.SetText(app.messages["btn_hide_changelog"])
					btnState.expanded = true
				}

				app.descExtraContainer.Objects = []fyne.CanvasObject{
					widget.NewSeparator(),
					btnState.btn,
					changelogContainer,
				}
				app.descExtraContainer.Refresh()
			}
		} else {
			app.descExtraContainer.Refresh()
		}
	}

	// --- Принудительное обновление контейнеров для пересчёта layout ---
	if app.descCardContent != nil {
		app.descCardContent.Refresh()
	}
	if app.descCardScroll != nil {
		app.descCardScroll.Refresh()
	}
}

func (app *App) enrichModFromNexus(mod *checks.ModInfo) {
	if app.getAuthToken() == "" || mod.URL == "" {
		return
	}
	modID := helpers.ExtractModIDFromURL(mod.URL)
	if modID == 0 {
		return
	}
	go func() {
		defer func() { recover() }()
		var fileInfo *FileInfo
		var err error
		if mod.Name == "base" || mod.Name == "dmf" {
			fileInfo, err = app.getLatestFileInfo(modID)
		} else {
			fileInfo, err = app.getLatestFileInfoForMod(modID, mod.Name)
		}
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_cannot_get_file_info"], mod.Name, err))
			return
		}
		cacheKey := fmt.Sprintf("%d:%s", modID, mod.Name)
		app.setLatestVersion(cacheKey, fileInfo.Version)

		// Last Updated
		if fileInfo != nil && fileInfo.UploadedTimestamp > 0 {
			mod.LastUpdated = time.Unix(fileInfo.UploadedTimestamp, 0)
		}

		// Original Upload (самый старый файл)
		oldestInfo, err := app.getOldestFileInfo(modID)
		if err == nil && oldestInfo != nil && oldestInfo.UploadedTimestamp > 0 {
			mod.OriginalUpload = time.Unix(oldestInfo.UploadedTimestamp, 0)
		}

		// Синхронизируем с основным списком allMods
		app.modsMutex.Lock()
		for i := range app.allMods {
			if app.allMods[i].Name == mod.Name {
				app.allMods[i].LastUpdated = mod.LastUpdated
				app.allMods[i].OriginalUpload = mod.OriginalUpload
				break
			}
		}
		app.modsMutex.Unlock()

		if fileInfo.FileName != "" {
			entry := checks.GetModDBEntry(mod.Name)
			if entry != nil && entry.NexusFilePattern == "" {
				pattern := extractPatternFromFilename(fileInfo.FileName)
				if pattern != "" {
					entry.NexusFilePattern = pattern
					checks.UpdateModDBEntry(*entry)
					checks.SaveModDatabase()
					app.appendLog(fmt.Sprintf(app.messages["log_autosaved_stable_pattern"], mod.Name, pattern))
				}
			}
		}
		fyne.Do(func() {
			if app.selectedModName == mod.Name {
				app.updateDescriptionForMod(mod.Name)
			}
		})
	}()
}

func (app *App) updateToggleButtonText(btn *CustomButton) {
	switch app.patcherType {
	case PatcherAutoPatch:
		if isModsEnabledAutoPatch(app.gameRoot) {
			btn.icon = app.toggleOnIcon
		} else {
			btn.icon = app.toggleOffIcon
		}
	case PatcherLegacy:
		if isModsEnabledLegacy(app.gameRoot) {
			btn.icon = app.toggleOnIcon
		} else {
			btn.icon = app.toggleOffIcon
		}
	default:
		// btn.SetText(app.messages["btn_no_patcher"])
		btn.icon = app.toggleOffIcon
		btn.Disable()
		return
	}
	btn.text = ""
	btn.Enable()
	btn.Refresh()
}

func (app *App) updateUpDownButtons() {
	if app.selectedModName == "" {
		app.btnUp.Disable()
		app.btnDown.Disable()
		app.btnUp.Refresh()
		app.btnDown.Refresh()
		return
	}
	if mod := app.findModByName(app.selectedModName); mod != nil && mod.IsSystem {
		app.btnUp.Disable()
		app.btnDown.Disable()
		app.moveToTopBtn.Disable()
		app.moveToBottomBtn.Disable()
		app.btnUp.Refresh()
		app.btnDown.Refresh()
		app.moveToTopBtn.Refresh()
		app.moveToBottomBtn.Refresh()
		return
	}
	idx := -1
	for i, m := range app.displayedMods {
		if m.Name == app.selectedModName {
			idx = i
			break
		}
	}
	app.selectedModIndex.Store(int32(idx))
	if idx < 0 {
		app.btnUp.Disable()
		app.btnDown.Disable()
	} else {
		app.btnUp.Enable()
		app.btnDown.Enable()
		if idx == 0 {
			app.btnUp.Disable()
		}
		if idx == len(app.displayedMods)-1 {
			app.btnDown.Disable()
		}
	}
	app.btnUp.Refresh()
	app.btnDown.Refresh()
}

type modFilterFunc func(checks.ModInfo) bool

func (app *App) filterModList() {
	if app.modTable == nil {
		app.appendLog("filterModList: modTable is nil, skipping")
		return
	}
	if app.filterSelect == nil {
		app.appendLog("filterModList: filterSelect is nil, using all mods")
		app.displayedMods = app.allMods
		app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
		if app.selectedModName != "" {
			for i, m := range app.displayedMods {
				if m.Name == app.selectedModName {
					app.selectedModIndex.Store(int32(i))
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0})
					break
				}
			}
		} else {
			app.selectedModIndex.Store(-1)
		}
		app.modTable.Refresh()
		activeCount := 0
		for _, m := range app.displayedMods {
			if m.Active {
				activeCount++
			}
		}
		if app.counterLabel != nil {
			app.counterLabel.SetText(fmt.Sprintf(app.messages["mods_counter"], len(app.displayedMods), len(app.allMods), activeCount))
		}
		app.forceRefreshTable()
		return
	}

	predicates := map[string]modFilterFunc{
		app.messages["filter_all"]:        func(m checks.ModInfo) bool { return true },
		app.messages["filter_active"]:     func(m checks.ModInfo) bool { return m.Active },
		app.messages["filter_inactive"]:   func(m checks.ModInfo) bool { return !m.Active },
		app.messages["filter_obsolete"]:   func(m checks.ModInfo) bool { return m.Obsolete },
		app.messages["filter_conflict"]:   func(m checks.ModInfo) bool { return m.Incompatible },
		app.messages["filter_missing"]:    func(m checks.ModInfo) bool { return m.MissingFolder },
		app.messages["filter_has_update"]: func(m checks.ModInfo) bool { return m.HasUpdate },
	}
	filter := app.filterSelect.Selected
	if filter == "" {
		filter = app.messages["filter_all"]
	}
	filterFn, ok := predicates[filter]
	if !ok {
		filterFn = predicates[app.messages["filter_all"]]
	}
	search := strings.ToLower(app.searchEntry.Text)
	app.displayedMods = nil
	for _, mod := range app.allMods {
		if search != "" {
			dn := strings.ToLower(mod.DisplayName)
			if !strings.Contains(strings.ToLower(mod.Name), search) && !strings.Contains(dn, search) {
				continue
			}
		}
		if filterFn(mod) {
			app.displayedMods = append(app.displayedMods, mod)
		}
	}
	app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
	if app.selectedModName != "" {
		found := false
		for i, m := range app.displayedMods {
			if m.Name == app.selectedModName {
				app.selectedModIndex.Store(int32(i))
				found = true
				break
			}
		}
		if !found {
			app.selectedModIndex.Store(-1)
			app.selectedModName = ""
		}
	} else {
		app.selectedModIndex.Store(-1)
	}
	app.modTable.Refresh()
	selIdx := app.selectedModIndex.Load()
	if selIdx >= 0 {
		app.modTable.ScrollTo(widget.TableCellID{Row: int(selIdx), Col: 0})
	} else {
		app.modTable.ScrollToTop()
	}
	app.updateUpDownButtons()
	activeCount := 0
	for _, m := range app.displayedMods {
		if m.Active {
			activeCount++
		}
	}
	if app.counterLabel != nil {
		app.counterLabel.SetText(fmt.Sprintf(app.messages["mods_counter"], len(app.displayedMods), len(app.allMods), activeCount))
	}
	app.forceRefreshTable()
}

func (app *App) filterOptions() []string {
	return []string{
		app.messages["filter_all"],
		app.messages["filter_active"],
		app.messages["filter_inactive"],
		app.messages["filter_obsolete"],
		app.messages["filter_conflict"],
		app.messages["filter_missing"],
		app.messages["filter_has_update"],
	}
}

// Выделение всех модов, с учётом фильтра
func (app *App) selectAllMods(selected bool) {
	app.modsMutex.Lock()
	defer app.modsMutex.Unlock()
	visibleNames := make(map[string]bool)
	for _, mod := range app.displayedMods {
		visibleNames[mod.Name] = true
	}
	for i := range app.allMods {
		if visibleNames[app.allMods[i].Name] {
			app.allMods[i].Selected = selected
		}
	}
	// UI-обновление после разблокировки
	fyne.Do(func() {
		app.filterModList()
	})
}

func (app *App) setSelectedActive(active bool) {
	app.modsMutex.Lock()
	changed := false
	for i := range app.allMods {
		if app.allMods[i].Selected && !app.allMods[i].IsSystem {
			if app.allMods[i].Active != active {
				app.allMods[i].Active = active
				changed = true
			}
		}
	}
	app.modsMutex.Unlock()
	if changed {
		app.orderDirty = true
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
			app.forceRefreshTable()
		})
	}
}

func (app *App) setAllModsActive(active bool) {
	app.modsMutex.Lock()
	changed := false
	for i := range app.allMods {
		if !app.allMods[i].IsSystem {
			if app.allMods[i].Active != active {
				app.allMods[i].Active = active
				changed = true
			}
		}
	}
	app.modsMutex.Unlock()
	if changed {
		app.orderDirty = true
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
		})
	}
}

func (app *App) startBlink(btn *CustomButton, activeFlag *bool, condition func() bool) {
	if *activeFlag {
		return
	}
	*activeFlag = true
	go func() {
		for *activeFlag && condition() {
			fyne.Do(func() {
				btn.Importance = widget.WarningImportance
				btn.Refresh()
			})
			time.Sleep(BlinkOnDuration)
			fyne.Do(func() {
				btn.Importance = widget.MediumImportance
				btn.Refresh()
			})
			time.Sleep(BlinkOffDuration)
		}
		fyne.Do(func() {
			btn.Importance = widget.MediumImportance
			btn.Refresh()
		})
	}()
}

func (app *App) startBlinkSaveButton() {
	app.startBlink(app.btnSaveOrder, &app.blinkSaveOrderActive, func() bool {
		return app.orderDirty
	})
}

func (app *App) stopBlinkSaveButton() {
	app.blinkSaveOrderActive = false
}

func (app *App) updateTableBorder() {
	if app.tableBorder == nil {
		return
	}
	if app.orderDirty {
		app.tableBorder.Show()
		if !app.blinkSaveOrderActive {
			app.startBlinkSaveButton()
		}
	} else {
		app.tableBorder.Hide()
		app.stopBlinkSaveButton()
	}
}

func (app *App) scheduleEnrich(mod *checks.ModInfo) {
	if app.enrichDebounce != nil {
		app.enrichDebounce.Stop()
	}
	app.enrichDebounce = time.AfterFunc(1500*time.Millisecond, func() {
		app.enrichModFromNexus(mod)
	})
}

// selectAndScrollToMod выделяет мод в таблице и прокручивает к нему
func (app *App) selectAndScrollToMod(modName string) {
	if modName == "" {
		return
	}
	// Небольшая задержка, чтобы таблица успела перестроиться после refreshModList()
	time.AfterFunc(50*time.Millisecond, func() {
		fyne.Do(func() {
			for i, m := range app.displayedMods {
				if m.Name == modName {
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0})
					app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
					return
				}
			}
		})
	})
}
