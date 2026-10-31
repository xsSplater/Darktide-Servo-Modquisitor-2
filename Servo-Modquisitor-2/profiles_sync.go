// Servo-Modquisitor-2/profiles_sync.go
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
)

// ComputeModsSignature считает стабильную сигнатуру папки mods:
// relpath + size + mtime (секунды). Используется для детекта внешних
// изменений между сессиями.
func ComputeModsSignature(modsPath string) (string, error) {
	if modsPath == "" {
		return "", nil
	}
	info, err := os.Stat(modsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", modsPath)
	}

	type entry struct {
		rel   string
		size  int64
		mtime int64
	}
	var entries []entry

	err = filepath.Walk(modsPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(modsPath, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		entries = append(entries, entry{
			rel:   filepath.ToSlash(rel),
			size:  info.Size(),
			mtime: info.ModTime().Unix(),
		})
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })

	h := sha256.New()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%d|%d\n", e.rel, e.size, e.mtime)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// updateProfileSyncSignature пересчитывает и сохраняет текущую
// сигнатуру в конфиг. Вызывается после каждой успешной синхронизации.
func (app *App) updateProfileSyncSignature() {
	app.cfgMutex.RLock()
	gamePath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	sig, err := ComputeModsSignature(gamePath)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("updateProfileSyncSignature: %v", err))
		return
	}

	app.cfgMutex.Lock()
	app.cfg.LastProfileSync = sig
	app.cfgMutex.Unlock()
	app.saveConfigSafe()
}

// syncActiveProfileFromGame копирует <game>/mods в активный профиль
// атомарно, через staging + swap.
func (app *App) syncActiveProfileFromGame() error {
	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	gamePath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	if activeProfile == "" {
		return fmt.Errorf("no active profile")
	}
	if gamePath == "" {
		return fmt.Errorf("mods path is empty")
	}

	profileMods := filepath.Join(app.profilePath(activeProfile), "mods")
	staging := profileMods + ".staging"
	oldDir := profileMods + ".old"

	_ = os.RemoveAll(staging)
	_ = os.RemoveAll(oldDir)

	if err := copyPath(gamePath, staging); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("copy to staging: %w", err)
	}

	if _, err := os.Stat(profileMods); err == nil {
		if err := os.Rename(profileMods, oldDir); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("rename old: %w", err)
		}
	}

	if err := os.Rename(staging, profileMods); err != nil {
		if _, err2 := os.Stat(oldDir); err2 == nil {
			_ = os.Rename(oldDir, profileMods)
		}
		return fmt.Errorf("rename staging: %w", err)
	}

	_ = os.RemoveAll(oldDir)
	app.updateProfileSyncSignature()

	// Стягиваем актуальные системные файлы из игры в профиль
	app.copySystemFilesToProfile(app.profilePath(activeProfile))

	return nil
}

// resetGameModsFromActiveProfile — обратная операция: заменяет <game>/mods
// содержимым активного профиля. Для «откатить game/mods из профиля».
func (app *App) resetGameModsFromActiveProfile() error {
	app.cfgMutex.RLock()
	activeProfile := app.cfg.ActiveProfile
	gamePath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	if activeProfile == "" {
		return fmt.Errorf("no active profile")
	}
	if gamePath == "" {
		return fmt.Errorf("mods path is empty")
	}

	profileMods := filepath.Join(app.profilePath(activeProfile), "mods")
	if _, err := os.Stat(profileMods); err != nil {
		return fmt.Errorf("profile mods not found: %w", err)
	}

	staging := gamePath + ".staging"
	_ = os.RemoveAll(staging)

	if err := copyPath(profileMods, staging); err != nil {
		_ = os.RemoveAll(staging)
		return fmt.Errorf("copy to staging: %w", err)
	}

	if _, err := os.Stat(gamePath); err == nil {
		if err := app.RemoveDirWithTrashFallback(gamePath, "mods"); err != nil {
			_ = os.RemoveAll(staging)
			return fmt.Errorf("remove old game/mods: %w", err)
		}
	}

	if err := os.Rename(staging, gamePath); err != nil {
		return fmt.Errorf("rename staging: %w", err)
	}

	app.updateProfileSyncSignature()

	// Раскладываем системные файлы из профиля в игру
	app.restoreSystemFilesFromProfile(app.profilePath(activeProfile))

	return nil
}

