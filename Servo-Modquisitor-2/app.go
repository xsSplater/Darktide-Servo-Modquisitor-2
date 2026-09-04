// app.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"log"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/zalando/go-keyring"
)

type Config struct {
	CustomColors map[string]color.NRGBA `json:"custom_colors"`

	InitialSetupDone          bool    `json:"initial_setup_done"`
	ForceEnglishModNames      bool    `json:"force_english_mod_names"`
	ModsGloballyEnabled       bool    `json:"mods_globally_enabled"`
	ShowSystemMods            bool    `json:"show_system_mods"`
	ShowModListAfterSort      bool    `json:"show_mod_list_after_sort"`
	SkipSortFilesPrompt       bool    `json:"skip_sort_files_prompt"`
	SuppressAMLWarning        bool    `json:"suppress_aml_warning"` // не предупреждаем об AML
	WindowMaximized           bool    `json:"window_maximized"`
	StatusRowSpacing          float32 `json:"status_row_spacing"` // отступ между строками
	StatusFontSize            float32 `json:"status_font_size"`   // размер шрифта для статуса
	WindowHeight              int     `json:"window_height"`
	WindowWidth               int     `json:"window_width"`
	LogFileSizeLimit          int64   `json:"log_file_size_limit"` // Размер файла лога
	DateFormat                string  `json:"date_format"`
	GameRoot                  string  `json:"game_root"`
	Language                  string  `json:"language"`
	LastMandatoryRulesVersion string  `json:"last_mandatory_rules_version"`
	LastModDatabaseVersion    string  `json:"last_mod_database_version"`
	LastUpdateCheck           string  `json:"last_update_check"`
	ModsPath                  string  `json:"mods_path"`
	Theme                     string  `json:"theme"`
	UpdateCheckFrequency      string  `json:"update_check_frequency"`
	ActiveProfile             string  `json:"active_profile"`
}

type ModVersionInfo struct {
	Timestamp   int64  `json:"timestamp"`
	Version     string `json:"version"`
	Folder      string `json:"folder"`                 // Название папки мода в nexus_versions.json
	Source      string `json:"source,omitempty"`       // "nexus" или "manual"
	InstalledAt int64  `json:"installed_at,omitempty"` // время установки/обновления
}

type App struct {
	launchGameFunc      func(version GameVersion, gameRoot string, skipLauncher bool) error
	messages            map[string]string
	nexusVersionCache   map[string]ModVersionInfo // локальная версия
	nexusLatestVersions map[string]string         // последняя версия с сайта
	changelogCache      map[string]string
	changelogTexts      map[string]string
	changelogExpanded   map[string]bool

	selectedModIndex         atomic.Int32
	orderDirty               bool
	blinkSaveOrderActive     bool
	amlDetected              bool
	showSelectColumn         bool
	pathsInitialized         bool
	tableBorder              *canvas.Rectangle // Рамка вокруг таблицы
	screenBgRect             *canvas.Rectangle // ссылки на динамически окрашиваемые объекты
	headerBoxBgRect          *canvas.Rectangle
	tipBgRect                *canvas.Rectangle
	topPanelBgRect           *canvas.Rectangle
	managePanelBgRect        *canvas.Rectangle
	descCardBgRect           *canvas.Rectangle
	descTitle                *canvas.Text
	logHeaderText            *canvas.Text
	allMods                  []checks.ModInfo
	displayedMods            []checks.ModInfo
	systemMods               []checks.ModInfo // base и dmf
	modDatabase              []checks.ModDBEntry
	cfg                      *Config
	consoleScroll            *container.Scroll
	descCardContent          *fyne.Container   // контейнер с описанием
	descCardScroll           *container.Scroll // скроллер описания
	manageBtn                *CustomButton
	selectAllBtn             *CustomButton
	deselectAllBtn           *CustomButton
	enableSelectedBtn        *CustomButton
	disableSelectedBtn       *CustomButton
	enableAllBtn             *CustomButton
	disableAllBtn            *CustomButton
	btnRemoveAll             *CustomButton
	btnRemoveSelected        *CustomButton
	moveToTopBtn             *CustomButton
	moveToBottomBtn          *CustomButton
	openFolderBtn            *CustomButton
	btnToggle                *CustomButton
	btnSaveOrder             *CustomButton
	btnRefresh               *CustomButton
	btnInstall               *CustomButton
	btnRemove                *CustomButton
	btnUp                    *CustomButton
	btnDown                  *CustomButton
	btnLaunchNormal          *CustomButton
	btnLaunchNoLauncher      *CustomButton
	btnSortChecks            *CustomButton
	btnUpdateAll             *CustomButton
	btnUpdateMod             *CustomButton
	btnUpdateSelected        *CustomButton
	btnCheckUpdates          *CustomButton
	btnEditVersion           *CustomButton
	searchClearBtn           *CustomButton
	btnAMLConfig             *CustomButton // AML
	myApp                    fyne.App
	mainWindow               fyne.Window
	selectColumnBgRes        fyne.Resource
	toggleOffIcon            fyne.Resource
	toggleOnIcon             fyne.Resource
	systemModsTableContainer *fyne.Container
	tableBorderContainer     *fyne.Container
	managePanel              *fyne.Container // Управление видимостью панели управления модами
	descExtraContainer       *fyne.Container
	nxmListener              net.Listener // слушатель nxm-ссылок
	logFile                  *os.File     // Логирование
	patcherType              PatcherType
	gameRoot                 string
	selectedModName          string
	oauthState               string // Nexus API
	oauthVerifier            string
	lastNxmURL               string
	modsMutex                sync.RWMutex // защита allMods
	cacheMutex               sync.RWMutex // для nexusVersionCache
	latestMutex              sync.RWMutex // для nexusLatestVersions
	changelogMutex           sync.RWMutex // для безопасного доступа к картам
	lastNxmTime              time.Time
	enrichDebounce           *time.Timer
	moveLabel                *widget.Label
	statusLabel              *widget.Label
	descAuthor               *widget.Label
	descInstalled            *widget.Label
	descBody                 *widget.Label
	filterLabel              *widget.Label
	counterLabel             *widget.Label
	descLocalVersion         *widget.Label
	descLatestVersion        *widget.Label
	descLastUpdated          *widget.Label
	descOriginalUpload       *widget.Label
	descConflict             *widget.Label // под descStatus
	profileLabel             *widget.Label
	moveToEntry              *widget.Entry
	searchEntry              *widget.Entry
	descURL                  *widget.Hyperlink
	githubLink               *widget.Hyperlink
	logWindow                *widget.RichText
	filterSelect             *widget.Select
	profileSelect            *widget.Select // выпадающий список профилей
	modTable                 *widget.Table
	headerTable              *widget.Table
	systemModsTable          *widget.Table // таблица системных модов
}

