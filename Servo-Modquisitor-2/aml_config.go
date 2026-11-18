// Servo-Modquisitor-2/aml_config.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type amlOverrideTheme struct {
	fyne.Theme
}

func (t amlOverrideTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNameSelection {
		return t.Theme.Color(themes.ColorTableRowSelected, variant)
	}
	return t.Theme.Color(name, variant)
}

// amlEdit is the in-memory working copy of the currently selected mod's config.
// lists[0]=load_after, lists[1]=load_before, lists[2]=require (matching
// checks' amlFields order).
type amlEdit struct {
	folder  string
	path    string
	version string
	author  string
	lists   [3][]string
}

// sectionHeaderKeys maps each editable list (in lists[] order) to its message key.
var sectionHeaderKeys = [3]string{"aml_editor_load_after", "aml_editor_load_before", "aml_editor_require"}

// modDisplayName возвращает отображаемое имя мода по имени папки.
func (app *App) modDisplayName(folder string) string {
	entry := checks.GetModDBEntry(folder)
	if entry == nil {
		return folder
	}

	app.cfgMutex.RLock()
	lang := app.cfg.Language
	forceEn := app.cfg.ForceEnglishModNames
	app.cfgMutex.RUnlock()

	if forceEn {
		lang = "en"
	}
	name := checks.PickLocalized(entry.Name, lang)
	if name == "" {
		return folder
	}
	return name
}

