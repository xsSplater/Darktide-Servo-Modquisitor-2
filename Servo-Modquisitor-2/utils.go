// Servo-Modquisitor-2/utils.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"archive/zip"
	"bufio"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// checkIncompatible проверяет, является ли мод несовместимым с любым
// другим установленным модом. Делегирует в checks.IsIncompatibleMod,
// который читает список пар под checksDataMutex — без гонки.
func (app *App) checkIncompatible(name string) bool {
	return checks.IsIncompatibleMod(name)
}

type GameVersion int

const (
	VersionUnknown GameVersion = iota
	VersionSteam
	VersionXbox
)

type PatcherType int

const (
	PatcherNone PatcherType = iota
	PatcherLegacy
	PatcherAutoPatch
)

var (
	errGameVersionUnknown  string
	errDarktideExeNotFound string
	errGameRootNotFound    string
	errWineNotFound        string
	errXboxOnLinux         string
)

func SetLauncherMessages(verUnknown, exeNotFound, rootNotFound string) {
	errGameVersionUnknown = verUnknown
	errDarktideExeNotFound = exeNotFound
	errGameRootNotFound = rootNotFound
}

func SetLinuxLauncherMessages(wineNotFound, xboxOnLinux string) {
	errWineNotFound = wineNotFound
	errXboxOnLinux = xboxOnLinux
}

func detectGameVersion(gameRoot string) GameVersion {
	if _, err := os.Stat(filepath.Join(gameRoot, "content")); err == nil {
		return VersionXbox
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "binaries")); err == nil {
		return VersionSteam
	}
	return VersionUnknown
}

func (app *App) makeCRTGradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	centerX, centerY := float64(w)/2, float64(h)/2
	maxDist := math.Sqrt(centerX*centerX + centerY*centerY)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)-centerX, float64(y)-centerY
			dist := math.Sqrt(dx*dx+dy*dy) / maxDist
			t := math.Pow(dist, 1.5)
			g := uint8(200 - 180*t)
			b := uint8(30 - 25*t)
			if g < 20 {
				g = 20
			}
			if b < 5 {
				b = 5
			}
			img.Set(x, y, color.NRGBA{R: 0, G: g, B: b, A: 180})
		}
	}
	return img
}

func (app *App) makeRedCRTGradient(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	centerX, centerY := float64(w)/2, float64(h)/2
	maxDist := math.Sqrt(centerX*centerX + centerY*centerY)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x)-centerX, float64(y)-centerY
			dist := math.Sqrt(dx*dx+dy*dy) / maxDist
			t := math.Pow(dist, 1.5)
			r := uint8(220 - 180*t)
			g := uint8(30 - 25*t)
			b := uint8(20 - 15*t)
			if r < 40 {
				r = 40
			}
			if g < 5 {
				g = 5
			}
			if b < 5 {
				b = 5
			}
			img.Set(x, y, color.NRGBA{R: r, G: g, B: b, A: 180})
		}
	}
	return img
}

