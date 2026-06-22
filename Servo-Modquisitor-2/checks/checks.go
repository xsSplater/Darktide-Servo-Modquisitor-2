// checks.go
package checks

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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

var (
	appendLog        func(string)
	messages         *map[string]string
	showChoiceDialog func(fyne.Window, string, string, ...string) int
	openURL          func(string)
	modsDir          string
	isModActiveFunc  func(string) bool

	modDBMap        map[string]*ModDBEntry
	externalVersion string
)

func InitGlobals(
	logger func(string),
	msg *map[string]string,
	dialogFunc func(fyne.Window, string, string, ...string) int,
	urlOpener func(string),
	modsDirPath string,
	isActiveFn func(string) bool,
	refreshFn func(),
) {
	appendLog = logger
	messages = msg
	showChoiceDialog = dialogFunc
	openURL = urlOpener
	modsDir = modsDirPath
	isModActiveFunc = isActiveFn
	refreshModListFunc = refreshFn
}

var refreshModListFunc func()

var currentLang string

func SetLanguage(lang string) { currentLang = lang }

func SetModDatabase(entries []ModDBEntry) {
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

func ModsDir() string { return modsDir }

func FolderExists(name string) bool {
	info, err := os.Stat(filepath.Join(modsDir, name))
	if os.IsNotExist(err) {
		return false
	}
	return info != nil && info.IsDir()
}

func RemoveMod(name string) { os.RemoveAll(filepath.Join(modsDir, name)) }

func ListModFolders() []string {
	var folders []string
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if appendLog != nil {
			appendLog(fmt.Sprintf((*messages)["log_error_reading_mods_dir"], err))
		}
		return folders
	}
	for _, e := range entries {
		if e.IsDir() {
			folders = append(folders, e.Name())
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
	Author            string
	Description       string
	DisplayName       string
	Name              string
	Note              string
	URL               string
	GitHubURL         string
	ModTime           time.Time
	NexusVersion      string
	NexusSummary      string
	NexusDownloads    int
	NexusEndorsements int
	NexusPictureURL   string
}

type ModDBEntry struct {
	Folder           string            `json:"folder"`
	NexusFilePattern string            `json:"nexus_file_pattern"`
	Name             map[string]string `json:"name"`
	Description      map[string]string `json:"description"`
	Author           string            `json:"author"`
	URL              string            `json:"url"`
	GitHubURL        string            `json:"github_url"`
	Note             map[string]string `json:"note"`
}

func GetModsInfo(lang string, forceEnglish bool) []ModInfo {
	folders := ListModFolders()
	var mods []ModInfo
	for _, name := range folders {
		fullPath := filepath.Join(modsDir, name)
		fi, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		mod := ModInfo{Name: name, Active: true}

		// Обработка специальных префиксов и суффиксов: отключаем мод и добавляем примечание
		if messages != nil {
			if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "__") {
				mod.Active = false
				mod.Note = (*messages)["note_disabled_prefix"]
			} else if strings.HasPrefix(name, "--") {
				mod.Active = false
				mod.Note = (*messages)["note_disabled_prefix_double"]
			} else if strings.Contains(name, " - Copy") || strings.Contains(name, " — копия") {
				mod.Active = false
				mod.Note = (*messages)["note_backup_copy"]
			}
		}

		// Исключение мода Mod List Dividers с разделителями ___System, ___Scoreboard и т.д.
		if contains(modListDividers, name) {
			mod.Active = true
			mod.Note = ""
		}

		mod.VortexDeployed = fileExists(filepath.Join(fullPath, "__folder_managed_by_vortex"))

		switch {
		case name == "base":
			mod.IsSystem = true
			mod.Active = false
			mod.ModTime = getModTimeFromFile(filepath.Join(fullPath, "mod_manager.lua"))
		case name == "dmf":
			mod.IsSystem = true
			mod.Active = false
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
					// Пытаемся исправить несовпадение имени папки и .mod файла
					newName := tryFixMismatchedModFolder(fullPath, name)
					if newName != "" {
						// Обновляем имя мода и путь
						mod.Name = newName
						name = newName
						fullPath = filepath.Join(modsDir, name)
						modFilePath = filepath.Join(fullPath, name+".mod")
						// Перечитываем время модификации папки
						if fi, err := os.Stat(fullPath); err == nil {
							mod.ModTime = fi.ModTime()
						}
						// Возможно, после переименования появился Lua-файл? Проверяем ещё раз
						luaPaths := []string{
							filepath.Join(fullPath, name+".lua"),
							filepath.Join(fullPath, "scripts", "mods", name, name+".lua"),
						}
						for _, lp := range luaPaths {
							if t := getModTimeFromFile(lp); !t.IsZero() {
								mod.ModTime = t
								foundLua = true
								break
							}
						}
					}
				}

				// Теперь проверяем .mod файл уже с учётом возможного переименования
				if !fileExists(modFilePath) && !foundLua {
					mod.Broken = true
					// Если нет .mod и нет Lua - значит мод точно сломан
				} else {
					mod.Broken = false
				}
				// Если папка отключена префиксом - не считаем её сломанной
				if strings.HasPrefix(mod.Name, "_") || strings.HasPrefix(mod.Name, "__") || strings.HasPrefix(mod.Name, "--") {
					mod.Broken = false
				}

				// Устанавливаем время модификации, если ещё не установлено
				if mod.ModTime.IsZero() {
					if modFileInfo, err := os.Stat(modFilePath); err == nil {
						mod.ModTime = modFileInfo.ModTime()
					} else {
						mod.ModTime = fi.ModTime()
					}
				}
			}
		}

		if db, ok := modDBMap[strings.ToLower(name)]; ok && db.Folder != "" {
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
		if mod.Description == "" {
			mod.Description = tryReadLocalization(name)
		}
		mods = append(mods, mod)
	}
	// Проверяем автопатчер
	if _, err := os.Stat(filepath.Join(modsDir, "..", "binaries", "plugins", "_dt_mod_autopatch.dll")); err == nil {
		mod := ModInfo{
			Name:     "autopatch",
			IsSystem: true,
			Active:   false,
			// Описание и прочее будет подтянуто ниже из modDBMap
		}
		dllPath := filepath.Join(modsDir, "..", "binaries", "plugins", "_dt_mod_autopatch.dll")
		mod.ModTime = getModTimeFromFile(dllPath)

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

// tryFixMismatchedModFolder проверяет, есть ли в папке ровно один .mod файл,
// имя которого не совпадает с именем папки. Если да - переименовывает папку
// в имя этого .mod файла (без расширения) и возвращает новое имя папки.
// Если переименование не удалось или условий нет - возвращает пустую строку.
func tryFixMismatchedModFolder(folderPath, currentName string) string {
	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return ""
	}
	var modFile string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			if modFile == "" {
				modFile = e.Name()
			} else {
				// больше одного .mod файла - не трогаем
				return ""
			}
		}
	}
	if modFile == "" {
		return "" // нет .mod файла вообще
	}
	expectedName := strings.TrimSuffix(modFile, ".mod")
	if expectedName == currentName {
		return "" // уже правильно
	}
	// Переименовываем папку
	newPath := filepath.Join(filepath.Dir(folderPath), expectedName)
	if err := os.Rename(folderPath, newPath); err != nil {
		appendLog(fmt.Sprintf("Failed to auto-rename folder %s -> %s: %v", currentName, expectedName, err))
		return ""
	}
	appendLog(fmt.Sprintf("Auto-renamed mismatched mod folder: %s -> %s", currentName, expectedName))
	return expectedName
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func tryReadLocalization(modName string) string {
	dir := filepath.Join(modsDir, modName)
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
	data, err := os.ReadFile(filepath.Join(modsDir, FileNameLoadOrder))
	if err != nil {
		return nil
	}
	var entries []LoadOrderEntry
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || (strings.HasPrefix(line, "--") && !strings.HasPrefix(line, "-- ")) {
			continue
		}
		active := true
		name := line
		if strings.HasPrefix(line, "-- ") {
			active = false
			name = strings.TrimPrefix(line, "-- ")
		}
		if name != "" && name != "base" && name != "dmf" {
			entries = append(entries, LoadOrderEntry{Name: name, Active: active})
		}
	}
	return entries
}

