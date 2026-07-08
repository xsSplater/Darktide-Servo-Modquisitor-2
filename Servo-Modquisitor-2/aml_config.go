// aml_config.go
//
// The "AML Configuration" window. AML (Auto Mod Loading and Ordering) reads the
// top-level load_after / load_before / require tables from each mod's ".mod"
// file to decide load order. This window scans every installed mod, shows which
// ones have that metadata, and lets the user edit it — fully button-driven,
// matching the main UI.
//
// The left mod list can be searched by name, filtered to only AML-configured
// mods, and sorted A→Z / Z→A. "Add" (per section) opens a searchable picker so
// long mod lists are easy to navigate.
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
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

// sectionHeaderKeys maps each editable list (in lists[] order) to its message key.
var sectionHeaderKeys = [3]string{"aml_editor_load_after", "aml_editor_load_before", "aml_editor_require"}

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
	configuredOnly := false
	sortDesc := false

	var modList *widget.List
	var applyModFilter func()
	var loadMod func(c checks.AMLModConfig)
	var openPicker func(idx int)
	var openRemovePicker func(idx int)

	// ── left: mod list ───────────────────────────────────────────────
	modList = widget.NewList(
		func() int { return len(displayed) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id < 0 || id >= len(displayed) {
				return
			}
			c := displayed[id]
			marker := "○"
			if c.HasConfig {
				marker = "●"
			}
			o.(*widget.Label).SetText(fmt.Sprintf("%s %s  (a%d b%d r%d)",
				marker, c.Folder, len(c.LoadAfter), len(c.LoadBefore), len(c.Require)))
		},
	)

	applyModFilter = func() {
		q := strings.ToLower(strings.TrimSpace(searchText))
		displayed = displayed[:0]
		for _, c := range configs {
			if configuredOnly && !c.HasConfig {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(c.Folder), q) {
				continue
			}
			displayed = append(displayed, c)
		}
		sort.SliceStable(displayed, func(i, j int) bool {
			a, b := strings.ToLower(displayed[i].Folder), strings.ToLower(displayed[j].Folder)
			if sortDesc {
				return a > b
			}
			return a < b
		})
		if modList != nil {
			modList.UnselectAll()
			modList.Refresh()
			// keep the current mod highlighted if it's still visible
			if selectedFolder != "" {
				for i, c := range displayed {
					if c.Folder == selectedFolder {
						modList.Select(i)
						break
					}
				}
			}
		}
	}

	// ── right: editor widgets ────────────────────────────────────────
	editorTitle := widget.NewLabelWithStyle(app.messages["aml_select_mod_hint"], fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	versionLabel := widget.NewLabel("")

	var sectionLists [3]*widget.List
	var addBtns [3]*CustomButton
	var remBtns [3]*CustomButton

	addEntry := func(idx int, name string) {
		if name == "" || name == edit.folder || app.containsStr(edit.lists[idx], name) {
			return
		}
		edit.lists[idx] = append(edit.lists[idx], name)
		sectionLists[idx].Refresh()
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
		search.SetPlaceHolder(app.messages["aml_search_placeholder"])
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
	openPicker = func(idx int) {
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
	openRemovePicker = func(idx int) {
		if selectedFolder == "" || len(edit.lists[idx]) == 0 {
			return
		}
		source := append([]string{}, edit.lists[idx]...)
		title := app.messages["aml_btn_remove"] + " · " + app.messages[sectionHeaderKeys[idx]]
		showSearchPopup(title, source, func(name string) { removeEntryByName(idx, name) })
	}

	for i := 0; i < 3; i++ {
		idx := i
		lst := widget.NewList(
			func() int { return len(edit.lists[idx]) },
			func() fyne.CanvasObject { return widget.NewLabel("") },
			func(id widget.ListItemID, o fyne.CanvasObject) {
				if id >= 0 && id < len(edit.lists[idx]) {
					o.(*widget.Label).SetText(edit.lists[idx][id])
				}
			},
		)
		sectionLists[idx] = lst

		addBtns[idx] = NewCustomButton(app.messages["aml_btn_add"], func() { openPicker(idx) })
		remBtns[idx] = NewCustomButton(app.messages["aml_btn_remove"], func() { openRemovePicker(idx) })
	}

	loadMod = func(c checks.AMLModConfig) {
		selectedFolder = c.Folder
		edit.folder = c.Folder
		edit.path = c.ModFilePath
		edit.version = c.Version
		edit.lists[0] = append([]string{}, c.LoadAfter...)
		edit.lists[1] = append([]string{}, c.LoadBefore...)
		edit.lists[2] = append([]string{}, c.Require...)

		editorTitle.SetText(c.Folder)
		ver := c.Version
		if ver == "" {
			ver = "-"
		}
		versionLabel.SetText(fmt.Sprintf(app.messages["aml_editor_version"], ver))
		for k := 0; k < 3; k++ {
			sectionLists[k].UnselectAll()
			sectionLists[k].Refresh()
		}
	}

	modList.OnSelected = func(id widget.ListItemID) {
		if int(id) >= 0 && int(id) < len(displayed) {
			loadMod(displayed[id])
		}
	}

	// updateConfigByFolder re-reads one mod's config in the master slice.
	updateConfigByFolder := func(folder string) {
		for i := range configs {
			if configs[i].Folder == folder {
				configs[i] = checks.ReadAMLConfig(folder)
				return
			}
		}
	}

	saveBtn := NewCustomButton(app.messages["aml_btn_save"], func() {
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

		updateConfigByFolder(selectedFolder)
		applyModFilter() // refresh marker/counts (and re-highlight)
	})
	app.applyTooltip(saveBtn, "btn_aml_config_tooltip")

	reloadBtn := NewCustomButton(app.messages["aml_btn_reload"], func() {
		configs = checks.ListAMLConfigs()
		allFolders = checks.ListModFolders()
		applyModFilter()
		// reload the editor for the still-selected mod, if present
		for _, c := range configs {
			if c.Folder == selectedFolder {
				loadMod(c)
				break
			}
		}
	})

	// ── left-list filter controls ────────────────────────────────────
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(app.messages["aml_search_placeholder"])
	searchEntry.OnChanged = func(s string) { searchText = s; applyModFilter() }

	configuredCheck := widget.NewCheck(app.messages["aml_filter_configured"], func(b bool) {
		configuredOnly = b
		applyModFilter()
	})

	sortSelect := widget.NewSelect(
		[]string{app.messages["aml_sort_az"], app.messages["aml_sort_za"]},
		func(s string) { sortDesc = (s == app.messages["aml_sort_za"]); applyModFilter() },
	)
	sortSelect.Selected = app.messages["aml_sort_az"]

	// ── assemble editor ──────────────────────────────────────────────
	buildSection := func(idx int) fyne.CanvasObject {
		header := widget.NewLabelWithStyle(app.messages[sectionHeaderKeys[idx]], fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
		// Add + Remove on one line, directly under the header.
		controls := container.NewHBox(addBtns[idx], remBtns[idx])
		// Current entries below the controls (this is what Add/Remove apply to).
		listScroll := container.NewVScroll(sectionLists[idx])
		listScroll.SetMinSize(fyne.NewSize(0, 64))
		return container.NewVBox(header, controls, listScroll)
	}

	editor := container.NewVBox(
		editorTitle,
		versionLabel,
		widget.NewSeparator(),
		buildSection(0),
		buildSection(1),
		buildSection(2),
		widget.NewSeparator(),
		container.NewHBox(saveBtn, reloadBtn),
	)
	editorScroll := container.NewVScroll(editor)

	hint := widget.NewLabel(app.messages["aml_editor_hint"])
	hint.Wrapping = fyne.TextWrapWord

	leftTop := container.NewVBox(
		searchEntry,
		container.NewBorder(nil, nil, nil, sortSelect, configuredCheck),
	)
	leftPanel := container.NewBorder(leftTop, hint, nil, nil, modList)

	applyModFilter() // initial population

	split := container.NewHSplit(leftPanel, editorScroll)
	split.Offset = 0.34
	win.SetContent(container.NewThemeOverride(split, selTheme))
	win.Resize(fyne.NewSize(780, 580))
	win.Show()
}
