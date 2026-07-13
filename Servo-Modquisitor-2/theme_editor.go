// theme_editor.go
package main

import (
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/lusingander/colorpicker"
)

// colorEntry describes one editable color.
type colorEntry struct {
	Key   string
	Label string
	Group string
}

// allColorEntries returns the complete list of editable colors.
func allColorEntries() []colorEntry {
	return []colorEntry{
		// Basic
		{Key: string(theme.ColorNameBackground), Label: "Background", Group: "Basic"},
		{Key: string(theme.ColorNameForeground), Label: "Foreground", Group: "Basic"},
		{Key: string(theme.ColorNamePrimary), Label: "Primary", Group: "Basic"},
		{Key: string(theme.ColorNameHyperlink), Label: "Hyperlink", Group: "Basic"},
		{Key: string(theme.ColorNameHeaderBackground), Label: "Header Background", Group: "Basic"},
		{Key: string(theme.ColorNameInputBackground), Label: "Input Background", Group: "Basic"},
		{Key: string(theme.ColorNameInputBorder), Label: "Input Border", Group: "Basic"},
		{Key: string(theme.ColorNameSeparator), Label: "Separator", Group: "Basic"},
		{Key: string(theme.ColorNameShadow), Label: "Shadow", Group: "Basic"},
		{Key: string(theme.ColorNameMenuBackground), Label: "Menu Background", Group: "Basic"},
		{Key: string(theme.ColorNameOverlayBackground), Label: "Overlay Background", Group: "Basic"},
		{Key: string(theme.ColorNameScrollBar), Label: "ScrollBar", Group: "Basic"},
		{Key: string(theme.ColorNameScrollBarBackground), Label: "ScrollBar Background", Group: "Basic"},
		{Key: string(theme.ColorNameSelection), Label: "Selection", Group: "Basic"},
		{Key: string(theme.ColorNameForegroundOnWarning), Label: "Warning Foreground", Group: "Basic"},

		// Buttons
		{Key: string(theme.ColorNameButton), Label: "Button", Group: "Buttons"},
		{Key: string(theme.ColorNameHover), Label: "Button Hover", Group: "Buttons"},
		{Key: string(theme.ColorNamePressed), Label: "Button Pressed", Group: "Buttons"},
		{Key: string(theme.ColorNameDisabledButton), Label: "Disabled Button", Group: "Buttons"},
		{Key: string(theme.ColorNameFocus), Label: "Focus", Group: "Buttons"},
		{Key: string(themes.ColorButtonShadow), Label: "Button Shadow", Group: "Buttons"},
		{Key: string(themes.ColorButtonShadowDisabled), Label: "Button Shadow Disabled", Group: "Buttons"},
		{Key: string(themes.ColorButtonStroke), Label: "Button Stroke", Group: "Buttons"},
		{Key: string(themes.ColorButtonStrokeImage), Label: "Button Stroke Image", Group: "Buttons"},

		// Text / Messages
		{Key: string(theme.ColorNameError), Label: "Error", Group: "Text"},
		{Key: string(theme.ColorNameDisabled), Label: "Disabled Text", Group: "Text"},
		{Key: string(theme.ColorNamePlaceHolder), Label: "Placeholder", Group: "Text"},

		// Statuses
		{Key: string(themes.ColorStatusSystem), Label: "Status: System", Group: "Statuses"},
		{Key: string(themes.ColorStatusBroken), Label: "Status: Broken", Group: "Statuses"},
		{Key: string(themes.ColorStatusConflict), Label: "Status: Conflict", Group: "Statuses"},
		{Key: string(themes.ColorStatusObsolete), Label: "Status: Obsolete", Group: "Statuses"},
		{Key: string(themes.ColorStatusMandatory), Label: "Status: Mandatory", Group: "Statuses"},
		{Key: string(themes.ColorStatusActive), Label: "Status: Active", Group: "Statuses"},
		{Key: string(themes.ColorStatusInactive), Label: "Status: Inactive", Group: "Statuses"},
		{Key: string(themes.ColorStatusVortex), Label: "Status: Vortex", Group: "Statuses"},
		{Key: string(themes.ColorStatusMissing), Label: "Status: Missing", Group: "Statuses"},
		{Key: string(themes.ColorStatusSymlink), Label: "Status: Symlink", Group: "Statuses"},
		{Key: string(themes.ColorStatusManual), Label: "Status: Manual", Group: "Statuses"},
		{Key: string(themes.ColorStatusNexus), Label: "Status: Nexus", Group: "Statuses"},

		// Table
		{Key: string(themes.ColorTableRowEven), Label: "Row Even", Group: "Table"},
		{Key: string(themes.ColorTableRowOdd), Label: "Row Odd", Group: "Table"},
		{Key: string(themes.ColorTableRowSelected), Label: "Row Selected", Group: "Table"},
		{Key: string(themes.ColorTableRowConflict), Label: "Row Conflict", Group: "Table"},
		{Key: string(themes.ColorTableBorderDirty), Label: "Border Dirty", Group: "Table"},
		{Key: string(themes.ColorTableHeaderBg), Label: "Header BG", Group: "Table"},
		{Key: string(themes.ColorSystemTableBg), Label: "System Table BG", Group: "Table"},
		{Key: string(themes.ColorTableObsoleteMod), Label: "Obsolete Mod BG", Group: "Table"},
		{Key: string(themes.ColorTableHasUpdateMod), Label: "Has Update BG", Group: "Table"},
		{Key: string(themes.ColorTableMissingFolder), Label: "Missing Folder BG", Group: "Table"},
		{Key: string(themes.ColorStatusSymlinkBg), Label: "Symlink BG", Group: "Table"},

		// Console
		{Key: string(themes.ColorConsoleText), Label: "Console Text", Group: "Console"},
		{Key: string(themes.ColorCRTScreenFill), Label: "CRT Fill", Group: "Console"},
		{Key: string(themes.ColorCRTScreenStroke), Label: "CRT Stroke", Group: "Console"},
		{Key: string(themes.ColorCRTHeaderBg), Label: "CRT Header", Group: "Console"},

		// Panels / Cards
		{Key: string(themes.ColorDescCardStroke), Label: "Card Stroke", Group: "Panels"},
		{Key: string(themes.ColorDescCardBg), Label: "Card BG", Group: "Panels"},
		{Key: string(themes.ColorManagePanelBg), Label: "Manage Panel BG", Group: "Panels"},
		{Key: string(themes.ColorTopPanelBg), Label: "Top Panel BG", Group: "Panels"},
		{Key: string(themes.ColorTipBg), Label: "Tip BG", Group: "Panels"},
	}
}

