package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/config"
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

func (app *App) refreshModList() {
	mods := checks.GetModsInfo(app.cfg.Language, app.cfg.ForceEnglishModNames)
	entries := checks.ReadLoadOrder()
	if entries == nil {
		sort.Slice(mods, func(i, j int) bool { return mods[i].Name < mods[j].Name })
		for i := range mods {
			mods[i].Active = false
		}
	} else {
		orderMap := make(map[string]int)
		for i, e := range entries {
			orderMap[e.Name] = i
		}
		for i := range mods {
			if _, ok := orderMap[mods[i].Name]; !ok {
				orderMap[mods[i].Name] = len(orderMap)
			}
		}
		sort.Slice(mods, func(i, j int) bool {
			return orderMap[mods[i].Name] < orderMap[mods[j].Name]
		})
	}
	for i := range mods {
		mods[i].Obsolete = app.containsStr(checks.ObsoleteMods, mods[i].Name)
		mods[i].Mandatory = checks.IsMandatoryMod(mods[i].Name)
		mods[i].Incompatible = app.checkIncompatible(mods[i].Name)
	}
	if app.selectedModName != "" {
		exists := false
		for _, m := range mods {
			if m.Name == app.selectedModName {
				exists = true
				break
			}
		}
		if !exists {
			app.selectedModName = ""
		}
	}
	app.allMods = mods
	app.orderDirty = false
	app.filterModList()
}

func (app *App) filterModList() {
	if app.filterSelect == nil {
		app.displayedMods = app.allMods
		if app.modTable != nil {
			app.modTable.Length = func() (int, int) { return len(app.displayedMods), config.TableColumnCount }
			app.modTable.Refresh()
		}
		return
	}

	filter := app.filterSelect.Selected
	if filter == "" || filter == app.messages["filter_all"] {
		filter = app.messages["filter_all"]
	}
	search := strings.ToLower(app.searchEntry.Text)
	app.displayedMods = nil
	for _, mod := range app.allMods {
		displayName := strings.ToLower(mod.DisplayName)
		if search != "" && !strings.Contains(strings.ToLower(mod.Name), search) &&
			!strings.Contains(displayName, search) {
			continue
		}
		switch filter {
		case app.messages["filter_active"]:
			if !mod.Active {
				continue
			}
		case app.messages["filter_inactive"]:
			if mod.Active {
				continue
			}
		case app.messages["filter_obsolete"]:
			if !mod.Obsolete {
				continue
			}
		case app.messages["filter_conflict"]:
			if !mod.Incompatible {
				continue
			}
		}
		app.displayedMods = append(app.displayedMods, mod)
	}
	if app.modTable != nil {
		app.modTable.Length = func() (int, int) { return len(app.displayedMods), config.TableColumnCount }
		app.modTable.Refresh()
		app.modTable.ScrollToTop()
		app.updateUpDownButtons()
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
	app.filterModList()
}

func (app *App) moveSelected(delta int) {
	if app.selectedModName == "" {
		return
	}
	idx := -1
	for i, m := range app.allMods {
		if m.Name == app.selectedModName {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	newIdx := idx + delta
	if newIdx < 0 || newIdx >= len(app.allMods) {
		return
	}
	app.allMods[idx], app.allMods[newIdx] = app.allMods[newIdx], app.allMods[idx]
	app.orderDirty = true
	app.filterModList()
}

func (app *App) findModByName(name string) *checks.ModInfo {
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			return &app.allMods[i]
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
		targetPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
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
		targetPath := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
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
		targetPath := filepath.Join(destDir, f.Name)
		if !strings.HasPrefix(filepath.Clean(targetPath), filepath.Clean(destDir)+string(os.PathSeparator)) {
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
		targetPath := filepath.Join(dst, relPath)
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
		// оставляем значение из конфига
	}
	saveConfig(app.cfg)
}
