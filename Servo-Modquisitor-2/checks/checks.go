// Servo-Modquisitor-2/checks/checks.go
package checks

import (
	"Servo-Modquisitor/helpers"
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
)

const (
	FileNameLoadOrder = "mod_load_order.txt"
	steamGuideEnURL   = "https://steamcommunity.com/sharedfiles/filedetails/?id=2953324027"
	steamGuideRuURL   = "https://steamcommunity.com/sharedfiles/filedetails/?id=2950374474"
)

var modListDividers = []string{
	"___System",
	"___Scoreboard",
	"___HUD",
	"___GamePlay Utilities",
	"___GamePlay Improve",
	"___Game Tools",
	"___Bug Fix",
}

var loadOrderHeaderPhrases = []string{
	"Enter user mod names below, separated by line.",
	"Order in the list determines the order in which mods are loaded.",
	"Do not rename a mod's folders.",
	"You do not need to include 'base' or 'dmf' mod folders.",
}

var (
	appendLog func(string)
	// msgGetter безопасно читает перевод по ключу — обычно указывает
	// на App.msg, работающий через atomic.Pointer. Так checks не
	// держит указатель на map, которую App подменяет целиком.
	msgGetter        func(string) string
	showChoiceDialog func(fyne.Window, string, string, ...string) int
	openURL          func(string)
	modsDir          string
	isModActiveFunc  func(string) bool
	modDBMap         map[string]*ModDBEntry
	modDBMutex       sync.RWMutex // защищает modDBMap
	externalVersion  string
	extVersionMutex  sync.RWMutex // защищает externalVersion
	getInstalledAt   func(string) int64

	// pathsMutex защищает modsDir, profileDataDir, globalDataDir.
	// Порядок: pathsMutex — внутренний (вкладывается в modDBMutex,
	// но не наоборот).
	pathsMutex     sync.RWMutex
	profileDataDir string
	globalDataDir  string
)

// getModsDir возвращает путь к папке модов.
func getModsDir() string {
	pathsMutex.RLock()
	defer pathsMutex.RUnlock()
	return modsDir
}

// getProfileDataDir возвращает путь к папке данных активного профиля.
func getProfileDataDir() string {
	pathsMutex.RLock()
	defer pathsMutex.RUnlock()
	return profileDataDir
}

// getGlobalDataDir возвращает путь к папке глобальных данных.
func getGlobalDataDir() string {
	pathsMutex.RLock()
	defer pathsMutex.RUnlock()
	return globalDataDir
}

func InitGlobals(
	logger func(string),
	msgFn func(string) string,
	dialogFunc func(fyne.Window, string, string, ...string) int,
	urlOpener func(string),
	modsDirPath string,
	isActiveFn func(string) bool,
	refreshFn func(),
	installedAtGetter func(string) int64,
) {
	appendLog = logger
	if msgFn == nil {
		msgFn = func(string) string { return "" }
	}
	msgGetter = msgFn
	showChoiceDialog = dialogFunc
	openURL = urlOpener

	pathsMutex.Lock()
	modsDir = modsDirPath
	pathsMutex.Unlock()

	isModActiveFunc = isActiveFn
	refreshModListFunc = refreshFn
	getInstalledAt = installedAtGetter
}

func SetProfileDataDir(path string) {
	pathsMutex.Lock()
	profileDataDir = path
	pathsMutex.Unlock()
}

func SetGlobalDataDir(path string) {
	pathsMutex.Lock()
	globalDataDir = path
	pathsMutex.Unlock()
}

var refreshModListFunc func()

// currentLang хранит текущий код языка. Атомарный указатель, потому что
// значение пишется из UI-потока (changeLanguage) и из фоновой инициализации
// (loadDataAfterInit), а читается в GetIncompatibleDesc, WriteLoadOrderHeader
// и askMissing.
var currentLang atomic.Pointer[string]

// SetLanguage сохраняет код языка потокобезопасно.
func SetLanguage(lang string) {
	currentLang.Store(&lang)
}

// getCurrentLang возвращает текущий код языка. Если SetLanguage ещё не
// вызывался, возвращает "en".
func getCurrentLang() string {
	if p := currentLang.Load(); p != nil {
		return *p
	}
	return "en"
}

func SetModDatabase(entries []ModDBEntry) {
	modDBMutex.Lock()
	defer modDBMutex.Unlock()
	modDBMap = make(map[string]*ModDBEntry, len(entries))
	for i := range entries {
		modDBMap[strings.ToLower(entries[i].Folder)] = &entries[i]
	}
}

type LoadOrderRule struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

var LoadOrderRules []LoadOrderRule

func ModsDir() string { return getModsDir() }

func FolderExists(name string) bool {
	info, err := os.Stat(filepath.Join(getModsDir(), name))
	if os.IsNotExist(err) {
		return false
	}
	return info != nil && info.IsDir()
}

func RemoveMod(name string) {
	path := filepath.Join(getModsDir(), name)
	info, err := os.Lstat(path)
	if err != nil {
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		os.Remove(path)
	} else {
		os.RemoveAll(path)
	}
}

