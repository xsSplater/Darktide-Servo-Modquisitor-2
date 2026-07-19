// mod_operations.go
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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/mholt/archives"
)

func safeJoin(destDir, name string) (string, error) {
	name = strings.TrimLeft(name, "/\\")
	if filepath.VolumeName(name) != "" {
		return "", fmt.Errorf("absolute path not allowed")
	}
	targetPath := filepath.Clean(filepath.Join(destDir, name))
	rel, err := filepath.Rel(destDir, targetPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("path traversal detected")
	}
	return targetPath, nil
}

func (app *App) refreshModList() {
	// Проверяем, что UI-виджеты созданы
	if app.modTable == nil || app.headerTable == nil || app.systemModsTable == nil {
		return
	}
	if app.mainWindow == nil || app.mainWindow.Canvas() == nil {
		return
	}

	mods := checks.GetModsInfo(app.cfg.Language, app.cfg.ForceEnglishModNames)

	var sysMods, regMods []checks.ModInfo
	for _, m := range mods {
		if m.IsSystem {
			m.Active = false
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}
	app.systemMods = sysMods

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
	for i := range regMods {
		regMods[i].Obsolete = helpers.ContainsString(checks.ObsoleteMods, regMods[i].Name)
		regMods[i].Mandatory = checks.IsMandatoryMod(regMods[i].Name)
		regMods[i].Incompatible = app.checkIncompatible(regMods[i].Name)

		// Описание несовместимости в таблице в Примечании
		if regMods[i].Incompatible {
			for _, pair := range checks.IncompatiblePairs {
				if pair.Mod1 == regMods[i].Name || pair.Mod2 == regMods[i].Name {
					other := pair.Mod1
					if other == regMods[i].Name {
						other = pair.Mod2
					}
					if checks.FolderExists(other) {
						lang := app.cfg.Language
						if app.cfg.ForceEnglishModNames {
							lang = "en"
						}
						displayName := other
						if entry := checks.GetModDBEntry(other); entry != nil {
							if name := checks.PickLocalized(entry.Name, lang); name != "" {
								displayName = name
							}
						}
						regMods[i].Note = strings.TrimSpace(regMods[i].Note + app.messages["conflict_with"] + displayName)
						break
					}
				}
			}
		}

		// Проверка на симлинк для обычных модов
		modPath := filepath.Join(app.cfg.ModsPath, regMods[i].Name)
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
		modPath := filepath.Join(app.cfg.ModsPath, sysMods[i].Name)
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
	wasAML := app.amlDetected
	app.amlDetected = checks.IsAMLInstalled(app.cfg.ModsPath)
	if wasAML != app.amlDetected {
		if app.amlDetected {
			app.btnSaveOrder.SetText(app.messages["btn_save_order_aml"])
			app.btnSortChecks.SetText(app.messages["btn_sort_checks_aml"])
			app.applyTooltip(app.btnSaveOrder, "aml_save_warning_tooltip")
			app.applyTooltip(app.btnSortChecks, "aml_sort_warning_tooltip")
		} else {
			app.btnSaveOrder.SetText(app.messages["btn_save_order"])
			app.btnSortChecks.SetText(app.messages["btn_sort_checks"])
			app.applyTooltip(app.btnSaveOrder, "btn_save_order_tooltip")
			app.applyTooltip(app.btnSortChecks, "btn_sort_checks_tooltip")
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

	app.allMods = regMods
	app.orderDirty = false

	app.filterModList()

	app.updateSystemModsTable()

	app.forceRefreshTable()

}

func (app *App) updateSystemModsTable() {
	if app.systemModsTable == nil {
		app.appendLog("updateSystemModsTable: systemModsTable is nil, skipping")
		return
	}
	app.systemModsTable.Length = func() (int, int) { return len(app.systemMods), TableColumnCount }
	app.systemModsTable.Refresh()
}

func (app *App) saveCurrentOrder() {
	entries := app.buildLoadOrderEntries()
	checks.WriteLoadOrder(entries)
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
	// UI-обновления - вне мьютекса, в главном потоке
	fyne.Do(func() {
		app.updateTableBorder()
		app.filterModList()
		app.forceRefreshTable()
	})
}

func (app *App) findModByName(name string) *checks.ModInfo {
	app.modsMutex.RLock()
	defer app.modsMutex.RUnlock()
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			return &app.allMods[i]
		}
	}
	for i := range app.systemMods {
		if app.systemMods[i].Name == name {
			return &app.systemMods[i]
		}
	}
	return nil
}

func (app *App) removeFromAllMods(name string) {
	for i, m := range app.allMods {
		if m.Name == name {
			app.allMods = append(app.allMods[:i], app.allMods[i+1:]...)
			break
		}
	}
}

func (app *App) toggleGlobalMods() {
	switch app.patcherType {
	case PatcherAutoPatch:
		err := toggleModsAutoPatch(app.gameRoot) // передаём gameRoot
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_toggle_fail"], err))
		} else {
			app.cfg.ModsGloballyEnabled = isModsEnabledAutoPatch(app.gameRoot) // передаём gameRoot
			state := app.messages["log_mods_enabled"]
			if !app.cfg.ModsGloballyEnabled {
				state = app.messages["log_mods_disabled"]
			}
			app.appendLog(state + app.messages["log_autopatcher"])
		}
	case PatcherLegacy:
		err := toggleModsLegacy(app.gameRoot) // передаём gameRoot
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_toggle_fail"], err))
		} else {
			app.cfg.ModsGloballyEnabled = !app.cfg.ModsGloballyEnabled
			state := app.messages["log_mods_enabled"]
			if !app.cfg.ModsGloballyEnabled {
				state = app.messages["log_mods_disabled"]
			}
			app.appendLog(state + app.messages["log_autopatcher_old"])
		}
	default:
		app.appendLog(app.messages["log_no_patcher"])
	}
	app.updateToggleButtonText(app.btnToggle)
	saveConfig(app.cfg)
}

