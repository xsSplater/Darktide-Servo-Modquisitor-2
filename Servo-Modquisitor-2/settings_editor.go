// settings_editor.go
package main

import (
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

// expandableEntry - обёртка над widget.Entry с увеличенной минимальной шириной.
type expandableEntry struct {
	widget.Entry
}

func (e *expandableEntry) MinSize() fyne.Size {
	return fyne.NewSize(300, e.Entry.MinSize().Height)
}

// expandableSelect - обёртка над widget.Select с увеличенной минимальной шириной.
type expandableSelect struct {
	widget.Select
}

func (e *expandableSelect) MinSize() fyne.Size {
	return fyne.NewSize(300, e.Select.MinSize().Height)
}

// SettingsNode представляет узел в структуре настроек.
type SettingsNode struct {
	Key      string                   // ключ (может быть с кавычками, но храним без)
	Value    interface{}              // может быть string, int, float64, bool, []interface{}, map[string]*SettingsNode
	Children map[string]*SettingsNode // для вложенных таблиц
	IsArray  bool                     // true если это массив
	Parent   *SettingsNode            // ссылка на родителя (для удобства)
	Modified bool                     // флаг, что узел или его потомки изменены
}

// SettingsEditorState хранит состояние редактора.
type SettingsEditorState struct {
	OriginalRoot *SettingsNode // исходные данные (для отката)
	WorkingRoot  *SettingsNode // текущие изменения
	SelectedPath string        // путь к выбранному узлу
	FileModified bool          // флаг изменения файла
	Window       fyne.Window
}

// Добавим карту описаний
var settingsDescriptions = map[string]string{
	"adapter_index":                "Индекс графического адаптера (0 - основной)",
	"aspect_ratio":                 "Соотношение сторон (-1 - авто)",
	"borderless_fullscreen":        "Полноэкранный режим без рамки",
	"fullscreen":                   "Полноэкранный режим",
	"fullscreen_output":            "Номер монитора для полноэкранного режима",
	"gamma":                        "Гамма-коррекция",
	"gpu_id":                       "Идентификатор видеокарты",
	"language_id":                  "Язык интерфейса (ru, en и т.д.)",
	"max_worker_threads":           "Максимальное число рабочих потоков",
	"screen_mode":                  "Режим экрана (window, borderless-fullscreen)",
	"screen_resolution":            "Разрешение экрана [ширина, высота]",
	"vsync":                        "Вертикальная синхронизация",
	"detected_user_settings":       "Настройки, обнаруженные автоматически",
	"master_render_settings":       "Основные настройки рендеринга",
	"render_settings":              "Настройки рендеринга",
	"sound_settings":               "Настройки звука",
	"network_settings":             "Настройки сети",
	"performance_settings":         "Настройки производительности",
	"texture_settings":             "Настройки текстур",
	"direct_storage":               "Настройки DirectStorage",
	"gore_settings":                "Настройки жестокости",
	"interface_settings":           "Настройки интерфейса",
	"launcher_overrides":           "Параметры запуска",
	"launcher_verification_passed": "Проверка лаунчера пройдена",
	"mod_manager_settings":         "Настройки менеджера модов",
	"mods_settings":                "Настройки модов",
	"mesh_streamer_settings":       "Настройки потоковой загрузки мешей",
	"threads":                      "Настройки потоков",
	"version":                      "Версия файла настроек",
}

func getDescription(key string) string {
	if desc, ok := settingsDescriptions[key]; ok {
		return desc
	}
	return ""
}

// Определяем типы виджетов для ключей
const (
	widgetTypeEntry    = "entry"
	widgetTypeBool     = "bool"
	widgetTypeLang     = "lang"
	widgetTypeScreen   = "screen"
	widgetTypeQuality  = "quality"
	widgetTypeGraphics = "graphics"
)

// Карты ключей для каждого типа
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

// Функция определения типа виджета по ключу
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

// Получение списка опций для Select по типу
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

// -------------------- Парсер (доработанный) --------------------

func parseSettingsFile(path string) (*SettingsNode, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	root := &SettingsNode{Key: "root", Children: make(map[string]*SettingsNode)}
	stack := []*SettingsNode{root}
	var currentArray *[]interface{}
	var currentArrayKey string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "--") {
			continue
		}

		// Если внутри массива
		if currentArray != nil {
			if strings.Contains(trimmed, "]") {
				// убираем ] и запятые
				trimmed = strings.TrimSuffix(trimmed, "]")
				trimmed = strings.TrimSuffix(trimmed, ",")
				trimmed = strings.TrimSpace(trimmed)
				if trimmed != "" {
					val := parseValue(trimmed)
					*currentArray = append(*currentArray, val)
				}
				parent := stack[len(stack)-1]
				if node, ok := parent.Children[currentArrayKey]; ok {
					node.Value = *currentArray
				}
				currentArray = nil
				currentArrayKey = ""
				continue
			}
			trimmed = strings.TrimSuffix(trimmed, ",")
			trimmed = strings.TrimSpace(trimmed)
			if trimmed != "" {
				val := parseValue(trimmed)
				*currentArray = append(*currentArray, val)
			}
			continue
		}

		// Обычная строка: ключ = значение или ключ = {
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			key := strings.TrimSpace(parts[0])
			key = strings.Trim(key, `"`)
			valuePart := strings.TrimSpace(parts[1])

			// Начало блока
			if strings.HasPrefix(valuePart, "{") {
				newNode := &SettingsNode{Key: key, Children: make(map[string]*SettingsNode)}
				parent := stack[len(stack)-1]
				parent.Children[key] = newNode
				newNode.Parent = parent
				stack = append(stack, newNode)
				// если блок закрывается в этой же строке
				if strings.Contains(valuePart, "}") {
					stack = stack[:len(stack)-1]
				}
				continue
			}

			// Начало массива
			if strings.HasPrefix(valuePart, "[") {
				arrNode := &SettingsNode{Key: key, IsArray: true, Children: make(map[string]*SettingsNode)}
				parent := stack[len(stack)-1]
				parent.Children[key] = arrNode
				arrNode.Parent = parent
				arr := make([]interface{}, 0)
				currentArray = &arr
				currentArrayKey = key
				// если массив закрывается в этой же строке
				if strings.Contains(valuePart, "]") {
					trimmedArr := strings.TrimPrefix(valuePart, "[")
					trimmedArr = strings.TrimSuffix(trimmedArr, "]")
					trimmedArr = strings.TrimSpace(trimmedArr)
					if trimmedArr != "" {
						vals := strings.Split(trimmedArr, ",")
						for _, v := range vals {
							arr = append(arr, parseValue(strings.TrimSpace(v)))
						}
					}
					arrNode.Value = arr
					currentArray = nil
					currentArrayKey = ""
				}
				continue
			}

			// Простое значение
			val := parseValue(valuePart)
			parent := stack[len(stack)-1]
			leaf := &SettingsNode{Key: key, Value: val}
			parent.Children[key] = leaf
			leaf.Parent = parent
			continue
		}

		// Закрывающая скобка
		if strings.Contains(trimmed, "}") && len(stack) > 1 {
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

// -------------------- Сериализатор (доработанный) --------------------

func serializeSettings(node *SettingsNode) string {
	if node.Key == "root" {
		var sb strings.Builder
		keys := getSortedKeys(node.Children)
		for _, key := range keys {
			child := node.Children[key]
			sb.WriteString(serializeNode(child, ""))
		}
		return sb.String()
	}
	return serializeNode(node, "")
}

func serializeNode(node *SettingsNode, indent string) string {
	var sb strings.Builder
	if node.IsArray {
		sb.WriteString(fmt.Sprintf("%s%s = [\n", indent, quoteKey(node.Key)))
		if node.Value != nil {
			arr, ok := node.Value.([]interface{})
			if ok {
				for _, v := range arr {
					sb.WriteString(fmt.Sprintf("%s\t%s\n", indent, valueToString(v)))
				}
			}
		}
		sb.WriteString(fmt.Sprintf("%s]\n", indent))
		return sb.String()
	}

	if len(node.Children) > 0 {
		sb.WriteString(fmt.Sprintf("%s%s = {\n", indent, quoteKey(node.Key)))
		keys := getSortedKeys(node.Children)
		for _, key := range keys {
			child := node.Children[key]
			sb.WriteString(serializeNode(child, indent+"\t"))
		}
		sb.WriteString(fmt.Sprintf("%s}\n", indent))
		return sb.String()
	}

	if node.Value != nil {
		sb.WriteString(fmt.Sprintf("%s%s = %s\n", indent, quoteKey(node.Key), valueToString(node.Value)))
	}
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
		return fmt.Sprintf(`"%s"`, val) // всегда в кавычках
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

// -------------------- Глубокое копирование --------------------

func deepCopy(node *SettingsNode) *SettingsNode {
	if node == nil {
		return nil
	}
	copy := &SettingsNode{
		Key:      node.Key,
		IsArray:  node.IsArray,
		Children: make(map[string]*SettingsNode),
		Modified: node.Modified,
	}
	if node.Value != nil {
		switch v := node.Value.(type) {
		case []interface{}:
			newArr := make([]interface{}, len(v))
			for i, elem := range v {
				newArr[i] = elem
			}
			copy.Value = newArr
		default:
			copy.Value = v
		}
	}
	for k, child := range node.Children {
		copy.Children[k] = deepCopy(child)
		copy.Children[k].Parent = copy
	}
	return copy
}

// -------------------- Функции для работы с путями --------------------

func getNodeByPath(root *SettingsNode, path string) *SettingsNode {
	if path == "" {
		return root
	}
	parts := strings.Split(path, "/")
	current := root
	for _, part := range parts {
		if current.Children == nil {
			return nil
		}
		if child, ok := current.Children[part]; ok {
			current = child
		} else {
			return nil
		}
	}
	return current
}

// -------------------- Основное окно редактора --------------------

func (app *App) showGameSettingsEditor() {
	settingsPath := app.getUserSettingsPath()
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		app.showInfoDialog(app.messages["error_title"], app.messages["settings_file_not_found"])
		return
	}

	win := app.myApp.NewWindow(app.messages["settings_editor_title"])
	win.Resize(fyne.NewSize(1200, 800))

	root, err := parseSettingsFile(settingsPath)
	if err != nil {
		app.showInfoDialog(app.messages["error_title"], fmt.Sprintf(app.messages["settings_parse_error"], err))
		return
	}

	state := &SettingsEditorState{
		OriginalRoot: deepCopy(root),
		WorkingRoot:  root,
		FileModified: false,
		Window:       win,
	}
	state.OriginalRoot = deepCopy(root)

	// Левая панель: дерево (только два уровня)
	tree := widget.NewTree(
		func(uid string) []string {
			if uid == "" {
				return []string{"system", "mods"}
			}
			if uid == "system" {
				node := getNodeByPath(state.WorkingRoot, "")
				if node == nil {
					return []string{}
				}
				var keys []string
				// Список ключей, которые НЕ показываем в дереве
				hiddenSystemKeys := map[string]bool{
					"detected_user_settings": true,
					"version":                true,
				}
				for k := range node.Children {
					if k != "mods_settings" && !hiddenSystemKeys[k] {
						keys = append(keys, k)
					}
				}
				sort.Strings(keys)
				return keys
			}
			if uid == "mods" {
				modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
				if modsNode == nil || modsNode.Children == nil {
					return []string{}
				}
				var keys []string
				for k := range modsNode.Children {
					// Возвращаем полный путь
					keys = append(keys, "mods_settings/"+k)
				}
				sort.Strings(keys)
				return keys
			}
			return []string{}
		},
		func(uid string) bool {
			return uid == "" || uid == "system" || uid == "mods"
		},
		func(branch bool) fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(uid string, branch bool, obj fyne.CanvasObject) {
			label := obj.(*widget.Label)
			if uid == "" {
				label.SetText(app.messages["menu_settings"])
				return
			}
			if uid == "system" {
				label.SetText(app.messages["menu_settings_game"])
				return
			}
			if uid == "mods" {
				label.SetText(app.messages["menu_settings_mods"])
				return
			}
			// Если uid начинается с "mods_settings/", показываем только имя мода
			if strings.HasPrefix(uid, "mods_settings/") {
				modName := strings.TrimPrefix(uid, "mods_settings/")
				label.SetText(modName)
			} else {
				node := getNodeByPath(state.WorkingRoot, uid)
				if node != nil {
					label.SetText(node.Key)
				} else {
					label.SetText(uid)
				}
			}
		},
	)
	tree.OpenBranch("")
	tree.OpenBranch("system")
	tree.OpenBranch("mods")

	// Переменные состояния для списка
	var selectedPath string
	var rows []SettingsRow
	var settingsList *widget.List

	// --- Вспомогательная функция для создания виджета с фиксированной шириной ---
	createFixedWidthLabel := func(width float32) fyne.CanvasObject {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(width, 1))
		label := widget.NewLabel("")
		label.Wrapping = fyne.TextWrapWord
		return container.NewStack(spacer, label)
	}

	// --- Заголовок (три колонки) ---
	headerKey := createFixedWidthLabel(250)
	headerKey.(*fyne.Container).Objects[1].(*widget.Label).SetText(app.messages["usettingsconf_key"])
	headerKey.(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerKey.(*fyne.Container).Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerVal := createFixedWidthLabel(200)
	headerVal.(*fyne.Container).Objects[1].(*widget.Label).SetText(app.messages["usettingsconf_value"])
	headerVal.(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerVal.(*fyne.Container).Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerDesc := createFixedWidthLabel(400)
	headerDesc.(*fyne.Container).Objects[1].(*widget.Label).SetText(app.messages["usettingsconf_description"])
	headerDesc.(*fyne.Container).Objects[1].(*widget.Label).TextStyle = fyne.TextStyle{Bold: true}
	headerDesc.(*fyne.Container).Objects[1].(*widget.Label).Alignment = fyne.TextAlignCenter

	headerContainer := container.NewHBox(headerKey, headerVal, headerDesc)

	// --- Список параметров ---
	settingsList = widget.NewList(
		func() int { return len(rows) },
		func() fyne.CanvasObject {
			// Возвращаем HBox с пустым контейнером для значения
			return container.NewHBox(
				createFixedWidthLabel(250), // ключ
				container.NewStack(),       // значение
				createFixedWidthLabel(400), // описание
			)
		},
		func(id int, obj fyne.CanvasObject) {
			if id >= len(rows) {
				return
			}
			row := rows[id]
			hbox := obj.(*fyne.Container)

			keyContainer := hbox.Objects[0].(*fyne.Container)
			keyLabel := keyContainer.Objects[1].(*widget.Label)

			valContainer := hbox.Objects[1].(*fyne.Container)
			// Очищаем контейнер значения
			valContainer.Objects = nil

			descContainer := hbox.Objects[2].(*fyne.Container)
			descLabel := descContainer.Objects[1].(*widget.Label)

			widgetType := getWidgetType(row.Key)

			if widgetType == widgetTypeEntry {
				entry := &expandableEntry{}
				entry.ExtendBaseWidget(entry)
				entry.SetText(formatValue(row.Node))
				entry.OnChanged = func(newText string) {
					newVal := parseValue(newText)
					if !compareValues(row.Value, newVal) {
						row.Value = newVal
						row.Node.Value = newVal
						row.Node.Modified = true
						state.FileModified = true
						for i := range rows {
							if rows[i].Key == row.Key {
								rows[i].Value = newVal
								break
							}
						}
						settingsList.Refresh()
					}
				}
				valContainer.Add(entry)
			} else {
				sel := &expandableSelect{}
				sel.ExtendBaseWidget(sel)
				options := getSelectOptions(widgetType)
				sel.Options = options
				currentVal := fmt.Sprintf("%v", row.Value)
				sel.SetSelected(currentVal)
				sel.OnChanged = func(selected string) {
					newVal := parseValue(selected)
					if !compareValues(row.Value, newVal) {
						row.Value = newVal
						row.Node.Value = newVal
						row.Node.Modified = true
						state.FileModified = true
						for i := range rows {
							if rows[i].Key == row.Key {
								rows[i].Value = newVal
								break
							}
						}
						settingsList.Refresh()
					}
				}
				valContainer.Add(sel)
			}

			keyLabel.SetText(row.Key)
			descLabel.SetText(row.Desc)
		},
	)

	// Обёртка для заголовка + списка
	tableContainer := container.NewBorder(headerContainer, nil, nil, nil, settingsList)

	// Обработчик выбора в дереве
	tree.OnSelected = func(uid string) {
		if uid == "" || uid == "system" || uid == "mods" {
			rows = []SettingsRow{}
			settingsList.Refresh()
			return
		}

		// Для модов путь уже содержит "mods_settings/", используем его как есть
		node := getNodeByPath(state.WorkingRoot, uid)
		if node == nil {
			rows = []SettingsRow{}
			settingsList.Refresh()
			return
		}

		selectedPath = uid

		rows = []SettingsRow{}
		if len(node.Children) > 0 {
			keys := getSortedKeys(node.Children)
			for _, key := range keys {
				child := node.Children[key]
				rows = append(rows, SettingsRow{
					Key:   key,
					Value: child.Value,
					Desc:  getDescription(key),
					Node:  child,
				})
			}
		} else {
			rows = append(rows, SettingsRow{
				Key:   node.Key,
				Value: node.Value,
				Desc:  getDescription(node.Key),
				Node:  node,
			})
		}
		settingsList.Refresh()
	}

	// Кнопки (размещаем наверху правой панели)
	saveBtn := widget.NewButton(app.messages["btn_save"], func() {
		serialized := serializeSettings(state.WorkingRoot)
		if strings.TrimSpace(serialized) == "" {
			app.showInfoDialog(app.messages["error_title"], "Empty settings, not saving.")
			return
		}

		err := os.WriteFile(settingsPath, []byte(serialized), 0644)
		if err != nil {
			app.showInfoDialog(app.messages["error_title"], fmt.Sprintf(app.messages["settings_save_error"], err))
			return
		}
		state.FileModified = false
		state.OriginalRoot = deepCopy(state.WorkingRoot)
		clearModifiedFlags(state.WorkingRoot)
		app.appendLog(app.messages["settings_saved"])
		settingsList.Refresh()
	})

	cancelBtn := widget.NewButton(app.messages["btn_cancel"], func() {
		state.WorkingRoot = deepCopy(state.OriginalRoot)
		state.FileModified = false
		clearModifiedFlags(state.WorkingRoot)
		app.appendLog(app.messages["settings_changes_discarded"])
		tree.Refresh()
		if selectedPath != "" {
			tree.Select(selectedPath)
		}
	})

	deleteModBtn := widget.NewButton("Удалить настройки мода", func() {
		if selectedPath == "" || !strings.HasPrefix(selectedPath, "mods/") {
			app.showInfoDialog("Информация", "Выберите мод для удаления настроек.")
			return
		}
		parts := strings.Split(selectedPath, "/")
		if len(parts) != 2 || parts[0] != "mods" {
			return
		}
		modName := parts[1]
		dialog.ShowConfirm("Подтверждение", fmt.Sprintf("Удалить настройки мода '%s'?", modName), func(ok bool) {
			if !ok {
				return
			}
			modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
			if modsNode != nil && modsNode.Children != nil {
				if _, exists := modsNode.Children[modName]; exists {
					delete(modsNode.Children, modName)
					state.FileModified = true
					tree.Refresh()
					rows = []SettingsRow{}
					settingsList.Refresh()
					app.appendLog(fmt.Sprintf("Настройки мода '%s' удалены.", modName))
				} else {
					app.showInfoDialog("Ошибка", "Мод не найден в настройках.")
				}
			}
		}, win)
	})

	exportBtn := widget.NewButton("Экспорт мода", func() {
		if selectedPath == "" || !strings.HasPrefix(selectedPath, "mods/") {
			app.showInfoDialog("Информация", "Выберите мод для экспорта.")
			return
		}
		parts := strings.Split(selectedPath, "/")
		if len(parts) != 2 || parts[0] != "mods" {
			return
		}
		modName := parts[1]
		modsNode := getNodeByPath(state.WorkingRoot, "mods_settings")
		if modsNode == nil || modsNode.Children == nil {
			app.showInfoDialog("Ошибка", "Нет настроек модов.")
			return
		}
		modNode, exists := modsNode.Children[modName]
		if !exists {
			app.showInfoDialog("Ошибка", "Мод не найден.")
			return
		}
		fd := dialog.NewFileSave(func(uri fyne.URIWriteCloser, err error) {
			if err != nil || uri == nil {
				return
			}
			defer uri.Close()
			serialized := serializeNode(modNode, "")
			_, err = uri.Write([]byte(serialized))
			if err != nil {
				app.showInfoDialog("Ошибка", fmt.Sprintf("Не удалось сохранить: %v", err))
			} else {
				app.appendLog(fmt.Sprintf("Настройки мода '%s' экспортированы.", modName))
			}
		}, win)
		fd.SetFileName(modName + ".modsettings")
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".modsettings"}))
		fd.Show()
	})

	importBtn := widget.NewButton("Импорт мода", func() {
		fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil || reader == nil {
				return
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			if err != nil {
				app.showInfoDialog("Ошибка", fmt.Sprintf("Не удалось прочитать файл: %v", err))
				return
			}
			tmpRoot, err := parseSettingsData(string(data))
			if err != nil {
				app.showInfoDialog("Ошибка", fmt.Sprintf("Некорректный файл: %v", err))
				return
			}
			if len(tmpRoot.Children) != 1 {
				app.showInfoDialog("Ошибка", "Файл должен содержать ровно один блок настроек мода.")
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
				root := state.WorkingRoot
				root.Children["mods_settings"] = modsNode
				modsNode.Parent = root
			}
			if _, exists := modsNode.Children[modKey]; exists {
				dialog.ShowConfirm("Подтверждение", fmt.Sprintf("Мод '%s' уже есть. Заменить?", modKey), func(ok bool) {
					if ok {
						modsNode.Children[modKey] = modNode
						modNode.Parent = modsNode
						state.FileModified = true
						tree.Refresh()
						if selectedPath != "" {
							tree.Select(selectedPath)
						}
						app.appendLog(fmt.Sprintf("Настройки мода '%s' импортированы (замена).", modKey))
					}
				}, win)
			} else {
				modsNode.Children[modKey] = modNode
				modNode.Parent = modsNode
				state.FileModified = true
				tree.Refresh()
				if selectedPath != "" {
					tree.Select(selectedPath)
				}
				app.appendLog(fmt.Sprintf("Настройки мода '%s' импортированы.", modKey))
			}
		}, win)
		fd.SetFilter(storage.NewExtensionFileFilter([]string{".modsettings"}))
		fd.Show()
	})

	btnRow := container.NewHBox(saveBtn, cancelBtn, deleteModBtn, exportBtn, importBtn)
	btnContainer := container.NewVBox(btnRow, widget.NewSeparator())

	rightPanel := container.NewBorder(btnContainer, nil, nil, nil, tableContainer)
	leftPanel := container.NewBorder(nil, nil, nil, nil, tree)

	split := container.NewHSplit(leftPanel, rightPanel)
	split.Offset = 0.3

	win.SetContent(split)
	win.Show()
}