func NewApp(cfg *Config, myApp fyne.App) *App {
	app := &App{
		cfg:                 cfg,
		messages:            map[string]string{},
		myApp:               myApp,
		nexusVersionCache:   make(map[string]ModVersionInfo),
		nexusLatestVersions: make(map[string]string),
		changelogCache:      make(map[string]string),
		changelogTexts:      make(map[string]string),
		changelogExpanded:   make(map[string]bool),
	}
	app.selectedModIndex.Store(-1)
	app.loadLanguage(cfg.Language)
	app.loadNexusVersionCache()

	switch cfg.Theme {
	case "light":
		myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
	case "highcontrast":
		myApp.Settings().SetTheme(&themes.HighContrastTheme{})
	case "custom":
		colors := make(map[string]color.Color)
		for k, v := range cfg.CustomColors {
			colors[k] = v
		}
		myApp.Settings().SetTheme(&themes.CustomTheme{Colors: colors})
	default:
		myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
	}

	// Инициализируем gameRoot и patcherType из конфига
	app.gameRoot = cfg.GameRoot
	if app.gameRoot == "" {
		// запасной вариант - старый поиск
		app.gameRoot = getGameRootLegacy()
	}
	app.patcherType = detectPatcherTypeWithRoot(app.gameRoot)

	return app
}

// getGameRootLegacy - старый способ поиска от exe (для обратной совместимости)
func getGameRootLegacy() string {
	exePath, _ := os.Executable()
	dir := filepath.Dir(exePath)
	for {
		if detectGameVersion(dir) != VersionUnknown {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func (app *App) loadNexusVersionCache() {
	if app.cfg.ActiveProfile == "" {
		app.nexusVersionCache = make(map[string]ModVersionInfo)
		return
	}
	path := filepath.Join(app.activeProfilePath(), FileNameNexusVersions)
	data, err := os.ReadFile(path)
	if err != nil {
		app.nexusVersionCache = make(map[string]ModVersionInfo)
		return
	}
	var raw map[string]ModVersionInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		// пробуем старый формат
		var old map[string]string
		if err2 := json.Unmarshal(data, &old); err2 == nil {
			raw = make(map[string]ModVersionInfo)
			for k, v := range old {
				ts, _ := strconv.ParseInt(v, 10, 64)
				raw[k] = ModVersionInfo{Timestamp: ts, Version: "", Folder: "", Source: "nexus"}
			}
		} else {
			app.nexusVersionCache = make(map[string]ModVersionInfo)
			return
		}
	}

	newCache := make(map[string]ModVersionInfo)
	for key, info := range raw {
		// Проверяем, что ключ не содержит ID > 1500
		if strings.Contains(key, ":") {
			parts := strings.SplitN(key, ":", 2)
			if len(parts) == 2 {
				if id, err := strconv.Atoi(parts[0]); err == nil && id > MaxModsID {
					// Пропускаем ошибочную запись
					continue
				}
			}
		}
		if info.Source == "" {
			info.Source = "nexus"
		}
		// Обратная совместимость
		if !strings.Contains(key, ":") && info.Folder != "" {
			newKey := key + ":" + info.Folder
			newCache[newKey] = info
		} else {
			newCache[key] = info
		}
	}
	app.nexusVersionCache = newCache
	if len(newCache) > 0 {
		app.saveNexusVersionCache()
	}
}

func (app *App) saveNexusVersionCache() {
	if app.cfg.ActiveProfile == "" {
		return
	}
	path := filepath.Join(app.activeProfilePath(), FileNameNexusVersions)

	keys := make([]string, 0, len(app.nexusVersionCache))
	for k := range app.nexusVersionCache {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		id1 := extractModIDFromKey(keys[i])
		id2 := extractModIDFromKey(keys[j])
		if id1 != id2 {
			return id1 < id2
		}
		return keys[i] < keys[j]
	})

	var buf bytes.Buffer
	buf.WriteString("{\n")
	for i, key := range keys {
		val := app.nexusVersionCache[key]
		// Ключ с одним табом
		buf.WriteString("\t\"")
		buf.WriteString(key)
		buf.WriteString("\": {\n")
		// Поля с двумя табами
		buf.WriteString("\t\t\"timestamp\": ")
		buf.WriteString(strconv.FormatInt(val.Timestamp, 10))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"version\": ")
		buf.WriteString(strconv.Quote(val.Version))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"folder\": ")
		buf.WriteString(strconv.Quote(val.Folder))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"source\": ")
		buf.WriteString(strconv.Quote(val.Source))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"installed_at\": ")
		buf.WriteString(strconv.FormatInt(val.InstalledAt, 10))
		buf.WriteString("\n")
		buf.WriteString("\t}")
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}")
	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		app.appendLog(fmt.Sprintf("Failed to write nexus versions cache: %v", err))
	}
}

func configFilePath() string {
	dir, _ := os.UserConfigDir()
	appDir := filepath.Join(dir, СonfigFolderSMQ)
	if err := os.MkdirAll(appDir, 0755); err != nil {
		log.Printf("Failed to create config directory: %v", err)
	}
	return filepath.Join(appDir, FileNameConfig)
}

func loadConfig() *Config {
	path := configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("Config file not found, using defaults: %v", err)
		return defaultConfig()
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		log.Printf("Failed to parse config, using defaults: %v", err)
		return defaultConfig()
	}
	if c.UpdateCheckFrequency == "" {
		c.UpdateCheckFrequency = "every_start"
	}
	if !c.InitialSetupDone {
		c.ShowSystemMods = true
	}
	// Нормализуем пути для Windows
	c.ModsPath = filepath.FromSlash(c.ModsPath)
	c.GameRoot = filepath.FromSlash(c.GameRoot)
	return &c
}

// вспомогательная функция
func defaultConfig() *Config {
	return &Config{
		Language:             "en",
		Theme:                "dark",
		DateFormat:           "dd-mm-yyyy",
		UpdateCheckFrequency: "every_start",
		ShowSystemMods:       true,
		ShowModListAfterSort: true,
	}
}

func saveConfig(c *Config) {
	path := configFilePath()
	data, err := json.MarshalIndent(c, "", "	")
	if err != nil {
		log.Printf("Failed to marshal config: %v", err)
		return
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		log.Printf("Failed to save config: %v", err)
	}
}

