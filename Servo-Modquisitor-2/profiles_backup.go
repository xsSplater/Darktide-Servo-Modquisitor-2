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

	// Создаём временную папку для сборки полного бэкапа
	tmpDir, err := os.MkdirTemp("", "smq-backup-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	// 1. Копируем профиль (mods/ и nexus_versions.json)
	dstProfile := filepath.Join(tmpDir, profileName)
	if err := copyPath(src, dstProfile); err != nil {
		return "", fmt.Errorf("copy profile: %w", err)
	}

	// 2. Докладываем системные компоненты из корня игры
	gameRoot, _ := app.getGameState()
	if gameRoot != "" {
		// DML
		modLoaderSrc := filepath.Join(gameRoot, "binaries", "mod_loader")
		if fileExists(modLoaderSrc) {
			_ = copyFile(modLoaderSrc, filepath.Join(dstProfile, "binaries", "mod_loader"))
		}

		// DLL из binaries/plugins (DMA, Custom Assets и т.д.)
		pluginsSrc := filepath.Join(gameRoot, "binaries", "plugins")
		if info, err := os.Stat(pluginsSrc); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(pluginsSrc)
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				nameLower := strings.ToLower(e.Name())
				// Пропускаем стандартные DLL игры, чтобы случайно не сломать её при восстановлении
				if strings.Contains(nameLower, "crash_monitor") ||
					strings.Contains(nameLower, "grpc") ||
					strings.Contains(nameLower, "gwnav") ||
					strings.Contains(nameLower, "lua_cjson") ||
					strings.Contains(nameLower, "rule_database") ||
					strings.Contains(nameLower, "texturereader") ||
					strings.Contains(nameLower, "wwise") {
					continue
				}
				_ = copyFile(filepath.Join(pluginsSrc, e.Name()), filepath.Join(dstProfile, "binaries", "plugins", e.Name()))
			}
		}

		// Патч bundle (только patch_999)
		patchSrc := filepath.Join(gameRoot, "bundle", "9ba626afa44a3aa3.patch_999")
		if fileExists(patchSrc) {
			_ = copyFile(patchSrc, filepath.Join(dstProfile, "bundle", "9ba626afa44a3aa3.patch_999"))
		}

		// Tools (dtkit-patch)
		toolsSrc := filepath.Join(gameRoot, "tools")
		if info, err := os.Stat(toolsSrc); err == nil && info.IsDir() {
			_ = copyPath(toolsSrc, filepath.Join(dstProfile, "tools"))
		}

		// Toggle-скрипты
		for _, f := range []string{"toggle_darktide_mods.bat", "toggle_dt_mod_autopatch.cmd"} {
			fSrc := filepath.Join(gameRoot, f)
			if fileExists(fSrc) {
				_ = copyFile(fSrc, filepath.Join(dstProfile, f))
			}
		}
	}

	// 3. Архивируем временную папку
	if err := app.createZipArchive(dstProfile, zipPath); err != nil {
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

	// 1. Копируем файлы профиля (mods/ и nexus_versions.json)
	if fileExists(filepath.Join(extracted, "mods")) {
		if err := copyPath(filepath.Join(extracted, "mods"), filepath.Join(target, "mods")); err != nil {
			return fmt.Errorf("copy extracted mods: %w", err)
		}
	}
	if fileExists(filepath.Join(extracted, "nexus_versions.json")) {
		if err := copyFile(filepath.Join(extracted, "nexus_versions.json"), filepath.Join(target, "nexus_versions.json")); err != nil {
			app.appendLogToFile(fmt.Sprintf("restoreProfileFromBackup: copy nexus_versions.json: %v", err))
		}
	}

	// 2. Копируем системные компоненты в корень игры
	gameRoot, _ := app.getGameState()
	if gameRoot != "" {
		if fileExists(filepath.Join(extracted, "binaries", "mod_loader")) {
			_ = copyFile(filepath.Join(extracted, "binaries", "mod_loader"), filepath.Join(gameRoot, "binaries", "mod_loader"))
		}

		pluginsExtracted := filepath.Join(extracted, "binaries", "plugins")
		if info, err := os.Stat(pluginsExtracted); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(pluginsExtracted)
			for _, e := range entries {
				if !e.IsDir() {
					_ = copyFile(filepath.Join(pluginsExtracted, e.Name()), filepath.Join(gameRoot, "binaries", "plugins", e.Name()))
				}
			}
		}

		if fileExists(filepath.Join(extracted, "bundle", "9ba626afa44a3aa3.patch_999")) {
			_ = copyFile(filepath.Join(extracted, "bundle", "9ba626afa44a3aa3.patch_999"), filepath.Join(gameRoot, "bundle", "9ba626afa44a3aa3.patch_999"))
		}

		if info, err := os.Stat(filepath.Join(extracted, "tools")); err == nil && info.IsDir() {
			_ = copyPath(filepath.Join(extracted, "tools"), filepath.Join(gameRoot, "tools"))
		}

		for _, f := range []string{"toggle_darktide_mods.bat", "toggle_dt_mod_autopatch.cmd"} {
			if fileExists(filepath.Join(extracted, f)) {
				_ = copyFile(filepath.Join(extracted, f), filepath.Join(gameRoot, f))
			}
		}
	}

	return nil
}