func ListModFolders() []string {
	var folders []string
	dir := getModsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if appendLog != nil {
			appendLog(fmt.Sprintf(msgGetter("log_error_reading_mods_dir"), err))
		}
		return folders
	}
	for _, e := range entries {
		if e.IsDir() {
			folders = append(folders, e.Name())
			continue
		}
		if e.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			info, err := os.Stat(filepath.Join(dir, e.Name()))
			if err == nil && info.IsDir() {
				folders = append(folders, e.Name())
			}
		}
	}
	return folders
}

type ModInfo struct {
	Active            bool
	Broken            bool
	Incompatible      bool
	Mandatory         bool
	Obsolete          bool
	Selected          bool
	IsSystem          bool
	VortexDeployed    bool
	IsSymlink         bool
	MissingFolder     bool
	HasUpdate         bool `json:"-"`
	NexusDownloads    int
	NexusEndorsements int
	Author            string
	Description       string
	DisplayName       string
	Name              string
	Note              string
	URL               string
	GitHubURL         string
	Category          string
	NexusVersion      string
	NexusSummary      string
	NexusPictureURL   string
	Source            string
	ModTime           time.Time
	LastUpdated       time.Time `json:"-"`
	OriginalUpload    time.Time `json:"-"`
}

type ModDBEntry struct {
	Folder           string            `json:"folder"`
	NexusFilePattern string            `json:"nexus_file_pattern"`
	Name             map[string]string `json:"name"`
	Description      map[string]string `json:"description"`
	Author           string            `json:"author"`
	Category         string            `json:"category"`
	URL              string            `json:"url"`
	GitHubURL        string            `json:"github_url"`
	SourceCodeURL    string            `json:"source_code_url"`
	Note             map[string]string `json:"note"`
}

// JoinNotes склеивает непустые заметки через разделитель " · ".
// Пустые игнорируются, полностью пустой результат — пустая строка.
//
// Используется там, где два независимых источника текста должны
// сосуществовать (например, структурная причина отключения мода по
// имени папки + свободная заметка из mod_database.json, или заметка
// + пометка о конфликте). Раньше прямое присваивание / конкатенация
// без разделителя затирали или склеивали текст.
func JoinNotes(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " · ")
}