// syncVersionCache синхронизирует кэш версий с локальными файлами,
// чтобы избежать ложных уведомлений об обновлении.
func (app *App) syncVersionCache() {
	// Программа
	exePath, err := os.Executable()
	if err == nil {
		if info, err := os.Stat(exePath); err == nil {
			ts := info.ModTime().Unix()
			// Обновляем кэш программы, если версия отличается или ключа нет
			if saved, ok := app.getCachedVersion(NexusCacheKeyProgram); !ok || saved.Version != AppVersion {
				app.setCachedVersion(NexusCacheKeyProgram, ModVersionInfo{
					Timestamp: ts,
					Version:   AppVersion,
					Folder:    "Program",
					Source:    "nexus",
				})
				app.saveNexusVersionCache()
				app.appendLog(app.messages["log_version_cached_program"] + AppVersion)
			}
		}
	}

	// Файлы сортировки (используем версию из mod_database.json)
	dbVersion := app.cfg.LastModDatabaseVersion
	if dbVersion != "" {
		dbPath := filepath.Join(app.cfg.ModsPath, FileNameModDatabase)
		if info, err := os.Stat(dbPath); err == nil {
			ts := info.ModTime().Unix()
			if saved, ok := app.getCachedVersion(NexusCacheKeyRules); !ok || saved.Version != dbVersion {
				app.setCachedVersion(NexusCacheKeyRules, ModVersionInfo{
					Timestamp: ts,
					Version:   dbVersion,
					Folder:    "Sorting Rules",
					Source:    "nexus",
				})
				app.saveNexusVersionCache()
				app.appendLog(app.messages["log_version_cached_sort"] + dbVersion)
			}
		}
	}
}

func (app *App) loadLanguage(lang string) error {
	data, err := embeddedFiles.ReadFile(FileNameMessages)
	if err != nil {
		return fmt.Errorf("cannot read messages.json: %w", err)
	}

	// Валидация JSON
	if !json.Valid(data) {
		// Попробуем найти строку с ошибкой
		var syntaxErr *json.SyntaxError
		dummy := make(map[string]map[string]string)
		if err := json.Unmarshal(data, &dummy); err != nil {
			if errors.As(err, &syntaxErr) {
				line, col := 1, 1
				for i := 0; i < int(syntaxErr.Offset) && i < len(data); i++ {
					if data[i] == '\n' {
						line++
						col = 1
					} else {
						col++
					}
				}
				start := int(syntaxErr.Offset) - 30
				if start < 0 {
					start = 0
				}
				end := int(syntaxErr.Offset) + 30
				if end > len(data) {
					end = len(data)
				}
				snippet := string(data[start:end])
				return fmt.Errorf("JSON error in messages.json at line %d, col %d: %v\nnear: ...%s...", line, col, syntaxErr, snippet)
			}
			return fmt.Errorf("messages.json is not valid JSON: %w", err)
		}
		return fmt.Errorf("messages.json is not valid JSON (unknown error)")
	}

	var raw map[string]map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("cannot unmarshal messages.json: %w", err)
	}

	newMessages := make(map[string]string)
	for key, trans := range raw {
		if val, ok := trans[lang]; ok && val != "" {
			newMessages[key] = val
		} else if val, ok := trans["en"]; ok && val != "" {
			newMessages[key] = val
		} else {
			for _, v := range trans {
				if v != "" {
					newMessages[key] = v
					break
				}
			}
		}
	}
	app.messages = newMessages
	return nil
}

func (app *App) getTitle() string {
	return app.messages["app_title_long"]
}

func (app *App) formatDate(t time.Time, pattern string) string {
	switch pattern {
	case "yyyy-mm-dd":
		return t.Format(YYYYMMDD_TimeFormat)
	case "mm-dd-yyyy":
		return t.Format(MMDDYYYY_TimeFormat)
	default:
		return t.Format(DDMMYYYY_TimeFormat)
	}
}

type ModDatabaseFile struct {
	Version string              `json:"version"`
	Mods    []checks.ModDBEntry `json:"mod_database"`
}

func (app *App) loadModDatabase(filename string) error {
	// Используем глобальный путь (рядом с программой)
	fullPath := filepath.Join(checks.GlobalDataDir(), filename)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", fullPath, err)
	}
	var container ModDatabaseFile
	if err := json.Unmarshal(data, &container); err != nil {
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			offset := int(syntaxErr.Offset)
			line, col := 1, 1
			for i := 0; i < offset && i < len(data); i++ {
				if data[i] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			start := offset - 20
			if start < 0 {
				start = 0
			}
			end := offset + 20
			if end > len(data) {
				end = len(data)
			}
			snippet := string(data[start:end])
			return fmt.Errorf("JSON error in %s at line %d, col %d: %v\nnear: ...%s...", fullPath, line, col, syntaxErr, snippet)
		}
		return fmt.Errorf("cannot unmarshal %s: %w", fullPath, err)
	}
	app.modDatabase = container.Mods
	app.cfg.LastModDatabaseVersion = container.Version
	saveConfig(app.cfg)
	return nil
}

func convertDeps(deps []checks.Dependency) []sorter.ModDependency {
	out := make([]sorter.ModDependency, len(deps))
	for i, d := range deps {
		out[i] = sorter.ModDependency{Dependent: d.Dependent, Required: d.Required}
	}
	return out
}

func (app *App) isModActive(name string) bool {
	mod := app.findModByName(name)
	return mod != nil && mod.Active
}

// logVersions выводит в GUI-лог текущие версии программы и файлов сортировки.
func (app *App) logVersions() {
	app.appendLog(fmt.Sprintf("Program version: %s", AppVersion))
	app.appendLog(fmt.Sprintf("mandatory_obsolete_incompatible_dependencies.json version: %s", checks.GetExternalVersion()))
	app.appendLog(fmt.Sprintf("mod_database.json version: %s", app.cfg.LastModDatabaseVersion))
}

