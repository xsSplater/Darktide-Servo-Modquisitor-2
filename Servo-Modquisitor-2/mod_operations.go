// Servo-Modquisitor-2/mod_operations.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/sorter"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	starkzip "github.com/STARRY-S/zip"
	"github.com/mholt/archives"
)

// Sentinel-ошибки распаковки. Оборачиваются через %w, чтобы вызывающая
// сторона могла отличить «отвергли по лимиту/безопасности» от «формат
// не тот» и, например, не пробовать fallback на другой распаковщик.
var (
	errArchiveLimitExceeded = errors.New("archive: size limit exceeded")
	errArchiveSecurity      = errors.New("archive: security violation")
)

func safeJoin(destDir, name string) (string, error) {
	// Приводим имя к Unix-стилю для проверок (заменяем \ на /)
	cleanName := strings.ReplaceAll(name, "\\", "/")
	// Запрещаем абсолютные пути (Unix-стиль)
	if strings.HasPrefix(cleanName, "/") {
		return "", fmt.Errorf("absolute path not allowed")
	}
	// Запрещаем Windows-абсолютные пути (содержат :/ или :\\)
	if strings.Contains(cleanName, ":/") || strings.Contains(cleanName, ":\\") {
		return "", fmt.Errorf("absolute path not allowed")
	}
	// Собираем путь с использованием системного разделителя
	full := filepath.Join(destDir, name)
	// Проверяем, что результирующий путь действительно внутри destDir
	rel, err := filepath.Rel(destDir, full)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path traversal detected")
	}
	return full, nil
}

func (app *App) refreshModList() {
	// Проверяем, что UI-виджеты созданы
	if app.modTable == nil || app.headerTable == nil || app.systemModsTable == nil {
		return
	}
	if app.mainWindow == nil || app.mainWindow.Canvas() == nil {
		return
	}

	// Сброс множественного выделения при обновлении списка
	if app.modTable != nil {
		app.modTable.ClearSelection()
	}

	app.cfgMutex.RLock()
	lang := app.cfg.Language
	forceEnglish := app.cfg.ForceEnglishModNames
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	mods := checks.GetModsInfo(lang, forceEnglish)

	var sysMods, regMods []checks.ModInfo
	for _, m := range mods {
		if m.IsSystem {
			m.Active = false
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}

	entries := checks.ReadLoadOrder()
	if entries == nil {
		sort.Slice(regMods, func(i, j int) bool { return regMods[i].Name < regMods[j].Name })
		for i := range regMods {
			regMods[i].Active = false
		}
	} else {
		activeMap := make(map[string]bool)
		for _, e := range entries {
			activeMap[e.Name] = e.Active
		}
		for i := range regMods {
			if act, ok := activeMap[regMods[i].Name]; ok {
				regMods[i].Active = act
			} else {
				regMods[i].Active = false
			}
		}

		orderMap := make(map[string]int)
		for i, e := range entries {
			orderMap[e.Name] = i
		}
		for i := range regMods {
			if _, ok := orderMap[regMods[i].Name]; !ok {
				orderMap[regMods[i].Name] = len(orderMap)
			}
		}
		sort.Slice(regMods, func(i, j int) bool {
			return orderMap[regMods[i].Name] < orderMap[regMods[j].Name]
		})
	}

	// Цикл для обработки обычных модов (regMods)
	// Один снимок на всю функцию — не дёргаем мьютекс в цикле
	incompatiblePairs := checks.GetIncompatiblePairs()

	for i := range regMods {
		regMods[i].Obsolete = helpers.ContainsString(checks.GetObsoleteMods(), regMods[i].Name)
		regMods[i].Mandatory = checks.IsMandatoryMod(regMods[i].Name)
		regMods[i].Incompatible = app.checkIncompatible(regMods[i].Name)

		// Описание несовместимости в таблице в Примечании
		if regMods[i].Incompatible {
			for _, pair := range incompatiblePairs {
				if pair.Mod1 == regMods[i].Name || pair.Mod2 == regMods[i].Name {
					other := pair.Mod1
					if other == regMods[i].Name {
						other = pair.Mod2
					}
					if checks.FolderExists(other) {
						conflictLang := lang
						if forceEnglish {
							conflictLang = "en"
						}
						displayName := other
						if entry := checks.GetModDBEntry(other); entry != nil {
							if name := checks.PickLocalized(entry.Name, conflictLang); name != "" {
								displayName = name
							}
						}
						conflictText := app.msg("conflict_with") + displayName
						regMods[i].Note = checks.JoinNotes(regMods[i].Note, conflictText)
						break
					}
				}
			}
		}

		// Проверка на симлинк для обычных модов
		modPath := filepath.Join(modsPath, regMods[i].Name)
		info, err := os.Lstat(modPath)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			regMods[i].IsSymlink = true
		} else {
			regMods[i].IsSymlink = false
		}

		// Определяем Source для обычных модов
		var cacheKey string
		switch regMods[i].Name {
		case "dmf":
			cacheKey = "8:dmf"
		case "base":
			cacheKey = "19:base"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if regMods[i].URL != "" {
				modID := helpers.ExtractModIDFromURL(regMods[i].URL)
				if modID != 0 {
					cacheKey = fmt.Sprintf("%d:%s", modID, regMods[i].Name)
				}
			}
		}
		if cacheKey != "" {
			if info, ok := app.getCachedVersion(cacheKey); ok {
				regMods[i].Source = info.Source
			} else {
				regMods[i].Source = "manual"
			}
		} else {
			regMods[i].Source = "manual"
		}
	}

	// Заполняем IsSymlink и Source для системных модов
	for i := range sysMods {
		modPath := filepath.Join(modsPath, sysMods[i].Name)
		info, err := os.Lstat(modPath)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			sysMods[i].IsSymlink = true
		} else {
			sysMods[i].IsSymlink = false
		}

		var cacheKey string
		switch sysMods[i].Name {
		case "dmf":
			cacheKey = "8:dmf"
		case "base":
			cacheKey = "19:base"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if sysMods[i].URL != "" {
				modID := helpers.ExtractModIDFromURL(sysMods[i].URL)
				if modID != 0 {
					cacheKey = fmt.Sprintf("%d:%s", modID, sysMods[i].Name)
				}
			}
		}
		if cacheKey != "" {
			if info, ok := app.getCachedVersion(cacheKey); ok {
				sysMods[i].Source = info.Source
			} else {
				sysMods[i].Source = "manual"
			}
		} else {
			sysMods[i].Source = "manual"
		}
	}

	// Цикл для MissingFolder
	for i := range regMods {
		if regMods[i].MissingFolder {
			regMods[i].Active = false
		}
	}

	// Восстановление выделения
	if app.selectedModName != "" {
		exists := false
		for _, m := range regMods {
			if m.Name == app.selectedModName {
				exists = true
				break
			}
		}
		if !exists {
			app.selectedModName = ""
		}
	}

	// AML
	wasAML := app.amlDetected.Load()
	app.amlDetected.Store(checks.IsAMLInstalled(modsPath))
	if wasAML != app.amlDetected.Load() {
		if app.amlDetected.Load() {
			app.btnSaveOrder.SetText(app.msg("btn_save_order_aml"))
			app.btnSortChecks.SetText(app.msg("btn_sort_checks_aml"))
			app.btnSaveOrder.SetToolTip(app.msg("aml_save_warning_tooltip"))
			app.btnSortChecks.SetToolTip(app.msg("aml_sort_warning_tooltip"))
		} else {
			app.btnSaveOrder.SetText(app.msg("btn_save_order"))
			app.btnSortChecks.SetText(app.msg("btn_sort_checks"))
			app.btnSaveOrder.SetToolTip(app.msg("btn_save_order_tooltip"))
			app.btnSortChecks.SetToolTip(app.msg("btn_sort_checks_tooltip"))
		}
	}

	// Установка флага HasUpdate
	for i := range regMods {
		mod := &regMods[i]
		var cacheKey string
		switch mod.Name {
		case "dmf":
			cacheKey = "8:dmf"
		case "base":
			cacheKey = "19:base"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if mod.URL != "" {
				modID := helpers.ExtractModIDFromURL(mod.URL)
				if modID != 0 {
					cacheKey = fmt.Sprintf("%d:%s", modID, mod.Name)
				}
			}
		}
		if cacheKey != "" {
			if saved, ok := app.getCachedVersion(cacheKey); ok {
				if latest, ok := app.getLatestVersion(cacheKey); ok {
					mod.HasUpdate = compareVersions(latest, saved.Version) > 0
				} else {
					mod.HasUpdate = false
				}
			} else {
				mod.HasUpdate = false
			}
		} else {
			mod.HasUpdate = false
		}
	}

	app.modsMutex.Lock()
	app.allMods = regMods
	app.systemMods = sysMods
	app.orderDirty = false
	app.modsMutex.Unlock()

	// Очистка кэша от записей, не соответствующих существующим модам
	app.pruneVersionCache()

	app.filterModList()

	app.updateSystemModsTable()

	app.forceRefreshTable()
}