func (app *App) performFirstRunSetup() {
	app.cfg.InitialSetupDone = true
	saveConfig(app.cfg)
}

func (app *App) handleDrop(uris []fyne.URI) {
	for _, uri := range uris {
		path := uri.Path()
		info, err := os.Stat(path)
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_error_drop"], err))
			continue
		}
		if info.IsDir() {
			app.copyFolder(path, filepath.Join(app.cfg.ModsPath, filepath.Base(path)))
			checks.AutoFixMalformed()
			app.refreshModList()
			app.orderDirty = true
			app.updateTableBorder()
			app.appendLog(fmt.Sprintf(app.messages["log_installed_folder"], filepath.Base(path)))
		} else {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".zip" || ext == ".rar" || ext == ".7z" {
				go func(p string) {
					installedName, _, err := app.InstallModFromArchive(p, true, "", "")
					fyne.Do(func() {
						if err != nil {
							app.appendLog(fmt.Sprintf(app.messages["log_extract_error"], err))
							return
						}
						checks.AutoFixMalformed()
						app.fixHubHotkeyMenus()
						app.refreshModList()
						if installedName != "" {
							app.selectAndScrollToMod(installedName)
							// Попробуем извлечь modID из имени файла для автодобавления в базу
							modID, _, _ := extractVersionAndModIDFromFilename(p)
							if modID != 0 {
								go app.autoAddModToDatabase(modID, installedName, filepath.Base(p))
							}
							app.orderDirty = true
							app.updateTableBorder()
							app.appendLog(fmt.Sprintf(app.messages["log_installed"], filepath.Base(p)))
						} else {
							// Это был архив с сортировочными файлами
							app.appendLog(app.messages["log_sorting_files_updated_manual"])
						}
					})
				}(path)
			} else {
				app.appendLog(app.messages["log_zip_only"])
			}
		}
	}
}

func (app *App) extractArchive(archivePath string) error {
	return app.extractArchiveTo(archivePath, app.cfg.ModsPath)
}