// initializePaths определяет пути к корню игры и папке mods.
// Вызывается один раз при старте, если пути ещё не заданы.
func (app *App) initializePaths() {

	// 0. Если путь уже есть и валиден — выходим
	if app.cfg.ModsPath != "" {
		if _, err := os.Stat(app.cfg.ModsPath); err == nil {
			if app.cfg.GameRoot == "" {
				app.cfg.GameRoot = filepath.Dir(app.cfg.ModsPath)
				saveConfig(app.cfg)
			}
			return
		}
		app.cfg.ModsPath = ""
		app.cfg.GameRoot = ""
		saveConfig(app.cfg)
	}

	// 1. Автопоиск по стандартным папкам (Steam, Xbox)
	autoRoot := autoFindGameRoot()
	if autoRoot != "" {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.messages["path_found_title"],
			fmt.Sprintf(app.messages["path_found_message"], autoRoot),
			app.messages["btn_yes"],
			app.messages["btn_choose_other"],
		)
		if choice == 0 {
			app.setGamePaths(autoRoot)
			return
		}
		// choice == 1 -> продолжаем к поиску от exe
	}

	// 2. Поиск от местоположения текущего exe (поднимаемся вверх по дереву папок)
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	guessedRoot := findGameRootFrom(exeDir)
	if guessedRoot != "" {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.messages["path_found_title"],
			fmt.Sprintf(app.messages["path_found_message"], guessedRoot),
			app.messages["btn_yes"],
			app.messages["btn_choose_other"],
		)
		if choice == 0 {
			app.setGamePaths(guessedRoot)
			return
		}
		// choice == 1 -> продолжаем к ручному выбору
	}

	// В initializePaths, после неудачного автопоиска по стандартным папкам и до ручного выбора:

	// 2.5. Расширенный поиск по дискам
	diskSearchResult := app.showDiskSearchDialog()
	res := <-diskSearchResult
	if res.Success && res.Path != "" {
		app.setGamePaths(res.Path)
		return
	}
	// Если пользователь закрыл диалог без выбора или выбрал отмену - переходим к ручному

	// 3. Ручной выбор папки (асинхронный, без блокировки главного потока)
	done := make(chan struct{})
	var selectedPath string
	var cancelled bool

	app.chooseGameRootManually(done, &selectedPath, &cancelled)

	<-done // ожидание в фоновой горутине

	if cancelled {
		app.appendLog("Game root not selected. Exiting.")
		app.closeApp()
	}

	// 4. Валидация выбранной папки (должна содержать binaries или content)
	if _, err := os.Stat(filepath.Join(selectedPath, "binaries")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(selectedPath, "content")); os.IsNotExist(err) {
			// Папка не похожа на корень игры - предлагаем выбрать другую или выйти
			choice := app.showChoiceDialogSync(
				app.mainWindow,
				app.messages["not_game_root_title"],
				fmt.Sprintf(app.messages["not_game_root_message"], selectedPath),
				app.messages["btn_choose_other"],
				app.messages["btn_cancel"],
			)
			if choice == 1 { // Отмена
				app.appendLog("Game root not selected. Exiting.")
				app.closeApp()
			}
			// choice == 0 -> повторяем выбор (рекурсивно, но пути пустые)
			app.initializePaths()
			return
		}
	}

	// 5. Путь корректен - сохраняем
	app.setGamePaths(selectedPath)
}

// findGameRootFrom поднимается от указанной директории вверх, пока не найдёт папку с binaries или content.
func findGameRootFrom(startDir string) string {
	dir := startDir
	for {
		if _, err := os.Stat(filepath.Join(dir, "binaries")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, "content")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// autoFindGameRoot ищет Darktide в стандартных местах установки.
// Возвращает путь к корню игры или пустую строку.
func autoFindGameRoot() string {
	possibleRoots := []string{
		// Steam (стандартный)
		"C:\\Program Files (x86)\\Steam\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"C:\\Program Files\\Steam\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		// Дополнительные библиотеки Steam (часто на D: или E:)
		"D:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"E:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"F:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		// Xbox Game Pass
		"C:\\XboxGames\\Warhammer 40,000 Darktide\\Content",
	}
	for _, path := range possibleRoots {
		if _, err := os.Stat(filepath.Join(path, "binaries")); err == nil {
			return path
		}
		if _, err := os.Stat(filepath.Join(path, "content")); err == nil {
			return path
		}
	}
	return ""
}

// setGamePaths устанавливает корень игры и определяет путь к mods.
func (app *App) setGamePaths(gameRoot string) {
	app.cfg.GameRoot = gameRoot
	modsPath := filepath.Join(gameRoot, "mods")

	if _, err := os.Stat(modsPath); os.IsNotExist(err) {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.messages["mods_not_found_title"],
			app.messages["mods_not_found_message"],
			app.messages["btn_yes_open_dml"],
			app.messages["btn_no_create_folder"],
		)
		if choice == 0 {
			u1, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/19")
			u2, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/8")
			_ = app.myApp.OpenURL(u1)
			_ = app.myApp.OpenURL(u2)
			if err := os.MkdirAll(modsPath, 0755); err != nil {
				app.appendLog(fmt.Sprintf("Failed to create mods folder: %v", err))
				app.showInfoDialog(app.messages["window_error_title"], fmt.Sprintf("Failed to create mods folder: %v", err))
				app.closeApp()
			}
		} else {
			if err := os.MkdirAll(modsPath, 0755); err != nil {
				app.appendLog(fmt.Sprintf("Failed to create mods folder: %v", err))
				app.showInfoDialog(app.messages["window_error_title"], fmt.Sprintf("Failed to create mods folder: %v", err))
				app.closeApp()
			}
		}
	} else {
	}
	app.cfg.ModsPath = modsPath
	saveConfig(app.cfg)
	app.gameRoot = gameRoot
	app.patcherType = detectPatcherTypeWithRoot(gameRoot)
}

// chooseGameRootManually открывает диалог выбора папки для корня игры.
func (app *App) chooseGameRootManually(done chan struct{}, selectedPath *string, cancelled *bool) {
	fyne.Do(func() {
		dlg := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				*cancelled = true
				close(done)
				return
			}
			*selectedPath = filepath.FromSlash(uri.Path())
			close(done)
		}, app.mainWindow)

		dlg.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
		dlg.Show()
	})
}

// reloadAfterPathChange перезагружает все данные после смены пути.
func (app *App) reloadAfterPathChange() {
	// Здесь только обновление UI и проверка DML/DMF, загрузка баз уже выполнена в loadDataAfterInit
	app.refreshModList()
	if app.btnToggle != nil {
		app.updateToggleButtonText(app.btnToggle)
	}
	// app.updateLaunchButtonTexts()
	app.updateDescriptionForMod(app.selectedModName)
	app.forceRefreshTable()

	if !app.pathsInitialized {
		app.pathsInitialized = true
		if !checks.FolderExists("base") {
			app.appendLog(app.messages["log_warn_base_missing"])
			app.showInfoDialog(
				app.messages["window_error_title"],
				app.messages["missing_base_dml"],
			)
		}
		if !checks.FolderExists("dmf") {
			app.appendLog(app.messages["dmf_missing"])
			app.showInfoDialog(
				app.messages["window_error_title"],
				app.messages["missing_dmf_dmf"],
			)
		}
		// Диалог внутри ensureSortFiles асинхронный, поэтому задержка не нужна
		fyne.Do(func() {
			app.ensureSortFiles()
		})
	}
}

