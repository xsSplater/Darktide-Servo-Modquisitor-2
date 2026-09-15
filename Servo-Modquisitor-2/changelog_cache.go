// Servo-Modquisitor-2/changelog_cache.go
package main

import "sync"

// ChangelogCache — потокобезопасный кэш текстов changelog'ов и состояния
// их развёрнутости. Заменяет три поля App (changelogCache / changelogTexts /
// changelogExpanded) и мьютекс changelogMutex.
//
// Ключи — "modID:fileName" для текстов и "modID:modName" для состояния
// развёрнутости. Формат ключей не унифицируем: так сложилось исторически,
// и менять его — отдельная задача.
//
// Всё живёт только в памяти: в файл не пишется.
type ChangelogCache struct {
	mu       sync.RWMutex
	texts    map[string]string
	expanded map[string]bool
}

// NewChangelogCache создаёт пустой кэш.
func NewChangelogCache() *ChangelogCache {
	return &ChangelogCache{
		texts:    make(map[string]string),
		expanded: make(map[string]bool),
	}
}

// Text возвращает сохранённый текст changelog'а по ключу.
func (c *ChangelogCache) Text(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.texts[key]
	return v, ok
}

// SetText сохраняет текст changelog'а.
func (c *ChangelogCache) SetText(key, text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.texts[key] = text
}

// Expanded сообщает, развёрнут ли changelog по ключу.
func (c *ChangelogCache) Expanded(key string) (bool, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.expanded[key]
	return v, ok
}

// SetExpanded сохраняет состояние развёрнутости.
func (c *ChangelogCache) SetExpanded(key string, expanded bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expanded[key] = expanded
}