func (app *App) runAllChecks() {
	app.appendLogToFile("// " + app.msg("log_start"))

	checks.CheckInstallation(app.mainWindow)

	checks.EnsureModLoadOrder(app.mainWindow)

	if !checks.CheckObsoleteMods(app.mainWindow) {
		return
	}

	if !checks.CheckMalformed(app.mainWindow) {
		return
	}

	if !checks.CheckEmptyFolders(app.mainWindow) {
		return
	}

	if !checks.CheckIncompatible(app.mainWindow) {
		return
	}

	if !checks.CheckDependencies(app.mainWindow) {
		return
	}

	if !checks.CheckBrokenMods(app.mainWindow) {
		return
	}

	// 1. Перечитываем самый свежий сохранённый файл
	fyne.Do(func() {
		app.refreshModList()
	})

	// 2. Собираем активные моды для сортировки
	// Защищённое чтение allMods и cfg.Language
	app.modsMutex.RLock()
	activeNames := []string{}
	notActive := make(map[string]bool)
	for _, mod := range app.allMods {
		if mod.Active && checks.FolderExists(mod.Name) {
			activeNames = append(activeNames, mod.Name)
		} else if checks.FolderExists(mod.Name) && !mod.IsSystem {
			notActive[mod.Name] = true
		}
	}
	app.modsMutex.RUnlock()

	// Если активных модов нет - просто завершаем
	if len(activeNames) == 0 {
		app.appendLog(app.msg("done"))
		return
	}

	// Копируем язык под мьютексом
	app.cfgMutex.RLock()
	lang := app.cfg.Language
	app.cfgMutex.RUnlock()

	// Весь блок записи файла — под loadOrderMutex. Иначе Ctrl+S
	// во время runAllChecks перезапишет файл in-memory порядком,
	// который устарел уже через миллисекунду, и syncLoadOrderToGame
	// утащит в игру этот устаревший вариант.
	app.loadOrderMutex.Lock()

	// Создаём порядок в папке профиля (путь уже установлен через updateSorterOutputPath)
	sorter.CreateLoadOrderFromActive(activeNames, lang)

	// Дописываем неактивные моды в файл профиля
	profileOrderPath := filepath.Join(app.activeProfilePath(), "mods", FileNameLoadOrder)
	f, err := os.OpenFile(profileOrderPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		existing := make(map[string]bool)
		data, _ := os.ReadFile(profileOrderPath)
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "--") {
				existing[line] = true
			}
		}
		for name := range notActive {
			if !existing[name] {
				fmt.Fprintln(f, "-- "+name)
			}
		}
		f.Close()
	} else {
		app.appendLogToFile(fmt.Sprintf("Failed to open profile load order for append: %v", err))
	}

	// Копируем итоговый файл из профиля в игровую папку (чтобы игра его видела)
	app.syncLoadOrderToGameLocked()

	app.loadOrderMutex.Unlock()

	// Логируем финальный порядок (для отладки) - читаем ModsPath под мьютексом
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	data, err := os.ReadFile(filepath.Join(modsPath, FileNameLoadOrder))
	if err == nil {
		app.appendLogToFile("=== Final load order after sorting ===")
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := scanner.Text()
			app.appendLogToFile(line)
		}
		app.appendLogToFile("=== End of load order ===")
	} else {
		app.appendLogToFile(fmt.Sprintf(app.msg("log_failed_to_read_lo"), err))
	}
	app.appendLog(app.msg("done"))

	// Тихое обновление данных без изменения выделения и прокрутки
	savedMod := app.selectedModName

	// Финальное обновление UI в зависимости от настройки
	// Чтение настройки под мьютексом
	app.cfgMutex.RLock()
	showAfterSort := app.cfg.ShowModListAfterSort
	app.cfgMutex.RUnlock()

	fyne.Do(func() {
		app.refreshModList()
		app.forceRefreshTable()

		if showAfterSort {
			// Открыть файл (как было)
			app.cfgMutex.RLock()
			absPath, _ := filepath.Abs(filepath.Join(app.cfg.ModsPath, FileNameLoadOrder))
			app.cfgMutex.RUnlock()
			if _, err := os.Stat(absPath); err == nil {
				go func() {
					if err := openFileWithDefaultApp(absPath); err != nil {
						app.appendLogToFile(fmt.Sprintf(app.msg("log_failed_open_file"), err))
					}
				}()
			}
		}

		// Восстанавливаем выделение в любом случае
		app.restoreSelectedMod(savedMod)
	})
}

func (app *App) forceRefreshTable() {
	if app.modTable == nil {
		app.appendLogToFile("forceRefreshTable: modTable is nil, skipping")
		return
	}
	if app.mainWindow == nil || app.mainWindow.Canvas() == nil {
		app.appendLogToFile("forceRefreshTable: mainWindow or Canvas is nil, skipping")
		return
	}
	app.modTable.Refresh()
	if app.tableBorderContainer != nil {
		app.tableBorderContainer.Refresh()
	}
	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	app.mainWindow.Canvas().Refresh(app.modTable)
}

// restoreSelectedMod после обновления списка пытается выделить и прокрутить к ранее выбранному моду
func (app *App) restoreSelectedMod(savedModName string) {
	if savedModName == "" {
		return
	}
	// Даём время таблице перестроиться после refreshModList()
	time.AfterFunc(50*time.Millisecond, func() {
		fyne.Do(func() {
			for i, m := range app.displayedMods {
				if m.Name == savedModName {
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
					app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
					// updateDescriptionForMod вызовется автоматически через OnSelected
					return
				}
			}
			// Если мод не найден (например, был удалён), очищаем выделение
			app.selectedModName = ""
			app.selectedModIndex.Store(-1)
			app.updateDescriptionForMod("")
			app.updateUpDownButtons()
		})
	})
}