// loadDataAfterInit загружает все данные (базы, кэш, списки модов) и обновляет UI.
// Вызывается из фоновой горутины после того, как пути определены.
func (app *App) loadDataAfterInit() {
	// 1. Установить язык для checks
	checks.SetLanguage(app.cfg.Language)

	// 2. Инициализировать глобальные переменные checks
	checks.InitGlobals(
		func(text string) { app.appendLog(text) },
		&app.messages,
		func(parent fyne.Window, header, msg string, opts ...string) int {
			return app.showChoiceDialogSync(parent, header, msg, opts...)
		},
		func(link string) {
			fyne.Do(func() { u, _ := url.Parse(link); app.myApp.OpenURL(u) })
		},
		app.cfg.ModsPath,
		func(modName string) bool {
			return app.isModActive(modName)
		},
		func() { fyne.Do(app.refreshModList) },
		func(key string) int64 {
			if info, ok := app.getCachedVersion(key); ok {
				return info.InstalledAt
			}
			return 0
		},
	)

	// 3. Настроить sorter с функциями checks
	sorter.SetFolderExistsFunc(checks.FolderExists)
	sorter.SetListModFoldersFunc(checks.ListModFolders)
	sorter.SetLogFunc(func(text string) { app.appendLog(text) })
	sorter.SetSortMessages(app.messages["sort_ru_warning"], app.messages["sort_en_warning"])
	sorter.SetHeaderFunc(checks.WriteLoadOrderHeader)
	// sorter.SetLoadOrderOutputPath(filepath.Join(app.cfg.ModsPath, FileNameLoadOrder))
	sorter.SetLogMessages(app.messages["log_create_mlot"], app.messages["log_mlot_created"])

	// 4. Загрузить внешние списки (один раз)
	if err := checks.LoadExternalLists(FileNameMandatoryRules); err != nil {
		app.appendLog(app.messages["log_warn_moid_not_found"] + ": " + err.Error())
	} else {
		app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
		saveConfig(app.cfg)
		app.appendLog(app.messages["log_succ_moid_found"])
	}
	sorter.SetMandatoryOrder(checks.MandatoryOrder)
	sorter.SetDependencies(convertDeps(checks.Dependencies))
	sorter.SetLoadOrderRules(checks.LoadOrderRules)

	// 5. Загрузить mod_database
	if err := app.loadModDatabase(FileNameModDatabase); err != nil {
		app.modDatabase = []checks.ModDBEntry{}
		app.appendLog(app.messages["log_mod_db_missing"] + ": " + err.Error())
		app.cfg.LastModDatabaseVersion = ""
	}
	checks.SetModDatabase(app.modDatabase)

	// 6. Синхронизировать кэш и записать версии в лог
	app.syncVersionCache()
	if app.logFile != nil {
		fmt.Fprintf(app.logFile, "Program version: %s\n", AppVersion)
		fmt.Fprintf(app.logFile, "mandatory_obsolete_incompatible_dependencies.json version: %s\n", checks.GetExternalVersion())
		fmt.Fprintf(app.logFile, "mod_database.json version: %s\n", app.cfg.LastModDatabaseVersion)
	}
	sorter.LoadSortOrders(checks.GlobalDataDir())

	// 7. Инициализация лаунчера
	SetLauncherMessages(
		app.messages["launcher_ver_unknown"],
		app.messages["launcher_exe_not_found"],
		app.messages["launcher_root_not_found"],
	)
	SetLinuxLauncherMessages(
		app.messages["linux_wine_not_found"],
		app.messages["linux_xbox_not_supported"],
	)
	app.launchGameFunc = launchGame

	app.syncModsEnabledState()

	// 8. ВСЕ ОПЕРАЦИИ С UI В ГЛАВНОМ ПОТОКЕ
	fyne.Do(func() {
		app.reloadAfterPathChange()

		if app.btnToggle != nil {
			app.updateToggleButtonText(app.btnToggle)
		}

		if !app.cfg.InitialSetupDone {
			app.performFirstRunSetup()
		}

		// Проверка AML (если ещё не проверяли)
		if !app.pathsInitialized {
			app.pathsInitialized = true
			app.amlDetected = checks.IsAMLInstalled(app.cfg.ModsPath)
			if app.amlDetected && !app.cfg.SuppressAMLWarning {
				app.showChoiceDialogAsync(
					app.mainWindow,
					app.messages["aml_detected_title"],
					app.messages["aml_detected_warning"],
					func(choice int) {
						switch choice {
						case 0:
							if u, err := url.Parse(DarktideModDML); err == nil {
								app.myApp.OpenURL(u)
							}
						case 2:
							app.cfg.SuppressAMLWarning = true
							saveConfig(app.cfg)
						}
					},
					app.messages["btn_open_dml_page"],
					app.messages["btn_continue"],
					app.messages["btn_dont_show_again"],
				)
			}
		}
	})

	// 9. Проверка специальных обновлений (один раз)
	if app.cfg.UpdateCheckFrequency != "never" && app.shouldCheckUpdates() {
		go app.checkSpecialUpdates()
	}

	// 10. Регистрация nxm и запуск слушателя
	if exePath, err := os.Executable(); err == nil {
		registerNXMProtocol(exePath)
	}

	if app.nxmListener == nil {
		listener, err := net.Listen(NXMProtocol, NXMAddress)
		if err == nil {
			app.nxmListener = listener
			go func() {
				for {
					if app.nxmListener == nil {
						return
					}
					conn, err := app.nxmListener.Accept()
					if err != nil {
						return
					}
					link, _ := bufio.NewReader(conn).ReadString('\n')
					conn.Close()
					fyne.Do(func() {
						app.handleNXMLink(strings.TrimSpace(link))
					})
				}
			}()
		}
	}
}

func (app *App) isLoggedIn() bool {
	_, err := keyring.Get(keyringService, "access_token")
	return err == nil
}

func (app *App) getCachedVersion(key string) (ModVersionInfo, bool) {
	app.cacheMutex.RLock()
	defer app.cacheMutex.RUnlock()
	v, ok := app.nexusVersionCache[key]
	return v, ok
}

func (app *App) setCachedVersion(key string, info ModVersionInfo) {
	app.cacheMutex.Lock()
	defer app.cacheMutex.Unlock()
	app.nexusVersionCache[key] = info
}

func (app *App) getLatestVersion(key string) (string, bool) {
	app.latestMutex.RLock()
	defer app.latestMutex.RUnlock()
	v, ok := app.nexusLatestVersions[key]
	return v, ok
}

func (app *App) setLatestVersion(key string, version string) {
	app.latestMutex.Lock()
	defer app.latestMutex.Unlock()
	app.nexusLatestVersions[key] = version
}

func (app *App) getChangelog(modID, fileID int) string {
	key := fmt.Sprintf("%d:%d", modID, fileID)
	app.changelogMutex.RLock()
	if cached, ok := app.changelogCache[key]; ok {
		app.changelogMutex.RUnlock()
		return cached
	}
	app.changelogMutex.RUnlock()

	changelog, err := app.FetchChangelog(modID, fileID)
	if err != nil {
		return "Changelog is unavailable"
	}
	clean := stripHTML(changelog)

	app.changelogMutex.Lock()
	app.changelogCache[key] = clean
	app.changelogMutex.Unlock()
	return clean
}

// profilesDir возвращает путь к папке профилей
func (app *App) profilesDir() string {
	return filepath.Join(filepath.Dir(configFilePath()), "profiles")
}

// profilePath возвращает путь к папке конкретного профиля
func (app *App) profilePath(name string) string {
	return filepath.Join(app.profilesDir(), name)
}

