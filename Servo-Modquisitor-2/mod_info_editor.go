// Servo-Modquisitor-2/mod_info_editor.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// infoEditorState — состояние открытого окна редактора.
type infoEditorState struct {
	ModName    string
	ModPath    string
	FilePath   string
	Data       map[string]interface{}
	OrigFormat string // "dependencies" | "order" | "" (файла не было)
	Dirty      bool
}

type fundingEntry struct {
	Platform string
	URL      string
}

type locEntry struct {
	Name string
	Desc string
}

// ─────────────────────────────────────────────────────────────────
// Публичная точка входа
// ─────────────────────────────────────────────────────────────────

// showInfoJSONEditor открывает редактор info.json для указанного мода.
//
// Если файла нет — открывается пустая форма; при сохранении файл
// создаётся. Если файла нет, но есть <mod>_localization.lua,
// поля name/description и таблица localization заполняются из неё.
func (app *App) showInfoJSONEditor(modName string) {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	if modsPath == "" {
		app.appendLog(app.msg("log_mods_path_not_set"))
		return
	}

	modPath := filepath.Join(modsPath, modName)
	infoPath := filepath.Join(modPath, "info.json")

	state := &infoEditorState{
		ModName:  modName,
		ModPath:  modPath,
		FilePath: infoPath,
		Data:     make(map[string]interface{}),
	}

	// 1. База данных → Data (name/description/author/url/version).
	if entry := checks.GetModDBEntry(modName); entry != nil {
		app.applyDBEntryToData(state.Data, entry)
	}

	// 2. Читаем info.json ДО применения глобальных правил — чтобы
	//    понять, какой формат зависимостей использовал автор
	//    ("dependencies" или "order"). От этого зависит, в какой
	//    форме писать правила.
	var fileData map[string]interface{}
	if raw, err := os.ReadFile(infoPath); err == nil {
		if jerr := json.Unmarshal(raw, &fileData); jerr == nil {
			if _, ok := fileData["dependencies"]; ok {
				state.OrigFormat = "dependencies"
			} else if _, ok := fileData["order"]; ok {
				state.OrigFormat = "order"
			}
		} else {
			app.appendLogToFile(fmt.Sprintf("info.json parse error (%s): %v", infoPath, jerr))
			fileData = nil
		}
	}

	// 3. Глобальные правила → Data в правильном формате.
	app.applyGlobalRulesToData(state.Data, modName, state.OrigFormat)

	// 4. Overlay файла поверх базы и глобальных правил.
	if fileData != nil {
		deepMergeInterfaceMap(state.Data, fileData)
	}

	// 5. Финальное определение формата (если файла не было —
	//    останется "dependencies" по умолчанию из шага 3).
	if state.OrigFormat == "" {
		if _, ok := state.Data["dependencies"]; ok {
			state.OrigFormat = "dependencies"
		} else if _, ok := state.Data["order"]; ok {
			state.OrigFormat = "order"
		}
	}

	// 6. Fallback из _localization.lua (только если name/description
	//    всё ещё пусты после базы и файла).
	if getStr(state.Data, "name") == "" || getStr(state.Data, "description") == "" {
		if locFile := findLocalizationFile(modPath, modName); locFile != "" {
			if raw, err := os.ReadFile(locFile); err == nil {
				parsed := parseLocalizationLua(raw)
				applyLocalizationToData(state.Data, parsed)
			}
		}
	}

	app.openInfoEditorWindow(state)
}