// appendLogToFile пишет сообщение только в файл лога (на английском), без вывода в консоль
func (app *App) appendLogToFile(msg string) {
	if app.logFile == nil {
		return
	}
	fmt.Fprintln(app.logFile, time.Now().Format(LogTimeFormat), msg)
}

// extractPatternFromFilename извлекает первое слово из имени файла (без расширения) и приводит к нижнему регистру.
// Используется для генерации nexus_file_pattern.
func extractPatternFromFilename(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	if ext != "" {
		base = base[:len(base)-len(ext)]
	}
	// Если внутри base осталось что-то вроде ".zip-", удаляем это расширение
	// Простой вариант: убрать известные расширения архивов внутри строки
	for _, archiveExt := range []string{".zip", ".rar", ".7z"} {
		if idx := strings.Index(base, archiveExt+"-"); idx != -1 {
			base = base[:idx] + base[idx+len(archiveExt):]
			break
		}
	}
	parts := strings.Fields(base)
	if len(parts) == 0 {
		return ""
	}
	word := parts[0]

	// Проверяем, есть ли в слове разделитель, за которым следует версия
	for _, sep := range []string{"_", "-"} {
		if idx := strings.Index(word, sep); idx != -1 {
			suffix := word[idx+1:]
			if strings.ContainsAny(suffix, "0123456789") && (strings.Contains(suffix, ".") || len(suffix) > 0) {
				word = word[:idx]
				break
			}
		}
	}
	return strings.ToLower(word)
}

// copyFile копирует файл src в dst, перезаписывая существующий.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// logNexusError логирует ошибку работы с Nexus.
// Для 403 (Mod not available) выводит понятное сообщение.
// Если передан customMsg, используется оно вместо стандартного.
func (app *App) logNexusError(err error, modName string, customMsg ...string) {
	if err == nil {
		return
	}
	errMsg := err.Error()
	if strings.Contains(errMsg, "403") && strings.Contains(errMsg, "Mod not available") {
		if len(customMsg) > 0 && customMsg[0] != "" {
			app.appendLog(customMsg[0])
		} else {
			app.appendLog(fmt.Sprintf(app.msg("log_error_mod_not_available"), modName))
		}
	} else {
		app.appendLogToFile(fmt.Sprintf("Failed to process %s: %v", modName, err))
	}
}

// isSymlinkFolder проверяет, является ли папка мода симлинком.
func (app *App) isSymlinkFolder(modName string) bool {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	modPath := filepath.Join(modsPath, modName)
	info, err := os.Lstat(modPath)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func (app *App) fixHubHotkeyMenus() {
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	wrongFolder := filepath.Join(modsPath, "hub_hotkey_menus-main")
	correctFolder := filepath.Join(modsPath, "hub_hotkey_menus")
	if info, err := os.Stat(wrongFolder); err == nil && info.IsDir() {
		if _, err := os.Stat(correctFolder); os.IsNotExist(err) {
			if err := os.Rename(wrongFolder, correctFolder); err == nil {
				app.appendLogToFile(app.msg("log_fix_hub_hk_menus"))
			} else {
				app.appendLogToFile(fmt.Sprintf(app.msg("log_failed_fix_hub_hk_menus"), err))
			}
		}
	}
}

func extractModIDFromKey(key string) int {
	parts := strings.SplitN(key, ":", 2)
	if len(parts) == 2 {
		id, err := strconv.Atoi(parts[0])
		if err == nil {
			return id
		}
	}
	return 0
}

// detectPatcherTypeWithRoot определяет тип патчера по переданному корню игры.
func detectPatcherTypeWithRoot(gameRoot string) PatcherType {
	if gameRoot == "" {
		return PatcherNone
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "toggle_dt_mod_autopatch.cmd")); err == nil {
		return PatcherAutoPatch
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "binaries", "plugins", "_dt_mod_autopatch.dll")); err == nil {
		return PatcherAutoPatch
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "tools", "dtkit-patch")); err == nil {
		return PatcherLegacy
	}
	if _, err := os.Stat(filepath.Join(gameRoot, "toggle_darktide_mods.bat")); err == nil {
		return PatcherLegacy
	}
	return PatcherNone
}

// isModsEnabledAutoPatch теперь принимает gameRoot.
func isModsEnabledAutoPatch(gameRoot string) bool {
	if gameRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(gameRoot, "DISABLE_AUTOPATCHER"))
	return os.IsNotExist(err)
}

