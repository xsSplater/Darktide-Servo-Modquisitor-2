// Servo-Modquisitor-2/app.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"Servo-Modquisitor/themes"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"log"
	"net/url"
	"os"
	"path/filepath"
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
	CustomColors    map[string]color.NRGBA `json:"custom_colors"`
	CustomBaseTheme string                 `json:"custom_base_theme"` // "dark" | "light" | "highcontrast"

	InitialSetupDone          bool    `json:"initial_setup_done"`
	ForceEnglishModNames      bool    `json:"force_english_mod_names"`
	ModsGloballyEnabled       bool    `json:"mods_globally_enabled"`
	ShowSystemMods            bool    `json:"show_system_mods"`
	ShowModListAfterSort      bool    `json:"show_mod_list_after_sort"`
	SkipSortFilesPrompt       bool    `json:"skip_sort_files_prompt"`
	SuppressAMLWarning        bool    `json:"suppress_aml_warning"`
	WindowMaximized           bool    `json:"window_maximized"`
	FirstRunWizardDisabled    bool    `json:"first_run_wizard_disabled"`
	WizardHelpInstalled       bool    `json:"wizard_help_installed"`
	StatusRowSpacing          float32 `json:"status_row_spacing"`
	StatusFontSize            float32 `json:"status_font_size"`
	WindowHeight              int     `json:"window_height"`
	WindowWidth               int     `json:"window_width"`
	LogFileSizeLimit          int64   `json:"log_file_size_limit"`
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
	Folder      string `json:"folder"`
	Source      string `json:"source,omitempty"`
	InstalledAt int64  `json:"installed_at,omitempty"`
}

// App — корневой объект приложения.
//
// ПОРЯДОК ВЗЯТИЯ МЬЮТЕКСОВ (если нужно несколько одновременно):
//
//	cfgMutex → modsMutex → loadOrderMutex
//	gameRootMutex, pathsMutex — независимы (берутся по одному).
//
// Обратный порядок запрещён. Мьютексы кэшей (VersionCache.mu,
// ChangelogCache.mu, OAuthState.mu, NXMListener.mu) — локальные для
// своих сервисов, с чужими мьютексами не пересекаются.
//
// Инварианты по данным:
//
//   - versionCache соответствует cfg.ActiveProfile. Читать/писать
//     их парой можно только под cfgMutex + внутри VersionCache.
//   - Списки модов (allMods, displayedMods, systemMods, modDatabase)
//     вынесены в ModsState (см. mods_state.go); доступны через embed
//     как app.allMods и т.д. Порядок блокировок — см. ModsState.
//   - Изменяемое UI-состояние (selectedMod*, orderDirty, amlDetected
//     и т.д.) вынесено в UIState (см. ui_state.go); доступны через
//     embed как app.selectedModName и т.д.
//   - UI-виджеты (modTable, mainWindow, ...) остаются в App и трогаем
//     их только из главного потока Fyne (через fyne.Do из горутин).
type App struct {
	// ─── Встроенные сервисы состояния ──────────────────────────────
	// ModsState группирует allMods/displayedMods/systemMods/modDatabase
	// с их мьютексом. UIState — изменяемое UI-состояние. Оба встроены,
	// поэтому поля доступны напрямую как app.allMods, app.orderDirty
	// и т.д.
	ModsState
	UIState

	// ─── Ядро и настройки ──────────────────────────────────────────
	cfg   *Config
	myApp fyne.App
	// messages хранит текущую карту переводов. Атомарный указатель,
	// потому что loadLanguage подменяет её целиком, а читается она
	// из UI-потока и из фоновых горутин (обновления, установка,
	// логирование). Старый map не мутируется — только заменяется.
	messages atomic.Pointer[map[string]string]

	// ─── Окна и верхнеуровневые контейнеры ─────────────────────────
	mainWindow     fyne.Window
	launchGameFunc func(version GameVersion, gameRoot string, skipLauncher bool) error
	gameRoot       string
	patcherType    PatcherType

	// ─── Кэши версий / метаданных (защищены cacheMutex / latestMutex) ─
	versionCache *VersionCache
	changelog    *ChangelogCache

	// ─── UI-виджеты ────────────────────────────────────────────────
	// Всё ниже — только для главного потока Fyne.
	modTable                 *widget.Table
	headerTable              *widget.Table
	systemModsTable          *widget.Table
	systemModsTableContainer *fyne.Container
	systemModsTableSpacer    *canvas.Rectangle // задаёт высоту контейнера
	tableBorderContainer     *fyne.Container
	tableBorder              *canvas.Rectangle
	managePanel              *fyne.Container
	descCardContent          *fyne.Container
	descCardScroll           *container.Scroll
	consoleScroll            *container.Scroll
	descExtraContainer       *fyne.Container

	screenBgRect      *canvas.Rectangle
	headerBoxBgRect   *canvas.Rectangle
	tipBgRect         *canvas.Rectangle
	topPanelBgRect    *canvas.Rectangle
	managePanelBgRect *canvas.Rectangle
	descCardBgRect    *canvas.Rectangle
	descTitle         *canvas.Text
	logHeaderText     *canvas.Text

	manageBtn           *CustomButton
	selectAllBtn        *CustomButton
	deselectAllBtn      *CustomButton
	enableSelectedBtn   *CustomButton
	disableSelectedBtn  *CustomButton
	enableAllBtn        *CustomButton
	disableAllBtn       *CustomButton
	btnRemoveAll        *CustomButton
	btnRemoveSelected   *CustomButton
	moveToTopBtn        *CustomButton
	moveToBottomBtn     *CustomButton
	openFolderBtn       *CustomButton
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
	btnUpdateSelected   *CustomButton
	btnCheckUpdates     *CustomButton
	btnEditVersion      *CustomButton
	searchClearBtn      *CustomButton
	btnAMLConfig        *CustomButton

	selectColumnBgRes fyne.Resource
	toggleOffIcon     fyne.Resource
	toggleOnIcon      fyne.Resource

	statusLabel        *widget.Label
	moveLabel          *widget.Label
	descAuthor         *widget.Label
	descInstalled      *widget.Label
	descBody           *widget.Label
	filterLabel        *widget.Label
	counterLabel       *widget.Label
	descLocalVersion   *widget.Label
	descLatestVersion  *widget.Label
	descLastUpdated    *widget.Label
	descOriginalUpload *widget.Label
	descConflict       *widget.Label
	profileLabel       *widget.Label

	moveToEntry   *widget.Entry
	searchEntry   *widget.Entry
	descURL       *widget.Hyperlink
	githubLink    *widget.Hyperlink
	logWindow     *widget.RichText
	filterSelect  *widget.Select
	profileSelect *widget.Select

	// ─── OAuth ─────────────────────────────────────────────────────
	oauth *OAuthState
	// loggedIn — кэш состояния авторизации. Реальное обращение к
	// системному keyring (keyring.Get) дорогое, а isLoggedIn()
	// вызывается из buildMainMenu, то есть на каждое обновление меню
	// в UI-потоке. Кэш обновляется только при старте, логине, логауте
	// и провале refresh-токена.
	loggedIn atomic.Bool

	// ─── Лог ───────────────────────────────────────────────────────
	logFile *os.File

	// ─── Сеть ──────────────────────────────────────────────────────
	nxm *NXMListener

	// ─── Тема ──────────────────────────────────────────────────────
	// lastAppliedTheme хранит тему, для которой refreshThemeColors уже
	// отработал. Fyne зовёт listeners из settings.apply() не только при
	// SetTheme, но и при перезагрузке settings.json (fileChanged), смене
	// Scale/PrimaryColor и смене системного варианта темы (Linux/Windows).
	// Без фильтра UI перерисовывался бы на каждое такое событие.
	//
	// Читается и пишется только в UI-потоке: SetTheme вызывается из
	// UI-событий (меню, редактор тем), fileChanged и applyVariant — через
	// fyne.Do. Отдельный мьютекс не нужен.
	lastAppliedTheme fyne.Theme

	// ─── Мьютексы (порядок взятия — см. комментарий над App) ──────
	cfgMutex      sync.RWMutex // защищает cfg
	gameRootMutex sync.RWMutex // защищает gameRoot и patcherType
	// loadOrderMutex сериализует запись mod_load_order.txt и его
	// синхронизацию с игровой папкой. Участники: saveCurrentOrder,
	// runAllChecks (через sorter), syncLoadOrderToGame. Порядок:
	// берётся последним, ничего внутри не вкладывается.
	loadOrderMutex sync.Mutex
	// Остальные мьютексы живут внутри сервисов:
	//   versionCache → app.versionCache (version_cache.go)
	//   changelog*   → app.changelog    (changelog_cache.go)
	//   oauth*       → app.oauth        (oauth_state.go)
	//   nxm*         → app.nxm          (nxm_listener.go)
	//   modsMutex    → app.modsMutex    (встроен через ModsState)
}