func GetModsInfo(lang string, forceEnglish bool) []ModInfo {
	modDBMutex.RLock()
	defer modDBMutex.RUnlock()

	modsPath := getModsDir() // снимок на функцию
	folders := ListModFolders()
	var mods []ModInfo

	// 1. Читаем файл порядка
	loadOrderEntries := ReadLoadOrder()
	orderMap := make(map[string]bool)
	for _, entry := range loadOrderEntries {
		orderMap[entry.Name] = entry.Active
	}

	// 2. Создаём записи для всех папок
	for _, name := range folders {
		fullPath := filepath.Join(modsPath, name)
		fi, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		mod := ModInfo{Name: name, Active: true}

		// Применяем активность из файла порядка
		if active, ok := orderMap[name]; ok {
			mod.Active = active
		} else {
			// Если мода нет в файле порядка, он считается неактивным
			mod.Active = false
		}

		// Определяем системные моды
		switch name {
		case "base", "dmf":
			mod.IsSystem = true
			mod.Active = false // системные моды всегда неактивны (они не управляются через порядок)
		}

		// Обработка префиксов для неактивных модов
		if msgGetter != nil {
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "__") {
				mod.Active = false
				mod.Note = msgGetter("note_disabled_prefix")
			} else if strings.HasPrefix(name, "--") {
				mod.Active = false
				mod.Note = msgGetter("note_disabled_prefix_double")
			} else if strings.Contains(name, " - Copy") || strings.Contains(name, " — копия") {
				mod.Active = false
				mod.Note = msgGetter("note_backup_copy")
			}
		}

		if helpers.ContainsString(modListDividers, name) {
			mod.Active = true
			mod.Note = ""
		}

		mod.VortexDeployed = fileExists(filepath.Join(fullPath, "__folder_managed_by_vortex"))

		// Определяем cacheKey для получения даты установки и версии
		var cacheKey string
		switch name {
		case "base":
			cacheKey = "19:base"
		case "dmf":
			cacheKey = "8:dmf"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if db, ok := modDBMap[strings.ToLower(name)]; ok && db.URL != "" {
				if id := helpers.ExtractModIDFromURL(db.URL); id != 0 {
					cacheKey = fmt.Sprintf("%d:%s", id, name)
				}
			}
		}

		modTimeSet := false
		if cacheKey != "" && getInstalledAt != nil {
			if installed := getInstalledAt(cacheKey); installed > 0 {
				mod.ModTime = time.Unix(installed, 0)
				modTimeSet = true
			}
		}

		if !modTimeSet {
			switch {
			case name == "base":
				mod.ModTime = getModTimeFromFile(filepath.Join(fullPath, "mod_manager.lua"))
			case name == "dmf":
				mod.ModTime = getModTimeFromFile(filepath.Join(fullPath, "scripts", "mods", "dmf", "dmf_loader.lua"))
			default:
				luaPaths := []string{
					filepath.Join(fullPath, name+".lua"),
					filepath.Join(fullPath, "scripts", "mods", name, name+".lua"),
				}
				foundLua := false
				for _, lp := range luaPaths {
					if t := getModTimeFromFile(lp); !t.IsZero() {
						mod.ModTime = t
						foundLua = true
						break
					}
				}
				if !foundLua {
					modFilePath := filepath.Join(fullPath, name+".mod")
					if !fileExists(modFilePath) {
						mod.Broken = true
					} else {
						mod.Broken = false
					}
					if strings.HasPrefix(mod.Name, "_") || strings.HasPrefix(mod.Name, "__") || strings.HasPrefix(mod.Name, "--") {
						mod.Broken = false
					}
					if mod.ModTime.IsZero() {
						if modFileInfo, err := os.Stat(modFilePath); err == nil {
							mod.ModTime = modFileInfo.ModTime()
						} else {
							mod.ModTime = fi.ModTime()
						}
					}
				}
			}
		}

		// Заполняем поля из базы данных
		if db, ok := modDBMap[strings.ToLower(name)]; ok && db.Folder != "" {
			mod.Author = db.Author
			mod.URL = db.URL
			mod.GitHubURL = db.GitHubURL
			mod.Description = PickLocalized(db.Description, lang)
			// Не перезаписываем Note: если выше уже проставлена
			// структурная причина (disabled-префикс, копия), она
			// должна сохраниться. joinNotes склеивает обе заметки
			// через разделитель, сохраняя порядок (структурная — первая).
			mod.Note = JoinNotes(mod.Note, PickLocalized(db.Note, lang))
			if forceEnglish {
				if enName := PickLocalized(db.Name, "en"); enName != "" {
					mod.DisplayName = enName
				}
			} else {
				if dn := PickLocalized(db.Name, lang); dn != "" {
					mod.DisplayName = dn
				}
			}
		}
		if mod.Description == "" {
			mod.Description = tryReadLocalization(name)
		}

		mods = append(mods, mod)
	}

	// 3. Добавляем записи из файла порядка для отсутствующих папок
	for _, entry := range loadOrderEntries {
		if !containsFolder(folders, entry.Name) {
			mod := ModInfo{
				Name:          entry.Name,
				Active:        entry.Active,
				MissingFolder: true,
				ModTime:       time.Time{},
			}
			// Заполняем из базы данных
			if db, ok := modDBMap[strings.ToLower(entry.Name)]; ok && db.Folder != "" {
				mod.Author = db.Author
				mod.URL = db.URL
				mod.GitHubURL = db.GitHubURL
				mod.Category = db.Category
				mod.Description = PickLocalized(db.Description, lang)
				mod.Note = PickLocalized(db.Note, lang)
				if forceEnglish {
					if enName := PickLocalized(db.Name, "en"); enName != "" {
						mod.DisplayName = enName
					}
				} else {
					if dn := PickLocalized(db.Name, lang); dn != "" {
						mod.DisplayName = dn
					}
				}
			}
			mods = append(mods, mod)
		}
	}

	// 4. Добавляем системный мод autopatch (если установлен)
	autopatchDLL := filepath.Join(modsPath, "..", "binaries", "plugins", "_dt_mod_autopatch.dll")
	if _, err := os.Stat(autopatchDLL); err == nil {
		mod := ModInfo{
			Name:     "autopatch",
			IsSystem: true,
			Active:   false,
		}
		mod.ModTime = getModTimeFromFile(autopatchDLL)

		if db, ok := modDBMap["autopatch"]; ok && db.Folder != "" {
			mod.Author = db.Author
			mod.URL = db.URL
			mod.GitHubURL = db.GitHubURL
			mod.Description = PickLocalized(db.Description, lang)
			mod.Note = PickLocalized(db.Note, lang)
			if forceEnglish {
				if enName := PickLocalized(db.Name, "en"); enName != "" {
					mod.DisplayName = enName
				}
			} else {
				if dn := PickLocalized(db.Name, lang); dn != "" {
					mod.DisplayName = dn
				}
			}
		}
		mods = append(mods, mod)
	}

	return mods
}

func getModTimeFromFile(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func TryFixMismatchedModFolder(folderPath, currentName string) string {
	if isDisabledByNaming(currentName) {
		return ""
	}
	modFile := findSingleModFile(folderPath)
	if modFile == "" {
		return ""
	}
	expectedName := strings.TrimSuffix(modFile, ".mod")
	if expectedName == currentName {
		return ""
	}
	if strings.Contains(modFile, string(os.PathSeparator)) {
		return ""
	}
	newPath := filepath.Join(filepath.Dir(folderPath), expectedName)
	if err := os.Rename(folderPath, newPath); err != nil {
		appendLog(fmt.Sprintf(msgGetter("log_failed_to_autorename"), currentName, expectedName, err))
		return ""
	}
	appendLog(fmt.Sprintf(msgGetter("log_failed_to_autorename_mis"), currentName, expectedName))
	return expectedName
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tryReadLocalization(modName string) string {
	dir := filepath.Join(getModsDir(), modName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), "_localization.lua") {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "mod_description") {
					parts := strings.SplitN(line, "=", 2)
					if len(parts) == 2 {
						desc := strings.TrimSpace(parts[1])
						return strings.Trim(desc, `"`)
					}
				}
			}
			break
		}
	}
	return ""
}

