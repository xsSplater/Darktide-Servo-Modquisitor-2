// app.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

type Config struct {
	Language                  string `json:"language"`
	Theme                     string `json:"theme"`
	ModsGloballyEnabled       bool   `json:"mods_globally_enabled"`
	InitialSetupDone          bool   `json:"initial_setup_done"`
	DateFormat                string `json:"date_format"`
	ForceEnglishModNames      bool   `json:"force_english_mod_names"`
	ModsPath                  string `json:"mods_path"`
	WindowWidth               int    `json:"window_width"`
	WindowHeight              int    `json:"window_height"`
	WindowMaximized           bool   `json:"window_maximized"`
	LastModDatabaseVersion    string `json:"last_mod_database_version"`
	LastMandatoryRulesVersion string `json:"last_mandatory_rules_version"`
	LastUpdateCheck           string `json:"last_update_check"`
	SkipSortFilesPrompt       bool   `json:"skip_sort_files_prompt"`
	UpdateCheckFrequency      string `json:"update_check_frequency"`
	ShowSystemMods            bool   `json:"show_system_mods"`
	ShowModListAfterSort      bool   `json:"show_mod_list_after_sort"`
	SuppressAMLWarning        bool   `json:"suppress_aml_warning"` // скрывать предупреждение об AML

	// Nexus API
	NexusAPIKey       string    `json:"nexus_api_key"`
	OAuthAccessToken  string    `json:"oauth_access_token,omitempty"`
	OAuthRefreshToken string    `json:"oauth_refresh_token,omitempty"`
	OAuthExpiry       time.Time `json:"oauth_expiry,omitempty"`

	// Размер файла лога в байтах
	LogFileSizeLimit int64 `json:"log_file_size_limit"`
}

type ModVersionInfo struct {
	Timestamp int64  `json:"timestamp"`
	Version   string `json:"version"`
	Folder    string `json:"folder"` // Название папки мода в nexus_versions.json
}

type App struct {
	cfg        *Config
	mainWindow fyne.Window
	myApp      fyne.App

	// Логирование
	logFile       *os.File
	consoleScroll *container.Scroll

	// Модели
	allMods                  []checks.ModInfo
	displayedMods            []checks.ModInfo
	systemMods               []checks.ModInfo // base и dmf
	systemModsTableContainer *fyne.Container
	selectedModName          string
	selectedModIndex         atomic.Int32
	orderDirty               bool

	// Рамка вокруг таблицы
	tableBorder          *canvas.Rectangle
	tableBorderContainer *fyne.Container
	blinkSaveOrderActive bool
	amlDetected          bool

	// Управление видимостью панели управления модами
	managePanel       *fyne.Container
	showSelectColumn  bool
	selectColumnBgRes fyne.Resource

	moveLabel   *widget.Label
	statusLabel *widget.Label

	tooltipStatus *TooltipStatusManager

	manageBtn           *CustomButton
	selectAllBtn        *CustomButton
	deselectAllBtn      *CustomButton
	enableSelectedBtn   *CustomButton
	disableSelectedBtn  *CustomButton
	enableAllBtn        *CustomButton
	disableAllBtn       *CustomButton
	removeAllBtn        *CustomButton
	removeSelectedBtn   *CustomButton
	moveToTopBtn        *CustomButton
	moveToBottomBtn     *CustomButton
	btnToggle           *CustomButton
	btnSaveOrder        *CustomButton
	btnRefresh          *CustomButton
	btnInstall          *CustomButton
	btnRemove           *CustomButton
	btnUp               *CustomButton
	btnDown             *CustomButton
	btnLaunchNormal     *CustomButton
	btnLaunchNoLauncher *CustomButton
	btnSortChecks       *CustomButton
	btnUpdateAll        *CustomButton
	btnUpdateMod        *CustomButton
	btnCheckUpdates     *CustomButton
	searchClearBtn      *CustomButton

	// Nexus API
	oauthState     string
	oauthVerifier  string
	nxmListener    net.Listener // слушатель nxm-ссылок
	enrichDebounce *time.Timer
	lastNxmURL     string
	lastNxmTime    time.Time

	moveToEntry *widget.Entry
	searchEntry *widget.Entry

	descTitle     *canvas.Text
	descAuthor    *widget.Label
	descInstalled *widget.Label
	descBody      *widget.Label
	descURL       *widget.Hyperlink
	githubLink    *widget.Hyperlink
	filterLabel   *widget.Label
	counterLabel  *widget.Label

	descLocalVersion  *widget.Label
	descLatestVersion *widget.Label
	descConflict      *widget.Label // под descStatus

	logHeaderText *canvas.Text
	logWindow     *widget.RichText

	filterSelect    *widget.Select
	modTable        *widget.Table
	headerTable     *widget.Table
	systemModsTable *widget.Table // таблица системных модов

	messages    map[string]string
	modDatabase []checks.ModDBEntry

	// ссылки на динамически окрашиваемые объекты
	screenBgRect      *canvas.Rectangle
	headerBoxBgRect   *canvas.Rectangle
	tipBgRect         *canvas.Rectangle
	topPanelBgRect    *canvas.Rectangle
	managePanelBgRect *canvas.Rectangle
	descCardBgRect    *canvas.Rectangle

	nexusVersionCache   map[string]ModVersionInfo // локальная версия
	nexusLatestVersions map[string]string         // последняя версия с сайта

	gameRoot       string
	patcherType    PatcherType
	launchGameFunc func(version GameVersion, gameRoot string, skipLauncher bool) error
}