func (app *App) updateSystemModsTable() {
	if app.systemModsTable == nil {
		app.appendLogToFile("updateSystemModsTable: systemModsTable is nil, skipping")
		return
	}
	app.modsMutex.RLock()
	n := len(app.systemMods)
	app.modsMutex.RUnlock()

	// Обновляем высоту контейнера под фактическое число строк.
	// Раньше высота была фиксированной (75 = 2 строки), и третий
	// системный мод (autopatch) не помещался в контейнер.
	if app.systemModsTableSpacer != nil {
		app.systemModsTableSpacer.SetMinSize(fyne.NewSize(
			1, SystemTableRowHeight*float32(n)))
		app.systemModsTableSpacer.Refresh()
	}

	app.systemModsTable.Length = func() (int, int) { return n, TableColumnCount }
	app.systemModsTable.Refresh()
}

func (app *App) saveCurrentOrder() {
	app.loadOrderMutex.Lock()
	defer app.loadOrderMutex.Unlock()

	entries := app.buildLoadOrderEntries()
	checks.WriteLoadOrder(entries)
	// После сохранения в профиле - синхронизируем с игровой папкой
	app.syncLoadOrderToGameLocked()
}

func (app *App) buildLoadOrderEntries() []checks.LoadOrderEntry {
	app.modsMutex.RLock()
	defer app.modsMutex.RUnlock()
	entries := make([]checks.LoadOrderEntry, len(app.allMods))
	for i, m := range app.allMods {
		entries[i] = checks.LoadOrderEntry{Name: m.Name, Active: m.Active}
	}
	return entries
}

func (app *App) toggleModActive(name string, active bool) {
	app.modsMutex.Lock()
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			app.allMods[i].Active = active
			app.orderDirty = true
			break
		}
	}
	app.modsMutex.Unlock()
	app.updateTableBorder()
	app.filterModList()
	app.forceRefreshTable()
}

// findModByName возвращает КОПИЮ ModInfo. Указатель наружу отдавать
// нельзя: после RUnlock любое чтение полей гоняется с параллельными
// записями в allMods. Копия безопасна.
func (app *App) findModByName(name string) (checks.ModInfo, bool) {
	app.modsMutex.RLock()
	defer app.modsMutex.RUnlock()
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			return app.allMods[i], true
		}
	}
	for i := range app.systemMods {
		if app.systemMods[i].Name == name {
			return app.systemMods[i], true
		}
	}
	return checks.ModInfo{}, false
}

func (app *App) removeFromAllMods(name string) {
	app.modsMutex.Lock()
	defer app.modsMutex.Unlock()
	for i, m := range app.allMods {
		if m.Name == name {
			app.allMods = append(app.allMods[:i], app.allMods[i+1:]...)
			break
		}
	}
}

func (app *App) toggleGlobalMods() {
	gameRoot, patcher := app.getGameState()
	switch patcher {
	case PatcherAutoPatch:
		err := toggleModsAutoPatch(gameRoot)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf(app.msg("log_toggle_fail"), err))
		} else {
			enabled := isModsEnabledAutoPatch(gameRoot)
			app.cfgMutex.Lock()
			app.cfg.ModsGloballyEnabled = enabled
			app.cfgMutex.Unlock()
			state := app.msg("log_mods_enabled")
			if !enabled {
				state = app.msg("log_mods_disabled")
			}
			app.appendLog(state + app.msg("log_autopatcher"))
		}
	case PatcherLegacy:
		err := toggleModsLegacy(gameRoot)
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_toggle_fail"), err))
		} else {
			app.syncModsEnabledState()
			app.updateToggleButtonText(app.btnToggle)
			app.cfgMutex.RLock()
			enabled := app.cfg.ModsGloballyEnabled
			app.cfgMutex.RUnlock()
			state := app.msg("log_mods_enabled")
			if !enabled {
				state = app.msg("log_mods_disabled")
			}
			app.appendLog(state + app.msg("log_autopatcher_old"))
		}
	default:
		app.appendLog(app.msg("log_no_patcher"))
	}
	app.updateToggleButtonText(app.btnToggle)
	app.saveConfigSafe()
}

func (app *App) handleDrop(uris []fyne.URI) {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()

	for _, uri := range uris {
		path := uri.Path()
		info, err := os.Stat(path)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf(app.msg("log_error_drop"), err))
			continue
		}
		if info.IsDir() {
			app.copyFolder(path, filepath.Join(modsPath, filepath.Base(path)))
			checks.AutoFixMalformed()
			app.refreshModList()
			app.orderDirty = true
			app.updateTableBorder()
			app.appendLog(fmt.Sprintf(app.msg("log_installed_folder"), filepath.Base(path)))
		} else {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".zip" || ext == ".rar" || ext == ".7z" {
				go func(p string) {
					installedName, _, err := app.InstallModFromArchive(p, true, "", "")
					fyne.Do(func() {
						if err != nil {
							app.appendLog(fmt.Sprintf(app.msg("log_extract_error"), err))
							return
						}
						checks.AutoFixMalformed()
						app.fixHubHotkeyMenus()
						app.refreshModList()
						if installedName != "" {
							app.selectAndScrollToMod(installedName)
							modID, _, _ := extractVersionAndModIDFromFilename(p)
							if modID != 0 {
								go app.autoAddModToDatabase(modID, installedName, filepath.Base(p))
							}
							app.orderDirty = true
							app.updateTableBorder()
							app.appendLog(fmt.Sprintf(app.msg("log_installed"), filepath.Base(p)))
						} else {
							app.appendLog(app.msg("log_sorting_files_updated_manual"))
						}
					})
				}(path)
			} else {
				app.appendLog(app.msg("log_zip_only"))
			}
		}
	}
}

func (app *App) extractArchive(archivePath string) error {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	return app.extractArchiveTo(archivePath, modsPath)
}

func (app *App) has7z() bool {
	_, err := exec.LookPath("7z")
	return err == nil
}

func (app *App) extractZipWith7z(archivePath, destDir string) error {
	if !app.has7z() {
		return fmt.Errorf("7z not found in system PATH")
	}
	cmd := exec.Command("7z", "x", archivePath, "-o"+destDir, "-y")
	cmd.Dir = destDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("7z failed: %v, output: %s", err, output)
	}
	return nil
}