// msg возвращает перевод по ключу. Безопасно вызывать из любой
// горутины.
func (app *App) msg(key string) string {
	m := app.messages.Load()
	if m == nil {
		return ""
	}
	return (*m)[key]
}

// setMessages атомарно подменяет карту переводов.
func (app *App) setMessages(newMap map[string]string) {
	app.messages.Store(&newMap)
}

func NewApp(cfg *Config, myApp fyne.App) *App {
	app := &App{
		cfg:   cfg,
		myApp: myApp,
	}
	app.setMessages(map[string]string{})
	app.versionCache = NewVersionCache(func(msg string) { app.appendLogToFile(msg) })
	app.changelog = NewChangelogCache()
	app.nxm = NewNXMListener(app.handleNXMLink)
	app.oauth = NewOAuthState()
	app.selectedModIndex.Store(-1)
	// Однократное обращение к keyring на старте — до того, как
	// buildMainMenu начнёт активно дёргать isLoggedIn().
	app.refreshLoginState()
	app.loadLanguage(cfg.Language)
	app.loadNexusVersionCache()

	switch cfg.Theme {
	case "light":
		myApp.Settings().SetTheme(&themes.ForcedLightTheme{})
	case "highcontrast":
		myApp.Settings().SetTheme(&themes.HighContrastTheme{})
	case "custom":
		colors := make(map[string]color.Color, len(cfg.CustomColors))
		for k, v := range cfg.CustomColors {
			colors[k] = v
		}
		base := pickBaseTheme(cfg.CustomBaseTheme)
		myApp.Settings().SetTheme(&themes.CustomTheme{Colors: colors, Base: base})
	default:
		myApp.Settings().SetTheme(&themes.ForcedDarkTheme{})
	}

	// Запоминаем тему на момент старта — чтобы первый callback listener'а
	// не считался «сменой темы» (иначе refreshThemeColors дёрнется зря,
	// когда UI ещё не построен).
	app.lastAppliedTheme = myApp.Settings().Theme()

	// Подписка на смену темы. Listener вызывается из settings.apply() в
	// UI-потоке: SetTheme — синхронно из UI-события; fileChanged и
	// applyVariant — через fyne.Do. Фильтр по lastAppliedTheme отсекает
	// ложные срабатывания при перезагрузке settings.json и смене Scale.
	myApp.Settings().AddListener(func(s fyne.Settings) {
		if s.Theme() == app.lastAppliedTheme {
			return
		}
		app.lastAppliedTheme = s.Theme()
		app.refreshThemeColors()
	})

	root := cfg.GameRoot
	if root == "" {
		root = getGameRootLegacy()
	}
	app.setGameState(root, detectPatcherTypeWithRoot(root))

	return app
}

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

// loadNexusVersionCache читает кэш версий для активного профиля.
// Тонкая обёртка над versionCache.LoadFromProfile — оставлена для
// совместимости call-sites.
func (app *App) loadNexusVersionCache() {
	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()

	if activeProfile == "" {
		app.versionCache.ReplaceAll(nil)
		return
	}

	app.versionCache.LoadFromProfile(app.profilePath(activeProfile))

	if app.versionCache.Len() > 0 {
		app.saveNexusVersionCache()
	}
}