// applyDBEntryToData переносит поля из записи mod_database.json
// в карту info.json. Карты name/description используются как source
// of truth для localization (все языки, что есть в базе).
func (app *App) applyDBEntryToData(data map[string]interface{}, entry *checks.ModDBEntry) {
	if entry == nil {
		return
	}

	if en := checks.PickLocalized(entry.Name, "en"); en != "" {
		data["name"] = en
	}
	if en := checks.PickLocalized(entry.Description, "en"); en != "" {
		data["description"] = en
	}
	if entry.Author != "" {
		data["author"] = entry.Author
	}
	if entry.URL != "" {
		data["homepage"] = entry.URL
		if id := helpers.ExtractModIDFromURL(entry.URL); id != 0 {
			data["nexusId"] = fmt.Sprintf("%d", id)
		}
	}
	if entry.SourceCodeURL != "" {
		data["source"] = entry.SourceCodeURL
	}

	// Версия из кеша nexus_versions.json — как в главной таблице.
	if entry.URL != "" {
		if id := helpers.ExtractModIDFromURL(entry.URL); id != 0 {
			cacheKey := fmt.Sprintf("%d:%s", id, entry.Folder)
			if info, ok := app.getCachedVersion(cacheKey); ok && info.Version != "" {
				data["version"] = info.Version
			}
		}
	}

	// localization: name/description по языкам.
	langs := make(map[string]bool)
	for l := range entry.Name {
		langs[l] = true
	}
	for l := range entry.Description {
		langs[l] = true
	}
	if len(langs) == 0 {
		return
	}

	loc := make(map[string]interface{})
	for lang := range langs {
		e := make(map[string]interface{})
		if v, ok := entry.Name[lang]; ok && v != "" {
			e["name"] = v
		}
		if v, ok := entry.Description[lang]; ok && v != "" {
			e["description"] = v
		}
		if len(e) > 0 {
			loc[lang] = e
		}
	}
	if len(loc) > 0 {
		data["localization"] = loc
	}
}

// deepMergeInterfaceMap рекурсивно сливает src в dst, перезаписывая
// скалярные значения и объединяя вложенные карты. Пустые строки в src
// тоже перезаписывают — это способ автора «явно очистить» поле.
func deepMergeInterfaceMap(dst, src map[string]interface{}) {
	for k, v := range src {
		if srcMap, ok := v.(map[string]interface{}); ok {
			if dstMap, ok := dst[k].(map[string]interface{}); ok {
				deepMergeInterfaceMap(dstMap, srcMap)
				continue
			}
		}
		dst[k] = v
	}
}

// ─────────────────────────────────────────────────────────────────
// Окно редактора
// ─────────────────────────────────────────────────────────────────

