// Servo-Modquisitor-2/mods_state.go
package main

import (
	"sync"

	"Servo-Modquisitor/checks"
)

// ModsState — контейнер для всех полей, связанных со списками модов.
// Встраивается в App, поэтому поля остаются доступны как app.allMods,
// app.displayedMods, app.systemMods, app.modDatabase, а мьютекс — как
// app.modsMutex. Никакие call-sites менять не требуется.
//
// Инварианты:
//   - allMods — источник истины по списку модов.
//   - displayedMods — подмножество allMods по Name; порядок может
//     отличаться (фильтр + сортировка).
//   - systemMods — системные (base, dmf, autopatch).
//   - modDatabase — кэш mod_database.json.
//
// Все четыре поля защищены modsMutex. Порядок взятия с другими
// мьютексами — см. комментарий над App.
type ModsState struct {
	modsMutex     sync.RWMutex
	allMods       []checks.ModInfo
	displayedMods []checks.ModInfo
	systemMods    []checks.ModInfo
	modDatabase   []checks.ModDBEntry
}

// CountAll возвращает число модов в allMods. Не требует внешнего
// мьютекса — берёт внутренний.
func (m *ModsState) CountAll() int {
	m.modsMutex.RLock()
	defer m.modsMutex.RUnlock()
	return len(m.allMods)
}

// CountDisplayed возвращает число модов в displayedMods.
func (m *ModsState) CountDisplayed() int {
	m.modsMutex.RLock()
	defer m.modsMutex.RUnlock()
	return len(m.displayedMods)
}

// FindByNameCopy возвращает копию мода по имени из allMods или
// systemMods (порядок: сначала allMods).
func (m *ModsState) FindByNameCopy(name string) (checks.ModInfo, bool) {
	m.modsMutex.RLock()
	defer m.modsMutex.RUnlock()
	for i := range m.allMods {
		if m.allMods[i].Name == name {
			return m.allMods[i], true
		}
	}
	for i := range m.systemMods {
		if m.systemMods[i].Name == name {
			return m.systemMods[i], true
		}
	}
	return checks.ModInfo{}, false
}