// saveNexusVersionCache записывает кэш версий активного профиля.
// Тонкая обёртка над versionCache.SaveToProfile.
func (app *App) saveNexusVersionCache() {
	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()

	if activeProfile == "" {
		return
	}

	if err := app.versionCache.SaveToProfile(app.profilePath(activeProfile)); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to write nexus versions cache: %v", err))
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
	c.ModsPath = filepath.FromSlash(c.ModsPath)
	c.GameRoot = filepath.FromSlash(c.GameRoot)
	return &c
}

func defaultConfig() *Config {
	return &Config{
		Language:               "en",
		Theme:                  "dark",
		DateFormat:             "dd-mm-yyyy",
		UpdateCheckFrequency:   "every_start",
		ShowSystemMods:         true,
		ShowModListAfterSort:   true,
		FirstRunWizardDisabled: false,
		WizardHelpInstalled:    false,
		CustomBaseTheme:        "dark",
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

// saveConfigSafe сохраняет текущий конфиг с защитой от гонок.
// Делает снимок cfg под cfgMutex.RLock (включая глубокую копию
// CustomColors), затем сохраняет копию. Безопасен для вызова из
// любого места, где cfg может параллельно меняться.
func (app *App) saveConfigSafe() {
	app.cfgMutex.RLock()
	cfgCopy := *app.cfg
	if app.cfg.CustomColors != nil {
		cfgCopy.CustomColors = make(map[string]color.NRGBA, len(app.cfg.CustomColors))
		for k, v := range app.cfg.CustomColors {
			cfgCopy.CustomColors[k] = v
		}
	}
	app.cfgMutex.RUnlock()
	saveConfig(&cfgCopy)
}

func (app *App) syncVersionCache() {
	exePath, err := os.Executable()
	if err == nil {
		if info, err := os.Stat(exePath); err == nil {
			ts := info.ModTime().Unix()
			if saved, ok := app.getCachedVersion(NexusCacheKeyProgram); !ok || saved.Version != AppVersion {
				app.setCachedVersion(NexusCacheKeyProgram, ModVersionInfo{
					Timestamp: ts,
					Version:   AppVersion,
					Folder:    "Program",
					Source:    "nexus",
				})
				app.saveNexusVersionCache()
				app.appendLogToFile(app.msg("log_version_cached_program") + AppVersion)
			}
		}
	}

	app.cfgMutex.RLock()
	dbVersion := app.cfg.LastModDatabaseVersion
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	if dbVersion != "" {
		dbPath := filepath.Join(modsPath, FileNameModDatabase)
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
				app.appendLogToFile(app.msg("log_version_cached_sort") + dbVersion)
			}
		}
	}
}

func (app *App) loadLanguage(lang string) error {
	data, err := embeddedFiles.ReadFile(FileNameMessages)
	if err != nil {
		return fmt.Errorf("cannot read messages.json: %w", err)
	}
	if !json.Valid(data) {
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
	app.setMessages(newMessages)
	return nil
}

func (app *App) getTitle() string {
	return app.msg("app_title_long")
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
	app.modsMutex.Lock()
	app.modDatabase = container.Mods
	app.modsMutex.Unlock()
	app.cfgMutex.Lock()
	app.cfg.LastModDatabaseVersion = container.Version
	app.cfgMutex.Unlock()
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
	mod, ok := app.findModByName(name)
	return ok && mod.Active
}

func (app *App) logVersions() {
	app.cfgMutex.RLock()
	dbVer := app.cfg.LastModDatabaseVersion
	app.cfgMutex.RUnlock()
	app.appendLog(fmt.Sprintf(app.msg("log_version_program"), AppVersion))
	app.appendLog(fmt.Sprintf(app.msg("log_version_sort"), checks.GetExternalVersion()))
	app.appendLogToFile(fmt.Sprintf("mandatory_obsolete_incompatible_dependencies.json version: %s", checks.GetExternalVersion()))
	app.appendLog(fmt.Sprintf(app.msg("log_version_moddb"), dbVer))
	app.appendLogToFile(fmt.Sprintf("mod_database.json version: %s", dbVer))
}

func (app *App) initializePaths() {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	gameRoot := app.cfg.GameRoot
	app.cfgMutex.RUnlock()

	if modsPath != "" {
		if _, err := os.Stat(modsPath); err == nil {
			if gameRoot == "" {
				app.cfgMutex.Lock()
				app.cfg.GameRoot = filepath.Dir(modsPath)
				app.cfgMutex.Unlock()
				saveConfig(app.cfg)
			}
			return
		}
		app.cfgMutex.Lock()
		app.cfg.ModsPath = ""
		app.cfg.GameRoot = ""
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
	}

	autoRoot := autoFindGameRoot()
	if autoRoot != "" {
		choiceChan := make(chan int, 1)
		go func() {
			choice := app.showChoiceDialogSync(
				app.mainWindow,
				app.msg("path_found_title"),
				fmt.Sprintf(app.msg("path_found_message"), autoRoot),
				app.msg("btn_yes"),
				app.msg("btn_choose_other"),
			)
			choiceChan <- choice
		}()
		choice := <-choiceChan
		if choice == 0 {
			app.setGamePaths(autoRoot)
			return
		}
	}

	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	guessedRoot := findGameRootFrom(exeDir)
	if guessedRoot != "" {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.msg("path_found_title"),
			fmt.Sprintf(app.msg("path_found_message"), guessedRoot),
			app.msg("btn_yes"),
			app.msg("btn_choose_other"),
		)
		if choice == 0 {
			app.setGamePaths(guessedRoot)
			return
		}
	}

	diskSearchResult := app.showDiskSearchDialog()
	res := <-diskSearchResult
	if res.Success && res.Path != "" {
		app.setGamePaths(res.Path)
		return
	}

	done := make(chan struct{})
	var selectedPath string
	var cancelled bool
	app.chooseGameRootManually(done, &selectedPath, &cancelled)
	<-done
	if cancelled {
		app.appendLog("Game root not selected. Exiting.")
		app.closeApp()
	}

	if _, err := os.Stat(filepath.Join(selectedPath, "binaries")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(selectedPath, "content")); os.IsNotExist(err) {
			choice := app.showChoiceDialogSync(
				app.mainWindow,
				app.msg("not_game_root_title"),
				fmt.Sprintf(app.msg("not_game_root_message"), selectedPath),
				app.msg("btn_choose_other"),
				app.msg("btn_cancel"),
			)
			if choice == 1 {
				app.appendLog("Game root not selected. Exiting.")
				app.closeApp()
			}
			app.initializePaths()
			return
		}
	}
	app.setGamePaths(selectedPath)
}

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