// extractArchiveTo распаковывает ZIP, RAR, 7z и другие архивы во временную папку,
// затем копирует содержимое в destDir через copyPath.
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

	format, _, err := archives.Identify(context.Background(), archivePath, f)
	if err != nil {
		return fmt.Errorf("unsupported archive format: %w", err)
	}
	extractor, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format does not support extraction")
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "servo-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	var totalExtracted int64
	var fileCount int
	err = extractor.Extract(context.Background(), f, func(ctx context.Context, fi archives.FileInfo) error {
		if fi.Size() > MaxFileSize {
			return fmt.Errorf("file %s size %d exceeds limit %d", fi.NameInArchive, fi.Size(), MaxFileSize)
		}
		totalExtracted += fi.Size()
		if totalExtracted > MaxExtractedSize {
			return fmt.Errorf("total extracted size %d exceeds limit %d", totalExtracted, MaxExtractedSize)
		}
		targetPath, err := safeJoin(tmpDir, fi.NameInArchive)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
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
		_, err = io.Copy(out, rc)
		if err != nil {
		}
		fileCount++
		if fileCount%10 == 0 {
		}
		return err
	})
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
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
	switch app.patcherType {
	case PatcherAutoPatch:
		app.cfg.ModsGloballyEnabled = isModsEnabledAutoPatch(app.gameRoot)
	case PatcherLegacy:
	}
	saveConfig(app.cfg)
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

	if len(selNames) == 1 {
		idx := app.findModIndexByName(selNames[0])
		if idx == -1 {
			return
		}
		newIdx := idx + delta
		if newIdx < 0 || newIdx >= len(app.allMods) {
			return
		}
		app.allMods[idx], app.allMods[newIdx] = app.allMods[newIdx], app.allMods[idx]
		app.orderDirty = true
		app.updateTableBorder()
		app.filterModList()
		app.forceRefreshTable()
		return
	}

	var selected []checks.ModInfo
	for _, m := range app.allMods {
		if m.Selected {
			selected = append(selected, m)
		}
	}

	var others []checks.ModInfo
	for _, m := range app.allMods {
		if !m.Selected {
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
	app.updateTableBorder()
	app.filterModList()
}

func (app *App) moveSelectedToTop() {
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}
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
	app.updateTableBorder()
	app.filterModList()
}

func (app *App) moveSelectedToBottom() {
	selNames := app.selectedMods()
	if len(selNames) == 0 {
		return
	}
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
	app.updateTableBorder()
	app.filterModList()
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
		app.appendLog(app.messages["log_invalid_position"])
		return
	}
	targetIdx := visiblePos - 1

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
	app.updateTableBorder()
	app.filterModList()
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
	for i, m := range app.displayedMods {
		if m.Name == name {
			app.modTable.Select(widget.TableCellID{Row: i, Col: 0})
			break
		}
	}
}

