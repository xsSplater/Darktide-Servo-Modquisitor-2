// app.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

type Config struct {
	Language					string	`json:"language"`
	Theme						string	`json:"theme"`
	ModsGloballyEnabled			bool	`json:"mods_globally_enabled"`
	InitialSetupDone			bool	`json:"initial_setup_done"`
	DateFormat					string	`json:"date_format"`
	ForceEnglishModNames		bool	`json:"force_english_mod_names"`
	ModsPath					string	`json:"mods_path"`
	WindowWidth					int		`json:"window_width"`
	WindowHeight				int		`json:"window_height"`
	WindowMaximized				bool	`json:"window_maximized"`
	LastModDatabaseVersion		string	`json:"last_mod_database_version"`
	LastMandatoryRulesVersion	string	`json:"last_mandatory_rules_version"`
	LastUpdateCheck				string	`json:"last_update_check"`
	SkipSortFilesPrompt			bool	`json:"skip_sort_files_prompt"`
	UpdateCheckFrequency		string	`json:"update_check_frequency"`
	ShowSystemMods				bool	`json:"show_system_mods"`
}

const (
	modDatabaseURL  = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mod_database.json"
	modMandatoryURL = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mandatory_obsolete_incompatible_dependencies.json"
)

type App struct {
	cfg							*Config
	mainWindow					fyne.Window
	myApp						fyne.App

	// логирование
	logFile						*os.File
	logContainer				*fyne.Container
	consoleScroll				*container.Scroll

	// модели
	allMods						[]checks.ModInfo
	displayedMods				[]checks.ModInfo
	systemMods					[]checks.ModInfo   // base и dmf
	systemModsTableContainer	*fyne.Container
	selectedModName				string
	selectedModIndex			atomic.Int32
	orderDirty					bool

	// рамка вокруг таблицы
	tableBorder					*canvas.Rectangle
	tableBorderContainer		*fyne.Container
	blinkSaveOrderActive		bool
	blinkCheckSortActive		bool

	// Управление видимостью панели управления модами
	managePanel					*fyne.Container
	showSelectColumn			bool
	selectColumnBgRes fyne.Resource

	moveLabel					*widget.Label
	statusLabel					*widget.Label

	tooltipStatus				*TooltipStatusManager
	manageBtn					*CustomButton
	selectAllBtn				*CustomButton
	deselectAllBtn				*CustomButton
	enableSelectedBtn			*CustomButton
	disableSelectedBtn			*CustomButton
	enableAllBtn				*CustomButton
	disableAllBtn				*CustomButton
	moveToTopBtn				*CustomButton
	moveToBottomBtn				*CustomButton
	btnToggle					*CustomButton
	btnSaveOrder				*CustomButton
	btnRefresh					*CustomButton
	btnInstall					*CustomButton
	btnRemove					*CustomButton
	btnUp, btnDown				*CustomButton
	btnLaunchNormal				*CustomButton
	btnLaunchNoLauncher			*CustomButton
	btnSortChecks				*CustomButton
	searchClearBtn				*CustomButton

	moveToEntry					*widget.Entry
	searchEntry					*widget.Entry

	descTitle					*widget.Label
	descAuthor					*widget.Label
	descInstalled				*widget.Label
	descBody					*widget.Label
	descURL						*widget.Hyperlink
    githubLink					*widget.Hyperlink
	filterLabel					*widget.Label
	counterLabel				*widget.Label

	logHeaderText				*canvas.Text
	logWindow					*widget.RichText

	filterSelect				*widget.Select
	modTable					*widget.Table
	headerTable					*widget.Table
	systemModsTable				*widget.Table // таблица системных модов

	mainSplit					*container.Split
	rightSplit					*container.Split

	messages					map[string]string
	modDatabase					[]checks.ModDBEntry

	// ссылки на динамически окрашиваемые объекты
	screenBgRect				*canvas.Rectangle
	headerBoxBgRect				*canvas.Rectangle
	tipBgRect					*canvas.Rectangle
	topPanelBgRect				*canvas.Rectangle
	managePanelBgRect			*canvas.Rectangle
	descCardBgRect				*canvas.Rectangle

	gameRoot					string
	patcherType					PatcherType
	launchGameFunc				func(version GameVersion, gameRoot string, skipLauncher bool) error
}

