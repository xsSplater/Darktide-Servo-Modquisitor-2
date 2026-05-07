package checks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

var (
	appendLog        func(string)
	messages         *map[string]string
	showChoiceDialog func(fyne.Window, string, string, ...string) int
	openURL          func(string)
	modsDir          string

	// кэш базы модов (ключ – нижний регистр папки)
	modDBMap map[string]*ModDBEntry
)

func InitGlobals(
	logger func(string),
	msg *map[string]string,
	dialogFunc func(fyne.Window, string, string, ...string) int,
	urlOpener func(string),
	modsDirPath string,
) {
	appendLog = logger
	messages = msg
	showChoiceDialog = dialogFunc
	openURL = urlOpener
	modsDir = modsDirPath
}

var currentLang string

func SetLanguage(lang string) { currentLang = lang }

// SetModDatabase строит map для быстрого поиска
func SetModDatabase(entries []ModDBEntry) {
	modDBMap = make(map[string]*ModDBEntry, len(entries))
	for i := range entries {
		modDBMap[strings.ToLower(entries[i].Folder)] = &entries[i]
	}
}

// ---------- Правила порядка загрузки ----------
type LoadOrderRule struct {
	Mod    string `json:"mod"`
	Before string `json:"before"`
}

var LoadOrderRules []LoadOrderRule

// ModsDir возвращает путь к папке с модами.
func ModsDir() string {
	return modsDir
}

// FolderExists проверяет, существует ли папка с именем name внутри modsDir.
func FolderExists(name string) bool {
	info, err := os.Stat(filepath.Join(modsDir, name))
	if os.IsNotExist(err) {
		return false
	}
	return info != nil && info.IsDir()
}

// RemoveMod удаляет папку мода.
func RemoveMod(name string) { os.RemoveAll(filepath.Join(modsDir, name)) }