func (app *App) InstallModFromArchive(archivePath string, activate bool, knownVersion string, modNameToUpdate string) (string, string, error) {
	// Если это обновление - удаляем старую папку
	if modNameToUpdate != "" {
		checks.RemoveMod(modNameToUpdate)
		app.appendLog(fmt.Sprintf("Removed old folder for update: %s", modNameToUpdate))
	}

	tmpDir, err := os.MkdirTemp("", "servo-mod-")
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_update_failed_temp_dir"], err))
		return "", "", err
	}
	defer os.RemoveAll(tmpDir)

	if err := app.extractArchiveTo(archivePath, tmpDir); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_extract_failed_v"], err))
		return "", "", err
	}

	// Нормализуем структуру архива
	if err := app.normalizeArchiveStructure(tmpDir); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_failed_normalize"], err))
		// Не выходим, т.к. это может быть архив программы или сортировки
	}

	// Проверяем, не является ли архив программой (ищем Servo-Modquisitor-2.exe)
	expectedExe := AppName + ".exe"
	if _, err := os.Stat(filepath.Join(tmpDir, expectedExe)); err == nil {
		app.appendLog("Program archive detected. Automatic update is not supported. Please install manually.")
		return "", "", fmt.Errorf("program update not supported")
	}

	// Проверяем, не является ли архив сортировочным (mod_database.json и/или mandatory_obsolete_incompatible_dependencies.json)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", "", err
	}
	var hasModDatabase, hasMandatory bool
	for _, e := range entries {
		if !e.IsDir() && e.Name() == FileNameModDatabase {
			hasModDatabase = true
		}
		if !e.IsDir() && e.Name() == FileNameMandatoryRules {
			hasMandatory = true
		}
	}
	if hasModDatabase || hasMandatory {
		// Это архив сортировки, обновляем файлы
		if hasModDatabase {
			src := filepath.Join(tmpDir, FileNameModDatabase)
			dst := filepath.Join(app.cfg.ModsPath, FileNameModDatabase)
			if err := copyFile(src, dst); err != nil {
				app.appendLog(app.messages["log_failed_to_copy_"] + FileNameModDatabase + ": " + err.Error())
				return "", "", err
			}
			app.appendLog(FileNameModDatabase + app.messages["log_updated"])
		}
		if hasMandatory {
			src := filepath.Join(tmpDir, FileNameMandatoryRules)
			dst := filepath.Join(app.cfg.ModsPath, FileNameMandatoryRules)
			if err := copyFile(src, dst); err != nil {
				app.appendLog(app.messages["log_failed_to_copy_"] + FileNameMandatoryRules + ": " + err.Error())
				return "", "", err
			}
			app.appendLog(FileNameMandatoryRules + " updated.")
		}
		// Перезагружаем базы
		if err := app.loadModDatabase(FileNameModDatabase); err == nil {
			checks.SetModDatabase(app.modDatabase)
		}
		if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
			app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
			saveConfig(app.cfg)
		}
		// Обновляем сортировщик
		sorter.SetMandatoryOrder(checks.MandatoryOrder)
		sorter.SetDependencies(convertDeps(checks.Dependencies))
		var sorterRules []checks.LoadOrderRule
		for _, r := range checks.LoadOrderRules {
			sorterRules = append(sorterRules, checks.LoadOrderRule{Before: r.Before, After: r.After})
		}
		sorter.SetLoadOrderRules(sorterRules)
		app.fixHubHotkeyMenus()
		// Обновляем UI
		app.refreshModList()
		// Синхронизируем кэш версий с обновлёнными локальными файлами
		app.syncVersionCache()
		app.logVersions()
		return "", "", nil
	}

	// Объявляем переменную для хранения имён установленных модов
	var installedNames []string

	// Обработка специальных папок binaries и mods
	if app.gameRoot != "" {
		// Копируем binaries в корень игры
		binariesSrc := filepath.Join(tmpDir, "binaries")
		if info, err := os.Stat(binariesSrc); err == nil && info.IsDir() {
			binariesDst := filepath.Join(app.gameRoot, "binaries")
			app.appendLog(fmt.Sprintf("Copying binaries to game root: %s -> %s", binariesSrc, binariesDst))
			if err := copyPath(binariesSrc, binariesDst); err != nil {
				app.appendLog(fmt.Sprintf("Failed to copy binaries: %v", err))
			} else {
				app.appendLog(app.messages["log_binaries_installed_success"])
			}
		}

		// Копируем содержимое mods в папку mods программы
		modsSrc := filepath.Join(tmpDir, "mods")
		if info, err := os.Stat(modsSrc); err == nil && info.IsDir() {
			entries, err := os.ReadDir(modsSrc)
			if err == nil {
				for _, e := range entries {
					if e.IsDir() {
						src := filepath.Join(modsSrc, e.Name())
						dst := filepath.Join(app.cfg.ModsPath, e.Name())
						app.appendLog(fmt.Sprintf("Copying mod folder: %s -> %s", src, dst))
						if err := copyPath(src, dst); err != nil {
							app.appendLog(fmt.Sprintf("Failed to copy mod %s: %v", e.Name(), err))
						} else {
							installedNames = append(installedNames, e.Name())
						}
					}
				}
			}
			// Удаляем папку mods, чтобы она не была скопирована как обычный мод ниже
			os.RemoveAll(modsSrc)
		}
	}

	// Получаем список оставшихся папок (обычные моды) — ПРИСВАИВАНИЕ, без объявления
	entries, err = os.ReadDir(tmpDir)
	if err != nil {
		return "", "", err
	}

	// Копируем все папки (моды) из временной папки
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		modName := e.Name()
		// Пропускаем уже обработанные папки
		if modName == "binaries" || modName == "mods" {
			continue
		}
		// Фикс для hub_hotkey_menus-main
		if modName == "hub_hotkey_menus-main" {
			newName := "hub_hotkey_menus"
			oldPath := filepath.Join(tmpDir, modName)
			newPath := filepath.Join(tmpDir, newName)
			if err := os.Rename(oldPath, newPath); err == nil {
				app.appendLog(fmt.Sprintf(app.messages["log_fix_hub_hk_menus_temp"], modName, newName))
				modName = newName
			} else {
				app.appendLog(fmt.Sprintf(app.messages["log_failed_fix_hub_hkm_temp"], modName, err))
			}
		}
		dest := filepath.Join(app.cfg.ModsPath, modName)
		app.appendLog(fmt.Sprintf(app.messages["log_moving_folder"], modName, dest))
		if err := app.copyFolder(filepath.Join(tmpDir, modName), dest); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_failed_copy"], err))
			return "", "", err
		}
		installedNames = append(installedNames, modName)
		// Исправляем несоответствие имени папки и .mod файла
		if newName := checks.TryFixMismatchedModFolder(dest, modName); newName != "" {
			// Если переименовали, обновляем имя в списке установленных
			installedNames[len(installedNames)-1] = newName
			// Также обновляем modName для дальнейшего использования, если нужно
			modName = newName
		}
	}

	if len(installedNames) == 0 {
		return "", "", errors.New(app.messages["log_no_mod_folder_found"])
	}

	installedName := installedNames[0] // для обратной совместимости возвращаем первый

	// Сначала пробуем извлечь ID и версию из имени файла (новый приоритет)
	modID := 0
	version := knownVersion
	if idFromFile, v, ok := extractVersionAndModIDFromFilename(archivePath); ok && idFromFile != 0 {
		modID = idFromFile
		if v != "" && version == "" {
			version = v
		}
	}

	// Если не удалось извлечь ID из имени, берём из базы
	if modID == 0 {
		if entry := checks.GetModDBEntry(installedName); entry != nil && entry.URL != "" {
			modID = helpers.ExtractModIDFromURL(entry.URL)
		}
	}

	// Если версия всё ещё пуста, спрашиваем пользователя
	if version == "" {
		version = app.promptUserForVersion(installedName)
	}

	// Обновляем UI
	fyne.Do(func() {
		app.refreshModList()
		if activate {
			// Активируем все установленные моды? По умолчанию включаем только первый
			for i := range app.allMods {
				if app.allMods[i].Name == installedName {
					app.allMods[i].Active = true
					break
				}
			}
			app.orderDirty = true
			app.updateTableBorder()
			app.filterModList()
			app.appendLog(fmt.Sprintf(app.messages["log_installed"], archivePath))
		} else {
			app.appendLog(fmt.Sprintf(app.messages["log_installed_inactive"], installedName))
		}
		app.selectAndScrollToMod(installedName)
	})

	// Кэшируем версию для первого мода, если есть modID и версия
	if version != "" && modID != 0 {
		cacheKey := fmt.Sprintf("%d:%s", modID, installedName)
		app.cacheModVersion(cacheKey, installedName, version, 0, "manual")
	}

	// Синхронизируем кэш версий с локальными файлами (особенно для правил)
	if installedName == "base" || installedName == "dmf" {
		app.syncVersionCache()
	}

	return installedName, version, nil
}

