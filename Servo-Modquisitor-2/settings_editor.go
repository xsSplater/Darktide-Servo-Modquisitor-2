// Servo-Modquisitor-2/settings_editor.go
package main

import (
	"Servo-Modquisitor/themes"
	"bufio"
	"fmt"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
)

// settingsEditorBgOpacity — прозрачность фонового изображения (mechanicus)
// под списком настроек. 0 — полностью прозрачное, 1 — полностью плотное.
// Отдельная константа, чтобы не менять TableBackgroundOpacity основной
// таблицы модов.
const settingsEditorBgOpacity = 0.97

// ─────────────────────────────────────────────────────────────────────
// Вспомогательные виджеты
// ─────────────────────────────────────────────────────────────────────

type expandableEntry struct {
	widget.Entry
}

func (e *expandableEntry) MinSize() fyne.Size {
	return fyne.NewSize(300, e.Entry.MinSize().Height)
}

type expandableSelect struct {
	widget.Select
}

func (e *expandableSelect) MinSize() fyne.Size {
	return fyne.NewSize(300, e.Select.MinSize().Height)
}

// ─────────────────────────────────────────────────────────────────────
// Модель данных
// ─────────────────────────────────────────────────────────────────────

type SettingsNode struct {
	Key      string
	Value    interface{}
	Children map[string]*SettingsNode
	IsArray  bool
	Parent   *SettingsNode
	Modified bool
}

// SettingsRow — строка правого списка.
type SettingsRow struct {
	Key      string
	Value    interface{}
	Desc     string
	Node     *SettingsNode // узел в WorkingRoot
	OrigNode *SettingsNode // соответствующий узел в OriginalRoot (для кнопки ⟲)
}

type SettingsEditorState struct {
	OriginalRoot *SettingsNode
	WorkingRoot  *SettingsNode
	FileModified bool
	Window       fyne.Window
}

// ─────────────────────────────────────────────────────────────────────
// Описания ключей
// ─────────────────────────────────────────────────────────────────────

// getSettingDescription возвращает локализованное описание настройки по её
// ключу. Перевод хранится в messages.json с префиксом "usettings_desc_".
// Если перевода нет — возвращает пустую строку (столбец «Описание» будет
// пустым, но интерфейс не сломается).
func (app *App) getSettingDescription(key string) string {
	return app.msg("usettings_desc_" + key)
}

const (
	widgetTypeEntry    = "entry"
	widgetTypeBool     = "bool"
	widgetTypeLang     = "lang"
	widgetTypeScreen   = "screen"
	widgetTypeQuality  = "quality"
	widgetTypeGraphics = "graphics"
)

var boolKeys = map[string]bool{
	"borderless_fullscreen":        true,
	"fullscreen":                   true,
	"vsync":                        true,
	"launcher_verification_passed": true,
}

var langKeys = map[string]bool{
	"language_id": true,
}

var screenModeKeys = map[string]bool{
	"screen_mode": true,
}

var qualityKeys = map[string]bool{
	"sun_shadow_map_filter_quality":          true,
	"local_lights_shadow_map_filter_quality": true,
}

var graphicsQualityKeys = map[string]bool{
	"graphics_quality": true,
}

func getWidgetType(key string) string {
	if boolKeys[key] {
		return widgetTypeBool
	}
	if langKeys[key] {
		return widgetTypeLang
	}
	if screenModeKeys[key] {
		return widgetTypeScreen
	}
	if graphicsQualityKeys[key] {
		return widgetTypeGraphics
	}
	if qualityKeys[key] {
		return widgetTypeQuality
	}
	return widgetTypeEntry
}

func getSelectOptions(widgetType string) []string {
	switch widgetType {
	case widgetTypeBool:
		return []string{"true", "false"}
	case widgetTypeLang:
		return []string{"de", "en", "es", "fr", "it", "ja", "ko", "pl", "pt-br", "ru", "zh-cn", "zh-tw"}
	case widgetTypeScreen:
		return []string{"window", "fullscreen"}
	case widgetTypeQuality:
		return []string{"low", "medium", "high"}
	case widgetTypeGraphics:
		return []string{"custom", "low", "medium", "high"}
	default:
		return []string{}
	}
}

// typeIcon возвращает короткую метку типа узла и ключ цвета темы.
func typeIcon(node *SettingsNode) (string, string) {
	if node == nil {
		return "", themes.ColorStatusInactive
	}
	if node.IsArray {
		return "[]", themes.ColorStatusManual
	}
	if node.Children != nil {
		return "{}", themes.ColorStatusObsolete
	}
	switch node.Value.(type) {
	case bool:
		return "◐", themes.ColorStatusActive
	case int:
		return "#", themes.ColorStatusSystem
	case float64:
		return "≈", themes.ColorStatusNexus
	default:
		return "S", themes.ColorStatusInactive
	}
}

// ─────────────────────────────────────────────────────────────────────
// Парсер
// ─────────────────────────────────────────────────────────────────────

