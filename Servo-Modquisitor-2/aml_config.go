// aml_config.go
//
// The "AML Configuration" window. AML (Auto Mod Loading and Ordering) reads the
// top-level load_after / load_before / require tables from each mod's ".mod"
// file to decide load order. This window scans every installed mod, shows which
// ones have that metadata, and lets the user edit it — fully button-driven,
// matching the main UI.
//
// The left mod list is now a table with columns: Folder Name | Load After | Load Before | Required
// The right panel has three sections that stretch evenly, with row striping similar to the main table.
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
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// amlOverrideTheme delegates to the app's active theme but makes selection
// highlights use the main UI's table-selection color (ColorTableRowSelected),
// so the AML window's hover/select match the rest of the program. (Hover
// already uses the shared ColorNameHover, so only selection needs remapping.)
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
	lists   [3][]string
}

// amlPresets - копия пресетов из aml.lua
var amlAfterPresets = map[string][]string{
	"LogMeIn":             {"dmf"},
	"DarktideLocalServer": {"dmf", "LogMeIn"},
	"psych_ward":          {"dmf", "LogMeIn"},
	"Audio":               {"DarktideLocalServer"},
	"Rock":                {"Audio"},
	"UnlimitedPower":      {"Audio"},
	"Clang":               {"Audio"},
	"EnemyAudioReplacer":  {"Audio"},
	"HeKnew":              {"Audio"},
	"LeeroyJenkins":       {"Audio"},
}

// before_presets в AML пуст, поэтому оставляем пустым
var amlBeforePresets = map[string][]string{}

// require_presets для сортировки не нужны, но можно добавить для полноты
var amlRequirePresets = map[string][]string{
	"Audio":              {"DarktideLocalServer"},
	"Rock":               {"Audio"},
	"UnlimitedPower":     {"Audio"},
	"Clang":              {"Audio"},
	"EnemyAudioReplacer": {"Audio"},
	"HeKnew":             {"Audio"},
	"LeeroyJenkins":      {"Audio"},
}

// sectionHeaderKeys maps each editable list (in lists[] order) to its message key.
var sectionHeaderKeys = [3]string{"aml_editor_load_after", "aml_editor_load_before", "aml_editor_require"}

// getAMLDisplayName возвращает локализованное отображаемое имя мода по имени папки.
func (app *App) getAMLDisplayName(folder string) string {
	entry := checks.GetModDBEntry(folder)
	if entry == nil {
		return folder
	}
	lang := app.cfg.Language
	if app.cfg.ForceEnglishModNames {
		lang = "en"
	}
	name := checks.PickLocalized(entry.Name, lang)
	if name == "" {
		return folder
	}
	return name
}