// Обновление одного мода.
// skipConfirm - если true, пропускаем диалог подтверждения загрузки.
func (app *App) updateModFromNexus(mod *checks.ModInfo, skipConfirm bool) {
	if mod.URL == "" || app.getAuthToken() == "" {
		app.appendLog(app.messages["update_skipped_no_url"])
		return
	}
	modID := helpers.ExtractModIDFromURL(mod.URL)
	if modID == 0 {
		app.appendLog(fmt.Sprintf(app.messages["cannot_extract_mod_id"], mod.URL))
		return
	}

	if app.isSymlinkFolder(mod.Name) {
		app.appendLog(fmt.Sprintf(app.messages["log_skipping_update_symlink"], mod.Name))
		app.tooltipStatus.Show(fmt.Sprintf(app.messages["log_skipping_update_symlink_skipped"], mod.Name))
		app.tooltipStatus.HideAfterDelay()
		return
	}

	modIDStr := strconv.Itoa(modID)
	cacheKey := modIDStr + ":" + mod.Name
	saved, exists := app.getCachedVersion(cacheKey)

	if exists && saved.Source == "manual" {
		// Этот диалог нужен всегда - он предупреждает, что мод ручной.
		app.showChoiceDialog(
			app.mainWindow,
			app.messages["warning_title"],
			fmt.Sprintf(app.messages["manual_mod_update_warning"], mod.Name),
			func(choice int) {
				if choice == 0 {
					app.doUpdateModFromNexus(mod, modID, modIDStr, cacheKey, skipConfirm)
				}
			},
			app.messages["btn_continue"],
			app.messages["btn_cancel"],
		)
		return
	}
	// Если не ручной, сразу запускаем обновление
	app.doUpdateModFromNexus(mod, modID, modIDStr, cacheKey, skipConfirm)
}