// activeProfilePath возвращает путь к текущему активному профилю
func (app *App) activeProfilePath() string {
	return app.profilePath(app.cfg.ActiveProfile)
}

// ensureProfileDir создаёт папку профиля, если её нет
func (app *App) ensureProfileDir(name string) error {
	path := app.profilePath(name)
	return os.MkdirAll(path, 0755)
}

// profileExists проверяет, существует ли профиль с таким именем
func (app *App) profileExists(name string) bool {
	path := app.profilePath(name)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// initProfiles инициализирует профили, создаёт Default при необходимости
func (app *App) initProfiles() {
	// 1. Создаём папку profiles, если нет
	if err := os.MkdirAll(app.profilesDir(), 0755); err != nil {
		app.appendLog(fmt.Sprintf("Failed to create profiles directory: %v", err))
		return
	}

	// 2. Сканируем подпапки в profilesDir
	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLog(fmt.Sprintf("Failed to read profiles directory: %v", err))
		return
	}

	var profileNames []string
	for _, e := range entries {
		if e.IsDir() {
			profileNames = append(profileNames, e.Name())
		}
	}

	// 3. Если профилей нет, создаём Default
	if len(profileNames) == 0 {
		app.appendLog("No profiles found. Creating default profile...")
		if err := app.createDefaultProfile(); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create default profile: %v", err))
			return
		}
		profileNames = append(profileNames, "Default")
		app.cfg.ActiveProfile = "Default"
		saveConfig(app.cfg)
	}

	// 4. Проверяем, что активный профиль существует
	if app.cfg.ActiveProfile == "" || !app.profileExists(app.cfg.ActiveProfile) {
		app.cfg.ActiveProfile = profileNames[0]
		saveConfig(app.cfg)
	}

	// 5. Удаляем возможный файл mod_load_order.txt из корня активного профиля
	app.cleanupProfileRoot(app.profilePath(app.cfg.ActiveProfile))

	// 6. Устанавливаем путь к данным профиля в checks
	checks.SetProfileDataDir(app.activeProfilePath())
	app.appendLog(fmt.Sprintf("Active profile: %s", app.cfg.ActiveProfile))

	// 7. Обновляем путь для сортировщика
	app.updateSorterOutputPath()

	// 8. Обновляем UI в главной горутине
	fyne.Do(func() {
		app.refreshProfileList()
		app.refreshModList()
		app.filterModList()
		app.forceRefreshTable()
	})
}

// createDefaultProfile создаёт профиль Default из текущей игровой папки mods
func (app *App) createDefaultProfile() error {
	defaultPath := app.profilePath("Default")
	if err := os.MkdirAll(defaultPath, 0755); err != nil {
		return err
	}

	// Копируем папку mods
	modsSrc := app.cfg.ModsPath
	modsDst := filepath.Join(defaultPath, "mods")
	if err := copyPath(modsSrc, modsDst); err != nil {
		return fmt.Errorf("failed to copy mods folder to profile: %w", err)
	}
	cleanupModsFolder(modsDst)

	// Копируем mod_load_order.txt, если есть
	srcLO := filepath.Join(modsSrc, FileNameLoadOrder)
	dstLO := filepath.Join(modsDst, FileNameLoadOrder)
	if _, err := os.Stat(srcLO); err == nil {
		if err := copyFile(srcLO, dstLO); err != nil {
			app.appendLog(fmt.Sprintf("Failed to copy load order: %v", err))
		}
	}

	// Копируем nexus_versions.json, если есть
	srcNV := filepath.Join(filepath.Dir(configFilePath()), FileNameNexusVersions)
	dstNV := filepath.Join(defaultPath, FileNameNexusVersions)
	if _, err := os.Stat(srcNV); err == nil {
		if err := copyFile(srcNV, dstNV); err != nil {
			app.appendLog(fmt.Sprintf("Failed to copy nexus versions: %v", err))
		}
	}

	return nil
}

// setGlobalDataDir устанавливает путь к глобальным данным (mod_database.json, mandatory_...)
func (app *App) setGlobalDataDir() {
	exePath, err := os.Executable()
	if err != nil {
		app.appendLog("Failed to get executable path: " + err.Error())
		return
	}
	globalDir := filepath.Dir(exePath)
	checks.SetGlobalDataDir(globalDir)
	app.appendLog(fmt.Sprintf("Global data directory set to: %s", globalDir))
}