// checkProfileSyncOnStart показывает диалог, если <game>/mods разошлась
// с последней синхронизированной сигнатурой. Вызывать из фоновой горутины.
func (app *App) checkProfileSyncOnStart() {
	app.cfgMutex.RLock()
	gamePath := app.cfg.ModsPath
	activeProfile := app.cfg.ActiveProfile
	lastSig := app.cfg.LastProfileSync
	autoChoice := app.cfg.ProfileSyncAutoChoice
	app.cfgMutex.RUnlock()

	if gamePath == "" || activeProfile == "" {
		return
	}

	// Самый первый запуск: сигнатуры нет, просто фиксируем.
	if lastSig == "" {
		app.updateProfileSyncSignature()
		return
	}

	currentSig, err := ComputeModsSignature(gamePath)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("checkProfileSyncOnStart: %v", err))
		return
	}

	if currentSig == lastSig {
		return
	}

	switch autoChoice {
	case "save":
		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLogToFile(fmt.Sprintf("auto-save profile: %v", err))
		}
		return
	case "load":
		if err := app.resetGameModsFromActiveProfile(); err != nil {
			app.appendLogToFile(fmt.Sprintf("auto-load profile: %v", err))
		}
		return
	}

	choice := app.showChoiceDialogSync(
		app.mainWindow,
		app.msg("profile_sync_check_title"),
		app.msg("profile_sync_check_message"),
		app.msg("profile_sync_save_to_profile"),
		app.msg("profile_sync_load_from_profile"),
		app.msg("profile_sync_skip"),
	)

	switch choice {
	case 0:
		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLogToFile(fmt.Sprintf("manual-save profile: %v", err))
		}
	case 1:
		if err := app.resetGameModsFromActiveProfile(); err != nil {
			app.appendLogToFile(fmt.Sprintf("manual-load profile: %v", err))
		}
	}
}

// SyncProfileManually — ручной синк из меню / кнопки. Показывает
// инфинитный прогресс-бар.
func (app *App) SyncProfileManually() {
	go func() {
		_, closeDialog := app.showInfiniteProgressDialog(
			app.msg("profile_sync_title"),
			app.msg("profile_sync_message"),
			false,
		)
		defer closeDialog()

		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("profile_sync_failed"), err.Error()))
			return
		}
		app.appendLog(app.msg("profile_sync_success"))

		fyne.Do(func() {
			app.refreshModList()
		})
	}()
}

// ResetGameModsWithConfirm — «сбросить game/mods из активного профиля»
// с подтверждением.
func (app *App) ResetGameModsWithConfirm() {
	app.showConfirmDialog(
		app.msg("profile_reset_title"),
		app.msg("profile_reset_message"),
		func() {
			go func() {
				_, closeDialog := app.showInfiniteProgressDialog(
					app.msg("profile_sync_title"),
					app.msg("profile_sync_message"),
					false,
				)
				defer closeDialog()

				if err := app.resetGameModsFromActiveProfile(); err != nil {
					fyne.Do(func() {
						app.showInfoDialog(app.msg("error_title"), err.Error())
					})
					return
				}
				fyne.Do(func() {
					app.refreshModList()
					app.appendLog(app.msg("profile_reset_success"))
				})
			}()
		},
	)
}

// copySystemFilesToProfile копирует загрузчики, патчи и скрипты из корня игры в папку профиля.
func (app *App) copySystemFilesToProfile(profilePath string) {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		return
	}

	// Если это чистый профиль — он не должен хранить системные файлы
	if fileExists(filepath.Join(profilePath, CleanProfileMarker)) {
		return
	}

	// 1. DML (mod_loader)
	src := filepath.Join(gameRoot, "binaries", "mod_loader")
	if fileExists(src) {
		dstDir := filepath.Join(profilePath, "binaries")
		_ = os.MkdirAll(dstDir, 0755)
		_ = copyFile(src, filepath.Join(dstDir, "mod_loader"))
	}

	// 2. Плагины (DMA, Custom Assets и т.д.)
	pluginsSrc := filepath.Join(gameRoot, "binaries", "plugins")
	if info, err := os.Stat(pluginsSrc); err == nil && info.IsDir() {
		entries, _ := os.ReadDir(pluginsSrc)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			nameLower := strings.ToLower(e.Name())
			// Пропускаем стандартные DLL игры
			if strings.Contains(nameLower, "crash_monitor") || strings.Contains(nameLower, "grpc") ||
				strings.Contains(nameLower, "gwnav") || strings.Contains(nameLower, "lua_cjson") ||
				strings.Contains(nameLower, "rule_database") || strings.Contains(nameLower, "texturereader") ||
				strings.Contains(nameLower, "wwise") {
				continue
			}
			// Создаём папку и копируем только если нашли модовый файл
			dstDir := filepath.Join(profilePath, "binaries", "plugins")
			_ = os.MkdirAll(dstDir, 0755)
			_ = copyFile(filepath.Join(pluginsSrc, e.Name()), filepath.Join(dstDir, e.Name()))
		}
	}

	// 3. Патч bundle
	src = filepath.Join(gameRoot, "bundle", "9ba626afa44a3aa3.patch_999")
	if fileExists(src) {
		dstDir := filepath.Join(profilePath, "bundle")
		_ = os.MkdirAll(dstDir, 0755)
		_ = copyFile(src, filepath.Join(dstDir, "9ba626afa44a3aa3.patch_999"))
	}

	// 4. Tools
	toolsSrc := filepath.Join(gameRoot, "tools")
	if info, err := os.Stat(toolsSrc); err == nil && info.IsDir() {
		toolsDst := filepath.Join(profilePath, "tools")
		_ = os.RemoveAll(toolsDst) // Очищаем старую папку в профиле, чтобы не оставалось мусора
		_ = copyPath(toolsSrc, toolsDst)
	}

	// 5. Скрипты в корне игры
	for _, f := range []string{"toggle_darktide_mods.bat", "toggle_dt_mod_autopatch.cmd", "CUSTOM_ASSETS_PATCH.bat"} {
		src := filepath.Join(gameRoot, f)
		if fileExists(src) {
			_ = copyFile(src, filepath.Join(profilePath, f))
		}
	}
}