// doUpdateModFromNexus - выполняет фактическое обновление (без лишних диалогов).
// skipConfirm - если true, пропускаем диалог подтверждения загрузки.
func (app *App) doUpdateModFromNexus(mod *checks.ModInfo, modID int, modIDStr, cacheKey string, skipConfirm bool) {
	fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
	if err != nil {
		app.logNexusError(err, mod.Name)
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, strconv.Itoa(fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}

	if skipConfirm {
		// Пропускаем диалог подтверждения, сразу начинаем скачивание.
		app.startDownload(directURL, filename, mod.Name, fileInfo, modIDStr)
	} else {
		app.showDownloadDialog(directURL, filename, mod.Name, fileInfo, modIDStr)
	}
}

// Обновление всех модов (только те, у которых есть обновление). Только для Premium-пользователей!
func (app *App) updateAllModsFromNexus() {
	if app.allMods == nil {
		app.appendLog("No mods loaded, cannot update.")
		return
	}
	app.appendLog(app.messages["log_starting_batch_update"])
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}

	app.appendLog(app.messages["log_collecting_mods"])
	var modsToUpdate []*checks.ModInfo
	totalMods := 0
	processed := 0

	// Считаем общее количество модов, которые будем проверять (исключаем системные и без URL)
	for _, mod := range app.allMods {
		if mod.URL != "" && !mod.IsSystem {
			totalMods++
		}
	}

	for i := range app.allMods {
		mod := &app.allMods[i]
		if mod.URL == "" || mod.IsSystem {
			continue
		}

		modID := helpers.ExtractModIDFromURL(mod.URL)
		if modID == 0 {
			processed++
			continue
		}
		if app.isSymlinkFolder(mod.Name) {
			app.appendLog(fmt.Sprintf("Skipping %s: folder is a symlink", mod.Name))
			processed++
			continue
		}

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
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
			modsToUpdate = append(modsToUpdate, mod)
		}
		processed++
		if processed%10 == 0 {
			app.appendLog(fmt.Sprintf("Progress: %d of %d mods checked", processed, totalMods))
		}
	}

	if len(modsToUpdate) == 0 {
		app.appendLog(app.messages["no_updates_found"])
		return
	}

	// Диалог подтверждения
	choice := app.showChoiceDialogSync(
		app.mainWindow,
		app.messages["update_title"],
		fmt.Sprintf(app.messages["updates_found_count_update"], len(modsToUpdate)),
		app.messages["btn_yes"],
		app.messages["btn_cancel"],
	)
	if choice != 0 {
		app.appendLog("Batch update cancelled by user.")
		return
	}

	// Обновляем
	updatedCount := 0
	for _, mod := range modsToUpdate {
		app.appendLog(fmt.Sprintf(app.messages["updating_mod"], mod.Name))
		app.updateModFromNexus(mod, true) // skipConfirm = true
		updatedCount++
		time.Sleep(500 * time.Millisecond)
		app.appendLog(fmt.Sprintf("Progress: %d of %d mods updated", updatedCount, len(modsToUpdate)))
	}

	app.appendLog(fmt.Sprintf(app.messages["update_all_finished"], updatedCount))

	// Финальное обновление UI
	fyne.Do(func() {
		app.refreshModList()
	})
}

// Удалить выбранные моды
func (app *App) removeSelectedMods() {
	var selectedNames []string
	for _, m := range app.allMods {
		if m.Selected && !m.IsSystem {
			selectedNames = append(selectedNames, m.Name)
		}
	}
	if len(selectedNames) == 0 {
		app.appendLog(app.messages["no_mods_selected"])
		return
	}

	// Запоминаем первый выбранный мод (по displayedMods)
	var firstSelectedName string
	for _, m := range app.displayedMods {
		if m.Selected {
			firstSelectedName = m.Name
			break
		}
	}

	// Удаляем каждый мод
	for _, name := range selectedNames {
		checks.RemoveMod(name)
		app.removeModFromData(name)
	}

	// Обновляем таблицу и счётчик
	app.updateModCounter()
	app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
	app.modTable.Refresh()
	app.orderDirty = true
	app.updateTableBorder()
	app.appendLog(app.messages["log_selected_mods_removed"])

	// Восстанавливаем выделение
	if len(app.displayedMods) > 0 {
		found := false
		if firstSelectedName != "" {
			for i, m := range app.displayedMods {
				if m.Name == firstSelectedName {
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0})
					app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
					found = true
					break
				}
			}
		}
		if !found {
			app.modTable.Select(widget.TableCellID{Row: 0, Col: 0})
			app.modTable.ScrollTo(widget.TableCellID{Row: 0, Col: 0})
		}
	} else {
		app.selectedModName = ""
		app.selectedModIndex.Store(-1)
		app.updateDescriptionForMod("")
		app.updateUpDownButtons()
	}
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
	for _, name := range mods {
		checks.RemoveMod(name)
		app.appendLog(fmt.Sprintf(app.messages["log_deleted"], name))
	}
	fyne.Do(func() {
		app.refreshModList()
		app.orderDirty = true
		app.updateTableBorder()
		app.appendLog(app.messages["log_all_mods_removed"])
	})
}