func NewApp(cfg *Config, myApp fyne.App) *App {
	app := &App{
		cfg:		cfg,
		messages:	map[string]string{},
		myApp:		myApp,
	}
	app.selectedModIndex.Store(-1)
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
		c := &Config{Language: "en", Theme: "dark", DateFormat: "dd-mm-yyyy", UpdateCheckFrequency: "every_start", ShowSystemMods: true}
		return c
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		c = Config{Language: "en", Theme: "dark", DateFormat: "dd-mm-yyyy", UpdateCheckFrequency: "every_start", ShowSystemMods: true}
		return &c
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

type ModDatabaseFile struct {
	Version string				`json:"version"`
	Mods	[]checks.ModDBEntry `json:"mod_database"`
}

func (app *App) loadModDatabase(filename string) error {
	data, err := os.ReadFile(filepath.Join(app.cfg.ModsPath, filename))
	if err != nil {
		return err
	}
	var container ModDatabaseFile
	if err := json.Unmarshal(data, &container); err != nil {
		return err
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

// ---- Новые методы для обновлений ----
func (app *App) downloadSortFiles() error {
	files := []struct {
		remote string
		local  string
	}{
		{modDatabaseURL, FileNameModDatabase},
		{modMandatoryURL, FileNameMandatoryRules},
	}
	for _, f := range files {
		dest := filepath.Join(app.cfg.ModsPath, f.local)
		if err := app.downloadFile(f.remote, dest); err != nil {
			return fmt.Errorf("%s: %w", f.local, err)
		}
	}
	return nil
}

func (app *App) ensureSortFiles() {
	missing := false
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameMandatoryRules)); os.IsNotExist(err) {
		missing = true
	}
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameModDatabase)); os.IsNotExist(err) {
		missing = true
	}
	if !missing {
		return
	}

	app.appendLog(app.messages["sort_files_missing_short"])

	if app.cfg.SkipSortFilesPrompt {
		app.appendLog(app.messages["download_skip_forever"])
		return
	}

	choice := app.showChoiceDialog(
		app.mainWindow,
		app.messages["sort_files_missing"],
		app.messages["download_sort_files_question"],
		app.messages["yes"],
		app.messages["skip"],
		app.messages["download_skip_forever"],
	)
	switch choice {
	case 0: // Да – скачать
		if err := app.downloadSortFiles(); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
			dialog.ShowInformation(app.messages["sort_files_missing"], fmt.Sprintf(app.messages["download_failed"], err), app.mainWindow)
		} else {
			app.appendLog(app.messages["sort_files_updated"])
			// Перезагрузить базы
			if err := app.loadModDatabase(FileNameModDatabase); err == nil {
				checks.SetModDatabase(app.modDatabase)
			}
			if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
				app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
				saveConfig(app.cfg)
			}
			app.refreshModList()
		}
	case 2: // Пропустить и больше не спрашивать
		app.cfg.SkipSortFilesPrompt = true
		saveConfig(app.cfg)
		fallthrough
	case 1: // Пропустить
		app.appendLog(app.messages["download_skipped"])
	}
}

func (app *App) shouldCheckUpdates() bool {
	if app.cfg.UpdateCheckFrequency == "never" {
		return false
	}
	if app.cfg.UpdateCheckFrequency == "every_start" {
		return true
	}
	if app.cfg.LastUpdateCheck == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, app.cfg.LastUpdateCheck)
	if err != nil {
		return true
	}
	now := time.Now()
	switch app.cfg.UpdateCheckFrequency {
	case "weekly":
		return now.Sub(last) >= 7*24*time.Hour
	case "monthly":
		return now.After(last.AddDate(0, 1, 0))
	case "yearly":
		return now.After(last.AddDate(1, 0, 0))
	}
	return false
}

func (app *App) downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(dest)
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}