// extractArchiveTo распаковывает ZIP, RAR, 7z и другие архивы.
func (app *App) extractArchiveTo(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat archive: %w", err)
	}
	if info.Size() > MaxArchiveSize {
		return fmt.Errorf("archive size %d exceeds limit %d", info.Size(), MaxArchiveSize)
	}
	if info.Size() < 100 {
		return fmt.Errorf("archive file too small (%d bytes), probably corrupted", info.Size())
	}

	header := make([]byte, 20)
	if _, err := f.Read(header); err != nil {
		return fmt.Errorf("failed to read archive header: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	isZip := len(header) >= 4 && header[0] == 0x50 && header[1] == 0x4B && header[2] == 0x03 && header[3] == 0x04
	is7z := len(header) >= 6 && header[0] == 0x37 && header[1] == 0x7A && header[2] == 0xBC && header[3] == 0xAF && header[4] == 0x27 && header[5] == 0x1C
	isRar := len(header) >= 6 && header[0] == 0x52 && header[1] == 0x61 && header[2] == 0x72 && header[3] == 0x21 && header[4] == 0x1A && header[5] == 0x07

	if isZip {
		app.appendLogToFile("Detected ZIP format")
	} else if isRar {
		app.appendLogToFile("Detected RAR format")
	} else if is7z {
		app.appendLogToFile("Detected 7z format")
	}

	if !isZip && !is7z && !isRar {
		app.appendLogToFile(fmt.Sprintf("Unrecognized archive format for %s (magic: %x), skipping", archivePath, header[:8]))
		return fmt.Errorf("unrecognized archive format")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if isZip {
		app.appendLogToFile("Using STARRY-S/zip for extraction")
		err := app.extractZipWithSTARRY(archivePath, destDir)
		if err == nil {
			return nil
		}

		if errors.Is(err, errArchiveLimitExceeded) || errors.Is(err, errArchiveSecurity) {
			app.appendLogToFile(fmt.Sprintf("STARRY-S/zip rejected archive: %v", err))
			return err
		}

		app.appendLogToFile(fmt.Sprintf("STARRY-S/zip failed: %v", err))
		app.appendLogToFile("Trying external 7z...")
		if err2 := app.extractZipWith7z(archivePath, destDir); err2 != nil {
			return fmt.Errorf("Both STARRY-S/zip and 7z failed. Please extract the archive manually.\nError: %v", err2)
		}
		app.appendLogToFile("7z extraction succeeded")
		return nil
	}

	var extractor archives.Extractor
	switch {
	case isRar:
		extractor = archives.Rar{}
	case is7z:
		extractor = archives.SevenZip{}
	default:
		return fmt.Errorf("unsupported archive format")
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	baseTemp := app.getProgramTempDir()
	tmpDir, err := os.MkdirTemp(baseTemp, "servo-extract-")
	if err != nil {
		tmpDir, err = os.MkdirTemp("", "servo-extract-")
		if err != nil {
			return err
		}
	}
	defer os.RemoveAll(tmpDir)

	var totalExtracted int64
	var fileCount int
	var extractErr error

	done := make(chan bool, 1)
	go func() {
		extractErr = extractor.Extract(ctx, f, func(ctx context.Context, fi archives.FileInfo) error {
			if fi.Size() > MaxFileSize {
				return fmt.Errorf("file %s size %d exceeds limit %d", fi.NameInArchive, fi.Size(), MaxFileSize)
			}
			targetPath, err := safeJoin(tmpDir, fi.NameInArchive)
			if err != nil {
				return err
			}

			if fi.IsDir() {
				if err := app.ensureDir(targetPath); err != nil {
					return err
				}
				return nil
			}

			parentDir := filepath.Dir(targetPath)
			if err := app.ensureDir(parentDir); err != nil {
				return err
			}

			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			defer out.Close()

			rc, err := fi.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			remaining := MaxExtractedSize - totalExtracted
			if remaining <= 0 {
				return fmt.Errorf("%w: total extracted would exceed %d",
					errArchiveLimitExceeded, MaxExtractedSize)
			}
			perFile := fi.Size()
			if perFile > MaxFileSize {
				perFile = MaxFileSize
			}
			if remaining < perFile {
				perFile = remaining
			}
			n, copyErr := io.Copy(out, io.LimitReader(rc, perFile+1))
			if copyErr != nil {
				return copyErr
			}
			if n > perFile {
				return fmt.Errorf("%w: file %q actually exceeded size limit",
					errArchiveLimitExceeded, fi.NameInArchive)
			}
			totalExtracted += n
			if totalExtracted > MaxExtractedSize {
				return fmt.Errorf("%w: total extracted %d exceeds limit %d",
					errArchiveLimitExceeded, totalExtracted, MaxExtractedSize)
			}
			fileCount++
			return nil
		})
		done <- true
	}()

	select {
	case <-done:
		if extractErr != nil {
			return extractErr
		}
	case <-ctx.Done():
		return fmt.Errorf("extraction timed out")
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("archive is empty or extraction produced no files")
	}
	for _, e := range entries {
		src := filepath.Join(tmpDir, e.Name())
		dst := filepath.Join(destDir, e.Name())
		if err := copyPath(src, dst); err != nil {
			return err
		}
	}
	return nil
}

// extractZipWithSTARRY распаковывает ZIP, применяя продуктовые лимиты
// (MaxFileSize / MaxExtractedSize). Это тонкая обёртка над
// extractZipWithLimits — параметры вынесены, чтобы юнит-тесты могли
// прогонять сценарии с маленькими лимитами без создания 500 МБ файлов.
func (app *App) extractZipWithSTARRY(archivePath, destDir string) error {
	return app.extractZipWithLimits(archivePath, destDir, MaxFileSize, MaxExtractedSize)
}

// extractZipWithLimits — реализация. maxFile — максимум на один файл,
// maxTotal — максимум на весь архив (сумма распакованных байт).
//
// Защита многоуровневая:
//  1. safeJoin нормализует путь и отсекает path traversal.
//  2. Заявленный в заголовке размер > maxFile отсекается сразу.
//  3. Реальный размер контролируется io.LimitReader.
//  4. Сумма по всем файлам сверяется с maxTotal после каждого файла.
//
// Файлы закрываются явно, без defer в цикле.
func (app *App) extractZipWithLimits(archivePath, destDir string, maxFile, maxTotal int64) error {
	r, err := starkzip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer r.Close()

	var totalExtracted int64

	for _, f := range r.File {
		targetPath, err := safeJoin(destDir, f.Name)
		if err != nil {
			return fmt.Errorf("%w: zip entry %q: %v", errArchiveSecurity, f.Name, err)
		}

		claimed := f.FileInfo().Size()
		if claimed > maxFile {
			return fmt.Errorf("%w: zip entry %q claimed size %d exceeds per-file limit %d",
				errArchiveLimitExceeded, f.Name, claimed, maxFile)
		}

		if f.FileInfo().IsDir() {
			if err := app.ensureDir(targetPath); err != nil {
				return err
			}
			continue
		}

		if err := app.ensureDir(filepath.Dir(targetPath)); err != nil {
			app.appendLogToFile(fmt.Sprintf("ensureDir failed for %s: %v", targetPath, err))
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open zip entry %q: %w", f.Name, err)
		}

		out, err := os.Create(targetPath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("create %s: %w", targetPath, err)
		}

		remaining := maxTotal - totalExtracted
		if remaining <= 0 {
			rc.Close()
			out.Close()
			os.Remove(targetPath)
			return fmt.Errorf("%w: total extracted size would exceed %d (already %d)",
				errArchiveLimitExceeded, maxTotal, totalExtracted)
		}

		perFile := maxFile
		if remaining < perFile {
			perFile = remaining
		}

		written, copyErr := io.Copy(out, io.LimitReader(rc, perFile+1))

		outErr := out.Close()
		rcErr := rc.Close()

		if copyErr != nil {
			os.Remove(targetPath)
			return fmt.Errorf("copy %q: %w", f.Name, copyErr)
		}
		if outErr != nil {
			os.Remove(targetPath)
			return fmt.Errorf("close %s: %w", targetPath, outErr)
		}
		if rcErr != nil {
			app.appendLogToFile(fmt.Sprintf("close zip reader for %q: %v", f.Name, rcErr))
		}

		if written > perFile {
			os.Remove(targetPath)
			return fmt.Errorf("%w: zip entry %q actually exceeded size limit (wrote > %d)",
				errArchiveLimitExceeded, f.Name, perFile)
		}

		totalExtracted += written
		if totalExtracted > maxTotal {
			return fmt.Errorf("%w: total extracted %d exceeds limit %d",
				errArchiveLimitExceeded, totalExtracted, maxTotal)
		}
	}

	return nil
}

func (app *App) copyFolder(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(src, path)
		targetPath, err := safeJoin(dst, relPath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0644)
	})
}

func (app *App) syncModsEnabledState() {
	gameRoot, patcher := app.getGameState()
	switch patcher {
	case PatcherAutoPatch:
		enabled := isModsEnabledAutoPatch(gameRoot)
		app.cfgMutex.Lock()
		app.cfg.ModsGloballyEnabled = enabled
		app.cfgMutex.Unlock()
	case PatcherLegacy:
		enabled := isModsEnabledLegacy(gameRoot)
		app.cfgMutex.Lock()
		app.cfg.ModsGloballyEnabled = enabled
		app.cfgMutex.Unlock()
	}
	app.saveConfigSafe()
}

func (app *App) selectedMods() []string {
	app.modsMutex.RLock()
	defer app.modsMutex.RUnlock()
	var names []string
	for _, m := range app.allMods {
		if m.Selected {
			names = append(names, m.Name)
		}
	}
	return names
}

func (app *App) moveSelected(delta int) {
	if app.selectedModName == "" {
		return
	}
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}

	app.modsMutex.Lock()

	if len(selNames) == 1 {
		idx := app.findModIndexByName(selNames[0])
		if idx == -1 {
			app.modsMutex.Unlock()
			return
		}
		newIdx := idx + delta
		if newIdx < 0 || newIdx >= len(app.allMods) {
			app.modsMutex.Unlock()
			return
		}
		app.allMods[idx], app.allMods[newIdx] = app.allMods[newIdx], app.allMods[idx]
		app.orderDirty = true
	} else {
		var selected []checks.ModInfo
		var others []checks.ModInfo
		for _, m := range app.allMods {
			if m.Selected {
				selected = append(selected, m)
			} else {
				others = append(others, m)
			}
		}
		originalFirstIdx := -1
		for i, m := range app.allMods {
			if m.Selected {
				originalFirstIdx = i
				break
			}
		}
		if originalFirstIdx == -1 {
			app.modsMutex.Unlock()
			return
		}
		insertIdx := originalFirstIdx + delta
		if insertIdx < 0 {
			insertIdx = 0
		} else if insertIdx > len(others) {
			insertIdx = len(others)
		}
		var result []checks.ModInfo
		result = append(result, others[:insertIdx]...)
		result = append(result, selected...)
		result = append(result, others[insertIdx:]...)
		app.allMods = result
		app.orderDirty = true
	}

	app.modsMutex.Unlock()

	app.updateTableBorder()
	app.filterModList()
	app.applySelectionFromMods()
	app.forceRefreshTable()
}

func (app *App) moveSelectedToTop() {
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}

	app.modsMutex.Lock()

	var selected []checks.ModInfo
	var others []checks.ModInfo
	for _, m := range app.allMods {
		if m.Selected {
			selected = append(selected, m)
		} else {
			others = append(others, m)
		}
	}
	app.allMods = append(selected, others...)
	app.orderDirty = true

	app.modsMutex.Unlock()

	app.updateTableBorder()
	app.filterModList()
	app.applySelectionFromMods()
	app.forceRefreshTable()
}

func (app *App) moveSelectedToBottom() {
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}

	app.modsMutex.Lock()

	var selected []checks.ModInfo
	var others []checks.ModInfo
	for _, m := range app.allMods {
		if m.Selected {
			selected = append(selected, m)
		} else {
			others = append(others, m)
		}
	}
	app.allMods = append(others, selected...)
	app.orderDirty = true

	app.modsMutex.Unlock()

	app.updateTableBorder()
	app.filterModList()
	app.applySelectionFromMods()
	app.forceRefreshTable()
}