// Установка DML в корень игры
func (app *App) installDMLFromArchive(archivePath string) error {
	if app.gameRoot == "" {
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
		if err := copyPath(srcPath, filepath.Join(app.gameRoot, entry.Name())); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_failed_to_copy_dml"], entry.Name(), err))
		}
	}
	return nil
}

// Рекурсивное копирование файлов/папок с заменой
func copyPath(src, dst string) error {
	srcInfo, err := os.Stat(src)
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
			if info.IsDir() {
				return os.MkdirAll(targetPath, 0755)
			}
			// Перед записью файла убедимся, что папка существует
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
	// Одиночный файл - создаём папку для него
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

func (app *App) installAutopatcherFromArchive(archivePath string) error {
	// Автопатчер устанавливается в корень игры, а не в mods
	return app.installDMLFromArchive(archivePath) // у него такая же структура установки
}

// extractVersionAndModIDFromFilename для нового формата:
// ModName ModID Version DateStamp RandomHash.zip
func extractVersionAndModIDFromFilename(filename string) (modID int, version string, ok bool) {
	name := filepath.Base(filename)
	// Удаляем расширение
	ext := filepath.Ext(name)
	if ext != "" {
		name = name[:len(name)-len(ext)]
	}

	// Разбиваем по пробелам
	parts := strings.Fields(name)
	if len(parts) < 3 {
		return 0, "", false
	}

	// Ищем часть, которая является числом (ID) и не содержит точек
	var idIdx = -1
	for i, part := range parts {
		if isNumeric(part) {
			id, err := strconv.Atoi(part)
			if err == nil && id > 0 && id < MaxModsID_less {
				modID = id
				idIdx = i
				break
			}
		}
	}
	if idIdx == -1 {
		return 0, "", false
	}

	// Версия — первая часть после ID, которая является числом или числом с точками
	var versionParts []string
	for i := idIdx + 1; i < len(parts); i++ {
		part := parts[i]
		// Проверяем, что это похоже на версию (цифры и точки)
		if isNumeric(strings.ReplaceAll(part, ".", "")) {
			versionParts = append(versionParts, part)
		} else {
			// если встретили нечисловой элемент — это дата или хэш, останавливаемся
			break
		}
	}
	if len(versionParts) > 0 {
		version = strings.Join(versionParts, ".")
		// Убираем лишние точки в конце
		version = strings.Trim(version, ".")
		if version != "" {
			ok = true
		}
	} else {
		// Если версия не найдена, пробуем взять следующую часть после ID (если она есть)
		if idIdx+1 < len(parts) {
			candidate := parts[idIdx+1]
			if isNumeric(strings.ReplaceAll(candidate, ".", "")) {
				version = candidate
				ok = true
			}
		}
	}
	if !ok {
		version = "unknown"
		ok = true // возвращаем unknown, чтобы не потерять ID
	}
	return modID, version, ok
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
		entry.SetPlaceHolder(app.messages["placeholder_mod_version"])

		var popUp *widget.PopUp
		titleLabel := widget.NewLabelWithStyle(app.messages["mod_version"], fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		msgLabel := widget.NewLabel(fmt.Sprintf(app.messages["failed_get_mod_version"], modName))
		msgLabel.Wrapping = fyne.TextWrapWord

		saveBtn := widget.NewButton(app.messages["btn_save"], func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- entry.Text
		})
		cancelBtn := widget.NewButton(app.messages["btn_cancel"], func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- ""
		})

		// Центрируем кнопки
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

func (app *App) cacheModVersion(cacheKey, folderName, version string, timestamp int64, source string) {
	if version == "" {
		return
	}
	if source == "" {
		source = "manual"
	}
	app.setCachedVersion(cacheKey, ModVersionInfo{
		Timestamp: timestamp,
		Version:   version,
		Folder:    folderName,
		Source:    source,
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
	activeCount := 0
	for _, m := range app.displayedMods {
		if m.Active {
			activeCount++
		}
	}
	app.counterLabel.SetText(fmt.Sprintf(app.messages["mods_counter"], len(app.displayedMods), len(app.allMods), activeCount))
}

// normalizeArchiveStructure исправляет типичные ошибки упаковки модов:
// 1. Автор пошёл во все тяжкие и закинул даже папку mods в другую папку Folder/mods/ModName/, что даёт папку Folder в mods. Убираем всё до ModName/.
// 2. Лишняя папка mods в архиве - mods/ModName, что даёт mods/mods/ModName в итоге. Поднимаем содержимое на уровень выше.
// 3. Отсутствие корневой папки мода ModName/, что даёт папку "scripts" вместе с файлом "ModName.mod" в mods. Cоздаём папку по имени ".mod" файла и перемещает туда всё.
func (app *App) normalizeArchiveStructure(tmpDir string) error {
	// Этап 1: убираем внешние обёртки вида "Folder/mods/..."
	for {
		entries, err := os.ReadDir(tmpDir)
		if err != nil {
			break
		}
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
		// Перемещаем всё содержимое modsPath в tmpDir
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
	}

	// Этап 2: если в корне единственная папка "mods" - поднимаем её содержимое
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return err
	}
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
		entries, err = os.ReadDir(tmpDir) // обновляем список
		if err != nil {
			return err
		}
	}

	// Этап 3: если нет ни одной папки, но есть .mod файл - создаём папку мода и перемещаем всё в неё
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
		if err := os.Mkdir(newDir, 0755); err != nil {
			return err
		}
		for _, e := range entries {
			src := filepath.Join(tmpDir, e.Name())
			dst := filepath.Join(newDir, e.Name())
			if e.Name() == modName {
				continue
			}
			if err := os.Rename(src, dst); err != nil {
				if err := copyPath(src, dst); err != nil {
					return err
				}
				os.RemoveAll(src)
			}
		}
	}
	return nil
}