func autoFindGameRoot() string {
	possibleRoots := []string{
		"C:\\Program Files (x86)\\Steam\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"C:\\Program Files\\Steam\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"D:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"E:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
		"F:\\SteamLibrary\\steamapps\\common\\Warhammer 40,000 DARKTIDE",
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

func (app *App) setGamePaths(gameRoot string) {
	app.cfgMutex.Lock()
	app.cfg.GameRoot = gameRoot
	app.cfgMutex.Unlock()
	modsPath := filepath.Join(gameRoot, "mods")

	if _, err := os.Stat(modsPath); os.IsNotExist(err) {
		choice := app.showChoiceDialogSync(
			app.mainWindow,
			app.msg("mods_not_found_title"),
			app.msg("mods_not_found_message"),
			app.msg("btn_yes_open_dml"),
			app.msg("btn_no_create_folder"),
		)
		if choice == 0 {
			u1, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/19")
			u2, _ := url.Parse("https://www.nexusmods.com/warhammer40kdarktide/mods/8")
			_ = app.myApp.OpenURL(u1)
			_ = app.myApp.OpenURL(u2)
			if err := os.MkdirAll(modsPath, 0755); err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to create mods folder: %v", err))
				app.showInfoDialog(app.msg("window_error_title"), fmt.Sprintf("Failed to create mods folder: %v", err))
				app.closeApp()
			}
		} else {
			if err := os.MkdirAll(modsPath, 0755); err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to create mods folder: %v", err))
				app.showInfoDialog(app.msg("window_error_title"), fmt.Sprintf("Failed to create mods folder: %v", err))
				app.closeApp()
			}
		}
	}
	app.cfgMutex.Lock()
	app.cfg.ModsPath = modsPath
	app.cfgMutex.Unlock()
	saveConfig(app.cfg)
	app.setGameState(gameRoot, detectPatcherTypeWithRoot(gameRoot))
}

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

func (app *App) reloadAfterPathChange() {
	app.refreshModList()
	if app.btnToggle != nil {
		app.updateToggleButtonText(app.btnToggle)
	}
	app.updateDescriptionForMod(app.selectedModName)
	app.forceRefreshTable()

	if !app.pathsInitialized {
		app.pathsInitialized = true
		if !checks.FolderExists("base") || !checks.FolderExists("dmf") {
			app.appendLog("DML or DMF missing, prompting wizard")
			app.promptRunWizard()
		}
		app.ensureSortFiles()
	}
}