type LoadOrderEntry struct {
	Name   string
	Active bool
}

func ReadLoadOrder() []LoadOrderEntry {
	dir := getProfileDataDir()
	if dir == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "mods", FileNameLoadOrder))
	if err != nil {
		return nil
	}
	var entries []LoadOrderEntry
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "-- ▒") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "modified by modtide on") {
			continue
		}
		if strings.HasPrefix(line, "--") && !strings.HasPrefix(line, "-- ") {
			continue
		}
		active := true
		name := line
		if strings.HasPrefix(line, "-- ") {
			active = false
			name = strings.TrimPrefix(line, "-- ")
		}
		if name == "" || name == "base" || name == "dmf" {
			continue
		}
		if helpers.ContainsString(loadOrderHeaderPhrases, name) {
			continue
		}
		hasValidChar := false
		for _, ch := range name {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
				hasValidChar = true
				break
			}
		}
		if !hasValidChar {
			continue
		}
		entries = append(entries, LoadOrderEntry{Name: name, Active: active})
	}
	return entries
}

func WriteLoadOrder(entries []LoadOrderEntry) error {
	dir := getProfileDataDir()
	if dir == "" {
		return fmt.Errorf("profile data directory not set")
	}
	f, err := os.Create(filepath.Join(dir, "mods", FileNameLoadOrder))
	if err != nil {
		return err
	}
	defer f.Close()
	WriteLoadOrderHeader(f, getCurrentLang())
	for _, e := range entries {
		if e.Active {
			fmt.Fprintln(f, e.Name)
		} else {
			fmt.Fprintln(f, "-- "+e.Name)
		}
	}
	return nil
}

func UpdateModActive(entries []LoadOrderEntry, modName string, active bool) []LoadOrderEntry {
	for i, e := range entries {
		if e.Name == modName {
			entries[i].Active = active
			return entries
		}
	}
	return append(entries, LoadOrderEntry{Name: modName, Active: active})
}

var (
	checksDataMutex   sync.RWMutex // Решение гонки 79
	ObsoleteMods      []string
	IncompatiblePairs []IncompatiblePair
	Dependencies      []Dependency
	MandatoryOrder    []string
)

type ExternalData struct {
	Version           string             `json:"version"`
	MandatoryOrder    []string           `json:"mandatory_order"`
	ObsoleteMods      []string           `json:"obsolete_mods"`
	IncompatiblePairs []IncompatiblePair `json:"incompatible_pairs"`
	Dependencies      []Dependency       `json:"dependencies"`
	LoadOrder         []LoadOrderRule    `json:"load_order"`
}

type IncompatiblePair struct {
	Mod1 string            `json:"mod1"`
	Mod2 string            `json:"mod2"`
	Desc map[string]string `json:"desc"`
	Type string            `json:"type,omitempty"`
}
type Dependency struct{ Dependent, Required, RequiredURL string }

func LoadExternalLists(filename string) error {
	dir := getGlobalDataDir()
	if dir == "" {
		return fmt.Errorf("global data directory not set")
	}
	fullPath := filepath.Join(dir, filename)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", fullPath, err)
	}
	var ext ExternalData
	if err := json.Unmarshal(data, &ext); err != nil {
		offset := -1
		var line, col int
		var snippet string

		switch e := err.(type) {
		case *json.SyntaxError:
			offset = int(e.Offset)
		case *json.UnmarshalTypeError:
			offset = int(e.Offset)
		}

		if offset >= 0 && offset < len(data) {
			line, col = 1, 1
			for i := 0; i < offset; i++ {
				if data[i] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			start := offset - 30
			if start < 0 {
				start = 0
			}
			end := offset + 30
			if end > len(data) {
				end = len(data)
			}
			snippet = string(data[start:end])
		}

		if line > 0 {
			return fmt.Errorf("JSON error in %s at line %d, col %d: %w\nnear: ...%s...", fullPath, line, col, err, snippet)
		}
		return fmt.Errorf("cannot unmarshal %s: %w", fullPath, err)
	}
	extVersionMutex.Lock()
	externalVersion = ext.Version
	extVersionMutex.Unlock()
	checksDataMutex.Lock() // Решение гонки 79
	ObsoleteMods = ext.ObsoleteMods
	IncompatiblePairs = ext.IncompatiblePairs
	Dependencies = ext.Dependencies
	LoadOrderRules = ext.LoadOrder
	MandatoryOrder = ext.MandatoryOrder
	checksDataMutex.Unlock()
	return nil
}

func GetExternalVersion() string {
	extVersionMutex.RLock()
	defer extVersionMutex.RUnlock()
	return externalVersion
}

func IsMandatoryMod(name string) bool {
	checksDataMutex.RLock() // Решение гонки 79
	defer checksDataMutex.RUnlock()
	for _, m := range MandatoryOrder {
		if m == name {
			return true
		}
	}
	return false
}