func (app *App) moveSelectedToPosition() {
	if app.selectedModName == "" {
		return
	}
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}
	posStr := app.moveToEntry.Text
	if posStr == "" {
		return
	}
	visiblePos, err := strconv.Atoi(posStr)
	if err != nil || visiblePos < 1 || visiblePos > len(app.allMods) {
		app.appendLog(app.msg("log_invalid_position"))
		return
	}
	targetIdx := visiblePos - 1

	app.modsMutex.Lock()

	var selected []checks.ModInfo
	var others []checks.ModInfo
	for _, m := range app.allMods {
		if m.Selected {
			selected = append(selected, m)
		} else {
			others = append(others, m)
		}
	}
	if targetIdx > len(others) {
		targetIdx = len(others)
	}
	var result []checks.ModInfo
	result = append(result, others[:targetIdx]...)
	result = append(result, selected...)
	result = append(result, others[targetIdx:]...)

	app.allMods = result
	app.orderDirty = true

	app.modsMutex.Unlock()

	app.updateTableBorder()
	app.filterModList()
	app.applySelectionFromMods()
	app.forceRefreshTable()
}

func (app *App) findModIndexByName(name string) int {
	app.modsMutex.RLock()
	defer app.modsMutex.RUnlock()
	for i, m := range app.allMods {
		if m.Name == name {
			return i
		}
	}
	return -1
}

func (app *App) selectModByName(name string) {
	// (#4) Снимок displayedMods — под RLock, чтобы итерация не гонялась
	// с параллельной перестройкой в filterModList.
	app.modsMutex.RLock()
	names := make([]string, len(app.displayedMods))
	for i := range app.displayedMods {
		names[i] = app.displayedMods[i].Name
	}
	app.modsMutex.RUnlock()

	for i, n := range names {
		if n == name {
			app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
			break
		}
	}
}

// archiveKind — классификация содержимого распакованного архива.
type archiveKind int

const (
	archiveKindMod     archiveKind = iota // обычный мод (одна или несколько папок)
	archiveKindSorting                    // файлы сортировки (mod_database.json / mandatory_rules)
	archiveKindProgram                    // архив самой программы Servo-Modquisitor
)

// InstallModFromArchive — оркестратор установки. Реальная работа разбита
// на шаги; каждый шаг — отдельный метод с одной обязанностью.
//
// Возвращает (installedName, version, error). installedName — имя первого
// установленного мода (исторически возвращается один мод, даже если
// установлено несколько); version — итоговая версия (может быть пустой
// или "unknown").
func (app *App) InstallModFromArchive(archivePath string, activate bool, knownVersion string, modNameToUpdate string) (string, string, error) {
	app.appendLogToFile(fmt.Sprintf("InstallModFromArchive: starting for archive %s, modNameToUpdate=%s", archivePath, modNameToUpdate))

	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	gameRoot, _ := app.getGameState()

	// 1. Распаковка во временную папку + нормализация структуры.
	tmpDir, err := app.prepareInstallTempDir(archivePath, modNameToUpdate)
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	// 2. Классификация архива.
	switch classifyArchive(tmpDir) {
	case archiveKindProgram:
		app.appendLogToFile("Program archive detected. Automatic update is not supported. Please install manually.")
		return "", "", fmt.Errorf("program update not supported")

	case archiveKindSorting:
		if err := app.installSortingArchive(tmpDir, modsPath); err != nil {
			return "", "", err
		}
		return "", "", nil
	}

	// 3. Обычный мод: копируем файлы.
	installedNames, err := app.installModFiles(tmpDir, modsPath, gameRoot)
	if err != nil {
		return "", "", err
	}
	if len(installedNames) == 0 {
		return "", "", errors.New(app.msg("log_no_mod_folder_found"))
	}
	installedName := installedNames[0] // для обратной совместимости возвращаем первый

	// 4. Определяем modID и версию.
	modID, version := app.resolveModIDAndVersion(archivePath, installedName, knownVersion)

	// 5. Обновляем UI и активируем мод, если нужно.
	app.finalizeModInstallUI(installedName, activate, archivePath)

	// 6. Кешируем версию.
	app.cacheInstalledVersion(modID, installedName, version)

	return installedName, version, nil
}

// ─────────────────────────────────────────────────────────────────
// Шаги установки
// ─────────────────────────────────────────────────────────────────

// prepareInstallTempDir создаёт временную папку для распаковки, опционально
// удаляет старую папку обновляемого мода, распаковывает архив и нормализует
// структуру.
//
// Возвращает путь к временной папке; вызывающая сторона обязана удалить её
// через `defer os.RemoveAll(tmpDir)`.
func (app *App) prepareInstallTempDir(archivePath, modNameToUpdate string) (string, error) {
	// Удаляем старую папку мода, если это обновление.
	if modNameToUpdate != "" {
		checks.RemoveMod(modNameToUpdate)
		app.appendLog(fmt.Sprintf(app.msg("removed_old_folder"), modNameToUpdate))
	}

	baseTemp := app.getProgramTempDir()
	tmpDir, err := os.MkdirTemp(baseTemp, "servo-mod-")
	if err != nil {
		// fallback на системную temp
		tmpDir, err = os.MkdirTemp("", "servo-mod-")
		if err != nil {
			app.appendLogToFile(fmt.Sprintf(app.msg("log_update_failed_temp_dir"), err))
			return "", err
		}
	}

	if err := app.extractArchiveTo(archivePath, tmpDir); err != nil {
		app.appendLog(fmt.Sprintf(app.msg("log_extract_failed_v"), err))
		os.RemoveAll(tmpDir)
		return "", err
	}

	// Нормализуем структуру — не критично, если не удастся (бывает архив
	// сортировки или программы).
	if err := app.normalizeArchiveStructure(tmpDir); err != nil {
		app.appendLog(fmt.Sprintf(app.msg("log_failed_normalize"), err))
	}

	return tmpDir, nil
}

// classifyArchive определяет тип архива по его содержимому:
//   - программа (в корне <AppName>.exe),
//   - файлы сортировки (в корне mod_database.json / mandatory_rules),
//   - обычный мод.
//
// Читает только корень tmpDir — вложенные каталоги не считаются.
func classifyArchive(tmpDir string) archiveKind {
	// Архив программы — ищем <AppName>.exe в корне.
	expectedExe := AppName + ".exe"
	if _, err := os.Stat(filepath.Join(tmpDir, expectedExe)); err == nil {
		return archiveKindProgram
	}

	// Архив сортировки — mod_database.json и/или mandatory_rules в корне.
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return archiveKindMod
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == FileNameModDatabase || e.Name() == FileNameMandatoryRules {
			return archiveKindSorting
		}
	}
	return archiveKindMod
}

// installSortingArchive копирует файлы сортировки в modsPath, перезагружает
// базы, обновляет sorter и пересобирает UI. Возвращает ошибку только если
// копирование файла не удалось.
func (app *App) installSortingArchive(tmpDir, modsPath string) error {
	hasModDatabase := fileExists(filepath.Join(tmpDir, FileNameModDatabase))
	hasMandatory := fileExists(filepath.Join(tmpDir, FileNameMandatoryRules))

	if hasModDatabase {
		src := filepath.Join(tmpDir, FileNameModDatabase)
		dst := filepath.Join(modsPath, FileNameModDatabase)
		if err := copyFile(src, dst); err != nil {
			app.appendLog(app.msg("log_failed_to_copy_") + FileNameModDatabase + ": " + err.Error())
			return err
		}
		app.appendLog(FileNameModDatabase + app.msg("log_updated"))
	}
	if hasMandatory {
		src := filepath.Join(tmpDir, FileNameMandatoryRules)
		dst := filepath.Join(modsPath, FileNameMandatoryRules)
		if err := copyFile(src, dst); err != nil {
			app.appendLog(app.msg("log_failed_to_copy_") + FileNameMandatoryRules + ": " + err.Error())
			return err
		}
		app.appendLog(FileNameMandatoryRules + app.msg("log_updated"))
	}

	// Перезагружаем базы.
	if err := app.loadModDatabase(FileNameModDatabase); err == nil {
		app.modsMutex.RLock()
		dbSnapshot := app.modDatabase
		app.modsMutex.RUnlock()
		checks.SetModDatabase(dbSnapshot)
	}
	if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
		app.cfgMutex.Lock()
		app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
		app.cfgMutex.Unlock()
		app.saveConfigSafe()
	}

	// Обновляем sorter. Геттеры возвращают снимки под checksDataMutex —
	// без гонки с параллельным LoadExternalLists.
	sorter.SetMandatoryOrder(checks.GetMandatoryOrder())
	sorter.SetDependencies(convertDeps(checks.GetDependencies()))
	sorter.SetLoadOrderRules(checks.GetLoadOrderRules())

	// Обновляем UI и кэш.
	app.fixHubHotkeyMenus()
	// refreshModList трогает виджеты (modTable/filterSelect/counterLabel) —
	// только на главной горутине.
	fyne.Do(app.refreshModList)
	app.syncVersionCache()
	app.logVersions()
	return nil
}