func (app *App) loadDataAfterInit() {
	app.cfgMutex.RLock()
	lang := app.cfg.Language
	modsPath := app.cfg.ModsPath
	freq := app.cfg.UpdateCheckFrequency
	initialSetupDone := app.cfg.InitialSetupDone
	suppressAML := app.cfg.SuppressAMLWarning
	app.cfgMutex.RUnlock()

	checks.SetLanguage(lang)

	checks.InitGlobals(
		func(text string) { app.appendLog(text) },
		app.msg, // функция-геттер вместо указателя на map
		func(parent fyne.Window, header, msg string, opts ...string) int {
			return app.showChoiceDialogSync(parent, header, msg, opts...)
		},
		func(link string) {
			fyne.Do(func() { u, _ := url.Parse(link); app.myApp.OpenURL(u) })
		},
		modsPath,
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

	sorter.SetFolderExistsFunc(checks.FolderExists)
	sorter.SetListModFoldersFunc(checks.ListModFolders)
	sorter.SetLogFunc(func(text string) { app.appendLog(text) })
	sorter.SetSortMessages(app.msg("sort_ru_warning"), app.msg("sort_en_warning"))
	sorter.SetHeaderFunc(checks.WriteLoadOrderHeader)
	sorter.SetLogMessages(app.msg("log_create_mlot"), app.msg("log_mlot_created"))

	if err := checks.LoadExternalLists(FileNameMandatoryRules); err != nil {
		app.appendLog(app.msg("log_warn_moid_not_found") + ": " + err.Error())
	} else {
		app.cfgMutex.Lock()
		app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
		app.appendLog(app.msg("log_succ_moid_found"))
	}
	sorter.SetMandatoryOrder(checks.GetMandatoryOrder())
	sorter.SetDependencies(convertDeps(checks.GetDependencies()))
	sorter.SetLoadOrderRules(checks.GetLoadOrderRules())

	if err := app.loadModDatabase(FileNameModDatabase); err != nil {
		app.modsMutex.Lock()
		app.modDatabase = []checks.ModDBEntry{}
		app.modsMutex.Unlock()
		app.appendLog(app.msg("log_mod_db_missing") + ": " + err.Error())
		app.cfgMutex.Lock()
		app.cfg.LastModDatabaseVersion = ""
		app.cfgMutex.Unlock()
	}
	app.modsMutex.RLock()
	dbSnapshot := app.modDatabase
	app.modsMutex.RUnlock()
	checks.SetModDatabase(dbSnapshot)

	app.syncVersionCache()
	if app.logFile != nil {
		fmt.Fprintf(app.logFile, "Program version: %s\n", AppVersion)
		fmt.Fprintf(app.logFile, "mandatory_obsolete_incompatible_dependencies.json version: %s\n", checks.GetExternalVersion())
		app.cfgMutex.RLock()
		dbVer := app.cfg.LastModDatabaseVersion
		app.cfgMutex.RUnlock()
		fmt.Fprintf(app.logFile, "mod_database.json version: %s\n", dbVer)
	}
	sorter.LoadSortOrders(checks.GlobalDataDir())

	SetLauncherMessages(
		app.msg("launcher_ver_unknown"),
		app.msg("launcher_exe_not_found"),
		app.msg("launcher_root_not_found"),
	)
	SetLinuxLauncherMessages(
		app.msg("linux_wine_not_found"),
		app.msg("linux_xbox_not_supported"),
	)
	app.launchGameFunc = launchGame

	app.syncModsEnabledState()

	fyne.Do(func() {
		app.reloadAfterPathChange()

		if app.btnToggle != nil {
			app.updateToggleButtonText(app.btnToggle)
		}

		if !initialSetupDone {
			app.performFirstRunSetup()
		}

		if !app.pathsInitialized {
			app.pathsInitialized = true
			app.amlDetected.Store(checks.IsAMLInstalled(modsPath))
			if app.amlDetected.Load() && !suppressAML {
				app.showChoiceDialogAsync(
					app.mainWindow,
					app.msg("aml_detected_title"),
					app.msg("aml_detected_warning"),
					func(choice int) {
						switch choice {
						case 0:
							if u, err := url.Parse(DarktideModDML); err == nil {
								app.myApp.OpenURL(u)
							}
						case 2:
							app.cfgMutex.Lock()
							app.cfg.SuppressAMLWarning = true
							app.cfgMutex.Unlock()
							saveConfig(app.cfg)
						}
					},
					app.msg("btn_open_dml_page"),
					app.msg("btn_continue"),
					app.msg("btn_dont_show_again"),
				)
			}
		}
	})

	if freq != "never" && app.shouldCheckUpdates() {
		go app.checkSpecialUpdates()
	}

	if exePath, err := os.Executable(); err == nil {
		registerNXMProtocol(exePath)
	}

	app.nxm.Start() // ошибку логируем, если нужно
}

// isLoggedIn возвращает кэшированное состояние авторизации.
// Дёшево — просто atomic.Load, без обращения к keyring.
// Вызывается из buildMainMenu (UI-поток) и обработчиков меню.
func (app *App) isLoggedIn() bool {
	return app.loggedIn.Load()
}

// refreshLoginState обращается к keyring и обновляет кэш.
// Единственный путь обновления флага из внешнего состояния keyring.
// Дорогостоящая операция — вызывать редко: старт, успешный
// token-exchange, логаут, провал refresh.
func (app *App) refreshLoginState() {
	_, err := keyring.Get(keyringService, "access_token")
	app.loggedIn.Store(err == nil)
}

func (app *App) getCachedVersion(key string) (ModVersionInfo, bool) {
	return app.versionCache.Get(key)
}

func (app *App) setCachedVersion(key string, info ModVersionInfo) {
	app.versionCache.Set(key, info)
}

func (app *App) getLatestVersion(key string) (string, bool) {
	return app.versionCache.GetLatest(key)
}

func (app *App) setLatestVersion(key string, version string) {
	app.versionCache.SetLatest(key, version)
}

func (app *App) profilesDir() string {
	return filepath.Join(filepath.Dir(configFilePath()), "profiles")
}

func (app *App) profilePath(name string) string {
	return filepath.Join(app.profilesDir(), name)
}

func (app *App) activeProfilePath() string {
	app.cfgMutex.RLock()
	active := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	return app.profilePath(active)
}

func (app *App) ensureProfileDir(name string) error {
	path := app.profilePath(name)
	return os.MkdirAll(path, 0755)
}

func (app *App) profileExists(name string) bool {
	path := app.profilePath(name)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (app *App) initProfiles() {
	if err := os.MkdirAll(app.profilesDir(), 0755); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to create profiles directory: %v", err))
		return
	}

	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to read profiles directory: %v", err))
		return
	}
	var profileNames []string
	for _, e := range entries {
		if e.IsDir() {
			profileNames = append(profileNames, e.Name())
		}
	}

	if len(profileNames) == 0 {
		app.appendLogToFile("No profiles found. Creating default profile...")
		if err := app.createDefaultProfile(); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create default profile: %v", err))
			return
		}
		profileNames = append(profileNames, "Default")
		app.cfgMutex.Lock()
		app.cfg.ActiveProfile = "Default"
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
	}

	app.cfgMutex.RLock()
	active := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	if active == "" || !app.profileExists(active) {
		app.cfgMutex.Lock()
		app.cfg.ActiveProfile = profileNames[0]
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
	}

	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	app.cleanupProfileRoot(app.profilePath(activeProfile))
	checks.SetProfileDataDir(app.activeProfilePath())
	app.appendLogToFile(fmt.Sprintf("Active profile: %s", activeProfile))
	app.updateSorterOutputPath()

	fyne.Do(func() {
		app.refreshProfileList()
		app.refreshModList()
		app.filterModList()
		app.forceRefreshTable()
	})
}

func (app *App) createDefaultProfile() error {
	defaultPath := app.profilePath("Default")
	if err := os.MkdirAll(defaultPath, 0755); err != nil {
		return err
	}

	app.cfgMutex.RLock()
	modsSrc := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	modsDst := filepath.Join(defaultPath, "mods")
	if err := copyPath(modsSrc, modsDst); err != nil {
		return fmt.Errorf("failed to copy mods folder to profile: %w", err)
	}
	cleanupModsFolder(modsDst)

	srcLO := filepath.Join(modsSrc, FileNameLoadOrder)
	dstLO := filepath.Join(modsDst, FileNameLoadOrder)
	if _, err := os.Stat(srcLO); err == nil {
		if err := copyFile(srcLO, dstLO); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy load order: %v", err))
		}
	}

	srcNV := filepath.Join(filepath.Dir(configFilePath()), FileNameNexusVersions)
	dstNV := filepath.Join(defaultPath, FileNameNexusVersions)
	if _, err := os.Stat(srcNV); err == nil {
		if err := copyFile(srcNV, dstNV); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy nexus versions: %v", err))
		}
	}
	return nil
}