func CheckInstallation(window fyne.Window) bool {
	if !FolderExistsWithTimeout("base", 3*time.Second) {
		appendLog(msgGetter("log_warn_base_missing"))
		return askMissing("base", "DML", "Darktide Mod Loader", "https://www.nexusmods.com/warhammer40kdarktide/mods/19", window)
	}
	if !FolderExistsWithTimeout("dmf", 3*time.Second) {
		appendLog(msgGetter("dmf_missing"))
		return askMissing("dmf", "DMF", "Darktide Mod Framework", "https://www.nexusmods.com/warhammer40kdarktide/mods/8", window)
	}
	appendLog(msgGetter("step_install"))
	return true
}

func askMissing(folder, modAbbr, modName, nexusURL string, window fyne.Window) bool {
	choice := showChoiceDialog(window, msgGetter("window_error_title"),
		fmt.Sprintf(msgGetter("window_error_dsc_dml_dmf"), folder, modName),
		msgGetter("btn_open_steam_guide"),
		fmt.Sprintf(msgGetter("btn_open_nexus_for_mod"), modAbbr),
		msgGetter("open_mods_folder"),
		msgGetter("btn_cancel"),
	)
	switch choice {
	case 0:
		appendLog(msgGetter("log_open_steam_guide"))
		guideURL := steamGuideEnURL
		if getCurrentLang() == "ru" {
			guideURL = steamGuideRuURL
		}
		openURL(guideURL)
	case 1:
		appendLog(msgGetter("log_open_nexus_page"))
		openURL(nexusURL)
	case 2:
		appendLog(msgGetter("log_open_mods_folder"))
		openURL("file://" + filepath.ToSlash(modsDir))
	case 3:
		return false
	}
	return false
}

func EnsureModLoadOrder(window fyne.Window) {
	path := filepath.Join(getModsDir(), FileNameLoadOrder)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		appendLog(msgGetter("window_error_dsc_mlo_mis"))
		choice := showChoiceDialog(window, msgGetter("window_error_title"),
			msgGetter("window_error_dsc_mlo_mis"),
			msgGetter("btn_create_new"),
			msgGetter("btn_quit"),
		)
		if choice == 0 {
			os.WriteFile(path, []byte{}, 0644)
			appendLog(msgGetter("mod_load_order_created"))
		} else {
			os.Exit(0)
		}
	}
}

func isDisabledByNaming(name string) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "--") {
		return true
	}
	if strings.Contains(name, " - Copy") || strings.Contains(name, " — копия") {
		return true
	}
	return false
}

// Решение гонки 79
func CheckObsoleteMods(window fyne.Window) bool {
	// Копируем список устаревших модов под защитой мьютекса
	checksDataMutex.RLock()
	obsoleteCopy := make([]string, len(ObsoleteMods))
	copy(obsoleteCopy, ObsoleteMods)
	checksDataMutex.RUnlock()

	var found []string
	for _, mod := range obsoleteCopy {
		if FolderExists(mod) && !isDisabledByNaming(mod) {
			found = append(found, mod)
		}
	}
	if len(found) == 0 {
		appendLog(msgGetter("no_obsolete_found"))
		return true
	}
	appendLog(fmt.Sprintf(msgGetter("obsolete_found_list"), strings.Join(found, ", ")))
	choice := showChoiceDialog(window, msgGetter("obsolete_title"),
		msgGetter("obsolete_message")+"\n\n"+strings.Join(found, "\n"),
		msgGetter("skip"),
		msgGetter("delete_obsolete"),
	)
	if choice == 1 {
		for _, mod := range found {
			RemoveMod(mod)
			appendLog(fmt.Sprintf(msgGetter("deleted_mod"), mod))
		}
	}
	return true
}

func CheckMalformed(window fyne.Window) bool {
	var malformed []string
	for _, folder := range ListModFolders() {
		if folder == "base" || folder == "dmf" || isDisabledByNaming(folder) {
			continue
		}
		if isLikelyWrapper(folder) {
			malformed = append(malformed, folder)
		}
	}
	if len(malformed) == 0 {
		appendLog(msgGetter("no_malformed_found"))
		return true
	}
	appendLog(fmt.Sprintf(msgGetter("malformed_found_list"), strings.Join(malformed, ", ")))
	choice := showChoiceDialog(window, msgGetter("window_error_title"),
		msgGetter("window_error_dsc_mlfrmd")+"\n\n"+strings.Join(malformed, "\n")+"\n\n"+msgGetter("window_error_dsc_mlfrmd2"),
		msgGetter("skip"),
		msgGetter("btn_fix_malformed"),
	)
	if choice == 1 {
		for _, wrapper := range malformed {
			fixWrapper(wrapper)
		}
		appendLog(msgGetter("log_succ_malformed_fixed"))
	}
	return true
}

func isLikelyWrapper(folderName string) bool {
	if isDisabledByNaming(folderName) {
		return false
	}
	fullPath := filepath.Join(getModsDir(), folderName)
	if folderName == "base" || folderName == "dmf" {
		return false
	}
	if fileExists(filepath.Join(fullPath, "__folder_managed_by_vortex")) {
		return false
	}
	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return false
	}
	var subdirs []string
	hasModFile := false
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		} else if strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			hasModFile = true
		}
	}
	return len(subdirs) == 1 && !hasModFile
}

