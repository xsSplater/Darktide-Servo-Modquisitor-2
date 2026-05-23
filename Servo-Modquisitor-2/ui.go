package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/themes"
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

func (app *App) buildUI() {
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
	app.filterLabel = widget.NewLabel(app.messages["filter_label"])

	// Статус-менеджер
	app.statusLabel = widget.NewLabel("")
	app.statusLabel.Alignment = fyne.TextAlignCenter
	app.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	app.tooltipStatus = NewTooltipStatusManager(app.statusLabel)

	app.tipBgRect = canvas.NewRectangle(th.Color(themes.ColorTipBg, variant))
	app.tipBgRect.CornerRadius = 6
	app.tipBgRect.SetMinSize(fyne.NewSize(500, 20))
	statusContainer := container.NewStack(app.tipBgRect, app.statusLabel)

	// Кнопки быстрого перемещения
		// Переместить моды наверх
	app.moveToTopBtn = NewCustomButton(app.messages["btn_move_to_top"], func() { app.moveSelectedToTop() })
	app.applyTooltip(app.moveToTopBtn, "btn_move_to_top_tooltip")
		// Переместить моды вниз
	app.moveToBottomBtn = NewCustomButton(app.messages["btn_move_to_bottom"], func() { app.moveSelectedToBottom() })
	app.applyTooltip(app.moveToBottomBtn, "btn_move_to_bottom_tooltip")
		// Переместить моды на строку номер...
	app.moveToEntry = widget.NewEntry()
	app.moveToEntry.SetPlaceHolder(app.messages["col_number"] + app.messages["col_number"])
	app.moveToEntry.OnSubmitted = func(text string) { app.moveSelectedToPosition() }
	app.moveLabel = widget.NewLabel(app.messages["lbl_move_to"])

	// Кнопки выделения и массовых операций
		// Выбрать все моды
	app.selectAllBtn = NewCustomButton(app.messages["btn_select_all"], func() { app.selectAllMods(true) })
	app.applyTooltip(app.selectAllBtn, "btn_select_all_tooltip")
		// Снять выделение со всех модов
	app.deselectAllBtn = NewCustomButton(app.messages["btn_deselect_all"], func() { app.selectAllMods(false) })
	app.applyTooltip(app.deselectAllBtn, "btn_deselect_all_tooltip")
		// Включить выбранные моды
	app.enableSelectedBtn = NewCustomButton(app.messages["btn_enable_selected"], func() { app.setSelectedActive(true) })
	app.applyTooltip(app.enableSelectedBtn, "btn_enable_selected_tooltip")
		// Отключить выбранные моды
	app.disableSelectedBtn = NewCustomButton(app.messages["btn_disable_selected"], func() { app.setSelectedActive(false) })
	app.applyTooltip(app.disableSelectedBtn, "btn_disable_selected_tooltip")
		// Включить все моды
	app.enableAllBtn = NewCustomButton(app.messages["btn_enable_all_mods"], func() { app.setAllModsActive(true) })
	app.applyTooltip(app.enableAllBtn, "btn_enable_all_tooltip")
		// Выключить все моды
	app.disableAllBtn = NewCustomButton(app.messages["btn_disable_all_mods"], func() { app.setAllModsActive(false) })
	app.applyTooltip(app.disableAllBtn, "btn_disable_all_tooltip")
		// Удалить все моды
	app.removeAllBtn = NewCustomButton(app.messages["btn_remove_all_mods"], func() {
		app.showConfirmDialog(
			app.messages["confirm_remove_all_title"],
			app.messages["confirm_remove_all_text"],
			"btn_yes",
			"btn_no",
			func(ok bool) {
				if ok {
					app.removeAllMods()
				}
			},
		)
	})
	app.applyTooltip(app.removeAllBtn, "btn_remove_all_tooltip")
		// Удалить выбранные моды
	app.removeSelectedBtn = NewCustomButton(app.messages["btn_remove_selected"], func() {
		sel := app.selectedMods()
		if len(sel) == 0 {
			app.appendLog(app.messages["no_mods_selected"])
			return
		}
		app.showConfirmDialog(
			app.messages["confirm_remove_selected_title"],
			fmt.Sprintf(app.messages["confirm_remove_selected_text"], len(sel)),
			"btn_yes",
			"btn_no",
			func(ok bool) {
				if ok {
					app.removeSelectedMods()
				}
			},
		)
	})
	app.applyTooltip(app.removeSelectedBtn, "btn_remove_selected_tooltip")

	// Основные кнопки
	app.btnUp = NewCustomButton(app.messages["btn_up"], func() { app.moveSelected(-1) })
	app.applyTooltip(app.btnUp, "btn_up_tooltip")

	app.btnDown = NewCustomButton(app.messages["btn_down"], func() { app.moveSelected(1) })
	app.applyTooltip(app.btnDown, "btn_down_tooltip")

	app.btnSaveOrder = NewCustomButton(app.messages["btn_save_order"], func() {
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
	app.applyTooltip(app.btnSaveOrder, "btn_save_order_tooltip")

	app.btnRefresh = NewCustomButton(app.messages["btn_refresh"], func() {
		go func() {
			if app.orderDirty {
				choice := app.showChoiceDialog(app.mainWindow,
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
					case 2:
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
	app.applyTooltip(app.btnRefresh, "btn_refresh_tooltip")

	app.btnToggle = NewCustomButton(app.messages["btn_disable_mods"], func() { app.toggleGlobalMods() })
	app.applyTooltip(app.btnToggle, "btn_toggle_tooltip")
	app.updateToggleButtonText(app.btnToggle)

	// Кнопка управления модами и панель
	app.manageBtn = NewCustomButton(app.messages["btn_manage_mods"], func() {
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
	app.applyTooltip(app.manageBtn, "btn_manage_mods_tooltip")

	if btnImgData, _ := embeddedFiles.ReadFile("assets/Yellow_BG_button.jpg"); btnImgData != nil {
		img := canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG_button", btnImgData))
		img.FillMode = canvas.ImageFillStretch
		img.Translucency = 0.8
		app.manageBtn.SetBackgroundImage(img)
	}

	if colImgData, _ := embeddedFiles.ReadFile("assets/Yellow_BG_col.jpg"); colImgData != nil {
		app.selectColumnBgRes = fyne.NewStaticResource("Yellow_BG_col", colImgData)
	}

	moveToGroup := container.NewHBox(app.moveLabel, app.moveToEntry)
	navigationGroup := container.NewHBox(app.btnUp, app.btnDown, app.moveToTopBtn, app.moveToBottomBtn, app.removeSelectedBtn, app.removeAllBtn)
	selectGroup := container.NewHBox(app.selectAllBtn, app.deselectAllBtn, app.enableSelectedBtn, app.disableSelectedBtn)
	allModsGroup := container.NewHBox(app.enableAllBtn, app.disableAllBtn)

	row1 := container.NewHBox(moveToGroup, navigationGroup)
	row2 := container.NewHBox(selectGroup, allModsGroup)

	yellowData, _ := embeddedFiles.ReadFile("assets/Yellow_BG.jpg")
	var yellowBg *canvas.Image
	if yellowData != nil {
		yellowBg = canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG", yellowData))
		yellowBg.FillMode = canvas.ImageFillStretch
		yellowBg.Translucency = 0.9
	}

	app.managePanelBgRect = canvas.NewRectangle(th.Color(themes.ColorManagePanelBg, variant))

	panelContent := container.NewVBox(row1, row2)
	if yellowBg != nil {
		app.managePanel = container.NewStack(app.managePanelBgRect, yellowBg, panelContent)
	} else {
		app.managePanel = container.NewStack(app.managePanelBgRect, panelContent)
	}
	app.managePanel.Hide()

	// Верхняя панель
	app.topPanelBgRect = canvas.NewRectangle(th.Color(themes.ColorTopPanelBg, variant))
	topPanelContent := container.NewHBox(app.manageBtn, app.filterLabel, app.filterSelect, searchBar, app.btnRefresh, app.btnSaveOrder)
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
			statusStr := app.messages["status_system"]
			statusText := canvas.NewText(statusStr, th.Color(themes.ColorStatusSystem, variant))
			cont.Add(statusText)
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
	// Скрываем колонки, которые не нужны для системных модов
	app.systemModsTable.SetColumnWidth(0, 0)	// Колонка с галочкой управления скрыта
		// app.systemModsTable.SetColumnWidth(1, 0)
		// app.systemModsTable.SetColumnWidth(2, 0)

	// Добавляем обработчик выделения для системной таблицы
	app.systemModsTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.systemMods) {
			mod := &app.systemMods[id.Row]
			app.selectedModName = mod.Name
			app.selectedModIndex.Store(-1) // нет в displayedMods
			app.updateDescriptionForMod(mod.Name)
			go app.enrichModFromNexus(mod)
			app.updateUpDownButtons()
			app.systemModsTable.Refresh()
			app.modTable.UnselectAll() // снимаем выделение с основной таблицы
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
		} else if mod.Incompatible {
			bgColor = th.Color(themes.ColorTableRowConflict, variant)
		} else {
			bgColor = baseBG
		}
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
		case 0:
			if app.showSelectColumn && !mod.IsSystem {
				cellBg := canvas.NewRectangle(theme.ButtonColor())
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
				num := widget.NewLabel(fmt.Sprintf("%2d.", id.Row+1))
				cont.Add(num)
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
			dateText := canvas.NewText(dateStr, theme.ForegroundColor())
			dateText.Alignment = fyne.TextAlignCenter
			cont.Add(dateText)
		case 5:
			var statusStr string
			var clr color.Color
			switch {
			case mod.VortexDeployed:
				statusStr = app.messages["status_vortex"]
				clr = th.Color(themes.ColorStatusVortex, variant)
			case mod.IsSystem:
				statusStr = app.messages["status_system"]
				clr = th.Color(themes.ColorStatusSystem, variant)
			case mod.Broken:
				statusStr = app.messages["desc_broken"]
				clr = th.Color(themes.ColorStatusBroken, variant)
			case mod.Incompatible:
				statusStr = app.messages["desc_conflict"]
				clr = th.Color(themes.ColorStatusConflict, variant)
			case mod.Obsolete:
				statusStr = app.messages["desc_obsolete"]
				clr = th.Color(themes.ColorStatusObsolete, variant)
			case mod.Mandatory && mod.Active:
				statusStr = app.messages["status_mandatory"]
				clr = th.Color(themes.ColorStatusMandatory, variant)
			case mod.Active:
				statusStr = app.messages["status_active"]
				clr = th.Color(themes.ColorStatusActive, variant)
			default:
				statusStr = app.messages["status_inactive"]
				clr = th.Color(themes.ColorStatusInactive, variant)
			}
			statusText := canvas.NewText(statusStr, clr)
			statusText.Alignment = fyne.TextAlignCenter
			cont.Add(statusText)
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
			go app.enrichModFromNexus(&app.displayedMods[id.Row])
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
	app.tableBorderContainer = container.NewStack(app.modTable, app.tableBorder)

	// Нижняя панель
	app.counterLabel = widget.NewLabel("")
	bottomPanel := container.NewBorder(
		nil, nil,
		app.counterLabel,
		statusContainer,
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

	// Описание
	app.descTitle = widget.NewLabel(app.messages["select_mod"])
	app.descTitle.TextStyle = fyne.TextStyle{Bold: true}

	app.descAuthor = widget.NewLabel("—")

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

	// Создаём ссылку GitHub
	app.githubLink = widget.NewHyperlink("", nil)
	app.githubLink.Alignment = fyne.TextAlignLeading

	// Создаём виджеты для данных из Nexus API
	app.descLocalVersion = widget.NewLabel("")  // Версия установленная
	app.descLatestVersion = widget.NewLabel("") // Версия новая на сайте

	// Карточка с описанием
	descHeader := container.NewBorder(
		nil, nil, nil, nil,
		container.NewVBox(
			app.descTitle,					// Название
			app.descAuthor,					// Автор
			container.NewHBox(
				widget.NewLabel(""),
				app.descURL ),				// Ссылка Nexus
			container.NewHBox(
				widget.NewLabel(""),
				app.githubLink ),			// Ссылка GitHub
			widget.NewSeparator(),
			container.NewHBox(
				widget.NewLabel(""),
				app.descLocalVersion, app.descLatestVersion ),		//Версия установленная
			// container.NewHBox(
				// widget.NewLabel(""),
				// app.descLatestVersion ),	// Версия новая на сайте
		),
	)

	descScroll := container.NewScroll(app.descBody)
	descScroll.SetMinSize(fyne.NewSize(DescScrollMinWidth, DescScrollMinHeight))

	descCardContent := container.NewVBox(
		descHeader,
		widget.NewSeparator(),
		descScroll,
	)

	descCard := container.NewStack(
		descCardBg,
		container.NewPadded(descCardContent),
	)

	// кнопки правой панели
	app.btnSortChecks = NewCustomButton(app.messages["btn_sort_checks"], func() { go app.runAllChecks() })
	app.applyTooltip(app.btnSortChecks, "btn_sort_checks_tooltip")

	if app.amlDetected {
		app.btnSaveOrder.SetText(app.messages["btn_save_order_aml"])
		app.btnSortChecks.SetText(app.messages["btn_sort_checks_aml"])
		app.applyTooltip(app.btnSaveOrder, "aml_save_warning_tooltip")
		app.applyTooltip(app.btnSortChecks, "aml_sort_warning_tooltip")
	}

	app.btnInstall = NewCustomButton(app.messages["btn_install"], func() {
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
		fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
		fd.Show()
	})
	app.applyTooltip(app.btnInstall, "btn_install_tooltip")

	app.btnRemove = NewCustomButton(app.messages["btn_remove"], func() {
		if app.selectedModName == "" {
			return
		}
		modName := app.selectedModName
		mod := app.findModByName(modName)
		if mod == nil {
			return
		}
		if mod.IsSystem {
			app.appendLog(app.messages["log_cannot_delete_system"])
			return
		}
		app.showConfirmDialog(
			app.messages["confirm_delete_title"],
			fmt.Sprintf(app.messages["confirm_delete_text"], mod.Name),
			"btn_yes",
			"btn_no",
			func(ok bool) {
				if ok {
					checks.RemoveMod(modName)
					app.removeFromAllMods(modName)
					app.refreshModList()
					app.appendLog(fmt.Sprintf(app.messages["log_deleted"], modName))
				}
			},
		)
	})
	app.applyTooltip(app.btnRemove, "btn_remove_tooltip")

	// запуск
	gameVer := detectGameVersion(app.gameRoot)
	app.btnLaunchNormal = NewCustomButton(app.messages["btn_launch_game"],
		func() {
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
	app.applyTooltip(app.btnLaunchNormal, "btn_launch_game_tooltip")

	app.btnLaunchNoLauncher = NewCustomButton(app.messages["btn_launch_nolauncher_long"],
		func() {
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
	app.applyTooltip(app.btnLaunchNoLauncher, "btn_launch_nolauncher_long_tooltip")

	// Кнопка обновления мода (с проверкой на системные)
	app.btnUpdateMod = NewCustomButton(app.messages["btn_update_mod"], func() {
		if app.selectedModName == "" {
			return
		}
		mod := app.findModByName(app.selectedModName)
		if mod == nil || mod.URL == "" {
			return
		}
		if mod.Name == "base" { // DMLoader
			app.updateDML()
			return
		}
		if mod.IsSystem { // DMFramework и другие системные
			app.appendLog(app.messages["log_cannot_update_system"])
			return
		}
		app.updateModFromNexus(mod)
	})
	app.applyTooltip(app.btnUpdateMod, "btn_update_mod_tooltip")

	app.btnUpdateAll = NewCustomButton(app.messages["btn_update_all"], func() {
		app.updateAllModsFromNexus()
	})
	app.applyTooltip(app.btnUpdateAll, "btn_update_all_tooltip")

	app.btnCheckUpdates = NewCustomButton(app.messages["btn_check_updates"], func() {
		go app.checkNexusUpdates()
	})
	app.applyTooltip(app.btnCheckUpdates, "btn_check_updates_tooltip")

	if gameVer == VersionUnknown {
		app.btnLaunchNormal.Hide()
		app.btnLaunchNoLauncher.Hide()
	}

	topRight := container.NewVBox(
		container.NewHBox(app.btnSortChecks, app.btnInstall, app.btnRemove),
		container.NewHBox(app.btnLaunchNormal, app.btnLaunchNoLauncher, app.btnToggle),
		container.NewHBox(app.btnCheckUpdates, app.btnUpdateMod, app.btnUpdateAll),
	)

	rightContent := container.NewVSplit(descCard, app.consoleScroll)
	rightContent.Offset = 0.65

	rightPanel := container.NewBorder(topRight, nil, nil, nil, rightContent)

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

	for _, btn := range []*CustomButton{
		app.btnSaveOrder, app.btnRefresh, app.btnInstall, app.btnRemove,
		app.btnUp, app.btnDown, app.btnSortChecks, app.btnToggle,
		app.btnLaunchNormal, app.btnLaunchNoLauncher,
		app.moveToTopBtn, app.moveToBottomBtn,
		app.selectAllBtn, app.deselectAllBtn, app.enableSelectedBtn,
		app.disableSelectedBtn, app.enableAllBtn, app.disableAllBtn,
		app.manageBtn, app.searchClearBtn, app.removeAllBtn, app.removeSelectedBtn,
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

func (app *App) showChoiceDialog(parent fyne.Window, title, message string, options ...string) int {
	resultChan := make(chan int, 1)
	fyne.DoAndWait(func() {
		var buttons []fyne.CanvasObject
		for i, opt := range options {
			idx := i
			btn := NewCustomButton(opt, func() {
				resultChan <- idx
			})
			buttons = append(buttons, btn)
		}
		gradHeader := canvas.NewImageFromImage(app.makeRedCRTGradient(DialogGradientWidth, DialogGradientHeight))
		gradHeader.FillMode = canvas.ImageFillStretch
		titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		headerContainer := container.NewStack(gradHeader, container.NewCenter(titleLabel))

		msgLabel := widget.NewLabel(message)
		msgLabel.Wrapping = fyne.TextWrapWord // Перенос строк для включения скролла
		msgScroll := container.NewScroll(msgLabel)
		msgScroll.SetMinSize(fyne.NewSize(666, 250)) // ограничение высоты для скролла
		content := container.NewVBox(
			headerContainer,
			msgScroll,
			container.NewCenter(container.NewHBox(buttons...)),
		)

		popUp := widget.NewModalPopUp(content, parent.Canvas())
		popUp.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
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
	app.descAuthor.SetText(fmt.Sprintf(app.messages["author_label"], author))
	app.descInstalled.SetText(fmt.Sprintf(app.messages["installed_label"], app.formatDate(mod.ModTime, app.cfg.DateFormat)))

    // Данные из Nexus API
	// Локальная версия (установленная)
	if app.descLocalVersion != nil {
		if mod.URL != "" {
			modID := fmt.Sprintf("%d", extractModIDFromURL(mod.URL))
			if cached, ok := app.nexusVersionCache[modID]; ok {
				app.descLocalVersion.SetText(fmt.Sprintf(app.messages["nexus_local_version_label"], cached))
			} else {
				app.descLocalVersion.SetText(app.messages["nexus_local_version_unknown"])
			}
		} else {
			app.descLocalVersion.SetText("")
		}
	}
	// Последняя версия с сайта
	if app.descLatestVersion != nil {
		if mod.URL != "" {
			modID := fmt.Sprintf("%d", extractModIDFromURL(mod.URL))
			if latest, ok := app.nexusLatestVersions[modID]; ok {
				app.descLatestVersion.SetText(fmt.Sprintf(app.messages["nexus_latest_version_label"], latest))
			} else {
				app.descLatestVersion.SetText(app.messages["nexus_latest_version_unknown"])
			}
		} else {
			app.descLatestVersion.SetText("")
		}
	}

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

	// Ссылка Nexus (основная)
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

	// Новая ссылка GitHub (под ней)
	if app.githubLink != nil {
		// Ссылка GitHub (если есть)
		if mod.GitHubURL != "" {
			u, err := url.Parse(mod.GitHubURL)
			if err == nil {
				app.githubLink.SetURL(u)
				app.githubLink.SetText(app.messages["github_url_label"])
			} else {
				app.githubLink.SetURL(nil)
				app.githubLink.SetText("")
			}
		} else {
			app.githubLink.SetURL(nil)
			app.githubLink.SetText("")
		}
	}
}

// enrichModFromNexus пытается получить информацию из Nexus API и обновить модель и UI.
func (app *App) enrichModFromNexus(mod *checks.ModInfo) {
    if app.cfg.NexusAPIKey == "" || mod.URL == "" {
        return
    }
    modID := extractModIDFromURL(mod.URL)
    if modID == 0 {
        return
    }
    // app.appendLog(fmt.Sprintf("DEBUG: Fetching Nexus info for mod ID %d", modID))

    // Запускаем горутину ТОЛЬКО ЗДЕСЬ, не дублируем
    go func() {
        defer func() {
            if r := recover(); r != nil {
                // app.appendLog(fmt.Sprintf("PANIC in enrichModFromNexus: %v", r))
            }
        }()

        info, err := app.FetchNexusModInfo(modID, app.cfg.NexusAPIKey)
        if err != nil {
            app.appendLog(fmt.Sprintf(app.messages["log_nexus_api_error"], mod.Name, err))
            return
        }
        // app.appendLog(fmt.Sprintf("DEBUG: Nexus info received: Name=%s, Version=%s", info.Name, info.Version))

        // Обновляем поля мода
        mod.NexusVersion = info.Version
        mod.NexusSummary = info.Summary
        mod.NexusDownloads = info.Downloads
        mod.NexusEndorsements = info.Endorsements
        mod.NexusPictureURL = info.PictureURL
        if info.Author != "" {
            mod.Author = info.Author
        }

		modIDStr := fmt.Sprintf("%d", extractModIDFromURL(mod.URL))
		app.nexusLatestVersions[modIDStr] = info.Version

        // Обновляем UI только если этот мод всё ещё выбран
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
	btn.Refresh()
	if app.mainWindow != nil {
		app.mainWindow.Canvas().Refresh(btn)
	}
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
		return
	}
	if app.filterSelect == nil {
		app.displayedMods = app.allMods
		if app.modTable != nil {
			app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
			if app.selectedModName != "" {
				for i, m := range app.displayedMods {
					if m.Name == app.selectedModName {
						app.selectedModIndex.Store(int32(i))
						break
					}
				}
			} else {
				app.selectedModIndex.Store(-1)
			}
			app.modTable.Refresh()
		}
		activeCount := 0
		for _, m := range app.displayedMods {
			if m.Active {
				activeCount++
			}
		}
		if app.counterLabel != nil {
			app.counterLabel.SetText(fmt.Sprintf(app.messages["mods_counter"], len(app.displayedMods), len(app.allMods), activeCount))
		}
		return
	}

	predicates := map[string]modFilterFunc{
		app.messages["filter_all"]:      func(m checks.ModInfo) bool { return true },
		app.messages["filter_active"]:   func(m checks.ModInfo) bool { return m.Active },
		app.messages["filter_inactive"]: func(m checks.ModInfo) bool { return !m.Active },
		app.messages["filter_obsolete"]: func(m checks.ModInfo) bool { return m.Obsolete },
		app.messages["filter_conflict"]: func(m checks.ModInfo) bool { return m.Incompatible },
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

	if app.modTable != nil {
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
	}

	activeCount := 0
	for _, m := range app.displayedMods {
		if m.Active {
			activeCount++
		}
	}
	if app.counterLabel != nil {
		app.counterLabel.SetText(fmt.Sprintf(app.messages["mods_counter"], len(app.displayedMods), len(app.allMods), activeCount))
	}
}

func (app *App) filterOptions() []string {
	return []string{
		app.messages["filter_all"], app.messages["filter_active"], app.messages["filter_inactive"],
		app.messages["filter_obsolete"], app.messages["filter_conflict"],
	}
}

func (app *App) setAllModsActive(active bool) {
	for i := range app.allMods {
		if !app.allMods[i].IsSystem {
			app.allMods[i].Active = active
		}
	}
	app.orderDirty = true
	app.updateTableBorder()
	app.filterModList()
}

func (app *App) selectAllMods(selected bool) {
	for i := range app.allMods {
		app.allMods[i].Selected = selected
	}
	app.filterModList()
}

func (app *App) setSelectedActive(active bool) {
	for i := range app.allMods {
		if app.allMods[i].Selected && !app.allMods[i].IsSystem {
			app.allMods[i].Active = active
		}
	}
	app.orderDirty = true
	app.updateTableBorder()
	app.filterModList()
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

// requestNexusAPIKey показывает диалог для ввода ключа API Nexus Mods.
func (app *App) requestNexusAPIKey() {
    if app.cfg.NexusAPIKey != "" { return }
    fyne.Do(func() {
        entry := widget.NewEntry()
        entry.SetPlaceHolder(app.messages["nexus_api_key_placeholder"])
        // Создаём диалог с явной переменной
        var dlg dialog.Dialog
        content := container.NewVBox(
            widget.NewLabel(app.messages["nexus_api_key_label"]),
            entry,
            widget.NewButton(app.messages["btn_save"], func() {
                app.cfg.NexusAPIKey = entry.Text
                saveConfig(app.cfg)
                app.appendLog(app.messages["nexus_api_key_saved"])
                dlg.Hide()           // закрываем
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

// showChoiceDialogAsync - асинхронная версия диалога с красным градиентом.
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
		msgLabel.Wrapping = fyne.TextWrapWord // Перенос строк для включения скролла
		msgScroll := container.NewScroll(msgLabel)
		msgScroll.SetMinSize(fyne.NewSize(666, 250)) // Ограничение высоты для скролла
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

// Новый метод для установки DML (вызывается из кнопки «Обновить мод»)
func (app *App) updateDML() {
	if app.cfg.NexusAPIKey == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	const dmlModID = 19
	info, err := app.FetchNexusModInfo(dmlModID, app.cfg.NexusAPIKey)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["version_check_failed"], "DML", err))
		return
	}
	modIDStr := fmt.Sprintf("%d", dmlModID)
	cached, exists := app.nexusVersionCache[modIDStr]
	if exists && cached == info.Version {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "DML", info.Version))
		return
	}
	app.appendLog(fmt.Sprintf(app.messages["looking_for_latest_file"], dmlModID))
	fileID, err := getLatestFileID(dmlModID, app.cfg.NexusAPIKey)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}
	directURL, filename, err := app.fetchDirectDownloadLink(
		fmt.Sprintf("%d", dmlModID), fmt.Sprintf("%d", fileID), app.cfg.NexusAPIKey)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	fyne.Do(func() {
		app.showDMLDownloadDialog(directURL, filename, app.cfg.NexusAPIKey)
	})
}

// Диалог загрузки и установки DML
func (app *App) showDMLDownloadDialog(url, filename, apiKey string) {
    app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], "Darktide Mod Loader"))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading_dml"], filename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)
	dlg.Show()

	go func() {
		dest := filepath.Join(app.cfg.ModsPath, filename) // временно скачиваем в mods
		err := app.DownloadFileWithProgress(url, dest, apiKey, bar, app.mainWindow)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				return
			}
			app.appendLog(app.messages["installing_dml"])
			if err := app.installDMLFromArchive(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["dml_install_failed"], err))
			} else {
				app.appendLog(app.messages["dml_updated"])
			}
			os.Remove(dest)
		})
	}()
}

// handleNXMLink с поддержкой DML (ID=19)
func (app *App) handleNXMLink(nxmURL string) {
	parts := strings.Split(nxmURL, "/")
	if len(parts) < 5 {
		app.appendLog(app.messages["log_invalid_nmx_link"])
		return
	}

	modID := parts[len(parts)-3]
	fileID := strings.Split(parts[len(parts)-1], "?")[0]

	if app.cfg.NexusAPIKey == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}

	// DML устанавливается в корень игры
	if modID == "19" {
		go func() {
			directURL, filename, err := app.fetchDirectDownloadLink(modID, fileID, app.cfg.NexusAPIKey)
			if err != nil {
				app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
				return
			}
			fyne.Do(func() {
				app.showDMLDownloadDialog(directURL, filename, app.cfg.NexusAPIKey)
			})
		}()
		return
	}

	// Обычные моды, с получением версии файла и сохранением после установки
	go func() {
		directURL, filename, err := app.fetchDirectDownloadLink(modID, fileID, app.cfg.NexusAPIKey)
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
			return
		}
		mid, _ := strconv.Atoi(modID)
		fid, _ := strconv.Atoi(fileID)
		var fileVersion string
		if mid > 0 && fid > 0 {
			if fi, err := app.FetchFileInfo(mid, fid, app.cfg.NexusAPIKey); err == nil {
				fileVersion = fi.Version
			}
		}
		// Получаем имя мода, если возможно
		modName := "Mod " + modID
		if mid > 0 {
			if info, err := app.FetchNexusModInfo(mid, app.cfg.NexusAPIKey); err == nil {
				modName = info.Name
			}
		}
		fyne.Do(func() {
			app.showDownloadDialog(directURL, filename, app.cfg.NexusAPIKey, modName, fileVersion, modID)
		})
	}()
}

// showNexusAPIKeyDialog открывает диалог ввода/изменения ключа API Nexus.
func (app *App) showNexusAPIKeyDialog() {
	fyne.Do(func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder(app.messages["nexus_api_key_placeholder"])
		entry.SetText(app.cfg.NexusAPIKey) // показываем текущий ключ, если есть
		var dlg dialog.Dialog
		content := container.NewVBox(
			widget.NewLabel(app.messages["nexus_api_key_label"]),
			entry,
			widget.NewButton(app.messages["btn_save_api"], func() {
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

// showDownloadDialog скачивает и устанавливает обычный мод с параметрами modName, fileVersion, modID и сохранением версии в кэш
func (app *App) showDownloadDialog(url, filename, apiKey, modName, fileVersion, modID string) {
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
        // Проверяем расширение, добавляем .zip при необходимости - мод Healthbars и т.д.
        ext := strings.ToLower(filepath.Ext(dest))
        knownExts := map[string]bool{".zip": true, ".rar": true, ".7z": true}
        if !knownExts[ext] {
            newDest := dest + ".zip"
            if err := os.Rename(dest, newDest); err == nil {
                dest = newDest
            }
        }
        err := app.DownloadFileWithProgress(url, dest, apiKey, bar, app.mainWindow)
        fyne.Do(func() {
            dlg.Hide()
            if err != nil {
                app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
                return
            }
            if info, e := os.Stat(dest); e == nil {
                app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_size"], float64(info.Size())/1024/1024))
            } else {
                app.appendLog(fmt.Sprintf(app.messages["log_downloaded_file_not_found"], e))
            }
            app.appendLog(app.messages["download_complete"])
            installedName, err := app.InstallModFromArchive(dest, false)
            if err != nil {
                app.appendLog(fmt.Sprintf(app.messages["log_install_failed"], err))
            } else {
                os.Remove(dest)
                if fileVersion != "" && modID != "" {
                    app.nexusVersionCache[modID] = fileVersion
                    app.saveNexusVersionCache()
                }
                if installedName != "" {
                    app.selectModByName(installedName)
                }
            }
        })
    }()
}

// showConfirmDialog показывает локализованный диалог подтверждения.
func (app *App) showConfirmDialog(title, message, confirmKey, cancelKey string, callback func(bool)) {
    fyne.Do(func() {
        var popUp *widget.PopUp
        confirmBtn := widget.NewButton(app.messages[confirmKey], func() {
            popUp.Hide()
            if callback != nil {
                callback(true)
            }
        })
        cancelBtn := widget.NewButton(app.messages[cancelKey], func() {
            popUp.Hide()
            if callback != nil {
                callback(false)
            }
        })
        // Красный градиентный заголовок
        gradHeader := canvas.NewImageFromImage(app.makeRedCRTGradient(DialogGradientWidth, DialogGradientHeight))
        gradHeader.FillMode = canvas.ImageFillStretch
        titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
        headerContainer := container.NewStack(gradHeader, container.NewCenter(titleLabel))
        msgLabel := widget.NewLabel(message)
        // msgLabel.Wrapping = fyne.TextWrapWord
        content := container.NewVBox(
            headerContainer,
            msgLabel,
            container.NewCenter(container.NewHBox(confirmBtn, cancelBtn)),
        )
        popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
        popUp.Show()
    })
}
