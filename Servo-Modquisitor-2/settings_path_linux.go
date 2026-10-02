//go:build linux

// Servo-Modquisitor-2/settings_path_linux.go

package main

import (
	"os"
	"path/filepath"
	"strings"
)

// findProtonSettingsPath ищет user_settings.config внутри Proton-контейнера
// Darktide. Проверяет все найденные Steam-библиотеки.
//
// Возвращает "" если ничего не нашли.
func findProtonSettingsPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	// Стандартные пути к корню Steam (native, Flatpak, Snap).
	candidates := []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".steam", "steam"),
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
	}

	seen := map[string]bool{}
	for _, root := range candidates {
		if root == "" || seen[root] {
			continue
		}
		seen[root] = true

		steamapps := filepath.Join(root, "steamapps")
		if p := checkCompatDataForDarktide(steamapps); p != "" {
			return p
		}

		// Читаем libraryfolders.vdf: там дополнительные steamapps на
		// других дисках. Формат простой — построчный парсер без VDF-парсера.
		for _, extraRoot := range parseLibraryFolders(filepath.Join(steamapps, "libraryfolders.vdf")) {
			if seen[extraRoot] {
				continue
			}
			seen[extraRoot] = true
			if p := checkCompatDataForDarktide(filepath.Join(extraRoot, "steamapps")); p != "" {
				return p
			}
		}
	}
	return ""
}

// checkCompatDataForDarktide проверяет <steamapps>/compatdata/1361210/pfx
// и возвращает путь к user_settings.config, если он там есть.
//
// Proton использует два пути: "Application Data" (junction на
// AppData/Roaming) и напрямую AppData/Roaming. Проверяем оба.
func checkCompatDataForDarktide(steamapps string) string {
	base := filepath.Join(steamapps, "compatdata", DarktideAppID, "pfx", "drive_c", "users", "steamuser")
	candidates := []string{
		filepath.Join(base, "Application Data", "Fatshark", "Darktide", "user_settings.config"),
		filepath.Join(base, "AppData", "Roaming", "Fatshark", "Darktide", "user_settings.config"),
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	return ""
}

// parseLibraryFolders читает libraryfolders.vdf и возвращает список
// корневых каталогов библиотек (родителей steamapps).
// Ищет строки вида: "path"		"/home/.../Steam"
//
// Простой строковый парсер: формат VDF стабилен, полноценный токенизатор
// тут избыточен.
func parseLibraryFolders(vdfPath string) []string {
	data, err := os.ReadFile(vdfPath)
	if err != nil {
		return nil
	}
	var roots []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		idx := strings.Index(line, `"path"`)
		if idx != 0 {
			continue
		}
		rest := strings.TrimSpace(line[len(`"path"`):])
		rest = strings.Trim(rest, `"`)
		// На всякий случай снимаем экранирование слэшей (в старых
		// версиях VDF писали \\ вместо /).
		rest = strings.ReplaceAll(rest, `\\`, `/`)
		if rest != "" {
			roots = append(roots, rest)
		}
	}
	return roots
}
