// Servo-Modquisitor-2/vortex_migration.go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

var vortexFolderRe = regexp.MustCompile(`^(.+?)-(\d+)-([0-9A-Za-z]+(?:-[0-9A-Za-z]+)*)-(\d{9,11})$`)

type VortexDeployment struct {
	Files []struct {
		RelPath string `json:"relPath"`
		Source  string `json:"source"`
	} `json:"files"`
}

func parseVortexFolderName(name string) (modID int, version string) {
	// Формат Vortex: "Mod Name-123-1-0-1620000000"
	if m := vortexFolderRe.FindStringSubmatch(name); m != nil {
		modID, _ = strconv.Atoi(m[2])
		version = strings.ReplaceAll(m[3], "-", ".")
		return modID, version
	}
	return 0, ""
}

// migrateFromVortex читает vortex.deployment.json и предлагает перенести моды.
func (app *App) migrateFromVortex() {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		app.showInfoDialog(app.msg("error_title"), "Game root not found")
		return
	}

	depPath := filepath.Join(gameRoot, "vortex.deployment.json")
	if _, err := os.Stat(depPath); os.IsNotExist(err) {
		app.showInfoDialog(app.msg("error_title"), "Vortex deployment file not found in game root.")
		return
	}

	// В реальности здесь нужен диалог выбора папки Staging (хранилища) Vortex,
	// так как его путь не всегда записан в deployment.json.
	// Для упрощения попросим пользователя выбрать папку, где Vortex хранит моды.
	app.showVortexStagingDialog(depPath)
}

func (app *App) showVortexStagingDialog(depPath string) {
	fd := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
		if err != nil || uri == nil {
			return
		}
		stagingPath := filepath.FromSlash(uri.Path())
		go app.doVortexMigration(depPath, stagingPath)
	}, app.mainWindow)
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
	fd.Show()
}

func (app *App) doVortexMigration(depPath, stagingPath string) {
	data, err := os.ReadFile(depPath)
	if err != nil {
		app.appendLog(fmt.Sprintf("Vortex migration: read failed: %v", err))
		return
	}

	var dep VortexDeployment
	if err := json.Unmarshal(data, &dep); err != nil {
		app.appendLog(fmt.Sprintf("Vortex migration: parse failed: %v", err))
		return
	}

	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	// Собираем уникальные папки модов
	modsToCopy := make(map[string]bool)
	for _, f := range dep.Files {
		modsToCopy[f.Source] = true
	}

	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	app.cfgMutex.RUnlock()
	profileModsPath := filepath.Join(app.profilePath(activeProfile), "mods")

	var copied []string

	for modName := range modsToCopy {
		srcDir := filepath.Join(stagingPath, modName)
		if _, err := os.Stat(srcDir); err != nil {
			app.appendLogToFile(fmt.Sprintf("Vortex migration: source %s not found", srcDir))
			continue
		}

		dstDir := filepath.Join(modsPath, modName)
		// Проверяем, нет ли уже такой папки в игре
		if _, err := os.Stat(dstDir); err == nil {
			app.appendLogToFile(fmt.Sprintf("Vortex migration: %s already exists in game, skipping", modName))
			continue
		}

		// Копируем в game/mods
		if err := app.copyFolder(srcDir, dstDir); err != nil {
			app.appendLogToFile(fmt.Sprintf("Vortex migration: copy %s failed: %v", modName, err))
			continue
		}

		// Копируем в profile/mods
		dstProfileDir := filepath.Join(profileModsPath, modName)
		_ = app.copyFolder(srcDir, dstProfileDir)

		// Пытаемся распарсить имя папки Vortex (например, "Mod Name-123-1-0-1620000000")
		// чтобы занести данные в кэш и не показывать мод как "manual"
		if modID, ver := parseVortexFolderName(modName); modID != 0 && ver != "" {
			cacheKey := fmt.Sprintf("%d:%s", modID, modName)
			app.cacheModVersion(cacheKey, modName, ver, 0, "nexus", 0)
			app.appendLogToFile(fmt.Sprintf("Vortex migration: cached version for %s (ID %d, v%s)", modName, modID, ver))
		}

		copied = append(copied, modName)
	}

	fyne.Do(func() {
		app.refreshModList()
		app.appendLog(fmt.Sprintf("Vortex migration finished: %d mods copied", len(copied)))
		app.showInfoDialog("Migration Complete", fmt.Sprintf("Successfully migrated %d mods from Vortex staging.", len(copied)))
	})
}