func fixWrapper(wrapper string) {
	dir := getModsDir()
	fullWrapper := filepath.Join(dir, wrapper)
	entries, err := os.ReadDir(fullWrapper)
	if err != nil || len(entries) == 0 {
		return
	}
	innerName := ""
	for _, e := range entries {
		if e.IsDir() {
			innerName = e.Name()
			break
		}
	}
	if innerName == "" || innerName == "base" || innerName == "dmf" {
		return
	}
	innerPath := filepath.Join(fullWrapper, innerName)
	targetPath := filepath.Join(dir, innerName)
	if FolderExists(innerName) {
		os.RemoveAll(targetPath)
	}
	os.Rename(innerPath, targetPath)
	os.RemoveAll(fullWrapper)
	appendLog(fmt.Sprintf(msgGetter("fixed_wrapper"), wrapper, innerName))
}

func CheckBrokenMods(window fyne.Window) bool {
	var broken []string
	for _, folder := range ListModFolders() {
		if folder == "base" || folder == "dmf" || isDisabledByNaming(folder) {
			continue
		}
		fullPath := filepath.Join(getModsDir(), folder)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			continue
		}
		hasModFile := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
				hasModFile = true
				break
			}
		}
		if !hasModFile {
			broken = append(broken, folder)
		}
	}
	if len(broken) == 0 {
		appendLog(msgGetter("no_broken_found"))
		return true
	}
	appendLog(fmt.Sprintf(msgGetter("broken_found_list"), strings.Join(broken, ", ")))
	choice := showChoiceDialog(window, msgGetter("broken_title"),
		msgGetter("broken_message")+"\n\n"+strings.Join(broken, "\n"),
		msgGetter("skip"),
		msgGetter("delete_broken"),
	)
	if choice == 1 {
		for _, mod := range broken {
			RemoveMod(mod)
			appendLog(fmt.Sprintf(msgGetter("deleted_mod"), mod))
		}
	}
	return true
}

func AutoFixMalformed() {
	for _, folder := range ListModFolders() {
		if folder == "base" || folder == "dmf" {
			continue
		}
		if isLikelyWrapper(folder) {
			fixWrapper(folder)
		}
	}
}

func CheckEmptyFolders(window fyne.Window) bool {
	var empty []string
	for _, folder := range ListModFolders() {
		fullPath := filepath.Join(getModsDir(), folder)
		if fileExists(filepath.Join(fullPath, "__folder_managed_by_vortex")) || isDisabledByNaming(folder) {
			continue
		}
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			continue
		}
		if len(entries) == 0 {
			empty = append(empty, folder)
		}
	}
	if len(empty) == 0 {
		appendLog(msgGetter("no_empty_found"))
		return true
	}
	appendLog(fmt.Sprintf(msgGetter("empty_found_list"), strings.Join(empty, ", ")))
	choice := showChoiceDialog(window, msgGetter("empty_folder_title"),
		msgGetter("empty_folder_message")+"\n\n"+strings.Join(empty, "\n"),
		msgGetter("skip"),
		msgGetter("delete_empty"),
	)
	if choice == 1 {
		dir := getModsDir()
		for _, folder := range empty {
			os.RemoveAll(filepath.Join(dir, folder))
		}
	}
	return true
}

// Решение гонки 79
func CheckIncompatible(window fyne.Window) bool {
	// Копируем список несовместимых пар под защитой мьютекса
	checksDataMutex.RLock()
	pairsCopy := make([]IncompatiblePair, len(IncompatiblePairs))
	copy(pairsCopy, IncompatiblePairs)
	checksDataMutex.RUnlock()

	skipped := make(map[string]bool)
	for {
		var found *IncompatiblePair
		for _, pair := range pairsCopy {
			if FolderExists(pair.Mod1) && FolderExists(pair.Mod2) &&
				isModActiveFunc != nil && isModActiveFunc(pair.Mod1) && isModActiveFunc(pair.Mod2) {
				key := pairKey(pair)
				if !skipped[key] {
					p := pair
					found = &p
					break
				}
			}
		}
		if found == nil {
			appendLog(msgGetter("no_incompatible_found"))
			return true
		}
		appendLog(fmt.Sprintf(msgGetter("incompatible_found_list"), found.Mod1, found.Mod2) + " - " + GetIncompatibleDesc(found.Mod1, found.Mod2))

		choice := showChoiceDialog(window, msgGetter("incompatible_title"),
			fmt.Sprintf(msgGetter("incompatible_desc"), found.Mod1, found.Mod2)+"\n"+GetIncompatibleDesc(found.Mod1, found.Mod2),
			msgGetter("skip"),
			fmt.Sprintf(msgGetter("delete_first"), found.Mod1),
			fmt.Sprintf(msgGetter("delete_second"), found.Mod2),
			msgGetter("skip_all"),
		)
		switch choice {
		case 0:
			skipped[pairKey(*found)] = true
		case 1:
			RemoveMod(found.Mod1)
			appendLog(fmt.Sprintf(msgGetter("deleted_mod"), found.Mod1))
			if refreshModListFunc != nil {
				refreshModListFunc()
			}
			time.Sleep(100 * time.Millisecond)
		case 2:
			RemoveMod(found.Mod2)
			appendLog(fmt.Sprintf(msgGetter("deleted_mod"), found.Mod2))
			if refreshModListFunc != nil {
				refreshModListFunc()
			}
			time.Sleep(100 * time.Millisecond)
		case 3:
			return true
		}
	}
}

