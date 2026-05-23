package main

import (
	"Servo-Modquisitor/checks"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
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

		if regMods[i].Incompatible {
			for _, pair := range checks.IncompatiblePairs {
				if pair.Mod1 == regMods[i].Name || pair.Mod2 == regMods[i].Name {
					other := pair.Mod1
					if other == regMods[i].Name {
						other = pair.Mod2
					}
					desc := checks.GetIncompatibleDesc(pair.Mod1, pair.Mod2)
					if desc != "" {
						regMods[i].Note = strings.TrimSpace(regMods[i].Note + app.messages["conflict_with"] + other + ": " + desc)
					}
					break
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
			app.appendLog(state + " (автопатчер)")
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
			app.appendLog(state + " (старый патчер)")
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
			app.appendLog(fmt.Sprintf(app.messages["log_installed_folder"], filepath.Base(path)))
		} else {
			ext := strings.ToLower(filepath.Ext(path))
			if ext == ".zip" || ext == ".rar" || ext == ".7z" {
				err := app.extractArchive(path)
				if err != nil {
					app.appendLog(fmt.Sprintf(app.messages["log_extract_error"], err))
				} else {
					checks.AutoFixMalformed()
					app.refreshModList()
					app.appendLog(fmt.Sprintf(app.messages["log_installed"], filepath.Base(path)))
				}
			} else {
				app.appendLog(app.messages["log_zip_only"])
			}
		}
	}
}

func (app *App) extractArchive(archivePath string) error {
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		return app.extractZip(archivePath)
	case ".rar":
		return app.extractRar(archivePath)
	case ".7z":
		return app.extract7z(archivePath)
	default:
		return fmt.Errorf(app.messages["error_uns_archive"], ext)
	}
}