func WriteLoadOrder(entries []LoadOrderEntry) error {
	f, err := os.Create(filepath.Join(modsDir, FileNameLoadOrder))
	if err != nil {
		return err
	}
	defer f.Close()
	WriteLoadOrderHeader(f, currentLang)
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

// type IncompatiblePair struct{ Mod1, Mod2, Desc string }
type IncompatiblePair struct {
	Mod1 string            `json:"mod1"`
	Mod2 string            `json:"mod2"`
	Desc map[string]string `json:"desc"`
	Type string            `json:"type,omitempty"` // "includes" (по умолчанию), "same", или пусто
}
type Dependency struct{ Dependent, Required, RequiredURL string }

func LoadExternalLists(filename string) error {
	fullPath := filepath.Join(modsDir, filename)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", fullPath, err)
	}
	var ext ExternalData
	if err := json.Unmarshal(data, &ext); err != nil {
		// Пытаемся дать максимум информации о месте ошибки
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
			// Вычисляем строку и столбец
			line, col = 1, 1
			for i := 0; i < offset; i++ {
				if data[i] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			// Берём контекст вокруг ошибки
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
	externalVersion = ext.Version
	ObsoleteMods = ext.ObsoleteMods
	IncompatiblePairs = ext.IncompatiblePairs
	Dependencies = ext.Dependencies
	LoadOrderRules = ext.LoadOrder
	MandatoryOrder = ext.MandatoryOrder
	return nil
}

func GetExternalVersion() string { return externalVersion }

func IsMandatoryMod(name string) bool {
	for _, m := range MandatoryOrder {
		if m == name {
			return true
		}
	}
	return false
}

func CheckInstallation(window fyne.Window) bool {
	if !FolderExistsWithTimeout("base", 3*time.Second) {
		appendLog((*messages)["log_warn_base_missing"])
		return askMissing("base", "DML", "Darktide Mod Loader", "https://www.nexusmods.com/warhammer40kdarktide/mods/19", window)
	}
	if !FolderExistsWithTimeout("dmf", 3*time.Second) {
		appendLog((*messages)["dmf_missing"])
		return askMissing("dmf", "DMF", "Darktide Mod Framework", "https://www.nexusmods.com/warhammer40kdarktide/mods/8", window)
	}
	appendLog((*messages)["step_install"])
	return true
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func askMissing(folder, modAbbr, modName, nexusURL string, window fyne.Window) bool {
	choice := showChoiceDialog(window, (*messages)["window_error_title"],
		fmt.Sprintf((*messages)["window_error_dsc_dml_dmf"], folder, modName),
		(*messages)["btn_open_steam_guide"],
		fmt.Sprintf((*messages)["btn_open_nexus_for_mod"], modAbbr),
		(*messages)["open_mods_folder"],
		(*messages)["btn_cancel"],
	)
	switch choice {
	case 0:
		appendLog((*messages)["log_open_steam_guide"])
		guideURL := steamGuideEnURL
		if currentLang == "ru" {
			guideURL = steamGuideRuURL
		}
		openURL(guideURL)
	case 1:
		appendLog((*messages)["log_open_nexus_page"])
		openURL(nexusURL)
	case 2:
		appendLog((*messages)["log_open_mods_folder"])
		openURL("file://" + filepath.ToSlash(modsDir))
	case 3:
		return false
	}
	return false
}

func EnsureModLoadOrder(window fyne.Window) {
	path := filepath.Join(modsDir, FileNameLoadOrder)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		appendLog((*messages)["window_error_dsc_mlo_mis"])
		choice := showChoiceDialog(window, (*messages)["window_error_title"],
			(*messages)["window_error_dsc_mlo_mis"],
			(*messages)["btn_create_new"],
			(*messages)["btn_quit"],
		)
		if choice == 0 {
			os.WriteFile(path, []byte{}, 0644)
			appendLog((*messages)["mod_load_order_created"])
		} else {
			os.Exit(0)
		}
	}
}

// isDisabledByNaming возвращает true, если папка отключена через префикс/суффикс
// (игнорируется при проверках на удаление).
func isDisabledByNaming(name string) bool {
	if strings.HasPrefix(name, "_") || strings.HasPrefix(name, "--") {
		return true
	}
	if strings.Contains(name, " - Copy") || strings.Contains(name, " — копия") {
		return true
	}
	return false
}

func CheckObsoleteMods(window fyne.Window) bool {
	var found []string
	for _, mod := range ObsoleteMods {
		if FolderExists(mod) && !isDisabledByNaming(mod) {
			found = append(found, mod)
		}
	}
	if len(found) == 0 {
		appendLog((*messages)["no_obsolete_found"])
		return true
	}
	appendLog(fmt.Sprintf((*messages)["obsolete_found_list"], strings.Join(found, ", ")))
	choice := showChoiceDialog(window, (*messages)["obsolete_title"],
		(*messages)["obsolete_message"]+"\n\n"+strings.Join(found, "\n"),
		(*messages)["skip"],
		(*messages)["delete_obsolete"],
	)
	if choice == 1 {
		for _, mod := range found {
			RemoveMod(mod)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], mod))
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
		appendLog((*messages)["no_malformed_found"])
		return true
	}
	appendLog(fmt.Sprintf((*messages)["malformed_found_list"], strings.Join(malformed, ", ")))
	choice := showChoiceDialog(window, (*messages)["window_error_title"],
		(*messages)["window_error_dsc_mlfrmd"]+"\n\n"+strings.Join(malformed, "\n")+"\n\n"+(*messages)["window_error_dsc_mlfrmd2"],
		(*messages)["skip"],
		(*messages)["btn_fix_malformed"],
	)
	if choice == 1 {
		for _, wrapper := range malformed {
			fixWrapper(wrapper)
		}
		appendLog((*messages)["log_succ_malformed_fixed"])
	}
	return true
}

