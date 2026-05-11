// ui.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/config"
	"fmt"
	"image/color"
	"net/url"
	"path/filepath"
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

func (app *App) buildUI() {
	// ---------- лог ----------
	app.logWindow = widget.NewRichText(
		&widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForegroundOnWarning,
				TextStyle: fyne.TextStyle{},
			},
			// Text: "",
		},
	)
	app.logWindow.Wrapping = fyne.TextWrapWord

	// CRT-фон и градиент (оставляем как было)
	crtData, _ := embeddedFiles.ReadFile(config.ConsoleBackgroundImage)
	var crtImg *canvas.Image
	var grad *canvas.Image
	if crtData != nil {
		crtImg = canvas.NewImageFromResource(fyne.NewStaticResource("CRT_BlackBG", crtData))
		crtImg.FillMode = canvas.ImageFillStretch
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
		grad.Translucency = config.ConsoleGradientOpacity
	} else {
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
	}

	// Рамка-экран с закруглением
	screenBg := canvas.NewRectangle(color.NRGBA{R: 192, G: 255, B: 26, A: 15})
	screenBg.CornerRadius = 22
	screenBg.StrokeWidth = 2
	screenBg.StrokeColor = color.NRGBA{R: 192, G: 255, B: 26, A: 111}

	// Заголовок (остаётся как раньше)
	app.logHeaderText = canvas.NewText("", color.NRGBA{R: 192, G: 255, B: 26, A: 255})
	app.logHeaderText.TextStyle = fyne.TextStyle{Bold: true}
	app.logHeaderText.Alignment = fyne.TextAlignCenter
	app.logHeaderText.TextSize = theme.TextSize()

	// Стек для «экрана»: фон/градиент/рамка + сам лог
	logStack := container.NewStack()
	if crtImg != nil {
		logStack.Add(crtImg)
	}
	logStack.Add(grad)
	logStack.Add(screenBg)
	logStack.Add(container.NewPadded(app.logWindow)) // небольшой отступ изнутри

	// Подложка под заголовок
	headerBox := container.NewStack(
		canvas.NewRectangle(color.NRGBA{R: 10, G: 10, B: 10, A: 175}),
		container.NewCenter(app.logHeaderText),
	)

	// Собираем всё вместе: заголовок сверху, экран под ним
	logPanel := container.NewBorder(headerBox, nil, nil, nil, logStack)

	app.consoleScroll = container.NewScroll(logPanel)
	app.consoleScroll.SetMinSize(fyne.NewSize(config.ConsoleWidth, config.ConsoleHeight))

	// ---------- поиск, фильтр ----------
	app.searchEntry = widget.NewEntry()
	app.searchEntry.SetPlaceHolder(app.messages["search_placeholder"])

	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(250, 1))
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

	app.filterSelect = widget.NewSelect([]string{
		app.messages["filter_all"], app.messages["filter_active"], app.messages["filter_inactive"],
		app.messages["filter_obsolete"], app.messages["filter_conflict"],
	}, nil)
	app.filterSelect.SetSelected(app.messages["filter_all"])
	app.filterSelect.OnChanged = func(s string) { app.filterModList() }
	app.filterLabel = widget.NewLabel(app.messages["filter_label"])

	// ---------- статус-менеджер для тултипов ----------
	app.statusLabel = widget.NewLabel("")
	app.statusLabel.Alignment = fyne.TextAlignCenter
	app.statusLabel.TextStyle = fyne.TextStyle{Bold: true}
	app.tooltipStatus = NewTooltipStatusManager(app.statusLabel)

	tipBg := canvas.NewRectangle(color.NRGBA{R: 10, G: 10, B: 10, A: 200})
	tipBg.CornerRadius = 6
	tipBg.SetMinSize(fyne.NewSize(500, 20))
	statusContainer := container.NewStack(tipBg, app.statusLabel)

	// ---------- кнопки быстрого перемещения ----------
	app.moveToTopBtn = NewCustomButton(app.messages["btn_move_to_top"], func() {
		app.moveSelectedToTop()
	})
	app.applyTooltip(app.moveToTopBtn, "btn_move_to_top_tooltip")
	app.moveToBottomBtn = NewCustomButton(app.messages["btn_move_to_bottom"], func() {
		app.moveSelectedToBottom()
	})
	app.applyTooltip(app.moveToBottomBtn, "btn_move_to_bottom_tooltip")

	app.moveToEntry = widget.NewEntry()
	app.moveToEntry.SetPlaceHolder(app.messages["col_number"] + app.messages["col_number"])
	app.moveToEntry.OnSubmitted = func(text string) {
		app.moveSelectedToPosition()
	}
	app.moveLabel = widget.NewLabel(app.messages["lbl_move_to"])

	// ---------- кнопки выделения и массовых операций ----------
	app.selectAllBtn = NewCustomButton(app.messages["btn_select_all"], func() {
		app.selectAllMods(true)
	})
	app.applyTooltip(app.selectAllBtn, "btn_select_all_tooltip")
	app.deselectAllBtn = NewCustomButton(app.messages["btn_deselect_all"], func() {
		app.selectAllMods(false)
	})
	app.applyTooltip(app.deselectAllBtn, "btn_deselect_all_tooltip")
	app.enableSelectedBtn = NewCustomButton(app.messages["btn_enable_selected"], func() {
		app.setSelectedActive(true)
	})
	app.applyTooltip(app.enableSelectedBtn, "btn_enable_selected_tooltip")
	app.disableSelectedBtn = NewCustomButton(app.messages["btn_disable_selected"], func() {
		app.setSelectedActive(false)
	})
	app.applyTooltip(app.disableSelectedBtn, "btn_disable_selected_tooltip")

	app.enableAllBtn = NewCustomButton(app.messages["btn_enable_all_mods"], func() {
		app.setAllModsActive(true)
	})
	app.applyTooltip(app.enableAllBtn, "btn_enable_all_tooltip")
	app.disableAllBtn = NewCustomButton(app.messages["btn_disable_all_mods"], func() {
		app.setAllModsActive(false)
	})
	app.applyTooltip(app.disableAllBtn, "btn_disable_all_tooltip")

	// ---------- основные управляющие кнопки ----------
	app.btnUp = NewCustomButton(app.messages["btn_up"], func() { app.moveSelected(-1) })
	app.applyTooltip(app.btnUp, "btn_up_tooltip")
	app.btnDown = NewCustomButton(app.messages["btn_down"], func() { app.moveSelected(1) })
	app.applyTooltip(app.btnDown, "btn_down_tooltip")
	app.btnSaveOrder = NewCustomButton(app.messages["btn_save_order"], func() {
		if app.orderDirty {
			app.saveCurrentOrder()
			app.orderDirty = false
			app.appendLog(app.messages["log_order_saved"])
			app.stopBlinkSaveButton()
			app.updateTableBorder()
		} else {
			app.appendLog(app.messages["log_order_unchanged"])
		}
	})
	app.applyTooltip(app.btnSaveOrder, "btn_save_order_tooltip")
	app.btnRefresh = NewCustomButton(app.messages["btn_refresh"], func() {
		app.refreshModList()
		app.appendLog(app.messages["log_list_refreshed"])
	})
	app.applyTooltip(app.btnRefresh, "btn_refresh_tooltip")
	app.btnToggle = NewCustomButton(app.messages["btn_disable_mods"], func() { app.toggleGlobalMods() })
	app.applyTooltip(app.btnToggle, "btn_toggle_tooltip")
	app.updateToggleButtonText(app.btnToggle)

	// ---------- кнопка «Управление модами» и скрываемая панель ----------
	app.manageBtn = NewCustomButton(app.messages["btn_manage_mods"], func() {
		if app.managePanel.Visible() {
			app.managePanel.Hide()
			app.showSelectColumn = false
			app.headerTable.SetColumnWidth(0, 0)
			app.modTable.SetColumnWidth(0, 0)
		} else {
			app.managePanel.Show()
			app.showSelectColumn = true
			app.headerTable.SetColumnWidth(0, config.ColSelectWidth)
			app.modTable.SetColumnWidth(0, config.ColSelectWidth)
		}
		app.headerTable.Refresh()
		app.modTable.Refresh()
		app.managePanel.Refresh()
	})
	app.applyTooltip(app.manageBtn, "btn_manage_mods_tooltip")

	// Фон для кнопки (отдельная картинка)
	if btnImgData, _ := embeddedFiles.ReadFile("assets/Yellow_BG_button.jpg"); btnImgData != nil {
		img := canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG_button", btnImgData))
		img.FillMode = canvas.ImageFillStretch
		img.Translucency = 0.8
		app.manageBtn.SetBackgroundImage(img)
	}

	// Фон для столбца выделения (отдельная картинка)
	if colImgData, _ := embeddedFiles.ReadFile("assets/Yellow_BG_col.jpg"); colImgData != nil {
		app.selectColumnBgRes = fyne.NewStaticResource("Yellow_BG_col", colImgData)
	}

	// Группы кнопок
	moveToGroup := container.NewHBox(app.moveLabel, app.moveToEntry)

	navigationGroup := container.NewHBox(app.btnUp, app.btnDown, app.moveToTopBtn, app.moveToBottomBtn)
	actionGroup := container.NewHBox(app.btnSaveOrder, app.btnRefresh)
	selectGroup := container.NewHBox(app.selectAllBtn, app.deselectAllBtn, app.enableSelectedBtn, app.disableSelectedBtn)
	allModsGroup := container.NewHBox(app.enableAllBtn, app.disableAllBtn)

	row1 := container.NewHBox(moveToGroup, navigationGroup, actionGroup)
	row2 := container.NewHBox(selectGroup, allModsGroup)

	// Фон для панели (отдельная картинка Yellow_BG.jpg)
	yellowData, _ := embeddedFiles.ReadFile("assets/Yellow_BG.jpg")
	var yellowBg *canvas.Image
	if yellowData != nil {
		yellowBg = canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG", yellowData))
		yellowBg.FillMode = canvas.ImageFillStretch
		yellowBg.Translucency = 0.9
	}

	// Чёрный фон панели
	blackBg := canvas.NewRectangle(color.NRGBA{R: 10, G: 10, B: 10, A: 255})

	panelContent := container.NewVBox(row1, row2)
	// Если yellowBg не nil, используем его для панели
	if yellowBg != nil {
		app.managePanel = container.NewStack(blackBg, yellowBg, panelContent)
	} else {
		app.managePanel = container.NewStack(blackBg, panelContent)
	}
	app.managePanel.Hide()

	// ---------- верхняя панель ----------
	darkBg := canvas.NewRectangle(color.NRGBA{R: 22, G: 22, B: 22, A: 255})
	topPanelContent := container.NewHBox(app.manageBtn, app.filterLabel, app.filterSelect, searchBar)
	topPanelWithBg := container.NewStack(darkBg, topPanelContent)

	// ---------- таблица заголовков ----------
	headerCreateCell := func() fyne.CanvasObject {
		return container.NewStack(
			canvas.NewRectangle(color.Transparent),
			widget.NewLabel(""),
		)
	}
	headerUpdateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		bg := canvas.NewRectangle(color.NRGBA{R: 20, G: 20, B: 20, A: 255})
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
		func() (int, int) { return 1, config.TableColumnCount },
		headerCreateCell,
		headerUpdateCell,
	)
	config.ApplyTableColumnWidths(app.headerTable)
	app.headerTable.SetColumnWidth(0, 0)
	app.headerTable.OnSelected = nil

	// ---------- таблица системных модов ----------
	// systemCreateCell := func() fyne.CanvasObject {
	// 	return container.NewStack(widget.NewLabel(""))
	// }
	systemCreateCell := func() fyne.CanvasObject {
        spacer := canvas.NewRectangle(color.Transparent)
        spacer.SetMinSize(fyne.NewSize(1, 6))   // уменьшено с ~32 до 6
        lbl := widget.NewLabel("")
        return container.NewStack(spacer, lbl)
    }
	systemUpdateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.systemMods) {
			return
		}
		mod := &app.systemMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		bgColor := color.NRGBA{R: 20, G: 20, B: 20, A: 150} // отличимый фон
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
		case 0:
			// пустая колонка для выравнивания с основной таблицей (если нужно)
			cont.Add(widget.NewLabel(""))
		case 1:
			cont.Add(widget.NewLabel("")) // вместо чекбокса
		case 2:
			cont.Add(widget.NewLabel("")) // без номера
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
			statusText := canvas.NewText(statusStr, color.NRGBA{R: 100, G: 180, B: 255, A: 255})
			cont.Add(statusText)
		case 6:
			noteLabel := widget.NewLabel(mod.Note)
			noteLabel.Wrapping = fyne.TextWrapWord
			cont.Add(noteLabel)
		}
	}

	app.systemModsTable = widget.NewTable(
		func() (int, int) { return len(app.systemMods), config.TableColumnCount },
		systemCreateCell,
		systemUpdateCell,
	)
	config.ApplyTableColumnWidths(app.systemModsTable)
	app.systemModsTable.SetColumnWidth(0, 0) // скрыть колонку выделения
	// Системная таблица не должна реагировать на выбор
	// Просто не назначаем OnSelected

	// Контейнер для системной таблицы с фиксированной высотой (две строки)
	sysHeight := float32(75)
	sysSpacer := canvas.NewRectangle(color.Transparent)
	sysSpacer.SetMinSize(fyne.NewSize(1, sysHeight))
	systemTableContainer := container.NewStack(sysSpacer, app.systemModsTable)
	if !app.cfg.ShowSystemMods {
		systemTableContainer.Hide()
	}
	app.systemModsTableContainer = systemTableContainer

	// ---------- таблица модов ----------
	createCell := func() fyne.CanvasObject {
        spacer := canvas.NewRectangle(color.Transparent)
        spacer.SetMinSize(fyne.NewSize(1, 6))   // уменьшено с ~32 до 6
        lbl := widget.NewLabel("")
        return container.NewStack(spacer, lbl)
    }
	updateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.displayedMods) {
			return
		}
		mod := &app.displayedMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		var bgColor color.Color = color.Transparent
		// Базовый фон строки
		baseBG := color.NRGBA{R: 38, G: 38, B: 42, A: 155}
		if id.Row%2 == 1 {
			baseBG = color.NRGBA{R: 34, G: 34, B: 38, A: 5}
		}
		// если выделенная строка – переопределить
		if id.Row == int(app.selectedModIndex.Load()) {
			bgColor = color.NRGBA{R: 60, G: 160, B: 30, A: 80}
		} else if mod.Incompatible {
			bgColor = color.NRGBA{R: 80, G: 40, B: 0, A: 120}
		} else {
			bgColor = baseBG
		}
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
        case 0:
            if app.showSelectColumn && !mod.IsSystem {
                // Фон ячейки – та же картинка, что у кнопки Mod Management
                cellBg := canvas.NewRectangle(theme.ButtonColor()) // fallback
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
			cont.Add(widget.NewLabel(dateStr))
		case 5:
			var statusStr string
			var clr color.Color = color.White
			switch {
			case mod.IsSystem:
				statusStr = app.messages["status_system"]
				clr = color.NRGBA{R: 100, G: 180, B: 255, A: 255} // голубой
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
		case 6:
			noteLabel := widget.NewLabel(mod.Note)
			noteLabel.Wrapping = fyne.TextWrapWord
			cont.Add(noteLabel)
		}
	}

	app.modTable = widget.NewTable(
		func() (int, int) { return len(app.displayedMods), config.TableColumnCount },
		createCell,
		updateCell,
	)
	config.ApplyTableColumnWidths(app.modTable)
	app.modTable.SetColumnWidth(0, 0)

	app.modTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.displayedMods) {
			app.selectedModName = app.displayedMods[id.Row].Name
			app.selectedModIndex.Store(int32(id.Row))
			app.updateDescriptionForMod(app.selectedModName)
			app.updateUpDownButtons()
			app.modTable.Refresh()
		}
	}

	// ---------- красная рамка вокруг таблицы ----------
	app.tableBorder = canvas.NewRectangle(color.NRGBA{R: 200, G: 0, B: 0, A: 255})
	app.tableBorder.StrokeWidth = 2
	app.tableBorder.FillColor = color.Transparent
	app.tableBorder.Hide()
	app.tableBorderContainer = container.NewStack(app.modTable, app.tableBorder)

	// ---------- нижняя панель со счётчиком и тултип-лейблом ----------
	app.counterLabel = widget.NewLabel("")
	bottomPanel := container.NewBorder(
		nil, nil,
		app.counterLabel,
		statusContainer,
	)

	// ---------- левая панель ----------
	modsArea := container.NewBorder(
		container.NewVBox(
			topPanelWithBg,
			app.managePanel,
			app.headerTable,
		),
		nil, nil, nil,
		container.NewBorder(
			container.NewVBox(systemTableContainer), // верх – системная таблица (не растягивается, запрашивает свою полную высоту)
			nil, nil, nil,
			app.tableBorderContainer,				// центр – основная таблица растянется на всё оставшееся место
		),
	)

	leftPanel := container.NewBorder(
		nil,
		bottomPanel,
		nil, nil,
		modsArea,
	)

 	// ---------- описание ----------
    app.descTitle = widget.NewLabel(app.messages["select_mod"])
    app.descTitle.TextStyle = fyne.TextStyle{Bold: true}
    app.descAuthor = widget.NewLabel("—")
	app.descInstalled = widget.NewLabel("")
    app.descBody = widget.NewLabel(app.messages["desc_placeholder"])
    app.descBody.Wrapping = fyne.TextWrapWord
    app.descURL = widget.NewHyperlink("", nil)

    // Карточка с обводкой (как у кнопок)
    descCardBg := canvas.NewRectangle(color.NRGBA{R: 45, G: 45, B: 50, A: 255})
    descCardBg.CornerRadius = 12
    descCardBg.StrokeWidth = 0.5
    descCardBg.StrokeColor = color.NRGBA{R: 0, G: 0, B: 0, A: 155}

    descHeader := container.NewBorder(
        nil, nil, nil, nil,
        container.NewVBox(
            app.descTitle,
            container.NewHBox(
                app.descAuthor,
                widget.NewLabel(" 👁‍🗨"),
                app.descURL,
            ),
        ),
    )

    descScroll := container.NewScroll(app.descBody)
    descScroll.SetMinSize(fyne.NewSize(config.DescScrollMinWidth, config.DescScrollMinHeight))

    descCardContent := container.NewVBox(
        descHeader,
        widget.NewSeparator(),
        descScroll,
    )

    descCard := container.NewStack(
        descCardBg,
        container.NewPadded(descCardContent),
    )

	// ---------- кнопки правой панели ----------
	app.btnSortChecks = NewCustomButton(app.messages["btn_sort_checks"], func() { go app.runAllChecks() })
	app.applyTooltip(app.btnSortChecks, "btn_sort_checks_tooltip")
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
		fd.Resize(fyne.NewSize(config.FileDialogWidth, config.FileDialogHeight))
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
			app.appendLog("Cannot delete system folder.")
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
	app.applyTooltip(app.btnRemove, "btn_remove_tooltip")

	// ---------- кнопки запуска ----------
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
	if gameVer == VersionUnknown {
		app.btnLaunchNormal.Hide()
		app.btnLaunchNoLauncher.Hide()
	}

    // ---------- правая панель ----------
    topRight := container.NewVBox(
        container.NewHBox(app.btnSortChecks, app.btnInstall, app.btnRemove),
        container.NewHBox(app.btnLaunchNormal, app.btnLaunchNoLauncher, app.btnToggle),
    )

    rightContent := container.NewVSplit(descCard, app.consoleScroll)
    rightContent.Offset = 0.65

	rightPanel := container.NewBorder(topRight, nil, nil, nil, rightContent)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = config.SplitOffset

	content := container.NewBorder(nil, nil, nil, nil, split)
	app.mainWindow.SetContent(content)

	app.registerShortcuts()

	app.appendCenteredLog(app.messages["log_start0"])
	app.filterModList()

	go app.blinkCheckSortIfNeeded()
	app.updateTableBorder()
}