func pairKey(pair IncompatiblePair) string {
	if pair.Mod1 < pair.Mod2 {
		return pair.Mod1 + "|" + pair.Mod2
	}
	return pair.Mod2 + "|" + pair.Mod1
}

func CheckDependencies(window fyne.Window) bool {
	// Копируем список зависимостей под защитой мьютекса
	checksDataMutex.RLock()
	depsCopy := make([]Dependency, len(Dependencies))
	copy(depsCopy, Dependencies)
	checksDataMutex.RUnlock()

	for {
		var found *Dependency
		for _, dep := range depsCopy {
			if isModActiveFunc != nil && isModActiveFunc(dep.Dependent) && !isModActiveFunc(dep.Required) {
				d := dep
				found = &d
				break
			}
		}
		if found == nil {
			appendLog(msgGetter("no_dependency_issues"))
			return true
		}
		appendLog(fmt.Sprintf(msgGetter("dependency_error_list"), found.Dependent, found.Required))
		choice := showChoiceDialog(window, msgGetter("dependency_title"),
			fmt.Sprintf(msgGetter("dependency_desc"), found.Dependent, found.Required),
			msgGetter("skip"),
			fmt.Sprintf(msgGetter("open_required_page"), found.Required),
			fmt.Sprintf(msgGetter("delete_dependent"), found.Dependent),
		)
		switch choice {
		case 1:
			openURL(found.RequiredURL)
			return false
		case 2:
			RemoveMod(found.Dependent)
			appendLog(fmt.Sprintf(msgGetter("deleted_mod"), found.Dependent))
			if refreshModListFunc != nil {
				refreshModListFunc()
			}
			time.Sleep(100 * time.Millisecond)
		case 0:
			return true
		}
	}
}

func PickLocalized(tr map[string]string, lang string) string {
	if tr == nil {
		return ""
	}
	if val, ok := tr[lang]; ok && val != "" {
		return val
	}
	if val, ok := tr["en"]; ok && val != "" {
		return val
	}
	for _, v := range tr {
		if v != "" {
			return v
		}
	}
	return ""
}

// Решение гонки 79
func GetIncompatibleDesc(mod1, mod2 string) string {
	// Копируем список пар под защитой мьютекса
	checksDataMutex.RLock()
	pairsCopy := make([]IncompatiblePair, len(IncompatiblePairs))
	copy(pairsCopy, IncompatiblePairs)
	checksDataMutex.RUnlock()

	lang := getCurrentLang() // один снимок на функцию
	for _, pair := range pairsCopy {
		if (pair.Mod1 == mod1 && pair.Mod2 == mod2) || (pair.Mod1 == mod2 && pair.Mod2 == mod1) {
			if pair.Desc != nil {
				if desc := PickLocalized(pair.Desc, lang); desc != "" {
					return desc
				}
			}
			name1 := getDisplayName(mod1, lang)
			name2 := getDisplayName(mod2, lang)
			templateKey := "conflict_template_includes"
			if pair.Type == "same" || pair.Type == "do_the_same" {
				templateKey = "conflict_template_same"
			}
			if msgGetter != nil {
				if tmpl := msgGetter(templateKey); tmpl != "" {
					return strings.ReplaceAll(strings.ReplaceAll(tmpl, "{mod1}", name1), "{mod2}", name2)
				}
			}
			return fmt.Sprintf("🔴 Conflict:\n%s  ⚔ %s.\nThese mods conflict with each other.", name1, name2)
		}
	}
	return ""
}

func IsAMLInstalled(modsDir string) bool {
	data, err := os.ReadFile(filepath.Join(modsDir, "base", "mod_manager.lua"))
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "aml_hook_load_order") ||
		strings.Contains(content, "AML IS MANAGING MOD LIST AND LOAD ORDER")
}

func WriteLoadOrderHeader(f *os.File, lang string) {
	fmt.Fprintln(f, msgGetter("load_order_header_title"))
	fmt.Fprintln(f, msgGetter("load_order_header_line1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule1_1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule2"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule2_1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule3"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule3_1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule4"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule4_1"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule5"))
	fmt.Fprintln(f, msgGetter("load_order_header_rule5_1"))
	fmt.Fprintln(f, msgGetter("load_order_header_discord"))
	fmt.Fprintln(f, msgGetter("load_order_header_nexus"))
	fmt.Fprintln(f, msgGetter("load_order_header_footer1"))
	fmt.Fprintln(f, msgGetter("load_order_header_footer2"))
	fmt.Fprintln(f, "")
}

func GetNexusFilePattern(folder string) string {
	modDBMutex.RLock()
	defer modDBMutex.RUnlock()
	if modDBMap == nil {
		return ""
	}
	if entry, ok := modDBMap[strings.ToLower(folder)]; ok && entry.NexusFilePattern != "" {
		return entry.NexusFilePattern
	}
	return ""
}