// installModFiles копирует содержимое tmpDir в целевую папку менеджера.
// Обрабатывает три случая:
//  1. binaries/  → gameRoot/binaries
//  2. mods/...   → modsPath/...
//  3. остальное  → modsPath/<folder> (обычные моды)
//
// Возвращает список имён установленных модов в порядке копирования.
func (app *App) installModFiles(tmpDir, modsPath, gameRoot string) ([]string, error) {
	var installedNames []string

	// Обработка специальных папок binaries и mods.
	if gameRoot != "" {
		if err := app.installBinaries(tmpDir, gameRoot); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_copy_binaries_failed"), err))
		}

		names, err := app.installModsSubfolder(tmpDir, modsPath)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to install mods subfolder: %v", err))
		} else {
			installedNames = append(installedNames, names...)
		}
	}

	// Проходим по корню tmpDir — это уже «распакованные моды».
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		modName := e.Name()
		if modName == "binaries" || modName == "mods" {
			continue // уже обработаны выше
		}

		// Фикс для hub_hotkey_menus-main.
		if modName == "hub_hotkey_menus-main" {
			newName := "hub_hotkey_menus"
			oldPath := filepath.Join(tmpDir, modName)
			newPath := filepath.Join(tmpDir, newName)
			if err := os.Rename(oldPath, newPath); err == nil {
				app.appendLogToFile(fmt.Sprintf(app.msg("log_fix_hub_hk_menus_temp"), modName, newName))
				modName = newName
			} else {
				app.appendLogToFile(fmt.Sprintf(app.msg("log_failed_fix_hub_hkm_temp"), modName, err))
			}
		}

		dest := filepath.Join(modsPath, modName)
		app.appendLog(fmt.Sprintf(app.msg("log_moving_folder"), modName, dest))
		if err := app.copyFolder(filepath.Join(tmpDir, modName), dest); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_failed_copy"), err))
			return nil, err
		}
		installedNames = append(installedNames, modName)

		// Исправляем несоответствие имени папки и .mod файла.
		if newName := checks.TryFixMismatchedModFolder(dest, modName); newName != "" {
			installedNames[len(installedNames)-1] = newName
			modName = newName
		}
	}

	return installedNames, nil
}

// installBinaries копирует <tmpDir>/binaries в <gameRoot>/binaries.
// Если папки нет — тихо возвращает nil.
func (app *App) installBinaries(tmpDir, gameRoot string) error {
	binariesSrc := filepath.Join(tmpDir, "binaries")
	info, err := os.Stat(binariesSrc)
	if err != nil || !info.IsDir() {
		return nil
	}

	binariesDst := filepath.Join(gameRoot, "binaries")
	app.appendLog(fmt.Sprintf(app.msg("log_copy_binaries"), binariesSrc, binariesDst))

	if err := copyPath(binariesSrc, binariesDst); err != nil {
		return err
	}
	app.appendLog(app.msg("log_binaries_installed_success"))
	return nil
}

// installModsSubfolder копирует содержимое <tmpDir>/mods в modsPath и
// удаляет сам каталог mods, чтобы ниже он не попал в общий обход.
// Возвращает имена скопированных подпапок.
func (app *App) installModsSubfolder(tmpDir, modsPath string) ([]string, error) {
	modsSrc := filepath.Join(tmpDir, "mods")
	info, err := os.Stat(modsSrc)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var names []string
	entries, err := os.ReadDir(modsSrc)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		src := filepath.Join(modsSrc, e.Name())
		dst := filepath.Join(modsPath, e.Name())
		app.appendLogToFile(fmt.Sprintf("Copying mod folder: %s -> %s", src, dst))
		if err := copyPath(src, dst); err != nil {
			app.appendLogToFile(fmt.Sprintf("Failed to copy mod %s: %v", e.Name(), err))
			continue
		}
		names = append(names, e.Name())
	}

	os.RemoveAll(modsSrc)
	return names, nil
}

// resolveModIDAndVersion извлекает ID мода и версию из имени архива, а
// если не получилось — из базы или из диалога с пользователем.
func (app *App) resolveModIDAndVersion(archivePath, installedName, knownVersion string) (int, string) {
	modID := 0
	version := knownVersion

	// Сначала — из имени файла (новый приоритет).
	if idFromFile, v, ok := extractVersionAndModIDFromFilename(archivePath); ok && idFromFile != 0 {
		modID = idFromFile
		if v != "" && version == "" {
			version = v
		}
	}

	// Потом — из базы.
	if modID == 0 {
		if entry := checks.GetModDBEntry(installedName); entry != nil && entry.URL != "" {
			modID = helpers.ExtractModIDFromURL(entry.URL)
		}
	}

	// Если версии всё ещё нет — спрашиваем пользователя.
	if version == "" {
		version = app.promptUserForVersion(installedName)
	}
	return modID, version
}

// finalizeModInstallUI обновляет UI после установки: пересобирает список,
// активирует мод (если нужно), синхронизирует профиль с игровой папкой,
// прокручивает к установленному моду.
//
// Вся работа идёт в главном потоке Fyne.
func (app *App) finalizeModInstallUI(installedName string, activate bool, archivePath string) {
	fyne.Do(func() {
		app.appendLogToFile("InstallModFromArchive: entering fyne.Do")
		app.refreshModList()
		app.appendLogToFile("InstallModFromArchive: after refreshModList")

		if activate {
			// #4: пишем Active под modsMutex.
			app.modsMutex.Lock()
			for i := range app.allMods {
				if app.allMods[i].Name == installedName {
					app.allMods[i].Active = true
					break
				}
			}
			app.modsMutex.Unlock()

			app.orderDirty = true
			app.updateTableBorder()
		}

		// ВАЖНО: синхронизация в профиль вызывается ВСЕГДА, независимо
		// от activate. Иначе свежеустановленный (но неактивный) мод
		// останется только в игровой папке: при следующем переключении
		// профиля switchProfile удалит его из <game>/mods/, не сохранив
		// копию в профиль — и мод потеряется безвозвратно.
		//
		// Используем syncModToProfile (один мод) вместо syncProfileFromGame
		// (все моды) — установка редкая, но копировать весь каталог на
		// каждую установку всё равно избыточно.
		app.syncModToProfile(installedName)

		app.filterModList()
		app.forceRefreshTable()

		if activate {
			app.appendLog(fmt.Sprintf(app.msg("log_installed"), archivePath))
		} else {
			app.appendLog(fmt.Sprintf(app.msg("log_installed_inactive"), installedName))
		}

		app.selectAndScrollToMod(installedName)
		app.appendLogToFile("InstallModFromArchive: after selectAndScrollToMod")
	})
}

// cacheInstalledVersion сохраняет версию мода в кеше и, для системных
// модов (base/dmf), синхронизирует кеш с локальными файлами.
func (app *App) cacheInstalledVersion(modID int, installedName, version string) {
	if version != "" && modID != 0 {
		cacheKey := fmt.Sprintf("%d:%s", modID, installedName)
		app.cacheModVersion(cacheKey, installedName, version, 0, "manual", 0)
	}
	if installedName == "base" || installedName == "dmf" {
		app.syncVersionCache()
	}
}

// Обновление одного мода.
// skipConfirm - если true, пропускаем диалог подтверждения загрузки.
func (app *App) updateModFromNexus(mod *checks.ModInfo, skipConfirm bool) {
	if mod.URL == "" || app.getAuthToken() == "" {
		app.appendLog(app.msg("update_skipped_no_url"))
		return
	}
	modID := helpers.ExtractModIDFromURL(mod.URL)
	if modID == 0 {
		app.appendLog(fmt.Sprintf(app.msg("cannot_extract_mod_id"), mod.URL))
		return
	}

	if app.isSymlinkFolder(mod.Name) {
		app.appendLog(fmt.Sprintf(app.msg("log_skipping_update_symlink"), mod.Name))
		return
	}

	modIDStr := strconv.Itoa(modID)
	cacheKey := modIDStr + ":" + mod.Name
	saved, exists := app.getCachedVersion(cacheKey)

	if exists && saved.Source == "manual" && !skipConfirm {
		app.showChoiceDialog(
			app.mainWindow,
			app.msg("warning_title"),
			fmt.Sprintf(app.msg("manual_mod_update_warning"), mod.Name),
			func(choice int) {
				if choice == 0 {
					app.doUpdateModFromNexus(mod, modID, modIDStr, cacheKey, skipConfirm)
				}
			},
			app.msg("btn_continue"),
			app.msg("btn_cancel"),
		)
		return
	}

	if exists && saved.Source == "manual" && skipConfirm {
		app.appendLog(fmt.Sprintf(app.msg("log_forcing_update_manual"), mod.Name))
	}

	app.doUpdateModFromNexus(mod, modID, modIDStr, cacheKey, skipConfirm)
}

// doUpdateModFromNexus - выполняет фактическое обновление (без лишних диалогов).
func (app *App) doUpdateModFromNexus(mod *checks.ModInfo, modID int, modIDStr, cacheKey string, skipConfirm bool) {
	fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
	if err != nil {
		app.logNexusError(err, mod.Name)
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, strconv.Itoa(fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.msg("failed_get_download_link"), err))
		return
	}

	if skipConfirm {
		app.startDownload(directURL, filename, mod.Name, fileInfo, modIDStr)
	} else {
		app.showDownloadDialog(directURL, filename, mod.Name, fileInfo, modIDStr)
	}
}

