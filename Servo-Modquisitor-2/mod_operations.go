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
	// Приводим имя к Unix-стилю (заменяем \ на /)
	cleanName := strings.ReplaceAll(name, "\\", "/")

	// Запрещаем абсолютные пути (Unix-стиль)
	if strings.HasPrefix(cleanName, "/") {
		return "", fmt.Errorf("absolute path not allowed")
	}
	// Запрещаем Windows-абсолютные пути (содержат :/)
	// Проверка :\\ уже не нужна, так как мы заменили \ на / выше.
	if strings.Contains(cleanName, ":/") {
		return "", fmt.Errorf("absolute path not allowed")
	}

	// Собираем путь с использованием системного разделителя
	// ВАЖНО: используем cleanName, а не name, чтобы избежать создания
	// файлов с обратными слешами в именах на Linux.
	full := filepath.Join(destDir, filepath.FromSlash(cleanName))

	// Проверяем, что результирующий путь действительно внутри destDir
	rel, err := filepath.Rel(destDir, full)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", fmt.Errorf("path traversal detected")
	}
	return full, nil
}

func (app *App) refreshModList() {
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
	showSystem := app.cfg.ShowSystemMods
	app.cfgMutex.RUnlock()

	mods := checks.GetModsInfo(lang, forceEnglish)

	var sysMods, regMods []checks.ModInfo
	for _, m := range mods {
		if m.IsSystem {
			if showSystem {
				m.Active = true // Системные моды всегда активны
				sysMods = append(sysMods, m)
			}
		} else {
			regMods = append(regMods, m)
		}
	}

	// Сортируем системные моды в нужном порядке
	sort.Slice(sysMods, func(i, j int) bool {
		order := map[string]int{"base": 0, "dmf": 1, "autopatch": 2}
		oi, ok1 := order[sysMods[i].Name]
		if !ok1 {
			oi = 99
		}
		oj, ok2 := order[sysMods[j].Name]
		if !ok2 {
			oj = 99
		}
		if oi != oj {
			return oi < oj
		}
		return sysMods[i].Name < sysMods[j].Name
	})

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

	// Снимки за цикл: GetObsoleteMods/GetMandatoryOrder возвращают
	// полную копию слайса при каждом вызове, а IsMandatoryMod берёт
	// RLock. На 267 модах это давало сотни аллокаций и lock/unlock
	// на пустом месте — существенная часть задержки refreshModList.
	incompatiblePairs := checks.GetIncompatiblePairs()
	obsoleteSnapshot := checks.GetObsoleteMods()
	mandatorySnapshot := checks.GetMandatoryOrder()

	for i := range regMods {
		regMods[i].Obsolete = helpers.ContainsString(obsoleteSnapshot, regMods[i].Name)
		regMods[i].Mandatory = helpers.ContainsString(mandatorySnapshot, regMods[i].Name)
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
						regMods[i].ConflictWith = displayName
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

	// Цикл для MissingFolder (только для regMods)
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
	app.amlDetected.Store(checks.IsAMLInstalled(modsPath))
	app.applyAMLWarningState()

	// Собираем все моды вместе
	app.modsMutex.Lock()
	app.allMods = append(sysMods, regMods...)
	app.systemMods = sysMods
	app.orderDirty.Store(false)
	app.modsMutex.Unlock()

	// Установка флага HasUpdate (теперь для всех модов)
	app.modsMutex.Lock()
	for i := range app.allMods {
		mod := &app.allMods[i]
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
	app.modsMutex.Unlock()

	app.pruneVersionCache()
	app.filterModList()
	app.forceRefreshTable()
	app.updateTableBorder()
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
	var entries []checks.LoadOrderEntry
	for _, m := range app.allMods {
		if m.IsSystem {
			continue
		}
		entries = append(entries, checks.LoadOrderEntry{Name: m.Name, Active: m.Active})
	}
	return entries
}

func (app *App) toggleModActive(name string, active bool) {
	app.modsMutex.Lock()
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			app.allMods[i].Active = active
			app.orderDirty.Store(true)
			break
		}
	}
	app.modsMutex.Unlock()
	app.updateTableBorder()
	app.filterModList()
	// app.forceRefreshTable()
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

// func (app *App) removeFromAllMods(name string) {
// 	app.modsMutex.Lock()
// 	defer app.modsMutex.Unlock()
// 	for i, m := range app.allMods {
// 		if m.Name == name {
// 			app.allMods = append(app.allMods[:i], app.allMods[i+1:]...)
// 			break
// 		}
// 	}
// }

// toggleGlobalMods — обработчик кнопки on/off в верхней панели
// и горячей клавиши Ctrl+O.
//
// Логика:
//  1. Проверяем, что игра вообще поддерживает патч (patcher == PatcherAutoPatch).
//  2. Вызываем ToggleBundle — он сам определяет состояние:
//     • чужой патч (DMA/dtkit)  → ErrBundleForeignPatch
//     • уже пропатчено (любым)  → ErrBundleAlreadyPatched
//     • нет патча, якорь есть   → PatchBundle → success
//     • нет патча, якоря нет    → ErrBundlePatchOffset
//  3. Каждую ветку обрабатываем отдельно:
//     • foreign/backup/offset — показываем сообщение пользователю;
//     • already patched — не ошибка, просто «моды уже включены»;
//     • success — пишем в cfg и обновляем кнопку.
func (app *App) toggleGlobalMods() {
	gameRoot, patcher := app.getGameState()
	if patcher != PatcherAutoPatch || gameRoot == "" {
		app.appendLog(app.msg("patcher_not_available"))
		return
	}

	// Учитывает DLL: если она установлена — управляет флагом
	// DISABLE_AUTOPATCHER, при этом bundle остаётся согласован.
	// Возвращаемое состояние намеренно игнорируем: ниже читаем
	// фактическое с диска через IsModsEnabled. Так UI не разъедется
	// с реальностью, если какая-то из операций (установка флага или
	// патч/распатч bundle) частично провалилась.
	_, err := ToggleBundleWithAutopatcher(gameRoot)

	// ErrBundleAlreadyPatched при включении с DLL — не ошибка;
	// ErrBundleForeignPatch при выключении — флаг всё равно встанет,
	// просто bundle может не откатиться.
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("toggleGlobalMods: %v", err))
		// Не выходим: даже при ошибке флаг мог поменяться. Читаем
		// фактическое состояние и обновляем UI по нему.
	}

	// Фактическое состояние с диска.
	enabled := IsModsEnabled(gameRoot)

	app.cfgMutex.Lock()
	app.cfg.ModsGloballyEnabled = enabled
	app.cfgMutex.Unlock()
	app.saveConfigSafe()

	if enabled {
		app.appendLog(app.msg("log_mods_enabled"))
	} else {
		app.appendLog(app.msg("log_mods_disabled"))
	}
	app.updateToggleButtonText(app.btnToggle)
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
			folderName := filepath.Base(path)
			dst := filepath.Join(modsPath, folderName)

			// Копирование папки — в горутину: обход дерева и запись
			// файлов на диск на большом моде могут занять секунды,
			// а SetOnDropped вызывается в UI-потоке. Без этого UI
			// подвисает до конца копирования.
			go func(src, dst, name string) {
				if err := app.copyFolder(src, dst); err != nil {
					app.appendLogToFile(fmt.Sprintf(
						"handleDrop: failed to copy folder %q: %v", name, err))
					fyne.Do(func() {
						app.appendLog(fmt.Sprintf(app.msg("log_extract_error"), err))
					})
					return
				}
				fyne.Do(func() {
					checks.AutoFixMalformed()
					app.refreshModList()
					app.orderDirty.Store(true)
					app.updateTableBorder()
					app.appendLog(fmt.Sprintf(app.msg("log_installed_folder"), name))
				})
			}(path, dst, folderName)
			continue
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".zip" && ext != ".rar" && ext != ".7z" {
			app.appendLog(app.msg("log_zip_only"))
			continue
		}

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

					// ModID определяем по записи в БД (по имени папки),
					// а не по имени файла. Причина: Nexus переписывает
					// схемы имён по несколько раз в год, и парсер имени
					// всегда отстаёт. Для LoadoutSelectionFix-
					// 811-1-2-1775345918 дефисный парсер извлекал «2»
					// (последнее короткое число) вместо 811, после чего
					// autoAddModToDatabase конфликтовал с уже корректной
					// записью в базе и писал в лог «already exists with
					// ID 2».
					//
					// Имя папки — стабильный якорь: оно же лежит в
					// load order, в mod_database.json (поле folder) и
					// в nexus_versions.json.
					//
					// Разбор имени файла оставляем только как fallback
					// для мода, которого ещё нет в базе (первая установка
					// чего-то нового).
					entry := checks.GetModDBEntry(installedName)
					if entry == nil || entry.URL == "" {
						if modID, _, _ := extractVersionAndModIDFromFilename(p); modID != 0 {
							go app.autoAddModToDatabase(modID, installedName, filepath.Base(p))
						}
					}
					app.orderDirty.Store(true)
					app.updateTableBorder()
					app.appendLog(fmt.Sprintf(app.msg("log_installed"), filepath.Base(p)))
				} else {
					app.appendLog(app.msg("log_sorting_files_updated_manual"))
				}
			})
		}(path)
	}
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

