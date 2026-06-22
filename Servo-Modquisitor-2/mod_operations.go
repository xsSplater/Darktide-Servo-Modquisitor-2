// mod_operations.go
package main

import (
	"Servo-Modquisitor/checks"
	"context"
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
	// --- Фикс для существующей кривой папки hub_hotkey_menus-main ---
	wrongFolder := filepath.Join(app.cfg.ModsPath, "hub_hotkey_menus-main")
	correctFolder := filepath.Join(app.cfg.ModsPath, "hub_hotkey_menus")
	if info, err := os.Stat(wrongFolder); err == nil && info.IsDir() {
		if _, err := os.Stat(correctFolder); os.IsNotExist(err) {
			if err := os.Rename(wrongFolder, correctFolder); err == nil {
				app.appendLog(app.messages["log_fix_hub_hk_menus"])
			} else {
				app.appendLog(fmt.Sprintf(app.messages["log_failed_fix_hub_hk_menus"], err))
			}
			// } else {
			// Обе папки есть - удаляем неправильную
			// os.RemoveAll(wrongFolder)
			// app.appendLog("Removed duplicate hub_hotkey_menus-main")
		}
	}
	// --- конец фикса ---
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

	for i := range regMods {
		regMods[i].Obsolete = app.containsStr(checks.ObsoleteMods, regMods[i].Name)
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
						// Определяем, на каком языке показывать имя конфликтующего мода
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
	}

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

	app.allMods = regMods
	app.orderDirty = false
	app.filterModList()
	app.updateSystemModsTable()
	app.forceRefreshTable()
}

func (app *App) updateSystemModsTable() {
	if app.systemModsTable != nil {
		app.systemModsTable.Length = func() (int, int) { return len(app.systemMods), TableColumnCount }
		app.systemModsTable.Refresh()
	}
}

func (app *App) saveCurrentOrder() {
	entries := app.buildLoadOrderEntries()
	checks.WriteLoadOrder(entries)
}

func (app *App) buildLoadOrderEntries() []checks.LoadOrderEntry {
	entries := make([]checks.LoadOrderEntry, len(app.allMods))
	for i, m := range app.allMods {
		entries[i] = checks.LoadOrderEntry{Name: m.Name, Active: m.Active}
	}
	return entries
}

func (app *App) toggleModActive(name string, active bool) {
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			app.allMods[i].Active = active
			app.orderDirty = true
			break
		}
	}
	app.updateTableBorder()
	app.filterModList()
}

