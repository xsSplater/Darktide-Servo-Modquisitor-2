// Servo-Modquisitor-2/sorter/sorter_test.go
package sorter

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"Servo-Modquisitor/checks"
)

// --- Вспомогательные функции ---

// setupTestEnv создаёт временную папку и устанавливает глобальные переменные.
// Возвращает путь к папке и функцию очистки.
func setupTestEnv(t *testing.T) (string, func()) {
	dir := t.TempDir()

	// Устанавливаем глобальные переменные
	folderExists = func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	listModFolders = func() []string {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil
		}
		var folders []string
		for _, e := range entries {
			if e.IsDir() {
				folders = append(folders, e.Name())
			}
		}
		return folders
	}
	logFunc = func(s string) {}
	mandatoryOrder = []string{}
	loadOrderRules = []checks.LoadOrderRule{}
	dependencies = []ModDependency{}
	sortWarningRu = ""
	sortWarningEn = ""
	logCreateMLOT = "Creating load order..."
	logMLOTCreated = "Load order created."
	writeHeaderFunc = nil
	loadOrderOutputPath = filepath.Join(dir, "mod_load_order.txt")

	cleanup := func() {
		// Ничего не делаем, t.TempDir() очистится автоматически
	}
	return dir, cleanup
}

// --- Тест topologicalSort ---

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name     string
		mods     []string
		deps     []ModDependency
		expected []string
	}{
		{
			name: "simple linear",
			mods: []string{"A", "B", "C"},
			deps: []ModDependency{
				{Required: "A", Dependent: "B"},
				{Required: "B", Dependent: "C"},
			},
			expected: []string{"A", "B", "C"},
		},
		{
			name: "diamond",
			mods: []string{"A", "B", "C", "D"},
			deps: []ModDependency{
				{Required: "A", Dependent: "B"},
				{Required: "A", Dependent: "C"},
				{Required: "B", Dependent: "D"},
				{Required: "C", Dependent: "D"},
			},
			expected: []string{"A", "B", "C", "D"}, // может быть A,C,B,D — но stable? Проверим
		},
		{
			name:     "no deps",
			mods:     []string{"X", "Y", "Z"},
			deps:     []ModDependency{},
			expected: []string{"X", "Y", "Z"}, // алфавитный порядок
		},
		{
			name:     "empty mods",
			mods:     []string{},
			deps:     []ModDependency{},
			expected: nil,
		},
		{
			name:     "single mod",
			mods:     []string{"A"},
			deps:     []ModDependency{},
			expected: []string{"A"},
		},
		{
			name: "cycle ignored (topologicalSort does not detect cycles)",
			mods: []string{"A", "B"},
			deps: []ModDependency{
				{Required: "A", Dependent: "B"},
				{Required: "B", Dependent: "A"},
			},
			expected: nil, // topologicalSort вернёт nil, так как ни одна вершина не имеет indegree 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := topologicalSort(tt.mods, tt.deps)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("topologicalSort(%v, %v) = %v, want %v", tt.mods, tt.deps, result, tt.expected)
			}
		})
	}
}

// --- Тест readSortOrder ---

func TestReadSortOrder(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём файл с порядком
	content := `
# Comment line
mod1
mod2
# Another comment
mod3

mod4
`
	filePath := filepath.Join(dir, "sort_order.txt")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result := readSortOrder(filePath)
	expected := []string{"mod1", "mod2", "mod3", "mod4"}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("readSortOrder = %v, want %v", result, expected)
	}

	// Несуществующий файл
	result = readSortOrder(filepath.Join(dir, "nonexistent.txt"))
	if result != nil {
		t.Errorf("readSortOrder(nonexistent) = %v, want nil", result)
	}
}

// --- Тест LoadSortOrders ---