// -------------------- Вспомогательные функции --------------------

type SettingsRow struct {
	Key   string
	Value interface{}
	Desc  string
	Node  *SettingsNode
}

func getSortedKeys(m map[string]*SettingsNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatValue(node *SettingsNode) string {
	if node.IsArray {
		if node.Value == nil {
			return "[]"
		}
		arr, ok := node.Value.([]interface{})
		if !ok {
			return valueToString(node.Value)
		}
		strs := make([]string, len(arr))
		for i, v := range arr {
			strs[i] = valueToString(v)
		}
		return strings.Join(strs, " x ")
	}
	if node.Value == nil {
		return ""
	}
	return valueToString(node.Value)
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

// -------------------- Функции бэкапа и восстановления --------------------

func (app *App) showRestoreSettingsDialog() {
	settingsPath := app.getUserSettingsPath()

	// Путь к папке конфигурации программы
	configDir := filepath.Dir(configFilePath())
	backupDir := filepath.Join(configDir, "backups", "user_settings")

	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		app.showInfoDialog("Информация", "Нет сохранённых бэкапов.")
		return
	}
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		app.showInfoDialog("Ошибка", fmt.Sprintf("Не удалось прочитать папку бэкапов: %v", err))
		return
	}
	var backupFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".config") {
			backupFiles = append(backupFiles, e.Name())
		}
	}
	if len(backupFiles) == 0 {
		app.showInfoDialog("Информация", "Нет файлов бэкапов.")
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
		widget.NewLabel("Выберите бэкап для восстановления:"),
		list,
		container.NewHBox(
			widget.NewButton("Восстановить", func() {
				if selected == "" {
					app.showInfoDialog("Информация", "Выберите файл.")
					return
				}
				popUp.Hide()
				dialog.ShowConfirm("Подтверждение", fmt.Sprintf("Восстановить настройки из файла %s? Текущие настройки будут заменены.", selected), func(ok bool) {
					if !ok {
						return
					}
					app.createSettingsBackup()
					src := filepath.Join(backupDir, selected)
					data, err := os.ReadFile(src)
					if err != nil {
						app.showInfoDialog("Ошибка", fmt.Sprintf("Не удалось прочитать бэкап: %v", err))
						return
					}
					err = os.WriteFile(settingsPath, data, 0644)
					if err != nil {
						app.showInfoDialog("Ошибка", fmt.Sprintf("Не удалось восстановить настройки: %v", err))
						return
					}
					app.appendLog(fmt.Sprintf("Настройки восстановлены из бэкапа: %s", selected))
				}, app.mainWindow)
			}),
			widget.NewButton(app.messages["btn_cancel"], func() { popUp.Hide() }),
		),
	)
	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(400, 300))
	popUp.Show()
}