func isLikelyWrapper(folderName string) bool {
	if isDisabledByNaming(folderName) {
		return false
	}
	fullPath := filepath.Join(modsDir, folderName)
	if folderName == "base" || folderName == "dmf" {
		return false
	}
	// Vortex-управляемые папки не считаем ошибочными обёртками
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
	fullWrapper := filepath.Join(modsDir, wrapper)
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
	targetPath := filepath.Join(modsDir, innerName)
	if FolderExists(innerName) {
		os.RemoveAll(targetPath)
	}
	os.Rename(innerPath, targetPath)
	os.RemoveAll(fullWrapper)
	appendLog(fmt.Sprintf((*messages)["fixed_wrapper"], wrapper, innerName))
}

func CheckBrokenMods(window fyne.Window) bool {
	var broken []string
	for _, folder := range ListModFolders() {
		if folder == "base" || folder == "dmf" || isDisabledByNaming(folder) {
			continue
		}
		if !fileExists(filepath.Join(modsDir, folder, folder+".mod")) {
			broken = append(broken, folder)
		}
	}
	if len(broken) == 0 {
		appendLog((*messages)["no_broken_found"])
		return true
	}
	appendLog(fmt.Sprintf((*messages)["broken_found_list"], strings.Join(broken, ", ")))
	choice := showChoiceDialog(window, (*messages)["broken_title"],
		(*messages)["broken_message"]+"\n\n"+strings.Join(broken, "\n"),
		(*messages)["skip"],
		(*messages)["delete_broken"],
	)
	if choice == 1 {
		for _, mod := range broken {
			RemoveMod(mod)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], mod))
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
		fullPath := filepath.Join(modsDir, folder)
		if fileExists(filepath.Join(fullPath, "__folder_managed_by_vortex")) || isDisabledByNaming(folder) {
			continue // это не пустая папка, а отключённый мод Vortex
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
		appendLog((*messages)["no_empty_found"])
		return true
	}
	appendLog(fmt.Sprintf((*messages)["empty_found_list"], strings.Join(empty, ", ")))
	choice := showChoiceDialog(window, (*messages)["empty_folder_title"],
		(*messages)["empty_folder_message"]+"\n\n"+strings.Join(empty, "\n"),
		(*messages)["skip"],
		(*messages)["delete_empty"],
	)
	if choice == 1 {
		for _, folder := range empty {
			os.RemoveAll(filepath.Join(modsDir, folder))
			appendLog(fmt.Sprintf((*messages)["deleted_empty_folder"], folder))
		}
	}
	return true
}