func (app *App) extractZip(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	destDir := app.cfg.ModsPath
	for _, f := range r.File {
		targetPath, err := safeJoin(destDir, f.Name)
		if err != nil {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.Create(targetPath)
		if err != nil {
			continue
		}
		rc, _ := f.Open()
		if rc != nil {
			io.Copy(outFile, rc)
			rc.Close()
		}
		outFile.Close()
	}
	return nil
}

func (app *App) extractRar(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f)
	if err != nil {
		return err
	}
	destDir := app.cfg.ModsPath
	for {
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetPath, err := safeJoin(destDir, header.Name)
		if err != nil {
			continue
		}
		if header.IsDir {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.Create(targetPath)
		if err != nil {
			continue
		}
		io.Copy(outFile, rr)
		outFile.Close()
	}
	return nil
}

func (app *App) extract7z(path string) error {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	destDir := app.cfg.ModsPath
	for _, f := range r.File {
		targetPath, err := safeJoin(destDir, f.Name)
		if err != nil {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.Create(targetPath)
		if err != nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}
		io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
	}
	return nil
}

func (app *App) extractArchiveTo(archivePath, destDir string) error {
	ext := strings.ToLower(filepath.Ext(archivePath))
	switch ext {
	case ".zip":
		return app.extractZipTo(archivePath, destDir)
	case ".rar":
		return app.extractRarTo(archivePath, destDir)
	case ".7z":
		return app.extract7zTo(archivePath, destDir)
	default:
		return fmt.Errorf(app.messages["error_uns_archive"], ext)
	}
}

func (app *App) extractZipTo(path, destDir string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		targetPath, err := safeJoin(destDir, f.Name)
		if err != nil {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, _ := os.Create(targetPath)
		rc, _ := f.Open()
		if rc != nil {
			io.Copy(outFile, rc)
			rc.Close()
		}
		outFile.Close()
	}
	return nil
}

func (app *App) extractRarTo(path, destDir string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	rr, err := rardecode.NewReader(f)
	if err != nil {
		return err
	}
	for {
		header, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		targetPath, err := safeJoin(destDir, header.Name)
		if err != nil {
			continue
		}
		if header.IsDir {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.Create(targetPath)
		if err != nil {
			continue
		}
		io.Copy(outFile, rr)
		outFile.Close()
	}
	return nil
}

func (app *App) extract7zTo(path, destDir string) error {
	r, err := sevenzip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		targetPath, err := safeJoin(destDir, f.Name)
		if err != nil {
			continue
		}
		if f.FileInfo().IsDir() {
			os.MkdirAll(targetPath, 0755)
			continue
		}
		os.MkdirAll(filepath.Dir(targetPath), 0755)
		outFile, err := os.Create(targetPath)
		if err != nil {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			continue
		}
		io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
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

// DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG DEBUG 
func (app *App) InstallModFromArchive(archivePath string, activate bool) (string, error) {
// app.appendLog(fmt.Sprintf("Installing from archive %s, activate=%v", archivePath, activate))

	tmpDir, err := os.MkdirTemp("", "servo-mod-")
	if err != nil {
app.appendLog(fmt.Sprintf("Failed to create temp dir: %v", err))
		return "", err
	}
	defer os.RemoveAll(tmpDir)
// app.appendLog(fmt.Sprintf("DEBUG: Temp dir: %s", tmpDir))

	if err := app.extractArchiveTo(archivePath, tmpDir); err != nil {
app.appendLog(fmt.Sprintf("Extract failed: %v", err))
		return "", err
	}
// app.appendLog("Extraction complete")

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
// app.appendLog(fmt.Sprintf("DEBUG: Read temp dir failed: %v", err))
		return "", err
	}
// app.appendLog(fmt.Sprintf("DEBUG: Temp dir contains %d entries", len(entries)))
// for _, e := range entries {
// 	app.appendLog(fmt.Sprintf("DEBUG:   %s (dir=%v)", e.Name(), e.IsDir()))
// }

	for _, e := range entries {
		if e.IsDir() {
			modName := e.Name()
			dest := filepath.Join(app.cfg.ModsPath, modName)
app.appendLog(fmt.Sprintf("Moving %s -> %s", modName, dest))
			if err := app.copyFolder(filepath.Join(tmpDir, modName), dest); err != nil {
app.appendLog(fmt.Sprintf("Copy failed: %v", err))
				return "", err
			}

			if !activate {
				app.refreshModList()
				for i := range app.allMods {
					if app.allMods[i].Name == modName {
						app.allMods[i].Active = false
						break
					}
				}
				app.filterModList()
				app.appendLog(fmt.Sprintf(app.messages["log_installed_inactive"], modName))
				return modName, nil
			} else {
				app.refreshModList()
				app.appendLog(fmt.Sprintf(app.messages["log_installed"], modName))
				return modName, nil
			}
		}
	}
	return "", fmt.Errorf("No mod folder found in archive")
}

// Обновление одного обычного мода
func (app *App) updateModFromNexus(mod *checks.ModInfo) {
	if mod.URL == "" || app.cfg.NexusAPIKey == "" {
		app.appendLog(app.messages["update_skipped_no_url"])
		return
	}
	modID := extractModIDFromURL(mod.URL)
	if modID == 0 {
		app.appendLog(fmt.Sprintf(app.messages["cannot_extract_mod_id"], mod.URL))
		return
	}

	modIDStr := fmt.Sprintf("%d", modID)
	fileID, err := getLatestFileID(modID, app.cfg.NexusAPIKey)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_latest_file_id"], err))
		return
	}
	var fileVersion string
	if fi, err := app.FetchFileInfo(modID, fileID, app.cfg.NexusAPIKey); err == nil {
		fileVersion = fi.Version
	}
	directURL, filename, err := app.fetchDirectDownloadLink(fmt.Sprintf("%d", modID), fmt.Sprintf("%d", fileID), app.cfg.NexusAPIKey)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["failed_get_download_link"], err))
		return
	}
	app.showDownloadDialog(directURL, filename, app.cfg.NexusAPIKey, mod.Name, fileVersion, modIDStr)
}

// Обновление всех модов (только проверка через лог)
func (app *App) updateAllModsFromNexus() {
	if app.cfg.NexusAPIKey == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}
	go app.checkNexusUpdates()
}

// Удалить выбранные моды
func (app *App) removeSelectedMods() {
    for _, mod := range app.allMods {
        if mod.Selected && !mod.IsSystem {
            checks.RemoveMod(mod.Name)
            app.appendLog(fmt.Sprintf(app.messages["log_deleted"], mod.Name))
        }
    }
    app.refreshModList()
    app.appendLog(app.messages["log_selected_mods_removed"])
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
			app.appendLog(fmt.Sprintf("Failed to copy %s: %v", entry.Name(), err))
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