func (app *App) setGlobalDataDir() {
	exePath, err := os.Executable()
	if err != nil {
		app.appendLogToFile("Failed to get executable path: " + err.Error())
		return
	}
	globalDir := filepath.Dir(exePath)
	checks.SetGlobalDataDir(globalDir)
	app.appendLogToFile(fmt.Sprintf("Global data directory set to: %s", globalDir))
}

func (app *App) migrateGlobalFilesFromMods() {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	exePath := app.getExePath()
	exeDir := filepath.Dir(exePath)

	hasModDB := fileExists(filepath.Join(modsPath, FileNameModDatabase))
	hasMandatory := fileExists(filepath.Join(modsPath, FileNameMandatoryRules))
	if !hasModDB && !hasMandatory {
		return
	}

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
			if hasModDB {
				os.Remove(filepath.Join(modsPath, FileNameModDatabase))
			}
			if hasMandatory {
				os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
			}
			app.appendLogToFile("Global files already exist in program directory. Duplicates removed from mods.")
			return
		}

		choice := app.showChoiceDialogSync(app.mainWindow,
			app.msg("migration_title"),
			app.msg("migration_message"),
			app.msg("migration_btn_move"),
			app.msg("btn_cancel"),
		)
		if choice != 0 {
			app.appendLogToFile("Migration cancelled.")
			return
		}

		if hasModDB {
			src := filepath.Join(modsPath, FileNameModDatabase)
			dst := filepath.Join(globalDir, FileNameModDatabase)
			if err := copyFile(src, dst); err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to copy %s: %v", FileNameModDatabase, err))
				return
			}
			app.appendLogToFile(fmt.Sprintf("Copied %s to %s", FileNameModDatabase, globalDir))
		}
		if hasMandatory {
			src := filepath.Join(modsPath, FileNameMandatoryRules)
			dst := filepath.Join(globalDir, FileNameMandatoryRules)
			if err := copyFile(src, dst); err != nil {
				app.appendLogToFile(fmt.Sprintf("Failed to copy %s: %v", FileNameMandatoryRules, err))
				return
			}
			app.appendLogToFile(fmt.Sprintf("Copied %s to %s", FileNameMandatoryRules, globalDir))
		}

		sortFiles := []string{"russian_sort_order.txt", "english_sort_order.txt"}
		for _, sf := range sortFiles {
			src := filepath.Join(modsPath, sf)
			if fileExists(src) {
				dst := filepath.Join(globalDir, sf)
				if err := copyFile(src, dst); err != nil {
					app.appendLogToFile(fmt.Sprintf("Failed to copy %s: %v", sf, err))
				} else {
					os.Remove(src)
					app.appendLogToFile(fmt.Sprintf("Copied %s to %s", sf, globalDir))
				}
			}
		}

		if hasModDB {
			os.Remove(filepath.Join(modsPath, FileNameModDatabase))
		}
		if hasMandatory {
			os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
		}

		app.setGlobalDataDir()
		if err := app.loadModDatabase(FileNameModDatabase); err == nil {
			app.modsMutex.RLock()
			dbSnapshot := app.modDatabase
			app.modsMutex.RUnlock()
			checks.SetModDatabase(dbSnapshot)
		}
		if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
			app.cfgMutex.Lock()
			app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
			app.cfgMutex.Unlock()
			saveConfig(app.cfg)
		}
		sorter.SetMandatoryOrder(checks.GetMandatoryOrder())
		sorter.SetDependencies(convertDeps(checks.GetDependencies()))
		sorter.SetLoadOrderRules(checks.GetLoadOrderRules())
		app.appendLogToFile("Global files migration completed successfully.")
		return
	}

	app.appendLogToFile("Program is located inside the mods folder. Please choose a new location for the program and global files.")

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
		app.appendLogToFile("Migration cancelled (no folder selected).")
		app.showInfoDialog(app.msg("warning_title"), "Migration cancelled. Program and files remain in the mods folder.")
		return
	}

	targetDir := filepath.Join(selectedPath, "Servo-Modquisitor")
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to create target directory: %v", err))
		app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to create folder: %v", err))
		return
	}

	exeName := filepath.Base(exePath)
	targetExe := filepath.Join(targetDir, exeName)
	if err := copyFile(exePath, targetExe); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to copy program: %v", err))
		app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to copy program: %v", err))
		return
	}
	app.appendLogToFile(fmt.Sprintf("Program copied to %s", targetExe))

	if hasModDB {
		src := filepath.Join(modsPath, FileNameModDatabase)
		dst := filepath.Join(targetDir, FileNameModDatabase)
		if err := copyFile(src, dst); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy %s: %v", FileNameModDatabase, err))
			return
		}
		app.appendLogToFile(fmt.Sprintf("Copied %s to %s", FileNameModDatabase, targetDir))
	}
	if hasMandatory {
		src := filepath.Join(modsPath, FileNameMandatoryRules)
		dst := filepath.Join(targetDir, FileNameMandatoryRules)
		if err := copyFile(src, dst); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy %s: %v", FileNameMandatoryRules, err))
			return
		}
		app.appendLogToFile(fmt.Sprintf("Copied %s to %s", FileNameMandatoryRules, targetDir))
	}

	if hasModDB {
		os.Remove(filepath.Join(modsPath, FileNameModDatabase))
	}
	if hasMandatory {
		os.Remove(filepath.Join(modsPath, FileNameMandatoryRules))
	}

	msg := fmt.Sprintf(
		"Program and global files have been copied to:\n%s\n\n"+
			"Please close this program and run the new copy from the new location.\n"+
			"After that, you can safely delete the old program folder:\n%s",
		targetDir, exeDir,
	)
	app.showInfoDialog(app.msg("migration_success_title"), msg)
	app.appendLogToFile("Program and files copied to new location. Please restart from the new copy.")
}