// syncModsEnabledState вычитывает актуальное состояние патча из
// bundle_database.data и сохраняет его в cfg.ModsGloballyEnabled.
//
// Патчер теперь один — наш собственный (bundle_patch.go).
// PatcherLegacy устарел и не возвращается detectPatcherTypeWithRoot.
func (app *App) syncModsEnabledState() {
	gameRoot, patcher := app.getGameState()

	// Если игры/патчера нет — просто выходим, не трогаем сохранённое
	// состояние. Раньше в этом случае конфиг оставался как был, и
	// пользователь видел то же значение, что и при последнем успешном
	// старте. Сохраняем прежнее поведение.
	if patcher != PatcherAutoPatch || gameRoot == "" {
		return
	}

	enabled := isModsEnabledAutoPatch(gameRoot)

	app.cfgMutex.Lock()
	app.cfg.ModsGloballyEnabled = enabled
	app.cfgMutex.Unlock()
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

	var sysMods, regMods []checks.ModInfo
	for _, m := range app.allMods {
		if m.IsSystem {
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}

	if len(selNames) == 1 {
		idx := -1
		for i, m := range regMods {
			if m.Name == selNames[0] {
				idx = i
				break
			}
		}
		if idx != -1 {
			newIdx := idx + delta
			if newIdx >= 0 && newIdx < len(regMods) {
				regMods[idx], regMods[newIdx] = regMods[newIdx], regMods[idx]
				app.orderDirty.Store(true)
			}
		}
	} else {
		var selected, others []checks.ModInfo
		for _, m := range regMods {
			if m.Selected {
				selected = append(selected, m)
			} else {
				others = append(others, m)
			}
		}
		originalFirstIdx := -1
		for i, m := range regMods {
			if m.Selected {
				originalFirstIdx = i
				break
			}
		}
		if originalFirstIdx != -1 {
			insertIdx := originalFirstIdx + delta
			if insertIdx < 0 {
				insertIdx = 0
			}
			if insertIdx > len(others) {
				insertIdx = len(others)
			}
			var result []checks.ModInfo
			result = append(result, others[:insertIdx]...)
			result = append(result, selected...)
			result = append(result, others[insertIdx:]...)
			regMods = result
			app.orderDirty.Store(true)
		}
	}

	app.allMods = append(sysMods, regMods...)
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
	var sysMods, regMods []checks.ModInfo
	for _, m := range app.allMods {
		if m.IsSystem {
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}
	var selected, others []checks.ModInfo
	for _, m := range regMods {
		if m.Selected {
			selected = append(selected, m)
		} else {
			others = append(others, m)
		}
	}
	regMods = append(selected, others...)
	app.allMods = append(sysMods, regMods...)
	app.orderDirty.Store(true)
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
	var sysMods, regMods []checks.ModInfo
	for _, m := range app.allMods {
		if m.IsSystem {
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}
	var selected, others []checks.ModInfo
	for _, m := range regMods {
		if m.Selected {
			selected = append(selected, m)
		} else {
			others = append(others, m)
		}
	}
	regMods = append(others, selected...)
	app.allMods = append(sysMods, regMods...)
	app.orderDirty.Store(true)
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
	if err != nil || visiblePos < 1 {
		return
	}

	app.modsMutex.Lock()
	var sysMods, regMods []checks.ModInfo
	for _, m := range app.allMods {
		if m.IsSystem {
			sysMods = append(sysMods, m)
		} else {
			regMods = append(regMods, m)
		}
	}

	targetIdx := visiblePos - 1 - len(sysMods)
	if targetIdx < 0 {
		targetIdx = 0
	}

	var selected, others []checks.ModInfo
	for _, m := range regMods {
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
	regMods = result

	app.allMods = append(sysMods, regMods...)
	app.orderDirty.Store(true)
	app.modsMutex.Unlock()

	app.updateTableBorder()
	app.filterModList()
	app.applySelectionFromMods()
	app.forceRefreshTable()
}

// archiveKind — классификация содержимого распакованного архива.
type archiveKind int

const (
	archiveKindMod     archiveKind = iota // обычный мод (одна или несколько папок)
	archiveKindSorting                    // файлы сортировки (mod_database.json / mandatory_rules)
	archiveKindProgram                    // архив самой программы Servo-Modquisitor
	// archiveKindGameOverlay — архив, который повторяет структуру
	// корня игры: содержит папку mods/ плюс сопутствующие файлы
	// (bat-скрипты, tools/, вспомогательные exe). Устанавливается
	// целиком в корень игры, а не в <game>/mods/<wrapper>.
	//
	// Пример: Custom Assets (Nexus mod 1200) — папка-обёртка
	// Custom-Assets-patcher содержит CUSTOM_ASSETS_PATCH.bat,
	// mods/CustomAssets/ и tools/custom-assets-patcher.exe.
	archiveKindGameOverlay
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

	case archiveKindGameOverlay:
		if gameRoot == "" {
			return "", "", fmt.Errorf("game root not set")
		}
		names, err := app.installGameOverlay(tmpDir, gameRoot)
		if err != nil {
			return "", "", err
		}
		if len(names) == 0 {
			// Overlay без модов — только файлы уровня корня (например,
			// патчер без модов). Ничего активировать не нужно.
			app.appendLogToFile("Game overlay installed without mods")
			return "", "", nil
		}
		// Первый мод — «главный» для UI и обратной совместимости.
		installedName := names[0]

		manual := modNameToUpdate == ""
		modID, version := app.resolveModIDAndVersion(archivePath, installedName, knownVersion, manual)
		app.finalizeModInstallUI(installedName, modID, activate, archivePath)

		source := "nexus"
		if manual {
			source = "manual"
		}
		app.cacheInstalledVersion(modID, installedName, version, source)

		return installedName, version, nil
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
	manual := modNameToUpdate == ""
	modID, version := app.resolveModIDAndVersion(archivePath, installedName, knownVersion, manual)

	// 5. Обновляем UI и активируем мод, если нужно.
	app.finalizeModInstallUI(installedName, modID, activate, archivePath)

	// 6. Кешируем версию.
	source := "nexus"
	if manual {
		source = "manual"
	}
	app.cacheInstalledVersion(modID, installedName, version, source)

	return installedName, version, nil
}

// ─────────────────────────────────────────────────────────────────
// Шаги установки
// ─────────────────────────────────────────────────────────────────

// detectOverlayWrapper ищет папку-обёртку вида Custom-Assets-patcher:
// в корне tmpDir ровно одна директория, внутри неё есть mods/ и
// хотя бы что-то ещё (bat, tools/, exe).
//
// Возвращает путь к обёртке или "" если не найдено.
func detectOverlayWrapper(tmpDir string) string {
	entries, err := os.ReadDir(tmpDir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return ""
	}
	wrapper := filepath.Join(tmpDir, entries[0].Name())

	inner, err := os.ReadDir(wrapper)
	if err != nil {
		return ""
	}
	hasMods := false
	hasOther := false
	for _, e := range inner {
		if e.IsDir() && strings.EqualFold(e.Name(), "mods") {
			hasMods = true
		} else {
			hasOther = true
		}
	}
	if hasMods && hasOther {
		return wrapper
	}
	return ""
}

// unwrapFolder поднимает содержимое wrapper/* на уровень tmpDir/*.
// Обёртка удаляется.
func (app *App) unwrapFolder(wrapper, tmpDir string) error {
	entries, err := os.ReadDir(wrapper)
	if err != nil {
		return err
	}
	for _, e := range entries {
		src := filepath.Join(wrapper, e.Name())
		dst := filepath.Join(tmpDir, e.Name())
		if err := os.Rename(src, dst); err != nil {
			// Кросс-девайс fallback: копируем и удаляем.
			if cpErr := copyPath(src, dst); cpErr != nil {
				return cpErr
			}
			os.RemoveAll(src)
		}
	}
	return os.RemoveAll(wrapper)
}

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

	// Unwrap game-overlay обёрток (Custom-Assets-patcher):
	// папка-обёртка с mods/ и содержимым корня игры поднимается
	// на уровень tmpDir. Затем classifyArchive увидит mods/ в корне
	// и обработает как archiveKindGameOverlay.
	//
	// ВАЖНО: это должно идти ДО normalizeArchiveStructure — иначе
	// Этап 1 нормализации сам «поднимет» mods/* и удалит обёртку
	// вместе со всем остальным (bat, tools/).
	if wrapper := detectOverlayWrapper(tmpDir); wrapper != "" {
		app.appendLogToFile(fmt.Sprintf(
			"Unwrapping game-overlay wrapper: %s", filepath.Base(wrapper)))
		if err := app.unwrapFolder(wrapper, tmpDir); err != nil {
			app.appendLogToFile(fmt.Sprintf(
				"unwrapFolder failed for %s: %v", wrapper, err))
			// Не прерываем — пусть normalize попробует.
		}
	}

	if err := app.normalizeArchiveStructure(tmpDir); err != nil {
		app.appendLog(fmt.Sprintf(app.msg("log_failed_normalize"), err))
	}

	return tmpDir, nil
}

// classifyArchive определяет тип архива по его содержимому:
//   - программа (в корне <AppName>.exe),
//   - файлы сортировки (в корне mod_database.json / mandatory_rules),
//   - game overlay (в корне mods/ + что-то ещё),
//   - обычный мод.
//
// Читает только корень tmpDir — вложенные каталоги не считаются.
func classifyArchive(tmpDir string) archiveKind {
	// Архив программы — ищем <AppName>.exe в корне.
	expectedExe := AppName + ".exe"
	if _, err := os.Stat(filepath.Join(tmpDir, expectedExe)); err == nil {
		return archiveKindProgram
	}

	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return archiveKindMod
	}

	// Архив сортировки — mod_database.json и/или mandatory_rules в корне.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == FileNameModDatabase || e.Name() == FileNameMandatoryRules {
			return archiveKindSorting
		}
	}

	// Game overlay — есть папка mods/ в корне.
	// После unwrap-шага (см. detectOverlayWrapper / unwrapFolder)
	// сюда попадают архивы вида Custom-Assets-patcher.
	//
	// Если mods/ — единственный элемент в корне, это тоже сработает:
	// результат идентичен обычной установке (game/mods/MyMod/).
	// Разницы в поведении нет, а логика одна.
	for _, e := range entries {
		if e.IsDir() && strings.EqualFold(e.Name(), "mods") {
			return archiveKindGameOverlay
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

// installGameOverlay копирует содержимое tmpDir прямо в корень игры.
//
// Используется для архивов, повторяющих структуру game root: bat-скрипт
// плюс tools/ плюс mods/. Пример: Custom Assets (Nexus mod 1200).
//
// Копирование сливающее: если game/mods/ уже существует, содержимое
// mods/CustomAssets/ из архива добавится рядом с существующими модами,
// а не заменит всю папку mods.
//
// Возвращает список имён модов, найденных в mods/, — чтобы вызывающий
// код мог добавить их в load order и обновить UI.
func (app *App) installGameOverlay(tmpDir, gameRoot string) ([]string, error) {
	if gameRoot == "" {
		return nil, fmt.Errorf("game root not set")
	}

	// Копируем всё как есть в корень игры.
	if err := copyPath(tmpDir, gameRoot); err != nil {
		return nil, fmt.Errorf("copy overlay to game root: %w", err)
	}
	app.appendLogToFile(fmt.Sprintf(
		"Game overlay installed to: %s", gameRoot))

	// Собираем имена модов из mods/.
	modsDir := filepath.Join(tmpDir, "mods")
	entries, err := os.ReadDir(modsDir)
	if err != nil {
		// Папки mods/ может не быть — тогда это overlay без модов
		// (просто вспомогательные файлы). Не ошибка.
		return nil, nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
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

// resolveModIDAndVersion определяет modID и версию устанавливаемого мода.
//
// manual == true — установка инициирована пользователем (drag-and-drop
// или диалог выбора файла). manual == false — установка/обновление
// через NXM-ссылку или API Nexus.
//
// Для ручной установки известного мода (запись есть в mod_database.json)
// имя файла НЕ разбирается вообще: пользователь мог переустановить
// старый архив, и версия в имени файла не отражает реальность. А имя
// папки — стабильно, оно же лежит в load order и в nexus_versions.json.
//
// Для первой установки неизвестного мода (записи в DB нет) разбор
// имени файла — единственная зацепка, поэтому он остаётся как fallback.
func (app *App) resolveModIDAndVersion(archivePath, installedName, knownVersion string, manual bool) (int, string) {
	modID := 0
	version := knownVersion

	// 1. ModID — база по имени папки. Самый надёжный источник,
	// потому что folder в DB и имя папки на диске совпадают по
	// определению.
	if entry := checks.GetModDBEntry(installedName); entry != nil && entry.URL != "" {
		modID = helpers.ExtractModIDFromURL(entry.URL)
	}

	// 2. Имя файла — только если мода нет в базе (первая установка
	// чего-то нового). Для NXM-flow сюда тоже заходим как fallback,
	// если API не отдал версию.
	if modID == 0 {
		if idFromFile, v, ok := extractVersionAndModIDFromFilename(archivePath); ok && idFromFile != 0 {
			modID = idFromFile
			// Версию из имени файла доверяем только в NXM-flow.
			// Для ручной установки неизвестного мода — тоже берём,
			// спросить всё равно не у кого.
			if v != "" && v != "unknown" && version == "" {
				version = v
			}
		}
	}

	// 3. Если версии всё ещё нет:
	if version == "" {
		if manual {
			// Пользователь сам закинул архив — пусть введёт версию.
			version = app.promptUserForVersion(installedName)
		} else {
			// Установка из Nexus. Не блокируем — ставим "unknown",
			// пользователь поправит кнопкой «Edit version» при желании.
			version = "unknown"
		}
	}
	return modID, version
}

// finalizeModInstallUI обновляет UI после установки: пересобирает список,
// активирует мод (если нужно), синхронизирует профиль с игровой папкой,
// прокручивает к установленному моду.
//
// Вся работа идёт в главном потоке Fyne.
// finalizeModInstallUI получает modID, чтобы корректно сигналить батч.
func (app *App) finalizeModInstallUI(installedName string, modID int, activate bool, archivePath string) {
	fyne.DoAndWait(func() {
		app.appendLogToFile("InstallModFromArchive: entering fyne.DoAndWait")

		// 1. СНАЧАЛА обновляем список, чтобы новый мод появился в app.allMods
		app.refreshModList()
		app.appendLogToFile("InstallModFromArchive: after refreshModList")

		// 2. ТЕПЕРЬ включаем его, так как он уже есть в списке
		if activate {
			app.modsMutex.Lock()
			for i := range app.allMods {
				if app.allMods[i].Name == installedName {
					app.allMods[i].Active = true
					break
				}
			}
			app.modsMutex.Unlock()

			app.orderDirty.Store(true)
			app.updateTableBorder()

			// 3. Сохраняем порядок уже с включенным модом
			app.saveCurrentOrder()
		}

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

		// Сигналим батчу, что мод установлен
		select {
		case app.batchInstallChan <- BatchSignal{Folder: installedName, NexusModID: modID}:
		default:
		}
	})
}

// cacheInstalledVersion сохраняет версию мода в кеше и, для системных
// модов (base/dmf), синхронизирует кеш с локальными файлами.
func (app *App) cacheInstalledVersion(modID int, installedName, version string, source string) {
	if version != "" && modID != 0 {
		cacheKey := fmt.Sprintf("%d:%s", modID, installedName)
		if source == "" {
			source = "manual"
		}
		app.cacheModVersion(cacheKey, installedName, version, 0, source, 0)
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
					// callback — UI-поток, а doUpdateModFromNexus →
					// showDownloadDialog → startDownload с fyne.Do + wait.
					// Обязательно уводим в горутину.
					go app.doUpdateModFromNexus(mod, modID, modIDStr, cacheKey, skipConfirm)
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
func (app *App) doUpdateModFromNexus(mod *checks.ModInfo, modID int, modIDStr, _ string, skipConfirm bool) {
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

// Удалить выбранные моды
func (app *App) removeSelectedMods() {
	app.modsMutex.RLock()
	var selectedNames []string
	for _, m := range app.allMods {
		if m.Selected && !m.IsSystem {
			selectedNames = append(selectedNames, m.Name)
		}
	}
	// Запоминаем ПОЗИЦИЮ первого выделенного мода в displayedMods.
	// После удаления на эту позицию придёт мод, который был следующим
	// за удалённым (или предыдущий, если удаляли с конца списка).
	// Имя удалённого мода использовать нельзя — его в новом списке нет,
	// из-за чего поиск не находил его и сбрасывал фокус на строку 0.
	firstSelectedRow := -1
	for i, m := range app.displayedMods {
		if m.Selected {
			firstSelectedRow = i
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
		// Используем атомарную синхронизацию, которая также обновляет сигнатуру
		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLogToFile(fmt.Sprintf("removeSelectedMods: sync failed: %v", err))
		}

		fyne.Do(func() {
			app.orderDirty.Store(false)
			app.updateTableBorder()
			app.refreshModList()
			app.filterModList()
			app.forceRefreshTable()

			app.modsMutex.RLock()
			displayed := make([]checks.ModInfo, len(app.displayedMods))
			copy(displayed, app.displayedMods)
			app.modsMutex.RUnlock()

			if len(displayed) > 0 {
				targetRow := firstSelectedRow
				if targetRow < 0 {
					targetRow = 0
				}
				if targetRow >= len(displayed) {
					targetRow = len(displayed) - 1
				}
				app.modTable.Select(widget.TableCellID{Row: targetRow, Col: 0}, 0)
				app.modTable.ScrollTo(widget.TableCellID{Row: targetRow, Col: 0})
			} else {
				app.selectedModName = ""
				app.selectedModIndex.Store(-1)
				app.updateDescriptionForMod("")
				app.updateUpDownButtons()
			}

			app.orderDirty.Store(false)
			app.updateTableBorder()
			app.appendLog(app.msg("log_selected_mods_removed"))
		})
	}()
}

// Удалить все моды
func (app *App) removeAllMods() {
	app.modsMutex.RLock()
	var mods []string
	for _, mod := range app.allMods {
		if !mod.IsSystem {
			mods = append(mods, mod.Name)
		}
	}
	app.modsMutex.RUnlock()

	app.modsMutex.Lock()
	var kept []checks.ModInfo
	for _, m := range app.allMods {
		if m.IsSystem {
			kept = append(kept, m)
		}
	}
	app.allMods = kept
	app.displayedMods = kept
	app.modsMutex.Unlock()

	app.orderDirty.Store(false)
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
		// Используем атомарную синхронизацию
		if err := app.syncActiveProfileFromGame(); err != nil {
			app.appendLogToFile(fmt.Sprintf("removeAllMods: sync failed: %v", err))
		}
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

	// Сигнал батчу — система установила компонент.
	// Не знаем, какой именно (base/dmf/autopatch), поэтому отправляем
	// имя файла архива — runBatchRunner сверит через isSameMod.
	app.refreshAfterSystemInstall()

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

// removeModFromData удаляет мод из внутренних структур и возвращает
// индекс, на котором он находился в displayedMods.
//
// ВАЖНО: selectedModName — обычная строка, а не atomic. Менять её
// разрешено ТОЛЬКО из UI-потока (всегда под fyne.Do). Поэтому здесь
// вместо прямой записи используется fyne.Do.
func (app *App) removeModFromData(modName string) (indexInDisplayed int, found bool) {
	app.modsMutex.Lock()
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
	clearSelection := app.selectedModName == modName
	app.modsMutex.Unlock()

	if clearSelection {
		fyne.Do(func() {
			app.selectedModName = ""
			app.selectedModIndex.Store(-1)
		})
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

	// --- Этап 6: убираем двойную вложенность ---
	// Если в корне единственная папка (например, ThirdPerson) содержит
	// внутри себя папку с тем же именем и .mod файл, поднимаем
	// содержимое внутренней папки на уровень выше.
	if len(entries) == 1 && entries[0].IsDir() {
		outerDir := entries[0].Name()
		outerPath := filepath.Join(tmpDir, outerDir)
		innerEntries, err := os.ReadDir(outerPath)
		if err == nil && len(innerEntries) == 1 && innerEntries[0].IsDir() && innerEntries[0].Name() == outerDir {
			innerPath := filepath.Join(outerPath, outerDir)
			modFile := filepath.Join(innerPath, outerDir+".mod")
			// Проверяем, что внутри действительно лежит .mod файл
			if _, err := os.Stat(modFile); err == nil {
				tempPath := outerPath + ".tmp_flatten"
				if err := os.Rename(innerPath, tempPath); err == nil {
					os.RemoveAll(outerPath)
					os.Rename(tempPath, outerPath)
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

	bar, label, cancelChan, closeDialog := app.showProgressDialog(
		app.msg("update_title"),
		app.msg("collecting_mods_for_update"),
	)
	defer closeDialog()

	total := len(eligible)
	processed := 0
	var batchItems []*BatchItem

	// Собираем информацию о модах для нового окна
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

		// Запрашиваем changelog для отображения в окне
		changelog, _ := app.FetchChangelog(modID, fileInfo.ID)

		// Создаем элемент для таблицы
		item := &BatchItem{
			ID:          mod.Name,
			Name:        mod.DisplayName,
			Description: mod.Description,
			Version:     fileInfo.Version,
			NexusModID:  modID,
			FileID:      fileInfo.ID,
			Selected:    true,
			Status:      BatchPending,
			Author:      mod.Author,
			Category:    mod.Category,
			PictureURL:  mod.NexusPictureURL,
			Changelog:   changelog,
		}
		batchItems = append(batchItems, item)

		processed++
		if processed%5 == 0 || processed == total {
			fyne.Do(func() {
				bar.SetValue(float64(processed) / float64(total))
				label.SetText(fmt.Sprintf(app.msg("collecting_mods_x_y"), processed, total))
			})
		}
	}

	closeDialog()

	if len(batchItems) == 0 {
		app.appendLog(app.msg("no_mods_found_for_force_update"))
		fyne.Do(func() {
			label.SetText(app.msg("updates_found_no"))
			bar.SetValue(1.0)
		})
		time.Sleep(1 * time.Second)
		return
	}

	// Запускаем новое красивое окно пакетного обновления
	fyne.Do(func() {
		app.showBatchDialog(app.msg("batch_title_updates"), batchItems, BatchModeUpdate)
	})
}