// toggleModsAutoPatch теперь принимает gameRoot.
func toggleModsAutoPatch(gameRoot string) error {
	if gameRoot == "" {
		return fmt.Errorf("game root not found")
	}
	if isModsEnabledAutoPatch(gameRoot) {
		f, _ := os.Create(filepath.Join(gameRoot, "DISABLE_AUTOPATCHER"))
		if f != nil {
			f.Close()
		}
		bak := filepath.Join(gameRoot, "bundle", "bundle_database.data.bak")
		original := filepath.Join(gameRoot, "bundle", "bundle_database.data")
		if _, err := os.Stat(bak); err == nil {
			os.Rename(bak, original)
		}
	} else {
		os.Remove(filepath.Join(gameRoot, "DISABLE_AUTOPATCHER"))
	}
	return nil
}

func toggleModsLegacy(gameRoot string) error {
	if gameRoot == "" {
		return fmt.Errorf("%s", errGameRootNotFound)
	}
	bat := filepath.Join(gameRoot, "toggle_darktide_mods.bat")
	if _, err := os.Stat(bat); err != nil {
		dtkit := filepath.Join(gameRoot, "tools", "dtkit-patch")
		if _, err := os.Stat(dtkit); err == nil {
			cmd := exec.Command(dtkit, "--toggle", "bundle")
			return cmd.Run()
		}
		return fmt.Errorf("no supported patcher found")
	}
	cmd := exec.Command(bat)
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// closeApp безопасно закрывает главное окно и завершает программу.
func (app *App) closeApp() {
	app.nxm.Stop()
	app.mainWindow.Close()
	os.Exit(0)
}

// sanitizeFilename проверяет, что имя файла безопасно для сохранения в папку mods.
// Возвращает очищенное имя или ошибку.
func sanitizeFilename(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("empty filename")
	}
	// filepath.Base удаляет все разделители и приводит к простому имени
	base := filepath.Base(filename)
	if base == "." || base == ".." {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}
	// Запрещаем абсолютные пути (filepath.Base уже убрал разделители, но проверим на всякий случай)
	if filepath.IsAbs(base) {
		return "", fmt.Errorf("absolute path not allowed: %s", filename)
	}
	// Проверяем, что в имени нет запрещённых символов (дополнительная защита)
	if strings.ContainsAny(base, "/\\") {
		return "", fmt.Errorf("filename contains path separators: %s", filename)
	}
	return base, nil
}

// Управление настройками игры
func (app *App) createSettingsBackup() {
	settingsPath := app.getUserSettingsPath()
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		app.showInfoDialog(app.msg("error_title"), app.msg("settings_file_not_found"))
		return
	}

	// Путь к папке конфигурации программы (там же, где config.json)
	configDir := filepath.Dir(configFilePath())
	backupDir := filepath.Join(configDir, "backups", "user_settings")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		app.appendLogToFile(fmt.Sprintf("Failed to create backup directory: %v", err))
		return
	}

	timestamp := time.Now().Format("20060102_150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("user_settings_%s.config", timestamp))

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to read settings: %v", err))
		return
	}
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to write backup: %v", err))
		return
	}
	app.appendLogToFile(fmt.Sprintf("Backup created: %s", backupPath))
}

// getUserSettingsPath возвращает путь к файлу настроек игры
func (app *App) getUserSettingsPath() string {
	// Для Windows Steam
	if appData := os.Getenv("APPDATA"); appData != "" {
		path := filepath.Join(appData, "Fatshark", "Darktide", "user_settings.config")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// Для Linux (Wine) — возможный путь
	if home := os.Getenv("HOME"); home != "" {
		path := filepath.Join(home, ".config", "Fatshark", "Darktide", "user_settings.config")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	// fallback: рядом с mods (если не нашли в стандартных местах)
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	return filepath.Join(modsPath, "..", "user_settings.config")
}

// getProgramTempDir возвращает путь к временной папке внутри директории программы.
func (app *App) getProgramTempDir() string {
	exe, err := os.Executable()
	if err != nil {
		return os.TempDir()
	}
	dir := filepath.Dir(exe)
	tempDir := filepath.Join(dir, "temp_extract")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return os.TempDir()
	}
	return tempDir
}

// cleanProgramTempDirs удаляет временные папки, созданные программой.
func cleanProgramTempDirs() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dir := filepath.Dir(exe)
	tempDir := filepath.Join(dir, "temp_extract")
	if _, err := os.Stat(tempDir); err == nil {
		_ = os.RemoveAll(tempDir)
	}
}