func (app *App) refreshProfileList() {
	entries, err := os.ReadDir(app.profilesDir())
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to read profiles: %v", err))
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
		app.cfgMutex.RLock()
		active := app.cfg.ActiveProfile
		app.cfgMutex.RUnlock()
		if active != "" {
			app.profileSelect.SetSelected(active)
		} else if len(names) > 0 {
			app.profileSelect.SetSelected(names[0])
		}
		app.profileSelect.Refresh()
	}
}

// switchProfile — обёртка без колбэка: используется там, где вызывающему
// всё равно, когда переключение завершится (обычный клик в выпадающем
// списке профилей).
func (app *App) switchProfile(name string) {
	app.switchProfileAsync(name, nil)
}

// switchProfileAsync переключает активный профиль и вызывает onDone в
// UI-потоке после того, как переключение завершено. onDone может быть nil.
//
// onDone гарантированно вызывается и при успешном переключении, и при
// раннем выходе (target совпадает с текущим или не существует) — чтобы
// вызывающий код, ожидающий завершения, не подвис.
//
// Порядок: колбэк регистрируется через fyne.Do ПОСЛЕ внутреннего
// fyne.Do, который обновляет UI, поэтому к моменту его выполнения
// cfg.ActiveProfile уже переключён, а интерфейс — перестроен.
func (app *App) switchProfileAsync(name string, onDone func()) {
	app.cfgMutex.RLock()
	current := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	if name == current || !app.profileExists(name) {
		if onDone != nil {
			onDone()
		}
		return
	}

	// UI-часть — сразу, в текущем UI-потоке
	if app.orderDirty {
		app.saveCurrentOrder()
		app.orderDirty = false
		app.stopBlinkSaveButton()
		app.updateTableBorder()
	}

	go func() {
		// onDone вызывается в UI-потоке после завершения горутины.
		// Даже если внутри случится ранний return (ошибка копирования),
		// колбэк сработает — и вызывающий код сможет показать ошибку.
		if onDone != nil {
			defer fyne.Do(onDone)
		}

		srcMods := filepath.Join(app.profilePath(name), "mods")
		app.cfgMutex.RLock()
		dstMods := app.cfg.ModsPath
		app.cfgMutex.RUnlock()

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

		if err := copyPath(srcMods, dstMods); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy mods from profile '%s': %v", name, err))
			return
		}
		cleanupModsFolder(dstMods)

		app.cleanupProfileRoot(app.profilePath(name))

		var newCache map[string]ModVersionInfo
		if tempCache := NewVersionCache(nil); true {
			tempCache.LoadFromProfile(app.profilePath(name))
			newCache = tempCache.Snapshot()
		}

		app.cfgMutex.Lock()
		app.cfg.ActiveProfile = name
		app.versionCache.ReplaceAll(newCache)
		app.cfgMutex.Unlock()

		app.saveConfigSafe()
		checks.SetProfileDataDir(app.activeProfilePath())
		app.updateSorterOutputPath()

		fyne.Do(func() {
			app.refreshProfileList()
			app.refreshModList()
			app.saveCurrentOrder()
			app.orderDirty = false
			app.stopBlinkSaveButton()
			app.updateTableBorder()
			app.filterModList()
			app.forceRefreshTable()
			app.appendLogToFile(fmt.Sprintf("Switched to profile: %s", name))
			if app.profileSelect != nil {
				app.profileSelect.SetSelected(name)
			}
		})
	}()
}

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
		app.cleanupProfileRoot(app.profilePath(name))
	} else {
		modsPath := filepath.Join(app.profilePath(name), "mods")
		if err := os.MkdirAll(modsPath, 0755); err != nil {
			return err
		}
		emptyPath := filepath.Join(modsPath, FileNameLoadOrder)
		if err := os.WriteFile(emptyPath, []byte(""), 0644); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create empty load order: %v", err))
		}
		emptyCache := map[string]ModVersionInfo{}
		data, _ := json.MarshalIndent(emptyCache, "", "\t")
		if err := os.WriteFile(filepath.Join(app.profilePath(name), FileNameNexusVersions), data, 0644); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to create empty nexus_versions.json: %v", err))
		}
	}

	app.appendLogToFile(fmt.Sprintf("Created profile: %s", name))
	app.refreshProfileList()
	return nil
}

func (app *App) cleanupProfileRoot(profilePath string) {
	path := filepath.Join(profilePath, FileNameLoadOrder)
	if _, err := os.Stat(path); err == nil {
		if err := os.Remove(path); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to remove legacy load order file from profile root: %v", err))
		} else {
			app.appendLogToFile(fmt.Sprintf("Removed legacy load order file from profile root: %s", path))
		}
	}
}

func (app *App) deleteProfile(name string) error {
	if name == "Default" {
		return fmt.Errorf("cannot delete default profile")
	}
	if !app.profileExists(name) {
		return fmt.Errorf("profile '%s' does not exist", name)
	}
	app.cfgMutex.RLock()
	current := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	if name == current {
		return fmt.Errorf("cannot delete active profile")
	}
	if err := os.RemoveAll(app.profilePath(name)); err != nil {
		return err
	}
	app.appendLogToFile(fmt.Sprintf("Deleted profile: %s", name))
	app.refreshProfileList()
	return nil
}

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
	app.cfgMutex.RLock()
	current := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	if current == oldName {
		app.cfgMutex.Lock()
		app.cfg.ActiveProfile = newName
		app.cfgMutex.Unlock()
		saveConfig(app.cfg)
	}
	app.appendLogToFile(fmt.Sprintf("Renamed profile: %s -> %s", oldName, newName))
	app.refreshProfileList()
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (app *App) syncProfileFromGame() {
	profilePath := app.activeProfilePath()
	if profilePath == "" {
		app.appendLogToFile("No active profile to sync")
		return
	}

	dstMods := filepath.Join(profilePath, "mods")
	app.cfgMutex.RLock()
	src := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

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

	if err := copyPath(src, dstMods); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to sync mods to profile: %v", err))
		return
	}
	cleanupModsFolder(dstMods)
	app.appendLogToFile("Profile synced from game folder.")
}

