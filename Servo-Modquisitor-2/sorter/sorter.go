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
)

type ModDependency struct {
	Dependent	string
	Required	string
}

func SetFolderExistsFunc(fn func(string) bool)	{ folderExists = fn }
func SetListModFoldersFunc(fn func() []string)	{ listModFolders = fn }
func SetLogFunc(fn func(string))				{ logFunc = fn }
func SetMandatoryOrder(order []string)			{ mandatoryOrder = order }
func SetDependencies(deps []ModDependency)		{ dependencies = deps }
func SetSortMessages(ru, en string)					{ sortWarningRu = ru; sortWarningEn = en }
func SetLogMessages(createMLOT, mlotCreated string) {
	logCreateMLOT = createMLOT
	logMLOTCreated = mlotCreated
}
func SetLoadOrderRules(rules []LoadOrderRule) {
	loadOrderRules = rules
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

	file, _ := os.Create("mod_load_order.txt")
	if file != nil {
		defer file.Close()
		writeHeader(file, lang)
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
		from, ok1 := index[dep.Dependent]
		to, ok2 := index[dep.Required]
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

func writeHeader(f *os.File, lang string) {
	if lang == "ru" {
		fmt.Fprintln(f, "-- ▒Servo-Modquisitor▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓1. Если вам нужно добавить мод вручную, введите название папки▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓вашего мода ниже. Каждый новый мод обязательно с новой строки.▓▒")
		fmt.Fprintln(f, "-- ▒▓2. Расположение в списке определяет порядок загрузки модов.▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓Чем ниже мод, тем больший приоритет в загрузке у него будет.▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓3. Не переименовывайте папку мода, т.к. внутри названия папок и▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓записи внутри файлов зависят от этого названия.▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓4. НЕ НУЖНО вносить в список папки «BASE» или «DMF» или вы▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓получите ошибку в игре‼▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓5. Если какой-то мод не попал в список, обязательно сообщите▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓мне об этом в моём Дискорде или на Nexusmods:▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓https://discord.gg/BGZagw3xnz ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓https://www.nexusmods.com/warhammer40kdarktide/mods/139 ▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒xsSplater▒")
		fmt.Fprintln(f, "")
	} else {
		fmt.Fprintln(f, "-- ▒Servo-Modquisitor▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓1. If you need to add a mod manually, enter the folder name of▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓your mod below. Each new mod must be on a new line.▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓2. Order in the list determines the order in which mods are▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓loaded. The lower the mod, the higher the loading priority.▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓3. Do not rename the mod folder, because the folder names and▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓entries inside the fs depend on this name.▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓4. DO NOT list the \"BASE\" or \"DMF\" folders or you will▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓get an error in the game‼▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓5. If any mod got 'lost' during sorting and wasn`t added to the▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓list, please let me know on my Discord or on Nexusmods:▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓https://discord.gg/BGZagw3xnz ▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓https://www.nexusmods.com/warhammer40kdarktide/mods/139 ▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▒")
		fmt.Fprintln(f, "-- ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒xsSplater▒")
		fmt.Fprintln(f, "")
	}
}