// ---------- логирование ----------
func (app *App) appendLog(text string) {
	if app.logWindow == nil {
		if app.logFile != nil {
			fmt.Fprintln(app.logFile, time.Now().Format("15:04:05"), text)
		}
		return
	}
	fyne.Do(func() {
		// Добавляем новый сегмент с текстом и цветом
		seg := &widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForegroundOnWarning,
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
		fmt.Fprintln(app.logFile, time.Now().Format("15:04:05"), text)
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
	app.descAuthor.SetText(fmt.Sprintf(app.messages["author_label"], author))
	app.descInstalled.SetText(fmt.Sprintf(app.messages["installed_label"], app.formatDate(mod.ModTime, app.cfg.DateFormat)))

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
	// Блокируем всё для системных папок
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

// ----- ФИЛЬТРАЦИЯ С ПРЕДИКАТАМИ -----

type modFilterFunc func(checks.ModInfo) bool

func (app *App) filterModList() {
	if app.modTable == nil {
		return
	}
	if app.filterSelect == nil {
		app.displayedMods = app.allMods
		if app.modTable != nil {
			app.modTable.Length = func() (int, int) { return len(app.displayedMods), config.TableColumnCount }
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
		app.messages["filter_all"]:	  func(m checks.ModInfo) bool { return true },
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
		app.modTable.Length = func() (int, int) { return len(app.displayedMods), config.TableColumnCount }

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

	// Обновление счётчика модов
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

// анимация мерцания кнопки "Сохранить"
func (app *App) startBlinkSaveButton() {
    if app.blinkSaveOrderActive {
        return
    }
    app.blinkSaveOrderActive = true
    go func() {
        for app.blinkSaveOrderActive && app.orderDirty {
            fyne.Do(func() {
                app.btnSaveOrder.Importance = widget.WarningImportance
                app.btnSaveOrder.Refresh()
            })
            time.Sleep(600 * time.Millisecond)
            fyne.Do(func() {
                app.btnSaveOrder.Importance = widget.MediumImportance
                app.btnSaveOrder.Refresh()
            })
            time.Sleep(1000 * time.Millisecond)
        }
        fyne.Do(func() {
            app.btnSaveOrder.Importance = widget.MediumImportance
            app.btnSaveOrder.Refresh()
        })
    }()
}

func (app *App) stopBlinkSaveButton() {
	app.blinkSaveOrderActive = false
}

// анимация мерцания кнопки "Checks and sorting"
func (app *App) startBlinkCheckSortButton() {
    if app.blinkCheckSortActive {
        return
    }
    app.blinkCheckSortActive = true
    go func() {
        for app.blinkCheckSortActive {
            fyne.Do(func() {
                app.btnSortChecks.Importance = widget.WarningImportance
                app.btnSortChecks.Refresh()
            })
            time.Sleep(600 * time.Millisecond)
            fyne.Do(func() {
                app.btnSortChecks.Importance = widget.MediumImportance
                app.btnSortChecks.Refresh()
            })
            time.Sleep(1000 * time.Millisecond)
        }
        fyne.Do(func() {
            app.btnSortChecks.Importance = widget.MediumImportance
            app.btnSortChecks.Refresh()
        })
    }()
}

func (app *App) stopBlinkCheckSortButton() {
	app.blinkCheckSortActive = false
}

// фоновая проверка, нужно ли мигать кнопкой проверки
func (app *App) blinkCheckSortIfNeeded() {
	for {
		time.Sleep(2 * time.Second)
		needBlink := false
		// проверяем критерии
		if app.orderDirty {
			needBlink = true
		}
		// здесь можно добавить проверку конфликтов, устаревших модов и т.д.
		if !needBlink && app.blinkCheckSortActive {
			app.stopBlinkCheckSortButton()
		} else if needBlink && !app.blinkCheckSortActive {
			app.startBlinkCheckSortButton()
		}
	}
}

// обновление красной рамки таблицы
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

// registerShortcuts добавляет горячие клавиши окну.
func (app *App) registerShortcuts() {
	c := app.mainWindow.Canvas()
	if c == nil {
		return
	}
	// Ctrl+S – сохранить порядок
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyS, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.btnSaveOrder.OnTapped()
	})
	// Alt+R – обновить список
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyR, Modifier: fyne.KeyModifierAlt}, func(shortcut fyne.Shortcut) {
		app.btnRefresh.OnTapped()
	})
	// Ctrl+I – установить мод
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyI, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.btnInstall.OnTapped()
	})
	// Delete – удалить мод
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDelete, Modifier: 0}, func(shortcut fyne.Shortcut) {
		app.btnRemove.OnTapped()
	})
	// Ctrl+T – глобальное вкл/выкл модов
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyT, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.btnToggle.OnTapped()
	})
	// Ctrl+Shift+C – проверки и сортировка
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyC, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(shortcut fyne.Shortcut) {
		app.btnSortChecks.OnTapped()
	})
	// Alt+Up / Alt+Down – перемещение мода
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyUp, Modifier: fyne.KeyModifierAlt}, func(shortcut fyne.Shortcut) {
		app.btnUp.OnTapped()
	})
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDown, Modifier: fyne.KeyModifierAlt}, func(shortcut fyne.Shortcut) {
		app.btnDown.OnTapped()
	})
	// Ctrl+Home – в начало
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyHome, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.moveToTopBtn.OnTapped()
	})
	// Ctrl+End – в конец
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEnd, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.moveToBottomBtn.OnTapped()
	})
	// Ctrl+F – фокус на поиск
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		if app.searchEntry != nil {
			app.mainWindow.Canvas().Focus(app.searchEntry)
		}
	})
	// Ctrl+A – выбрать все
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyA, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.selectAllBtn.OnTapped()
	})
	// Ctrl+D – снять выделение
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyD, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.deselectAllBtn.OnTapped()
	})
	// Ctrl+E – включить выделенные
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.enableSelectedBtn.OnTapped()
	})
	// Ctrl+Shift+E – выключить выделенные
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyE, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(shortcut fyne.Shortcut) {
		app.disableSelectedBtn.OnTapped()
	})
	// Ctrl+L – обычный запуск
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		if app.btnLaunchNormal != nil {
			app.btnLaunchNormal.OnTapped()
		}
	})
	// Ctrl+Shift+L – быстрый запуск
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyL, Modifier: fyne.KeyModifierControl | fyne.KeyModifierShift}, func(shortcut fyne.Shortcut) {
		if app.btnLaunchNoLauncher != nil {
			app.btnLaunchNoLauncher.OnTapped()
		}
	})
	// Ctrl+M – показать/скрыть панель управления
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyM, Modifier: fyne.KeyModifierControl}, func(shortcut fyne.Shortcut) {
		app.manageBtn.OnTapped()
	})
	// Escape – очистить поиск
	c.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyEscape, Modifier: 0}, func(shortcut fyne.Shortcut) {
		if app.searchEntry != nil {
			app.searchEntry.SetText("")
		}
	})
}

// applyTooltip настраивает колбеки кнопки для работы со статус‑лейблом.
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