func (app *App) showAMLConfigWindow() {
	win := app.myApp.NewWindow(app.messages["aml_config_title"])
	selTheme := amlOverrideTheme{Theme: app.myApp.Settings().Theme()}

	configs := checks.ListAMLConfigs() // all mods (source of truth)
	allFolders := checks.ListModFolders()

	var displayed []checks.AMLModConfig // filtered + sorted view of configs
	edit := &amlEdit{}
	selectedFolder := "" // track selection by folder (indices change on filter/sort)

	// left-list filter state
	searchText := ""
	filterMode := 0 // 0 - Все, 1 - С настройками AML, 2 - Без настроек AML
	dirty := false  // unsaved changes flag

	var modList *widget.Table
	var applyModFilter func()
	var loadMod func(c checks.AMLModConfig)
	var saveBtn *CustomButton
	var reloadBtn *CustomButton
	var blinkActive bool

	// ── left: mod table (5 columns: marker | Mod Name | Load After | Load Before | Required) ──
	markerWidth := float32(30)

	modList = widget.NewTable(
		func() (int, int) { return len(displayed), 5 },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			lbl := widget.NewLabel("")
			lbl.Alignment = fyne.TextAlignCenter
			return container.NewStack(bg, lbl)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row >= len(displayed) {
				return
			}
			c := displayed[id.Row]
			stack := cell.(*fyne.Container)
			bg := stack.Objects[0].(*canvas.Rectangle)
			lbl := stack.Objects[1].(*widget.Label)
			switch id.Col {
			case 0:
				if c.HasConfig {
					lbl.SetText("✅")
				} else {
					lbl.SetText("❌")
				}
			case 1:
				lbl.SetText(app.getAMLDisplayName(c.Folder))
			case 2:
				lbl.SetText(fmt.Sprintf("%d", len(c.LoadAfter)))
			case 3:
				lbl.SetText(fmt.Sprintf("%d", len(c.LoadBefore)))
			case 4:
				lbl.SetText(fmt.Sprintf("%d", len(c.Require)))
			}
			// Устанавливаем цвет фона в зависимости от чётности строки
			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()
			if id.Row%2 == 0 {
				bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
			} else {
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

	modList.OnSelected = func(id widget.TableCellID) {
		if int(id.Row) >= 0 && int(id.Row) < len(displayed) {
			loadMod(displayed[id.Row])
		}
	}

	// ── filter apply ──────────────────────────────────────────────────
	applyModFilter = func() {
		q := strings.ToLower(strings.TrimSpace(searchText))
		sortedAll := sortAMLOrder(configs)

		// --- Логирование для дебага (построчно с номерами) ---
		// app.appendLog("=== AML Sorted Order ===")
		// for i, c := range sortedAll {
		// app.appendLog(fmt.Sprintf("%3d: %s", i+1, c.Folder))
		// }
		// app.appendLog("=== End of AML Order ===")
		// --- Конец лога ---

		displayed = displayed[:0]
		for _, c := range sortedAll {
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
						modList.Select(widget.TableCellID{Row: i, Col: 0})
						break
					}
				}
			}
		}
	}

	// ── Создаём saveBtn с пустым обработчиком, потом определим функцию и назначим обработчик ──
	saveBtn = NewCustomButton(app.messages["aml_btn_save"], nil)

	// ── Определяем функцию обновления внешнего вида кнопки Save (мигание) ──
	updateSaveButtonAppearance := func() {
		if dirty {
			if !blinkActive {
				blinkActive = true
				go func() {
					for blinkActive && dirty {
						fyne.Do(func() {
							saveBtn.Importance = widget.WarningImportance
							saveBtn.Refresh()
						})
						time.Sleep(600 * time.Millisecond)
						if !dirty {
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
			blinkActive = false
			fyne.Do(func() {
				saveBtn.Importance = widget.MediumImportance
				saveBtn.Refresh()
			})
		}
	}

	// ── Теперь назначаем обработчик для saveBtn ──
	saveBtn.OnTapped = func() {
		if selectedFolder == "" {
			return
		}
		cfg := checks.AMLModConfig{
			Folder:      edit.folder,
			ModFilePath: edit.path,
			LoadAfter:   edit.lists[0],
			LoadBefore:  edit.lists[1],
			Require:     edit.lists[2],
		}
		if err := checks.WriteAMLConfig(cfg); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["aml_log_save_failed"], edit.folder, err))
			return
		}
		app.appendLog(fmt.Sprintf(app.messages["aml_log_saved"], edit.folder))

		// Warn about any entry referencing a mod that isn't installed.
		installed := make(map[string]bool, len(allFolders))
		for _, f := range allFolders {
			installed[f] = true
		}
		for _, lst := range edit.lists {
			for _, e := range lst {
				if !installed[e] {
					app.appendLog(fmt.Sprintf(app.messages["aml_log_unknown_ref"], edit.folder, e))
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
		dirty = false
		updateSaveButtonAppearance()
	}
	app.applyTooltip(saveBtn, "btn_aml_config_tooltip")

	// ── reloadBtn ──
	reloadBtn = NewCustomButton(app.messages["aml_btn_reload"], func() {
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
					applyModFilter()
					for _, c := range configs {
						if c.Folder == selectedFolder {
							loadMod(c)
							break
						}
					}
					dirty = false
					updateSaveButtonAppearance()
				})
			}

			if dirty {
				// Диалог должен показываться в главном потоке, поэтому используем fyne.Do
				choice := app.showChoiceDialogSync(
					win,
					app.messages["warning_title"],
					app.messages["refresh_discard_changes"],
					app.messages["btn_save_and_refresh"],
					app.messages["btn_cancel"],
					app.messages["btn_refresh_anyway"],
				)
				switch choice {
				case 0: // Save and Reload
					// Сохраняем изменения
					if selectedFolder != "" {
						cfg := checks.AMLModConfig{
							Folder:      edit.folder,
							ModFilePath: edit.path,
							LoadAfter:   edit.lists[0],
							LoadBefore:  edit.lists[1],
							Require:     edit.lists[2],
						}
						if err := checks.WriteAMLConfig(cfg); err != nil {
							fyne.Do(func() {
								app.appendLog(fmt.Sprintf(app.messages["aml_log_save_failed"], edit.folder, err))
							})
							return
						}
						fyne.Do(func() {
							app.appendLog(fmt.Sprintf(app.messages["aml_log_saved"], edit.folder))
						})
						// Проверка зависимостей
						installed := make(map[string]bool, len(allFolders))
						for _, f := range allFolders {
							installed[f] = true
						}
						for _, lst := range edit.lists {
							for _, e := range lst {
								if !installed[e] {
									fyne.Do(func() {
										app.appendLog(fmt.Sprintf(app.messages["aml_log_unknown_ref"], edit.folder, e))
									})
								}
							}
						}
						// Обновляем configs для выбранного мода (в фоне)
						for i := range configs {
							if configs[i].Folder == selectedFolder {
								configs[i] = checks.ReadAMLConfig(selectedFolder)
								break
							}
						}
					}
					// После сохранения выполняем перезагрузку
					doReload()
				case 2: // Reload anyway
					doReload()
				case 1: // Cancel - ничего не делаем
					return
				}
			} else {
				// обычная перезагрузка
				doReload()
			}
		}()
	})
	app.applyTooltip(reloadBtn, "btn_refresh_tooltip") // используем существующий тултип

	// ── right: editor widgets ────────────────────────────────────────
	editorTitle := widget.NewLabelWithStyle(app.messages["aml_select_mod_hint"], fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	versionLabel := widget.NewLabel("")

	var sectionLists [3]*widget.List
	var addBtns [3]*CustomButton
	var remBtns [3]*CustomButton

	// ── Функции добавления/удаления записей ──
	addEntry := func(idx int, name string) {
		if name == "" || name == edit.folder || helpers.ContainsString(edit.lists[idx], name) {
			return
		}
		edit.lists[idx] = append(edit.lists[idx], name)
		sectionLists[idx].Refresh()
		dirty = true
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
		sectionLists[idx].UnselectAll()
		sectionLists[idx].Refresh()
		dirty = true
		updateSaveButtonAppearance()
	}

	// showSearchPopup displays a searchable modal list and calls onPick with the
	// chosen value. Both Add and Remove use it, so they behave the same way —
	// Add's candidates are the installed mods, Remove's are the array's current
	// entries.
	showSearchPopup := func(title string, source []string, onPick func(string)) {
		var filtered []string
		apply := func(q string) {
			q = strings.ToLower(strings.TrimSpace(q))
			filtered = filtered[:0]
			for _, f := range source {
				if q == "" || strings.Contains(strings.ToLower(f), q) {
					filtered = append(filtered, f)
				}
			}
		}
		apply("")

		var pop *widget.PopUp
		list := widget.NewList(
			func() int { return len(filtered) },
			func() fyne.CanvasObject { return widget.NewLabel("") },
			func(id widget.ListItemID, o fyne.CanvasObject) {
				if id >= 0 && id < len(filtered) {
					o.(*widget.Label).SetText(filtered[id])
				}
			},
		)
		list.OnSelected = func(id widget.ListItemID) {
			if int(id) >= 0 && int(id) < len(filtered) {
				onPick(filtered[id])
				pop.Hide()
			}
		}

		search := widget.NewEntry()
		search.SetPlaceHolder(app.messages["search_placeholder"])
		search.OnChanged = func(q string) {
			apply(q)
			list.UnselectAll()
			list.Refresh()
		}

		titleLbl := widget.NewLabelWithStyle(title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		listScroll := container.NewVScroll(list)
		listScroll.SetMinSize(fyne.NewSize(330, 320))
		cancel := NewCustomButton(app.messages["btn_cancel"], func() { pop.Hide() })

		content := container.NewBorder(
			container.NewVBox(titleLbl, search),
			container.NewCenter(cancel),
			nil, nil,
			listScroll,
		)
		pop = widget.NewModalPopUp(container.NewThemeOverride(content, selTheme), win.Canvas())
		pop.Resize(fyne.NewSize(390, 440))
		pop.Show()
		win.Canvas().Focus(search)
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
		title := app.messages["aml_btn_add"] + " · " + app.messages[sectionHeaderKeys[idx]]
		showSearchPopup(title, source, func(name string) { addEntry(idx, name) })
	}

	// openRemovePicker (Remove): pick one of the array's current entries to remove.
	openRemovePicker := func(idx int) {
		if selectedFolder == "" || len(edit.lists[idx]) == 0 {
			return
		}
		source := append([]string{}, edit.lists[idx]...)
		title := app.messages["aml_btn_remove"] + " · " + app.messages[sectionHeaderKeys[idx]]
		showSearchPopup(title, source, func(name string) { removeEntryByName(idx, name) })
	}

	// ── build three sections with striped lists ──────────────────────
	buildSection := func(idx int) fyne.CanvasObject {
		header := widget.NewLabelWithStyle(app.messages[sectionHeaderKeys[idx]], fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		controls := container.NewHBox(addBtns[idx], remBtns[idx])

		// Создаём список с чередованием фона
		lst := widget.NewList(
			func() int { return len(edit.lists[idx]) },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				label := widget.NewLabel("")
				return container.NewStack(bg, label)
			},
			func(id widget.ListItemID, obj fyne.CanvasObject) {
				if id >= len(edit.lists[idx]) {
					return
				}
				stack := obj.(*fyne.Container)
				bg := stack.Objects[0].(*canvas.Rectangle)
				label := stack.Objects[1].(*widget.Label)
				label.SetText(edit.lists[idx][id])
				th := app.myApp.Settings().Theme()
				variant := app.myApp.Settings().ThemeVariant()
				if id%2 == 0 {
					bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
				} else {
					bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
				}
				bg.Refresh()
			},
		)
		sectionLists[idx] = lst

		listScroll := container.NewVScroll(lst)
		listScroll.SetMinSize(fyne.NewSize(0, 120))
		return container.NewBorder(header, nil, nil, nil,
			container.NewBorder(controls, nil, nil, nil, listScroll))
	}

	// ── loadMod function ─────────────────────────────────────────────
	loadMod = func(c checks.AMLModConfig) {
		selectedFolder = c.Folder
		edit.folder = c.Folder
		edit.path = c.ModFilePath
		edit.version = c.Version
		edit.lists[0] = append([]string{}, c.LoadAfter...)
		edit.lists[1] = append([]string{}, c.LoadBefore...)
		edit.lists[2] = append([]string{}, c.Require...)

		editorTitle.SetText(app.getAMLDisplayName(c.Folder))
		ver := c.Version
		if ver == "" {
			ver = "-"
		}
		versionLabel.SetText(fmt.Sprintf(app.messages["aml_editor_version"], ver))
		for k := 0; k < 3; k++ {
			sectionLists[k].UnselectAll()
			sectionLists[k].Refresh()
		}
		// Сбрасываем dirty при загрузке нового мода
		dirty = false
		updateSaveButtonAppearance()
	}

	// left-list filter controls
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(app.messages["search_placeholder"])

	// Кнопка очистки
	searchClearBtn := NewCustomButton("✕", func() {
		searchEntry.SetText("")
	})
	searchClearBtn.Importance = widget.DangerImportance
	searchClearBtn.Hide()

	searchEntry.OnChanged = func(s string) {
		searchText = s
		if s != "" {
			searchClearBtn.Show()
		} else {
			searchClearBtn.Hide()
		}
		applyModFilter()
	}

	// Минимальная ширина поля поиска (можно оставить 500 или уменьшить до 300)
	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(AMLSearchMinWidth, 1))
	searchEntryBox := container.NewStack(searchSpacer, searchEntry)

	// Поле поиска с кнопкой очистки справа
	searchBox := container.NewBorder(nil, nil, nil, searchClearBtn, searchEntryBox)

	// Метка "Фильтр:"
	filterLabel := widget.NewLabel(app.messages["filter_label"])

	// Выпадающий список фильтров
	filterSelect := widget.NewSelect(
		[]string{
			app.messages["filter_all"],
			app.messages["aml_filter_configured"],
			app.messages["aml_filter_not_configured"],
		},
		func(s string) {
			switch s {
			case app.messages["filter_all"]:
				filterMode = 0
			case app.messages["aml_filter_configured"]:
				filterMode = 1
			case app.messages["aml_filter_not_configured"]:
				filterMode = 2
			}
			applyModFilter()
		},
	)
	filterSelect.Selected = app.messages["filter_all"]

	// Задаём минимальную ширину для выпадающего списка (чтобы он был шире)
	filterSpacer := canvas.NewRectangle(color.Transparent)
	filterSpacer.SetMinSize(fyne.NewSize(AMLFilterMinWidth, 1)) // регулируйте под свой вкус
	filterSelectWithSize := container.NewStack(filterSpacer, filterSelect)

	// Всё в одной строке
	leftTop := container.NewHBox(
		filterLabel,
		filterSelectWithSize,
		searchBox,
	)

	// Заголовок таблицы
	headerTable := widget.NewTable(
		func() (int, int) { return 1, 5 },
		func() fyne.CanvasObject {
			return container.NewStack(
				canvas.NewRectangle(color.Transparent),
				widget.NewLabel(""),
			)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cont := cell.(*fyne.Container)
			bg := cont.Objects[0].(*canvas.Rectangle)
			lbl := cont.Objects[1].(*widget.Label)
			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()
			bg.FillColor = th.Color(themes.ColorTableHeaderBg, variant)
			lbl.TextStyle = fyne.TextStyle{Bold: true}
			lbl.Alignment = fyne.TextAlignCenter
			switch id.Col {
			case 0:
				lbl.SetText("")
			case 1:
				lbl.SetText("Mod Name")
			case 2:
				lbl.SetText("Load After")
			case 3:
				lbl.SetText("Load Before")
			case 4:
				lbl.SetText("Required")
			}
		},
	)
	headerTable.SetColumnWidth(0, ColCheckboxWidth)
	headerTable.SetColumnWidth(1, ModNameWidth)
	headerTable.SetColumnWidth(2, LABRWidth)
	headerTable.SetColumnWidth(3, LABRWidth)
	headerTable.SetColumnWidth(4, LABRWidth)

	// Фон для левой панели (как в основной таблице, ImageFillContain)
	leftContent := container.NewBorder(headerTable, nil, nil, nil, modList)
	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	if mechData != nil {
		mechBg := canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechBg.FillMode = canvas.ImageFillContain
		mechBg.Translucency = TableBackgroundOpacity
		leftContent = container.NewStack(mechBg, leftContent)
	}
	leftPanel := container.NewBorder(leftTop, nil, nil, nil, leftContent)

	// ── assemble right editor with buttons on top ──────────────────
	// Создаём кнопки Add/Remove для каждой секции
	for i := 0; i < 3; i++ {
		idx := i
		addBtns[idx] = NewCustomButton(app.messages["aml_btn_add"], func() { openPicker(idx) })
		remBtns[idx] = NewCustomButton(app.messages["aml_btn_remove"], func() { openRemovePicker(idx) })
	}

	section0 := buildSection(0)
	section1 := buildSection(1)
	section2 := buildSection(2)

	topPart := container.NewVBox(
		container.NewHBox(saveBtn, reloadBtn),
		widget.NewSeparator(),
		editorTitle,
		versionLabel,
		widget.NewSeparator(),
		widget.NewSeparator(),
	)

	// Сплиты для равномерного растягивания (3 равные части)
	splitMiddleBottom := container.NewVSplit(section1, section2)
	splitMiddleBottom.Offset = 0.5

	splitTopRest := container.NewVSplit(section0, splitMiddleBottom)
	splitTopRest.Offset = 0.333

	// Правая панель — без фонового изображения
	editorScroll := container.NewVScroll(container.NewBorder(topPart, nil, nil, nil, splitTopRest))

	// ── final layout ──────────────────────────────────────────────────
	applyModFilter() // initial population

	split := container.NewHSplit(leftPanel, editorScroll)
	split.Offset = 0.6
	win.SetContent(container.NewThemeOverride(split, selTheme))
	win.Resize(fyne.NewSize(AMLWindowWidth, AMLWindowHeight))
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

// contains - вспомогательная функция для проверки вхождения строки в срез
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