// groupOptions returns unique group names for the filter dropdown.
func groupOptions() []string {
	m := map[string]bool{}
	for _, e := range allColorEntries() {
		m[e.Group] = true
	}
	groups := make([]string, 0, len(m))
	for g := range m {
		groups = append(groups, g)
	}
	return groups
}

// getColorFromMap returns the color from the map, or black if not found.
func getColorFromMap(m map[string]color.Color, key string) color.Color {
	if c, ok := m[key]; ok {
		return c
	}
	return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
}

// hexFromColor возвращает #RRGGBBAA (8 символов)
func hexFromColor(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// colorFromHex парсит #RRGGBB или #RRGGBBAA
func colorFromHex(hex string) (color.NRGBA, error) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) == 6 {
		var r, g, b uint8
		_, err := fmt.Sscanf(hex, "%02X%02X%02X", &r, &g, &b)
		if err == nil {
			return color.NRGBA{R: r, G: g, B: b, A: 255}, nil
		}
	} else if len(hex) == 8 {
		var r, g, b, a uint8
		_, err := fmt.Sscanf(hex, "%02X%02X%02X%02X", &r, &g, &b, &a)
		if err == nil {
			return color.NRGBA{R: r, G: g, B: b, A: a}, nil
		}
	}
	return color.NRGBA{}, fmt.Errorf("invalid hex")
}