func (app *App) showAMLConfigWindow() {
	if !app.acquireWindow("aml_config") {
		return
	}

	win := app.myApp.NewWindow(app.msg("aml_config_title"))
	app.registerWindow("aml_config", win)
	selTheme := amlOverrideTheme{Theme: app.myApp.Settings().Theme()}

	configs := checks.ListAMLConfigs()
	allFolders := checks.ListModFolders()

	// sortedConfigs — кэш результата sortAMLOrder. Пересчитывается:
	//   - при первом обращении (initial applyModFilter ниже);
	//   - после Reload (пользователь мог обновить auto_mod_loader_log.txt).
	// После Save НЕ пересчитываем: сохраняются только списки/версия/автор,
	// имена папок не меняются, порядок из лог-файла остаётся тем же.
	var sortedConfigs []checks.AMLModConfig
	var sortedConfigsValid bool

	var displayed []checks.AMLModConfig // filtered + sorted view of configs
	edit := &amlEdit{}
	selectedFolder := "" // track selection by folder (indices change on filter/sort)

	// left-list filter state
	searchText := ""
	filterMode := 0 // 0 - Все, 1 - С настройками AML, 2 - Без настроек AML

	// dirty и blinkActive — читаются из фоновой горутины мигания,
	// поэтому держим их в atomic, а не в обычных bool.
	var dirty atomic.Bool
	var blinkActive atomic.Bool

	var modList *widget.Table
	var applyModFilter func()
	var loadMod func(c checks.AMLModConfig)
	var saveBtn *CustomButton
	var reloadBtn *CustomButton

	// Section-level state
	var sectionTables [3]*widget.Table
	var sectionCountLabels [3]*canvas.Text

	// ── left: mod table (5 columns: marker | Mod Name | Load After | Load Before | Required) ──
	markerWidth := float32(30)

	// Иконки-маркеры для колонки «есть ли AML-настройки у мода».
	amlCheckedIcon := app.loadIconResource("checked_box", "assets/buttons/checked_box.png")
	amlUncheckedIcon := app.loadIconResource("unchecked_box_red", "assets/buttons/unchecked_box_red.png")

	modList = widget.NewTable(
		func() (int, int) { return len(displayed), 5 },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			bg.SetMinSize(fyne.NewSize(1, 28))
			content := container.NewStack()
			return container.NewStack(bg, content)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row >= len(displayed) {
				return
			}
			c := displayed[id.Row]
			outer := cell.(*fyne.Container)
			bg := outer.Objects[0].(*canvas.Rectangle)
			content := outer.Objects[1].(*fyne.Container)

			content.Objects = nil

			switch id.Col {
			case 0:
				var icon fyne.Resource
				if c.HasConfig {
					icon = amlCheckedIcon
				} else {
					icon = amlUncheckedIcon
				}
				if icon != nil {
					img := canvas.NewImageFromResource(icon)
					img.FillMode = canvas.ImageFillContain
					img.SetMinSize(fyne.NewSize(18, 18))
					content.Add(container.NewCenter(img))
				}

			case 1:
				lbl := widget.NewLabel(app.modDisplayName(c.Folder))
				lbl.Alignment = fyne.TextAlignLeading
				content.Add(lbl)

			case 2:
				lbl := widget.NewLabel(fmt.Sprintf("%d", len(c.LoadAfter)))
				lbl.Alignment = fyne.TextAlignCenter
				content.Add(lbl)

			case 3:
				lbl := widget.NewLabel(fmt.Sprintf("%d", len(c.LoadBefore)))
				lbl.Alignment = fyne.TextAlignCenter
				content.Add(lbl)

			case 4:
				lbl := widget.NewLabel(fmt.Sprintf("%d", len(c.Require)))
				lbl.Alignment = fyne.TextAlignCenter
				content.Add(lbl)
			}
			content.Refresh()

			// Фон: выделенная строка — цвет выделения, иначе полосатый.
			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()
			switch {
			case c.Folder != "" && c.Folder == selectedFolder:
				bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
			case id.Row%2 == 0:
				bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
			default:
				bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
			}
			bg.Refresh()
		},
	)

	modList.SetColumnWidth(0, markerWidth)
	modList.SetColumnWidth(1, ModNameWidth)
	modList.SetColumnWidth(2, LABRWidth)
	modList.SetColumnWidth(3, LABRWidth)
	modList.SetColumnWidth(4, LABRWidth)
	modList.SetRowHeight(-1, 28)

	modList.OnSelected = func(id widget.TableCellID) {
		if int(id.Row) >= 0 && int(id.Row) < len(displayed) {
			c := displayed[id.Row]
			if c.Folder == selectedFolder {
				modList.Refresh()
				return
			}
			loadMod(c)
			modList.Refresh()
		}
	}

	// ── filter apply ──────────────────────────────────────────────────
	applyModFilter = func() {
		if !sortedConfigsValid {
			sortedConfigs = sortAMLOrder(configs)
			sortedConfigsValid = true
		}
		q := strings.ToLower(strings.TrimSpace(searchText))

		displayed = displayed[:0]
		for _, c := range sortedConfigs {
			if filterMode == 1 && !c.HasConfig {
				continue
			}
			if filterMode == 2 && c.HasConfig {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(c.Folder), q) {
				continue
			}
			displayed = append(displayed, c)
		}
		if modList != nil {
			modList.UnselectAll()
			modList.Refresh()
			if selectedFolder != "" {
				for i, c := range displayed {
					if c.Folder == selectedFolder {
						modList.Select(widget.TableCellID{Row: i, Col: 0}, 0)
						break
					}
				}
			}
		}
	}

	saveBtn = NewCustomIconButton(app.msg(""), app.loadIconResource("save", "assets/buttons/save.png"), nil)

	// ── Определяем функцию обновления внешнего вида кнопки Save (мигание) ──
	// dirty и blinkActive — atomic.Bool, потому что цикл мигания живёт
	// в отдельной горутине и читает оба флага, пока UI-поток их пишет.
	updateSaveButtonAppearance := func() {
		if dirty.Load() {
			if !blinkActive.Load() {
				blinkActive.Store(true)
				go func() {
					for blinkActive.Load() && dirty.Load() {
						fyne.Do(func() {
							saveBtn.Importance = widget.WarningImportance
							saveBtn.Refresh()
						})
						time.Sleep(600 * time.Millisecond)
						if !dirty.Load() {
							break
						}
						fyne.Do(func() {
							saveBtn.Importance = widget.MediumImportance
							saveBtn.Refresh()
						})
						time.Sleep(1000 * time.Millisecond)
					}
					fyne.Do(func() {
						saveBtn.Importance = widget.MediumImportance
						saveBtn.Refresh()
					})
				}()
			}
		} else {
			blinkActive.Store(false)
			fyne.Do(func() {
				saveBtn.Importance = widget.MediumImportance
				saveBtn.Refresh()
			})
		}
	}

	// ── Обновляет счётчики в section headers ──
	updateSectionCounts := func() {
		for k := 0; k < 3; k++ {
			if sectionCountLabels[k] != nil {
				sectionCountLabels[k].Text = fmt.Sprintf("(%d)", len(edit.lists[k]))
				sectionCountLabels[k].Refresh()
			}
		}
	}

	// ── Теперь назначаем обработчик для saveBtn ──
	saveBtn.OnTapped = func() {
		if selectedFolder == "" {
			return
		}
		if !dirty.Load() {
			app.appendLogToFile("AML: save skipped, no changes")
			return
		}

		cfg := checks.AMLModConfig{
			Folder:      edit.folder,
			ModFilePath: edit.path,
			LoadAfter:   edit.lists[0],
			LoadBefore:  edit.lists[1],
			Require:     edit.lists[2],
			Version:     edit.version,
			Author:      edit.author,
		}
		if err := checks.WriteAMLConfig(cfg); err != nil {
			app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_save_failed"), edit.folder, err))
			return
		}
		app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_saved"), edit.folder))

		// Warn about any entry referencing a mod that isn't installed.
		installed := make(map[string]bool, len(allFolders))
		for _, f := range allFolders {
			installed[f] = true
		}
		for _, lst := range edit.lists {
			for _, e := range lst {
				if !installed[e] {
					app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_unknown_ref"), edit.folder, e))
				}
			}
		}

		updateConfigByFolder := func(folder string) {
			for i := range configs {
				if configs[i].Folder == folder {
					configs[i] = checks.ReadAMLConfig(folder)
					return
				}
			}
		}
		updateConfigByFolder(selectedFolder)
		applyModFilter() // refresh marker/counts (and re-highlight)
		dirty.Store(false)
		updateSaveButtonAppearance()
	}
	saveBtn.SetToolTip(app.msg("btn_aml_config_tooltip"))

	// ── reloadBtn ──
	reloadBtn = NewCustomIconButton(app.msg(""), app.loadIconResource("refresh", "assets/buttons/refresh.png"), func() {
		go func() {
			// Функция для выполнения перезагрузки (вынесена для избежания дублирования)
			doReload := func() {
				// Тяжёлые операции - чтение файлов
				newConfigs := checks.ListAMLConfigs()
				newAllFolders := checks.ListModFolders()
				// Обновляем UI в главном потоке
				fyne.Do(func() {
					configs = newConfigs
					allFolders = newAllFolders
					sortedConfigsValid = false
					applyModFilter()
					for _, c := range configs {
						if c.Folder == selectedFolder {
							loadMod(c)
							break
						}
					}
					dirty.Store(false)
					updateSaveButtonAppearance()
				})
			}

			if dirty.Load() {
				// Диалог должен показываться в главном потоке, поэтому используем fyne.Do
				choice := app.showChoiceDialogSync(
					win,
					app.msg("warning_title"),
					app.msg("refresh_discard_changes"),
					app.msg("btn_save_and_refresh"),
					app.msg("btn_cancel"),
					app.msg("btn_refresh_anyway"),
				)
				switch choice {
				case 0: // Save and Reload
					// Снимок состояния — на UI-потоке, чтобы не гонять
					// edit.* и allFolders между горутиной и UI.
					var (
						snapFolder     string
						snapPath       string
						snapVersion    string
						snapAuthor     string
						snapLists      [3][]string
						snapAllFolders []string
					)
					fyne.DoAndWait(func() {
						snapFolder = edit.folder
						snapPath = edit.path
						snapVersion = edit.version
						snapAuthor = edit.author
						for k := 0; k < 3; k++ {
							snapLists[k] = append([]string{}, edit.lists[k]...)
						}
						snapAllFolders = append([]string{}, allFolders...)
					})

					if snapFolder != "" {
						cfg := checks.AMLModConfig{
							Folder:      snapFolder,
							ModFilePath: snapPath,
							LoadAfter:   snapLists[0],
							LoadBefore:  snapLists[1],
							Require:     snapLists[2],
							Version:     snapVersion,
							Author:      snapAuthor,
						}
						if err := checks.WriteAMLConfig(cfg); err != nil {
							fyne.Do(func() {
								app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_save_failed"), snapFolder, err))
							})
							return
						}
						fyne.Do(func() {
							app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_saved"), snapFolder))
						})

						installed := make(map[string]bool, len(snapAllFolders))
						for _, f := range snapAllFolders {
							installed[f] = true
						}
						for _, lst := range snapLists {
							for _, e := range lst {
								if !installed[e] {
									fyne.Do(func() {
										app.appendLogToFile(fmt.Sprintf(app.msg("aml_log_unknown_ref"), snapFolder, e))
									})
								}
							}
						}
					}
					doReload()
				case 2: // Reload anyway
					doReload()
				case 1: // Cancel
					return
				}
			} else {
				doReload()
			}
		}()
	})
	reloadBtn.SetToolTip(app.msg("btn_refresh_tooltip")) // используем существующий тултип

	// ── right: editor widgets ────────────────────────────────────────
	editorTitle := helpers.BoldLabel(app.msg("aml_select_mod_hint"))
	editorTitle.Wrapping = fyne.TextWrapWord

	var suppressChange atomic.Bool

	versionEntry := widget.NewEntry()
	versionEntry.SetPlaceHolder(app.msg("placeholder_mod_version"))

	versionEntry.OnChanged = func(s string) {
		if suppressChange.Load() {
			return
		}
		edit.version = s
		dirty.Store(true)
		updateSaveButtonAppearance()
	}
	versionSpacer := helpers.NewFixedSpacer(150, 1)
	versionEntryBox := container.NewStack(versionSpacer, versionEntry)

	authorEntry := widget.NewEntry()
	authorEntry.SetPlaceHolder(app.msg("author_unknown"))

	authorEntry.OnChanged = func(s string) {
		if suppressChange.Load() {
			return
		}
		edit.author = s
		dirty.Store(true)
		updateSaveButtonAppearance()
	}
	authorSpacer := helpers.NewFixedSpacer(300, 1)
	authorEntryBox := container.NewStack(authorSpacer, authorEntry)

	var addBtns [3]*CustomButton
	var remBtns [3]*CustomButton

	// ── Функции добавления/удаления записей ──
	addEntry := func(idx int, name string) {
		if name == "" || name == edit.folder || helpers.ContainsString(edit.lists[idx], name) {
			return
		}
		edit.lists[idx] = append(edit.lists[idx], name)
		if sectionTables[idx] != nil {
			sectionTables[idx].Refresh()
		}
		updateSectionCounts()
		dirty.Store(true)
		updateSaveButtonAppearance()
	}
	removeEntryByName := func(idx int, name string) {
		out := edit.lists[idx][:0]
		for _, e := range edit.lists[idx] {
			if e != name {
				out = append(out, e)
			}
		}
		edit.lists[idx] = out
		if sectionTables[idx] != nil {
			sectionTables[idx].Refresh()
		}
		updateSectionCounts()
		dirty.Store(true)
		updateSaveButtonAppearance()
	}

	// showSearchPopup теперь вызывает универсальный app.ShowSearchPopup.
	showSearchPopup := func(title string, source []string, onPick func(string)) {
		app.ShowSearchPopup(SearchPopupOptions{
			Title:  title,
			Source: source,
			Parent: win,
			OnPick: onPick,
			// FormatRow: по умолчанию app.modDisplayName(folder)
		})
	}

	// openPicker (Add): pick an installed mod (minus the current mod and any
	// already-added entries) to add to lists[idx].
	openPicker := func(idx int) {
		if selectedFolder == "" {
			return // no mod selected yet
		}
		exclude := map[string]bool{edit.folder: true}
		for _, e := range edit.lists[idx] {
			exclude[e] = true
		}
		var source []string
		for _, f := range allFolders {
			if !exclude[f] {
				source = append(source, f)
			}
		}
		title := app.msg("aml_btn_add") + " · " + app.msg(sectionHeaderKeys[idx])
		showSearchPopup(title, source, func(name string) { addEntry(idx, name) })
	}

	// openRemovePicker (Remove): pick one of the array's current entries to remove.
	openRemovePicker := func(idx int) {
		if selectedFolder == "" || len(edit.lists[idx]) == 0 {
			return
		}
		source := append([]string{}, edit.lists[idx]...)
		title := app.msg("aml_btn_remove") + " · " + app.msg(sectionHeaderKeys[idx])
		showSearchPopup(title, source, func(name string) { removeEntryByName(idx, name) })
	}

	// ── build three sections with striped tables ─────────────────────
	buildSection := func(idx int) fyne.CanvasObject {
		titleText := canvas.NewText(app.msg(sectionHeaderKeys[idx]), color.White)
		titleText.TextStyle = fyne.TextStyle{Bold: true}
		titleText.TextSize = 13

		countText := canvas.NewText("(0)", color.White)
		countText.Alignment = fyne.TextAlignTrailing
		countText.TextSize = 12
		sectionCountLabels[idx] = countText

		th := app.myApp.Settings().Theme()
		variant := app.myApp.Settings().ThemeVariant()
		headerBg := canvas.NewRectangle(th.Color(themes.ColorTableHeaderBg, variant))

		headerInner := container.NewBorder(
			nil, nil,
			container.NewPadded(titleText),
			container.NewPadded(countText),
			nil,
		)
		headerBox := container.NewStack(headerBg, headerInner)

		controls := container.NewHBox(addBtns[idx], remBtns[idx])

		secTable := widget.NewTable(
			func() (int, int) {
				n := len(edit.lists[idx])
				if n == 0 {
					return 1, 2 // 1 пустая колонка-номер + 1 для текста «пусто»
				}
				return n, 2
			},
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				bg.SetMinSize(fyne.NewSize(1, 26))
				label := widget.NewLabel("")
				return container.NewStack(bg, label)
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				stack := cell.(*fyne.Container)
				bg := stack.Objects[0].(*canvas.Rectangle)
				label := stack.Objects[1].(*widget.Label)

				tTh := fyne.CurrentApp().Settings().Theme()
				tVariant := fyne.CurrentApp().Settings().ThemeVariant()

				if len(edit.lists[idx]) == 0 {
					// Empty state. Текст кладём во вторую колонку —
					// там ей хватает ширины (см. SetColumnWidth ниже),
					// и он не вылезает за левый край, как было бы в
					// узкой колонке с номером.
					bg.FillColor = color.Transparent
					if id.Col == 0 {
						label.SetText("")
					} else {
						label.SetText(app.msg("aml_empty_list"))
						label.TextStyle = fyne.TextStyle{Italic: true}
						label.Alignment = fyne.TextAlignLeading
					}
				} else {
					if id.Row%2 == 0 {
						bg.FillColor = tTh.Color(themes.ColorTableRowEven, tVariant)
					} else {
						bg.FillColor = tTh.Color(themes.ColorTableRowOdd, tVariant)
					}
					if id.Col == 0 {
						label.SetText(fmt.Sprintf("%d", id.Row+1))
						label.Alignment = fyne.TextAlignCenter
					} else {
						label.SetText(edit.lists[idx][id.Row])
						label.Alignment = fyne.TextAlignLeading
					}
					label.TextStyle = fyne.TextStyle{}
				}
				bg.Refresh()
				label.Refresh()
			},
		)
		secTable.SetColumnWidth(0, ColSelectWidth)
		secTable.SetColumnWidth(1, ColAuthorWidth)
		secTable.SetRowHeight(-1, 26)
		sectionTables[idx] = secTable

		scroll := container.NewVScroll(secTable)
		scroll.SetMinSize(fyne.NewSize(0, 100))

		return container.NewBorder(headerBox,
			container.NewVBox(controls, widget.NewSeparator()),
			nil, nil,
			scroll,
		)
	}

	// ── loadMod function ─────────────────────────────────────────────
	loadMod = func(c checks.AMLModConfig) {
		selectedFolder = c.Folder
		edit.folder = c.Folder
		edit.path = c.ModFilePath
		edit.version = c.Version
		edit.author = c.Author
		edit.lists[0] = append([]string{}, c.LoadAfter...)
		edit.lists[1] = append([]string{}, c.LoadBefore...)
		edit.lists[2] = append([]string{}, c.Require...)

		editorTitle.SetText(app.modDisplayName(c.Folder))

		suppressChange.Store(true)
		versionEntry.SetText(c.Version)
		authorEntry.SetText(c.Author)
		suppressChange.Store(false)

		// Обновляем таблицы разделов.
		for k := 0; k < 3; k++ {
			if sectionTables[k] != nil {
				sectionTables[k].Refresh()
			}
		}
		updateSectionCounts()

		// Сбрасываем dirty при загрузке нового мода
		dirty.Store(false)
		updateSaveButtonAppearance()
	}

	searchField := app.NewSearchField(SearchFieldOptions{
		Width:         AMLSearchMinWidth,
		DebounceDelay: 300 * time.Millisecond,
		ShowClear:     true,
		// Backsp-иконка теперь единая. Раньше тут стояла
		// select_all_de.png — семантически неверная.
		OnFilter: func(s string) {
			searchText = s
			applyModFilter()
		},
	})
	searchBox := searchField.Container

	filterLabel := widget.NewLabel(app.msg("filter_label"))

	filterSelect := widget.NewSelect(
		[]string{
			app.msg("filter_all"),
			app.msg("aml_filter_configured"),
			app.msg("aml_filter_not_configured"),
		},
		func(s string) {
			switch s {
			case app.msg("filter_all"):
				filterMode = 0
			case app.msg("aml_filter_configured"):
				filterMode = 1
			case app.msg("aml_filter_not_configured"):
				filterMode = 2
			}
			applyModFilter()
		},
	)
	filterSelect.Selected = app.msg("filter_all")

	filterSpacer := helpers.NewFixedSpacer(AMLFilterMinWidth, 1)
	filterSelectWithSize := container.NewStack(filterSpacer, filterSelect)

	leftTop := container.NewHBox(
		filterLabel,
		filterSelectWithSize,
		searchBox,
	)

	// Заголовок таблицы
	headerTable := widget.NewTable(
		func() (int, int) { return 1, 5 },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			bg.SetMinSize(fyne.NewSize(1, 26))
			lbl := widget.NewLabel("")
			lbl.Alignment = fyne.TextAlignCenter
			lbl.TextStyle = fyne.TextStyle{Bold: true}
			return container.NewStack(bg, lbl)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cont := cell.(*fyne.Container)
			bg := cont.Objects[0].(*canvas.Rectangle)
			lbl := cont.Objects[1].(*widget.Label)
			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()
			bg.FillColor = th.Color(themes.ColorTableHeaderBg, variant)
			bg.Refresh()
			switch id.Col {
			case 0:
				lbl.SetText(app.msg("col_checkbox"))
			case 1:
				lbl.SetText(app.msg("col_name"))
			case 2:
				lbl.SetText(app.msg("after"))
			case 3:
				lbl.SetText(app.msg("before"))
			case 4:
				lbl.SetText(app.msg("aml_col_required"))
			}
			lbl.Refresh()
		},
	)
	headerTable.SetColumnWidth(0, ColCheckboxWidth)
	headerTable.SetColumnWidth(1, ModNameWidth)
	headerTable.SetColumnWidth(2, LABRWidth)
	headerTable.SetColumnWidth(3, LABRWidth)
	headerTable.SetColumnWidth(4, LABRWidth)
	headerTable.SetRowHeight(-1, 26)

	// Фон для левой панели (как в основной таблице, ImageFillContain)
	leftContent := container.NewBorder(headerTable, nil, nil, nil, modList)
	leftContent = app.newTableBackground(TableBackgroundOpacity, leftContent)
	leftPanel := container.NewBorder(leftTop, nil, nil, nil, leftContent)

	// ── assemble right editor with buttons on top ──────────────────
	// Создаём кнопки Add/Remove для каждой секции
	for i := 0; i < 3; i++ {
		idx := i
		addBtns[idx] = NewCustomIconButton(app.msg(""), app.loadIconResource("add", "assets/buttons/add.png"), func() { openPicker(idx) })
		remBtns[idx] = NewCustomIconButton(app.msg(""), app.loadIconResource("trash", "assets/buttons/trashcan_red.png"), func() { openRemovePicker(idx) })
	}

	section0 := buildSection(0)
	section1 := buildSection(1)
	section2 := buildSection(2)

	topPart := container.NewVBox(
		container.NewHBox(saveBtn, reloadBtn),
		widget.NewSeparator(),
		editorTitle,
		container.NewHBox(
			widget.NewLabel(app.msg("table_lbl_version")),
			versionEntryBox,
			widget.NewLabel(app.msg("aml_editor_author")),
			authorEntryBox,
		),
		widget.NewSeparator(),
	)

	// Сплиты для равномерного растягивания (3 равные части)
	splitMiddleBottom := container.NewVSplit(section1, section2)
	splitMiddleBottom.Offset = 0.5

	splitTopRest := container.NewVSplit(section0, splitMiddleBottom)
	splitTopRest.Offset = 0.333

	editorScroll := container.NewVScroll(container.NewBorder(topPart, nil, nil, nil, splitTopRest))

	// ── final layout ──────────────────────────────────────────────────
	applyModFilter() // initial population

	split := container.NewHSplit(leftPanel, editorScroll)
	split.Offset = 0.6
	win.SetContent(container.NewThemeOverride(split, selTheme))
	win.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
	win.Show()
}

