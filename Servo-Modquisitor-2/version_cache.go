// Servo-Modquisitor-2/version_cache.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// VersionCache — потокобезопасный кэш версий модов и «последних» версий
// с Nexus. Заменяет поля App.nexusVersionCache / App.nexusLatestVersions
// с их мьютексами.
//
// Инварианты:
//   - cached соответствует активному профилю. Ключ — "id:folder".
//   - latest заполняется из ответов Nexus и живёт только в памяти
//     (не сохраняется на диск).
//   - Порядок взятия мьютексов внутри сервиса один: один mu на всё.
//     Изменение относительно прежнего App, где cfgMutex и cacheMutex
//     брались парой — см. метод LoadFromProfile.
type VersionCache struct {
	mu     sync.RWMutex
	cached map[string]ModVersionInfo
	latest map[string]string

	// logFn используется для не-критичных сообщений (например, ошибок
	// записи в файл). Может быть nil.
	logFn func(string)
}

// NewVersionCache создаёт пустой кэш. logFn может быть nil.
func NewVersionCache(logFn func(string)) *VersionCache {
	return &VersionCache{
		cached: make(map[string]ModVersionInfo),
		latest: make(map[string]string),
		logFn:  logFn,
	}
}

func (c *VersionCache) log(msg string) {
	if c.logFn != nil {
		c.logFn(msg)
	}
}

// ─────────────────────────────────────────────────────────────────
// Базовые операции
// ─────────────────────────────────────────────────────────────────

// Get возвращает сохранённую версию мода по ключу "id:folder".
func (c *VersionCache) Get(key string) (ModVersionInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.cached[key]
	return v, ok
}

// Set сохраняет версию мода.
func (c *VersionCache) Set(key string, info ModVersionInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cached[key] = info
}

// Delete удаляет запись по ключу.
func (c *VersionCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.cached, key)
}

// GetLatest возвращает последнюю известную версию с Nexus.
func (c *VersionCache) GetLatest(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.latest[key]
	return v, ok
}

// SetLatest сохраняет последнюю известную версию с Nexus.
func (c *VersionCache) SetLatest(key, version string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.latest[key] = version
}

// Snapshot возвращает копию всей сохранённой карты. Безопасно читать
// без мьютекса — данные уже не будут меняться конкурентно.
func (c *VersionCache) Snapshot() map[string]ModVersionInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]ModVersionInfo, len(c.cached))
	for k, v := range c.cached {
		out[k] = v
	}
	return out
}

// ReplaceAll заменяет содержимое кэша целиком. Используется при
// переключении профиля.
func (c *VersionCache) ReplaceAll(items map[string]ModVersionInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if items == nil {
		items = make(map[string]ModVersionInfo)
	}
	c.cached = items
}

// Len возвращает число сохранённых версий.
func (c *VersionCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cached)
}

