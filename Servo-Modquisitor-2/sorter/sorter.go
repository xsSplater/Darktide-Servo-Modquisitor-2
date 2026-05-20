package sorter

import (
	"bufio"
	"container/heap"
	"fmt"
	"os"
	"strings"
)

type LoadOrderRule struct {
	Mod		string
	Before	string
}

var (
	folderExists	func(string) bool
	listModFolders	func() []string
	logFunc			func(string)
	mandatoryOrder	[]string
	loadOrderRules	[]LoadOrderRule
	dependencies	[]ModDependency
	sortWarningRu	string
	sortWarningEn	string
	logCreateMLOT	string
	logMLOTCreated	string
	writeHeaderFunc	func(*os.File, string)
)

type ModDependency struct {
	Dependent	string
	Required	string
}

// Функции-сеттеры
func SetFolderExistsFunc(fn func(string) bool)	{ folderExists = fn }
func SetListModFoldersFunc(fn func() []string)	{ listModFolders = fn }
func SetLogFunc(fn func(string))				{ logFunc = fn }
func SetMandatoryOrder(order []string)			{ mandatoryOrder = order }
func SetDependencies(deps []ModDependency)		{ dependencies = deps }
func SetSortMessages(ru, en string)				{ sortWarningRu = ru; sortWarningEn = en }
func SetLogMessages(createMLOT, mlotCreated string) {
	logCreateMLOT = createMLOT
	logMLOTCreated = mlotCreated
}
func SetLoadOrderRules(rules []LoadOrderRule) {
	loadOrderRules = rules
}
func SetHeaderFunc(fn func(*os.File, string)) {
	writeHeaderFunc = fn
}


// кеш кастомных порядков
var cachedRussianOrder, cachedEnglishOrder []string

func LoadSortOrders() {
	cachedRussianOrder = readSortOrder("russian_sort_order.txt")
	cachedEnglishOrder = readSortOrder("english_sort_order.txt")
}

func CreateLoadOrderFromActive(activeMods []string, lang string) {
	if logFunc != nil {
		logFunc(logCreateMLOT)
	}

	customOrder := loadCustomOrder(lang)

	activeSet := make(map[string]bool)
	for _, m := range activeMods {
		activeSet[m] = true
	}

	var finalOrder []string
	added := make(map[string]bool)

	for _, m := range mandatoryOrder {
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

	allDeps := make([]ModDependency, len(dependencies))
	copy(allDeps, dependencies)
	for _, rule := range loadOrderRules {
		if rule.Before == "*" {
			continue
		}
		allDeps = append(allDeps, ModDependency{
			Dependent: rule.Mod,
			Required:  rule.Before,
		})
	}
	for i := 1; i < len(mandatoryOrder); i++ {
		allDeps = append(allDeps, ModDependency{
			Dependent: mandatoryOrder[i-1],
			Required:  mandatoryOrder[i],
		})
	}

	sortedRest := topologicalSort(rest, allDeps)
	finalOrder = append(finalOrder, sortedRest...)

	// ДИАГНОСТИКА: потери на этапе rest (если что‑то не попало в sortedRest)
	// if logFunc != nil {
	// 	logFunc(fmt.Sprintf("--- DEBUG: rest before sort: %d", len(rest)))
	// }

	file, _ := os.Create("mod_load_order.txt")
	if file != nil {
		defer file.Close()
		if writeHeaderFunc != nil {
			writeHeaderFunc(file, lang)
		}
		for _, mod := range finalOrder {
			fmt.Fprintln(file, mod)
		}
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
		from, ok1 := index[dep.Required]	// Зависимость ставится первой
		to, ok2 := index[dep.Dependent]		// Зависимые ставятся после
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

	// ДИАГНОСТИКА: проверяем потери
	// if logFunc != nil {
	// 	logFunc(fmt.Sprintf("--- DEBUG: topologicalSort: input=%d, output=%d", len(mods), len(result)))
	// 	if len(mods) != len(result) {
	// 		present := make(map[string]bool)
	// 		for _, m := range result {
	// 			present[m] = true
	// 		}
	// 		for _, m := range mods {
	// 			if !present[m] {
	// 				logFunc("  MISSING: " + m)
	// 			}
	// 		}
	// 	}
	// }

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
