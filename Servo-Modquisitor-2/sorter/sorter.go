// Servo-Modquisitor-2/sorter/sorter.go
package sorter

import (
	"Servo-Modquisitor/checks"
	"path/filepath"
	"sync"

	"bufio"
	"container/heap"
	"fmt"
	"os"
	"strings"
)

var (
	folderExists    func(string) bool
	listModFolders  func() []string
	logFunc         func(string)
	sorterDataMutex sync.RWMutex // Решение гонки 79
	mandatoryOrder  []string
	loadOrderRules  []checks.LoadOrderRule
	dependencies    []ModDependency
	sortWarningRu   string
	sortWarningEn   string
	logCreateMLOT   string
	logMLOTCreated  string
	writeHeaderFunc func(*os.File, string)
)

type ModDependency struct {
	Dependent string
	Required  string
}

// Функции-сеттеры
func SetMandatoryOrder(order []string) { // Решение гонки 79
	sorterDataMutex.Lock()
	mandatoryOrder = order
	sorterDataMutex.Unlock()
}
func SetDependencies(deps []ModDependency) {
	sorterDataMutex.Lock()
	dependencies = deps
	sorterDataMutex.Unlock()
}
func SetLoadOrderRules(rules []checks.LoadOrderRule) {
	sorterDataMutex.Lock()
	loadOrderRules = rules
	sorterDataMutex.Unlock()
}
func SetFolderExistsFunc(fn func(string) bool) { folderExists = fn }
func SetListModFoldersFunc(fn func() []string) { listModFolders = fn }
func SetLogFunc(fn func(string))               { logFunc = fn }
func SetSortMessages(ru, en string)            { sortWarningRu = ru; sortWarningEn = en }
func SetLogMessages(createMLOT, mlotCreated string) {
	logCreateMLOT = createMLOT
	logMLOTCreated = mlotCreated
}
func SetHeaderFunc(fn func(*os.File, string)) {
	writeHeaderFunc = fn
}

// Кэш кастомных порядков
var cachedRussianOrder, cachedEnglishOrder []string

var (
	loadOrderOutputPath string
	loadOrderPathMutex  sync.Mutex
)

func SetLoadOrderOutputPath(path string) {
	loadOrderPathMutex.Lock()
	loadOrderOutputPath = path
	loadOrderPathMutex.Unlock()
}

// getLoadOrderOutputPath возвращает снимок пути для записи.
func getLoadOrderOutputPath() string {
	loadOrderPathMutex.Lock()
	defer loadOrderPathMutex.Unlock()
	return loadOrderOutputPath
}

func LoadSortOrders(dataDir string) {
	cachedRussianOrder = readSortOrder(filepath.Join(dataDir, "russian_sort_order.txt"))
	cachedEnglishOrder = readSortOrder(filepath.Join(dataDir, "english_sort_order.txt"))
}

func CreateLoadOrderFromActive(activeMods []string, lang string) {
	sorterDataMutex.RLock()
	customOrder := loadCustomOrder(lang)
	mandatory := mandatoryOrder
	deps := dependencies
	rules := loadOrderRules
	sorterDataMutex.RUnlock()
	if logFunc != nil {
		logFunc(logCreateMLOT)
	}

	activeSet := make(map[string]bool)
	for _, m := range activeMods {
		activeSet[m] = true
	}

	var finalOrder []string
	added := make(map[string]bool)

	for _, m := range mandatory {
		if activeSet[m] && folderExists(m) && !added[m] {
			finalOrder = append(finalOrder, m)
			added[m] = true
		}
	}

	for _, m := range customOrder {
		if activeSet[m] && folderExists(m) && !added[m] {
			finalOrder = append(finalOrder, m)
			added[m] = true
		}
	}

	var rest []string
	for m := range activeSet {
		if !added[m] && folderExists(m) {
			rest = append(rest, m)
		}
	}

	allDeps := make([]ModDependency, len(deps))
	copy(allDeps, deps)
	for _, rule := range rules {
		if rule.Before == "*" {
			continue
		}
		allDeps = append(allDeps, ModDependency{
			Required:  rule.Before, // Зависимость ДО
			Dependent: rule.After,  // Зависимый ПОСЛЕ
		})
	}
	for i := 0; i < len(mandatory)-1; i++ {
		allDeps = append(allDeps, ModDependency{
			Required:  mandatory[i],   // Зависимость ДО
			Dependent: mandatory[i+1], // Зависимый ПОСЛЕ
		})
	}
	sortedRest := topologicalSort(rest, allDeps)
	finalOrder = append(finalOrder, sortedRest...)

	outPath := getLoadOrderOutputPath()
	if outPath == "" {
		if logFunc != nil {
			logFunc("loadOrderOutputPath is not set")
		}
		return
	}
	file, err := os.Create(outPath)
	if err != nil {
		if logFunc != nil {
			logFunc(fmt.Sprintf("Failed to create load order file: %v", err))
		}
		return
	}
	defer file.Close()

	if writeHeaderFunc != nil {
		writeHeaderFunc(file, lang)
	}
	for _, mod := range finalOrder {
		fmt.Fprintln(file, mod)
	}

	if logFunc != nil {
		logFunc(logMLOTCreated)
	}
}

func loadCustomOrder(lang string) []string {
	if lang == "ru" {
		return cachedRussianOrder
	}
	return cachedEnglishOrder
}

func readSortOrder(filename string) []string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil
	}
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func topologicalSort(mods []string, deps []ModDependency) []string {
	if len(mods) == 0 {
		return nil
	}

	index := make(map[string]int)
	for i, m := range mods {
		index[m] = i
	}

	adj := make([][]int, len(mods))
	indeg := make([]int, len(mods))

	for _, dep := range deps {
		from, ok1 := index[dep.Required] // Зависимость ставится первой
		to, ok2 := index[dep.Dependent]  // Зависимые ставятся после
		if ok1 && ok2 {
			adj[from] = append(adj[from], to)
			indeg[to]++
		}
	}

	pq := &MinHeap{mods: mods}
	heap.Init(pq)

	for i, deg := range indeg {
		if deg == 0 {
			heap.Push(pq, i)
		}
	}

	var result []string
	for pq.Len() > 0 {
		u := heap.Pop(pq).(int)
		result = append(result, mods[u])
		for _, v := range adj[u] {
			indeg[v]--
			if indeg[v] == 0 {
				heap.Push(pq, v)
			}
		}
	}

	return result
}

type MinHeap struct {
	indices []int
	mods    []string
}

func (h MinHeap) Len() int { return len(h.indices) }

func (h MinHeap) Less(i, j int) bool {
	a := h.mods[h.indices[i]]
	b := h.mods[h.indices[j]]
	lowerA, lowerB := strings.ToLower(a), strings.ToLower(b)
	if lowerA != lowerB {
		return lowerA < lowerB
	}
	return a < b
}

func (h MinHeap) Swap(i, j int) {
	h.indices[i], h.indices[j] = h.indices[j], h.indices[i]
}

func (h *MinHeap) Push(x interface{}) {
	h.indices = append(h.indices, x.(int))
}

func (h *MinHeap) Pop() interface{} {
	old := h.indices
	n := len(old)
	x := old[n-1]
	h.indices = old[0 : n-1]
	return x
}
