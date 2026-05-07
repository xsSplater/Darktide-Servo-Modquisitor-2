//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// launchGame запускает Darktide на Linux.
// Для обычного запуска используется xdg-open steam:// (открывает игру через Steam).
// Для пропуска лаунчера — прямой вызов Darktide.exe через Wine/Proton.
func launchGame(version GameVersion, gameRoot string, skipLauncher bool) error {
	switch version {
	case VersionSteam:
		if !skipLauncher {
			// Обычный запуск через Steam
			return exec.Command("xdg-open", "steam://rungameid/1361210").Start()
		}
		// Прямой запуск (пропуск лаунчера)
		exePath := filepath.Join(gameRoot, "binaries", "Darktide.exe")
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			return fmt.Errorf("%s: %s", errDarktideExeNotFound, exePath)
		}

		// Записываем steam_appid.txt, чтобы игра знала свой ID
		steamAppIDPath := filepath.Join(gameRoot, "binaries", "steam_appid.txt")
		os.WriteFile(steamAppIDPath, []byte("1361210"), 0644)

		// Аргументы запуска (такие же, как на Windows)
		args := []string{
			exePath,
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

		// Пробуем запустить через wine (должен быть установлен пользователем)
		winePath, err := findWine()
		if err != nil {
			return fmt.Errorf(
				"Wine/Proton not found. Please install Wine and ensure it's in PATH, "+
					"or launch normally without 'Skip launcher': %w", err,
			)
		}

		cmd := exec.Command(winePath, args...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Env = append(os.Environ(),
			"SteamAppId=1361210",
			"SteamGameId=1361210",
		)
		return cmd.Start()

	case VersionXbox:
		// Xbox-версия игры не поддерживается на Linux
		return fmt.Errorf("Xbox/Game Pass version is not supported on Linux. Use Steam version.")

	default:
		return fmt.Errorf("%s", errGameVersionUnknown)
	}
}

// findWine ищет установленный Wine или Proton в системе.
// Возвращает путь к исполняемому файлу wine.
func findWine() (string, error) {
	// Стандартные пути, где может находиться wine
	possiblePaths := []string{
		"wine",                // если wine есть в PATH
		"/usr/bin/wine",       // стандартный путь
		"/usr/local/bin/wine", // локальная установка
	}

	// Проверяем стандартные пути
	for _, path := range possiblePaths {
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
	}

	// Ищем Proton в Steam (он установлен вместе со Steam Play)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		// Типичный путь к Proton в Steam
		steamProtonBase := filepath.Join(
			homeDir, ".steam", "steam", "steamapps", "common",
		)
		if entries, err := os.ReadDir(steamProtonBase); err == nil {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "Proton") && entry.IsDir() {
					protonWine := filepath.Join(
						steamProtonBase, entry.Name(), "dist", "bin", "wine",
					)
					if _, err := os.Stat(protonWine); err == nil {
						// Устанавливаем переменные окружения для Proton
						os.Setenv("STEAM_COMPAT_DATA_PATH",
							filepath.Join(homeDir, ".steam", "steam", "steamapps", "compatdata", "1361210"))
						os.Setenv("SteamAppId", "1361210")
						return protonWine, nil
					}
				}
			}
		}
	}

	// Ищем Wine через Flatpak (для Steam Deck / Bazzite)
	if entries, err := os.ReadDir("/var/lib/flatpak/runtime"); err == nil {
		for _, entry := range entries {
			if strings.Contains(entry.Name(), "wine") && entry.IsDir() {
				flatpakWine := filepath.Join("/var/lib/flatpak/runtime", entry.Name(), "active", "files", "bin", "wine")
				if _, err := os.Stat(flatpakWine); err == nil {
					return flatpakWine, nil
				}
			}
		}
	}

	return "", fmt.Errorf("wine not found")
}
