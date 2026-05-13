//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func launchGame(version GameVersion, gameRoot string, skipLauncher bool) error {
	switch version {
	case VersionSteam:
		if !skipLauncher {
			return exec.Command("xdg-open", "steam://rungameid/"+DarktideAppID).Start()
		}
		exePath := filepath.Join(gameRoot, "binaries", "Darktide.exe")
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			return fmt.Errorf("%s: %s", errDarktideExeNotFound, exePath)
		}

		steamAppIDPath := filepath.Join(gameRoot, "binaries", "steam_appid.txt")
		os.WriteFile(steamAppIDPath, []byte(DarktideAppID), 0644)

		args := []string{
			exePath,
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

		winePath, err := findWine()
		if err != nil {
			return fmt.Errorf("%s: %w", errWineNotFound, err)
		}

		cmd := exec.Command(winePath, args...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Env = append(os.Environ(),
			"SteamAppId="+DarktideAppID,
			"SteamGameId="+DarktideAppID,
		)
		return cmd.Start()

	case VersionXbox:
		return fmt.Errorf("%s", errXboxOnLinux)

	default:
		return fmt.Errorf("%s", errGameVersionUnknown)
	}
}

func findWine() (string, error) {
	possiblePaths := []string{
		"wine",
		"/usr/bin/wine",
		"/usr/local/bin/wine",
	}

	for _, path := range possiblePaths {
		if _, err := exec.LookPath(path); err == nil {
			return path, nil
		}
	}

	homeDir, err := os.UserHomeDir()
	if err == nil {
		steamProtonBase := filepath.Join(homeDir, ".steam", "steam", "steamapps", "common")
		if entries, err := os.ReadDir(steamProtonBase); err == nil {
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "Proton") && entry.IsDir() {
					protonWine := filepath.Join(steamProtonBase, entry.Name(), "dist", "bin", "wine")
					if _, err := os.Stat(protonWine); err == nil {
						os.Setenv("STEAM_COMPAT_DATA_PATH",
							filepath.Join(homeDir, ".steam", "steam", "steamapps", "compatdata", DarktideAppID))
						os.Setenv("SteamAppId", DarktideAppID)
						return protonWine, nil
					}
				}
			}
		}
	}

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
