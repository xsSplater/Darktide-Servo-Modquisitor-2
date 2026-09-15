// Servo-Modquisitor-2/checks/checks_test.go
package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

// --- Вспомогательные функции ---

// setupTestEnv создаёт временную папку и устанавливает глобальные переменные.
// Возвращает путь к папке и функцию очистки.
func setupTestEnv(t *testing.T) (string, func()) {
	dir := t.TempDir()
	// Устанавливаем глобальные пути
	modsDir = dir
	profileDataDir = dir
	globalDataDir = dir

	// Сбрасываем глобальные переменные, которые могут влиять на тесты.
	// msgGetter должен быть не-nil, чтобы избежать паники в WriteLoadOrderHeader
	// и подобных местах. По умолчанию возвращает пустую строку.
	msgGetter = func(string) string { return "" }
	showChoiceDialog = nil
	openURL = nil
	appendLog = nil
	isModActiveFunc = nil
	refreshModListFunc = nil
	getInstalledAt = nil
	modDBMap = nil
	externalVersion = ""
	SetLanguage("en")
	LoadOrderRules = nil
	ObsoleteMods = nil
	IncompatiblePairs = nil
	Dependencies = nil
	MandatoryOrder = nil

	cleanup := func() {
		// Ничего не делаем, t.TempDir() очистится автоматически
	}
	return dir, cleanup
}

// writeDummyMod создаёт папку мода и .mod файл.
func writeDummyMod(t *testing.T, dir, folder, content string) string {
	modPath := filepath.Join(dir, folder)
	if err := os.MkdirAll(modPath, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", modPath, err)
	}
	if content != "" {
		modFile := filepath.Join(modPath, folder+".mod")
		if err := os.WriteFile(modFile, []byte(content), 0644); err != nil {
			t.Fatalf("write %s: %v", modFile, err)
		}
	}
	return modPath
}

// --- Тесты для базовых операций ---

