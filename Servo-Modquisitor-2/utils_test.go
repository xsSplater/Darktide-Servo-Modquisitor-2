// Servo-Modquisitor-2/utils_test.go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"file.zip", "file.zip", false},
		{"path/to/file.zip", "file.zip", false}, // filepath.Base извлекает file.zip
		{"", "", true},
		{".", "", true},
		{"..", "", true},
		{"file with spaces.zip", "file with spaces.zip", false},
		{"file:name.zip", "file:name.zip", false},
		{"file/with/slash.zip", "slash.zip", false},           // исправлено: ожидаем имя файла
		{"file\\with\\backslash.zip", "backslash.zip", false}, // исправлено
		{"C:\\Windows\\System32", "System32", false},          // исправлено
	}
	for _, tt := range tests {
		got, err := sanitizeFilename(tt.input)
		if tt.err {
			if err == nil {
				t.Errorf("sanitizeFilename(%q) expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("sanitizeFilename(%q) error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestExtractVersionAndModIDFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		modID    int
		version  string
		ok       bool
	}{
		{"ModName 123 1.0.0 20250101 abcdef.zip", 123, "1.0.0", true},
		{"ModName 456 2.3 20250101 abcdef.zip", 456, "2.3", true},
		{"ModName 789 1.2.3 20250101 abcdef.7z", 789, "1.2.3", true},
		{"ModName 1000 unknown 20250101 abcdef.zip", 1000, "unknown", true},
		{"ModName 1000 20250101 abcdef.zip", 1000, "unknown", true},
		{"ModName 1000", 0, "", false},
		{"1000 1.0.0", 1000, "1.0.0", true},
		{"ModName 1000 1.0.0", 1000, "1.0.0", true},
		{"ModName 1000 1.0.0 20250101", 1000, "1.0.0", true},
		{"ModName 1000 1.0.0.zip", 1000, "1.0.0", true},
		{"ModName 1000 1.0.0 20250101 abcdef", 1000, "1.0.0", true},
		{"ModName 0 1.0.0", 0, "", false},
	}
	for _, tt := range tests {
		gotID, gotVersion, gotOk := extractVersionAndModIDFromFilename(tt.filename)
		if gotOk != tt.ok {
			t.Errorf("extractVersionAndModIDFromFilename(%q) ok = %v, want %v", tt.filename, gotOk, tt.ok)
			continue
		}
		if gotOk {
			if gotID != tt.modID {
				t.Errorf("extractVersionAndModIDFromFilename(%q) modID = %d, want %d", tt.filename, gotID, tt.modID)
			}
			if gotVersion != tt.version {
				t.Errorf("extractVersionAndModIDFromFilename(%q) version = %q, want %q", tt.filename, gotVersion, tt.version)
			}
		}
	}
}

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		{"123", true},
		{"0", true},
		{"", false},
		{"12a", false},
		{"-1", false},
		{"1.0", false},
	}
	for _, tt := range tests {
		got := isNumeric(tt.s)
		if got != tt.want {
			t.Errorf("isNumeric(%q) = %v, want %v", tt.s, got, tt.want)
		}
	}
}

func TestExtractPatternFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		{"modname_v1.0.0.zip", "modname"},
		{"MODNAME 1.0.0.zip", "modname"},
		{"some_mod 2.0.zip", "some_mod"},
		{"file.ext", "file"},
		{"", ""},
		{"   leading spaces.zip", "leading"},
	}
	for _, tt := range tests {
		got := extractPatternFromFilename(tt.filename)
		if got != tt.want {
			t.Errorf("extractPatternFromFilename(%q) = %q, want %q", tt.filename, got, tt.want)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.9.0", "1.10.0", -1},
		{"1.10.0", "1.9.0", 1},
		{"2.0", "1.0", 1},
		{"1.0", "1.0.0", 0},
		{"", "1.0", 0},
	}
	for _, tt := range tests {
		got := compareVersions(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}

func TestIsModsEnabledLegacy(t *testing.T) {
	dir := t.TempDir()
	// Создаём папку bundle и файл бэкапа (означает, что моды включены)
	bundleDir := filepath.Join(dir, "bundle")
	if err := os.Mkdir(bundleDir, 0755); err != nil {
		t.Fatalf("mkdir bundle: %v", err)
	}
	bakFile := filepath.Join(bundleDir, "bundle_database.data.bak")
	if err := os.WriteFile(bakFile, []byte("backup"), 0644); err != nil {
		t.Fatalf("write bak: %v", err)
	}
	if !isModsEnabledLegacy(dir) {
		t.Error("isModsEnabledLegacy returned false when backup exists")
	}

	// Удаляем бэкап — моды выключены
	if err := os.Remove(bakFile); err != nil {
		t.Fatalf("remove bak: %v", err)
	}
	if isModsEnabledLegacy(dir) {
		t.Error("isModsEnabledLegacy returned true when backup missing")
	}

	// Пустой gameRoot
	if isModsEnabledLegacy("") {
		t.Error("isModsEnabledLegacy returned true for empty gameRoot")
	}
}

// TestEnsureDir проверяет создание папок с защитой от удаления .mod файлов.
func TestEnsureDir(t *testing.T) {
	app := &App{}

	dir := t.TempDir()

	// 1. Создание новой папки
	testDir := filepath.Join(dir, "test")
	if err := app.ensureDir(testDir); err != nil {
		t.Fatalf("ensureDir failed: %v", err)
	}
	if _, err := os.Stat(testDir); os.IsNotExist(err) {
		t.Errorf("directory not created")
	}

	// 2. Замена файла на папку (обычный файл)
	filePath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(filePath, []byte("data"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := app.ensureDir(filePath); err != nil {
		t.Fatalf("ensureDir over file failed: %v", err)
	}
	if info, err := os.Stat(filePath); err != nil || !info.IsDir() {
		t.Errorf("file was not replaced by directory")
	}

	// 3. .mod файл - не должен быть удалён
	modFile := filepath.Join(dir, "mod.mod")
	if err := os.WriteFile(modFile, []byte(""), 0644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}
	if err := app.ensureDir(modFile); err == nil {
		t.Errorf("ensureDir should not remove .mod file")
	}
	if _, err := os.Stat(modFile); os.IsNotExist(err) {
		t.Errorf(".mod file was removed")
	}

	// 4. Рекурсивное создание вложенных папок
	nested := filepath.Join(dir, "a", "b", "c")
	if err := app.ensureDir(nested); err != nil {
		t.Fatalf("ensureDir nested failed: %v", err)
	}
	if _, err := os.Stat(nested); os.IsNotExist(err) {
		t.Errorf("nested directory not created")
	}
}

// TestCopyPath проверяет рекурсивное копирование файлов и папок.
func TestCopyPath(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	// Создаём структуру src
	subDir := filepath.Join(srcDir, "sub")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	file1 := filepath.Join(srcDir, "file1.txt")
	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	file2 := filepath.Join(subDir, "file2.txt")
	if err := os.WriteFile(file2, []byte("world"), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	// Копируем srcDir в dstDir
	if err := copyPath(srcDir, dstDir); err != nil {
		t.Fatalf("copyPath failed: %v", err)
	}

	// Проверяем, что файлы скопированы
	if data, err := os.ReadFile(filepath.Join(dstDir, "file1.txt")); err != nil || string(data) != "hello" {
		t.Errorf("file1.txt not copied correctly")
	}
	if data, err := os.ReadFile(filepath.Join(dstDir, "sub", "file2.txt")); err != nil || string(data) != "world" {
		t.Errorf("sub/file2.txt not copied correctly")
	}

	// Проверяем, что папка sub существует
	if _, err := os.Stat(filepath.Join(dstDir, "sub")); os.IsNotExist(err) {
		t.Errorf("sub directory not copied")
	}
}

// TestCleanupModsFolder проверяет удаление нежелательных файлов.
func TestCleanupModsFolder(t *testing.T) {
	dir := t.TempDir()

	// Создаём файлы, которые должны быть удалены
	unwanted := []string{
		FileNameModDatabase,
		FileNameMandatoryRules,
		AppName + ".exe",
	}
	for _, f := range unwanted {
		path := filepath.Join(dir, f)
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	// Создаём обычный файл, который не должен быть удалён
	keepFile := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(keepFile, []byte("keep"), 0644); err != nil {
		t.Fatalf("write keep.txt: %v", err)
	}

	cleanupModsFolder(dir)

	for _, f := range unwanted {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("file %s should have been removed", f)
		}
	}
	if _, err := os.Stat(keepFile); os.IsNotExist(err) {
		t.Errorf("keep.txt should not have been removed")
	}
}