func SaveModDatabase() error {
	dir := getGlobalDataDir()
	if dir == "" {
		return fmt.Errorf("global data directory not set")
	}
	modDBMutex.RLock()
	if modDBMap == nil {
		modDBMutex.RUnlock()
		return fmt.Errorf("mod database is empty")
	}
	if len(modDBMap) < 5 {
		modDBMutex.RUnlock()
		return fmt.Errorf("mod database has only %d entries, refusing to save (possible data loss)", len(modDBMap))
	}
	// Копируем данные для сохранения
	var mods []ModDBEntry
	for _, entry := range modDBMap {
		mods = append(mods, *entry)
	}
	modDBMutex.RUnlock()

	sort.Slice(mods, func(i, j int) bool {
		return helpers.ExtractModIDFromURL(mods[i].URL) < helpers.ExtractModIDFromURL(mods[j].URL)
	})

	type modDatabaseFile struct {
		Version string       `json:"version"`
		Mods    []ModDBEntry `json:"mod_database"`
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")

	extVersionMutex.RLock()
	ver := externalVersion
	extVersionMutex.RUnlock()

	err := encoder.Encode(modDatabaseFile{Version: ver, Mods: mods})
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "mod_database.json")
	return os.WriteFile(path, buf.Bytes(), 0644)
}

func GetModDBEntry(folder string) *ModDBEntry {
	modDBMutex.RLock()
	defer modDBMutex.RUnlock()
	if modDBMap == nil {
		return nil
	}
	return modDBMap[strings.ToLower(folder)]
}

// getModDBEntryLocked - внутренняя функция, предполагает, что вызывающий уже взял блокировку,
// но для безопасности используем RLock.
func getModDBEntryLocked(folder string) *ModDBEntry {
	modDBMutex.RLock()
	defer modDBMutex.RUnlock()
	if modDBMap == nil {
		return nil
	}
	return modDBMap[strings.ToLower(folder)]
}

func UpdateModDBEntry(entry ModDBEntry) {
	modDBMutex.Lock()
	defer modDBMutex.Unlock()
	if modDBMap == nil {
		modDBMap = make(map[string]*ModDBEntry)
	}
	modDBMap[strings.ToLower(entry.Folder)] = &entry
}

func GetModDBList() []ModDBEntry {
	modDBMutex.RLock()
	defer modDBMutex.RUnlock()
	var list []ModDBEntry
	for _, e := range modDBMap {
		list = append(list, *e)
	}
	return list
}

func FolderExistsWithTimeout(name string, timeout time.Duration) bool {
	type result struct {
		exists bool
	}
	dir := getModsDir() // снимок до запуска горутины
	ch := make(chan result, 1)
	go func() {
		_, err := os.Stat(filepath.Join(dir, name))
		ch <- result{exists: err == nil}
	}()
	select {
	case res := <-ch:
		return res.exists
	case <-time.After(timeout):
		appendLog(fmt.Sprintf("Timeout checking %s, assuming missing", name))
		return false
	}
}

func getDisplayName(folder string, lang string) string {
	if entry := GetModDBEntry(folder); entry != nil {
		if name := PickLocalized(entry.Name, lang); name != "" {
			return name
		}
	}
	return folder
}

func GlobalDataDir() string { return getGlobalDataDir() }

func containsFolder(folders []string, name string) bool {
	for _, f := range folders {
		if f == name {
			return true
		}
	}
	return false
}

// GetIncompatiblePairs возвращает копию списка несовместимых пар (потокобезопасно).
func GetIncompatiblePairs() []IncompatiblePair {
	checksDataMutex.RLock()
	defer checksDataMutex.RUnlock()
	return append([]IncompatiblePair(nil), IncompatiblePairs...)
}

// IsIncompatibleMod проверяет, является ли мод с именем name несовместимым с любым другим установленным модом.
func IsIncompatibleMod(name string) bool {
	pairs := GetIncompatiblePairs()
	for _, pair := range pairs {
		if (pair.Mod1 == name || pair.Mod2 == name) && FolderExists(pair.Mod1) && FolderExists(pair.Mod2) {
			return true
		}
	}
	return false
}

// GetObsoleteMods возвращает копию списка устаревших модов.
// (нужно для правки из пункта 2 — если ещё не добавлено)
func GetObsoleteMods() []string {
	checksDataMutex.RLock()
	defer checksDataMutex.RUnlock()
	return append([]string(nil), ObsoleteMods...)
}

// GetMandatoryOrder возвращает копию списка обязательных модов.
func GetMandatoryOrder() []string {
	checksDataMutex.RLock()
	defer checksDataMutex.RUnlock()
	return append([]string(nil), MandatoryOrder...)
}

// GetDependencies возвращает копию списка зависимостей.
func GetDependencies() []Dependency {
	checksDataMutex.RLock()
	defer checksDataMutex.RUnlock()
	return append([]Dependency(nil), Dependencies...)
}

// GetLoadOrderRules возвращает копию списка правил порядка загрузки.
func GetLoadOrderRules() []LoadOrderRule {
	checksDataMutex.RLock()
	defer checksDataMutex.RUnlock()
	return append([]LoadOrderRule(nil), LoadOrderRules...)
}