// Обновление всех модов (только те, у которых есть обновление). Только для Premium-пользователей!
func (app *App) updateAllModsFromNexus() {
	app.appendLog(app.msg("log_starting_batch_update"))
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}

	app.appendLog(app.msg("log_collecting_mods"))
	type modUpdateItem struct {
		ModName  string
		ModID    int
		FileInfo *FileInfo
	}
	var modsToUpdate []modUpdateItem

	bar, label, cancelChan, closeDialog := app.showProgressDialog(
		app.msg("update_title"),
		app.msg("collecting_mods_for_update"),
	)
	defer closeDialog()

	totalMods := 0
	processed := 0
	app.modsMutex.RLock()
	allModsCopy := make([]checks.ModInfo, len(app.allMods))
	copy(allModsCopy, app.allMods)
	app.modsMutex.RUnlock()

	for _, mod := range allModsCopy {
		if mod.URL != "" && !mod.IsSystem {
			totalMods++
		}
	}

	for i := range allModsCopy {
		select {
		case <-cancelChan:
			app.appendLog(app.msg("collecting_mods_cancelled"))
			return
		default:
		}

		mod := &allModsCopy[i]
		if mod.URL == "" || mod.IsSystem {
			continue
		}
		modID := helpers.ExtractModIDFromURL(mod.URL)
		if modID == 0 {
			processed++
			continue
		}
		if app.isSymlinkFolder(mod.Name) {
			app.appendLog(fmt.Sprintf(app.msg("log_skipping_symlink"), mod.Name))
			processed++
			continue
		}

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
			processed++
			continue
		}
		if fileInfo == nil {
			processed++
			continue
		}

		cacheKey := fmt.Sprintf("%d:%s", modID, mod.Name)
		saved, exists := app.getCachedVersion(cacheKey)
		if !exists || saved.Source == "" || saved.Source == "manual" {
			processed++
			continue
		}
		if saved.Timestamp == 0 || fileInfo.UploadedTimestamp > saved.Timestamp {
			modsToUpdate = append(modsToUpdate, modUpdateItem{
				ModName:  mod.Name,
				ModID:    modID,
				FileInfo: fileInfo,
			})
			if saved.Version != "" {
				app.appendLog(fmt.Sprintf(app.msg("log_update_available"), mod.Name, saved.Version, fileInfo.Version))
			} else {
				app.appendLog(fmt.Sprintf(app.msg("log_update_available_x"), mod.Name, fileInfo.Version))
			}
		}
		processed++

		if processed%5 == 0 || processed == totalMods {
			fyne.Do(func() {
				bar.SetValue(float64(processed) / float64(totalMods))
				label.SetText(fmt.Sprintf(app.msg("collecting_mods_x_y"), processed, totalMods))
			})
		}
	}

	if len(modsToUpdate) == 0 {
		app.appendLog(app.msg("no_updates_found"))
		fyne.Do(func() {
			label.SetText(app.msg("updates_found_no"))
			bar.SetValue(1.0)
		})
		time.Sleep(1 * time.Second)
		return
	}

	fyne.Do(func() {
		label.SetText(app.msg("downloading_changelog"))
		bar.SetValue(0)
	})

	var updateChoices []*ModUpdateChoice
	for idx, item := range modsToUpdate {
		select {
		case <-cancelChan:
			app.appendLog(app.msg("collecting_mods_cancelled"))
			return
		default:
		}

		changelog, err := app.FetchChangelog(item.ModID, item.FileInfo.ID)
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("failed_fetch_changlog_for"), item.ModName, err))
			changelog = ""
		}
		modInfo := &checks.ModInfo{
			Name:        item.ModName,
			DisplayName: item.ModName,
			URL:         fmt.Sprintf(NexusModIDLink, item.ModID),
		}
		updateChoices = append(updateChoices, &ModUpdateChoice{
			Mod:       modInfo,
			FileInfo:  item.FileInfo,
			Changelog: changelog,
			Selected:  true,
		})

		fyne.Do(func() {
			bar.SetValue(float64(idx+1) / float64(len(modsToUpdate)))
			label.SetText(fmt.Sprintf(app.msg("downloading_changelog_x_y"), idx+1, len(modsToUpdate)))
		})
	}

	closeDialog()

	resultChan := make(chan struct {
		indices []int
		ok      bool
	}, 1)

	// ВАЖНО: не добавлять fyne.Do внутрь showUpdateChoiceDialog —
	// это приведёт к deadlock из-за вложенности в DoAndWait.
	// Если понадобится — сначала переписать на fyne.Do + select.
	fyne.DoAndWait(func() {
		app.showUpdateChoiceDialog(updateChoices, resultChan)
	})

	res := <-resultChan

	if !res.ok || len(res.indices) == 0 {
		return
	}

	app.modsMutex.RLock()
	currentMods := make([]checks.ModInfo, len(app.allMods))
	copy(currentMods, app.allMods)
	app.modsMutex.RUnlock()

	var selectedMods []*checks.ModInfo
	for _, idx := range res.indices {
		if idx < 0 || idx >= len(modsToUpdate) {
			continue
		}
		item := modsToUpdate[idx]
		for i := range currentMods {
			if currentMods[i].Name == item.ModName {
				selectedMods = append(selectedMods, &currentMods[i])
				break
			}
		}
	}

	if len(selectedMods) == 0 {
		app.appendLog(app.msg("log_no_mods_selected_update"))
		return
	}

	bar2, label2, cancelChan2, closeDialog2 := app.showProgressDialog(
		app.msg("update_title"),
		fmt.Sprintf("0 / %d", len(selectedMods)),
	)
	defer closeDialog2()

	updatedCount := 0
	for _, mod := range selectedMods {
		select {
		case <-cancelChan2:
			app.appendLogToFile("Update cancelled by user")
			return
		default:
		}
		app.appendLog(fmt.Sprintf(app.msg("updating_mod"), mod.Name))
		app.appendLogToFile(fmt.Sprintf("Starting update for %s at %s", mod.Name, time.Now().Format("15:04:05")))
		app.updateModFromNexus(mod, true)
		app.appendLogToFile(fmt.Sprintf("Finished update for %s at %s", mod.Name, time.Now().Format("15:04:05")))
		updatedCount++

		fyne.Do(func() {
			bar2.SetValue(float64(updatedCount) / float64(len(selectedMods)))
			label2.SetText(fmt.Sprintf("%d / %d - %s", updatedCount, len(selectedMods), mod.Name))
		})

		time.Sleep(500 * time.Millisecond)
	}

	fyne.Do(func() {
		bar2.SetValue(1.0)
		label2.SetText(fmt.Sprintf("%d / %d - ✅", updatedCount, len(selectedMods)))
	})

	app.appendLog(fmt.Sprintf(app.msg("update_all_finished"), updatedCount))
	time.Sleep(1 * time.Second)
}

// Удалить выбранные моды
func (app *App) removeSelectedMods() {
	app.modsMutex.RLock()
	var selectedNames []string
	for _, m := range app.allMods {
		if m.Selected && !m.IsSystem {
			selectedNames = append(selectedNames, m.Name)
		}
	}
	firstSelectedName := ""
	for _, m := range app.displayedMods {
		if m.Selected {
			firstSelectedName = m.Name
			break
		}
	}
	app.modsMutex.RUnlock()

	if len(selectedNames) == 0 {
		app.appendLog(app.msg("no_mods_selected"))
		return
	}

	go func() {
		for _, name := range selectedNames {
			checks.RemoveMod(name)
			app.removeModFromData(name)
			app.removeModFromCache(name)
		}
		app.saveCurrentOrder()
		app.syncProfileFromGame()

		fyne.Do(func() {
			app.orderDirty = false
			app.updateTableBorder()
			app.refreshModList()
			app.filterModList()
			app.forceRefreshTable()

			app.modsMutex.RLock()
			displayed := make([]checks.ModInfo, len(app.displayedMods))
			copy(displayed, app.displayedMods)
			app.modsMutex.RUnlock()

			if len(displayed) > 0 {
				found := false
				if firstSelectedName != "" {
					for i, m := range displayed {
						if m.Name == firstSelectedName {
							app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
							app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
							found = true
							break
						}
					}
				}
				if !found {
					app.modTable.Select(widget.TableCellID{Row: 0, Col: 0}, 0)
					app.modTable.ScrollTo(widget.TableCellID{Row: 0, Col: 0})
				}
			} else {
				app.selectedModName = ""
				app.selectedModIndex.Store(-1)
				app.updateDescriptionForMod("")
				app.updateUpDownButtons()
			}

			app.orderDirty = false
			app.updateTableBorder()
			app.appendLog(app.msg("log_selected_mods_removed"))
		})
	}()
}

