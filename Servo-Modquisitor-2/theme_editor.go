// Servo-Modquisitor-2/theme_editor.go
package main

import (
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
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

// groupOrder — фиксированный порядок групп в выпадашке и списке.
// Группы не из этого среза идут в конец по алфавиту.
var groupOrder = []string{
	"Basic",
	"Buttons",
	"Text",
	"Inputs",
	"Scrollbars",
	"Menus",
	"Statuses",
	"Table",
	"Console",
	"Panels",
}

// allColorEntries returns the complete list of editable colors.
func allColorEntries() []colorEntry {
	return []colorEntry{
		// ─── Basic ─────────────────────────────────────────────
		{Key: string(theme.ColorNameBackground), Label: "Background", Group: "Basic"},
		{Key: string(theme.ColorNameForeground), Label: "Foreground", Group: "Basic"},
		{Key: string(theme.ColorNamePrimary), Label: "Primary", Group: "Basic"},
		{Key: string(theme.ColorNameHyperlink), Label: "Hyperlink", Group: "Basic"},
		{Key: string(theme.ColorNameHeaderBackground), Label: "Header Background", Group: "Basic"},
		{Key: string(theme.ColorNameSeparator), Label: "Separator", Group: "Basic"},
		{Key: string(theme.ColorNameShadow), Label: "Shadow", Group: "Basic"},
		{Key: string(theme.ColorNameSelection), Label: "Selection", Group: "Basic"},

		// ─── Buttons ───────────────────────────────────────────
		{Key: string(theme.ColorNameButton), Label: "Button", Group: "Buttons"},
		{Key: string(theme.ColorNameHover), Label: "Button Hover", Group: "Buttons"},
		{Key: string(theme.ColorNamePressed), Label: "Button Pressed", Group: "Buttons"},
		{Key: string(theme.ColorNameDisabledButton), Label: "Disabled Button", Group: "Buttons"},
		{Key: string(theme.ColorNameFocus), Label: "Focus", Group: "Buttons"},
		{Key: string(themes.ColorButtonShadow), Label: "Button Shadow", Group: "Buttons"},
		{Key: string(themes.ColorButtonShadowDisabled), Label: "Button Shadow Disabled", Group: "Buttons"},
		{Key: string(themes.ColorButtonStroke), Label: "Button Stroke", Group: "Buttons"},
		{Key: string(themes.ColorButtonStrokeImage), Label: "Button Stroke Image", Group: "Buttons"},

		// ─── Text ──────────────────────────────────────────────
		{Key: string(theme.ColorNameError), Label: "Error", Group: "Text"},
		{Key: string(theme.ColorNameDisabled), Label: "Disabled Text", Group: "Text"},
		{Key: string(theme.ColorNamePlaceHolder), Label: "Placeholder", Group: "Text"},
		{Key: string(theme.ColorNameForegroundOnWarning), Label: "Foreground on Warning", Group: "Text"},
		{Key: string(theme.ColorNameForegroundOnError), Label: "Foreground on Error", Group: "Text"},
		{Key: string(theme.ColorNameForegroundOnSuccess), Label: "Foreground on Success", Group: "Text"},
		{Key: string(theme.ColorNameForegroundOnPrimary), Label: "Foreground on Primary", Group: "Text"},
		{Key: string(theme.ColorNameSuccess), Label: "Success", Group: "Text"},
		{Key: string(theme.ColorNameWarning), Label: "Warning", Group: "Text"},
		{Key: string(themes.ColorHighlightData), Label: "Highlight Data", Group: "Text"},

		// ─── Inputs ────────────────────────────────────────────
		{Key: string(theme.ColorNameInputBackground), Label: "Input Background", Group: "Inputs"},
		{Key: string(theme.ColorNameInputBorder), Label: "Input Border", Group: "Inputs"},

		// ─── Scrollbars ────────────────────────────────────────
		{Key: string(theme.ColorNameScrollBar), Label: "ScrollBar", Group: "Scrollbars"},
		{Key: string(theme.ColorNameScrollBarBackground), Label: "ScrollBar Background", Group: "Scrollbars"},

		// ─── Menus ─────────────────────────────────────────────
		{Key: string(theme.ColorNameMenuBackground), Label: "Menu Background", Group: "Menus"},
		{Key: string(theme.ColorNameMenuBorder), Label: "Menu Border", Group: "Menus"},
		{Key: string(theme.ColorNameMenuBarAccent), Label: "Menu Bar Accent", Group: "Menus"},
		{Key: string(theme.ColorNameMenuBarActiveBg), Label: "Menu Bar Active BG", Group: "Menus"},
		{Key: string(theme.ColorNameMenuBarHoverBg), Label: "Menu Bar Hover BG", Group: "Menus"},
		{Key: string(theme.ColorNameMenuItemActiveBorder), Label: "Menu Item Active Border", Group: "Menus"},
		{Key: string(theme.ColorNameMenuItemDanger), Label: "Menu Item Danger", Group: "Menus"},
		{Key: string(theme.ColorNameMenuItemHeader), Label: "Menu Item Header", Group: "Menus"},
		{Key: string(theme.ColorNameMenuItemHeaderBg), Label: "Menu Item Header BG", Group: "Menus"},
		{Key: string(theme.ColorNameOverlayBackground), Label: "Overlay Background", Group: "Menus"},

		// ─── Statuses ──────────────────────────────────────────
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

		// ─── Table ─────────────────────────────────────────────
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

		// ─── Console ───────────────────────────────────────────
		{Key: string(themes.ColorConsoleText), Label: "Console Text", Group: "Console"},
		{Key: string(themes.ColorCRTScreenFill), Label: "CRT Fill", Group: "Console"},
		{Key: string(themes.ColorCRTScreenStroke), Label: "CRT Stroke", Group: "Console"},
		{Key: string(themes.ColorCRTHeaderBg), Label: "CRT Header", Group: "Console"},

		// ─── Panels ────────────────────────────────────────────
		{Key: string(themes.ColorDescCardStroke), Label: "Card Stroke", Group: "Panels"},
		{Key: string(themes.ColorDescCardBg), Label: "Card BG", Group: "Panels"},
		{Key: string(themes.ColorManagePanelBg), Label: "Manage Panel BG", Group: "Panels"},
		{Key: string(themes.ColorTopPanelBg), Label: "Top Panel BG", Group: "Panels"},
		{Key: string(themes.ColorTipBg), Label: "Tip BG", Group: "Panels"},
	}
}

// groupOptions returns unique group names, sorted by groupOrder.
func groupOptions() []string {
	seen := map[string]bool{}
	for _, e := range allColorEntries() {
		seen[e.Group] = true
	}
	groups := make([]string, 0, len(seen))
	for g := range seen {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		pi := indexOfGroup(groupOrder, groups[i])
		pj := indexOfGroup(groupOrder, groups[j])
		if pi == -1 && pj == -1 {
			return groups[i] < groups[j]
		}
		if pi == -1 {
			return false
		}
		if pj == -1 {
			return true
		}
		return pi < pj
	})
	return groups
}