func CheckIncompatible(window fyne.Window) bool {
	skipped := make(map[string]bool)
	for {
		var found *IncompatiblePair
		for _, pair := range IncompatiblePairs {
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
			appendLog((*messages)["no_incompatible_found"])
			return true
		}
		appendLog(fmt.Sprintf((*messages)["incompatible_found_list"], found.Mod1, found.Mod2) + " - " + GetIncompatibleDesc(found.Mod1, found.Mod2))

		choice := showChoiceDialog(window, (*messages)["incompatible_title"],
			fmt.Sprintf((*messages)["incompatible_desc"], found.Mod1, found.Mod2)+"\n"+GetIncompatibleDesc(found.Mod1, found.Mod2),
			(*messages)["skip"],
			fmt.Sprintf((*messages)["delete_first"], found.Mod1),
			fmt.Sprintf((*messages)["delete_second"], found.Mod2),
			(*messages)["skip_all"],
		)
		switch choice {
		case 0: // Skip - пропускаем эту пару, запоминаем
			skipped[pairKey(*found)] = true
			// продолжаем цикл, найдутся другие пары
		case 1:
			RemoveMod(found.Mod1)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], found.Mod1))
			if refreshModListFunc != nil {
				refreshModListFunc()
			}
			time.Sleep(100 * time.Millisecond)
		case 2:
			RemoveMod(found.Mod2)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], found.Mod2))
			if refreshModListFunc != nil {
				refreshModListFunc()
			}
			time.Sleep(100 * time.Millisecond)
		case 3: // Skip All - полностью выходим из проверок несовместимости
			return true
		}
	}
}