func parseSettingsFile(path string) (*SettingsNode, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	root := &SettingsNode{Key: "root", Children: make(map[string]*SettingsNode)}

	type frame struct {
		node    *SettingsNode
		arr     []interface{}
		isArray bool
	}
	stack := []frame{{node: root, isArray: false}}

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		top := &stack[len(stack)-1]

		if top.isArray {
			if strings.HasPrefix(trimmed, "]") {
				closed := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if closed.node != nil {
					closed.node.Value = closed.arr
				}
				if len(stack) > 0 && stack[len(stack)-1].isArray {
					stack[len(stack)-1].arr = append(stack[len(stack)-1].arr, closed.arr)
				}
				continue
			}
			if strings.HasPrefix(trimmed, "[") {
				rest := strings.TrimPrefix(trimmed, "[")
				if idx := strings.Index(rest, "]"); idx >= 0 {
					inner := strings.TrimSpace(rest[:idx])
					nested := []interface{}{}
					if inner != "" {
						nested = parseArrayValues(inner)
					}
					stack[len(stack)-1].arr = append(stack[len(stack)-1].arr, nested)
					continue
				}
				stack = append(stack, frame{isArray: true, arr: []interface{}{}})
				continue
			}
			trimmed = strings.TrimSuffix(trimmed, ",")
			vals := parseArrayValues(trimmed)
			stack[len(stack)-1].arr = append(stack[len(stack)-1].arr, vals...)
			continue
		}

		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.Trim(strings.TrimSpace(parts[0]), `"`)
			valuePart := strings.TrimSpace(parts[1])

			if strings.HasPrefix(valuePart, "{") {
				newNode := &SettingsNode{
					Key:      key,
					Children: make(map[string]*SettingsNode),
				}
				parent := stack[len(stack)-1].node
				parent.Children[key] = newNode
				newNode.Parent = parent
				if strings.Contains(valuePart, "}") {
					continue
				}
				stack = append(stack, frame{node: newNode, isArray: false})
				continue
			}

			if strings.HasPrefix(valuePart, "[") {
				arrNode := &SettingsNode{Key: key, IsArray: true}
				parent := stack[len(stack)-1].node
				parent.Children[key] = arrNode
				arrNode.Parent = parent
				if idx := strings.Index(valuePart, "]"); idx > 0 {
					inner := strings.TrimSpace(valuePart[1:idx])
					arr := []interface{}{}
					if inner != "" {
						arr = parseArrayValues(inner)
					}
					arrNode.Value = arr
					continue
				}
				stack = append(stack, frame{node: arrNode, isArray: true, arr: []interface{}{}})
				continue
			}

			val := parseValue(valuePart)
			parent := stack[len(stack)-1].node
			leaf := &SettingsNode{Key: key, Value: val}
			parent.Children[key] = leaf
			leaf.Parent = parent
			continue
		}

		if strings.HasPrefix(trimmed, "}") && len(stack) > 1 {
			stack = stack[:len(stack)-1]
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return root, nil
}

func parseValue(s string) interface{} {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ",")
	if strings.HasPrefix(s, `"`) && strings.HasSuffix(s, `"`) {
		return strings.Trim(s, `"`)
	}
	if s == "true" {
		return true
	}
	if s == "false" {
		return false
	}
	if i, err := strconv.Atoi(s); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}