// ListModFolders возвращает список имён папок в modsDir.
func ListModFolders() []string {
	var folders []string
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if appendLog != nil {
			appendLog(fmt.Sprintf("Error reading mods directory: %v", err))
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

// ---------- Модели для UI ----------
type ModInfo struct {
	Name         string
	DisplayName  string
	ModTime      time.Time
	Active       bool
	Obsolete     bool
	Incompatible bool
	Mandatory    bool
	Broken       bool
	Description  string
	Author       string
	URL          string
	Note         string
}

// ModDBEntry – запись во внешней базе модов (mod_database.json)
type ModDBEntry struct {
	Folder      string            `json:"folder"`
	Name        map[string]string `json:"name"`
	Description map[string]string `json:"description"`
	Author      string            `json:"author"`
	URL         string            `json:"url"`
	Note        map[string]string `json:"note"`
}

// GetModsInfo теперь использует modDBMap
func GetModsInfo(lang string, forceEnglish bool) []ModInfo {
	folders := ListModFolders()
	var mods []ModInfo
	for _, name := range folders {
		if name == "base" || name == "dmf" {
			continue
		}
		fullPath := filepath.Join(modsDir, name)
		fi, err := os.Stat(fullPath)
		if err != nil {
			continue
		}
		mod := ModInfo{Name: name, ModTime: fi.ModTime(), Active: true}
		modFilePath := filepath.Join(fullPath, name+".mod")
		if modFileInfo, err := os.Stat(modFilePath); os.IsNotExist(err) {
			mod.Broken = true
		} else if err == nil {
			mod.ModTime = modFileInfo.ModTime()
		}

		// поиск в кэше базы
		if db, ok := modDBMap[strings.ToLower(name)]; ok {
			mod.Author = db.Author
			mod.URL = db.URL
			mod.Description = pickLocalized(db.Description, lang)
			mod.Note = pickLocalized(db.Note, lang)

			if forceEnglish {
				if enName := pickLocalized(db.Name, "en"); enName != "" {
					mod.DisplayName = enName
				}
			} else {
				if dn := pickLocalized(db.Name, lang); dn != "" {
					mod.DisplayName = dn
				}
			}
		}
		if mod.Description == "" {
			mod.Description = tryReadLocalization(name)
		}
		mods = append(mods, mod)
	}
	return mods
}

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

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

// ---------- Load Order ----------
type LoadOrderEntry struct {
	Name   string
	Active bool
}

func ReadLoadOrder() []LoadOrderEntry {
	data, err := os.ReadFile(filepath.Join(modsDir, "mod_load_order.txt"))
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
	f, err := os.Create(filepath.Join(modsDir, "mod_load_order.txt"))
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintln(f, "-- ▒Servo-Modquisitor▒ load order ▒")
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

// ---------- Внешние списки ----------
var (
	ObsoleteMods      []string
	IncompatiblePairs []IncompatiblePair
	Dependencies      []Dependency
	MandatoryOrder    []string
)

type ExternalData struct {
	MandatoryOrder    []string           `json:"mandatory_order"`
	ObsoleteMods      []string           `json:"obsolete_mods"`
	IncompatiblePairs []IncompatiblePair `json:"incompatible_pairs"`
	Dependencies      []Dependency       `json:"dependencies"`
	LoadOrder         []LoadOrderRule    `json:"load_order"`
}

type IncompatiblePair struct{ Mod1, Mod2, Desc string }
type Dependency struct{ Dependent, Required, RequiredURL string }

func LoadExternalLists(filename string) error {
	data, err := os.ReadFile(filepath.Join(modsDir, filename))
	if err != nil {
		return err
	}
	var ext ExternalData
	if err := json.Unmarshal(data, &ext); err != nil {
		return err
	}
	ObsoleteMods = ext.ObsoleteMods
	IncompatiblePairs = ext.IncompatiblePairs
	Dependencies = ext.Dependencies
	LoadOrderRules = ext.LoadOrder
	MandatoryOrder = ext.MandatoryOrder
	return nil
}

func IsMandatoryMod(name string) bool {
	for _, m := range MandatoryOrder {
		if m == name {
			return true
		}
	}
	return false
}

// ---------- Проверки ----------
func CheckInstallation(window fyne.Window) bool {
	if !FolderExists("base") {
		appendLog((*messages)["log_warn_base_missing"])
		return askMissing("base", "DML", "Darktide Mod Loader", "https://www.nexusmods.com/warhammer40kdarktide/mods/19", window)
	}
	if !FolderExists("dmf") {
		appendLog((*messages)["dmf_missing"])
		return askMissing("dmf", "DMF", "Darktide Mod Framework", "https://www.nexusmods.com/warhammer40kdarktide/mods/8", window)
	}
	appendLog((*messages)["step_install"])
	return true
}

func askMissing(folder, modAbbr, modName, nexusURL string, window fyne.Window) bool {
	choice := showChoiceDialog(window, (*messages)["window_error_title"],
		fmt.Sprintf((*messages)["window_error_dsc_dml_dmf"], folder, modName),
		(*messages)["btn_open_steam_guide"],
		fmt.Sprintf((*messages)["btn_open_nexus_for_mod"], modAbbr),
		(*messages)["open_mods_folder"],
		(*messages)["btn_abort"],
	)
	switch choice {
	case 0:
		appendLog((*messages)["log_open_steam_guide"])
		guideURL := "https://steamcommunity.com/sharedfiles/filedetails/?id=2953324027"
		if currentLang == "ru" {
			guideURL = "https://steamcommunity.com/sharedfiles/filedetails/?id=2950374474"
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
	path := filepath.Join(modsDir, "mod_load_order.txt")
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

func CheckObsoleteMods(window fyne.Window) bool {
	var found []string
	for _, mod := range ObsoleteMods {
		if FolderExists(mod) {
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
		if folder == "base" || folder == "dmf" {
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
	fullPath := filepath.Join(modsDir, folderName)
	if folderName == "base" || folderName == "dmf" {
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

// CheckBrokenMods – поиск папок без .mod файла и предложение удалить
func CheckBrokenMods(window fyne.Window) bool {
	var broken []string
	for _, folder := range ListModFolders() {
		if folder == "base" || folder == "dmf" {
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

// AutoFixMalformed – тихое исправление всех обёрток (после установки из ZIP)
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

// CheckIncompatible – итеративная проверка конфликтов, без рекурсии
func CheckIncompatible(window fyne.Window) bool {
	for {
		var found *IncompatiblePair
		for _, pair := range IncompatiblePairs {
			if FolderExists(pair.Mod1) && FolderExists(pair.Mod2) {
				p := pair
				found = &p
				break
			}
		}
		if found == nil {
			appendLog((*messages)["no_incompatible_found"])
			return true
		}
		appendLog(fmt.Sprintf((*messages)["incompatible_found_list"], found.Mod1, found.Mod2))
		choice := showChoiceDialog(window, (*messages)["incompatible_title"],
			fmt.Sprintf((*messages)["incompatible_desc"], found.Mod1, found.Mod2, found.Desc),
			(*messages)["skip"],
			fmt.Sprintf((*messages)["delete_first"], found.Mod1),
			fmt.Sprintf((*messages)["delete_second"], found.Mod2),
		)
		switch choice {
		case 1:
			RemoveMod(found.Mod1)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], found.Mod1))
		case 2:
			RemoveMod(found.Mod2)
			appendLog(fmt.Sprintf((*messages)["deleted_mod"], found.Mod2))
		case 0:
			// пропустить – прекращаем проверку
			return true
		}
	}
}

// CheckDependencies – итеративная проверка зависимостей
func CheckDependencies(window fyne.Window) bool {
	for {
		var found *Dependency
		for _, dep := range Dependencies {
			if FolderExists(dep.Dependent) && !FolderExists(dep.Required) {
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
		case 0:
			return true // пропустить
		}
	}
}

// pickLocalized выбирает строку для заданного языка с fallback-ом.
func pickLocalized(tr map[string]string, lang string) string {
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