// Удалить все моды
func (app *App) removeAllMods() {
	app.modsMutex.RLock()
	mods := make([]string, 0, len(app.allMods))
	for _, mod := range app.allMods {
		if !mod.IsSystem {
			mods = append(mods, mod.Name)
		}
	}
	app.modsMutex.RUnlock()

	app.modsMutex.Lock()
	app.allMods = []checks.ModInfo{}
	app.displayedMods = []checks.ModInfo{}
	app.modsMutex.Unlock()

	app.orderDirty = false
	app.selectedModName = ""
	app.selectedModIndex.Store(-1)

	app.updateTableBorder()
	app.updateDescriptionForMod("")
	app.refreshModList()
	app.filterModList()
	app.forceRefreshTable()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				app.appendLogToFile(fmt.Sprintf("PANIC in removeAllMods: %v", r))
			}
		}()

		for _, name := range mods {
			checks.RemoveMod(name)
			app.removeModFromCache(name)
			app.appendLog(fmt.Sprintf(app.msg("log_deleted"), name))
		}

		app.saveCurrentOrder()
		app.syncProfileFromGame()
		app.appendLog(app.msg("log_all_mods_removed"))

		fyne.Do(func() {
			app.refreshModList()
			app.filterModList()
			app.forceRefreshTable()
		})
	}()
}

// Установка DML в корень игры
func (app *App) installDMLFromArchive(archivePath string) error {
	gameRoot, _ := app.getGameState()
	if gameRoot == "" {
		return fmt.Errorf("game root not found")
	}
	tmpDir, err := os.MkdirTemp("", "dml-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	if err := app.extractArchiveTo(archivePath, tmpDir); err != nil {
		return fmt.Errorf("extract failed: %w", err)
	}
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(tmpDir, entry.Name())
		if err := copyPath(srcPath, filepath.Join(gameRoot, entry.Name())); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_failed_to_copy_dml"), entry.Name(), err))
		}
	}
	return nil
}

// Рекурсивное копирование файлов/папок с заменой, с поддержкой символических ссылок
func copyPath(src, dst string) error {
	srcInfo, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relPath, _ := filepath.Rel(src, path)
			targetPath := filepath.Join(dst, relPath)

			if info.Mode()&os.ModeSymlink != 0 {
				linkTarget, err := os.Readlink(path)
				if err != nil {
					return err
				}
				if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
					return err
				}
				if _, err := os.Lstat(targetPath); err == nil {
					if err := os.Remove(targetPath); err != nil {
						return err
					}
				}
				return os.Symlink(linkTarget, targetPath)
			}

			if info.IsDir() {
				return os.MkdirAll(targetPath, 0755)
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return os.WriteFile(targetPath, data, 0644)
		})
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		linkTarget, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if _, err := os.Lstat(dst); err == nil {
			if err := os.Remove(dst); err != nil {
				return err
			}
		}
		return os.Symlink(linkTarget, dst)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func (app *App) installAutopatcherFromArchive(archivePath string) error {
	return app.installDMLFromArchive(archivePath)
}