func (app *App) findModByName(name string) *checks.ModInfo {
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
		err := toggleModsAutoPatch()
		if err != nil {
			app.appendLog(fmt.Sprintf(app.messages["log_toggle_fail"], err))
		} else {
			app.cfg.ModsGloballyEnabled = isModsEnabledAutoPatch()
			state := app.messages["log_mods_enabled"]
			if !app.cfg.ModsGloballyEnabled {
				state = app.messages["log_mods_disabled"]
			}
			app.appendLog(state + app.messages["log_autopatcher"])
		}
	case PatcherLegacy:
		err := toggleModsLegacy()
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
					installedName, version, err := app.InstallModFromArchive(p, true, "")
					fyne.Do(func() {
						if err != nil {
							app.appendLog(fmt.Sprintf(app.messages["log_extract_error"], err))
							return
						}
						checks.AutoFixMalformed()
						app.refreshModList()
						app.selectAndScrollToMod(installedName)
						// Попробуем найти modID по имени папки или извлечь из имени архива
						modID, _, _ := extractVersionAndModIDFromFilename(p)
						if modID != 0 && version != "" {
							cacheKey := fmt.Sprintf("%d:%s", modID, installedName)
							app.cacheModVersion(cacheKey, installedName, version, 0)
						}
						if modID != 0 {
							go app.autoAddModToDatabase(modID, installedName)
						}
						app.orderDirty = true
						app.updateTableBorder()
						app.appendLog(fmt.Sprintf(app.messages["log_installed"], filepath.Base(p)))
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

	// Определяем формат архива по сигнатуре
	format, _, err := archives.Identify(context.Background(), archivePath, f)
	if err != nil {
		return fmt.Errorf("unsupported archive format: %w", err)
	}
	extractor, ok := format.(archives.Extractor)
	if !ok {
		return fmt.Errorf("format does not support extraction")
	}

	// Возвращаем указатель в начало файла
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	// Создаём временную папку для безопасного извлечения
	tmpDir, err := os.MkdirTemp("", "servo-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	// Распаковываем во временную папку
	err = extractor.Extract(context.Background(), f, func(ctx context.Context, fi archives.FileInfo) error {
		// Защита от path traversal
		targetPath, err := safeJoin(tmpDir, fi.NameInArchive)
		if err != nil {
			return err
		}
		if fi.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		// Создаём родительские каталоги
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
		return err
	})
	if err != nil {
		return err
	}

	// Копируем содержимое временной папки в целевую директорию
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
		app.cfg.ModsGloballyEnabled = isModsEnabledAutoPatch()
	case PatcherLegacy:
	}
	saveConfig(app.cfg)
}

func (app *App) selectedMods() []string {
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

func (app *App) InstallModFromArchive(archivePath string, activate bool, knownVersion string) (string, string, error) {
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
		return "", "", err
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
				app.appendLog("Binaries installed successfully.")
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

	// Получаем список оставшихся папок (обычные моды)
	entries, err := os.ReadDir(tmpDir)
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
		// Защита от случайной установки системных папок
		if modName == "base" || modName == "dmf" {
			app.appendLog(fmt.Sprintf(app.messages["log_skipping_sys_folder"], modName))
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
	}

	if len(installedNames) == 0 {
		return "", "", fmt.Errorf(app.messages["log_no_mod_folder_found"], err)
	}

	installedName := installedNames[0] // для обратной совместимости возвращаем первый

	// Определяем версию (только для первого мода)
	version := knownVersion
	if version == "" {
		_, extractedVersion, _ := extractVersionAndModIDFromFilename(archivePath)
		if extractedVersion != "" {
			version = extractedVersion
		} else {
			version = app.promptUserForVersion(installedName)
		}
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

	// Кэшируем версию для первого мода
	if modID, _, _ := extractVersionAndModIDFromFilename(archivePath); modID != 0 && version != "" {
		cacheKey := fmt.Sprintf("%d:%s", modID, installedName)
		app.cacheModVersion(cacheKey, installedName, version, 0)
	}

	// Автоматически добавляем в базу данных
	if modID, _, _ := extractVersionAndModIDFromFilename(archivePath); modID != 0 {
		go app.autoAddModToDatabase(modID, installedName)
	}

	return installedName, version, nil
}

// Обновление одного мода. Только для Premium-пользователей!
func (app *App) updateModFromNexus(mod *checks.ModInfo) {
	if mod.URL == "" || app.getAuthToken() == "" {
		app.appendLog(app.messages["update_skipped_no_url"])
		return
	}
	modID := extractModIDFromURL(mod.URL)
	if modID == 0 {
		app.appendLog(fmt.Sprintf(app.messages["cannot_extract_mod_id"], mod.URL))
		return
	}

	modIDStr := fmt.Sprintf("%d", modID) // ← оставь эту строку

	fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
	if err != nil {
		app.logNexusError(err, mod.Name)
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showDownloadDialog(directURL, filename, mod.Name, fileInfo, modIDStr)
}

// Обновление всех модов (только те, у которых есть обновление). Только для Premium-пользователей!
func (app *App) updateAllModsFromNexus() {
	app.appendLog(app.messages["log_starting_batch_update"])
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}

	app.appendLog(app.messages["log_collecting_mods"])
	// Сначала собираем список модов, для которых действительно есть обновление
	var modsToUpdate []*checks.ModInfo
	for i := range app.allMods {
		mod := &app.allMods[i]
		if mod.URL == "" || mod.IsSystem {
			continue
		}
		modID := extractModIDFromURL(mod.URL)
		if modID == 0 {
			continue
		}
		// Получаем актуальную информацию о последнем файле, соответствующем папке
		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
			continue
		}
		cacheKey := fmt.Sprintf("%d:%s", modID, mod.Name)
		var saved ModVersionInfo
		if info, exists := app.nexusVersionCache[cacheKey]; exists {
			saved = info
		}
		if saved.Timestamp == 0 || fileInfo.UploadedTimestamp > saved.Timestamp {
			modsToUpdate = append(modsToUpdate, mod)
		}
	}

	if len(modsToUpdate) == 0 {
		app.appendLog(app.messages["no_updates_found"])
		return
	}

	// Диалог подтверждения
	choice := app.showChoiceDialog(
		app.mainWindow,
		app.messages["update_title"],
		fmt.Sprintf(app.messages["updates_found_count_update"], len(modsToUpdate)),
		app.messages["yes"],
		app.messages["btn_cancel"],
	)
	if choice != 0 {
		return
	}

	// Обновляем
	updatedCount := 0
	for _, mod := range modsToUpdate {
		app.appendLog(fmt.Sprintf(app.messages["updating_mod"], mod.Name))
		app.updateModFromNexus(mod)
		updatedCount++
		time.Sleep(500 * time.Millisecond)
	}
	app.appendLog(fmt.Sprintf(app.messages["update_all_finished"], updatedCount))
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
	for _, mod := range app.allMods {
		if !mod.IsSystem {
			checks.RemoveMod(mod.Name)
			app.appendLog(fmt.Sprintf(app.messages["log_deleted"], mod.Name))
		}
	}
	app.refreshModList()
	app.orderDirty = true
	app.updateTableBorder()
	app.appendLog(app.messages["log_all_mods_removed"])
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

func (app *App) updateAutopatcher() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	const autopatchModID = 709
	app.appendLog(fmt.Sprintf(app.messages["looking_for_latest_file"], autopatchModID))
	fileInfo, err := app.getLatestFileInfo(autopatchModID)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}

	modIDStr := fmt.Sprintf("%d", autopatchModID)
	cacheKey := "709:autopatch"
	var saved ModVersionInfo
	if info, exists := app.nexusVersionCache[cacheKey]; exists {
		saved = info
	}
	if saved.Timestamp != 0 && fileInfo.UploadedTimestamp <= saved.Timestamp {
		app.appendLog(fmt.Sprintf(app.messages["already_latest"], "Autopatcher", fileInfo.Version))
		return
	}

	directURL, filename, err := app.getPremiumDownloadURL(modIDStr, fmt.Sprintf("%d", fileInfo.ID))
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	fyne.Do(func() {
		app.showAutopatcherDownloadDialog(directURL, filename, fileInfo)
	})
}

func (app *App) installAutopatcherFromArchive(archivePath string) error {
	// Автопатчер устанавливается в корень игры, а не в mods
	return app.installDMLFromArchive(archivePath) // у него такая же структура установки
}

// extractVersionAndModIDFromFilename пытается извлечь версию и ID мода из имени файла
// Паттерн: Название-МодID-Версия-Время.zip
// Версия может состоять из нескольких частей (например, 1-01, 26.02.08-1).
// Последняя часть, похожая на Unix timestamp (число из 8-10 цифр), не включается в версию.
func extractVersionAndModIDFromFilename(filename string) (modID int, version string, ok bool) {
	name := filepath.Base(filename)
	// Удаляем расширение
	ext := filepath.Ext(name)
	if ext != "" {
		name = name[:len(name)-len(ext)]
	}
	parts := strings.Split(name, "-")
	if len(parts) < 3 {
		return 0, "", false
	}

	// Ищем часть, которая является modID (число от 1 до 9999)
	modIDIdx := -1
	for i, part := range parts {
		if id, err := strconv.Atoi(part); err == nil && id > 0 && id < 10000 {
			modID = id
			modIDIdx = i
			break
		}
	}
	if modIDIdx == -1 {
		return 0, "", false
	}

	// Собираем версию из частей, следующих за modID, до тех пор, пока не встретим timestamp
	var versionParts []string
	for i := modIDIdx + 1; i < len(parts); i++ {
		part := parts[i]
		// Если часть выглядит как Unix timestamp (все цифры, длина 8-10) - останавливаемся
		if isNumeric(part) && len(part) >= 8 && len(part) <= 10 {
			break
		}
		versionParts = append(versionParts, part)
	}

	if len(versionParts) == 0 {
		return modID, "", false
	}

	// Объединяем части версии через точку
	version = strings.Join(versionParts, ".")
	// Убираем лишние точки в начале/конце
	version = strings.Trim(version, ".")
	if version == "" {
		return modID, "", false
	}
	return modID, version, true
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
		entry.SetPlaceHolder(app.messages["placeholder_mod_verion"])
		var dlg dialog.Dialog
		content := container.NewVBox(
			widget.NewLabel(fmt.Sprintf(app.messages["failed_get_mod_version"], modName)),
			entry,
			container.NewHBox(
				widget.NewButton(app.messages["btn_save"], func() {
					resultChan <- entry.Text
					dlg.Hide()
				}),
				widget.NewButton(app.messages["btn_cancel"], func() {
					resultChan <- ""
					dlg.Hide()
				}),
			),
		)
		dlg = dialog.NewCustom(app.messages["mod_version"], "", content, app.mainWindow)
		dlg.Resize(fyne.NewSize(400, 200))
		dlg.Show()
	})
	return <-resultChan
}

func (app *App) cacheModVersion(cacheKey, folderName, version string, timestamp int64) {
	if version == "" {
		return
	}
	app.nexusVersionCache[cacheKey] = ModVersionInfo{
		Timestamp: timestamp,
		Version:   version,
		Folder:    folderName,
	}
	app.saveNexusVersionCache()
}

// removeModFromData удаляет мод из внутренних структур и возвращает индекс, на котором он находился в displayedMods
func (app *App) removeModFromData(modName string) (indexInDisplayed int, found bool) {
	// Удаляем из allMods
	for i, m := range app.allMods {
		if m.Name == modName {
			app.allMods = append(app.allMods[:i], app.allMods[i+1:]...)
			break
		}
	}
	// Удаляем из displayedMods и запоминаем индекс
	for i, m := range app.displayedMods {
		if m.Name == modName {
			indexInDisplayed = i
			found = true
			app.displayedMods = append(app.displayedMods[:i], app.displayedMods[i+1:]...)
			break
		}
	}
	// Если мод был выбран, сбрасываем выделение (оно будет восстановлено позже)
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
