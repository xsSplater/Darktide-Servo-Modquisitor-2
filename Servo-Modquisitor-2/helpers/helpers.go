// helpers/helpers.go
package helpers

import (
	"strconv"
	"strings"
)

// ExtractModIDFromURL извлекает числовой ID мода из ссылки Nexus Mods.
// Поддерживает URL вида: https://www.nexusmods.com/warhammer40kdarktide/mods/123
func ExtractModIDFromURL(rawURL string) int {
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	if len(parts) < 2 {
		return 0
	}
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return id
}

// ContainsString проверяет, есть ли строка в слайсе.
func ContainsString(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
