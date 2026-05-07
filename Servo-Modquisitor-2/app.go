package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// Config – настройки приложения
type Config struct {
	Language             string `json:"language"`
	Theme                string `json:"theme"`
	ModsGloballyEnabled  bool   `json:"mods_globally_enabled"`
	InitialSetupDone     bool   `json:"initial_setup_done"`
	DateFormat           string `json:"date_format"`
	ForceEnglishModNames bool   `json:"force_english_mod_names"`
	ModsPath             string `json:"mods_path"`
	WindowWidth          int    `json:"window_width"`
	WindowHeight         int    `json:"window_height"`
}

// App объединяет всё состояние
type App struct {
	cfg        *Config
	mainWindow fyne.Window
	myApp      fyne.App

	// логирование
	logWindow     *widget.Entry
	logFile       *os.File
	logContainer  *fyne.Container
	consoleScroll *container.Scroll

	// модели
	allMods         []checks.ModInfo
	displayedMods   []checks.ModInfo
	selectedModName string
	orderDirty      bool

	// виджеты
	modTable         *widget.Table
	descTitle        *widget.Label
	descAuthor       *widget.Label
	descBody         *widget.Label
	descURL          *widget.Hyperlink
	searchEntry      *widget.Entry
	filterSelect     *widget.Select
	btnToggle        *widget.Button
	btnUp, btnDown   *widget.Button
	modListTitle     *widget.Label
	filterLabel      *widget.Label
	btnSaveOrder     *widget.Button
	btnSortChecks    *widget.Button
	btnRefresh       *widget.Button
	btnInstall       *widget.Button
	btnInstallFolder *widget.Button
	btnRemove        *widget.Button
	btnExport        *widget.Button
	btnImport        *widget.Button

	messages    map[string]string
	modDatabase []checks.ModDBEntry

	gameRoot      string
	patcherType   PatcherType
	launchGameFunc func(version GameVersion, gameRoot string, skipLauncher bool) error
}

func NewApp(cfg *Config, myApp fyne.App) *App {
	app := &App{
		cfg:      cfg,
		messages: map[string]string{},
		myApp:    myApp,
	}
	app.loadLanguage(cfg.Language)

	if cfg.Theme == "light" {
		myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
	} else {
		myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
	}

	app.gameRoot = getGameRoot()
	app.patcherType = detectPatcherType()

	return app
}

func configFilePath() string {
	dir, _ := os.UserConfigDir()
	appDir := filepath.Join(dir, "Servo-Modquisitor")
	os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "config.json")
}

func loadConfig() *Config {
	path := configFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		return &Config{Language: "en", Theme: "dark", DateFormat: "dd-mm-yyyy"}
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Config{Language: "en", Theme: "dark", DateFormat: "dd-mm-yyyy"}
	}
	return &c
}

func saveConfig(c *Config) {
	path := configFilePath()
	data, _ := json.MarshalIndent(c, "", "  ")
	os.WriteFile(path, data, 0644)
}

func (app *App) loadLanguage(lang string) error {
	data, err := embeddedFiles.ReadFile("lang/messages.json")
	if err != nil {
		return err
	}
	var raw map[string]map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
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
		return t.Format("2006-01-02")
	case "mm-dd-yyyy":
		return t.Format("01-02-2006")
	default:
		return t.Format("02-01-2006")
	}
}

func (app *App) loadModDatabase(filename string) error {
	data, err := os.ReadFile(filepath.Join(app.cfg.ModsPath, filename))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &app.modDatabase)
}

func convertDeps(deps []checks.Dependency) []sorter.ModDependency {
	out := make([]sorter.ModDependency, len(deps))
	for i, d := range deps {
		out[i] = sorter.ModDependency{Dependent: d.Dependent, Required: d.Required}
	}
	return out
}