func NewApp(cfg *Config, myApp fyne.App) *App {
	app := &App{
		cfg:                 cfg,
		messages:            map[string]string{},
		myApp:               myApp,
		nexusVersionCache:   make(map[string]ModVersionInfo),
		nexusLatestVersions: make(map[string]string),
	}
	app.selectedModIndex.Store(-1)
	app.loadLanguage(cfg.Language)
	app.loadNexusVersionCache()

	if cfg.Theme == "light" {
		myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
	} else {
		myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
	}

	app.gameRoot = getGameRoot()
	app.patcherType = detectPatcherType()

	return app
}

func (app *App) loadNexusVersionCache() {
	path := filepath.Join(filepath.Dir(configFilePath()), FileNameNexusVersions)
	data, err := os.ReadFile(path)
	if err != nil {
		app.nexusVersionCache = make(map[string]ModVersionInfo)
		return
	}
	var raw map[string]ModVersionInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		// Пробуем старый формат (map[string]string) для миграции
		var old map[string]string
		if err2 := json.Unmarshal(data, &old); err2 == nil {
			raw = make(map[string]ModVersionInfo)
			for k, v := range old {
				ts, _ := strconv.ParseInt(v, 10, 64)
				raw[k] = ModVersionInfo{Timestamp: ts, Version: ""}
			}
		} else {
			app.nexusVersionCache = make(map[string]ModVersionInfo)
			return
		}
	}

	newCache := make(map[string]ModVersionInfo)
	for key, info := range raw {
		if !strings.Contains(key, ":") && info.Folder != "" {
			// Старый ключ: преобразуем в "modID:folder"
			newKey := key + ":" + info.Folder
			newCache[newKey] = info
		} else {
			newCache[key] = info
		}
	}
	app.nexusVersionCache = newCache
	if len(newCache) > 0 {
		app.saveNexusVersionCache() // сохраняем в новом формате
	}
}

func (app *App) saveNexusVersionCache() {
	path := filepath.Join(filepath.Dir(configFilePath()), FileNameNexusVersions)
	data, _ := json.MarshalIndent(app.nexusVersionCache, "", "  ")
	os.WriteFile(path, data, 0644)
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
	// Значение по умолчанию для новой настройки
	if !c.InitialSetupDone {
		c.ShowSystemMods = true
	}
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
	fullPath := filepath.Join(app.cfg.ModsPath, filename)
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
