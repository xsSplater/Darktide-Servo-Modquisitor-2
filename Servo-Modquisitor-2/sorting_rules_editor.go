// Servo-Modquisitor-2/sorting_rules_editor.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"encoding/json"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type rulesEditorState struct {
	Data     checks.ExternalData
	FilePath string
	Dirty    atomic.Bool
}

type TreeRow struct {
	IsGroup     bool
	FolderName  string
	DisplayName string
	RuleIdx     int // индекс группы в state.Data.LoadOrder / Dependencies
	ChildIdx    int // для !IsGroup: индекс в After[] / Dependents[]
	IsLast      bool
}

type tabContext struct {
	Refresh func()
	Add     func() // «+ After» / «+ Dependent» / «+ Add»
	AddAlt  func() // «+ Before» / «+ Required» (nil если не применимо)
	Remove  func()
	Up      func()
	Down    func()
}

// ─── Утилиты ─────────────────────────────────────────────────────

// hasCycleLoadOrder проверяет, создаст ли новое ребро from->to
// цикл в текущем графе правил.
func hasCycleLoadOrder(rules []checks.LoadOrderRule, from, to string) bool {
	// adjacency: before -> список after
	adj := make(map[string][]string)
	for _, g := range rules {
		for _, after := range g.After {
			adj[g.Before] = append(adj[g.Before], after)
		}
	}
	// Если уже есть путь to -> ... -> from, то добавление from->to создаст цикл.
	visited := make(map[string]bool)
	var dfs func(n string) bool
	dfs = func(n string) bool {
		if n == from {
			return true
		}
		if visited[n] {
			return false
		}
		visited[n] = true
		for _, next := range adj[n] {
			if dfs(next) {
				return true
			}
		}
		return false
	}
	return dfs(to)
}

// filterLoadOrder удаляет пустые группы и дубликаты before,
// сохраняя порядок добавления After (НЕ сортирует).
func filterLoadOrder(rules []checks.LoadOrderRule) []checks.LoadOrderRule {
	// byBefore[a] — множество already-seen After для before=a.
	byBefore := make(map[string]map[string]bool)
	// orderBefore — порядок появления before.
	var orderBefore []string
	// orderAfter[a] — порядок появления After для before=a.
	orderAfter := make(map[string][]string)

	for _, g := range rules {
		if _, ok := byBefore[g.Before]; !ok {
			byBefore[g.Before] = make(map[string]bool)
			orderBefore = append(orderBefore, g.Before)
		}
		for _, a := range g.After {
			if !byBefore[g.Before][a] {
				byBefore[g.Before][a] = true
				orderAfter[g.Before] = append(orderAfter[g.Before], a)
			}
		}
	}

	var out []checks.LoadOrderRule
	for _, before := range orderBefore {
		afters := orderAfter[before]
		if len(afters) == 0 {
			continue
		}
		out = append(out, checks.LoadOrderRule{Before: before, After: afters})
	}
	return out
}

// filterDependencies удаляет пустые группы и дубликаты required,
// сохраняя порядок добавления Dependents (НЕ сортирует).
func filterDependencies(deps []checks.Dependency) []checks.Dependency {
	byReq := make(map[string]map[string]bool)
	urlByReq := make(map[string]string)
	var orderReq []string
	orderDeps := make(map[string][]string)

	for _, d := range deps {
		if _, ok := byReq[d.Required]; !ok {
			byReq[d.Required] = make(map[string]bool)
			orderReq = append(orderReq, d.Required)
			urlByReq[d.Required] = d.RequiredURL
		} else if urlByReq[d.Required] == "" && d.RequiredURL != "" {
			urlByReq[d.Required] = d.RequiredURL
		}
		for _, dep := range d.Dependents {
			if !byReq[d.Required][dep] {
				byReq[d.Required][dep] = true
				orderDeps[d.Required] = append(orderDeps[d.Required], dep)
			}
		}
	}

	var out []checks.Dependency
	for _, required := range orderReq {
		dependents := orderDeps[required]
		if len(dependents) == 0 {
			continue
		}
		out = append(out, checks.Dependency{
			Required:    required,
			RequiredURL: urlByReq[required],
			Dependents:  dependents,
		})
	}
	return out
}

// cleanDescs удаляет пустые языки из Desc, а пустые map обнуляет.
func cleanDescs(pairs []checks.IncompatiblePair) {
	for i := range pairs {
		p := &pairs[i]
		if p.Desc == nil {
			continue
		}
		for k, v := range p.Desc {
			if strings.TrimSpace(v) == "" {
				delete(p.Desc, k)
			}
		}
		if len(p.Desc) == 0 {
			p.Desc = nil
		}
	}
}

// dedupStrings убирает дубликаты и пустые строки, сохраняя порядок
// первого появления. Для flat-списков (obsolete_mods, mandatory_order),
// куда дубликаты могут попасть после ручной правки JSON.
//
// TrimSpace перед проверкой: в JSON от руки легко получить " base" с
// пробелом, и он не совпадёт с "base" при проверке. Убираем пробелы
// явно, чтобы не накапливать «фантомные» записи.
func dedupStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ─── Главное окно ────────────────────────────────────────────────

