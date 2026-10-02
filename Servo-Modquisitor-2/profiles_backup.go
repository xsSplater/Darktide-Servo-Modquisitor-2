// Servo-Modquisitor-2/profiles_backup.go
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultBackupsLimit = 5

// backupsDir возвращает корень бэкапов профилей:
// <configdir>/Servo-Modquisitor/backups/profiles/
//
// Раньше бэкапы лежали рядом с exe (<exe_dir>/backups/profiles). Это
// плохо: при переустановке или переносе программы они теряются, а на
// Windows exe обычно в Program Files, куда писать нельзя. Конфиг-папка
// (XDG_CONFIG_HOME / %APPDATA%) — стандартное место для пользовательских
// данных, переживает переустановку.
//
// Папка создаётся при каждом вызове (как раньше). Если создать не
// удалось — возвращаем "", вызывающий код это обрабатывает.
func (app *App) backupsDir() string {
	dir := filepath.Join(filepath.Dir(configFilePath()), "backups", "profiles")
	if err := os.MkdirAll(dir, 0755); err != nil {
		app.appendLogToFile(fmt.Sprintf("backupsDir: mkdir failed: %v", err))
		return ""
	}
	return dir
}

func (app *App) backupsLimit() int {
	app.cfgMutex.RLock()
	defer app.cfgMutex.RUnlock()
	if app.cfg.BackupsLimit <= 0 {
		return defaultBackupsLimit
	}
	return app.cfg.BackupsLimit
}

// CreateProfileBackup архивирует профиль в zip и подчищает старые.
func (app *App) CreateProfileBackup(profileName string) (string, error) {
	src := app.profilePath(profileName)
	info, err := os.Stat(src)
	if err != nil {
		return "", fmt.Errorf("profile '%s' not found: %w", profileName, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("profile '%s' is not a directory", profileName)
	}

	dir := app.backupsDir()
	if dir == "" {
		return "", fmt.Errorf("backups dir unavailable")
	}

	ts := time.Now().Format("20060102_150405")
	zipPath := filepath.Join(dir, fmt.Sprintf("%s_%s.zip", ts, profileName))

	if err := app.createZipArchive(src, zipPath); err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}

	app.pruneProfileBackups()
	return zipPath, nil
}

// pruneProfileBackups оставляет только N самых свежих zip-файлов.
func (app *App) pruneProfileBackups() {
	dir := app.backupsDir()
	if dir == "" {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("pruneProfileBackups: %v", err))
		return
	}

	type item struct {
		name    string
		modTime time.Time
	}
	var zips []item
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		zips = append(zips, item{name: e.Name(), modTime: info.ModTime()})
	}

	limit := app.backupsLimit()
	if len(zips) <= limit {
		return
	}

	sort.Slice(zips, func(i, j int) bool {
		return zips[i].modTime.Before(zips[j].modTime)
	})

	for i := 0; i < len(zips)-limit; i++ {
		p := filepath.Join(dir, zips[i].name)
		if err := os.Remove(p); err != nil {
			app.appendLogToFile(fmt.Sprintf("pruneProfileBackups: remove %s: %v", p, err))
		} else {
			app.appendLogToFile(fmt.Sprintf("Pruned old backup: %s", zips[i].name))
		}
	}
}

// restoreProfileFromBackup разворачивает zip в профиль. Существующий
// профиль с этим именем уходит в корзину.
func (app *App) restoreProfileFromBackup(zipPath, profileName string) error {
	tmpDir, err := os.MkdirTemp("", "profile-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := app.extractZipArchive(zipPath, tmpDir); err != nil {
		return fmt.Errorf("extract: %w", err)
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	if len(entries) != 1 || !entries[0].IsDir() {
		return fmt.Errorf("backup must contain a single profile folder")
	}
	extracted := filepath.Join(tmpDir, entries[0].Name())

	target := app.profilePath(profileName)
	if _, err := os.Stat(target); err == nil {
		if err := app.RemoveDirWithTrashFallback(target, profileName); err != nil {
			return err
		}
	}

	if err := copyPath(extracted, target); err != nil {
		return fmt.Errorf("copy extracted: %w", err)
	}
	return nil
}