func (app *App) openInfoEditorWindow(state *infoEditorState) {
	win := app.myApp.NewWindow(app.msg("info_editor_title") + ": " + state.ModName)
	win.Resize(fyne.NewSize(920, 720))
	win.CenterOnScreen()

	markDirty := func() {
		if !state.Dirty {
			state.Dirty = true
			win.SetTitle(app.msg("info_editor_title") + ": " + state.ModName + " *")
		}
	}

	// ─── Basic tab ────────────────────────────────────────────────
	nameEntry := widget.NewEntry()
	nameEntry.SetText(getStr(state.Data, "name"))
	nameEntry.OnChanged = func(s string) { state.Data["name"] = s; markDirty() }

	descEntry := widget.NewMultiLineEntry()
	descEntry.SetText(getStr(state.Data, "description"))
	descEntry.Wrapping = fyne.TextWrapWord
	descEntry.SetMinRowsVisible(3)
	descEntry.OnChanged = func(s string) { state.Data["description"] = s; markDirty() }

	versionEntry := widget.NewEntry()
	versionEntry.SetText(getStr(state.Data, "version"))
	versionEntry.OnChanged = func(s string) { state.Data["version"] = s; markDirty() }

	authorEntry := widget.NewEntry()
	authorEntry.SetText(getStr(state.Data, "author"))
	authorEntry.OnChanged = func(s string) { state.Data["author"] = s; markDirty() }

	homepageEntry := widget.NewEntry()
	homepageEntry.SetText(getStr(state.Data, "homepage"))
	homepageEntry.OnChanged = func(s string) { state.Data["homepage"] = s; markDirty() }

	sourceEntry := widget.NewEntry()
	sourceEntry.SetText(getStr(state.Data, "source"))
	sourceEntry.OnChanged = func(s string) { state.Data["source"] = s; markDirty() }

	nexusIDEntry := widget.NewEntry()
	nexusIDEntry.SetText(getStr(state.Data, "nexusId"))
	nexusIDEntry.OnChanged = func(s string) { state.Data["nexusId"] = s; markDirty() }

	basicTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(app.msg("info_editor_field_name")),
		nameEntry,
		widget.NewSeparator(),
		widget.NewLabel(app.msg("info_editor_field_description")),
		descEntry,
		widget.NewSeparator(),
		container.NewGridWithColumns(2,
			container.NewVBox(widget.NewLabel(app.msg("col_version")), versionEntry),
			container.NewVBox(widget.NewLabel(app.msg("col_author")), authorEntry),
		),
		widget.NewSeparator(),
		widget.NewLabel(app.msg("mod_url_label")),
		homepageEntry,
		widget.NewLabel(app.msg("source_code_url")),
		sourceEntry,
		widget.NewLabel(app.msg("info_editor_field_nexus_id")),
		nexusIDEntry,
	))

	// ─── Funding tab ──────────────────────────────────────────────
	var fundingEntries []fundingEntry
	if fm, ok := state.Data["funding"].(map[string]interface{}); ok {
		keys := make([]string, 0, len(fm))
		for k := range fm {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if v, ok := fm[k].(string); ok {
				fundingEntries = append(fundingEntries, fundingEntry{Platform: k, URL: v})
			}
		}
	}

	fundingContainer := container.NewVBox()
	var rebuildFunding func()
	rebuildFunding = func() {
		fundingContainer.Objects = nil
		for i := range fundingEntries {
			idx := i
			platEntry := widget.NewEntry()
			platEntry.SetText(fundingEntries[idx].Platform)
			platEntry.SetPlaceHolder(app.msg("info_editor_funding_platform_ph"))
			urlEntry := widget.NewEntry()
			urlEntry.SetText(fundingEntries[idx].URL)
			urlEntry.SetPlaceHolder(app.msg("info_editor_funding_url_ph"))

			platEntry.OnChanged = func(s string) {
				fundingEntries[idx].Platform = s
				markDirty()
			}
			urlEntry.OnChanged = func(s string) {
				fundingEntries[idx].URL = s
				markDirty()
			}

			delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
				fundingEntries = append(fundingEntries[:idx], fundingEntries[idx+1:]...)
				rebuildFunding()
				markDirty()
			})

			row := container.NewBorder(nil, nil, nil, delBtn,
				container.NewGridWithColumns(2, platEntry, urlEntry))
			fundingContainer.Add(row)
		}
		fundingContainer.Refresh()
	}
	rebuildFunding()

	addFundingBtn := widget.NewButtonWithIcon(
		app.msg("info_editor_add_funding"),
		theme.ContentAddIcon(),
		func() {
			fundingEntries = append(fundingEntries, fundingEntry{})
			rebuildFunding()
			markDirty()
		},
	)

	fundingTab := container.NewBorder(
		container.NewVBox(
			widget.NewLabel(app.msg("info_editor_funding_hint")),
			addFundingBtn,
			widget.NewSeparator(),
		),
		nil, nil, nil,
		container.NewVScroll(fundingContainer),
	)

	// ─── Localization tab ────────────────────────────────────────
	locEntries := make(map[string]locEntry)
	if lm, ok := state.Data["localization"].(map[string]interface{}); ok {
		for lang, v := range lm {
			if vm, ok := v.(map[string]interface{}); ok {
				e := locEntry{}
				if n, ok := vm["name"].(string); ok {
					e.Name = n
				}
				if d, ok := vm["description"].(string); ok {
					e.Desc = d
				}
				locEntries[lang] = e
			}
		}
	}

	// Нормализация китайских языков.
	// Игра и файлы локализации используют zh-cn / zh-tw.
	// Внутри SMQ (и в старых базах) может быть zh-hans / zh-hant.
	// Сводим всё к игровым кодам, чтобы дропдаун не рябил.
	if e, ok := locEntries["zh-hans"]; ok {
		if _, exists := locEntries["zh-cn"]; !exists {
			locEntries["zh-cn"] = e
		}
		delete(locEntries, "zh-hans")
	}
	if e, ok := locEntries["zh-hant"]; ok {
		if _, exists := locEntries["zh-tw"]; !exists {
			locEntries["zh-tw"] = e
		}
		delete(locEntries, "zh-hant")
	}

	knownLangs := []string{
		"en", "ru", "de", "es", "fr", "it", "ja", "ko", "pl",
		"pt-br", "zh-cn", "zh-tw",
	}
	// Добавляем языки, которые уже есть в файле, но не в списке.
	for lang := range locEntries {
		found := false
		for _, kl := range knownLangs {
			if kl == lang {
				found = true
				break
			}
		}
		if !found {
			knownLangs = append(knownLangs, lang)
		}
	}

	locLangSelect := widget.NewSelect(knownLangs, nil)
	locLangSelect.SetSelected("en")

	locNameEntry := widget.NewEntry()
	locDescEntry := widget.NewMultiLineEntry()
	locDescEntry.Wrapping = fyne.TextWrapWord
	locDescEntry.SetMinRowsVisible(3)

	loadLocLang := func(lang string) {
		if lang == "" {
			return
		}
		e := locEntries[lang]
		locNameEntry.SetText(e.Name)
		locDescEntry.SetText(e.Desc)
	}

	locLangSelect.OnChanged = loadLocLang

	locNameEntry.OnChanged = func(s string) {
		lang := locLangSelect.Selected
		if lang == "" {
			return
		}
		e := locEntries[lang]
		e.Name = s
		locEntries[lang] = e
		markDirty()
	}
	locDescEntry.OnChanged = func(s string) {
		lang := locLangSelect.Selected
		if lang == "" {
			return
		}
		e := locEntries[lang]
		e.Desc = s
		locEntries[lang] = e
		markDirty()
	}

	loadLocLang("en")

	localizationTab := container.NewVScroll(container.NewVBox(
		widget.NewLabel(app.msg("menu_language")),
		locLangSelect,
		widget.NewSeparator(),
		widget.NewLabel(app.msg("info_editor_loc_name_label")),
		locNameEntry,
		widget.NewLabel(app.msg("info_editor_loc_desc_label")),
		locDescEntry,
		widget.NewSeparator(),
		widget.NewLabel(app.msg("info_editor_loc_hint")),
	))

	// ─── Dependencies tab ────────────────────────────────────────
	depRoot := state.OrigFormat
	if depRoot == "" {
		depRoot = "dependencies"
	}
	var depKeys []string
	if depRoot == "order" {
		depKeys = []string{"require", "this_after", "this_before"}
	} else {
		depKeys = []string{"required", "self_after", "self_before"}
	}

	depFormatLbl := widget.NewLabel(fmt.Sprintf(
		app.msg("info_editor_dep_format"), depRoot, strings.Join(depKeys, ", "),
	))

	depTexts := make([]*widget.Entry, len(depKeys))
	for i, k := range depKeys {
		val := strings.Join(getStrSlice(state.Data, depRoot, k), "\n")
		e := widget.NewMultiLineEntry()
		e.SetText(val)
		e.SetMinRowsVisible(4)
		depTexts[i] = e
	}

	makeDepField := func(i int, key, label string) fyne.CanvasObject {
		lbl := widget.NewLabel(label)
		entry := depTexts[i]
		entry.OnChanged = func(s string) {
			setStrSlice(state.Data, depRoot, key, splitLines(s))
			markDirty()
		}
		return container.NewVBox(lbl, entry, widget.NewSeparator())
	}

	depLabels := []string{
		app.msg("info_editor_dep_required"),
		app.msg("info_editor_dep_after"),
		app.msg("info_editor_dep_before"),
	}
	if depLabels[0] == "" {
		depLabels = []string{"Required (one per line)", "Load after (one per line)", "Load before (one per line)"}
	}

	dependenciesTab := container.NewVScroll(container.NewVBox(
		depFormatLbl,
		widget.NewSeparator(),
		makeDepField(0, depKeys[0], depLabels[0]),
		makeDepField(1, depKeys[1], depLabels[1]),
		makeDepField(2, depKeys[2], depLabels[2]),
	))

	// ─── Собираем вкладки ─────────────────────────────────────────
	tabs := container.NewAppTabs(
		container.NewTabItem(app.msg("info_editor_tab_basic"), basicTab),
		container.NewTabItem(app.msg("info_editor_tab_funding"), fundingTab),
		container.NewTabItem(app.msg("info_editor_tab_localization"), localizationTab),
		container.NewTabItem(app.msg("info_editor_tab_dependencies"), dependenciesTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// ─── Toolbar ──────────────────────────────────────────────────
	importBtn := widget.NewButtonWithIcon(
		app.msg("info_editor_import_loc"),
		theme.DownloadIcon(),
		func() {
			locFile := findLocalizationFile(state.ModPath, state.ModName)
			if locFile == "" {
				app.showInfoDialog(app.msg("warning_title"),
					app.msg("info_editor_loc_not_found"))
				return
			}
			raw, err := os.ReadFile(locFile)
			if err != nil {
				app.showInfoDialog(app.msg("error_title"), err.Error())
				return
			}
			parsed := parseLocalizationLua(raw)

			// Заполняем только пустые поля.
			if m, ok := parsed["mod_name"]; ok {
				if v, ok := m["en"]; ok && v != "" && getStr(state.Data, "name") == "" {
					state.Data["name"] = v
					nameEntry.SetText(v)
				}
			}
			if m, ok := parsed["mod_description"]; ok {
				if v, ok := m["en"]; ok && v != "" && getStr(state.Data, "description") == "" {
					state.Data["description"] = v
					descEntry.SetText(v)
				}
			}

			// Заполняем per-language name/description.
			langs := make(map[string]bool)
			for _, key := range []string{"mod_name", "mod_description"} {
				if m, ok := parsed[key]; ok {
					for l := range m {
						langs[l] = true
					}
				}
			}
			for lang := range langs {
				e := locEntries[lang]
				if m, ok := parsed["mod_name"]; ok {
					if v, ok := m[lang]; ok && v != "" {
						e.Name = v
					}
				}
				if m, ok := parsed["mod_description"]; ok {
					if v, ok := m[lang]; ok && v != "" {
						e.Desc = v
					}
				}
				locEntries[lang] = e
			}

			loadLocLang(locLangSelect.Selected)
			markDirty()
			app.appendLog(fmt.Sprintf("info.json: imported localization from %s", filepath.Base(locFile)))
		},
	)

	saveBtn := widget.NewButtonWithIcon(
		app.msg("btn_save"),
		theme.DocumentSaveIcon(),
		func() {
			// Синхронизируем funding и localization обратно в Data.
			fm := make(map[string]interface{})
			for _, e := range fundingEntries {
				if e.Platform != "" && e.URL != "" {
					fm[e.Platform] = e.URL
				}
			}
			if len(fm) > 0 {
				state.Data["funding"] = fm
			} else {
				delete(state.Data, "funding")
			}

			lm := make(map[string]interface{})
			for lang, e := range locEntries {
				if e.Name == "" && e.Desc == "" {
					continue
				}
				entry := make(map[string]interface{})
				if e.Name != "" {
					entry["name"] = e.Name
				}
				if e.Desc != "" {
					entry["description"] = e.Desc
				}
				lm[lang] = entry
			}
			if len(lm) > 0 {
				state.Data["localization"] = lm
			} else {
				delete(state.Data, "localization")
			}

			// Удаляем пустые поля верхнего уровня, чтобы файл был
			// чистым. Кроме dependencies/order — их оставляем даже
			// пустыми: иначе следующий редактор не поймёт формат.
			keepEmpty := map[string]bool{
				"dependencies": true, "order": true,
			}
			for k, v := range state.Data {
				if keepEmpty[k] {
					continue
				}
				if s, ok := v.(string); ok && s == "" {
					delete(state.Data, k)
				}
			}

			if err := saveInfoJSON(state); err != nil {
				app.showInfoDialog(app.msg("error_title"),
					fmt.Sprintf("%s: %v", app.msg("info_editor_save_failed"), err))
				return
			}
			state.Dirty = false
			app.appendLog(fmt.Sprintf("info.json saved: %s", state.FilePath))
			win.Close()
		},
	)
	saveBtn.Importance = widget.HighImportance

	var confirmDiscard func()

	cancelBtn := widget.NewButton(app.msg("btn_cancel"), func() {
		confirmDiscard()
	})

	confirmDiscard = func() {
		if !state.Dirty {
			win.Close()
			return
		}
		dialog.ShowConfirm(
			app.msg("warning_title"),
			app.msg("info_editor_discard_changes"),
			func(ok bool) {
				if ok {
					win.Close()
				}
			},
			win,
		)
	}

	toolbar := container.NewHBox(
		importBtn,
		layout.NewSpacer(),
		saveBtn,
		cancelBtn,
	)

	content := container.NewBorder(
		nil,
		container.NewVBox(widget.NewSeparator(), toolbar),
		nil, nil,
		tabs,
	)
	win.SetContent(content)

	win.SetCloseIntercept(confirmDiscard)
	win.Show()
}

// ─────────────────────────────────────────────────────────────────
// Сохранение
// ─────────────────────────────────────────────────────────────────

// saveInfoJSON атомарно пишет state.Data в state.FilePath.
// tmp + rename — та же защита, что и в SaveModDatabase: если процесс
// убьют посреди записи, старый info.json останется целым.
func saveInfoJSON(state *infoEditorState) error {
	if err := os.MkdirAll(filepath.Dir(state.FilePath), 0755); err != nil {
		return err
	}

	// Вместо стандартного json.MarshalIndent, который сортирует ключи
	// по алфавиту, пишем вручную в нужном порядке.
	var buf bytes.Buffer
	buf.WriteString("{\n")

	// Желаемый порядок ключей
	keys := []string{
		"name",
		"description",
		"version",
		"author",
		"homepage",
		"source",
		"funding",
		"nexusId",
		"dependencies",
		"localization",
	}

	written := make(map[string]bool)
	first := true

	writeVal := func(k string, v interface{}) error {
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		buf.WriteString(fmt.Sprintf("\t%q: ", k))

		// json.MarshalIndent с префиксом "\t" сделает правильный отступ для вложенных объектов
		b, err := json.MarshalIndent(v, "\t", "\t")
		if err != nil {
			return err
		}
		buf.Write(b)
		return nil
	}

	// Пишем известные ключи в нужном порядке
	for _, k := range keys {
		if v, ok := state.Data[k]; ok {
			if err := writeVal(k, v); err != nil {
				return fmt.Errorf("marshal %s: %w", k, err)
			}
			written[k] = true
		}
	}

	// Остальные (неизвестные) ключи пишем в конце в алфавитном порядке
	var rest []string
	for k := range state.Data {
		if !written[k] {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	for _, k := range rest {
		if err := writeVal(k, state.Data[k]); err != nil {
			return fmt.Errorf("marshal %s: %w", k, err)
		}
	}

	buf.WriteString("\n}")

	tmp := state.FilePath + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, state.FilePath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Поиск _localization.lua
// ─────────────────────────────────────────────────────────────────

// findLocalizationFile ищет файл локализации мода.
// Порядок поиска: плоская структура → scripts/mods/<name> → рекурсивно.
func findLocalizationFile(modPath, modName string) string {
	candidates := []string{
		filepath.Join(modPath, modName+"_localization.lua"),
		filepath.Join(modPath, "scripts", "mods", modName, modName+"_localization.lua"),
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && !info.IsDir() {
			return c
		}
	}
	// Рекурсивный fallback: любой *_localization.lua.
	var found string
	_ = filepath.Walk(modPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), "_localization.lua") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// ─────────────────────────────────────────────────────────────────
// Парсер _localization.lua
// ─────────────────────────────────────────────────────────────────

// parseLocalizationLua извлекает плоскую карту "loc_key -> {lang -> text}".
// Поддерживает:
//   - плоские:        mod_name = { en = "X", ru = "Y" }
//   - inline:         alm_weapon_selector = { en = "Weapon" }
//   - блочные ключи:  ["zh-tw"] = "text"
//   - экранирование:  \" \\ \n \t \r
//
// Пропускает комментарии -- и не падает на неожиданных конструкциях.
func parseLocalizationLua(data []byte) map[string]map[string]string {
	result := make(map[string]map[string]string)
	currentKey := ""
	currentMap := make(map[string]string)

	flush := func() {
		if currentKey != "" && len(currentMap) > 0 {
			result[currentKey] = currentMap
		}
		currentKey = ""
		currentMap = make(map[string]string)
	}

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	// Увеличиваем буфер: локализация бывает длинной.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Убираем строчные комментарии.
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		// Отрезаем "return " в начале файла.
		line = strings.TrimSpace(strings.TrimPrefix(line, "return "))

		// Открывающая скобка корня.
		if line == "{" {
			continue
		}
		// Закрывающая скобка (закрывает текущий блок).
		if line == "}" || line == "}," {
			flush()
			continue
		}

		// Плоские записи верхнего уровня: "key = value,".
		line = strings.TrimSpace(strings.TrimSuffix(line, ","))

		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		key := normalizeLuaKey(line[:eq])
		rest := strings.TrimSpace(line[eq+1:])

		// key = { — начало блока.
		if rest == "{" {
			flush()
			currentKey = key
			continue
		}

		// key = { lang = "x", ... } — inline-блок.
		if strings.HasPrefix(rest, "{") && strings.HasSuffix(rest, "}") {
			inner := strings.TrimSpace(rest[1 : len(rest)-1])
			m := make(map[string]string)
			parseInlineLuaTable(inner, m)
			if len(m) > 0 {
				result[key] = m
			}
			continue
		}

		// lang = "text" — значение внутри блока.
		if strings.HasPrefix(rest, "\"") || strings.HasPrefix(rest, "'") {
			val, _ := readLuaString(rest)
			if currentKey != "" {
				currentMap[key] = val
			}
			continue
		}
	}
	flush()

	return result
}

func normalizeLuaKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.TrimSuffix(key, ",")
	// ["lang"] → lang
	if strings.HasPrefix(key, "[") && strings.HasSuffix(key, "]") {
		key = strings.TrimSpace(key[1 : len(key)-1])
	}
	// "lang" или 'lang' → lang
	if len(key) >= 2 {
		first, last := key[0], key[len(key)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			key = key[1 : len(key)-1]
		}
	}
	return key
}

// readLuaString читает строку в кавычках с экранированием.
// Возвращает (значение, позиция после закрывающей кавычки).
func readLuaString(s string) (string, int) {
	if s == "" {
		return "", 0
	}
	quote := s[0]
	if quote != '"' && quote != '\'' {
		return "", 0
	}
	i := 1
	var b strings.Builder
	for i < len(s) {
		c := s[i]
		if c == quote {
			return b.String(), i + 1
		}
		if c == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\'':
				b.WriteByte('\'')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(c)
		}
		i++
	}
	return b.String(), i
}

func parseInlineLuaTable(s string, out map[string]string) {
	i := 0
	for i < len(s) {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			return
		}
		keyStart := i
		for i < len(s) && s[i] != '=' {
			i++
		}
		if i >= len(s) {
			return
		}
		key := normalizeLuaKey(s[keyStart:i])
		i++ // skip '='
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i >= len(s) || (s[i] != '"' && s[i] != '\'') {
			return
		}
		val, next := readLuaString(s[i:])
		if next == 0 {
			return
		}
		i += next
		out[key] = val
	}
}

