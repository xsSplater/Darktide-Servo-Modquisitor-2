// Servo-Modquisitor-2/problem_collector.go
package main

import (
	"sort"
	"strings"

	"Servo-Modquisitor/checks"
)

// collectAllProblems делает один проход по app.allMods и возвращает
// список BatchItem с указанной проблемой. Не удаляет и не показывает
// диалогов — чистая функция по данным.
//
// Каждая запись получает:
//
//	Selected = true (по умолчанию предлагаем удалить всё);
//	Kind     = тип проблемы;
//	ID       = имя папки (стабильный якорь — то же, что в
//	           load order и nexus_versions.json);
//	Name/Description/Author/Category/PictureURL — для карточки справа.
//
// Дубликаты не создаются: если мод одновременно obsolete и broken,
// в результат попадёт первый по приоритету (obsolete → conflict →
// broken → missing). Приоритет условный, но стабильный.
func (app *App) collectAllProblems() []*BatchItem {
	app.modsMutex.RLock()
	modsSnapshot := make([]checks.ModInfo, len(app.allMods))
	copy(modsSnapshot, app.allMods)
	app.modsMutex.RUnlock()

	// Множества для быстрой проверки.
	obsolete := make(map[string]bool)
	for _, o := range checks.GetObsoleteMods() {
		obsolete[o] = true
	}

	// Конфликты — пары, где оба мода установлены, не disabled
	// и хотя бы один из пары в текущем списке.
	// GetIncompatiblePairs уже отфильтрован по folder-exists внутри
	// GetConflictPartner, но нам нужен полный список пар для
	// заполнения ConflictFolder.
	pairPartners := make(map[string][]string) // folder → все партнёры
	for _, pair := range checks.GetIncompatiblePairs() {
		if !checks.FolderExists(pair.Mod1) || !checks.FolderExists(pair.Mod2) {
			continue
		}
		if live, _ := checks.GetConflictPartner(pair.Mod1); live == "" {
			continue
		}
		pairPartners[pair.Mod1] = append(pairPartners[pair.Mod1], pair.Mod2)
		pairPartners[pair.Mod2] = append(pairPartners[pair.Mod2], pair.Mod1)
	}

	// Кластеры конфликтов — связные компоненты графа.
	// Обход по алфавиту: номера групп детерминированы между запусками.
	groupOf := make(map[string]int)
	{
		names := make([]string, 0, len(pairPartners))
		for n := range pairPartners {
			names = append(names, n)
		}
		sort.Strings(names)
		gid := 0
		for _, start := range names {
			if _, done := groupOf[start]; done {
				continue
			}
			groupOf[start] = gid
			queue := []string{start}
			for len(queue) > 0 {
				cur := queue[0]
				queue = queue[1:]
				for _, p := range pairPartners[cur] {
					if _, done := groupOf[p]; !done {
						groupOf[p] = gid
						queue = append(queue, p)
					}
				}
			}
			gid++
		}
	}

	var items []*BatchItem
	seen := make(map[string]bool)

	for _, mod := range modsSnapshot {
		if mod.IsSystem || seen[mod.Name] {
			continue
		}

		var kind ProblemKind
		var conflictPartner string

		switch {
		case obsolete[mod.Name]:
			kind = ProblemObsolete
		case len(pairPartners[mod.Name]) > 0:
			kind = ProblemConflict
			conflictPartner = pairPartners[mod.Name][0]
		case mod.Broken:
			kind = ProblemBroken
		case mod.MissingFolder:
			kind = ProblemMissingFolder
		default:
			continue
		}

		display := mod.DisplayName
		if display == "" {
			display = mod.Name
		}

		item := &BatchItem{
			ID:               mod.Name,
			Name:             display,
			Author:           mod.Author,
			Category:         mod.Category,
			Description:      mod.Description,
			PictureURL:       mod.NexusPictureURL,
			Selected:         true,
			Status:           BatchPending,
			Kind:             kind,
			ConflictFolder:   conflictPartner,
			ConflictPartners: pairPartners[mod.Name],
			ConflictGroup:    -1,
		}
		if kind == ProblemConflict {
			item.ConflictGroup = groupOf[mod.Name]
		}
		items = append(items, item)
		seen[mod.Name] = true
	}

	// Предвыбор конфликтов: по умолчанию предлагаем удалить только
	// одну сторону пары, вторую оставляем unchecked. Пользователь
	// может переставить галки.
	removeSet := app.buildRemovalSet()
	for _, item := range items {
		if item.Kind == ProblemConflict {
			item.Selected = removeSet[item.ID]
		}
	}

	// Сортировка: сначала obsolete (массово), потом конфликты,
	// потом broken, потом missing. Внутри группы — по имени.
	// Внутри конфликтов — по номеру группы, чтобы члены одной
	// группы стояли подряд: buildRows строит по ним дерево.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Kind != items[j].Kind {
			return items[i].Kind < items[j].Kind
		}
		if items[i].Kind == ProblemConflict && items[i].ConflictGroup != items[j].ConflictGroup {
			return items[i].ConflictGroup < items[j].ConflictGroup
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return items
}

// buildRemovalSet решает, кого из конфликтующих пар предлагать
// удалить по умолчанию. Возвращает map[folder]→true для тех,
// кого предвыбрано на удаление.
//
// Правило приоритета:
//  1. mandatory_order — не трогаем. Мод, стоящий в начале списка
//     для загрузки, обычно критичен для работы других.
//  2. required для активных зависимостей — не трогаем. Удалить
//     его — сломать тех, кому он нужен.
//  3. Иначе — по алфавиту: первый (в нижнем регистре) остаётся,
//     второй уходит на удаление.
//
// Мод, «проигравший» хотя бы в одной паре, попадает в результат.
// Source (nexus/manual) на выбор не влияет.
func (app *App) buildRemovalSet() map[string]bool {
	remove := make(map[string]bool)

	protect := make(map[string]bool)
	for _, m := range checks.GetMandatoryOrder() {
		protect[m] = true
	}
	for _, d := range checks.GetDependenciesFlat() {
		if app.isModActive(d.Dependent) {
			protect[d.Required] = true
		}
	}

	for _, pair := range checks.GetIncompatiblePairs() {
		if !checks.FolderExists(pair.Mod1) || !checks.FolderExists(pair.Mod2) {
			continue
		}
		if live, _ := checks.GetConflictPartner(pair.Mod1); live == "" {
			continue
		}
		a, b := pair.Mod1, pair.Mod2

		if protect[a] && protect[b] {
			continue
		}
		if protect[a] {
			remove[b] = true
			continue
		}
		if protect[b] {
			remove[a] = true
			continue
		}

		if strings.ToLower(a) < strings.ToLower(b) {
			remove[b] = true
		} else {
			remove[a] = true
		}
	}
	return remove
}
