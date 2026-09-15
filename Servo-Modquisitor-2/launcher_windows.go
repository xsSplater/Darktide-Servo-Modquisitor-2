//go:build windows

// Servo-Modquisitor-2/launcher_windows.go

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func launchGame(version GameVersion, gameRoot string, skipLauncher bool) error {
	var exePath string
	var args []string

	switch version {
	case VersionSteam:
		// Steam: exe лежит прямо в binaries/, bundle — рядом.
		exePath = filepath.Join(gameRoot, "binaries", "Darktide.exe")
		if !skipLauncher {
			return exec.Command("rundll32", "url.dll,FileProtocolHandler", "steam://rungameid/"+DarktideAppID).Start()
		}
		steamAppIDPath := filepath.Join(gameRoot, "binaries", "steam_appid.txt")
		os.WriteFile(steamAppIDPath, []byte(DarktideAppID), 0644)
		args = []string{
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

	case VersionXbox:
		// Xbox: exe внутри content/binaries/. Это совпадает с
		// detectGameVersion, которая считает корнем тот, где есть
		// content/, и со старым батником:
		//   %game_root%\content\binaries\Darktide.exe
		exePath = filepath.Join(gameRoot, "content", "binaries", "Darktide.exe")
		if !skipLauncher {
			return fmt.Errorf("Xbox launcher not supported. Please use 'Launch without launcher' option.")
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

	if _, err := os.Stat(exePath); os.IsNotExist(err) {
		return fmt.Errorf("%s: %s", errDarktideExeNotFound, exePath)
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir = filepath.Dir(exePath)

	return cmd.Start()
}