// migrateGlobalFilesFromMods проверяет, лежат ли глобальные файлы в папке mods, и предлагает переместить их.
func (app *App) migrateGlobalFilesFromMods() {
	modsPath := app.cfg.ModsPath
	exePath := app.getExePath()
	exeDir := filepath.Dir(exePath)

	hasModDB := fileExists(filepath.Join(modsPath, FileNameModDatabase))
	hasMandatory := fileExists(filepath.Join(modsPath, FileNameMandatoryRules))

	if !hasModDB && !hasMandatory {
		return
	}

	// Если программа уже не внутри mods — просто копируем файлы в папку программы
	if !strings.HasPrefix(exeDir, modsPath) {
		globalDir := exeDir
		needCopy := false
		if hasModDB && !fileExists(filepath.Join(globalDir, FileNameModDatabase)) {
			needCopy = true
		}
		if hasMandatory && !fileExists(filepath.Join(globalDir, FileNameMandatoryRules)) {
			needCopy = true
		}
		if !needCopy {
			// Файлы уже есть в папке программы — просто удаляем дубли из mods
			if hasModDB {
				os.Remove(filepath.Join(modsPath, FileNameModDatabase))
			}
			if hasMandatory {
				os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
			}
			app.appendLog("Global files already exist in program directory. Duplicates removed from mods.")
			return
		}

		choice := app.showChoiceDialogSync(app.mainWindow,
			app.messages["migration_title"],
			app.messages["migration_message"],
			app.messages["migration_btn_move"],
			app.messages["btn_cancel"],
		)
		if choice != 0 {
			app.appendLog("Migration cancelled.")
			return
		}

		// Копируем файлы
		if hasModDB {
			src := filepath.Join(modsPath, FileNameModDatabase)
			dst := filepath.Join(globalDir, FileNameModDatabase)
			if err := copyFile(src, dst); err != nil {
				app.appendLog(fmt.Sprintf("Failed to copy %s: %v", FileNameModDatabase, err))
				return
			}
			app.appendLog(fmt.Sprintf("Copied %s to %s", FileNameModDatabase, globalDir))
		}
		if hasMandatory {
			src := filepath.Join(modsPath, FileNameMandatoryRules)
			dst := filepath.Join(globalDir, FileNameMandatoryRules)
			if err := copyFile(src, dst); err != nil {
				app.appendLog(fmt.Sprintf("Failed to copy %s: %v", FileNameMandatoryRules, err))
				return
			}
			app.appendLog(fmt.Sprintf("Copied %s to %s", FileNameMandatoryRules, globalDir))
		}

		// После копирования основных файлов добавьте:
		sortFiles := []string{"russian_sort_order.txt", "english_sort_order.txt"}
		for _, sf := range sortFiles {
			src := filepath.Join(modsPath, sf)
			if fileExists(src) {
				dst := filepath.Join(globalDir, sf)
				if err := copyFile(src, dst); err != nil {
					app.appendLog(fmt.Sprintf("Failed to copy %s: %v", sf, err))
				} else {
					os.Remove(src)
					app.appendLog(fmt.Sprintf("Copied %s to %s", sf, globalDir))
				}
			}
		}

		// Удаляем исходные файлы из mods
		if hasModDB {
			os.Remove(filepath.Join(modsPath, FileNameModDatabase))
		}
		if hasMandatory {
			os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
		}

		// Перезагружаем базы
		app.setGlobalDataDir()
		if err := app.loadModDatabase(FileNameModDatabase); err == nil {
			checks.SetModDatabase(app.modDatabase)
		}
		if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
			app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
			saveConfig(app.cfg)
		}
		sorter.SetMandatoryOrder(checks.MandatoryOrder)
		sorter.SetDependencies(convertDeps(checks.Dependencies))
		sorter.SetLoadOrderRules(checks.LoadOrderRules)
		app.appendLog("Global files migration completed successfully.")
		return
	}

	// Если программа находится внутри папки mods — предлагаем скопировать программу и файлы в новое место
	app.appendLog("Program is located inside the mods folder. Please choose a new location for the program and global files.")

	// Показываем диалог выбора папки (без установки заголовка)
	resultChan := make(chan string, 1)
	fyne.Do(func() {
		dlg := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				resultChan <- ""
				return
			}
			resultChan <- filepath.FromSlash(uri.Path())
		}, app.mainWindow)
		dlg.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
		dlg.Show()
	})

	selectedPath := <-resultChan
	if selectedPath == "" {
		app.appendLog("Migration cancelled (no folder selected).")
		app.showInfoDialog(app.messages["warning_title"], "Migration cancelled. Program and files remain in the mods folder.")
		return
	}

	// Создаём подпапку Servo-Modquisitor
	targetDir := filepath.Join(selectedPath, "Servo-Modquisitor")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		app.appendLog(fmt.Sprintf("Failed to create target directory: %v", err))
		app.showInfoDialog(app.messages["error_title"], fmt.Sprintf("Failed to create folder: %v", err))
		return
	}

	// Копируем программу (exe)
	exeName := filepath.Base(exePath)
	targetExe := filepath.Join(targetDir, exeName)
	if err := copyFile(exePath, targetExe); err != nil {
		app.appendLog(fmt.Sprintf("Failed to copy program: %v", err))
		app.showInfoDialog(app.messages["error_title"], fmt.Sprintf("Failed to copy program: %v", err))
		return
	}
	app.appendLog(fmt.Sprintf("Program copied to %s", targetExe))

	// Копируем файлы
	if hasModDB {
		src := filepath.Join(modsPath, FileNameModDatabase)
		dst := filepath.Join(targetDir, FileNameModDatabase)
		if err := copyFile(src, dst); err != nil {
			app.appendLog(fmt.Sprintf("Failed to copy %s: %v", FileNameModDatabase, err))
			return
		}
		app.appendLog(fmt.Sprintf("Copied %s to %s", FileNameModDatabase, targetDir))
	}
	if hasMandatory {
		src := filepath.Join(modsPath, FileNameMandatoryRules)
		dst := filepath.Join(targetDir, FileNameMandatoryRules)
		if err := copyFile(src, dst); err != nil {
			app.appendLog(fmt.Sprintf("Failed to copy %s: %v", FileNameMandatoryRules, err))
			return
		}
		app.appendLog(fmt.Sprintf("Copied %s to %s", FileNameMandatoryRules, targetDir))
	}

	// Удаляем исходные файлы из mods
	if hasModDB {
		os.Remove(filepath.Join(modsPath, FileNameModDatabase))
	}
	if hasMandatory {
		os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
	}

	// Показываем сообщение об успехе с инструкцией по удалению старой папки
	msg := fmt.Sprintf(
		"Program and global files have been copied to:\n%s\n\n"+
			"Please close this program and run the new copy from the new location.\n"+
			"After that, you can safely delete the old program folder:\n%s",
		targetDir, exeDir,
	)
	app.showInfoDialog(app.messages["migration_success_title"], msg)
	app.appendLog("Program and files copied to new location. Please restart from the new copy.")
}

// refreshProfileList обновляет список профилей и выпадающий список
func (app *App) refreshProfileList() {
	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLog(fmt.Sprintf("Failed to read profiles: %v", err))
		return
	}
	names := []string{}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if app.profileSelect != nil {
		app.profileSelect.Options = names
		if app.cfg.ActiveProfile != "" {
			app.profileSelect.SetSelected(app.cfg.ActiveProfile)
		} else if len(names) > 0 {
			app.profileSelect.SetSelected(names[0])
		}
		app.profileSelect.Refresh()
	}
}

// switchProfile переключает активный профиль
func (app *App) switchProfile(name string) {
	if name == app.cfg.ActiveProfile {
		return
	}
	if !app.profileExists(name) {
		app.appendLog(fmt.Sprintf("Profile '%s' does not exist", name))
		return
	}

	// Сохраняем изменения текущего профиля, если есть
	if app.orderDirty {
		app.saveCurrentOrder() // теперь внутри вызовет syncLoadOrderToGame
		app.orderDirty = false
		app.stopBlinkSaveButton()
		app.updateTableBorder()
	}

	srcMods := filepath.Join(app.profilePath(name), "mods")
	dstMods := app.cfg.ModsPath

	// Удаляем пользовательские моды из игровой папки
	if _, err := os.Stat(dstMods); err == nil {
		entries, _ := os.ReadDir(dstMods)
		for _, e := range entries {
			if e.IsDir() {
				modName := e.Name()
				if modName != "base" && modName != "dmf" && modName != "autopatch" {
					os.RemoveAll(filepath.Join(dstMods, modName))
				}
			}
		}
	}

	// Копируем папку mods из профиля в игровую папку
	if err := copyPath(srcMods, dstMods); err != nil {
		app.appendLog(fmt.Sprintf("Failed to copy mods from profile '%s': %v", name, err))
		return
	}
	cleanupModsFolder(dstMods)

	// Удаляем возможный файл mod_load_order.txt в корне профиля (если завалялся)
	app.cleanupProfileRoot(app.profilePath(name))

	app.cfg.ActiveProfile = name
	saveConfig(app.cfg)
	checks.SetProfileDataDir(app.activeProfilePath())
	app.loadNexusVersionCache()

	// Обновляем путь для сортировщика ПЕРЕД сохранением порядка
	app.updateSorterOutputPath()

	// Все UI-операции — в главной горутине
	fyne.Do(func() {
		app.refreshModList()
		app.saveCurrentOrder() // сохранит порядок и синхронизирует с игрой
		app.orderDirty = false
		app.stopBlinkSaveButton()
		app.updateTableBorder()
		app.filterModList()
		app.forceRefreshTable()

		app.appendLog(fmt.Sprintf("Switched to profile: %s", name))
		if app.profileSelect != nil {
			app.profileSelect.SetSelected(name)
		}
	})
}