func parseArrayValues(s string) []interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return []interface{}{}
	}
	var result []interface{}
	var current strings.Builder
	inQuote := false
	for _, r := range s {
		if r == '"' {
			inQuote = !inQuote
			current.WriteRune(r)
		} else if (r == ' ' || r == '\t' || r == ',') && !inQuote {
			if current.Len() > 0 {
				token := strings.TrimSpace(current.String())
				if token != "" {
					result = append(result, parseValue(token))
				}
				current.Reset()
			}
		} else {
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		token := strings.TrimSpace(current.String())
		if token != "" {
			result = append(result, parseValue(token))
		}
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────
// Сериализатор
// ─────────────────────────────────────────────────────────────────────

func serializeSettings(node *SettingsNode) string {
	if node.Key != "root" {
		return serializeNode(node, "")
	}
	var sb strings.Builder
	for _, key := range getSortedKeys(node.Children) {
		sb.WriteString(serializeNode(node.Children[key], ""))
	}
	return sb.String()
}

func serializeNode(node *SettingsNode, indent string) string {
	var sb strings.Builder

	if node.IsArray {
		sb.WriteString(indent)
		sb.WriteString(quoteKey(node.Key))
		sb.WriteString(" = [\n")
		arr, _ := node.Value.([]interface{})
		for _, v := range arr {
			sb.WriteString(serializeArrayValue(v, indent+"\t"))
		}
		sb.WriteString(indent)
		sb.WriteString("]\n")
		return sb.String()
	}

	if node.Children != nil {
		sb.WriteString(indent)
		sb.WriteString(quoteKey(node.Key))
		sb.WriteString(" = {\n")
		for _, key := range getSortedKeys(node.Children) {
			sb.WriteString(serializeNode(node.Children[key], indent+"\t"))
		}
		sb.WriteString(indent)
		sb.WriteString("}\n")
		return sb.String()
	}

	if node.Value != nil {
		sb.WriteString(indent)
		sb.WriteString(quoteKey(node.Key))
		sb.WriteString(" = ")
		sb.WriteString(valueToString(node.Value))
		sb.WriteString("\n")
	}
	return sb.String()
}

func serializeArrayValue(v interface{}, indent string) string {
	var sb strings.Builder

	if arr, ok := v.([]interface{}); ok {
		sb.WriteString(indent)
		sb.WriteString("[\n")
		for _, elem := range arr {
			sb.WriteString(serializeArrayValue(elem, indent+"\t"))
		}
		sb.WriteString(indent)
		sb.WriteString("]\n")
		return sb.String()
	}

	sb.WriteString(indent)
	sb.WriteString(valueToString(v))
	sb.WriteString("\n")
	return sb.String()
}

func quoteKey(key string) string {
	if strings.ContainsAny(key, " {}[]=:.\"/") || strings.Contains(key, " ") {
		return fmt.Sprintf(`"%s"`, key)
	}
	return key
}

func valueToString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return fmt.Sprintf(`"%s"`, val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int:
		return strconv.Itoa(val)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

// ─────────────────────────────────────────────────────────────────────
// Глубокое копирование
// ─────────────────────────────────────────────────────────────────────

func deepCopy(node *SettingsNode) *SettingsNode {
	if node == nil {
		return nil
	}
	cp := &SettingsNode{
		Key:      node.Key,
		IsArray:  node.IsArray,
		Modified: node.Modified,
	}
	if node.Value != nil {
		switch v := node.Value.(type) {
		case []interface{}:
			cp.Value = deepCopyArrayValue(v)
		default:
			cp.Value = v
		}
	}
	if node.Children != nil {
		cp.Children = make(map[string]*SettingsNode, len(node.Children))
		for k, child := range node.Children {
			cpy := deepCopy(child)
			cp.Children[k] = cpy
			cpy.Parent = cp
		}
	}
	return cp
}

func deepCopyArrayValue(arr []interface{}) []interface{} {
	out := make([]interface{}, len(arr))
	for i, v := range arr {
		if inner, ok := v.([]interface{}); ok {
			out[i] = deepCopyArrayValue(inner)
		} else {
			out[i] = v
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────
// Работа с путями
// ─────────────────────────────────────────────────────────────────────

const uidSep = ">>"

func getNodeByPath(root *SettingsNode, path string) *SettingsNode {
	if path == "" {
		return root
	}
	current := root
	for _, part := range strings.Split(path, "/") {
		if current == nil || current.Children == nil {
			return nil
		}
		child, ok := current.Children[part]
		if !ok {
			return nil
		}
		current = child
	}
	return current
}

func getNodeByUID(root *SettingsNode, uid string) *SettingsNode {
	switch {
	case uid == "":
		return root
	case uid == "sys":
		return root
	case uid == "mods":
		return getNodeByPath(root, "mods_settings")
	}
	parts := strings.Split(uid, uidSep)
	var node *SettingsNode
	switch parts[0] {
	case "sys":
		node = root
	case "mods":
		node = getNodeByPath(root, "mods_settings")
	default:
		return nil
	}
	for _, p := range parts[1:] {
		if node == nil || node.Children == nil {
			return nil
		}
		node = node.Children[p]
	}
	return node
}

func getSortedKeys(m map[string]*SettingsNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (app *App) uidToPretty(uid string) string {
	switch uid {
	case "":
		return app.msg("usettings_path_root")
	case "sys":
		return app.msg("usettings_path_game")
	case "mods":
		return app.msg("usettings_path_mods")
	}
	parts := strings.Split(uid, uidSep)
	prefix := ""
	switch parts[0] {
	case "sys":
		prefix = app.msg("usettings_path_game")
	case "mods":
		prefix = app.msg("usettings_path_mods")
	}
	if len(parts) > 1 {
		return prefix + strings.Join(parts[1:], " / ")
	}
	return prefix
}

// ─────────────────────────────────────────────────────────────────────
// Форматирование
// ─────────────────────────────────────────────────────────────────────

func formatValue(node *SettingsNode) string {
	if node.IsArray {
		arr, _ := node.Value.([]interface{})
		return formatArrayValue(arr)
	}
	if node.Children != nil {
		return "(block)"
	}
	if node.Value == nil {
		return ""
	}
	return valueToString(node.Value)
}

func formatArrayValue(arr []interface{}) string {
	strs := make([]string, len(arr))
	for i, v := range arr {
		if inner, ok := v.([]interface{}); ok {
			strs[i] = "[" + formatArrayValue(inner) + "]"
		} else {
			strs[i] = valueToString(v)
		}
	}
	return "[" + strings.Join(strs, " ") + "]"
}

func compareValues(a, b interface{}) bool {
	return valueToString(a) == valueToString(b)
}

func clearModifiedFlags(node *SettingsNode) {
	if node == nil {
		return
	}
	node.Modified = false
	for _, child := range node.Children {
		clearModifiedFlags(child)
	}
}

func parseSettingsData(data string) (*SettingsNode, error) {
	tmpFile, err := os.CreateTemp("", "import_settings_*.config")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())
	if _, err := tmpFile.WriteString(data); err != nil {
		return nil, err
	}
	tmpFile.Close()
	return parseSettingsFile(tmpFile.Name())
}

// ─────────────────────────────────────────────────────────────────────
// Основное окно редактора
// ─────────────────────────────────────────────────────────────────────

func (app *App) showGameSettingsEditor() {
	settingsPath := app.getUserSettingsPath()
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		app.showInfoDialog(app.msg("error_title"), app.msg("settings_file_not_found"))
		return
	}

	win := app.myApp.NewWindow(app.msg("settings_editor_title"))
	win.Resize(fyne.NewSize(1320, 820))

	root, err := parseSettingsFile(settingsPath)
	if err != nil {
		app.showInfoDialog(app.msg("error_title"), fmt.Sprintf(app.msg("settings_parse_error"), err))
		return
	}

	state := &SettingsEditorState{
		OriginalRoot: deepCopy(root),
		WorkingRoot:  root,
		FileModified: false,
		Window:       win,
	}

	hiddenSystemKeys := map[string]bool{
		"detected_user_settings": true,
		"version":                true,
	}

	// ─── Состояние правой панели ────────────────────────────────
	var (
		selectedUID string
		searchText  string

		allRows      []SettingsRow
		rows         []SettingsRow
		settingsList *widget.Table
		tree         *widget.Tree
		pathLabel    *widget.Label
		countLabel   *widget.Label
		listBorder   *canvas.Rectangle
	)

	winTitleBase := app.msg("settings_editor_title")

	setTitleModified := func(modified bool) {
		if modified {
			win.SetTitle(winTitleBase + " *")
		} else {
			win.SetTitle(winTitleBase)
		}
	}

	// ─── Функции UID ─────────────────────────────────────────────
	treeChildren := func(uid string) []string {
		var node *SettingsNode
		switch uid {
		case "":
			return []string{"sys", "mods"}
		case "sys":
			node = state.WorkingRoot
		case "mods":
			node = getNodeByPath(state.WorkingRoot, "mods_settings")
		default:
			node = getNodeByUID(state.WorkingRoot, uid)
		}
		if node == nil || node.Children == nil {
			return []string{}
		}
		var children []string
		for k := range node.Children {
			if uid == "sys" {
				if k == "mods_settings" || hiddenSystemKeys[k] {
					continue
				}
			}
			children = append(children, uid+uidSep+k)
		}
		sort.Strings(children)
		return children
	}

	treeIsBranch := func(uid string) bool {
		if uid == "" || uid == "sys" || uid == "mods" {
			return true
		}
		node := getNodeByUID(state.WorkingRoot, uid)
		if node == nil || node.Children == nil {
			return false
		}
		return len(node.Children) > 0
	}

	treeLabel := func(uid string) string {
		switch uid {
		case "":
			return app.msg("menu_settings")
		case "sys":
			return app.msg("menu_settings_game")
		case "mods":
			return app.msg("menu_settings_mods")
		}
		parts := strings.Split(uid, uidSep)
		return parts[len(parts)-1]
	}

	// ─── Построение строк ────────────────────────────────────────
	rebuildRows := func() {
		allRows = nil

		var source, origSource *SettingsNode
		switch selectedUID {
		case "", "sys":
			source = state.WorkingRoot
			origSource = state.OriginalRoot
		case "mods":
			source = getNodeByPath(state.WorkingRoot, "mods_settings")
			origSource = getNodeByPath(state.OriginalRoot, "mods_settings")
		default:
			source = getNodeByUID(state.WorkingRoot, selectedUID)
			origSource = getNodeByUID(state.OriginalRoot, selectedUID)
		}

		if source != nil {
			if source.Children != nil {
				// Блок: строки — его дети.
				for _, k := range getSortedKeys(source.Children) {
					if selectedUID == "sys" && (k == "mods_settings" || hiddenSystemKeys[k]) {
						continue
					}
					child := source.Children[k]
					var origChild *SettingsNode
					if origSource != nil && origSource.Children != nil {
						origChild = origSource.Children[k]
					}
					allRows = append(allRows, SettingsRow{
						Key:      k,
						Value:    child.Value,
						Desc:     app.getSettingDescription(k),
						Node:     child,
						OrigNode: origChild,
					})
				}
			} else {
				// Лист: одна строка — сам узел.
				// Так клик по leaf-узлу в дереве даёт редактируемое
				// значение в правой панели.
				allRows = append(allRows, SettingsRow{
					Key:      source.Key,
					Value:    source.Value,
					Desc:     app.getSettingDescription(source.Key),
					Node:     source,
					OrigNode: origSource,
				})
			}
		}

		// Фильтр по поиску.
		searchLower := strings.ToLower(searchText)
		rows = rows[:0]
		for _, r := range allRows {
			if searchLower != "" && !strings.Contains(strings.ToLower(r.Key), searchLower) {
				continue
			}
			rows = append(rows, r)
		}

		if pathLabel != nil {
			pathLabel.SetText(app.uidToPretty(selectedUID))
		}
		if countLabel != nil {
			if searchText != "" {
				countLabel.SetText(fmt.Sprintf("%d / %d", len(rows), len(allRows)))
				countLabel.TextStyle = fyne.TextStyle{Bold: true}
			} else {
				countLabel.SetText(fmt.Sprintf("%d", len(rows)))
				countLabel.TextStyle = fyne.TextStyle{}
			}
			countLabel.Refresh()
		}
		if settingsList != nil {
			// При смене контекста (двойной клик в дереве или списке)
			// обязательно сбрасываем скролл. Иначе таблица сохраняет
			// старую позицию, а после уменьшения количества строк
			// показывает пустоту и мусорные ячейки из кэша.
			settingsList.ScrollToTop()
			settingsList.ScrollToLeading()
			settingsList.Refresh()
		}
	}

	markModified := func() {
		state.FileModified = true
		setTitleModified(true)
		if listBorder != nil {
			listBorder.Show()
			listBorder.Refresh()
		}
	}

	clearModified := func() {
		state.FileModified = false
		setTitleModified(false)
		if listBorder != nil {
			listBorder.Hide()
			listBorder.Refresh()
		}
	}

	// ─── Виджет значения ─────────────────────────────────────────
	createValueWidget := func(row SettingsRow, onChanged func(newVal interface{})) fyne.CanvasObject {
		if row.Node != nil && row.Node.Children != nil && !row.Node.IsArray {
			lbl := widget.NewLabel(app.msg("usettings_block_label"))
			lbl.TextStyle = fyne.TextStyle{Italic: true}
			return lbl
		}

		switch v := row.Value.(type) {
		case bool:
			sel := &expandableSelect{}
			sel.ExtendBaseWidget(sel)
			sel.Options = []string{"true", "false"}
			sel.SetSelected(fmt.Sprintf("%v", v))
			sel.OnChanged = func(s string) {
				newVal := parseValue(s)
				if !compareValues(row.Value, newVal) {
					onChanged(newVal)
				}
			}
			return sel

		case string:
			wt := getWidgetType(row.Key)
			if wt != widgetTypeEntry {
				sel := &expandableSelect{}
				sel.ExtendBaseWidget(sel)
				sel.Options = getSelectOptions(wt)
				sel.SetSelected(v)
				sel.OnChanged = func(s string) {
					newVal := parseValue(s)
					if !compareValues(row.Value, newVal) {
						onChanged(newVal)
					}
				}
				return sel
			}
			entry := &expandableEntry{}
			entry.ExtendBaseWidget(entry)
			entry.SetText(formatValue(row.Node))
			entry.OnChanged = func(newText string) {
				newVal := parseValue(newText)
				if !compareValues(row.Value, newVal) {
					onChanged(newVal)
				}
			}
			return entry

		default:
			entry := &expandableEntry{}
			entry.ExtendBaseWidget(entry)
			entry.SetText(formatValue(row.Node))
			entry.OnChanged = func(newText string) {
				newVal := parseValue(newText)
				if !compareValues(row.Value, newVal) {
					onChanged(newVal)
				}
			}
			return entry
		}
	}

	// ─── Дерево ──────────────────────────────────────────────────
	tree = widget.NewTree(
		treeChildren,
		treeIsBranch,
		func(bool) fyne.CanvasObject { return widget.NewLabel("") },
		func(uid string, branch bool, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(treeLabel(uid))
		},
	)
	tree.OnSelected = func(uid string) {
		selectedUID = uid
		rebuildRows()
	}

	// navigateToRowByIndex обрабатывает двойной клик по строке списка.
	//
	// Логика:
	//   1. Если текущий узел — лист (или массив, который в дереве
	//      тоже выглядит как лист), двойной клик ведёт «назад» к
	//      родительскому узлу.
	//   2. Иначе — «внутрь» к узлу, соответствующему строке.
	navigateToRowByIndex := func(id int) {
		if id < 0 || id >= len(rows) {
			return
		}
		row := rows[id]
		if row.Node == nil {
			return
		}

		// Шаг 1: назад к родителю.
		// selectedUID == "" / "sys" / "mods" — это псевдо-корни,
		// для них «назад» некуда, всегда идём «внутрь».
		if selectedUID != "" && selectedUID != "sys" && selectedUID != "mods" {
			cur := getNodeByUID(state.WorkingRoot, selectedUID)
			if cur != nil && cur.Children == nil {
				parts := strings.Split(selectedUID, uidSep)
				if len(parts) > 1 {
					parentUID := strings.Join(parts[:len(parts)-1], uidSep)
					tree.Select(parentUID)
				}
				return
			}
		}

		// Шаг 2: внутрь.
		baseUID := selectedUID
		if baseUID == "" {
			baseUID = "sys"
		}
		newUID := baseUID + uidSep + row.Key

		if getNodeByUID(state.WorkingRoot, newUID) == nil {
			return
		}

		parts := strings.Split(newUID, uidSep)
		tree.OpenBranch("")
		tree.OpenBranch("sys")
		tree.OpenBranch("mods")
		for i := 1; i < len(parts); i++ {
			tree.OpenBranch(strings.Join(parts[:i], uidSep))
		}

		if newUID == selectedUID {
			rebuildRows()
			return
		}
		tree.Select(newUID)
	}

	// ─── Заголовок списка ────────────────────────────────────────
	createFixedWidthLabel := func(width float32) *fyne.Container {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(width, 1))
		label := widget.NewLabel("")
		label.Wrapping = fyne.TextWrapWord
		return container.NewStack(spacer, label)
	}

	headerType := createFixedWidthLabel(30)
	headerType.Objects[1].(*widget.Label).SetText(app.msg("usettings_type_header"))
	headerType.Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerType.Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerKey := createFixedWidthLabel(220)
	headerKey.Objects[1].(*widget.Label).SetText(app.msg("usettingsconf_key"))
	headerKey.Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerKey.Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerVal := createFixedWidthLabel(220)
	headerVal.Objects[1].(*widget.Label).SetText(app.msg("usettingsconf_value"))
	headerVal.Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerVal.Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerDesc := createFixedWidthLabel(400)
	headerDesc.Objects[1].(*widget.Label).SetText(app.msg("usettingsconf_description"))
	headerDesc.Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerDesc.Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerReset := createFixedWidthLabel(36)
	headerReset.Objects[1].(*widget.Label).SetText("⟲")
	headerReset.Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerReset.Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	headerBg := canvas.NewRectangle(th.Color(themes.ColorTableHeaderBg, variant))
	headerContainer := container.NewStack(
		headerBg,
		container.NewHBox(headerType, headerKey, headerVal, headerDesc, headerReset),
	)

	// ─── Список ──────────────────────────────────────────────────
	settingsList = widget.NewTable(
		// Length: количество строк = len(rows), колонок = 5 (тип, ключ,
		// значение, описание, сброс).
		func() (int, int) { return len(rows), 5 },
		// CreateCell: универсальный контейнер (bg + content). Что
		// положить в content — решает UpdateCell по id.Col.
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			// Fyne в Table использует MinSize() этого шаблона как высоту
			// строки по умолчанию. Пустой прозрачный прямоугольник даёт
			// MinSize = (0, 0), и все строки схлопываются в одну точку.
			// Поэтому задаём минимальную высоту явно.
			bg.SetMinSize(fyne.NewSize(1, 45))
			content := container.NewStack()
			return container.NewStack(bg, content)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			if id.Row >= len(rows) {
				return
			}
			row := rows[id.Row]

			outer := cell.(*fyne.Container)
			bg := outer.Objects[0].(*canvas.Rectangle)
			content := outer.Objects[1].(*fyne.Container)
			content.Objects = nil

			rowTh := fyne.CurrentApp().Settings().Theme()
			rowVariant := fyne.CurrentApp().Settings().ThemeVariant()

			// Фон строки.
			switch {
			case row.Node != nil && row.Node.Modified:
				bg.FillColor = rowTh.Color(themes.ColorTableRowConflict, rowVariant)
			case id.Row%2 == 0:
				bg.FillColor = rowTh.Color(themes.ColorTableRowEven, rowVariant)
			default:
				bg.FillColor = rowTh.Color(themes.ColorTableRowOdd, rowVariant)
			}
			bg.Refresh()

			switch id.Col {
			case 0: // тип
				icon, iconColorKey := typeIcon(row.Node)
				text := canvas.NewText(icon, color.White)
				text.TextSize = 14
				text.TextStyle = fyne.TextStyle{Bold: true}
				text.Alignment = fyne.TextAlignCenter
				if iconColorKey != "" {
					text.Color = rowTh.Color(fyne.ThemeColorName(iconColorKey), rowVariant)
				}
				content.Add(container.NewCenter(text))

			case 1: // ключ
				displayKey := row.Key
				style := fyne.TextStyle{}
				if row.Node != nil && row.Node.Modified {
					displayKey = "● " + displayKey
					style = fyne.TextStyle{Bold: true}
				}
				label := widget.NewLabel(displayKey)
				label.TextStyle = style
				label.Wrapping = fyne.TextWrapOff
				content.Add(container.NewPadded(label))

			case 2: // значение
				valueWidget := createValueWidget(row, func(newVal interface{}) {
					row.Node.Value = newVal
					row.Node.Modified = true
					for i := range allRows {
						if allRows[i].Node == row.Node {
							allRows[i].Value = newVal
						}
					}
					for i := range rows {
						if rows[i].Node == row.Node {
							rows[i].Value = newVal
						}
					}
					markModified()
					settingsList.Refresh()
				})
				content.Add(container.NewPadded(valueWidget))

			case 3: // описание
				label := widget.NewLabel(row.Desc)
				label.Wrapping = fyne.TextWrapWord
				content.Add(container.NewPadded(label))

			case 4: // кнопка сброса
				if row.OrigNode != nil && row.Node != nil {
					canReset := false
					if row.OrigNode.Value == nil && row.Node.Value != nil {
						canReset = true
					} else if row.OrigNode.Value != nil && row.Node.Value == nil {
						canReset = true
					} else if row.OrigNode.Value != nil && row.Node.Value != nil {
						canReset = !compareValues(row.Node.Value, row.OrigNode.Value)
					}

					if canReset {
						origVal := row.OrigNode.Value
						btn := widget.NewButton("⟲", func() {
							row.Node.Value = origVal
							row.Node.Modified = false
							for i := range rows {
								if rows[i].Node == row.Node {
									rows[i].Value = origVal
								}
							}
							for i := range allRows {
								if allRows[i].Node == row.Node {
									allRows[i].Value = origVal
								}
							}
							settingsList.Refresh()
						})
						content.Add(container.NewCenter(btn))
					}
				}
			}
			content.Refresh()
		},
	)

	settingsList.SetColumnWidth(0, 30)  // тип
	settingsList.SetColumnWidth(1, 220) // ключ
	settingsList.SetColumnWidth(2, 220) // значение
	settingsList.SetColumnWidth(3, 400) // описание
	settingsList.SetColumnWidth(4, 40)  // кнопка сброса
	settingsList.SetRowHeight(-1, 32)

	// Двойной клик по строке — переход к соответствующему узлу в дереве.
	// Используем встроенный OnDoubleTapped форка.
	settingsList.OnDoubleTapped = func(id widget.TableCellID) {
		navigateToRowByIndex(id.Row)
	}

	// ─── Поиск ───────────────────────────────────────────────────
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder(app.msg("usettings_search_placeholder"))
	searchEntry.OnChanged = func(s string) {
		searchText = s
		rebuildRows()
	}
	clearSearchBtn := widget.NewButton("✕", func() {
		searchEntry.SetText("")
	})
	searchBox := container.NewBorder(
		nil, nil,
		widget.NewLabel("🔍"), clearSearchBtn,
		searchEntry,
	)

	// ─── Path-bar и счётчик ──────────────────────────────────────
	pathLabel = widget.NewLabel("📂 /")
	pathLabel.TextStyle = fyne.TextStyle{Bold: true}

	countLabel = widget.NewLabel("0")
	countLabel.Alignment = fyne.TextAlignTrailing

	pathBar := container.NewBorder(
		nil, nil,
		pathLabel, countLabel,
		nil,
	)

	// ─── Фон mechanicus под списком ──────────────────────────────
	var listStack *fyne.Container
	if mechData, err := embeddedFiles.ReadFile(TableBackgroundImage); err == nil && len(mechData) > 0 {
		mechImg := canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechImg.FillMode = canvas.ImageFillContain
		mechImg.Translucency = settingsEditorBgOpacity

		listBorder = canvas.NewRectangle(color.Transparent)
		listBorder.StrokeWidth = 2
		listBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
		listBorder.FillColor = color.Transparent
		listBorder.Hide()

		listStack = container.NewStack(mechImg, settingsList, listBorder)
	} else {
		listBorder = canvas.NewRectangle(color.Transparent)
		listBorder.StrokeWidth = 2
		listBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
		listBorder.FillColor = color.Transparent
		listBorder.Hide()

		listStack = container.NewStack(settingsList, listBorder)
	}

	tableContainer := container.NewBorder(headerContainer, nil, nil, nil, listStack)

	filterBar := container.NewVBox(
		searchBox,
		pathBar,
		widget.NewSeparator(),
	)

	rightInner := container.NewBorder(filterBar, nil, nil, nil, tableContainer)

	// ─── Кнопки ──────────────────────────────────────────────────
	saveBtn := widget.NewButton(app.msg("btn_save"), func() {
		serialized := serializeSettings(state.WorkingRoot)
		if strings.TrimSpace(serialized) == "" {
			app.showInfoDialog(app.msg("error_title"), app.msg("usettings_empty_settings_warning"))
			return
		}
		if err := os.WriteFile(settingsPath, []byte(serialized), 0644); err != nil {
			app.showInfoDialog(app.msg("error_title"), fmt.Sprintf(app.msg("settings_save_error"), err))
			return
		}
		state.OriginalRoot = deepCopy(state.WorkingRoot)
		clearModifiedFlags(state.WorkingRoot)
		clearModified()
		app.appendLog(app.msg("settings_saved"))
		rebuildRows()
	})

	cancelBtn := widget.NewButton(app.msg("btn_cancel"), func() {
		state.WorkingRoot = deepCopy(state.OriginalRoot)
		clearModifiedFlags(state.WorkingRoot)
		clearModified()
		app.appendLog(app.msg("settings_changes_discarded"))
		tree.Refresh()
		rebuildRows()
		if selectedUID != "" {
			tree.Select(selectedUID)
		}
	})

	deleteModBtn := widget.NewButton(app.msg("usettings_delete_mod_btn"), func() {
		if selectedUID == "" || !strings.HasPrefix(selectedUID, "mods"+uidSep) {
			app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_mod_to_delete"))
			return
		}
		modName := strings.TrimPrefix(selectedUID, "mods"+uidSep)
		if strings.Contains(modName, uidSep) {
			app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_mod_root"))
			return
		}
		dialog.ShowConfirm(app.msg("usettings_confirm_title"), fmt.Sprintf(app.msg("usettings_confirm_delete_mod"), modName), func(ok bool) {
			if !ok {
				return
			}
			modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
			if modsNode != nil && modsNode.Children != nil {
				if _, exists := modsNode.Children[modName]; exists {
					delete(modsNode.Children, modName)
					markModified()
					selectedUID = ""
					tree.Refresh()
					rebuildRows()
					app.appendLog(fmt.Sprintf(app.msg("usettings_mod_deleted"), modName))
				} else {
					app.showInfoDialog(app.msg("usettings_error_title"), app.msg("usettings_mod_not_found"))
				}
			}
		}, win)
	})

	exportBtn := widget.NewButton(app.msg("usettings_export_mod_btn"), func() {
		if selectedUID == "" || !strings.HasPrefix(selectedUID, "mods"+uidSep) {
			app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_mod_to_export"))
			return
		}
		modName := strings.TrimPrefix(selectedUID, "mods"+uidSep)
		if strings.Contains(modName, uidSep) {
			app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_mod_root_short"))
			return
		}
		modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
		if modsNode == nil || modsNode.Children == nil {
			app.showInfoDialog(app.msg("usettings_error_title"), app.msg("usettings_no_mod_settings"))
			return
		}
		modNode, exists := modsNode.Children[modName]
		if !exists {
			app.showInfoDialog(app.msg("usettings_error_title"), app.msg("usettings_mod_not_found_short"))
			return
		}
		fd := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			if err != nil || uri == nil {
				return
			}
			defer uri.Close()
			serialized := serializeNode(modNode, "")
			if _, err := uri.Write([]byte(serialized)); err != nil {
				app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_save_failed"), err))
			} else {
				app.appendLog(fmt.Sprintf(app.msg("usettings_mod_exported"), modName))
			}
		}, win)
		fd.SetFileName(modName + ".modsettings")
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".modsettings"}))
		fd.Show()
	})

	importBtn := widget.NewButton(app.msg("usettings_import_mod_btn"), func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			if err != nil {
				app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_read_failed"), err))
				return
			}
			tmpRoot, err := parseSettingsData(string(data))
			if err != nil {
				app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_invalid_file"), err))
				return
			}
			if len(tmpRoot.Children) != 1 {
				app.showInfoDialog(app.msg("usettings_error_title"), app.msg("usettings_one_mod_required"))
				return
			}
			var modKey string
			var modNode *SettingsNode
			for k, v := range tmpRoot.Children {
				modKey = k
				modNode = v
				break
			}
			modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
			if modsNode == nil {
				modsNode = &SettingsNode{Key: "mods_settings", Children: make(map[string]*SettingsNode)}
				state.WorkingRoot.Children["mods_settings"] = modsNode
				modsNode.Parent = state.WorkingRoot
			}
			apply := func() {
				modsNode.Children[modKey] = modNode
				modNode.Parent = modsNode
				markModified()
				tree.Refresh()
				rebuildRows()
				app.appendLog(fmt.Sprintf(app.msg("usettings_mod_imported"), modKey))
			}
			if _, exists := modsNode.Children[modKey]; exists {
				dialog.ShowConfirm(app.msg("usettings_confirm_title"), fmt.Sprintf(app.msg("usettings_mod_exists_replace"), modKey), func(ok bool) {
					if ok {
						apply()
					}
				}, win)
			} else {
				apply()
			}
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".modsettings"}))
		fd.Show()
	})

	btnRow := container.NewHBox(saveBtn, cancelBtn, widget.NewSeparator(), deleteModBtn, exportBtn, importBtn)
	btnContainer := container.NewVBox(btnRow, widget.NewSeparator())

	rightPanel := container.NewBorder(btnContainer, nil, nil, nil, rightInner)
	leftPanel := container.NewBorder(nil, nil, nil, nil, tree)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.28

	win.SetContent(split)
	win.Show()

	tree.OpenBranch("")
	tree.OpenBranch("sys")
	tree.OpenBranch("mods")
	rebuildRows()
}

