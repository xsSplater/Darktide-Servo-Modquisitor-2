//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// launchGame запускает игру в зависимости от версии и флага skipLauncher.
func launchGame(version GameVersion, gameRoot string, skipLauncher bool) error {
	var exePath string
	var args []string

	switch version {
	case VersionSteam:
		exePath = filepath.Join(gameRoot, "binaries", "Darktide.exe")
		if !skipLauncher {
			// Открываем Steam-ссылку через обработчик протоколов
			return exec.Command("rundll32", "url.dll,FileProtocolHandler", "steam://rungameid/1361210").Start()
		}
		// Прямой запуск: прописываем steam_appid.txt и формируем аргументы
		steamAppIDPath := filepath.Join(gameRoot, "binaries", "steam_appid.txt")
		os.WriteFile(steamAppIDPath, []byte("1361210"), 0644)
		args = []string{
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

	case VersionXbox:
		exePath = filepath.Join(gameRoot, "content", "binaries", "Darktide.exe")
		if !skipLauncher {
			return exec.Command("explorer", "shell:AppsFolder\\...").Start()
		}
		args = []string{
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

	default:
		return fmt.Errorf("%s", errGameVersionUnknown)
	}

	// Проверяем существование исполняемого файла
	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return fmt.Errorf("%s: %s", errDarktideExeNotFound, exePath)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)
	return cmd.Start()
}