// pairKey создаёт уникальный ключ для пары модов (независимо от порядка).
func pairKey(pair IncompatiblePair) string {
	if pair.Mod1 < pair.Mod2 {
		return pair.Mod1 + "|" + pair.Mod2
	}
	return pair.Mod2 + "|" + pair.Mod1
}

func CheckDependencies(window fyne.Window) bool {
	for {
		var found *Dependency
		for _, dep := range Dependencies {
			// зависимый мод активен, а требуемый неактивен (или отсутствует)
			if isModActiveFunc != nil && isModActiveFunc(dep.Dependent) && !isModActiveFunc(dep.Required) {
				d := dep
				found = &d
				break
			}
		}
		if found == nil {
			appendLog((*messages)["no_dependency_issues"])
			return true
		}
		appendLog(fmt.Sprintf((*messages)["dependency_error_list"], found.Dependent, found.Required))
		choice := showChoiceDialog(window, (*messages)["dependency_title"],
			fmt.Sprintf((*messages)["dependency_desc"], found.Dependent, found.Required),
			(*messages)["skip"],
			fmt.Sprintf((*messages)["open_required_page"], found.Required),
			fmt.Sprintf((*messages)["delete_dependent"], found.Dependent),
		)
		switch choice {
		case 1:
			openURL(found.RequiredURL)
			return false
		case 2:
			RemoveMod(found.Dependent)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], found.Dependent))
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

// GetIncompatibleDesc возвращает локализованное описание конфликта для пары модов.
//
//	func GetIncompatibleDesc(mod1, mod2 string) string {
//		for _, pair := range IncompatiblePairs {
//			if (pair.Mod1 == mod1 && pair.Mod2 == mod2) || (pair.Mod1 == mod2 && pair.Mod2 == mod1) {
//				desc := PickLocalized(pair.Desc, currentLang)
//
//	if appendLog != nil {
//		   appendLog(fmt.Sprintf("DEBUG: GetIncompatibleDesc found desc='%s'", desc))
//	}
//
//				return desc
//			}
//		}
//		return ""
//	}
func GetIncompatibleDesc(mod1, mod2 string) string {
	for _, pair := range IncompatiblePairs {
		if (pair.Mod1 == mod1 && pair.Mod2 == mod2) || (pair.Mod1 == mod2 && pair.Mod2 == mod1) {
			// Если есть готовое описание - используем его
			if pair.Desc != nil {
				if desc := PickLocalized(pair.Desc, currentLang); desc != "" {
					return desc
				}
			}
			// Иначе генерируем по шаблону
			name1 := getDisplayName(mod1, currentLang)
			name2 := getDisplayName(mod2, currentLang)
			// Выбираем шаблон в зависимости от типа конфликта
			templateKey := "conflict_template_includes" // по умолчанию "includes functionality"
			if pair.Type == "same" || pair.Type == "do_the_same" {
				templateKey = "conflict_template_same"
			}
			if messages != nil {
				if tmpl, ok := (*messages)[templateKey]; ok {
					return strings.ReplaceAll(strings.ReplaceAll(tmpl, "{mod1}", name1), "{mod2}", name2)
				}
			}
			// Fallback на английский, если сообщения нет
			return fmt.Sprintf("🔴 Conflict:\n%s  ⚔ %s.\nThese mods conflict with each other.", name1, name2)
		}
	}
	return ""
}