// ─────────────────────────────────────────────────────────────────────
// Восстановление из бэкапа
// ─────────────────────────────────────────────────────────────────────

func (app *App) showRestoreSettingsDialog() {
	settingsPath := app.getUserSettingsPath()
	configDir := filepath.Dir(configFilePath())
	backupDir := filepath.Join(configDir, "backups", "user_settings")

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_no_backups"))
		return
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_read_backup_dir_failed"), err))
		return
	}
	var backupFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".config") {
			backupFiles = append(backupFiles, e.Name())
		}
	}
	if len(backupFiles) == 0 {
		app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_no_backup_files"))
		return
	}
	sort.Slice(backupFiles, func(i, j int) bool {
		return backupFiles[i] > backupFiles[j]
	})

	selected := ""
	list := widget.NewList(
		func() int { return len(backupFiles) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			obj.(*widget.Label).SetText(backupFiles[id])
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		selected = backupFiles[id]
	}
	var popUp *widget.PopUp
	content := container.NewVBox(
		widget.NewLabel(app.msg("usettings_select_backup")),
		list,
		container.NewHBox(
			widget.NewButton(app.msg("usettings_restore_btn"), func() {
				if selected == "" {
					app.showInfoDialog(app.msg("usettings_info_title"), app.msg("usettings_select_file"))
					return
				}
				popUp.Hide()
				dialog.ShowConfirm(app.msg("usettings_confirm_title"), fmt.Sprintf(app.msg("usettings_confirm_restore"), selected), func(ok bool) {
					if !ok {
						return
					}
					app.createSettingsBackup()
					src := filepath.Join(backupDir, selected)
					data, err := os.ReadFile(src)
					if err != nil {
						app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_read_backup_failed"), err))
						return
					}
					if err := os.WriteFile(settingsPath, data, 0644); err != nil {
						app.showInfoDialog(app.msg("usettings_error_title"), fmt.Sprintf(app.msg("usettings_restore_failed"), err))
						return
					}
					app.appendLog(fmt.Sprintf(app.msg("usettings_restored"), selected))
				}, app.mainWindow)
			}),
			widget.NewButton(app.msg("btn_cancel"), func() { popUp.Hide() }),
		),
	)
	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(400, 300))
	popUp.Show()
}