// ensureDir создаёт папку по указанному пути, удаляя или переименовывая файлы, мешающие созданию папок.
// Защищает .mod файлы от удаления. Добавлены повторные попытки при блокировке.
func (app *App) ensureDir(path string) error {
	// Проверяем, существует ли путь
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return nil
	}
	if err == nil && !info.IsDir() {
		// Это файл, мешающий созданию папки
		if strings.HasSuffix(strings.ToLower(path), ".mod") {
			return fmt.Errorf("cannot remove .mod file %s", path)
		}
		// Пытаемся удалить или переименовать файл с повторными попытками
		for attempt := 0; attempt < 5; attempt++ {
			// Пытаемся удалить
			if removeErr := os.Remove(path); removeErr == nil {
				break // успешно удалили
			} else if attempt == 4 {
				// Последняя попытка: пытаемся переименовать
				newPath := path + ".old"
				if renameErr := os.Rename(path, newPath); renameErr != nil {
					return fmt.Errorf("cannot remove or rename file %s: %w", path, renameErr)
				}
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	// Рекурсивно обрабатываем родителя
	parent := filepath.Dir(path)
	if parent != path {
		if err := app.ensureDir(parent); err != nil {
			return err
		}
	}
	return os.MkdirAll(path, 0755)
}

// isModsEnabledLegacy определяет, включены ли моды для Legacy-патчера (toggle_darktide_mods.bat).
// Возвращает true, если моды включены (бэкап-файл отсутствует).
func isModsEnabledLegacy(gameRoot string) bool {
	if gameRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(gameRoot, "bundle", "bundle_database.data.bak"))
	return err == nil // true если бэкап есть (моды включены)
}

// removeModFromCache удаляет запись о моде из кэша версий активного
// профиля и сохраняет обновлённый файл.
func (app *App) removeModFromCache(modName string) {
	if !app.versionCache.RemoveByFolder(modName) {
		return // нет записи в кэше
	}
	app.saveNexusVersionCache()
	app.appendLogToFile(fmt.Sprintf("Removed cache entry for mod: %s", modName))
}

// pruneVersionCache удаляет мёртвые и дублирующиеся записи из кэша
// версий. Логика вынесена в VersionCache.Prune.
func (app *App) pruneVersionCache() {
	// 1. Собираем существующие папки.
	app.modsMutex.RLock()
	existing := make(map[string]bool)
	for _, mod := range app.allMods {
		existing[mod.Name] = true
	}
	for _, mod := range app.systemMods {
		existing[mod.Name] = true
	}
	app.modsMutex.RUnlock()

	// 2. Prune.
	removed := app.versionCache.Prune(existing)

	// 3. Сохраняем только если что-то удалилось.
	if removed > 0 {
		app.saveNexusVersionCache()
		app.appendLogToFile(fmt.Sprintf("Pruned %d duplicate/stale entries from version cache.", removed))
	}
}

// createZipArchive создаёт zip-архив из папки src в файл dst.
func (app *App) createZipArchive(src, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	zipWriter := zip.NewWriter(f)
	defer zipWriter.Close()

	baseDir := filepath.Base(src)
	err = filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		zipPath := filepath.Join(baseDir, relPath)
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(zipPath)
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})
	return err
}

// extractZipArchive распаковывает zip-архив в папку dst с защитой от path traversal.
func (app *App) extractZipArchive(src, dst string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// 1. Запрещаем абсолютные пути и обход каталогов
		if filepath.IsAbs(f.Name) || strings.Contains(f.Name, "..") {
			return fmt.Errorf("invalid path in archive: %s", f.Name)
		}

		// 2. Строим безопасный путь внутри dst
		targetPath := filepath.Join(dst, f.Name)

		// 3. Дополнительная проверка: убеждаемся, что путь действительно внутри dst
		rel, err := filepath.Rel(dst, targetPath)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("path traversal attempt: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		out, err := os.Create(targetPath)
		if err != nil {
			return err
		}
		defer out.Close()

		if _, err := io.Copy(out, rc); err != nil {
			return err
		}
	}
	return nil
}

// isVersionToken проверяет, что строка выглядит как часть версии (содержит точку или букву, не является датой).
func isVersionToken(s string) bool {
	if s == "" {
		return false
	}
	// Если это дата с T или дефисами (YYYY-MM-DD), не считаем версией
	if strings.Contains(s, "T") || strings.Contains(s, "-") && len(s) >= 8 && isNumeric(strings.ReplaceAll(s, "-", "")) {
		return false
	}
	// Содержит точку и состоит из цифр, букв и точек
	if strings.Contains(s, ".") {
		for _, ch := range s {
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '.') {
				return false
			}
		}
		return true
	}
	// Просто число - тоже может быть версией (например, "1")
	if isNumeric(s) {
		return true
	}
	return false
}