// syncModToProfile копирует одну папку мода из игровой папки в активный
// профиль. Дешевле полного syncProfileFromGame (который копирует все
// моды разом) — используется после установки одного мода через Nexus.
//
// Если папка уже есть в профиле — удаляется перед копированием, чтобы
// не осталось «хвостов» от старой версии (файлы, которых нет в новой,
// но которые сохранились бы при простой перезаписи).
//
// Ограничение: копируется ровно один мод. Для архивов с несколькими
// модами внутри вызывающий должен пройти по списку и вызвать функцию
// для каждого — либо воспользоваться syncProfileFromGame.
func (app *App) syncModToProfile(modName string) {
	if modName == "" {
		return
	}
	profilePath := app.activeProfilePath()
	if profilePath == "" {
		app.appendLogToFile("syncModToProfile: no active profile to sync")
		return
	}

	app.cfgMutex.RLock()
	src := filepath.Join(app.cfg.ModsPath, modName)
	app.cfgMutex.RUnlock()

	if _, err := os.Stat(src); err != nil {
		app.appendLogToFile(fmt.Sprintf("syncModToProfile: source %s not found: %v", src, err))
		return
	}

	dst := filepath.Join(profilePath, "mods", modName)
	// Удаляем старую версию, чтобы не оставить устаревших файлов.
	if _, err := os.Stat(dst); err == nil {
		if err := os.RemoveAll(dst); err != nil {
			app.appendLogToFile(fmt.Sprintf("syncModToProfile: failed to remove old %s: %v", dst, err))
			return
		}
	}

	if err := copyPath(src, dst); err != nil {
		app.appendLogToFile(fmt.Sprintf("syncModToProfile: failed to copy %s -> %s: %v", src, dst, err))
		return
	}
	app.appendLogToFile(fmt.Sprintf("Mod %s synced to profile.", modName))
}

func (app *App) getExePath() string {
	exe, _ := os.Executable()
	return exe
}

func cleanupModsFolder(path string) {
	unwanted := []string{
		FileNameModDatabase,
		FileNameMandatoryRules,
		AppName + ".exe",
	}
	for _, f := range unwanted {
		toRemove := filepath.Join(path, f)
		if _, err := os.Stat(toRemove); err == nil {
			if err := os.Remove(toRemove); err != nil {
				// ignore
			}
		}
	}
}

func (app *App) updateSorterOutputPath() {
	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	if activeProfile != "" {
		sorter.SetLoadOrderOutputPath(filepath.Join(app.profilePath(activeProfile), "mods", FileNameLoadOrder))
	} else {
		sorter.SetLoadOrderOutputPath(filepath.Join(modsPath, FileNameLoadOrder))
	}
}

// syncLoadOrderToGameLocked копирует mod_load_order.txt из активного
// профиля в игровую папку. Вызывающий ОБЯЗАН держать app.loadOrderMutex.
func (app *App) syncLoadOrderToGameLocked() {
	app.cfgMutex.RLock()
	src := filepath.Join(app.activeProfilePath(), "mods", FileNameLoadOrder)
	dst := filepath.Join(app.cfg.ModsPath, FileNameLoadOrder)
	app.cfgMutex.RUnlock()
	if err := copyFile(src, dst); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to copy load order to game folder: %v", err))
	} else {
		app.appendLogToFile("Load order synced to game folder")
	}
}

// syncLoadOrderToGame — публичная обёртка: берёт loadOrderMutex и
// делегирует в locked-версию. Используется там, где вызывающий не
// держит мьютекс (например, из InstallModFromArchive).
func (app *App) syncLoadOrderToGame() {
	app.loadOrderMutex.Lock()
	defer app.loadOrderMutex.Unlock()
	app.syncLoadOrderToGameLocked()
}

func (app *App) performFirstRunSetup() {
	app.cfgMutex.Lock()
	app.cfg.InitialSetupDone = true
	app.cfgMutex.Unlock()
	saveConfig(app.cfg)
}

func (app *App) shouldCheckUpdates() bool {
	app.cfgMutex.RLock()
	freq := app.cfg.UpdateCheckFrequency
	lastStr := app.cfg.LastUpdateCheck
	app.cfgMutex.RUnlock()
	if freq == "never" {
		return false
	}
	if lastStr == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, lastStr)
	if err != nil {
		return true
	}
	now := time.Now()
	switch freq {
	case "weekly":
		return now.Sub(last) >= 7*24*time.Hour
	case "monthly":
		return now.After(last.AddDate(0, 1, 0))
	case "yearly":
		return now.After(last.AddDate(1, 0, 0))
	}
	return false
}

// getGameState возвращает согласованный снимок gameRoot+patcherType.
// Читать их парой вне блокировки нельзя: setGamePaths меняет оба
// поля, и UI может увидеть «старый root + новый patcher».
func (app *App) getGameState() (string, PatcherType) {
	app.gameRootMutex.RLock()
	defer app.gameRootMutex.RUnlock()
	return app.gameRoot, app.patcherType
}

func (app *App) setGameState(root string, patcher PatcherType) {
	app.gameRootMutex.Lock()
	app.gameRoot = root
	app.patcherType = patcher
	app.gameRootMutex.Unlock()
}

// pickBaseTheme возвращает базовую тему по строковому идентификатору
// из конфига. Используется для построения CustomTheme: для имён,
// которые пользователь не переопределил, цвета берутся отсюда.
func pickBaseTheme(name string) fyne.Theme {
	switch name {
	case "light":
		return &themes.ForcedLightTheme{}
	case "highcontrast":
		return &themes.HighContrastTheme{}
	default:
		return &themes.ForcedDarkTheme{}
	}
}