// showThemeEditor opens the theme editor window.
func (app *App) showThemeEditor() {
	// Build a working copy of colors from the current config.
	currentTheme := app.myApp.Settings().Theme()
	currentColors := make(map[string]color.Color)
	entries := allColorEntries()
	for _, e := range entries {
		var col color.Color
		if app.cfg.CustomColors != nil && app.cfg.CustomColors[e.Key] != (color.NRGBA{}) {
			col = app.cfg.CustomColors[e.Key]
		} else {
			col = currentTheme.Color(fyne.ThemeColorName(e.Key), app.myApp.Settings().ThemeVariant())
		}
		currentColors[e.Key] = col
	}

	// ── Сохраняем исходные цвета для Reset ──
	originalColors := make(map[string]color.Color)
	for k, v := range currentColors {
		originalColors[k] = v
	}

	win := app.myApp.NewWindow("Theme Editor")
	win.Resize(fyne.NewSize(1200, 800))

	searchText := ""
	filterGroup := ""
	selectedKey := ""
	if len(entries) > 0 {
		selectedKey = entries[0].Key
	}

	var displayedEntries []colorEntry
	var colorList *widget.Table
	var previewContainer *fyne.Container
	var colorPicker colorpicker.ColorPicker
	var colorNameLabel *widget.Label

	// ── Флаг для предотвращения рекурсии ──
	var updating bool

	// ── Виджеты правой панели ──
	colorNameLabel = widget.NewLabel("")
	sampleRect := canvas.NewRectangle(color.Transparent)
	sampleRect.SetMinSize(fyne.NewSize(60, 40))

	hexEntry := widget.NewEntry()
	hexEntry.Validator = validation.NewRegexp(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`, "Invalid hex (#RRGGBB or #RRGGBBAA)")
	hexEntry.SetPlaceHolder("#RRGGBB")

	rLabel := widget.NewLabel("255")
	gLabel := widget.NewLabel("255")
	bLabel := widget.NewLabel("255")
	aLabel := widget.NewLabel("255")

	// ── Сохраняемый текущий цвет пикера ──
	var currentPickerColor color.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	// Функция обновления отображения цвета (образец, hex, RGB)
	updateColorDisplay := func(c color.Color) {
		currentPickerColor = c
		r, g, b, a := c.RGBA()
		r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
		sampleRect.FillColor = c
		sampleRect.Refresh()
		hexEntry.SetText(hexFromColor(c))
		rLabel.SetText(fmt.Sprintf("%d", r8))
		gLabel.SetText(fmt.Sprintf("%d", g8))
		bLabel.SetText(fmt.Sprintf("%d", b8))
		aLabel.SetText(fmt.Sprintf("%d", a8))
	}

	// Функция обновления редактора при выборе цвета в списке
	updateEditorForSelected := func(key string) {
		if updating || key == "" {
			return
		}
		updating = true
		defer func() { updating = false }()

		selectedKey = key
		col := getColorFromMap(currentColors, key)
		r, g, b, _ := col.RGBA()
		// Используем альфа из currentPickerColor, если он есть
		alpha := uint8(255)
		if currentPickerColor != nil {
			_, _, _, a := currentPickerColor.RGBA()
			alpha = uint8(a >> 8)
		}
		newCol := color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: alpha}
		currentPickerColor = newCol
		updateColorDisplay(newCol)
		if colorPicker != nil {
			colorPicker.SetColor(newCol)
			colorPicker.Refresh()
		}
		if colorNameLabel != nil {
			for _, e := range entries {
				if e.Key == key {
					colorNameLabel.SetText(e.Label)
					break
				}
			}
		}
	}

	// Функция построения превью
	buildPreview := func() fyne.CanvasObject {
		bgColor := getColorFromMap(currentColors, string(theme.ColorNameBackground))
		fgColor := getColorFromMap(currentColors, string(theme.ColorNameForeground))
		btnHover := getColorFromMap(currentColors, string(theme.ColorNameHover))
		btnPressed := getColorFromMap(currentColors, string(theme.ColorNamePressed))

		bgRect := canvas.NewRectangle(bgColor)

		// ---- Заголовок ----
		headerLabel := canvas.NewText("Preview", fgColor)
		headerLabel.TextSize = 16
		headerLabel.TextStyle = fyne.TextStyle{Bold: true}
		headerLabel.Alignment = fyne.TextAlignCenter

		// ---- Ряд кнопок ----
		normalBtn := widget.NewButton("Normal", nil)
		hoverRect := canvas.NewRectangle(btnHover)
		hoverRect.SetMinSize(fyne.NewSize(70, 30))
		pressedRect := canvas.NewRectangle(btnPressed)
		pressedRect.SetMinSize(fyne.NewSize(70, 30))
		disabledBtn := widget.NewButton("Disabled", nil)
		disabledBtn.Disable()

		btnRow := container.NewHBox(
			normalBtn,
			container.NewStack(hoverRect, widget.NewLabel("Hover")),
			container.NewStack(pressedRect, widget.NewLabel("Pressed")),
			disabledBtn,
		)

		// ---- Поле ввода с Placeholder (размещаем справа от кнопок) ----
		placeholderEntry := widget.NewEntry()
		placeholderEntry.SetPlaceHolder("Placeholder text longer")
		placeholderSpacer := canvas.NewRectangle(color.Transparent)
		placeholderSpacer.SetMinSize(fyne.NewSize(200, 0))
		placeholderContainerLocal := container.NewStack(placeholderSpacer, placeholderEntry)

		topRowButtons := container.NewHBox(
			btnRow,
			container.NewPadded(placeholderContainerLocal),
		)

		// ---- Данные для таблицы ----
		type rowData struct {
			label    string
			colorKey string
			checkbox bool
		}
		rows := []rowData{
			{"Even", themes.ColorTableRowEven, true},
			{"Odd", themes.ColorTableRowOdd, false},
			{"Selected", themes.ColorTableRowSelected, true},
			{"Conflict", themes.ColorTableRowConflict, true},
			{"Obsolete", themes.ColorTableObsoleteMod, true},
			{"Has Update", themes.ColorTableHasUpdateMod, true},
			{"Missing", themes.ColorTableMissingFolder, true},
		}

		statusKeys := []string{
			themes.ColorStatusActive,
			themes.ColorStatusInactive,
			themes.ColorStatusBroken,
			themes.ColorStatusConflict,
			themes.ColorStatusObsolete,
			themes.ColorStatusMissing,
			themes.ColorStatusVortex,
			themes.ColorStatusSymlink,
			themes.ColorStatusManual,
		}
		statusLabels := []string{
			"Active", "Inactive", "Broken", "Conflict",
			"Obsolete", "Missing", "Vortex", "Symlink",
			"Manual",
		}

		numCols := 2 + len(statusKeys)

		table := widget.NewTable(
			func() (int, int) { return len(rows) + 1, numCols },
			func() fyne.CanvasObject {
				spacer := canvas.NewRectangle(color.Transparent)
				spacer.SetMinSize(fyne.NewSize(1, 30))
				return container.NewStack(spacer)
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				stack := cell.(*fyne.Container)
				var spacer fyne.CanvasObject
				if len(stack.Objects) > 0 {
					spacer = stack.Objects[0]
				}
				stack.Objects = nil
				if spacer != nil {
					stack.Add(spacer)
				} else {
					s := canvas.NewRectangle(color.Transparent)
					s.SetMinSize(fyne.NewSize(1, 30))
					stack.Add(s)
				}

				if id.Row == 0 {
					bg := canvas.NewRectangle(getColorFromMap(currentColors, themes.ColorTableHeaderBg))
					stack.Add(bg)
					text := canvas.NewText("", fgColor)
					text.TextStyle = fyne.TextStyle{Bold: true}
					switch id.Col {
					case 0:
						text.Text = "✔"
					case 1:
						text.Text = "Color"
					default:
						idx := id.Col - 2
						if idx < len(statusLabels) {
							text.Text = statusLabels[idx]
						}
					}
					stack.Add(text)
					return
				}

				rowIdx := id.Row - 1
				if rowIdx >= len(rows) {
					return
				}
				row := rows[rowIdx]

				bg := canvas.NewRectangle(getColorFromMap(currentColors, row.colorKey))
				stack.Add(bg)

				switch id.Col {
				case 0:
					check := widget.NewCheck("", nil)
					check.SetChecked(row.checkbox)
					check.OnChanged = func(b bool) {}
					stack.Add(check)
				case 1:
					text := canvas.NewText(row.label, fgColor)
					stack.Add(text)
				default:
					idx := id.Col - 2
					if idx < len(statusKeys) {
						statusColor := getColorFromMap(currentColors, statusKeys[idx])
						text := canvas.NewText(statusLabels[idx], statusColor)
						stack.Add(text)
					}
				}
				stack.Refresh()
			},
		)

		table.SetColumnWidth(0, 40)
		table.SetColumnWidth(1, 100)
		for i := 0; i < len(statusKeys); i++ {
			table.SetColumnWidth(2+i, 80)
		}
		table.SetRowHeight(-1, 30)

		tableScroll := container.NewVScroll(table)
		tableScroll.SetMinSize(fyne.NewSize(0, 220))

		// ---- Консоль (имитация) ----
		consoleHeader := canvas.NewText("Console", fgColor)
		consoleHeader.TextSize = 14
		consoleHeader.TextStyle = fyne.TextStyle{Bold: true}
		consoleHeader.Alignment = fyne.TextAlignCenter

		consoleBg := canvas.NewRectangle(getColorFromMap(currentColors, themes.ColorCRTScreenFill))
		consoleBg.StrokeColor = getColorFromMap(currentColors, themes.ColorCRTScreenStroke)
		consoleBg.StrokeWidth = 1

		consoleText := canvas.NewText("> Ready", getColorFromMap(currentColors, themes.ColorConsoleText))
		consoleText.Alignment = fyne.TextAlignLeading

		consolePanel := container.NewBorder(
			consoleHeader,
			nil, nil, nil,
			container.NewPadded(consoleText),
		)

		consoleSpacer := canvas.NewRectangle(color.Transparent)
		consoleSpacer.SetMinSize(fyne.NewSize(0, 80))
		consoleStack := container.NewStack(consoleSpacer, consoleBg, consolePanel)

		// ---- Сборка всей превью ----
		top := container.NewVBox(
			headerLabel,
			topRowButtons,
			widget.NewSeparator(),
			tableScroll,
			consoleStack,
		)

		return container.NewStack(bgRect, container.NewPadded(top))
	}

	refreshPreview := func() {
		if previewContainer != nil {
			newPreview := buildPreview()
			previewContainer.Objects = []fyne.CanvasObject{newPreview}
			previewContainer.Refresh()
		}
	}

	// ── Левая таблица ──────────────────────────────────────────────
	colorList = widget.NewTable(
		func() (int, int) { return len(displayedEntries), 2 },
		func() fyne.CanvasObject {
			rect := canvas.NewRectangle(color.Transparent)
			rect.SetMinSize(fyne.NewSize(30, 20))
			label := widget.NewLabel("")
			label.Wrapping = fyne.TextWrapOff
			label.Alignment = fyne.TextAlignLeading
			labelBg := canvas.NewRectangle(color.Transparent)
			labelBg.SetMinSize(fyne.NewSize(170, 20))
			stack := container.NewStack(labelBg, label)
			return container.NewHBox(rect, stack)
		},
		func(id widget.TableCellID, obj fyne.CanvasObject) {
			if id.Row >= len(displayedEntries) {
				return
			}
			entry := displayedEntries[id.Row]
			hbox := obj.(*fyne.Container)
			hbox.Objects = nil

			if id.Col == 0 {
				rect := canvas.NewRectangle(color.Transparent)
				rect.SetMinSize(fyne.NewSize(130, 20))
				col := getColorFromMap(currentColors, entry.Key)
				rect.FillColor = col
				hbox.Add(rect)
			} else {
				label := widget.NewLabel(entry.Label)
				label.Wrapping = fyne.TextWrapOff
				label.Alignment = fyne.TextAlignLeading
				label.TextStyle = fyne.TextStyle{Bold: true}
				labelBg := canvas.NewRectangle(color.Transparent)
				labelBg.SetMinSize(fyne.NewSize(170, 20))
				stack := container.NewStack(labelBg, label)
				hbox.Add(stack)
			}
			hbox.Refresh()
		},
	)
	colorList.SetColumnWidth(0, 130)
	colorList.SetColumnWidth(1, 220)
	colorList.SetRowHeight(-1, 30)
	colorList.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(displayedEntries) {
			updateEditorForSelected(displayedEntries[id.Row].Key)
		}
	}

	applyFilter := func() {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		displayedEntries = nil
		searchLower := strings.ToLower(searchText)
		for _, e := range entries {
			if filterGroup != "" && e.Group != filterGroup {
				continue
			}
			if searchLower != "" && !strings.Contains(strings.ToLower(e.Label), searchLower) {
				continue
			}
			displayedEntries = append(displayedEntries, e)
		}
		colorList.Length = func() (int, int) { return len(displayedEntries), 2 }
		colorList.Refresh()
		if selectedKey != "" {
			for i, e := range displayedEntries {
				if e.Key == selectedKey {
					colorList.Select(widget.TableCellID{Row: i, Col: 0})
					break
				}
			}
		} else if len(displayedEntries) > 0 {
			updateEditorForSelected(displayedEntries[0].Key)
		}
	}

	// ── Поиск и фильтр ──────────────────────────────────────────────
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Search...")
	searchEntry.OnChanged = func(s string) {
		searchText = s
		applyFilter()
	}
	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(SearchMinWidth, 1))
	searchEntryBox := container.NewStack(searchSpacer, searchEntry)

	groupSelect := widget.NewSelect(groupOptions(), func(s string) {
		filterGroup = s
		applyFilter()
	})
	groupSelect.SetSelected("")

	filterSpacer := canvas.NewRectangle(color.Transparent)
	filterSpacer.SetMinSize(fyne.NewSize(AMLFilterMinWidth, 1))
	filterSelectWithSize := container.NewStack(filterSpacer, groupSelect)

	filterBox := container.NewHBox(
		widget.NewLabel("Search:"),
		searchEntryBox,
		widget.NewLabel("Group:"),
		filterSelectWithSize,
	)

	// ── Правая панель ──────────────────────────────────────────────
	// HEX ввод
	hexEntry.OnChanged = func(s string) {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		if c, err := colorFromHex(s); err == nil {
			// Если currentPickerColor не nil, берём альфа из него, иначе 255
			alpha := uint8(255)
			if currentPickerColor != nil {
				_, _, _, a := currentPickerColor.RGBA()
				alpha = uint8(a >> 8)
			}
			newCol := color.NRGBA{R: c.R, G: c.G, B: c.B, A: alpha}
			currentPickerColor = newCol
			currentColors[selectedKey] = newCol
			updateColorDisplay(newCol)
			colorPicker.SetColor(newCol)
			colorPicker.Refresh()
			refreshPreview()
			colorList.Refresh()
		}
	}

	// Пикер
	colorPicker = colorpicker.New(200, colorpicker.StyleHue)
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(200, 200))
	pickerWrapper := container.NewStack(spacer, colorPicker)

	colorPicker.SetOnChanged(func(c color.Color) {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		if selectedKey == "" {
			return
		}
		currentPickerColor = c // сохраняем
		currentColors[selectedKey] = c
		updateColorDisplay(c)
		refreshPreview()
		colorList.Refresh()
	})

	// Правая колонка (hex + RGB)
	hexLabel := widget.NewLabel("Hex")
	rgbLabel := widget.NewLabel("RGB")
	rgbLabel.TextStyle = fyne.TextStyle{Bold: true}

	hexBox := container.NewVBox(hexLabel, hexEntry)
	rgbBox := container.NewVBox(
		rgbLabel,
		container.NewHBox(
			widget.NewLabel("R:"), rLabel,
			widget.NewLabel("  G:"), gLabel,
			widget.NewLabel("  B:"), bLabel,
			widget.NewLabel("  A:"), aLabel,
		),
	)
	rightColumn := container.NewVBox(hexBox, widget.NewSeparator(), rgbBox)

	// Горизонтальный сплит: пикер слева, колонка справа
	pickerSplit := container.NewHSplit(pickerWrapper, rightColumn)
	pickerSplit.Offset = 0.6

	topRow := container.NewHBox(sampleRect, container.NewPadded(colorNameLabel))
	pickerBox := container.NewVBox(topRow, pickerSplit)

	previewContainer = container.NewStack(buildPreview())

	// ── Кнопки ──────────────────────────────────────────────────────
	applyBtn := widget.NewButton("Apply", func() {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		if app.cfg.CustomColors == nil {
			app.cfg.CustomColors = make(map[string]color.NRGBA)
		}
		for k, c := range currentColors {
			r, g, b, a := c.RGBA()
			app.cfg.CustomColors[k] = color.NRGBA{
				R: uint8(r >> 8),
				G: uint8(g >> 8),
				B: uint8(b >> 8),
				A: uint8(a >> 8),
			}
		}
		app.cfg.Theme = "custom"
		saveConfig(app.cfg)
		colors := make(map[string]color.Color)
		for k, v := range app.cfg.CustomColors {
			colors[k] = v
		}
		app.myApp.Settings().SetTheme(&themes.CustomTheme{Colors: colors})
		app.refreshThemeColors()
		app.mainWindow.Canvas().Refresh(app.mainWindow.Content())
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	})

	resetBtn := widget.NewButton("Reset to Default", func() {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		// Сброс к originalColors
		for k, v := range originalColors {
			currentColors[k] = v
		}
		refreshPreview()
		colorList.Refresh()
		if selectedKey != "" {
			updateEditorForSelected(selectedKey)
		}
	})

	closeBtn := widget.NewButton("Close", func() {
		win.Close()
	})

	btnBox := container.NewHBox(applyBtn, resetBtn, closeBtn)

	// ── Сборка макета ──────────────────────────────────────────────
	leftPanel := container.NewBorder(filterBox, nil, nil, nil, colorList)

	previewScroll := container.NewScroll(previewContainer)
	previewScroll.SetMinSize(fyne.NewSize(400, 300))

	topRowWithButtons := container.NewHBox(btnBox)
	topBar := container.NewVBox(
		topRowWithButtons,
		widget.NewSeparator(),
	)

	contentArea := container.NewBorder(
		pickerBox,
		nil, nil, nil,
		previewScroll,
	)

	rightPanel := container.NewBorder(
		topBar,
		nil, nil, nil,
		contentArea,
	)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.35

	win.SetContent(split)
	win.Show()

	applyFilter()
}