// applyLocalizationToData — fallback для модов, которых нет в базе
// и у которых нет info.json. Заполняет только пустые ячейки,
// не перетирая ничего, что уже пришло из базы.
func applyLocalizationToData(data map[string]interface{}, parsed map[string]map[string]string) {
	if m, ok := parsed["mod_name"]; ok {
		if en, ok := m["en"]; ok && en != "" && getStr(data, "name") == "" {
			data["name"] = en
		}
	}
	if m, ok := parsed["mod_description"]; ok {
		if en, ok := m["en"]; ok && en != "" && getStr(data, "description") == "" {
			data["description"] = en
		}
	}

	existing, _ := data["localization"].(map[string]interface{})
	if existing == nil {
		existing = make(map[string]interface{})
		data["localization"] = existing
	}

	langs := make(map[string]bool)
	for _, key := range []string{"mod_name", "mod_description"} {
		if m, ok := parsed[key]; ok {
			for l := range m {
				langs[l] = true
			}
		}
	}

	for lang := range langs {
		langMap, _ := existing[lang].(map[string]interface{})
		if langMap == nil {
			langMap = make(map[string]interface{})
			existing[lang] = langMap
		}
		if m, ok := parsed["mod_name"]; ok {
			if v, ok := m[lang]; ok && v != "" {
				if cur, _ := langMap["name"].(string); cur == "" {
					langMap["name"] = v
				}
			}
		}
		if m, ok := parsed["mod_description"]; ok {
			if v, ok := m[lang]; ok && v != "" {
				if cur, ok := langMap["description"].(string); ok && cur != "" {
					continue
				}
				langMap["description"] = v
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// Хелперы работы с map[string]interface{}
// ─────────────────────────────────────────────────────────────────

func getStr(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getStrSlice(m map[string]interface{}, parent, key string) []string {
	sub, ok := m[parent].(map[string]interface{})
	if !ok {
		return nil
	}
	arr, ok := sub[key].([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func setStrSlice(m map[string]interface{}, parent, key string, values []string) {
	sub, ok := m[parent].(map[string]interface{})
	if !ok {
		sub = make(map[string]interface{})
		m[parent] = sub
	}
	arr := make([]interface{}, len(values))
	for i, v := range values {
		arr[i] = v
	}
	sub[key] = arr
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// applyGlobalRulesToData заполняет data правилами из глобального
// mandatory_obsolete_incompatible_dependencies.json (dependencies +
// load_order), отфильтрованными по текущему моду.
//
// info.json поддерживает два формата зависимостей:
//   - "dependencies": {"required": [...], "self_after": [...], "self_before": [...]}
//   - "order":        {"require": [...], "this_after": [...], "this_before": [...]}
//
// origFormat выбирает формат; "" → по умолчанию "dependencies".
// Существующие значения в data[depRoot] сохраняются (складываются,
// без дубликатов). Это даёт «база + глобальные правила» как черновик,
// который автор потом уточняет.
func (app *App) applyGlobalRulesToData(data map[string]interface{}, modName, origFormat string) {
	depRoot := origFormat
	if depRoot == "" {
		depRoot = "dependencies"
	}

	var reqKey, afterKey, beforeKey string
	if depRoot == "order" {
		reqKey, afterKey, beforeKey = "require", "this_after", "this_before"
	} else {
		reqKey, afterKey, beforeKey = "required", "self_after", "self_before"
	}

	sub, ok := data[depRoot].(map[string]interface{})
	if !ok {
		sub = make(map[string]interface{})
		data[depRoot] = sub
	}

	getList := func(key string) []string {
		arr, _ := sub[key].([]interface{})
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	reqs := getList(reqKey)
	afters := getList(afterKey)
	befores := getList(beforeKey)

	reqSet := make(map[string]bool, len(reqs))
	for _, v := range reqs {
		reqSet[v] = true
	}
	afterSet := make(map[string]bool, len(afters))
	for _, v := range afters {
		afterSet[v] = true
	}
	beforeSet := make(map[string]bool, len(befores))
	for _, v := range befores {
		beforeSet[v] = true
	}

	for _, dep := range checks.GetDependenciesFlat() {
		if dep.Dependent == modName && !reqSet[dep.Required] {
			reqSet[dep.Required] = true
			reqs = append(reqs, dep.Required)
		}
	}

	for _, rule := range checks.GetLoadOrderRulesFlat() {
		if rule.Before == "*" {
			continue
		}
		if rule.After == modName && !afterSet[rule.Before] {
			afterSet[rule.Before] = true
			afters = append(afters, rule.Before)
		}
		if rule.Before == modName && !beforeSet[rule.After] {
			beforeSet[rule.After] = true
			befores = append(befores, rule.After)
		}
	}

	sub[reqKey] = strSliceToInterface(reqs)
	sub[afterKey] = strSliceToInterface(afters)
	sub[beforeKey] = strSliceToInterface(befores)
}

func strSliceToInterface(ss []string) []interface{} {
	out := make([]interface{}, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