// sortAMLOrder сортирует моды в порядке, записанном в auto_mod_loader_log.txt (если он есть).
// Если файла нет, используется алфавитный порядок.
func sortAMLOrder(configs []checks.AMLModConfig) []checks.AMLModConfig {
	if len(configs) == 0 {
		return configs
	}

	logPath := filepath.Join(checks.ModsDir(), "auto_mod_loader_log.txt")
	data, err := os.ReadFile(logPath)
	if err != nil {
		return sortAlphabetically(configs)
	}

	orderMap := make(map[string]int)
	var orderList []string
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Ищем начало имени: `:"`
		idx := strings.Index(line, `:"`)
		if idx == -1 {
			continue
		}

		nameStart := idx + 2 // пропускаем :"
		if nameStart >= len(line) {
			continue
		}

		// Ищем разделитель ` ("` (пробел, открывающая скобка, кавычка)
		// Он отделяет имя от версии.
		// Если разделителя нет — имя до конца строки.
		nameEnd := strings.Index(line[nameStart:], ` ("`)
		var name string
		if nameEnd == -1 {
			name = line[nameStart:]
		} else {
			name = line[nameStart : nameStart+nameEnd]
		}

		name = strings.TrimSpace(name)
		name = strings.Trim(name, `"`) // на случай, если где-то осталась кавычка
		if name == "" || name == "dmf" || name == "base" {
			continue
		}

		if _, exists := orderMap[name]; !exists {
			orderMap[name] = len(orderList)
			orderList = append(orderList, name)
		}
	}

	if len(orderList) == 0 {
		return sortAlphabetically(configs)
	}

	// Собираем все моды, кроме dmf/base
	sorted := make([]checks.AMLModConfig, 0, len(configs))
	for _, c := range configs {
		if c.Folder != "dmf" && c.Folder != "base" {
			sorted = append(sorted, c)
		}
	}

	sort.Slice(sorted, func(i, j int) bool {
		posI, okI := orderMap[sorted[i].Folder]
		posJ, okJ := orderMap[sorted[j].Folder]
		if okI && okJ {
			return posI < posJ
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return strings.ToLower(sorted[i].Folder) < strings.ToLower(sorted[j].Folder)
	})

	return sorted
}

func sortAlphabetically(configs []checks.AMLModConfig) []checks.AMLModConfig {
	result := make([]checks.AMLModConfig, 0, len(configs))
	for _, c := range configs {
		if c.Folder != "dmf" && c.Folder != "base" {
			result = append(result, c)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Folder) < strings.ToLower(result[j].Folder)
	})
	return result
}