func TestFolderExists(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Папка не существует
	if FolderExists("nonexistent") {
		t.Error("FolderExists returned true for nonexistent folder")
	}

	// Создаём папку
	folderPath := filepath.Join(dir, "testfolder")
	if err := os.Mkdir(folderPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !FolderExists("testfolder") {
		t.Error("FolderExists returned false for existing folder")
	}

	// Создаём файл (не папку)
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if FolderExists("file.txt") {
		t.Error("FolderExists returned true for a file")
	}
}

func TestListModFolders(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папки
	folders := []string{"mod1", "mod2", "mod3"}
	for _, f := range folders {
		if err := os.Mkdir(filepath.Join(dir, f), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", f, err)
		}
	}
	// Создаём файл (не папку)
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Создаём симлинк на папку (для проверки, что он тоже учитывается)
	if err := os.Symlink(filepath.Join(dir, "mod1"), filepath.Join(dir, "link_to_mod1")); err != nil {
		// На Windows может не работать, игнорируем
		t.Logf("symlink creation skipped: %v", err)
	}

	result := ListModFolders()
	expected := []string{"mod1", "mod2", "mod3"}
	// На Windows symlink может не работать, поэтому не проверяем его наличие
	// Просто проверяем, что все основные папки есть
	for _, f := range expected {
		found := false
		for _, r := range result {
			if r == f {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListModFolders missing expected folder %s", f)
		}
	}
	// Проверяем, что файл не попал
	for _, r := range result {
		if r == "file.txt" {
			t.Error("ListModFolders included file 'file.txt'")
		}
	}
}

func TestRemoveMod(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папку мода с содержимым
	folder := "testmod"
	modPath := filepath.Join(dir, folder)
	if err := os.Mkdir(modPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	subFile := filepath.Join(modPath, "file.txt")
	if err := os.WriteFile(subFile, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	RemoveMod(folder)
	if _, err := os.Stat(modPath); !os.IsNotExist(err) {
		t.Error("RemoveMod did not remove folder")
	}

	// Попытка удалить несуществующий мод не должна паниковать
	RemoveMod("nonexistent")
}

// --- Тесты для ReadLoadOrder и WriteLoadOrder ---

func TestReadWriteLoadOrder(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	entries := []LoadOrderEntry{
		{Name: "mod1", Active: true},
		{Name: "mod2", Active: false},
		{Name: "mod3", Active: true},
	}

	// Создаём папку mods внутри профиля
	modsFolder := filepath.Join(profileDataDir, "mods")
	if err := os.MkdirAll(modsFolder, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Записываем
	if err := WriteLoadOrder(entries); err != nil {
		t.Fatalf("WriteLoadOrder: %v", err)
	}

	// Читаем
	readEntries := ReadLoadOrder()
	if len(readEntries) != len(entries) {
		t.Fatalf("ReadLoadOrder returned %d entries, want %d", len(readEntries), len(entries))
	}
	for i, e := range entries {
		if readEntries[i].Name != e.Name || readEntries[i].Active != e.Active {
			t.Errorf("entry %d: got %+v, want %+v", i, readEntries[i], e)
		}
	}
}

// --- Тест IsMandatoryMod ---

func TestIsMandatoryMod(t *testing.T) {
	MandatoryOrder = []string{"base", "dmf", "core"}
	if !IsMandatoryMod("base") {
		t.Error("IsMandatoryMod('base') returned false")
	}
	if !IsMandatoryMod("dmf") {
		t.Error("IsMandatoryMod('dmf') returned false")
	}
	if IsMandatoryMod("other") {
		t.Error("IsMandatoryMod('other') returned true")
	}
}

// --- Тест PickLocalized ---

func TestPickLocalized(t *testing.T) {
	tr := map[string]string{
		"en": "English",
		"ru": "Русский",
	}
	if got := PickLocalized(tr, "en"); got != "English" {
		t.Errorf("PickLocalized(en) = %s, want English", got)
	}
	if got := PickLocalized(tr, "ru"); got != "Русский" {
		t.Errorf("PickLocalized(ru) = %s, want Русский", got)
	}
	if got := PickLocalized(tr, "de"); got != "English" {
		t.Errorf("PickLocalized(de) = %s, want English (fallback)", got)
	}
	if got := PickLocalized(nil, "en"); got != "" {
		t.Errorf("PickLocalized(nil) = %s, want empty", got)
	}
}

// --- Тест GetModsInfo (базовая часть без базы данных) ---

func TestGetModsInfoBasic(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папки модов
	mods := []string{"mod1", "mod2", "_disabled", "__disabled", "--disabled", "mod3 - Copy"}
	for _, m := range mods {
		writeDummyMod(t, modsDir, m, "")
	}
	// Добавляем .mod файл для mod1
	modFile := filepath.Join(modsDir, "mod1", "mod1.mod")
	if err := os.WriteFile(modFile, []byte(""), 0644); err != nil {
		t.Fatalf("write mod1.mod: %v", err)
	}

	// Создаём файл порядка для активации модов
	modsFolder := filepath.Join(profileDataDir, "mods")
	if err := os.MkdirAll(modsFolder, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	orderContent := "mod1\n-- mod2\nmod3\nmod4\n"
	if err := os.WriteFile(filepath.Join(modsFolder, "mod_load_order.txt"), []byte(orderContent), 0644); err != nil {
		t.Fatalf("write order: %v", err)
	}

	// Устанавливаем язык
	SetLanguage("en")

	// Вызываем GetModsInfo
	info := GetModsInfo("en", false)

	// Проверяем, что все моды есть
	foundNames := make(map[string]bool)
	for _, m := range info {
		foundNames[m.Name] = true
	}
	expectedMods := []string{"mod1", "mod2", "_disabled", "__disabled", "--disabled", "mod3 - Copy", "mod4"}
	for _, m := range expectedMods {
		if !foundNames[m] {
			t.Errorf("GetModsInfo missing mod %s", m)
		}
	}

	// Проверяем активность
	for _, m := range info {
		switch m.Name {
		case "mod1", "mod3":
			if !m.Active {
				t.Errorf("%s should be active", m.Name)
			}
		case "mod2", "_disabled", "__disabled", "--disabled", "mod3 - Copy":
			if m.Active {
				t.Errorf("%s should be inactive", m.Name)
			}
		}
		// Проверяем системные
		if m.Name == "base" || m.Name == "dmf" {
			t.Errorf("system mods should not appear")
		}
	}
}

// --- Тест CheckObsoleteMods с моком диалога ---

func TestCheckObsoleteMods(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём устаревшие моды
	ObsoleteMods = []string{"obsolete1", "obsolete2", "obsolete3"}
	for _, m := range ObsoleteMods {
		writeDummyMod(t, dir, m, "")
	}
	// Создаём обычный мод
	writeDummyMod(t, dir, "normal", "")

	// Мокаем showChoiceDialog так, чтобы он возвращал 1 (удалить)
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		return 1 // delete
	}
	// Мокаем appendLog, чтобы не падать
	appendLog = func(s string) {}

	// Вызываем CheckObsoleteMods
	result := CheckObsoleteMods(nil)
	if !result {
		t.Error("CheckObsoleteMods returned false")
	}
	// Проверяем, что устаревшие моды удалены
	for _, m := range ObsoleteMods {
		if FolderExists(m) {
			t.Errorf("obsolete mod %s was not removed", m)
		}
	}
	if !FolderExists("normal") {
		t.Error("normal mod was removed")
	}

	// Проверяем случай, когда выбор пользователя "skip" (0)
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		return 0 // skip
	}
	// Восстанавливаем устаревшие моды
	for _, m := range ObsoleteMods {
		writeDummyMod(t, dir, m, "")
	}
	result = CheckObsoleteMods(nil)
	if !result {
		t.Error("CheckObsoleteMods returned false on skip")
	}
	for _, m := range ObsoleteMods {
		if !FolderExists(m) {
			t.Errorf("obsolete mod %s was removed on skip", m)
		}
	}
}

// --- Тест CheckIncompatible с моком isModActiveFunc и диалога ---

func TestCheckIncompatible(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём конфликтующие моды
	IncompatiblePairs = []IncompatiblePair{
		{Mod1: "modA", Mod2: "modB", Desc: map[string]string{"en": "Conflict desc"}},
	}
	for _, m := range []string{"modA", "modB"} {
		writeDummyMod(t, modsDir, m, "")
	}

	// Мокаем isModActiveFunc: оба активны
	isModActiveFunc = func(name string) bool {
		return name == "modA" || name == "modB"
	}
	// Мокаем showChoiceDialog для первого конфликта: возвращаем 1 (удалить modA)
	callCount := 0
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		callCount++
		if callCount == 1 {
			return 1 // delete first (modA)
		}
		return 3 // skip all
	}
	appendLog = func(s string) {}
	refreshModListFunc = func() {}

	result := CheckIncompatible(nil)
	if !result {
		t.Error("CheckIncompatible returned false")
	}
	// Проверяем, что modA удалён
	if FolderExists("modA") {
		t.Error("modA was not removed")
	}
	if !FolderExists("modB") {
		t.Error("modB was removed")
	}

	// Проверяем, что диалог вызывался
	if callCount < 1 {
		t.Error("showChoiceDialog not called")
	}
}

// --- Тест CheckDependencies с моком isModActiveFunc ---

func TestCheckDependencies(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём моды
	Dependencies = []Dependency{
		{Dependent: "modC", Required: "modD", RequiredURL: "http://example.com"},
	}
	for _, m := range []string{"modC", "modD"} {
		writeDummyMod(t, dir, m, "")
	}

	// Мокаем isModActiveFunc: modC активен, modD неактивен
	isModActiveFunc = func(name string) bool {
		if name == "modC" {
			return true
		}
		return false
	}
	// Мокаем showChoiceDialog: сначала возвращаем 2 (удалить зависимый), затем 0 (skip)
	callCount := 0
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		callCount++
		if callCount == 1 {
			return 2 // delete dependent (modC)
		}
		return 0 // skip
	}
	appendLog = func(s string) {}
	refreshModListFunc = func() {}

	result := CheckDependencies(nil)
	if !result {
		t.Error("CheckDependencies returned false")
	}
	// Проверяем, что modC удалён
	if FolderExists("modC") {
		t.Error("modC was not removed")
	}
	if !FolderExists("modD") {
		t.Error("modD was removed")
	}

	// Проверяем случай, когда зависимость не обнаружена (обе активны)
	isModActiveFunc = func(name string) bool {
		return true
	}
	// Восстанавливаем modC
	writeDummyMod(t, dir, "modC", "")
	result = CheckDependencies(nil)
	if !result {
		t.Error("CheckDependencies returned false when no issue")
	}
	if !FolderExists("modC") {
		t.Error("modC was removed incorrectly")
	}
}

// --- Тест CheckInstallation (базовый, без диалогов) ---

func TestCheckInstallation(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Без папок base и dmf
	// Мокаем appendLog
	appendLog = func(s string) {}
	// Мокаем showChoiceDialog: возвращаем 3 (cancel) чтобы прервать
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		return 3 // cancel
	}

	result := CheckInstallation(nil)
	if result {
		t.Error("CheckInstallation should return false when base missing and user cancels")
	}

	// Создаём папку base, но не dmf
	if err := os.Mkdir(filepath.Join(dir, "base"), 0755); err != nil {
		t.Fatalf("mkdir base: %v", err)
	}
	result = CheckInstallation(nil)
	if result {
		t.Error("CheckInstallation should return false when dmf missing and user cancels")
	}

	// Создаём dmf
	if err := os.Mkdir(filepath.Join(dir, "dmf"), 0755); err != nil {
		t.Fatalf("mkdir dmf: %v", err)
	}
	// Мокаем showChoiceDialog: возвращаем 0 (это не должно вызваться, потому что папки есть)
	showChoiceDialog = nil // сбрасываем, чтобы тест упал, если вызовется
	result = CheckInstallation(nil)
	if !result {
		t.Error("CheckInstallation returned false when base and dmf exist")
	}
}

// --- Тест TryFixMismatchedModFolder (экспортированная) ---

func TestTryFixMismatchedModFolder(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папку мода с неправильным именем (не совпадает с .mod файлом)
	folder := "wrongname"
	modPath := filepath.Join(dir, folder)
	if err := os.Mkdir(modPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Создаём .mod файл с именем "correctname.mod"
	modFile := filepath.Join(modPath, "correctname.mod")
	if err := os.WriteFile(modFile, []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	appendLog = func(s string) {}

	newName := TryFixMismatchedModFolder(modPath, folder)
	if newName != "correctname" {
		t.Errorf("TryFixMismatchedModFolder returned %s, want correctname", newName)
	}
	// Проверяем, что папка переименована
	if FolderExists("wrongname") {
		t.Error("wrongname folder still exists")
	}
	if !FolderExists("correctname") {
		t.Error("correctname folder not created")
	}
	// Проверяем, что .mod файл перемещён
	if _, err := os.Stat(filepath.Join(dir, "correctname", "correctname.mod")); err != nil {
		t.Error(".mod file not found in renamed folder")
	}

	// Проверяем, что функция не трогает папки с правильным именем
	folder2 := "goodname"
	modPath2 := filepath.Join(dir, folder2)
	if err := os.Mkdir(modPath2, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	modFile2 := filepath.Join(modPath2, "goodname.mod")
	if err := os.WriteFile(modFile2, []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	newName2 := TryFixMismatchedModFolder(modPath2, folder2)
	if newName2 != "" {
		t.Errorf("TryFixMismatchedModFolder returned %s, want empty for correct folder", newName2)
	}
	if !FolderExists("goodname") {
		t.Error("goodname folder was renamed incorrectly")
	}
}

// --- Тест для FolderExistsWithTimeout ---

func TestFolderExistsWithTimeout(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папку
	folder := "test"
	if err := os.Mkdir(filepath.Join(dir, folder), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	appendLog = func(s string) {}

	exists := FolderExistsWithTimeout(folder, 1*time.Second)
	if !exists {
		t.Error("FolderExistsWithTimeout returned false for existing folder")
	}

	exists = FolderExistsWithTimeout("nonexistent", 1*time.Second)
	if exists {
		t.Error("FolderExistsWithTimeout returned true for nonexistent folder")
	}
}

// --- Тест для getDisplayName ---

func TestGetDisplayName(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём базу данных
	modDBMap = map[string]*ModDBEntry{
		"testmod": {
			Folder: "testmod",
			Name:   map[string]string{"en": "Test Mod", "ru": "Тестовый Мод"},
		},
	}
	name := getDisplayName("testmod", "en")
	if name != "Test Mod" {
		t.Errorf("getDisplayName(en) = %s, want Test Mod", name)
	}
	name = getDisplayName("testmod", "ru")
	if name != "Тестовый Мод" {
		t.Errorf("getDisplayName(ru) = %s, want Тестовый Мод", name)
	}
	name = getDisplayName("unknown", "en")
	if name != "unknown" {
		t.Errorf("getDisplayName(unknown) = %s, want unknown", name)
	}
}

// --- Тесты для isLikelyWrapper, fixWrapper, AutoFixMalformed ---

func TestIsLikelyWrapper(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// 1. Папка с одной подпапкой и без .mod файла - обёртка
	wrapperDir := filepath.Join(modsDir, "wrapper")
	if err := os.Mkdir(wrapperDir, 0755); err != nil {
		t.Fatalf("mkdir wrapper: %v", err)
	}
	subDir := filepath.Join(wrapperDir, "subfolder")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir subfolder: %v", err)
	}
	if !isLikelyWrapper("wrapper") {
		t.Error("isLikelyWrapper should return true for folder with one subdir and no .mod")
	}

	// 2. Папка с несколькими подпапками - не обёртка
	wrapperDir2 := filepath.Join(modsDir, "wrapper2")
	if err := os.Mkdir(wrapperDir2, 0755); err != nil {
		t.Fatalf("mkdir wrapper2: %v", err)
	}
	if err := os.Mkdir(filepath.Join(wrapperDir2, "sub1"), 0755); err != nil {
		t.Fatalf("mkdir sub1: %v", err)
	}
	if err := os.Mkdir(filepath.Join(wrapperDir2, "sub2"), 0755); err != nil {
		t.Fatalf("mkdir sub2: %v", err)
	}
	if isLikelyWrapper("wrapper2") {
		t.Error("isLikelyWrapper should return false for folder with multiple subdirs")
	}

	// 3. Папка с .mod файлом - не обёртка
	wrapperDir3 := filepath.Join(modsDir, "wrapper3")
	if err := os.Mkdir(wrapperDir3, 0755); err != nil {
		t.Fatalf("mkdir wrapper3: %v", err)
	}
	if err := os.Mkdir(filepath.Join(wrapperDir3, "sub"), 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	modFile := filepath.Join(wrapperDir3, "wrapper3.mod")
	if err := os.WriteFile(modFile, []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	if isLikelyWrapper("wrapper3") {
		t.Error("isLikelyWrapper should return false for folder with .mod file")
	}

	// 4. Папка с именем base или dmf - не обёртка (системные)
	if isLikelyWrapper("base") {
		t.Error("isLikelyWrapper should return false for base folder")
	}
	if isLikelyWrapper("dmf") {
		t.Error("isLikelyWrapper should return false for dmf folder")
	}

	// 5. Папка с симлинком (не директория) - не обёртка
	// Создаём симлинк на папку, но сама папка не является обёрткой
	linkPath := filepath.Join(modsDir, "symlink_wrapper")
	if err := os.Symlink(subDir, linkPath); err == nil {
		// Если симлинк создался, проверяем, что isLikelyWrapper возвращает false
		if isLikelyWrapper("symlink_wrapper") {
			t.Error("isLikelyWrapper should return false for symlink")
		}
		os.Remove(linkPath)
	}
}

func TestFixWrapper(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём обёртку: wrapper -> inner (папка с .mod файлом)
	wrapperName := "wrapper"
	wrapperPath := filepath.Join(modsDir, wrapperName)
	if err := os.Mkdir(wrapperPath, 0755); err != nil {
		t.Fatalf("mkdir wrapper: %v", err)
	}
	innerName := "inner_mod"
	innerPath := filepath.Join(wrapperPath, innerName)
	if err := os.Mkdir(innerPath, 0755); err != nil {
		t.Fatalf("mkdir inner: %v", err)
	}
	// Создаём .mod файл внутри inner
	modFile := filepath.Join(innerPath, innerName+".mod")
	if err := os.WriteFile(modFile, []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	// Создаём какой-то файл внутри inner, чтобы проверить копирование
	dataFile := filepath.Join(innerPath, "data.txt")
	if err := os.WriteFile(dataFile, []byte("test"), 0644); err != nil {
		t.Fatalf("write data: %v", err)
	}

	appendLog = func(s string) {}
	fixWrapper(wrapperName)

	// Проверяем, что inner_mod появился в корне modsDir
	if !FolderExists(innerName) {
		t.Errorf("inner_mod not moved to root")
	}
	// Проверяем, что файлы внутри inner_mod сохранены
	if !fileExists(filepath.Join(modsDir, innerName, innerName+".mod")) {
		t.Errorf(".mod file not found in inner_mod")
	}
	if !fileExists(filepath.Join(modsDir, innerName, "data.txt")) {
		t.Errorf("data.txt not found in inner_mod")
	}
	// Проверяем, что папка wrapper удалена
	if FolderExists(wrapperName) {
		t.Errorf("wrapper folder still exists")
	}
}

func TestAutoFixMalformed(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём две обёртки
	wrappers := []string{"wrap1", "wrap2"}
	for _, w := range wrappers {
		wPath := filepath.Join(modsDir, w)
		if err := os.Mkdir(wPath, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", w, err)
		}
		inner := filepath.Join(wPath, w+"_inner")
		if err := os.Mkdir(inner, 0755); err != nil {
			t.Fatalf("mkdir inner: %v", err)
		}
		// .mod файл
		if err := os.WriteFile(filepath.Join(inner, w+"_inner.mod"), []byte(""), 0644); err != nil {
			t.Fatalf("write mod: %v", err)
		}
	}

	appendLog = func(s string) {}
	AutoFixMalformed()

	for _, w := range wrappers {
		expectedName := w + "_inner"
		if !FolderExists(expectedName) {
			t.Errorf("%s not moved to root", expectedName)
		}
		if FolderExists(w) {
			t.Errorf("%s still exists", w)
		}
	}
}

func TestTryFixMismatchedModFolder_Ambiguous(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	folder := "testmod"
	modPath := filepath.Join(modsDir, folder)
	if err := os.Mkdir(modPath, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Создаём два .mod файла (неоднозначность)
	if err := os.WriteFile(filepath.Join(modPath, "a.mod"), []byte(""), 0644); err != nil {
		t.Fatalf("write a.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modPath, "b.mod"), []byte(""), 0644); err != nil {
		t.Fatalf("write b.mod: %v", err)
	}
	appendLog = func(s string) {}

	newName := TryFixMismatchedModFolder(modPath, folder)
	if newName != "" {
		t.Errorf("TryFixMismatchedModFolder returned %s, want empty (ambiguous)", newName)
	}
	// Папка не должна быть переименована
	if !FolderExists(folder) {
		t.Error("folder was renamed incorrectly")
	}
}

// --- Тесты для CheckMalformed, CheckBrokenMods, CheckEmptyFolders ---

func TestCheckMalformed(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папку-обёртку
	wrapperName := "bad_mod"
	wrapperPath := filepath.Join(modsDir, wrapperName)
	if err := os.Mkdir(wrapperPath, 0755); err != nil {
		t.Fatalf("mkdir bad_mod: %v", err)
	}
	innerName := "real_mod"
	innerPath := filepath.Join(wrapperPath, innerName)
	if err := os.Mkdir(innerPath, 0755); err != nil {
		t.Fatalf("mkdir real_mod: %v", err)
	}
	// .mod файл внутри inner
	if err := os.WriteFile(filepath.Join(innerPath, innerName+".mod"), []byte(""), 0644); err != nil {
		t.Fatalf("write mod: %v", err)
	}

	appendLog = func(s string) {}
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		// Выбираем "Fix all" (индекс 1)
		return 1
	}
	refreshModListFunc = func() {}

	result := CheckMalformed(nil)
	if !result {
		t.Error("CheckMalformed returned false")
	}
	// Проверяем, что папка исправлена
	if FolderExists(wrapperName) {
		t.Errorf("wrapper folder still exists")
	}
	if !FolderExists(innerName) {
		t.Errorf("inner folder not moved to root")
	}
}

func TestCheckBrokenMods(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём папку мода без .mod файла
	brokenName := "broken_mod"
	brokenPath := filepath.Join(modsDir, brokenName)
	if err := os.Mkdir(brokenPath, 0755); err != nil {
		t.Fatalf("mkdir broken_mod: %v", err)
	}
	// Создаём нормальный мод с .mod
	normalName := "normal_mod"
	normalPath := filepath.Join(modsDir, normalName)
	if err := os.Mkdir(normalPath, 0755); err != nil {
		t.Fatalf("mkdir normal_mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(normalPath, normalName+".mod"), []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	// Создаём папку _disabled (не должна считаться сломанной)
	disabledName := "_disabled"
	disabledPath := filepath.Join(modsDir, disabledName)
	if err := os.Mkdir(disabledPath, 0755); err != nil {
		t.Fatalf("mkdir _disabled: %v", err)
	}

	appendLog = func(s string) {}
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		// Выбираем "Delete All Broken" (индекс 1)
		return 1
	}
	refreshModListFunc = func() {}

	result := CheckBrokenMods(nil)
	if !result {
		t.Error("CheckBrokenMods returned false")
	}
	// Проверяем, что broken_mod удалён
	if FolderExists(brokenName) {
		t.Errorf("broken_mod was not removed")
	}
	// Проверяем, что normal_mod и _disabled остались
	if !FolderExists(normalName) {
		t.Errorf("normal_mod was removed")
	}
	if !FolderExists(disabledName) {
		t.Errorf("_disabled was removed")
	}
}

func TestCheckEmptyFolders(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём пустую папку
	emptyName := "empty_mod"
	emptyPath := filepath.Join(modsDir, emptyName)
	if err := os.Mkdir(emptyPath, 0755); err != nil {
		t.Fatalf("mkdir empty_mod: %v", err)
	}
	// Создаём непустую папку
	nonEmptyName := "non_empty"
	nonEmptyPath := filepath.Join(modsDir, nonEmptyName)
	if err := os.Mkdir(nonEmptyPath, 0755); err != nil {
		t.Fatalf("mkdir non_empty: %v", err)
	}
	// Кладём файл в non_empty
	if err := os.WriteFile(filepath.Join(nonEmptyPath, "file.txt"), []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	// Создаём папку с префиксом _ (должна игнорироваться)
	prefixedName := "_prefixed"
	prefixedPath := filepath.Join(modsDir, prefixedName)
	if err := os.Mkdir(prefixedPath, 0755); err != nil {
		t.Fatalf("mkdir _prefixed: %v", err)
	}

	appendLog = func(s string) {}
	showChoiceDialog = func(window fyne.Window, title, message string, options ...string) int {
		// Выбираем "Delete empty" (индекс 1)
		return 1
	}
	refreshModListFunc = func() {}

	result := CheckEmptyFolders(nil)
	if !result {
		t.Error("CheckEmptyFolders returned false")
	}
	// Проверяем, что empty_mod удалён
	if FolderExists(emptyName) {
		t.Errorf("empty_mod was not removed")
	}
	// Проверяем, что non_empty и _prefixed остались
	if !FolderExists(nonEmptyName) {
		t.Errorf("non_empty was removed")
	}
	if !FolderExists(prefixedName) {
		t.Errorf("_prefixed was removed")
	}
}

// #5: заметка о disabled-префиксе не должна теряться, если мод
// также присутствует в mod_database.json со своей заметкой.
func TestGetModsInfoKeepsPrefixNoteWithDBNote(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	// Перебиваем msgGetter после setupTestEnv — он сбрасывает его в пустую функцию.
	trans := map[string]string{
		"note_disabled_prefix":        "Disabled: prefix _",
		"note_disabled_prefix_double": "Disabled: prefix __",
		"note_backup_copy":            "Disabled: backup copy",
	}
	msgGetter = func(key string) string { return trans[key] }

	// Мод с префиксом _ и записью в БД (со своей заметкой).
	writeDummyMod(t, modsDir, "_legacy", "")

	// SetModDatabase потокобезопасен и берёт modDBMutex — то, что нужно.
	SetModDatabase([]ModDBEntry{
		{
			Folder: "_legacy",
			Note:   map[string]string{"en": "Superseded by legacy2"},
		},
	})

	info := GetModsInfo("en", false)

	var found *ModInfo
	for i := range info {
		if info[i].Name == "_legacy" {
			found = &info[i]
			break
		}
	}
	if found == nil {
		t.Fatal("_legacy not found in GetModsInfo result")
	}
	if found.Active {
		t.Errorf("_legacy should be inactive (has _ prefix)")
	}
	if !strings.Contains(found.Note, "Disabled: prefix") {
		t.Errorf("Note lost structural reason: %q", found.Note)
	}
	if !strings.Contains(found.Note, "Superseded by legacy2") {
		t.Errorf("Note lost DB note: %q", found.Note)
	}
}

// #5: если БД-заметки нет, структурная не должна стать пустой
// (и наоборот — мод без префикса получает только БД-заметку).
func TestGetModsInfoNoteFallbacks(t *testing.T) {
	_, cleanup := setupTestEnv(t)
	defer cleanup()

	trans := map[string]string{
		"note_disabled_prefix": "Disabled: prefix _",
	}
	msgGetter = func(key string) string { return trans[key] }

	writeDummyMod(t, modsDir, "_onlyprefix", "")
	writeDummyMod(t, modsDir, "onlydb", "")

	SetModDatabase([]ModDBEntry{
		{
			Folder: "onlydb",
			Note:   map[string]string{"en": "From database"},
		},
	})

	info := GetModsInfo("en", false)

	byName := make(map[string]string, len(info))
	for _, m := range info {
		byName[m.Name] = m.Note
	}

	// Только структурная (нет БД).
	if got := byName["_onlyprefix"]; got != "Disabled: prefix _" {
		t.Errorf("_onlyprefix Note = %q, want %q", got, "Disabled: prefix _")
	}
	// Только БД (нет префикса).
	if got := byName["onlydb"]; got != "From database" {
		t.Errorf("onlydb Note = %q, want %q", got, "From database")
	}
}

// Прямая проверка JoinNotes: склейка с разделителем, игнор пустых,
// trim пробелов.
func TestJoinNotes(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"a", "b"}, "a · b"},
		{[]string{"a", "", "b"}, "a · b"},
		{[]string{"", ""}, ""},
		{[]string{"  a  ", " b "}, "a · b"},
		{[]string{"a"}, "a"},
		{[]string{}, ""},
		{[]string{"  ", ""}, ""},
		{[]string{"only"}, "only"},
	}
	for _, tc := range cases {
		got := JoinNotes(tc.in...)
		if got != tc.want {
			t.Errorf("JoinNotes(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Сценарий, эквивалентный refreshModList: заметка от префикса +
// текст о конфликте. Должны быть обе части и разделитель " · ".
func TestJoinNotesPrefixAndConflict(t *testing.T) {
	got := JoinNotes("Disabled: prefix _", "Conflict with: Foo")
	want := "Disabled: prefix _ · Conflict with: Foo"
	if got != want {
		t.Errorf("JoinNotes = %q, want %q", got, want)
	}
	// Если структурной заметки нет — должна остаться только конфликтная.
	got = JoinNotes("", "Conflict with: Foo")
	if got != "Conflict with: Foo" {
		t.Errorf("JoinNotes(empty, conflict) = %q, want %q", got, "Conflict with: Foo")
	}
}