// downloadAndInstallSystemMod загружает и устанавливает системный мод.
func (app *App) downloadAndInstallSystemMod(downloadURL, filename, displayName string, fileInfo *FileInfo, cacheKey string, modID int, installFunc func(string) error, logInstalling, logSuccess, logManual string) {
	safeFilename, err := sanitizeFilename(filename)
	if err != nil {
		app.appendLog(fmt.Sprintf("Invalid filename for %s: %v", displayName, err))
		return
	}

	// Диалог подтверждения
	choice := app.showChoiceDialogSync(
		app.mainWindow,
		app.messages["confirm_download_title"],
		fmt.Sprintf(app.messages["confirm_download_text"], displayName, safeFilename),
		app.messages["btn_yes"],
		app.messages["btn_no"],
	)
	if choice != 0 {
		return
	}

	app.appendLog(fmt.Sprintf(app.messages["log_downloading_mod"], displayName))
	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(fmt.Sprintf(app.messages["downloading"], safeFilename))
	content := container.NewVBox(lbl, bar)
	dlg := dialog.NewCustom(app.messages["download_title"], app.messages["btn_cancel"], content, app.mainWindow)

	ctx, cancel := context.WithCancel(context.Background())
	dlg.SetOnClosed(func() {
		cancel()
	})

	dlg.Show()
	go func() {
		dest := filepath.Join(app.cfg.ModsPath, safeFilename)
		err := app.DownloadFileWithProgress(ctx, downloadURL, dest, bar)
		fyne.Do(func() {
			dlg.Hide()
			if err != nil {
				if err == context.Canceled {
					app.appendLog(app.messages["download_cancelled"])
				} else {
					app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
				}
				os.Remove(dest)
				return
			}
			info, e := os.Stat(dest)
			if e == nil && info.Size() < 100 {
				app.appendLog(fmt.Sprintf(app.messages["log_error_file_too_small"], info.Size()))
				os.Remove(dest)
				return
			}
			app.appendLog(logInstalling)
			if err := installFunc(dest); err != nil {
				app.appendLog(fmt.Sprintf(app.messages["log_install_failed"], err))
			} else {
				if fileInfo != nil {
					app.setCachedVersion(cacheKey, ModVersionInfo{
						Timestamp: fileInfo.UploadedTimestamp,
						Version:   fileInfo.Version,
						Folder:    displayName,
						Source:    "nexus",
					})
					app.saveNexusVersionCache()
				}
				app.appendLog(logSuccess)
			}
			os.Remove(dest)
		})
	}()
}