// IsAMLInstalled проверяет, модифицирован ли mod_manager.lua модом Auto Mod Loading and Ordering
func IsAMLInstalled(modsDir string) bool {
	data, err := os.ReadFile(filepath.Join(modsDir, "base", "mod_manager.lua"))
	if err != nil {
		return false
	}
	content := string(data)
	// Ключевые фразы, уникальные для AML
	return strings.Contains(content, "aml_hook_load_order") ||
		strings.Contains(content, "AML IS MANAGING MOD LIST AND LOAD ORDER")
}

// WriteLoadOrderHeader записывает подробный заголовок в файл порядка загрузки
func WriteLoadOrderHeader(f *os.File, lang string) {
	// Все необходимые ключи уже есть в messages.json с переводами на все языки
	fmt.Fprintln(f, (*messages)["load_order_header_title"])
	fmt.Fprintln(f, (*messages)["load_order_header_line1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule1_1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule2"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule2_1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule3"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule3_1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule4"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule4_1"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule5"])
	fmt.Fprintln(f, (*messages)["load_order_header_rule5_1"])
	fmt.Fprintln(f, (*messages)["load_order_header_discord"])
	fmt.Fprintln(f, (*messages)["load_order_header_nexus"])
	fmt.Fprintln(f, (*messages)["load_order_header_footer1"])
	fmt.Fprintln(f, (*messages)["load_order_header_footer2"])
	fmt.Fprintln(f, "")
}

// GetNexusFilePattern возвращает nexus_file_pattern для папки мода (или пустую строку)
func GetNexusFilePattern(folder string) string {
	if modDBMap == nil {
		return ""
	}
	if entry, ok := modDBMap[strings.ToLower(folder)]; ok && entry.NexusFilePattern != "" {
		return entry.NexusFilePattern
	}
	return ""
}

// SaveModDatabase сохраняет текущее состояние modDBMap в файл mod_database.json.
func SaveModDatabase() error {
	if modDBMap == nil {
		return fmt.Errorf("mod database is empty")
	}
	// Если записей меньше 5 — скорее всего, база повреждена, не сохраняем
	if len(modDBMap) < 5 {
		return fmt.Errorf("mod database has only %d entries, refusing to save (possible data loss)", len(modDBMap))
	}
	type modDatabaseFile struct {
		Version string       `json:"version"`
		Mods    []ModDBEntry `json:"mod_database"`
	}
	var mods []ModDBEntry
	for _, entry := range modDBMap {
		mods = append(mods, *entry)
	}
	sort.Slice(mods, func(i, j int) bool {
		return modIDFromURL(mods[i].URL) < modIDFromURL(mods[j].URL)
	})

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false) // отключаем экранирование <, >, &
	encoder.SetIndent("", "\t")
	err := encoder.Encode(modDatabaseFile{Version: externalVersion, Mods: mods})
	if err != nil {
		return err
	}
	path := filepath.Join(modsDir, "mod_database.json")
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// GetModDBEntry возвращает запись из базы модов по folder (если есть).
func GetModDBEntry(folder string) *ModDBEntry {
	if modDBMap == nil {
		return nil
	}
	return modDBMap[strings.ToLower(folder)]
}

// UpdateModDBEntry обновляет или добавляет запись в памяти.
func UpdateModDBEntry(entry ModDBEntry) {
	if modDBMap == nil {
		modDBMap = make(map[string]*ModDBEntry)
	}
	modDBMap[strings.ToLower(entry.Folder)] = &entry
}

// GetModDBList возвращает срез всех записей (для передачи в main).
func GetModDBList() []ModDBEntry {
	var list []ModDBEntry
	for _, e := range modDBMap {
		list = append(list, *e)
	}
	return list
}

func modIDFromURL(rawURL string) int {
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	if len(parts) < 2 {
		return 0
	}
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return id
}

// FolderExistsWithTimeout используется только для критических проверок при старте
func FolderExistsWithTimeout(name string, timeout time.Duration) bool {
	type result struct {
		exists bool
	}
	ch := make(chan result, 1)
	go func() {
		_, err := os.Stat(filepath.Join(modsDir, name))
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

// getDisplayName возвращает читаемое имя мода на языке lang (с учётом ForceEnglish в будущем можно расширить, но здесь используем просто lang)
func getDisplayName(folder string, lang string) string {
	if entry, ok := modDBMap[strings.ToLower(folder)]; ok {
		if name := PickLocalized(entry.Name, lang); name != "" {
			return name
		}
	}
	return folder // fallback на техническое имя
}