// restoreSystemFilesFromProfile раскладывает загрузчики, патчи и скрипты из профиля обратно в игру.
func (app *App) restoreSystemFilesFromProfile(profilePath string) {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		return
	}

	isClean := fileExists(filepath.Join(profilePath, CleanProfileMarker))

	// 1. DML
	src := filepath.Join(profilePath, "binaries", "mod_loader")
	dst := filepath.Join(gameRoot, "binaries", "mod_loader")
	if fileExists(src) {
		_ = os.MkdirAll(filepath.Dir(dst), 0755)
		_ = copyFile(src, dst)
	} else if isClean {
		_ = os.Remove(dst) // Удаляем, только если это чистый профиль
	}

	// 2. Плагины
	gamePluginsDir := filepath.Join(gameRoot, "binaries", "plugins")

	// Вычищаем модовые DLL из игры, только если профиль чистый
	if isClean {
		if info, err := os.Stat(gamePluginsDir); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(gamePluginsDir)
			for _, e := range entries {
				nameLower := strings.ToLower(e.Name())
				if strings.Contains(nameLower, "crash_monitor") || strings.Contains(nameLower, "grpc") ||
					strings.Contains(nameLower, "gwnav") || strings.Contains(nameLower, "lua_cjson") ||
					strings.Contains(nameLower, "rule_database") || strings.Contains(nameLower, "texturereader") ||
					strings.Contains(nameLower, "wwise") {
					continue
				}
				_ = os.Remove(filepath.Join(gamePluginsDir, e.Name()))
			}
		}
	}

	// Копируем плагины из профиля (если они там есть)
	pluginsSrc := filepath.Join(profilePath, "binaries", "plugins")
	if info, err := os.Stat(pluginsSrc); err == nil && info.IsDir() {
		_ = os.MkdirAll(gamePluginsDir, 0755)
		entries, _ := os.ReadDir(pluginsSrc)
		for _, e := range entries {
			if !e.IsDir() {
				_ = copyFile(filepath.Join(pluginsSrc, e.Name()), filepath.Join(gamePluginsDir, e.Name()))
			}
		}
	}

	// 3. Патч bundle
	src = filepath.Join(profilePath, "bundle", "9ba626afa44a3aa3.patch_999")
	dst = filepath.Join(gameRoot, "bundle", "9ba626afa44a3aa3.patch_999")
	if fileExists(src) {
		_ = os.MkdirAll(filepath.Dir(dst), 0755)
		_ = copyFile(src, dst)
	} else if isClean {
		_ = os.Remove(dst)
	}

	// 4. Tools
	toolsSrc := filepath.Join(profilePath, "tools")
	toolsDst := filepath.Join(gameRoot, "tools")
	if info, err := os.Stat(toolsSrc); err == nil && info.IsDir() {
		_ = os.RemoveAll(toolsDst)
		_ = copyPath(toolsSrc, toolsDst)
	} else if isClean {
		_ = os.RemoveAll(toolsDst)
	}

	// 5. Скрипты в корне игры
	for _, f := range []string{"toggle_darktide_mods.bat", "toggle_dt_mod_autopatch.cmd", "CUSTOM_ASSETS_PATCH.bat"} {
		src := filepath.Join(profilePath, f)
		dst := filepath.Join(gameRoot, f)
		if fileExists(src) {
			_ = copyFile(src, dst)
		} else if isClean {
			_ = os.Remove(dst)
		}
	}
}