// isDateToken проверяет, является ли токен датой (8 цифр или YYYY-MM-DD или содержит T)
func isDateToken(s string) bool {
	if strings.Contains(s, "T") {
		return true
	}
	if len(s) == 8 && isNumeric(s) {
		return true
	}
	if len(s) == 10 && strings.Contains(s, "-") {
		parts := strings.Split(s, "-")
		if len(parts) == 3 && len(parts[0]) == 4 && len(parts[1]) == 2 && len(parts[2]) == 2 &&
			isNumeric(parts[0]) && isNumeric(parts[1]) && isNumeric(parts[2]) {
			return true
		}
	}
	return false
}

func extractVersionAndModIDFromFilename(filename string) (modID int, version string, ok bool) {
	name := filepath.Base(filename)
	ext := filepath.Ext(name)
	if ext != "" && !strings.ContainsAny(ext, " \t") {
		// Проверяем, что расширение содержит хотя бы одну букву
		hasLetter := false
		for _, ch := range ext[1:] { // пропускаем точку
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				hasLetter = true
				break
			}
		}
		if hasLetter {
			name = name[:len(name)-len(ext)]
		}
	}

	// Пробельный формат (пробуем сначала)
	parts := strings.Fields(name)
	var idIdx = -1
	for i, p := range parts {
		if isNumeric(p) {
			id, err := strconv.Atoi(p)
			if err == nil && id > 0 && id < MaxModsID_less {
				idIdx = i
				break
			}
		}
	}
	if idIdx != -1 {
		modID, _ = strconv.Atoi(parts[idIdx])

		// Если после ID нет токенов → версия отсутствует
		if idIdx+1 >= len(parts) {
			return 0, "", false
		}

		// Собираем токены после ID, пока они являются версионными
		var verParts []string
		for i := idIdx + 1; i < len(parts); i++ {
			token := parts[i]
			if isDateToken(token) {
				break
			}
			if isVersionToken(token) {
				verParts = append(verParts, token)
			} else {
				break
			}
		}

		if len(verParts) > 0 {
			version = strings.Join(verParts, ".")
			ok = true
			return
		}

		// Если первый токен после ID — дата → версия неизвестна
		if idIdx+1 < len(parts) && isDateToken(parts[idIdx+1]) {
			return modID, "unknown", true
		}

		// В остальных случаях версия неизвестна
		return modID, "unknown", true
	}

	// Дефисный формат (если пробельный не сработал)
	if strings.Contains(name, "-") {
		parts := strings.Split(name, "-")
		var idIdx, tsIdx = -1, -1
		for i, p := range parts {
			if isNumeric(p) {
				id, err := strconv.Atoi(p)
				if err == nil && id > 0 && id < MaxModsID_less {
					idIdx = i
				}
			}
			if len(p) == 10 && isNumeric(p) {
				tsIdx = i
			}
		}
		if idIdx != -1 && tsIdx != -1 && idIdx < tsIdx {
			modID, _ = strconv.Atoi(parts[idIdx])
			var verParts []string
			for i := idIdx + 1; i < tsIdx; i++ {
				verParts = append(verParts, parts[i])
			}
			if len(verParts) > 0 {
				version = strings.Join(verParts, ".")
				ok = true
				return
			}
			return modID, "unknown", true
		}
		if idIdx != -1 {
			modID, _ = strconv.Atoi(parts[idIdx])
			var verParts []string
			for i := idIdx + 1; i < len(parts); i++ {
				verParts = append(verParts, parts[i])
			}
			if len(verParts) > 0 {
				version = strings.Join(verParts, ".")
				ok = true
				return
			}
			return modID, "unknown", true
		}
	}

	return 0, "", false
}

// compareVersions сравнивает две семантические версии (например, "1.9.0" и "1.9.5").
func compareVersions(v1, v2 string) int {
	if v1 == "" || v2 == "" {
		return 0
	}
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	for i := 0; i < maxLen; i++ {
		n1, _ := strconv.Atoi(getPart(parts1, i))
		n2, _ := strconv.Atoi(getPart(parts2, i))
		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}
	return 0
}

func getPart(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return "0"
}