func TestLoadSortOrders(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Создаём файлы
	russianContent := "ru_mod1\nru_mod2\n"
	englishContent := "en_mod1\nen_mod2\n"
	if err := os.WriteFile(filepath.Join(dir, "russian_sort_order.txt"), []byte(russianContent), 0644); err != nil {
		t.Fatalf("write russian: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "english_sort_order.txt"), []byte(englishContent), 0644); err != nil {
		t.Fatalf("write english: %v", err)
	}

	LoadSortOrders(dir)
	expectedRu := []string{"ru_mod1", "ru_mod2"}
	expectedEn := []string{"en_mod1", "en_mod2"}
	if !reflect.DeepEqual(cachedRussianOrder, expectedRu) {
		t.Errorf("cachedRussianOrder = %v, want %v", cachedRussianOrder, expectedRu)
	}
	if !reflect.DeepEqual(cachedEnglishOrder, expectedEn) {
		t.Errorf("cachedEnglishOrder = %v, want %v", cachedEnglishOrder, expectedEn)
	}
}

// --- Тест CreateLoadOrderFromActive ---

func TestCreateLoadOrderFromActive(t *testing.T) {
	dir, cleanup := setupTestEnv(t)
	defer cleanup()

	// Устанавливаем обязательный порядок
	mandatoryOrder = []string{"core", "base"}

	// Создаём зависимости
	dependencies = []ModDependency{
		{Required: "core", Dependent: "modA"},
		{Required: "modA", Dependent: "modB"},
	}

	// Правила загрузки
	loadOrderRules = []checks.LoadOrderRule{
		{Before: "modC", After: "modB"},
	}

	// Создаём папки для модов (чтобы folderExists работал)
	for _, m := range []string{"core", "base", "modA", "modB", "modC", "modD"} {
		if err := os.Mkdir(filepath.Join(dir, m), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", m, err)
		}
	}

	// Устанавливаем кастомный порядок
	cachedRussianOrder = []string{"modD", "modC"}
	cachedEnglishOrder = []string{"modC", "modD"}

	// Активные моды
	activeMods := []string{"modA", "modB", "modC", "modD", "core"}

	// Вызываем функцию
	CreateLoadOrderFromActive(activeMods, "ru")

	// Проверяем выходной файл
	data, err := os.ReadFile(loadOrderOutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	expectedOrder := []string{"core", "modD", "modC", "modA", "modB"} // mandatoryOrder first, then custom, then sorted rest
	// Проверяем, что порядок соответствует ожидаемому
	if len(lines) != len(expectedOrder) {
		t.Errorf("output has %d lines, want %d", len(lines), len(expectedOrder))
	}
	for i, line := range lines {
		if line != expectedOrder[i] {
			t.Errorf("line %d: %s, want %s", i, line, expectedOrder[i])
		}
	}
}

// --- Тест loadCustomOrder ---

func TestLoadCustomOrder(t *testing.T) {
	cachedRussianOrder = []string{"ru1", "ru2"}
	cachedEnglishOrder = []string{"en1", "en2"}

	if result := loadCustomOrder("ru"); !reflect.DeepEqual(result, cachedRussianOrder) {
		t.Errorf("loadCustomOrder(ru) = %v, want %v", result, cachedRussianOrder)
	}
	if result := loadCustomOrder("en"); !reflect.DeepEqual(result, cachedEnglishOrder) {
		t.Errorf("loadCustomOrder(en) = %v, want %v", result, cachedEnglishOrder)
	}
	if result := loadCustomOrder("de"); !reflect.DeepEqual(result, cachedEnglishOrder) {
		t.Errorf("loadCustomOrder(de) = %v, want %v (fallback to en)", result, cachedEnglishOrder)
	}
}

// --- Тест Set функций (просто проверка, что они устанавливают) ---

func TestSetters(t *testing.T) {
	// Просто проверяем, что установка не паникует
	SetFolderExistsFunc(nil)
	SetListModFoldersFunc(nil)
	SetLogFunc(nil)
	SetMandatoryOrder([]string{"a"})
	SetDependencies([]ModDependency{{Required: "r", Dependent: "d"}})
	SetSortMessages("ru", "en")
	SetLogMessages("create", "created")
	SetLoadOrderRules([]checks.LoadOrderRule{{Before: "x", After: "y"}})
	SetHeaderFunc(nil)
	SetLoadOrderOutputPath("/tmp/out")

	// Проверяем, что глобальные переменные изменились
	if len(mandatoryOrder) != 1 || mandatoryOrder[0] != "a" {
		t.Error("SetMandatoryOrder failed")
	}
	if len(dependencies) != 1 || dependencies[0].Required != "r" {
		t.Error("SetDependencies failed")
	}
	if sortWarningRu != "ru" || sortWarningEn != "en" {
		t.Error("SetSortMessages failed")
	}
	if logCreateMLOT != "create" || logMLOTCreated != "created" {
		t.Error("SetLogMessages failed")
	}
	if len(loadOrderRules) != 1 || loadOrderRules[0].Before != "x" {
		t.Error("SetLoadOrderRules failed")
	}
	if loadOrderOutputPath != "/tmp/out" {
		t.Error("SetLoadOrderOutputPath failed")
	}
}