// createProfile создаёт новый профиль
func (app *App) createProfile(name string, copyFrom string) error {
	if name == "" {
		return fmt.Errorf("profile name cannot be empty")
	}
	if app.profileExists(name) {
		return fmt.Errorf("profile '%s' already exists", name)
	}
	if err := app.ensureProfileDir(name); err != nil {
		return err
	}

	if copyFrom != "" && app.profileExists(copyFrom) {
		src := app.profilePath(copyFrom)
		dst := app.profilePath(name)
		if err := copyPath(src, dst); err != nil {
			return fmt.Errorf("failed to copy from '%s': %w", copyFrom, err)
		}
		// После копирования удаляем возможный файл в корне (если завалялся)
		app.cleanupProfileRoot(app.profilePath(name))
	} else {
		// Пустой профиль
		modsPath := filepath.Join(app.profilePath(name), "mods")
		if err := os.MkdirAll(modsPath, 0755); err != nil {
			return err
		}
		// Создаём пустой файл порядка внутри mods
		emptyPath := filepath.Join(modsPath, FileNameLoadOrder)
		if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create empty load order: %v", err))
		}
		// Создаём пустой nexus_versions.json
		emptyCache := map[string]ModVersionInfo{}
		data, _ := json.MarshalIndent(emptyCache, "", "\t")
		if err := os.WriteFile(filepath.Join(app.profilePath(name), FileNameNexusVersions), data, 0644); err != nil {
			app.appendLog(fmt.Sprintf("Failed to create empty nexus_versions.json: %v", err))
		}
	}

	app.appendLog(fmt.Sprintf("Created profile: %s", name))
	fyne.Do(func() {
		app.refreshProfileList()
	})
	return nil
}

// cleanupProfileRoot удаляет файл mod_load_order.txt из корня профиля, если он существует.
func (app *App) cleanupProfileRoot(profilePath string) {
	path := filepath.Join(profilePath, FileNameLoadOrder)
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			app.appendLog(fmt.Sprintf("Failed to remove legacy load order file from profile root: %v", err))
		} else {
			app.appendLog(fmt.Sprintf("Removed legacy load order file from profile root: %s", path))
		}
	}
}

// deleteProfile удаляет профиль
func (app *App) deleteProfile(name string) error {
	if name == "Default" {
		return fmt.Errorf("cannot delete default profile")
	}
	if !app.profileExists(name) {
		return fmt.Errorf("profile '%s' does not exist", name)
	}
	if name == app.cfg.ActiveProfile {
		return fmt.Errorf("cannot delete active profile")
	}
	if err := os.RemoveAll(app.profilePath(name)); err != nil {
		return err
	}
	app.appendLog(fmt.Sprintf("Deleted profile: %s", name))
	app.refreshProfileList()
	return nil
}

// renameProfile переименовывает профиль
func (app *App) renameProfile(oldName, newName string) error {
	if oldName == "Default" {
		return fmt.Errorf("cannot rename default profile")
	}
	if !app.profileExists(oldName) {
		return fmt.Errorf("profile '%s' does not exist", oldName)
	}
	if app.profileExists(newName) {
		return fmt.Errorf("profile '%s' already exists", newName)
	}
	oldPath := app.profilePath(oldName)
	newPath := app.profilePath(newName)
	if err := os.Rename(oldPath, newPath); err != nil {
		return err
	}
	if app.cfg.ActiveProfile == oldName {
		app.cfg.ActiveProfile = newName
		saveConfig(app.cfg)
	}
	app.appendLog(fmt.Sprintf("Renamed profile: %s -> %s", oldName, newName))
	app.refreshProfileList()
	return nil
}

// fileExists проверяет существование файла или папки
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// syncProfileFromGame копирует текущую игровую папку mods в активный профиль (перезаписывая)
func (app *App) syncProfileFromGame() {
	profilePath := app.activeProfilePath()
	if profilePath == "" {
		app.appendLog("No active profile to sync")
		return
	}

	dstMods := filepath.Join(profilePath, "mods")
	src := app.cfg.ModsPath

	// Удаляем старые папки обычных модов в профиле
	if _, err := os.Stat(dstMods); err == nil {
		entries, err := os.ReadDir(dstMods)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					name := e.Name()
					if name == "base" || name == "dmf" || name == "autopatch" {
						continue
					}
					os.RemoveAll(filepath.Join(dstMods, name))
				}
			}
		}
	}

	// Копируем свежую папку mods из игры в профиль
	if err := copyPath(src, dstMods); err != nil {
		app.appendLog(fmt.Sprintf("Failed to sync mods to profile: %v", err))
		return
	}

	cleanupModsFolder(dstMods)
	app.appendLog("Profile synced from game folder.")
}

func (app *App) getExePath() string {
	exe, _ := os.Executable()
	return exe
}

func cleanupModsFolder(path string) {
	unwanted := []string{
		FileNameModDatabase,
		FileNameMandatoryRules,
		// FileNameLog,
		AppName + ".exe",
	}
	for _, f := range unwanted {
		toRemove := filepath.Join(path, f)
		if _, err := os.Stat(toRemove); err == nil {
			if err := os.Remove(toRemove); err != nil {
				// логировать ошибку, но не прерывать выполнение
			}
		}
	}
}

// updateSorterOutputPath устанавливает путь для записи порядка загрузки в папку активного профиля.
func (app *App) updateSorterOutputPath() {
	if app.cfg.ActiveProfile != "" {
		sorter.SetLoadOrderOutputPath(filepath.Join(app.activeProfilePath(), "mods", FileNameLoadOrder))
	} else {
		sorter.SetLoadOrderOutputPath(filepath.Join(app.cfg.ModsPath, FileNameLoadOrder))
	}
}

// syncLoadOrderToGame копирует файл порядка из профиля в игровую папку.
func (app *App) syncLoadOrderToGame() {
	src := filepath.Join(app.activeProfilePath(), "mods", FileNameLoadOrder)
	dst := filepath.Join(app.cfg.ModsPath, FileNameLoadOrder)
	if err := copyFile(src, dst); err != nil {
		app.appendLog(fmt.Sprintf("Failed to copy load order to game folder: %v", err))
	} else {
		app.appendLog("Load order synced to game folder")
	}
}