// isNumeric проверяет, состоит ли строка только из цифр
func isNumeric(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// promptUserForVersion показывает диалог ввода версии и возвращает введённую строку
func (app *App) promptUserForVersion(modName string) string {
	resultChan := make(chan string, 1)
	fyne.Do(func() {
		entry := widget.NewEntry()
		entry.SetPlaceHolder(app.msg("placeholder_mod_version"))

		var popUp *widget.PopUp
		titleLabel := widget.NewLabelWithStyle(app.msg("mod_version"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		msgLabel := widget.NewLabel(fmt.Sprintf(app.msg("failed_get_mod_version"), modName))
		msgLabel.Wrapping = fyne.TextWrapWord

		saveBtn := widget.NewButton(app.msg("btn_save"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- entry.Text
		})
		cancelBtn := widget.NewButton(app.msg("btn_cancel"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- ""
		})

		btnContainer := container.NewCenter(container.NewHBox(saveBtn, cancelBtn))

		content := container.NewVBox(
			titleLabel,
			widget.NewSeparator(),
			msgLabel,
			entry,
			widget.NewSeparator(),
			btnContainer,
		)

		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
		popUp.Resize(fyne.NewSize(400, 200))
		popUp.Show()
	})
	return <-resultChan
}

func (app *App) cacheModVersion(cacheKey, folderName, version string, timestamp int64, source string, installedAt int64) {
	if version == "" {
		return
	}
	if source == "" {
		source = "manual"
	}
	if installedAt == 0 {
		installedAt = time.Now().Unix()
	}
	app.setCachedVersion(cacheKey, ModVersionInfo{
		Timestamp:   timestamp,
		Version:     version,
		Folder:      folderName,
		Source:      source,
		InstalledAt: installedAt,
	})
	app.saveNexusVersionCache()
}

// removeModFromData удаляет мод из внутренних структур и возвращает индекс, на котором он находился в displayedMods
func (app *App) removeModFromData(modName string) (indexInDisplayed int, found bool) {
	app.modsMutex.Lock()
	defer app.modsMutex.Unlock()
	for i, m := range app.allMods {
		if m.Name == modName {
			app.allMods = append(app.allMods[:i], app.allMods[i+1:]...)
			break
		}
	}
	for i, m := range app.displayedMods {
		if m.Name == modName {
			indexInDisplayed = i
			found = true
			app.displayedMods = append(app.displayedMods[:i], app.displayedMods[i+1:]...)
			break
		}
	}
	if app.selectedModName == modName {
		app.selectedModName = ""
		app.selectedModIndex.Store(-1)
	}
	return indexInDisplayed, found
}

func (app *App) updateModCounter() {
	if app.counterLabel == nil {
		return
	}
	// (#4) Снимок под RLock — иначе гонка с filterModList.
	app.modsMutex.RLock()
	displayedLen := len(app.displayedMods)
	allLen := len(app.allMods)
	activeCount := 0
	for _, m := range app.displayedMods {
		if m.Active {
			activeCount++
		}
	}
	app.modsMutex.RUnlock()

	fyne.Do(func() {
		app.counterLabel.SetText(fmt.Sprintf(app.msg("mods_counter"), displayedLen, allLen, activeCount))
	})
}

// normalizeArchiveStructure исправляет типичные ошибки упаковки модов.
func (app *App) normalizeArchiveStructure(tmpDir string) error {
	readEntries := func() ([]os.DirEntry, error) {
		return os.ReadDir(tmpDir)
	}

	entries, err := readEntries()
	if err != nil {
		return err
	}

	// --- Этап 1: убираем внешние обёртки вида "Folder/mods/..." ---
	for {
		if len(entries) != 1 || !entries[0].IsDir() {
			break
		}
		outerDir := entries[0].Name()
		outerPath := filepath.Join(tmpDir, outerDir)
		innerEntries, err := os.ReadDir(outerPath)
		if err != nil {
			break
		}
		var modsPath string
		for _, e := range innerEntries {
			if e.IsDir() && strings.EqualFold(e.Name(), "mods") {
				modsPath = filepath.Join(outerPath, e.Name())
				break
			}
		}
		if modsPath == "" {
			break
		}
		subEntries, err := os.ReadDir(modsPath)
		if err != nil {
			break
		}
		for _, sub := range subEntries {
			src := filepath.Join(modsPath, sub.Name())
			dst := filepath.Join(tmpDir, sub.Name())
			if err := os.Rename(src, dst); err != nil {
				if err := copyPath(src, dst); err != nil {
					return err
				}
				os.RemoveAll(src)
			}
		}
		os.RemoveAll(outerPath)
		entries, err = readEntries()
		if err != nil {
			return err
		}
	}

	// --- Этап 2: если в корне единственная папка "mods" - поднимаем её содержимое ---
	if len(entries) == 1 && entries[0].IsDir() && strings.EqualFold(entries[0].Name(), "mods") {
		modsDir := filepath.Join(tmpDir, entries[0].Name())
		subEntries, err := os.ReadDir(modsDir)
		if err != nil {
			return err
		}
		for _, sub := range subEntries {
			src := filepath.Join(modsDir, sub.Name())
			dst := filepath.Join(tmpDir, sub.Name())
			if err := os.Rename(src, dst); err != nil {
				if err := copyPath(src, dst); err != nil {
					return err
				}
				os.RemoveAll(src)
			}
		}
		os.Remove(modsDir)
		entries, err = readEntries()
		if err != nil {
			return err
		}
	}

	// --- Этап 3: если нет ни одной папки, но есть .mod файл - создаём папку мода ---
	hasFolder := false
	var modFile string
	for _, e := range entries {
		if e.IsDir() {
			hasFolder = true
			break
		}
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			if modFile == "" {
				modFile = e.Name()
			}
		}
	}
	if !hasFolder && modFile != "" {
		modName := strings.TrimSuffix(modFile, ".mod")
		newDir := filepath.Join(tmpDir, modName)
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			if err := os.Mkdir(newDir, 0755); err != nil {
				return err
			}
		}
		for _, e := range entries {
			src := filepath.Join(tmpDir, e.Name())
			if e.Name() == modFile && e.Name() == modName+".mod" {
				continue
			}
			dst := filepath.Join(newDir, e.Name())
			if err := os.Rename(src, dst); err != nil {
				if err := copyPath(src, dst); err != nil {
					return err
				}
				os.RemoveAll(src)
			}
		}
		entries, err = readEntries()
		if err != nil {
			return err
		}
	}

	// --- Этап 4: если есть .mod файл в корне, но нет папки с его именем ---
	var modFileInRoot string
	var modNameFromFile string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			modFileInRoot = e.Name()
			modNameFromFile = strings.TrimSuffix(modFileInRoot, ".mod")
			break
		}
	}
	if modFileInRoot != "" {
		hasModFolder := false
		for _, e := range entries {
			if e.IsDir() && e.Name() == modNameFromFile {
				hasModFolder = true
				break
			}
		}
		if !hasModFolder {
			newDir := filepath.Join(tmpDir, modNameFromFile)
			if _, err := os.Stat(newDir); os.IsNotExist(err) {
				if err := os.Mkdir(newDir, 0755); err != nil {
					return err
				}
			}
			for _, e := range entries {
				src := filepath.Join(tmpDir, e.Name())
				if e.Name() == modFileInRoot {
					dst := filepath.Join(newDir, e.Name())
					if err := os.Rename(src, dst); err != nil {
						if err := copyPath(src, dst); err != nil {
							return err
						}
						os.RemoveAll(src)
					}
				} else if e.IsDir() && e.Name() != modNameFromFile {
					dst := filepath.Join(newDir, e.Name())
					if err := os.Rename(src, dst); err != nil {
						if err := copyPath(src, dst); err != nil {
							return err
						}
						os.RemoveAll(src)
					}
				}
			}
			entries, err = readEntries()
			if err != nil {
				return err
			}
		}
	}

	// --- Этап 5: если есть папка, имя которой совпадает с именем .mod файла, но внутри папки нет .mod файла ---
	var modFileInRoot2 string
	var modNameFromFile2 string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			modFileInRoot2 = e.Name()
			modNameFromFile2 = strings.TrimSuffix(modFileInRoot2, ".mod")
			break
		}
	}
	if modFileInRoot2 != "" {
		var targetFolder string
		for _, e := range entries {
			if e.IsDir() && e.Name() == modNameFromFile2 {
				targetFolder = e.Name()
				break
			}
		}
		if targetFolder != "" {
			folderPath := filepath.Join(tmpDir, targetFolder)
			innerEntries, err := os.ReadDir(folderPath)
			if err == nil {
				hasModInside := false
				for _, inner := range innerEntries {
					if !inner.IsDir() && strings.HasSuffix(strings.ToLower(inner.Name()), ".mod") {
						hasModInside = true
						break
					}
				}
				if !hasModInside {
					src := filepath.Join(tmpDir, modFileInRoot2)
					dst := filepath.Join(folderPath, modFileInRoot2)
					if err := os.Rename(src, dst); err != nil {
						if err := copyPath(src, dst); err != nil {
							return err
						}
						os.RemoveAll(src)
					}
					entries, err = readEntries()
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

// updateSelectedMods принудительно обновляет выбранные моды до последней версии с Nexus.
//
// (#4) Раньше в блоке фильтрации был мусор: `mmod, ok := ...; if !ok || mod.IsSystem`.
// Функция не компилировалась. Переписан чистый цикл:
// findModByName возвращает (ModInfo, bool), из неё берём локальную
// копию mod и берём её адрес — указатели в eligible живут столько,
// сколько живёт функция.
func (app *App) updateSelectedMods() {
	selected := app.selectedMods()
	if len(selected) == 0 {
		app.appendLog(app.msg("no_mods_selected"))
		return
	}

	var eligible []*checks.ModInfo
	for _, name := range selected {
		mod, ok := app.findModByName(name)
		if !ok || mod.IsSystem {
			continue
		}
		if app.isSymlinkFolder(name) {
			app.appendLog(fmt.Sprintf(app.msg("log_skipping_update_symlink"), name))
			continue
		}
		if mod.URL == "" {
			app.appendLog(fmt.Sprintf(app.msg("update_no_url_for"), name))
			continue
		}
		// Копия в куче — чтобы указатель не ссылался на переменную цикла
		// (безопасно и в Go 1.22+, но так надёжнее читается).
		m := mod
		eligible = append(eligible, &m)
	}

	if len(eligible) == 0 {
		app.appendLog(app.msg("no_eligible_mods_for_update"))
		return
	}

	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}

	app.appendLog(app.msg("log_collecting_mods_for_force_update"))
	type modUpdateItem struct {
		Mod      *checks.ModInfo
		ModID    int
		FileInfo *FileInfo
	}
	var modsToUpdate []modUpdateItem

	bar, label, cancelChan, closeDialog := app.showProgressDialog(
		app.msg("update_title"),
		app.msg("collecting_mods_for_update"),
	)
	defer closeDialog()

	total := len(eligible)
	processed := 0

	for _, mod := range eligible {
		select {
		case <-cancelChan:
			app.appendLog(app.msg("collecting_mods_cancelled"))
			return
		default:
		}

		modID := helpers.ExtractModIDFromURL(mod.URL)
		if modID == 0 {
			processed++
			continue
		}

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
			processed++
			continue
		}
		if fileInfo == nil {
			processed++
			continue
		}

		modsToUpdate = append(modsToUpdate, modUpdateItem{
			Mod:      mod,
			ModID:    modID,
			FileInfo: fileInfo,
		})
		processed++

		if processed%5 == 0 || processed == total {
			fyne.Do(func() {
				bar.SetValue(float64(processed) / float64(total))
				label.SetText(fmt.Sprintf(app.msg("collecting_mods_x_y"), processed, total))
			})
		}
	}

	if len(modsToUpdate) == 0 {
		app.appendLog(app.msg("no_mods_found_for_force_update"))
		fyne.Do(func() {
			label.SetText(app.msg("updates_found_no"))
			bar.SetValue(1.0)
		})
		time.Sleep(1 * time.Second)
		return
	}

	fyne.Do(func() {
		label.SetText(app.msg("downloading_changelog"))
		bar.SetValue(0)
	})

	var updateChoices []*ModUpdateChoice
	for idx, item := range modsToUpdate {
		select {
		case <-cancelChan:
			app.appendLog(app.msg("collecting_mods_cancelled"))
			return
		default:
		}

		changelog, err := app.FetchChangelog(item.ModID, item.FileInfo.ID)
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("failed_fetch_changlog_for"), item.Mod.Name, err))
			changelog = ""
		}
		updateChoices = append(updateChoices, &ModUpdateChoice{
			Mod:       item.Mod,
			FileInfo:  item.FileInfo,
			Changelog: changelog,
			Selected:  true,
		})

		fyne.Do(func() {
			bar.SetValue(float64(idx+1) / float64(len(modsToUpdate)))
			label.SetText(fmt.Sprintf(app.msg("downloading_changelog_x_y"), idx+1, len(modsToUpdate)))
		})
	}

	closeDialog()

	resultChan := make(chan struct {
		indices []int
		ok      bool
	}, 1)

	// ВАЖНО: не добавлять fyne.Do внутрь showUpdateChoiceDialog —
	// это приведёт к deadlock из-за вложенности в DoAndWait.
	// Если понадобится — сначала переписать на fyne.Do + select.
	fyne.DoAndWait(func() {
		app.showUpdateChoiceDialog(updateChoices, resultChan)
	})

	res := <-resultChan
	if !res.ok || len(res.indices) == 0 {
		app.appendLog(app.msg("update_cancelled"))
		return
	}

	app.modsMutex.RLock()
	currentMods := make([]checks.ModInfo, len(app.allMods))
	copy(currentMods, app.allMods)
	app.modsMutex.RUnlock()

	var selectedMods []*checks.ModInfo
	for _, idx := range res.indices {
		if idx < 0 || idx >= len(modsToUpdate) {
			continue
		}
		item := modsToUpdate[idx]
		for i := range currentMods {
			if currentMods[i].Name == item.Mod.Name {
				selectedMods = append(selectedMods, &currentMods[i])
				break
			}
		}
	}

	if len(selectedMods) == 0 {
		app.appendLog(app.msg("log_no_mods_selected_update"))
		return
	}

	bar2, label2, cancelChan2, closeDialog2 := app.showProgressDialog(
		app.msg("update_title"),
		fmt.Sprintf("0 / %d", len(selectedMods)),
	)
	defer closeDialog2()

	updatedCount := 0
	for _, mod := range selectedMods {
		select {
		case <-cancelChan2:
			app.appendLog(app.msg("log_update_cancelled"))
			return
		default:
		}

		app.appendLog(fmt.Sprintf(app.msg("updating_mod"), mod.Name))
		app.updateModFromNexus(mod, true)
		updatedCount++

		fyne.Do(func() {
			bar2.SetValue(float64(updatedCount) / float64(len(selectedMods)))
			label2.SetText(fmt.Sprintf("%d / %d - %s", updatedCount, len(selectedMods), mod.Name))
		})

		time.Sleep(500 * time.Millisecond)
	}

	fyne.Do(func() {
		bar2.SetValue(1.0)
		label2.SetText(fmt.Sprintf("%d / %d - ✅", updatedCount, len(selectedMods)))
	})

	app.appendLog(fmt.Sprintf(app.msg("update_selected_forced_finished"), updatedCount))
	time.Sleep(1 * time.Second)
}