func indexOfGroup(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
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
	nrgba := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02X%02X%02X%02X", nrgba.R, nrgba.G, nrgba.B, nrgba.A)
}

// colorFromHex парсит #RRGGBB или #RRGGBBAA
func colorFromHex(hex string) (color.NRGBA, error) {
	if !strings.HasPrefix(hex, "#") {
		return color.NRGBA{}, fmt.Errorf("invalid hex: missing # prefix")
	}
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

// colorsEqual сравнивает два цвета как NRGBA.
func colorsEqual(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	an := color.NRGBAModel.Convert(a).(color.NRGBA)
	bn := color.NRGBAModel.Convert(b).(color.NRGBA)
	return an == bn
}

// toNRGBA приводит color.Color к color.NRGBA.
func toNRGBA(c color.Color) color.NRGBA {
	return color.NRGBAModel.Convert(c).(color.NRGBA)
}

// showThemeEditor opens the theme editor window.
func (app *App) showThemeEditor() {
	// Снимок конфига.
	app.cfgMutex.RLock()
	cfgCustomColors := make(map[string]color.NRGBA, len(app.cfg.CustomColors))
	for k, v := range app.cfg.CustomColors {
		cfgCustomColors[k] = v
	}
	baseThemeName := app.cfg.CustomBaseTheme
	currentThemeName := app.cfg.Theme
	app.cfgMutex.RUnlock()

	if baseThemeName == "" {
		baseThemeName = "dark"
	}

	activeVariant := app.myApp.Settings().ThemeVariant()

	// Если открываем редактор не из custom — считаем активную тему
	// основой для fallback-цветов.
	if currentThemeName != "custom" {
		switch currentThemeName {
		case "light":
			baseThemeName = "light"
		case "highcontrast":
			baseThemeName = "highcontrast"
		default:
			baseThemeName = "dark"
		}
	}

	baseTheme := pickBaseTheme(baseThemeName)

	entries := allColorEntries()
	currentColors := make(map[string]color.Color, len(entries))
	baseColors := make(map[string]color.Color, len(entries))
	recomputeBaseColors := func(base fyne.Theme) {
		for _, e := range entries {
			baseColors[e.Key] = base.Color(fyne.ThemeColorName(e.Key), activeVariant)
		}
	}
	recomputeBaseColors(baseTheme)

	for _, e := range entries {
		if c, ok := cfgCustomColors[e.Key]; ok {
			currentColors[e.Key] = c
		} else {
			currentColors[e.Key] = baseColors[e.Key]
		}
	}

	// Снимок исходного состояния.
	originalColors := make(map[string]color.Color, len(currentColors))
	for k, v := range currentColors {
		originalColors[k] = v
	}
	originalBase := baseThemeName

	win := app.myApp.NewWindow(app.msg("theme_editor_title"))
	win.Resize(fyne.NewSize(1200, 800))

	searchText := ""
	filterGroup := ""
	selectedKey := ""

	var displayedEntries []colorEntry
	var colorList *widget.Table
	var previewContainer *fyne.Container
	var colorPicker colorpicker.ColorPicker
	var colorNameLabel *widget.Label

	var updating bool

	colorNameLabel = widget.NewLabel("")
	sampleRect := canvas.NewRectangle(color.Transparent)
	sampleRect.SetMinSize(fyne.NewSize(60, 40))

	hexEntry := widget.NewEntry()
	hexEntry.Validator = validation.NewRegexp(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`, app.msg("theme_editor_invalid_hex"))
	hexEntry.SetPlaceHolder("#RRGGBB")

	rLabel := widget.NewLabel("255")
	gLabel := widget.NewLabel("255")
	bLabel := widget.NewLabel("255")
	aLabel := widget.NewLabel("255")

	var currentPickerColor color.Color = color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	updateColorDisplay := func(c color.Color) {
		currentPickerColor = c
		n := toNRGBA(c)
		sampleRect.FillColor = c
		sampleRect.Refresh()
		hexEntry.SetText(hexFromColor(c))
		rLabel.SetText(fmt.Sprintf("%d", n.R))
		gLabel.SetText(fmt.Sprintf("%d", n.G))
		bLabel.SetText(fmt.Sprintf("%d", n.B))
		aLabel.SetText(fmt.Sprintf("%d", n.A))
	}

	updateEditorForSelected := func(key string) {
		if updating || key == "" {
			return
		}
		updating = true
		defer func() { updating = false }()

		selectedKey = key
		col := getColorFromMap(currentColors, key)
		newCol := toNRGBA(col)
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

	buildPreviewInner := func() fyne.CanvasObject {
		bgColor := getColorFromMap(currentColors, string(theme.ColorNameBackground))
		fgColor := getColorFromMap(currentColors, string(theme.ColorNameForeground))
		btnHover := getColorFromMap(currentColors, string(theme.ColorNameHover))
		btnPressed := getColorFromMap(currentColors, string(theme.ColorNamePressed))

		bgRect := canvas.NewRectangle(bgColor)

		headerLabel := canvas.NewText(app.msg("theme_editor_preview"), fgColor)
		headerLabel.TextSize = 16
		headerLabel.TextStyle = fyne.TextStyle{Bold: true}
		headerLabel.Alignment = fyne.TextAlignCenter

		normalBtn := widget.NewButton(app.msg("theme_editor_btn_normal"), nil)
		hoverRect := canvas.NewRectangle(btnHover)
		hoverRect.SetMinSize(fyne.NewSize(70, 30))
		pressedRect := canvas.NewRectangle(btnPressed)
		pressedRect.SetMinSize(fyne.NewSize(70, 30))
		disabledBtn := widget.NewButton(app.msg("theme_editor_btn_disabled"), nil)
		disabledBtn.Disable()

		btnRow := container.NewHBox(
			normalBtn,
			container.NewStack(hoverRect, widget.NewLabel(app.msg("theme_editor_btn_hover"))),
			container.NewStack(pressedRect, widget.NewLabel(app.msg("theme_editor_btn_pressed"))),
			disabledBtn,
		)

		placeholderEntry := widget.NewEntry()
		placeholderEntry.SetPlaceHolder(app.msg("theme_editor_placeholder_text"))
		placeholderSpacer := canvas.NewRectangle(color.Transparent)
		placeholderSpacer.SetMinSize(fyne.NewSize(200, 0))
		placeholderContainerLocal := container.NewStack(placeholderSpacer, placeholderEntry)

		topRowButtons := container.NewHBox(
			btnRow,
			container.NewPadded(placeholderContainerLocal),
		)

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
						text.Text = app.msg("theme_editor_col_color")
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

		consoleHeader := canvas.NewText(app.msg("theme_editor_console"), fgColor)
		consoleHeader.TextSize = 14
		consoleHeader.TextStyle = fyne.TextStyle{Bold: true}
		consoleHeader.Alignment = fyne.TextAlignCenter

		consoleBg := canvas.NewRectangle(getColorFromMap(currentColors, themes.ColorCRTScreenFill))
		consoleBg.StrokeColor = getColorFromMap(currentColors, themes.ColorCRTScreenStroke)
		consoleBg.StrokeWidth = 1

		consoleText := canvas.NewText(app.msg("theme_editor_console_ready"), getColorFromMap(currentColors, themes.ColorConsoleText))
		consoleText.Alignment = fyne.TextAlignLeading

		consolePanel := container.NewBorder(
			consoleHeader,
			nil, nil, nil,
			container.NewPadded(consoleText),
		)

		consoleSpacer := canvas.NewRectangle(color.Transparent)
		consoleSpacer.SetMinSize(fyne.NewSize(0, 80))
		consoleStack := container.NewStack(consoleSpacer, consoleBg, consolePanel)

		// Мини-меню-бар.
		menuBarBg := canvas.NewRectangle(getColorFromMap(currentColors, string(theme.ColorNameMenuBackground)))
		menuBarBg.SetMinSize(fyne.NewSize(0, 28))
		menuBarAccent := canvas.NewRectangle(getColorFromMap(currentColors, string(theme.ColorNameMenuBarAccent)))
		menuBarAccent.SetMinSize(fyne.NewSize(60, 2))

		fileLabel := canvas.NewText(app.msg("theme_editor_menu_file"), getColorFromMap(currentColors, string(theme.ColorNameForeground)))
		editLabel := canvas.NewText(app.msg("theme_editor_menu_edit"), getColorFromMap(currentColors, string(theme.ColorNameForeground)))
		viewLabel := canvas.NewText(app.msg("theme_editor_menu_view"), getColorFromMap(currentColors, string(theme.ColorNameForeground)))

		fileBox := container.NewVBox(
			container.NewPadded(fileLabel),
			menuBarAccent,
		)

		menuItems := container.NewHBox(
			fileBox,
			container.NewPadded(editLabel),
			container.NewPadded(viewLabel),
		)

		menuBar := container.NewStack(menuBarBg, menuItems)

		top := container.NewVBox(
			menuBar,
			headerLabel,
			topRowButtons,
			widget.NewSeparator(),
			tableScroll,
			consoleStack,
		)

		return container.NewStack(bgRect, container.NewPadded(top))
	}

	refreshPreview := func() {
		if previewContainer == nil {
			return
		}
		inner := buildPreviewInner()
		th := &themes.CustomTheme{
			Colors: currentColors,
			Base:   pickBaseTheme(baseThemeName),
		}
		wrapped := container.NewThemeOverride(inner, th)
		previewContainer.Objects = []fyne.CanvasObject{wrapped}
		previewContainer.Refresh()
	}

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

			cur := getColorFromMap(currentColors, entry.Key)
			baseCol := baseColors[entry.Key]
			modified := !colorsEqual(cur, baseCol)

			if id.Col == 0 {
				rect := canvas.NewRectangle(color.Transparent)
				rect.SetMinSize(fyne.NewSize(130, 20))
				rect.FillColor = cur
				hbox.Add(rect)
				return
			}

			displayLabel := entry.Label
			style := fyne.TextStyle{}
			if modified {
				displayLabel = "● " + displayLabel
				style = fyne.TextStyle{Bold: true}
			}
			label := widget.NewLabel(displayLabel)
			label.Wrapping = fyne.TextWrapOff
			label.Alignment = fyne.TextAlignLeading
			label.TextStyle = style
			labelBg := canvas.NewRectangle(color.Transparent)
			labelBg.SetMinSize(fyne.NewSize(170, 20))
			stack := container.NewStack(labelBg, label)
			hbox.Add(stack)
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

		if len(displayedEntries) == 0 {
			return
		}

		targetRow := 0
		if selectedKey != "" {
			found := false
			for i, e := range displayedEntries {
				if e.Key == selectedKey {
					targetRow = i
					found = true
					break
				}
			}
			if !found {
				selectedKey = displayedEntries[0].Key
			}
		} else {
			selectedKey = displayedEntries[0].Key
		}

		// Сброс флага ДО Select, чтобы OnSelected выполнился и
		// заполнил правую панель.
		updating = false
		colorList.Select(widget.TableCellID{Row: targetRow, Col: 0}, 0)
	}

	// applyLiveNow — тяжёлая часть: применяет тему к главному окну.
	// Вызывать только из UI-потока. Меню НЕ пересобираем — после
	// SetTheme listener из NewApp вызовет refreshThemeColors, и Fyne
	// сам обновит существующий MainMenu через Refresh.
	applyLiveNow := func() {
		colors := make(map[string]color.Color, len(currentColors))
		for k, v := range currentColors {
			colors[k] = v
		}
		app.myApp.Settings().SetTheme(&themes.CustomTheme{
			Colors: colors,
			Base:   pickBaseTheme(baseThemeName),
		})
	}

	// applyLive — throttled-обёртка. Движение пикера зовёт OnChanged
	// десятки раз в секунду; без throttle refreshThemeColors
	// захлёбывается. Применяем не чаще, чем раз в liveApplyInterval.
	//
	// Если с последнего применения прошло >= liveApplyInterval —
	// применяем сразу. Иначе планируем отложенный вызов на остаток
	// интервала (один на всё окно, не плодим таймеры).
	const liveApplyInterval = 1000 * time.Millisecond
	var (
		lastApplyTime time.Time
		pendingApply  *time.Timer
	)

	cancelPendingApply := func() {
		if pendingApply != nil {
			pendingApply.Stop()
			pendingApply = nil
		}
	}

	applyLive := func() {
		now := time.Now()
		elapsed := now.Sub(lastApplyTime)
		if elapsed >= liveApplyInterval {
			cancelPendingApply()
			lastApplyTime = now
			applyLiveNow()
			return
		}
		if pendingApply != nil {
			// Отложенный вызов уже запланирован — второй не нужен,
			// он всё равно читает currentColors в момент срабатывания.
			return
		}
		delay := liveApplyInterval - elapsed
		pendingApply = time.AfterFunc(delay, func() {
			fyne.Do(func() {
				pendingApply = nil
				lastApplyTime = time.Now()
				applyLiveNow()
			})
		})
	}

	liveApplyCheck := widget.NewCheck(app.msg("theme_editor_live_apply"), func(b bool) {
		if b {
			applyLive()
		}
	})
	liveApplyCheck.SetToolTip(app.msg("theme_editor_live_apply_tooltip"))

	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(app.msg("theme_editor_search_placeholder"))
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

	baseSelect := widget.NewSelect([]string{"dark", "light", "highcontrast"}, nil)
	baseSelect.SetSelected(baseThemeName)
	baseSelect.OnChanged = func(s string) {
		if s == "" || s == baseThemeName {
			return
		}
		baseThemeName = s
		newBase := pickBaseTheme(baseThemeName)
		recomputeBaseColors(newBase)
		for _, e := range entries {
			if _, ok := cfgCustomColors[e.Key]; !ok {
				currentColors[e.Key] = baseColors[e.Key]
			}
		}
		refreshPreview()
		colorList.Refresh()
		if selectedKey != "" {
			updateEditorForSelected(selectedKey)
		}
		if liveApplyCheck.Checked {
			applyLive()
		}
	}

	filterBox := container.NewHBox(
		widget.NewLabel(app.msg("theme_editor_base")),
		baseSelect,
		widget.NewSeparator(),
		widget.NewLabel(app.msg("theme_editor_search")),
		searchEntryBox,
		widget.NewLabel(app.msg("theme_editor_group")),
		filterSelectWithSize,
	)

	hexEntry.OnChanged = func(s string) {
		if updating {
			return
		}
		updating = true
		defer func() { updating = false }()

		if c, err := colorFromHex(s); err == nil {
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
			if liveApplyCheck.Checked {
				applyLive()
			}
		}
	}

	// Маркеры и шахматка пикера — в цветах текущей темы, чтобы они были
	// видны и в тёмной, и в светлой, и в high-contrast.
	themeForPicker := app.myApp.Settings().Theme()
	variantForPicker := app.myApp.Settings().ThemeVariant()
	fg := themeForPicker.Color(theme.ColorNameForeground, variantForPicker)
	shadow := themeForPicker.Color(theme.ColorNameShadow, variantForPicker)
	bg := themeForPicker.Color(theme.ColorNameBackground, variantForPicker)

	colorpicker.SetDefaultStyle(colorpicker.Style{
		MarkerFill:     color.NRGBA{R: 255, G: 255, B: 255, A: 200},
		MarkerStroke:   shadow,
		CheckerLight:   lighten(bg, 0.15),
		CheckerDark:    lighten(bg, 0.30),
		CheckerBoxSize: 10,
	})
	_ = fg // пока не используется, оставлено для будущей доработки

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
		currentPickerColor = c
		currentColors[selectedKey] = c
		updateColorDisplay(c)
		refreshPreview()
		colorList.Refresh()
		if liveApplyCheck.Checked {
			applyLive()
		}
	})

	hexLabel := widget.NewLabel(app.msg("theme_editor_hex"))
	rgbLabel := widget.NewLabel(app.msg("theme_editor_rgb"))
	rgbLabel.TextStyle = fyne.TextStyle{Bold: true}

	hexBox := container.NewVBox(hexLabel, hexEntry)
	rgbBox := container.NewVBox(
		rgbLabel,
		container.NewHBox(
			widget.NewLabel(app.msg("theme_editor_r")), rLabel,
			widget.NewLabel(app.msg("theme_editor_g")), gLabel,
			widget.NewLabel(app.msg("theme_editor_b")), bLabel,
			widget.NewLabel(app.msg("theme_editor_a")), aLabel,
		),
	)
	rightColumn := container.NewVBox(hexBox, widget.NewSeparator(), rgbBox)

	pickerSplit := container.NewHSplit(pickerWrapper, rightColumn)
	pickerSplit.Offset = 0.6

	topRow := container.NewHBox(sampleRect, container.NewPadded(colorNameLabel))
	pickerBox := container.NewVBox(topRow, pickerSplit)

	previewContainer = container.NewStack()
	refreshPreview()

	// doApply сохраняет override-цвета в cfg и применяет тему.
	doApply := func() {
		base := pickBaseTheme(baseThemeName)
		overrides := make(map[string]color.NRGBA)
		for k, c := range currentColors {
			baseCol := base.Color(fyne.ThemeColorName(k), activeVariant)
			if !colorsEqual(c, baseCol) {
				overrides[k] = toNRGBA(c)
			}
		}

		app.cfgMutex.Lock()
		app.cfg.CustomColors = overrides
		app.cfg.CustomBaseTheme = baseThemeName
		app.cfg.Theme = "custom"
		app.cfgMutex.Unlock()

		app.saveConfigSafe()

		colors := make(map[string]color.Color, len(overrides))
		for k, v := range overrides {
			colors[k] = v
		}
		app.myApp.Settings().SetTheme(&themes.CustomTheme{
			Colors: colors,
			Base:   base,
		})
		app.mainWindow.SetMainMenu(app.buildMainMenu())

		cancelPendingApply()

		// Обновляем снимок.
		for k := range originalColors {
			delete(originalColors, k)
		}
		for k, v := range currentColors {
			originalColors[k] = v
		}
		originalBase = baseThemeName
		cfgCustomColors = make(map[string]color.NRGBA, len(overrides))
		for k, v := range overrides {
			cfgCustomColors[k] = v
		}
	}

	resetThisBtn := widget.NewButton(app.msg("theme_editor_btn_reset_this"), func() {
		if selectedKey == "" {
			return
		}
		base := pickBaseTheme(baseThemeName)
		baseCol := base.Color(fyne.ThemeColorName(selectedKey), activeVariant)
		currentColors[selectedKey] = baseCol
		refreshPreview()
		colorList.Refresh()
		updateEditorForSelected(selectedKey)
	})

	resetAllBtn := widget.NewButton(app.msg("theme_editor_btn_reset_all"), func() {
		dialog.ShowConfirm(
			app.msg("theme_editor_reset_title"),
			app.msg("theme_editor_reset_message"),
			func(ok bool) {
				if !ok {
					return
				}
				base := pickBaseTheme(baseThemeName)
				for _, e := range entries {
					currentColors[e.Key] = base.Color(fyne.ThemeColorName(e.Key), activeVariant)
				}
				refreshPreview()
				colorList.Refresh()
				if selectedKey != "" {
					updateEditorForSelected(selectedKey)
				}
			},
			win,
		)
	})

	resetOriginalBtn := widget.NewButton(app.msg("theme_editor_btn_reset_original"), func() {
		for k, v := range originalColors {
			currentColors[k] = v
		}
		baseThemeName = originalBase
		baseSelect.SetSelected(originalBase)
		recomputeBaseColors(pickBaseTheme(baseThemeName))
		refreshPreview()
		colorList.Refresh()
		if selectedKey != "" {
			updateEditorForSelected(selectedKey)
		}
		if liveApplyCheck.Checked {
			applyLive()
		}
	})

	applyBtn := widget.NewButton(app.msg("theme_editor_btn_apply"), doApply)

	restoreOriginalTheme := func() {
		// Восстанавливаем состояние темы на момент открытия редактора.
		// Нужно, если был включён Live apply — он менял тему без записи
		// в cfg, и при отмене пользователь не должен увидеть «висящие»
		// изменения.
		app.cfgMutex.RLock()
		cfgTheme := app.cfg.Theme
		cfgBase := app.cfg.CustomBaseTheme
		cfgOverrides := make(map[string]color.NRGBA, len(app.cfg.CustomColors))
		for k, v := range app.cfg.CustomColors {
			cfgOverrides[k] = v
		}
		app.cfgMutex.RUnlock()

		switch cfgTheme {
		case "light":
			app.myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
		case "highcontrast":
			app.myApp.Settings().SetTheme(&themes.HighContrastTheme{})
		case "custom":
			colors := make(map[string]color.Color, len(cfgOverrides))
			for k, v := range cfgOverrides {
				colors[k] = v
			}
			app.myApp.Settings().SetTheme(&themes.CustomTheme{
				Colors: colors,
				Base:   pickBaseTheme(cfgBase),
			})
		default:
			app.myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
		}
		app.mainWindow.SetMainMenu(app.buildMainMenu())
	}

	closeBtn := widget.NewButton(app.msg("theme_editor_btn_close"), func() {
		cancelPendingApply()

		hasChanges := baseThemeName != originalBase
		if !hasChanges {
			for k, v := range currentColors {
				orig, ok := originalColors[k]
				if !ok || !colorsEqual(v, orig) {
					hasChanges = true
					break
				}
			}
		}
		if !hasChanges {
			if liveApplyCheck.Checked {
				restoreOriginalTheme()
			}
			win.Close()
			return
		}
		dialog.ShowConfirm(
			app.msg("theme_editor_unsaved_title"),
			app.msg("theme_editor_unsaved_message"),
			func(ok bool) {
				if ok {
					if liveApplyCheck.Checked {
						restoreOriginalTheme()
					}
					win.Close()
				}
			},
			win,
		)
	})

	btnBox := container.NewHBox(
		applyBtn,
		widget.NewSeparator(),
		resetThisBtn, resetAllBtn, resetOriginalBtn,
		widget.NewSeparator(),
		liveApplyCheck,
		widget.NewSeparator(),
		closeBtn,
	)

	win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierControl,
	}, func(_ fyne.Shortcut) {
		doApply()
	})
	win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyEscape,
		Modifier: 0,
	}, func(_ fyne.Shortcut) {
		closeBtn.OnTapped()
	})

	leftPanel := container.NewBorder(filterBox, nil, nil, nil, colorList)

	previewScroll := container.NewScroll(previewContainer)
	previewScroll.SetMinSize(fyne.NewSize(400, 300))

	topBar := container.NewVBox(
		btnBox,
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

// lighten осветляет цвет на указанную долю (0..1). Используется для
// построения шахматного узора под alpha-полосой пикера: он должен быть
// чуть светлее фона, чтобы не сливаться.
func lighten(c color.Color, amount float64) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	mix := func(v uint8) uint8 {
		f := float64(v) + (255.0-float64(v))*amount
		if f > 255 {
			f = 255
		}
		return uint8(f)
	}
	return color.NRGBA{R: mix(n.R), G: mix(n.G), B: mix(n.B), A: 255}
}