// RemoveByFolder удаляет первую найденную запись с указанным Folder.
// Возвращает true, если запись была найдена и удалена.
func (c *VersionCache) RemoveByFolder(folder string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, info := range c.cached {
		if info.Folder == folder {
			delete(c.cached, k)
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────
// Prune: очистка мёртвых и дублирующихся записей
// ─────────────────────────────────────────────────────────────────

// Prune удаляет:
//  1. Записи для папок, которых нет в existing.
//  2. Дубликаты по Folder (оставляет «лучшую» запись — с source=nexus
//     или с бо́льшим timestamp).
//
// Возвращает число удалённых записей.
func (c *VersionCache) Prune(existing map[string]bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. Удаляем записи для несуществующих папок.
	for key, info := range c.cached {
		if !existing[info.Folder] {
			delete(c.cached, key)
		}
	}

	// 2. Ищем «лучшую» запись для каждой папки.
	best := make(map[string]string) // folder -> key
	for key, info := range c.cached {
		current, ok := best[info.Folder]
		if !ok {
			best[info.Folder] = key
			continue
		}
		curInfo := c.cached[current]
		if betterVersionInfo(info, curInfo) {
			best[info.Folder] = key
		}
	}

	// 3. Удаляем всё, что не попало в best.
	var toDelete []string
	for key, info := range c.cached {
		if best[info.Folder] != key {
			toDelete = append(toDelete, key)
		}
	}
	for _, key := range toDelete {
		delete(c.cached, key)
	}

	return len(toDelete)
}

// betterVersionInfo сообщает, следует ли отдать предпочтение a перед b
// при дедупликации по Folder.
func betterVersionInfo(a, b ModVersionInfo) bool {
	// nexus всегда лучше manual.
	if a.Source == "nexus" && b.Source != "nexus" {
		return true
	}
	if a.Source != "nexus" && b.Source == "nexus" {
		return false
	}
	// Иначе — по timestamp.
	if a.Timestamp != b.Timestamp {
		return a.Timestamp > b.Timestamp
	}
	// При равных — по наличию Version.
	return a.Version != "" && b.Version == ""
}

// ─────────────────────────────────────────────────────────────────
// Файловые операции: load / save
// ─────────────────────────────────────────────────────────────────

// LoadFromProfile читает nexus_versions.json из указанной папки профиля
// и заменяет содержимое кэша. Если файла нет — кэш становится пустым.
//
// Пустой profilePath — тоже пустой кэш (например, при инициализации
// до выбора профиля).
func (c *VersionCache) LoadFromProfile(profilePath string) {
	if profilePath == "" {
		c.ReplaceAll(nil)
		return
	}

	path := filepath.Join(profilePath, FileNameNexusVersions)
	bakPath := path + ".bak"

	// Пробуем основной файл.
	items, ok := c.tryLoadFromFile(path)

	// Основной файл не подошёл — пробуем .bak.
	//
	// Что считается «не подошёл»:
	//   - файла нет;
	//   - JSON не парсится;
	//   - JSON валиден, но содержит 0 записей.
	//
	// Последний случай — самая частая причина «все моды manual»:
	// при каком-то сбое основной файл оказался с одной записью
	// (или вообще пустой map), и без fallback на .bak кэш
	// терялся целиком.
	if !ok {
		if bakItems, bakOK := c.tryLoadFromFile(bakPath); bakOK {
			c.log(fmt.Sprintf(
				"LoadFromProfile: recovered %d entries from %s (main file was missing/empty/corrupt)",
				len(bakItems), bakPath))
			// Возвращаем .bak на место основного, чтобы следующий
			// запуск сразу получил валидные данные и не проваливался
			// в fallback снова.
			_ = os.Rename(bakPath, path)
			items = bakItems
			ok = true
		}
	}

	if !ok {
		c.log(fmt.Sprintf("LoadFromProfile: no usable cache at %s", path))
		c.ReplaceAll(nil)
		return
	}

	c.ReplaceAll(items)
}

// tryLoadFromFile читает и парсит файл кэша. Возвращает:
//
//	(items, true)  — файл есть, парсится, содержит ≥1 запись.
//	(nil, false)   — файла нет, парсится с ошибкой, или 0 записей.
//
// Вынесено в отдельный метод, чтобы LoadFromProfile мог вызвать
// одну и ту же логику и для основного файла, и для .bak.
func (c *VersionCache) tryLoadFromFile(path string) (map[string]ModVersionInfo, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	if len(data) == 0 {
		return nil, false
	}
	items, err := parseVersionCacheJSON(data)
	if err != nil {
		c.log(fmt.Sprintf("Failed to parse %s: %v", path, err))
		return nil, false
	}
	if len(items) == 0 {
		// Валидный JSON, но пустой map — считаем файл непригодным.
		// См. комментарий в LoadFromProfile.
		return nil, false
	}
	return items, true
}

// SaveToProfile записывает текущее содержимое кэша в nexus_versions.json
// внутри указанной папки профиля. Атомарность не гарантируется: файл
// пишется одним вызовом os.WriteFile.
func (c *VersionCache) SaveToProfile(profilePath string) error {
	if profilePath == "" {
		return nil
	}

	snapshot := c.Snapshot()
	path := filepath.Join(profilePath, FileNameNexusVersions)

	var buf bytes.Buffer
	buf.WriteString("{\n")
	keys := sortedCacheKeys(snapshot)
	for i, key := range keys {
		val := snapshot[key]
		buf.WriteString("\t\"")
		buf.WriteString(key)
		buf.WriteString("\": {\n")
		buf.WriteString("\t\t\"timestamp\": ")
		buf.WriteString(strconv.FormatInt(val.Timestamp, 10))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"version\": ")
		buf.WriteString(strconv.Quote(val.Version))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"folder\": ")
		buf.WriteString(strconv.Quote(val.Folder))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"source\": ")
		buf.WriteString(strconv.Quote(val.Source))
		buf.WriteString(",\n")
		buf.WriteString("\t\t\"installed_at\": ")
		buf.WriteString(strconv.FormatInt(val.InstalledAt, 10))
		buf.WriteString("\n\t}")
		if i < len(keys)-1 {
			buf.WriteString(",")
		}
		buf.WriteString("\n")
	}
	buf.WriteString("}")

	// ─── Атомарная запись + .bak ──────────────────────────────────
	//
	// Порядок шагов важен, каждый ловит свою категорию сбоя:
	//
	//   1. .bak ← текущий файл (rename, атомарно).
	//   2. .tmp ← новые данные.
	//   3. path ← .tmp (rename, атомарно).
	//
	// Если процесс упадёт на шаге 2 — на диске лежит актуальный
	// .bak и частично записанный .tmp; LoadFromProfile увидит битый
	// основной файл и восстановится из .bak.
	//
	// Если упадёт на шаге 3 — на диске .bak и полноценный .tmp;
	// LoadFromProfile возьмёт либо старый (валидный) основной,
	// либо .bak.
	//
	// До этого патча .bak не создавался вообще, и единственным
	// восстановлением был сам основной файл — если он повреждён,
	// кэш терялся целиком (540 модов → 1 запись).
	tmpPath := path + ".tmp"
	bakPath := path + ".bak"

	// Шаг 1. Сохраняем предыдущую версию как .bak.
	// Переименование, а не копирование: атомарно и O(1).
	// Если основного файла нет (первый запуск) — шаг no-op.
	if _, err := os.Stat(path); err == nil {
		// .bak мог остаться с прошлого раза — перезаписываем.
		_ = os.Remove(bakPath)
		if err := os.Rename(path, bakPath); err != nil {
			// Не критично: продолжаем, .bak — это best-effort
			// страховка. Не валим запись из-за него.
			c.log(fmt.Sprintf("SaveToProfile: rename to .bak failed: %v", err))
		}
	}

	// Шаг 2. Пишем новые данные во временный файл.
	if err := os.WriteFile(tmpPath, buf.Bytes(), 0644); err != nil {
		// Восстанавливаем .bak на место основного, чтобы не
		// оставить ситуацию «файла нет, только .bak и .tmp».
		if _, statErr := os.Stat(bakPath); statErr == nil {
			_ = os.Rename(bakPath, path)
		}
		return err
	}

	// Шаг 3. Атомарная замена.
	if err := os.Rename(tmpPath, path); err != nil {
		// Восстанавливаем .bak, чтобы не остаться без файла.
		if _, statErr := os.Stat(bakPath); statErr == nil {
			_ = os.Rename(bakPath, path)
		}
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

// parseVersionCacheJSON разбирает JSON из nexus_versions.json, поддерживая
// миграцию старого формата (map[string]string) и отсеивая записи с
// подозрительно большими ID.
func parseVersionCacheJSON(data []byte) (map[string]ModVersionInfo, error) {
	var raw map[string]ModVersionInfo
	if err := json.Unmarshal(data, &raw); err != nil {
		// Попытка миграции старого формата.
		var old map[string]string
		if err2 := json.Unmarshal(data, &old); err2 != nil {
			return nil, fmt.Errorf("unmarshal nexus_versions: %w", err)
		}
		raw = make(map[string]ModVersionInfo, len(old))
		for k, v := range old {
			ts, _ := strconv.ParseInt(v, 10, 64)
			raw[k] = ModVersionInfo{Timestamp: ts, Source: "nexus"}
		}
	}

	out := make(map[string]ModVersionInfo, len(raw))
	for key, info := range raw {
		// Отсеиваем подозрительно большие ID (типично для мусора).
		if strings.Contains(key, ":") {
			parts := strings.SplitN(key, ":", 2)
			if id, err := strconv.Atoi(parts[0]); err == nil && id > MaxModsID {
				continue
			}
		}
		if info.Source == "" {
			info.Source = "nexus"
		}
		// Старый ключ без folder — дописываем его.
		if !strings.Contains(key, ":") && info.Folder != "" {
			out[key+":"+info.Folder] = info
		} else {
			out[key] = info
		}
	}
	return out, nil
}

func extractModIDFromKey(key string) int {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		id, err := strconv.Atoi(parts[0])
		if err == nil {
			return id
		}
	}
	return 0
}

// sortedCacheKeys сортирует ключи кэша по числовому modID, затем по строке —
// чтобы вывод файла был стабильным и диффы при правках были минимальные.
func sortedCacheKeys(items map[string]ModVersionInfo) []string {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		id1 := extractModIDFromKey(keys[i])
		id2 := extractModIDFromKey(keys[j])
		if id1 != id2 {
			return id1 < id2
		}
		return keys[i] < keys[j]
	})
	return keys
}