func (app *App) showSortingRulesEditor() {
	if !app.acquireWindow("sorting_rules") {
		return
	}

	filePath := filepath.Join(checks.GlobalDataDir(), FileNameMandatoryRules)
	state := &rulesEditorState{FilePath: filePath}

	data, err := os.ReadFile(filePath)
	if err != nil {
		app.showInfoDialog(app.msg("error_title"),
			fmt.Sprintf(app.msg("rules_load_read_error"), err))
		return
	}
	if err := json.Unmarshal(data, &state.Data); err != nil {
		app.showInfoDialog(app.msg("error_title"),
			fmt.Sprintf(app.msg("rules_load_parse_error"), err))
		return
	}

	win := app.myApp.NewWindow(app.msg("rules_editor_title"))
	app.registerWindow("sorting_rules", win)
	win.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
	win.CenterOnScreen()

	iconSkull := app.loadIconResource("skull", "assets/buttons/skull.png")
	iconPin := app.loadIconResource("pin_bir", "assets/buttons/pin_bir.png")
	iconTasks := app.loadIconResource("tasks", "assets/buttons/tasks.png")
	iconConflict := app.loadIconResource("conflict", "assets/buttons/conflict.png")
	iconWarn := app.loadIconResource("warn_yellow_romb", "assets/buttons/warn_yellow_romb.png")

	var rightPanelContent *fyne.Container
	var currentRightID string
	var activeCtx *tabContext
	var saveBtn *CustomButton
	var searchEntry *widget.Entry

	updateDirtyUI := func() {
		state.Dirty.Store(true)
		win.SetTitle(app.msg("rules_editor_title") + " *")
		app.startBlink(saveBtn, &app.blinkRulesEditorActive, func() bool { return state.Dirty.Load() })
	}

	// ─── Помощники ──────────────────────────────────────────────

	makeHighlightedText := func(text, query string, bold bool) fyne.CanvasObject {
		if query == "" {
			lbl := widget.NewLabel(text)
			if bold {
				lbl.TextStyle = fyne.TextStyle{Bold: true}
			}
			return lbl
		}
		style := widget.RichTextStyle{Inline: true}
		if bold {
			style.TextStyle = fyne.TextStyle{Bold: true}
		}
		segs := createHighlightedSegments(text, query, style)
		rt := widget.NewRichText(segs...)
		rt.Wrapping = fyne.TextWrapOff
		rt.Truncation = fyne.TextTruncateEllipsis
		return rt
	}

	// buildModCard — карточка мода. Используется во всех вкладках.
	buildModCard := func(folderName, expectID string, hideImage bool) fyne.CanvasObject {
		entry := checks.GetModDBEntry(folderName)
		if entry == nil {
			return widget.NewLabel(fmt.Sprintf("Mod: %s (Not in DB)", folderName))
		}

		app.cfgMutex.RLock()
		lang := app.cfg.Language
		app.cfgMutex.RUnlock()

		displayName := app.modDisplayName(folderName)

		title := canvas.NewText(displayName, theme.Color(theme.ColorNameForeground))
		title.TextSize = theme.TextSize() + 2
		title.TextStyle = fyne.TextStyle{Bold: true}

		titleSep := widget.NewThemedRectangle(themes.ColorCRTScreenStroke)
		titleSep.SetMinSize(fyne.NewSize(0, 3))

		author := widget.NewLabel(fmt.Sprintf(app.msg("author_label"), entry.Author))

		var links []fyne.CanvasObject
		if entry.URL != "" {
			if u, err := url.Parse(entry.URL); err == nil {
				links = append(links, widget.NewHyperlink(app.msg("mod_url_label"), u))
			}
		}
		if entry.SourceCodeURL != "" {
			if u, err := url.Parse(entry.SourceCodeURL); err == nil {
				links = append(links, widget.NewHyperlink(app.msg("source_code_url"), u))
			}
		}

		desc := checks.PickLocalized(entry.Description, lang)
		if desc == "" {
			desc = app.msg("desc_placeholder")
		}
		descLbl := widget.NewLabel(desc)
		descLbl.Wrapping = fyne.TextWrapWord

		metaLeft := container.NewVBox(
			author,
			widget.NewSeparator(),
			container.NewVBox(links...),
			widget.NewSeparator(),
			widget.NewLabel(fmt.Sprintf(app.msg("category_label")+": %s", entry.Category)),
		)

		var imgBox fyne.CanvasObject
		if !hideImage && entry.NexusPictureURL != "" {
			imgBg := widget.NewThemedRectangle("")
			imgBg.SetFillColor(color.NRGBA{R: 0, G: 0, B: 0, A: 15})
			imgBg.SetCornerRadius(CornerRadius15)
			imgBg.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

			img := canvas.NewImageFromResource(nil)
			img.FillMode = canvas.ImageFillContain
			img.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))
			img.CornerRadius = CornerRadius15

			imgBorder := widget.NewThemedRectangle("")
			imgBorder.SetCornerRadius(CornerRadius15)
			imgBorder.SetStroke(theme.ColorNameSeparator, 1)
			imgBorder.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

			imgBox = container.NewStack(imgBg, img, imgBorder)

			go func(u, expect string) {
				res, err := fetchNexusThumbnail(u)
				if err == nil {
					fyne.Do(func() {
						if currentRightID == expect {
							img.Resource = res
							img.Refresh()
						}
					})
				}
			}(entry.NexusPictureURL, expectID)
		} else {
			imgBox = canvas.NewRectangle(color.Transparent)
		}

		metaRow := container.NewBorder(nil, nil, nil, imgBox, metaLeft)

		return container.NewVBox(
			title,
			titleSep,
			metaRow,
			widget.NewSeparator(),
			descLbl,
		)
	}

	buildPlaceholder := func() fyne.CanvasObject {
		const (
			placeholderBoxSize  = 210.0
			placeholderLogoSize = 200.0
			placeholderGlowSize = 215.0
		)
		logoData, _ := embeddedFiles.ReadFile("assets/mechanicus.png")
		glowData, _ := embeddedFiles.ReadFile("assets/mechanicus_glow.png")
		logoRes := fyne.NewStaticResource("placeholder-logo.png", logoData)
		glowRes := fyne.NewStaticResource("placeholder-glow.png", glowData)

		glowImg := canvas.NewImageFromResource(glowRes)
		glowImg.FillMode = canvas.ImageFillStretch
		glowImg.Translucency = 0.25
		glowImg.SetMinSize(fyne.NewSize(placeholderGlowSize, placeholderGlowSize))

		logoImg := canvas.NewImageFromResource(logoRes)
		logoImg.FillMode = canvas.ImageFillStretch
		logoImg.SetMinSize(fyne.NewSize(placeholderLogoSize, placeholderLogoSize))

		logoStack := container.NewStack(
			container.NewCenter(glowImg),
			container.NewCenter(logoImg),
		)
		logoWithSize := container.NewGridWrap(fyne.NewSize(placeholderBoxSize, placeholderBoxSize), logoStack)

		title := canvas.NewText(app.msg("select_mod"), theme.Color(theme.ColorNameForeground))
		title.Alignment = fyne.TextAlignCenter
		title.TextSize = theme.TextSize() + 14
		title.TextStyle = fyne.TextStyle{Bold: true}

		hint := canvas.NewText(app.msg("desc_placeholder"), theme.Color(theme.ColorNamePlaceHolder))
		hint.Alignment = fyne.TextAlignCenter
		hint.TextSize = theme.TextSize() + 8

		return container.NewVBox(
			helpers.NewFixedSpacer(0, 40),
			container.NewCenter(logoWithSize),
			helpers.NewFixedSpacer(0, 16),
			container.NewCenter(title),
			helpers.NewFixedSpacer(0, 16),
			container.NewCenter(hint),
		)
	}

	// updateRightPanel — принимает строковый id:
	//   "" — placeholder
	//   "LOAD_<gi>_<ci>" — группа (ci=-1) или ребёнок Load Order
	//   "DEP_<gi>_<ci>"  — группа (ci=-1) или ребёнок Dependencies
	//   "INC_<i>"        — пара Incompatible
	//   "<folder>"       — обычный мод (Obsolete/Mandatory)
	updateRightPanel := func(id string) {
		currentRightID = id
		rightPanelContent.Objects = nil

		if id == "" {
			rightPanelContent.Add(buildPlaceholder())
			rightPanelContent.Refresh()
			return
		}

		switch {
		case strings.HasPrefix(id, "LOAD_"):
			parts := strings.Split(id, "_")
			if len(parts) != 3 {
				break
			}
			var gi, ci int
			fmt.Sscanf(parts[1], "%d", &gi)
			fmt.Sscanf(parts[2], "%d", &ci)
			if gi < 0 || gi >= len(state.Data.LoadOrder) {
				break
			}
			group := state.Data.LoadOrder[gi]

			if ci < 0 {
				// Группа (before)
				rightPanelContent.Add(widget.NewLabelWithStyle(
					fmt.Sprintf("%s: %s (%d %s)",
						app.msg("load_before"),
						app.modDisplayName(group.Before),
						len(group.After),
						app.msg("load_after")),
					fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(group.Before, id, false))
			} else if ci < len(group.After) {
				after := group.After[ci]
				rightPanelContent.Add(widget.NewLabelWithStyle(
					fmt.Sprintf("%s: %s -> %s",
						app.msg("load_order"),
						app.modDisplayName(group.Before),
						app.modDisplayName(after)),
					fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(after, id, false))
			}

		case strings.HasPrefix(id, "DEP_"):
			parts := strings.Split(id, "_")
			if len(parts) != 3 {
				break
			}
			var gi, ci int
			fmt.Sscanf(parts[1], "%d", &gi)
			fmt.Sscanf(parts[2], "%d", &ci)
			if gi < 0 || gi >= len(state.Data.Dependencies) {
				break
			}
			group := state.Data.Dependencies[gi]

			if ci < 0 {
				rightPanelContent.Add(widget.NewLabelWithStyle(
					fmt.Sprintf("%s: %s (%d %s)",
						app.msg("aml_col_required"),
						app.modDisplayName(group.Required),
						len(group.Dependents),
						app.msg("sort_mod_dependent")),
					fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(group.Required, id, false))
			} else if ci < len(group.Dependents) {
				dep := group.Dependents[ci]
				rightPanelContent.Add(widget.NewLabelWithStyle(
					fmt.Sprintf("%s: %s ← %s",
						app.msg("sort_mod_dependent"),
						app.modDisplayName(group.Required),
						app.modDisplayName(dep)),
					fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(dep, id, false))
			}

		case strings.HasPrefix(id, "INC_"):
			var idx int
			fmt.Sscanf(strings.TrimPrefix(id, "INC_"), "%d", &idx)
			if idx >= 0 && idx < len(state.Data.IncompatiblePairs) {
				pair := &state.Data.IncompatiblePairs[idx]

				rightPanelContent.Add(widget.NewLabelWithStyle(
					fmt.Sprintf("%s: %s ⚔ %s",
						app.msg("sort_conflict"),
						app.modDisplayName(pair.Mod1),
						app.modDisplayName(pair.Mod2)),
					fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(pair.Mod1, id, false))
				rightPanelContent.Add(widget.NewSeparator())
				rightPanelContent.Add(buildModCard(pair.Mod2, id, false))
				rightPanelContent.Add(widget.NewSeparator())

				// Multilang desc
				rightPanelContent.Add(widget.NewLabelWithStyle(
					app.msg("description")+":",
					fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))

				if pair.Desc == nil {
					pair.Desc = make(map[string]string)
				}
				langs := []string{
					"en", "ru", "de", "es", "fr", "it",
					"ja", "ko", "pl", "pt-br", "zh-cn", "zh-tw",
				}
				var curLang = "en"
				langSel := widget.NewSelect(langs, nil)
				langSel.SetSelected(curLang)

				descEntry := widget.NewMultiLineEntry()
				descEntry.Wrapping = fyne.TextWrapWord
				descEntry.SetMinRowsVisible(3)
				descEntry.SetText(pair.Desc[curLang])

				// suppressDescDirty гасит ложный Dirty при программном SetText
				// в langSel.OnChanged. Без него переключение языка воспринимается
				// как «пользователь отредактировал desc», и мигающая кнопка Save
				// включается на пустом месте.
				var suppressDescDirty bool

				descEntry.OnChanged = func(s string) {
					pair.Desc[curLang] = s
					if !suppressDescDirty {
						updateDirtyUI()
					}
				}
				langSel.OnChanged = func(l string) {
					curLang = l
					suppressDescDirty = true
					descEntry.SetText(pair.Desc[l])
					suppressDescDirty = false
				}

				rightPanelContent.Add(container.NewGridWithColumns(2,
					widget.NewLabel(app.msg("menu_language")+":"),
					langSel,
				))
				rightPanelContent.Add(descEntry)
			}

		default:
			// Обычный мод
			rightPanelContent.Add(buildModCard(id, id, false))
		}

		rightPanelContent.Refresh()
	}

	// ─── Глобальные кнопки ──────────────────────────────────────
	saveBtn = NewIconButton(app.loadIconResource("save", "assets/buttons/save.png"), func() {
		// Чистим пустые группы, пустые desc.
		state.Data.LoadOrder = filterLoadOrder(state.Data.LoadOrder)
		state.Data.Dependencies = filterDependencies(state.Data.Dependencies)
		state.Data.ObsoleteMods = dedupStrings(state.Data.ObsoleteMods)
		state.Data.MandatoryOrder = dedupStrings(state.Data.MandatoryOrder)
		cleanDescs(state.Data.IncompatiblePairs)

		outData, err := json.MarshalIndent(state.Data, "", "\t")
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("rules editor: marshal failed: %v", err))
			app.showInfoDialog(app.msg("error_title"),
				fmt.Sprintf("Failed to save rules: %v", err))
			return
		}
		_ = os.Rename(filePath, filePath+".bak")
		if err := os.WriteFile(filePath, outData, 0644); err != nil {
			app.appendLogToFile(fmt.Sprintf("rules editor: write failed: %v", err))
			app.showInfoDialog(app.msg("error_title"),
				fmt.Sprintf("Failed to save rules: %v", err))
			return
		}

		state.Dirty.Store(false)
		app.blinkRulesEditorActive.Store(false)
		win.SetTitle(app.msg("rules_editor_title"))
		saveBtn.Importance = widget.MediumImportance
		saveBtn.Refresh()
		app.appendLog(app.msg("log_sorting_saved"))

		// ─── Перезагружаем состояние программы из только что
		// сохранённого файла. checks и sorter держат копии в памяти,
		// загруженные при старте. Без перезагрузки автосортировка
		// продолжит работать по старым правилам, а obsolete/mandatory/
		// incompatible флаги останутся от прошлой версии.
		if err := checks.LoadExternalLists(FileNameMandatoryRules); err != nil {
			app.appendLogToFile(fmt.Sprintf(
				"rules editor: reload external lists failed: %v", err))
		} else {
			app.cfgMutex.Lock()
			app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
			app.cfgMutex.Unlock()
			app.saveConfigSafe()
		}
		sorter.SetMandatoryOrder(checks.GetMandatoryOrder())
		sorter.SetDependencies(convertDeps(checks.GetDependenciesFlat()))
		sorter.SetLoadOrderRules(checks.GetLoadOrderRules())

		// ─── Пересчёт визуальных флагов в главной таблице.
		// refreshModList перечитывает mod_load_order.txt с диска. Если
		// в главном окне есть pending orderDirty — это перезапишет
		// несохранённые изменения. Пропускаем, пишем подсказку в лог;
		// sorter/checks уже обновлены, так что будущая автосортировка
		// работает по новым правилам. Пользователь сам нажмёт Refresh
		// в главном окне, когда будет готов.
		if !app.orderDirty.Load() {
			fyne.Do(func() {
				app.refreshModList()
			})
		} else {
			app.appendLogToFile("rules saved: mod list refresh skipped " +
				"(pending order changes in main window)")
		}

		// ─── Пересобираем текущую вкладку редактора. filterLoadOrder/
		// filterDependencies могли изменить структуру state.Data (убрать
		// пустые группы). Без Refresh таблица и правый panel будут
		// ссылаться на старые индексы.
		if activeCtx != nil && activeCtx.Refresh != nil {
			activeCtx.Refresh()
		}
		updateRightPanel("")
	})
	saveBtn.SetToolTip(app.msg("sre_tooltip_save_list"))

	reloadBtn := NewIconButton(app.loadIconResource("refresh", "assets/buttons/refresh.png"), func() {
		app.forgetWindow("sorting_rules")
		win.Close()
		app.showSortingRulesEditor()
	})
	reloadBtn.SetToolTip(app.msg("sre_tooltip_reload_list"))

	addBtn := NewIconButton(app.loadIconResource("add", "assets/buttons/add.png"), func() {
		if activeCtx != nil && activeCtx.Add != nil {
			activeCtx.Add()
		}
	})
	addAltBtn := NewIconButton(app.loadIconResource("add", "assets/buttons/add.png"), func() {
		if activeCtx != nil && activeCtx.AddAlt != nil {
			activeCtx.AddAlt()
		}
	})
	addAltBtn.Hide()

	// applyTabLabels обновляет текст и tooltip кнопок Add/AddAlt
	// в зависимости от активной вкладки. Кнопки существуют всегда,
	// но семантика у них разная:
	//
	//   Obsolete/Mandatory: AddAlt = nil, единственная кнопка = «добавить мод»
	//   LoadOrder:          Add = «+After», AddAlt = «+Before»
	//   Incompatible:       AddAlt = nil, единственная = «+Pair»
	//   Dependencies:       Add = «+Dependent», AddAlt = «+Required»
	//
	// Без этого пользователь видел две одинаковые [ + ] и не понимал,
	// какая из них делает что.
	applyTabLabels := func(idx int) {
		switch idx {
		case 0, 1: // Obsolete, Mandatory
			addBtn.SetText("")
			addBtn.SetToolTip(app.msg("sre_add_mod"))
			addAltBtn.Hide()
		case 2: // Load Order
			addBtn.SetText(app.msg("after"))
			addBtn.SetToolTip(app.msg("sre_tooltip_add_after"))
			addAltBtn.SetText(app.msg("before"))
			addAltBtn.SetToolTip(app.msg("sre_tooltip_add_before"))
			addAltBtn.Show()
		case 3: // Incompatible
			addBtn.SetText("")
			addBtn.SetToolTip(app.msg("sre_tooltip_add_pair"))
			addAltBtn.Hide()
		case 4: // Dependencies
			addBtn.SetText(app.msg("sort_mod_dependent"))
			addBtn.SetToolTip(app.msg("sre_tooltip_add_dependent"))
			addAltBtn.SetText(app.msg("aml_col_required"))
			addAltBtn.SetToolTip(app.msg("sre_tooltip_add_required"))
			addAltBtn.Show()
		}
	}

	remBtn := NewIconButton(app.loadIconResource("trash_x", "assets/buttons/trashcan_red.png"), func() {
		if activeCtx != nil && activeCtx.Remove != nil {
			activeCtx.Remove()
		}
	})
	remBtn.SetToolTip(app.msg("sre_tooltip_remove"))

	upBtn := NewIconButton(app.loadIconResource("up", "assets/buttons/up.png"), func() {
		if activeCtx != nil && activeCtx.Up != nil {
			activeCtx.Up()
		}
	})
	upBtn.SetToolTip(app.msg("sre_tooltip_move_up"))

	downBtn := NewIconButton(app.loadIconResource("down", "assets/buttons/down.png"), func() {
		if activeCtx != nil && activeCtx.Down != nil {
			activeCtx.Down()
		}
	})
	downBtn.SetToolTip(app.msg("sre_tooltip_move_down"))

	searchField := app.NewSearchField(SearchFieldOptions{
		Width:         SearchMinWidth,
		DebounceDelay: 300 * time.Millisecond,
		ShowClear:     true,
		OnFilter: func(s string) {
			if activeCtx != nil && activeCtx.Refresh != nil {
				activeCtx.Refresh()
			}
		},
	})
	searchEntry = searchField.Entry
	searchBar := searchField.Container

	toolbar := container.NewHBox(
		addAltBtn, addBtn, remBtn,
		widget.NewSeparator(),
		upBtn, downBtn,
		widget.NewSeparator(),
		saveBtn, reloadBtn,
		widget.NewSeparator(),
		searchBar,
	)

	// ─── Простой список (Obsolete / Mandatory) ──────────────────

	buildSimpleListTab := func(items *[]string, ctx *tabContext) fyne.CanvasObject {
		var listTable *widget.Table
		getDisplayName := func(folder string) string { return app.modDisplayName(folder) }

		var filtered []string
		refresh := func() {
			query := strings.ToLower(searchEntry.Text)
			filtered = filtered[:0]
			for _, m := range *items {
				if query == "" || strings.Contains(strings.ToLower(getDisplayName(m)), query) || strings.Contains(strings.ToLower(m), query) {
					filtered = append(filtered, m)
				}
			}
			if listTable != nil {
				listTable.Length = func() (int, int) { return len(filtered), 4 }
				listTable.Refresh()
			}
		}

		listTable = widget.NewTable(
			func() (int, int) { return 0, 4 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				bg.SetMinSize(fyne.NewSize(1, 32))
				return container.NewStack(bg, widget.NewLabel(""))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				cont := cell.(*fyne.Container)
				bg := cont.Objects[0].(*canvas.Rectangle)
				if id.Row >= len(filtered) {
					return
				}
				th := app.myApp.Settings().Theme()
				variant := app.myApp.Settings().ThemeVariant()
				if listTable.IsRowSelected(id.Row) {
					bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
				} else if id.Row%2 == 0 {
					bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
				} else {
					bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
				}
				bg.Refresh()

				modFolder := filtered[id.Row]
				query := strings.ToLower(searchEntry.Text)

				cont.Objects = []fyne.CanvasObject{bg}
				switch id.Col {
				case 0:
					lbl := widget.NewLabel(fmt.Sprintf("%d", id.Row+1))
					lbl.Alignment = fyne.TextAlignCenter
					cont.Add(lbl)
				case 1:
					cont.Add(makeHighlightedText(getDisplayName(modFolder), query, false))
				case 2:
					author := "-"
					if entry := checks.GetModDBEntry(modFolder); entry != nil && entry.Author != "" {
						author = entry.Author
					}
					cont.Add(makeHighlightedText(author, query, false))
				case 3:
					note := app.userNotes.Get(modFolder)
					cont.Add(makeHighlightedText(note, query, false))
				}
				cont.Refresh()
			},
		)
		listTable.SetColumnWidth(0, ColNumberWidth)
		listTable.SetColumnWidth(1, ColNameWidth)
		listTable.SetColumnWidth(2, ColAuthorWidth)
		listTable.SetColumnWidth(3, ColNoteWidth)
		listTable.SetRowHeight(-1, ColSelectWidth)

		listTable.OnSelected = func(id widget.TableCellID) {
			if id.Row >= 0 && id.Row < len(filtered) {
				updateRightPanel(filtered[id.Row])
			}
		}

		ctx.Add = func() {
			existingSet := make(map[string]bool, len(*items))
			for _, ex := range *items {
				existingSet[ex] = true
			}
			var available []string
			for _, f := range app.allKnownModFolders() {
				if !existingSet[f] {
					available = append(available, f)
				}
			}

			app.ShowSearchPopup(SearchPopupOptions{
				Title:  app.msg("aml_btn_add"),
				Source: available,
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(sel string) {
					*items = append(*items, sel)
					refresh()
					updateDirtyUI()
				},
			})
		}
		ctx.AddAlt = nil

		ctx.Remove = func() {
			sel := listTable.SelectedRow()
			if sel >= 0 && sel < len(filtered) {
				target := filtered[sel]
				for i, v := range *items {
					if v == target {
						*items = append((*items)[:i], (*items)[i+1:]...)
						break
					}
				}
				refresh()
				listTable.UnselectAll()
				updateDirtyUI()
			}
		}
		ctx.Up = func() {
			sel := listTable.SelectedRow()
			if sel <= 0 || sel >= len(filtered) {
				return
			}
			// Меняем местами target и предыдущий ВИДИМЫЙ элемент (в *items).
			// Тогда они поменяются местами и в filtered — это ровно то,
			// что ожидает увидеть пользователь, глядя на отфильтрованный
			// список.
			//
			// Старая версия двигала target на idx-1 в ПОЛНОМ списке, где
			// между idx и idx-1 могли стоять скрытые фильтром элементы.
			// Ни filtered, ни выделение визуально не менялись.
			target := filtered[sel]
			prevVisible := filtered[sel-1]
			idxTarget, idxPrev := -1, -1
			for i, v := range *items {
				if v == target {
					idxTarget = i
				}
				if v == prevVisible {
					idxPrev = i
				}
			}
			if idxTarget < 0 || idxPrev < 0 {
				return
			}
			(*items)[idxTarget], (*items)[idxPrev] = (*items)[idxPrev], (*items)[idxTarget]
			refresh()
			listTable.Select(widget.TableCellID{Row: sel - 1, Col: 0}, 0)
			updateDirtyUI()
		}
		ctx.Down = func() {
			sel := listTable.SelectedRow()
			if sel < 0 || sel >= len(filtered)-1 {
				return
			}
			target := filtered[sel]
			nextVisible := filtered[sel+1]
			idxTarget, idxNext := -1, -1
			for i, v := range *items {
				if v == target {
					idxTarget = i
				}
				if v == nextVisible {
					idxNext = i
				}
			}
			if idxTarget < 0 || idxNext < 0 {
				return
			}
			(*items)[idxTarget], (*items)[idxNext] = (*items)[idxNext], (*items)[idxTarget]
			refresh()
			listTable.Select(widget.TableCellID{Row: sel + 1, Col: 0}, 0)
			updateDirtyUI()
		}
		ctx.Refresh = refresh
		refresh()

		header := widget.NewTable(
			func() (int, int) { return 1, 4 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(app.myApp.Settings().Theme().Color(themes.ColorTableHeaderBg, app.myApp.Settings().ThemeVariant()))
				bg.SetMinSize(fyne.NewSize(1, 30))
				return container.NewStack(bg, widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				lbl := cell.(*fyne.Container).Objects[1].(*widget.Label)
				switch id.Col {
				case 0:
					lbl.SetText(app.msg("col_number"))
				case 1:
					lbl.SetText(app.msg("col_name"))
				case 2:
					lbl.SetText(app.msg("col_author"))
				case 3:
					lbl.SetText(app.msg("col_note"))
				}
			},
		)
		header.SetColumnWidth(0, ColNumberWidth)
		header.SetColumnWidth(1, ColNameWidth)
		header.SetColumnWidth(2, ColAuthorWidth)
		header.SetColumnWidth(3, ColNoteWidth)
		header.SetRowHeight(-1, ColSelectWidth)

		borderRect := widget.NewThemedRectangle("")
		borderRect.SetStroke(themes.ColorCRTScreenStroke, 1)
		borderRect.SetFillColor(color.Transparent)

		tableContent := app.newTableBackground(TableBackgroundOpacity, listTable, borderRect)

		return container.NewBorder(
			container.NewVBox(toolbar, widget.NewSeparator(), header),
			nil, nil, nil,
			tableContent,
		)
	}

	// ─── Load Order ─────────────────────────────────────────────

	buildLoadOrderTab := func(ctx *tabContext) fyne.CanvasObject {
		var table *widget.Table
		var treeRows []TreeRow

		refresh := func() {
			query := strings.ToLower(searchEntry.Text)
			treeRows = nil
			for gi, group := range state.Data.LoadOrder {
				groupMatch := query == "" ||
					strings.Contains(strings.ToLower(group.Before), query) ||
					strings.Contains(strings.ToLower(app.modDisplayName(group.Before)), query)

				var children []int
				for ci, after := range group.After {
					if groupMatch || query == "" ||
						strings.Contains(strings.ToLower(after), query) ||
						strings.Contains(strings.ToLower(app.modDisplayName(after)), query) {
						children = append(children, ci)
					}
				}
				if !groupMatch && len(children) == 0 {
					continue
				}

				treeRows = append(treeRows, TreeRow{
					IsGroup:     true,
					FolderName:  group.Before,
					DisplayName: app.modDisplayName(group.Before),
					RuleIdx:     gi,
					ChildIdx:    -1,
				})
				for k, ci := range children {
					isLast := k == len(children)-1
					treeRows = append(treeRows, TreeRow{
						IsGroup:     false,
						FolderName:  group.After[ci],
						DisplayName: app.modDisplayName(group.After[ci]),
						RuleIdx:     gi,
						ChildIdx:    ci,
						IsLast:      isLast,
					})
				}
			}
			table.Length = func() (int, int) { return len(treeRows), 2 }
			table.Refresh()
		}

		table = widget.NewTable(
			func() (int, int) { return 0, 2 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				bg.SetMinSize(fyne.NewSize(1, 32))
				return container.NewStack(bg, widget.NewLabel(""))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				cont := cell.(*fyne.Container)
				bg := cont.Objects[0].(*canvas.Rectangle)
				if id.Row >= len(treeRows) {
					return
				}
				row := treeRows[id.Row]
				th := app.myApp.Settings().Theme()
				variant := app.myApp.Settings().ThemeVariant()
				if table.IsRowSelected(id.Row) {
					bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
				} else if id.Row%2 == 0 {
					bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
				} else {
					bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
				}
				bg.Refresh()

				cont.Objects = []fyne.CanvasObject{bg}
				query := strings.ToLower(searchEntry.Text)
				switch id.Col {
				case 0:
					if row.IsGroup {
						cont.Add(makeHighlightedText(row.DisplayName, query, true))
					} else {
						textWidget := makeHighlightedText(row.DisplayName, query, false)
						cont.Add(app.makeTreeBranch(row.IsLast, textWidget))
					}
				case 1:
					if !row.IsGroup {
						group := state.Data.LoadOrder[row.RuleIdx]
						cont.Add(widget.NewLabel(app.modDisplayName(group.Before)))
					} else {
						cont.Add(widget.NewLabel(""))
					}
				}
				cont.Refresh()
			},
		)
		table.SetColumnWidth(0, ColNoteWidth)
		table.SetColumnWidth(1, ColNameWidth)
		table.SetRowHeight(-1, ColSelectWidth)

		table.OnSelected = func(id widget.TableCellID) {
			if id.Row >= 0 && id.Row < len(treeRows) {
				row := treeRows[id.Row]
				updateRightPanel(fmt.Sprintf("LOAD_%d_%d", row.RuleIdx, row.ChildIdx))
			}
		}

		table.OnSecondaryTapped = func(id widget.TableCellID, ev *fyne.PointEvent) {
			if id.Row < 0 || id.Row >= len(treeRows) {
				return
			}
			if !table.IsRowSelected(id.Row) {
				table.Select(id, 0)
			}
			row := treeRows[id.Row]
			menu := fyne.NewMenu("",
				fyne.NewMenuItem(app.msg("ctx_rule_delete"), func() {
					ctx.Remove()
				}),
			)
			if row.IsGroup {
				menu.Items = append([]*fyne.MenuItem{
					fyne.NewMenuItem(app.msg("ctx_rules_remove")+" ("+fmt.Sprint(len(state.Data.LoadOrder[row.RuleIdx].After))+")", func() {
						// Удалить всю группу
						doRemoveGroup := func() {
							state.Data.LoadOrder = append(
								state.Data.LoadOrder[:row.RuleIdx],
								state.Data.LoadOrder[row.RuleIdx+1:]...,
							)
							refresh()
							table.UnselectAll()
							updateRightPanel("")
							updateDirtyUI()
						}
						n := len(state.Data.LoadOrder[row.RuleIdx].After)
						if n > 1 {
							app.showChoiceDialog(win,
								app.msg("confirm_delete_title"),
								fmt.Sprintf(app.msg("confirm_delete_text"), fmt.Sprintf("%d %s", n, app.msg("aml_col_load_after"))),
								func(choice int) {
									if choice == 0 {
										doRemoveGroup()
									}
								},
								app.msg("btn_yes"), app.msg("btn_no"),
							)
						} else {
							doRemoveGroup()
						}
					}),
					fyne.NewMenuItemSeparator(),
				}, menu.Items...)
			}
			widget.ShowPopUpMenuAtPosition(menu, win.Canvas(), ev.AbsolutePosition)
		}

		// ctx.Add — «+ After»: добавить after к выделенной группе или создать новую
		ctx.Add = func() {
			sel := table.SelectedRow()
			var groupIdx = -1
			if sel >= 0 && sel < len(treeRows) {
				groupIdx = treeRows[sel].RuleIdx
			}
			if groupIdx < 0 {
				app.showInfoDialog(app.msg("warning_title"), app.msg("sre_warning_after"))
				return
			}
			// Далее идет существующий код, который добавляет After к группе (без else-ветки!)
			group := &state.Data.LoadOrder[groupIdx]
			app.ShowSearchPopup(SearchPopupOptions{
				Title:  app.msg("sort_mod") + "(" + app.msg("aml_col_load_after") + ")",
				Source: app.allKnownModFolders(),
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(sel string) {
					for _, a := range group.After {
						if a == sel {
							return
						}
					}
					if hasCycleLoadOrder(state.Data.LoadOrder, group.Before, sel) {
						app.showChoiceDialog(win,
							app.msg("warning_title"),
							fmt.Sprintf(app.msg("cycle_warning"), app.modDisplayName(group.Before), app.modDisplayName(sel)),
							func(choice int) {
								if choice != 0 {
									return
								}
								group.After = append(group.After, sel)
								refresh()
								updateDirtyUI()
							},
							app.msg("btn_yes"), app.msg("btn_no"),
						)
						return
					}
					group.After = append(group.After, sel)
					refresh()
					updateDirtyUI()
				},
			})
		}

		// ctx.AddAlt — «+ Before»: всегда создаёт новую группу (не зависит от выделения)
		ctx.AddAlt = func() {
			app.ShowSearchPopup(SearchPopupOptions{
				Title:  app.msg("sort_mod") + "(" + app.msg("aml_col_load_before") + ")",
				Source: app.allKnownModFolders(),
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(before string) {
					app.ShowSearchPopup(SearchPopupOptions{
						Title:  app.msg("sort_mod") + "(" + app.msg("aml_col_load_after") + ")",
						Source: app.allKnownModFolders(),
						Parent: win,
						FormatRow: func(f string) string {
							return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
						},
						OnPick: func(after string) {
							if before == after {
								return
							}
							applyBefore := func() {
								for i := range state.Data.LoadOrder {
									if state.Data.LoadOrder[i].Before == before {
										for _, a := range state.Data.LoadOrder[i].After {
											if a == after {
												return
											}
										}
										state.Data.LoadOrder[i].After = append(state.Data.LoadOrder[i].After, after)
										refresh()
										updateDirtyUI()
										return
									}
								}
								state.Data.LoadOrder = append(state.Data.LoadOrder, checks.LoadOrderRule{
									Before: before,
									After:  []string{after},
								})
								refresh()
								updateDirtyUI()
							}
							if hasCycleLoadOrder(state.Data.LoadOrder, before, after) {
								app.showChoiceDialog(win,
									app.msg("warning_title"),
									fmt.Sprintf(app.msg("cycle_warning"), app.modDisplayName(before), app.modDisplayName(after)),
									func(choice int) {
										if choice != 0 {
											return
										}
										applyBefore()
									},
									app.msg("btn_yes"), app.msg("btn_no"),
								)
								return
							}
							applyBefore()
						},
					})
				},
			})
		}

		ctx.Remove = func() {
			sel := table.SelectedRow()
			if sel < 0 || sel >= len(treeRows) {
				return
			}
			row := treeRows[sel]
			if row.IsGroup {
				n := len(state.Data.LoadOrder[row.RuleIdx].After)
				doRemove := func() {
					state.Data.LoadOrder = append(
						state.Data.LoadOrder[:row.RuleIdx],
						state.Data.LoadOrder[row.RuleIdx+1:]...,
					)
					refresh()
					table.UnselectAll()
					updateRightPanel("")
					updateDirtyUI()
				}
				if n > 1 {
					app.showChoiceDialog(win,
						app.msg("confirm_delete_title"),
						fmt.Sprintf(app.msg("confirm_delete_text"), fmt.Sprintf("%d %s", n, app.msg("aml_col_load_after"))),
						func(choice int) {
							if choice == 0 {
								doRemove()
							}
						},
						app.msg("btn_yes"), app.msg("btn_no"),
					)
				} else {
					doRemove()
				}
			} else {
				group := &state.Data.LoadOrder[row.RuleIdx]
				group.After = append(group.After[:row.ChildIdx], group.After[row.ChildIdx+1:]...)
				if len(group.After) == 0 {
					state.Data.LoadOrder = append(
						state.Data.LoadOrder[:row.RuleIdx],
						state.Data.LoadOrder[row.RuleIdx+1:]...,
					)
				}
				refresh()
				table.UnselectAll()
				updateRightPanel("")
				updateDirtyUI()
			}
		}
		ctx.Up = func() {}
		ctx.Down = func() {}
		ctx.Refresh = refresh
		refresh()

		header := widget.NewTable(
			func() (int, int) { return 1, 2 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(app.myApp.Settings().Theme().Color(themes.ColorTableHeaderBg, app.myApp.Settings().ThemeVariant()))
				bg.SetMinSize(fyne.NewSize(1, 30))
				return container.NewStack(bg, widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				lbl := cell.(*fyne.Container).Objects[1].(*widget.Label)
				switch id.Col {
				case 0:
					lbl.SetText(app.msg("sort_mods_struct"))
				case 1:
					lbl.SetText(app.msg("aml_col_load_before"))
				}
			},
		)
		header.SetColumnWidth(0, ColNoteWidth)
		header.SetColumnWidth(1, ColNameWidth)
		header.SetRowHeight(-1, ColSelectWidth)

		borderRect := widget.NewThemedRectangle("")
		borderRect.SetStroke(themes.ColorCRTScreenStroke, 1)
		borderRect.SetFillColor(color.Transparent)

		tableContent := app.newTableBackground(TableBackgroundOpacity, table, borderRect)

		return container.NewBorder(
			container.NewVBox(toolbar, widget.NewSeparator(), header),
			nil, nil, nil,
			tableContent,
		)
	}

	// ─── Incompatible ───────────────────────────────────────────

	buildIncompatibleTab := func(ctx *tabContext) fyne.CanvasObject {
		var table *widget.Table
		var incRows []int
		refresh := func() {
			query := strings.ToLower(searchEntry.Text)
			incRows = nil
			for i, p := range state.Data.IncompatiblePairs {
				if query == "" ||
					strings.Contains(strings.ToLower(p.Mod1), query) ||
					strings.Contains(strings.ToLower(p.Mod2), query) {
					incRows = append(incRows, i)
				}
			}
			table.Length = func() (int, int) { return len(incRows), 4 }
			table.Refresh()
		}

		table = widget.NewTable(
			func() (int, int) { return 0, 4 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				bg.SetMinSize(fyne.NewSize(1, 32))
				return container.NewStack(bg, widget.NewLabel(""))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				cont := cell.(*fyne.Container)
				bg := cont.Objects[0].(*canvas.Rectangle)
				if id.Row >= len(incRows) {
					return
				}
				pair := state.Data.IncompatiblePairs[incRows[id.Row]]
				th := app.myApp.Settings().Theme()
				variant := app.myApp.Settings().ThemeVariant()
				if table.IsRowSelected(id.Row) {
					bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
				} else if id.Row%2 == 0 {
					bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
				} else {
					bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
				}
				bg.Refresh()

				cont.Objects = []fyne.CanvasObject{bg}
				query := strings.ToLower(searchEntry.Text)
				switch id.Col {
				case 0:
					lbl := widget.NewLabel(fmt.Sprintf("%d", id.Row+1))
					lbl.Alignment = fyne.TextAlignCenter
					cont.Add(lbl)
				case 1:
					cont.Add(makeHighlightedText(app.modDisplayName(pair.Mod1), query, false))
				case 2:
					cont.Add(makeHighlightedText(app.modDisplayName(pair.Mod2), query, false))
				case 3:
					cont.Add(makeHighlightedText(pair.Type, query, false))
				}
				cont.Refresh()
			},
		)
		table.SetColumnWidth(0, ColNumberWidth)
		table.SetColumnWidth(1, ColNameWidth)
		table.SetColumnWidth(2, ColNameWidth)
		table.SetColumnWidth(3, ColCategoryWidth)
		table.SetRowHeight(-1, ColSelectWidth)

		table.OnSecondaryTapped = func(id widget.TableCellID, ev *fyne.PointEvent) {
			if id.Row < 0 || id.Row >= len(incRows) {
				return
			}
			if !table.IsRowSelected(id.Row) {
				table.Select(id, 0)
			}
			menu := fyne.NewMenu("",
				fyne.NewMenuItem(app.msg("ctx_conflict_delete"), func() {
					ctx.Remove()
				}),
			)
			widget.ShowPopUpMenuAtPosition(menu, win.Canvas(), ev.AbsolutePosition)
		}

		table.OnSelected = func(id widget.TableCellID) {
			if id.Row >= 0 && id.Row < len(incRows) {
				updateRightPanel(fmt.Sprintf("INC_%d", incRows[id.Row]))
			}
		}

		ctx.Add = func() {
			app.ShowSearchPopup(SearchPopupOptions{
				Title:  fmt.Sprintf("%s #1", app.msg("sort_mod")),
				Source: app.allKnownModFolders(),
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(m1 string) {
					app.ShowSearchPopup(SearchPopupOptions{
						Title:  fmt.Sprintf("%s #2", app.msg("sort_mod")),
						Source: app.allKnownModFolders(),
						Parent: win,
						FormatRow: func(f string) string {
							return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
						},
						OnPick: func(m2 string) {
							if m1 == m2 {
								return
							}
							state.Data.IncompatiblePairs = append(state.Data.IncompatiblePairs,
								checks.IncompatiblePair{Mod1: m1, Mod2: m2})
							refresh()
							updateDirtyUI()
						},
					})
				},
			})
		}
		ctx.AddAlt = nil

		ctx.Remove = func() {
			sel := table.SelectedRow()
			if sel >= 0 && sel < len(incRows) {
				idx := incRows[sel]
				state.Data.IncompatiblePairs = append(
					state.Data.IncompatiblePairs[:idx],
					state.Data.IncompatiblePairs[idx+1:]...,
				)
				refresh()
				table.UnselectAll()
				updateRightPanel("")
				updateDirtyUI()
			}
		}
		ctx.Up = func() {}
		ctx.Down = func() {}
		ctx.Refresh = refresh
		refresh()

		header := widget.NewTable(
			func() (int, int) { return 1, 4 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(app.myApp.Settings().Theme().Color(themes.ColorTableHeaderBg, app.myApp.Settings().ThemeVariant()))
				bg.SetMinSize(fyne.NewSize(1, 30))
				return container.NewStack(bg, widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				lbl := cell.(*fyne.Container).Objects[1].(*widget.Label)
				switch id.Col {
				case 0:
					lbl.SetText(app.msg("col_number"))
				case 1:
					lbl.SetText(app.msg("sort_mod") + "1")
				case 2:
					lbl.SetText(app.msg("sort_mod") + "2")
				case 3:
					lbl.SetText(app.msg("sort_mod_type"))
				}
			},
		)
		header.SetColumnWidth(0, ColNumberWidth)
		header.SetColumnWidth(1, ColNameWidth)
		header.SetColumnWidth(2, ColNameWidth)
		header.SetColumnWidth(3, ColCategoryWidth)
		header.SetRowHeight(-1, ColSelectWidth)

		borderRect := widget.NewThemedRectangle("")
		borderRect.SetStroke(themes.ColorCRTScreenStroke, 1)
		borderRect.SetFillColor(color.Transparent)

		tableContent := app.newTableBackground(TableBackgroundOpacity, table, borderRect)

		return container.NewBorder(
			container.NewVBox(toolbar, widget.NewSeparator(), header),
			nil, nil, nil,
			tableContent,
		)
	}

	// ─── Dependencies ───────────────────────────────────────────

	buildDependenciesTab := func(ctx *tabContext) fyne.CanvasObject {
		var table *widget.Table
		var treeRows []TreeRow

		refresh := func() {
			query := strings.ToLower(searchEntry.Text)
			treeRows = nil
			for gi, group := range state.Data.Dependencies {
				groupMatch := query == "" ||
					strings.Contains(strings.ToLower(group.Required), query) ||
					strings.Contains(strings.ToLower(app.modDisplayName(group.Required)), query)

				var children []int
				for ci, dep := range group.Dependents {
					if groupMatch || query == "" ||
						strings.Contains(strings.ToLower(dep), query) ||
						strings.Contains(strings.ToLower(app.modDisplayName(dep)), query) {
						children = append(children, ci)
					}
				}
				if !groupMatch && len(children) == 0 {
					continue
				}

				treeRows = append(treeRows, TreeRow{
					IsGroup:     true,
					FolderName:  group.Required,
					DisplayName: app.modDisplayName(group.Required),
					RuleIdx:     gi,
					ChildIdx:    -1,
				})
				for k, ci := range children {
					isLast := k == len(children)-1
					treeRows = append(treeRows, TreeRow{
						IsGroup:     false,
						FolderName:  group.Dependents[ci],
						DisplayName: app.modDisplayName(group.Dependents[ci]),
						RuleIdx:     gi,
						ChildIdx:    ci,
						IsLast:      isLast,
					})
				}
			}
			table.Length = func() (int, int) { return len(treeRows), 2 }
			table.Refresh()
		}

		table = widget.NewTable(
			func() (int, int) { return 0, 2 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(color.Transparent)
				bg.SetMinSize(fyne.NewSize(1, 32))
				return container.NewStack(bg, widget.NewLabel(""))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				cont := cell.(*fyne.Container)
				bg := cont.Objects[0].(*canvas.Rectangle)
				if id.Row >= len(treeRows) {
					return
				}
				row := treeRows[id.Row]
				th := app.myApp.Settings().Theme()
				variant := app.myApp.Settings().ThemeVariant()
				if table.IsRowSelected(id.Row) {
					bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
				} else if id.Row%2 == 0 {
					bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
				} else {
					bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
				}
				bg.Refresh()

				cont.Objects = []fyne.CanvasObject{bg}
				query := strings.ToLower(searchEntry.Text)
				switch id.Col {
				case 0:
					if row.IsGroup {
						cont.Add(makeHighlightedText(row.DisplayName, query, true))
					} else {
						textWidget := makeHighlightedText(row.DisplayName, query, false)
						cont.Add(app.makeTreeBranch(row.IsLast, textWidget))
					}
				case 1:
					if !row.IsGroup {
						cont.Add(makeHighlightedText(app.msg("sort_mod_dependent"), query, false))
					} else {
						cont.Add(widget.NewLabel(""))
					}
				}
				cont.Refresh()
			},
		)
		table.SetColumnWidth(0, ColNameWidth)
		table.SetColumnWidth(1, ColNoteWidth)
		table.SetRowHeight(-1, ColSelectWidth)

		table.OnSelected = func(id widget.TableCellID) {
			if id.Row >= 0 && id.Row < len(treeRows) {
				row := treeRows[id.Row]
				updateRightPanel(fmt.Sprintf("DEP_%d_%d", row.RuleIdx, row.ChildIdx))
			}
		}

		table.OnSecondaryTapped = func(id widget.TableCellID, ev *fyne.PointEvent) {
			if id.Row < 0 || id.Row >= len(treeRows) {
				return
			}
			if !table.IsRowSelected(id.Row) {
				table.Select(id, 0)
			}
			row := treeRows[id.Row]
			menu := fyne.NewMenu("",
				fyne.NewMenuItem(app.msg("ctx_rule_delete"), func() {
					ctx.Remove()
				}),
			)
			if row.IsGroup {
				menu.Items = append([]*fyne.MenuItem{
					fyne.NewMenuItem(app.msg("ctx_rules_remove")+" ("+fmt.Sprint(len(state.Data.Dependencies[row.RuleIdx].Dependents))+")", func() {
						doRemoveGroup := func() {
							state.Data.Dependencies = append(
								state.Data.Dependencies[:row.RuleIdx],
								state.Data.Dependencies[row.RuleIdx+1:]...,
							)
							refresh()
							table.UnselectAll()
							updateRightPanel("")
							updateDirtyUI()
						}
						n := len(state.Data.Dependencies[row.RuleIdx].Dependents)
						if n > 1 {
							app.showChoiceDialog(win,
								app.msg("confirm_delete_title"),
								fmt.Sprintf(app.msg("confirm_delete_text"), fmt.Sprintf("%d %s", n, app.msg("sort_mod_dependent"))),
								func(choice int) {
									if choice == 0 {
										doRemoveGroup()
									}
								},
								app.msg("btn_yes"), app.msg("btn_no"),
							)
						} else {
							doRemoveGroup()
						}
					}),
					fyne.NewMenuItemSeparator(),
				}, menu.Items...)
			}
			widget.ShowPopUpMenuAtPosition(menu, win.Canvas(), ev.AbsolutePosition)
		}

		// ctx.Add — «+ Dependent»: добавить dependent к выделенной группе required
		ctx.Add = func() {
			sel := table.SelectedRow()
			var groupIdx = -1
			if sel >= 0 && sel < len(treeRows) {
				groupIdx = treeRows[sel].RuleIdx
			}
			if groupIdx < 0 {
				app.showInfoDialog(app.msg("warning_title"), app.msg("sre_warning_dependent"))
				return
			}
			group := &state.Data.Dependencies[groupIdx]
			app.ShowSearchPopup(SearchPopupOptions{
				Title:  app.msg("sort_mod") + "(" + app.msg("sort_mod_dependent") + ")",
				Source: app.allKnownModFolders(),
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(sel string) {
					for _, d := range group.Dependents {
						if d == sel {
							return
						}
					}
					group.Dependents = append(group.Dependents, sel)
					refresh()
					updateDirtyUI()
				},
			})
		}

		// ctx.AddAlt — «+ Required»: всегда создаёт новую группу (не зависит от выделения)
		ctx.AddAlt = func() {
			app.ShowSearchPopup(SearchPopupOptions{
				Title:  app.msg("sort_mod") + "(" + app.msg("aml_col_required") + ")",
				Source: app.allKnownModFolders(),
				Parent: win,
				FormatRow: func(f string) string {
					return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
				},
				OnPick: func(required string) {
					app.ShowSearchPopup(SearchPopupOptions{
						Title:  app.msg("sort_mod") + "(" + app.msg("sort_mod_dependent") + ")",
						Source: app.allKnownModFolders(),
						Parent: win,
						FormatRow: func(f string) string {
							return fmt.Sprintf("%s (%s)", app.modDisplayName(f), f)
						},
						OnPick: func(dependent string) {
							if required == dependent {
								return
							}
							addDep := func(dep string) {
								for i := range state.Data.Dependencies {
									if state.Data.Dependencies[i].Required == required {
										for _, d := range state.Data.Dependencies[i].Dependents {
											if d == dep {
												return
											}
										}
										state.Data.Dependencies[i].Dependents = append(
											state.Data.Dependencies[i].Dependents, dep)
										refresh()
										updateDirtyUI()
										return
									}
								}
								url := ""
								if e := checks.GetModDBEntry(required); e != nil {
									url = e.URL
								}
								state.Data.Dependencies = append(state.Data.Dependencies, checks.Dependency{
									Required:    required,
									RequiredURL: url,
									Dependents:  []string{dep},
								})
								refresh()
								updateDirtyUI()
							}
							addDep(dependent)
						},
					})
				},
			})
		}

		ctx.Remove = func() {
			sel := table.SelectedRow()
			if sel < 0 || sel >= len(treeRows) {
				return
			}
			row := treeRows[sel]
			if row.IsGroup {
				n := len(state.Data.Dependencies[row.RuleIdx].Dependents)
				doRemove := func() {
					state.Data.Dependencies = append(
						state.Data.Dependencies[:row.RuleIdx],
						state.Data.Dependencies[row.RuleIdx+1:]...,
					)
					refresh()
					table.UnselectAll()
					updateRightPanel("")
					updateDirtyUI()
				}
				if n > 1 {
					app.showChoiceDialog(win,
						app.msg("confirm_delete_title"),
						fmt.Sprintf(app.msg("confirm_delete_text"), fmt.Sprintf("%d %s", n, app.msg("sort_mod_dependent"))),
						func(choice int) {
							if choice == 0 {
								doRemove()
							}
						},
						app.msg("btn_yes"), app.msg("btn_no"),
					)
				} else {
					doRemove()
				}
			} else {
				group := &state.Data.Dependencies[row.RuleIdx]
				group.Dependents = append(group.Dependents[:row.ChildIdx], group.Dependents[row.ChildIdx+1:]...)
				if len(group.Dependents) == 0 {
					state.Data.Dependencies = append(
						state.Data.Dependencies[:row.RuleIdx],
						state.Data.Dependencies[row.RuleIdx+1:]...,
					)
				}
				refresh()
				table.UnselectAll()
				updateRightPanel("")
				updateDirtyUI()
			}
		}
		ctx.Up = func() {}
		ctx.Down = func() {}
		ctx.Refresh = refresh
		refresh()

		header := widget.NewTable(
			func() (int, int) { return 1, 2 },
			func() fyne.CanvasObject {
				bg := canvas.NewRectangle(app.myApp.Settings().Theme().Color(themes.ColorTableHeaderBg, app.myApp.Settings().ThemeVariant()))
				bg.SetMinSize(fyne.NewSize(1, 30))
				return container.NewStack(bg, widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}))
			},
			func(id widget.TableCellID, cell fyne.CanvasObject) {
				lbl := cell.(*fyne.Container).Objects[1].(*widget.Label)
				switch id.Col {
				case 0:
					lbl.SetText(app.msg("sort_mods_struct"))
				case 1:
					lbl.SetText(app.msg("col_note"))
				}
			},
		)
		header.SetColumnWidth(0, ColNameWidth)
		header.SetColumnWidth(1, ColNoteWidth)
		header.SetRowHeight(-1, ColSelectWidth)

		borderRect := widget.NewThemedRectangle("")
		borderRect.SetStroke(themes.ColorCRTScreenStroke, 1)
		borderRect.SetFillColor(color.Transparent)

		tableContent := app.newTableBackground(TableBackgroundOpacity, table, borderRect)

		return container.NewBorder(
			container.NewVBox(toolbar, widget.NewSeparator(), header),
			nil, nil, nil,
			tableContent,
		)
	}

	// ─── Собираем вкладки ───────────────────────────────────────

	ctxObsolete := &tabContext{}
	ctxMandatory := &tabContext{}
	ctxLoadOrder := &tabContext{}
	ctxInc := &tabContext{}
	ctxDep := &tabContext{}

	contexts := []*tabContext{ctxObsolete, ctxMandatory, ctxLoadOrder, ctxInc, ctxDep}

	tabItems := []*container.TabItem{
		container.NewTabItemWithIcon(app.msg("filter_obsolete"), iconSkull,
			buildSimpleListTab(&state.Data.ObsoleteMods, ctxObsolete)),
		container.NewTabItemWithIcon(app.msg("mandatory"), iconPin,
			buildSimpleListTab(&state.Data.MandatoryOrder, ctxMandatory)),
		container.NewTabItemWithIcon(app.msg("load_order"), iconTasks,
			buildLoadOrderTab(ctxLoadOrder)),
		container.NewTabItemWithIcon(app.msg("incompatible"), iconConflict,
			buildIncompatibleTab(ctxInc)),
		container.NewTabItemWithIcon(app.msg("info_editor_tab_dependencies"), iconWarn,
			buildDependenciesTab(ctxDep)),
	}

	tabs := container.NewAppTabs(tabItems...)
	tabs.SetTabLocation(container.TabLocationTop)

	// Инициализация активного контекста. На вкладке «Устаревшие» по умолчанию.
	activeCtx = ctxObsolete

	// Инициализация подписей для вкладки по умолчанию (Obsolete).
	// OnSelected может не сработать при первом показе вкладок, поэтому
	// вызываем applyTabLabels явно до установки колбэка.
	applyTabLabels(0)

	tabs.OnSelected = func(ti *container.TabItem) {
		for i, item := range tabItems {
			if item == ti {
				activeCtx = contexts[i]
				applyTabLabels(i)
				break
			}
		}
		if activeCtx != nil && activeCtx.Refresh != nil {
			activeCtx.Refresh()
		}
		updateRightPanel("")
	}

	rightPanelContent = container.NewVBox(buildPlaceholder())
	rightPanelScroll := container.NewVScroll(rightPanelContent)
	rightPanelScroll.SetMinSize(fyne.NewSize(350, 0))

	rightBg := widget.NewThemedRectangle(themes.ColorDescCardBg)
	rightBg.SetCornerRadius(12)
	rightBg.SetStroke(themes.ColorDescCardStroke, 1)
	rightPanel := container.NewStack(rightBg, container.NewPadded(rightPanelScroll))

	split := container.NewHSplit(tabs, rightPanel)
	split.Offset = 0.65

	win.SetContent(split)
	win.Show()

	win.SetOnClosed(func() {
		app.blinkRulesEditorActive.Store(false)
		app.forgetWindow("sorting_rules")
	})
}
