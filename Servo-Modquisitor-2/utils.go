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
	"runtime"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	golocale "github.com/jeandeaual/go-locale"
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

// PatcherType — состояние патчера игры. Реализация одна: SMQ сам патчит
// bundle_database.data (см. bundle_patch.go). PatcherLegacy убран:
// логика dtkit-patch теперь встроена в Go.
type PatcherType int

const (
	PatcherNone PatcherType = iota
	PatcherAutoPatch
)

var (
	errGameVersionUnknown  string
	errDarktideExeNotFound string
	errWineNotFound        string
	errXboxOnLinux         string
)

func SetLauncherMessages(verUnknown, exeNotFound, rootNotFound string) {
	errGameVersionUnknown = verUnknown
	errDarktideExeNotFound = exeNotFound
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
	if app.mainWindow == nil || app.mainWindow.Canvas() == nil {
		app.appendLogToFile("runAllChecks: mainWindow not ready, aborting")
		return
	}
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

	app.appendLog(app.msg("log_create_mlot"))
	app.appendLog(app.msg("log_mlot_created"))

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

	// Тихое обновление данных без изменения выделения и прокрутки.
	// runAllChecks крутится в фоновой горутине, а selectedModName
	// пишется в UI-потоке — берём снимок через fyne.Do.
	var savedMod string
	done := make(chan struct{})
	fyne.Do(func() {
		savedMod = app.selectedModName
		close(done)
	})
	<-done

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
	app.logFileMu.Lock()
	defer app.logFileMu.Unlock()
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

// detectPatcherTypeWithRoot определяет, есть ли у игры bundle-каталог
// с bundle_database.data — то есть можем ли мы её патчить.
//
// Раньше функция искала файлы сторонних патчеров (toggle_*.cmd,
// _dt_mod_autopatch.dll, dtkit-patch). Это было хрупко: пользователь
// мог удалить эти файлы, а патчер (DLL) оставался живым — SMQ терял
// способность понять, что игра вообще модифицируема.
//
// Теперь SMQ сам себе патчер (bundle_patch.go), и наличие
// bundle_database.data — единственное, что важно.
func detectPatcherTypeWithRoot(gameRoot string) PatcherType {
	if gameRoot == "" {
		return PatcherNone
	}
	dbPath := filepath.Join(gameRoot, "bundle", "bundle_database.data")
	if _, err := os.Stat(dbPath); err != nil {
		return PatcherNone
	}
	return PatcherAutoPatch
}

// isModsEnabledAutoPatch сообщает, включены ли моды.
//
// Источник истины зависит от наличия _dt_mod_autopatch.dll:
//   - DLL установлена  → флаг mods/DISABLE_AUTOPATCHER (его читает DLL);
//   - DLL не установлена → тег .patch_999 в bundle_database.data.
//
// Оба варианта собраны в IsModsEnabled (bundle_patch.go) — единая точка
// правды, чтобы UI и логика Toggle не разъезжались.
func isModsEnabledAutoPatch(gameRoot string) bool {
	return IsModsEnabled(gameRoot)
}

// closeApp безопасно закрывает главное окно и завершает программу.
// Вызывается и из UI (меню, OnClosed), и из фоновых горутин
// (initializePaths, setGamePaths) — поэтому Window.Close() обязан идти
// через fyne.Do.
//
// os.Exit(0) — после Close. Окно всё равно уничтожается драйвером
// асинхронно; для выхода процесса важен только Close(), который
// гарантирует корректное освобождение порта NXM.
func (app *App) closeApp() {
	app.nxm.Stop()
	if app.mainWindow != nil {
		fyne.Do(func() {
			app.mainWindow.Close()
		})
	}
	os.Exit(0)
}

// sanitizeFilename проверяет, что имя файла безопасно для сохранения в папку mods.
// Возвращает очищенное имя или ошибку.
var windowsReservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true,
	"COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true,
	"LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

func sanitizeFilename(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("empty filename")
	}
	normalized := strings.ReplaceAll(filename, "\\", "/")
	base := filepath.Base(normalized)

	if base == "." || base == ".." {
		return "", fmt.Errorf("invalid filename: %s", filename)
	}
	if filepath.IsAbs(base) {
		return "", fmt.Errorf("absolute path not allowed: %s", filename)
	}
	if strings.ContainsAny(base, "/\\") {
		return "", fmt.Errorf("filename contains path separators: %s", filename)
	}

	// Windows: имена с хвостовой точкой или пробелом молча обрезаются
	base = strings.TrimRight(base, ". ")
	if base == "" {
		return "", fmt.Errorf("filename becomes empty after trimming: %s", filename)
	}

	// Windows: зарезервированные имена (CON, NUL, COM1…)
	stem := strings.ToUpper(strings.TrimSuffix(base, filepath.Ext(base)))
	if windowsReservedNames[stem] {
		return "", fmt.Errorf("filename uses reserved Windows name: %s", filename)
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

// getUserSettingsPath возвращает путь к файлу настроек игры.
// Порядок поиска:
//  1. Windows: %APPDATA%\Fatshark\Darktide\user_settings.config
//     (работает и для Steam, и для Xbox — у обоих один и тот же путь).
//  2. Linux Proton: <steamapps>/compatdata/1361210/pfx/drive_c/users/steamuser/...
//     Перебираются все Steam-библиотеки (native, Flatpak, Snap, доп. диски).
//  3. Linux Wine (legacy): $HOME/.config/Fatshark/Darktide/user_settings.config.
//  4. Fallback: рядом с mods (если игра установлена рядом с программой).
func (app *App) getUserSettingsPath() string {
	// 1. Windows
	if appData := os.Getenv("APPDATA"); appData != "" {
		path := filepath.Join(appData, "Fatshark", "Darktide", "user_settings.config")
		if fileExists(path) {
			app.appendLogToFile(fmt.Sprintf("user_settings.config found (Windows): %s", path))
			return path
		}
	}

	// 2. Linux Proton
	if p := findProtonSettingsPath(); p != "" {
		app.appendLogToFile(fmt.Sprintf("user_settings.config found (Proton): %s", p))
		return p
	}

	// 3. Linux Wine legacy
	if home := os.Getenv("HOME"); home != "" {
		path := filepath.Join(home, ".config", "Fatshark", "Darktide", "user_settings.config")
		if fileExists(path) {
			app.appendLogToFile(fmt.Sprintf("user_settings.config found (Wine legacy): %s", path))
			return path
		}
	}

	// 4. Fallback — путь, которого, скорее всего, нет. Возвращаем
	// его для консистентности: вызывающий код покажет «файл не найден».
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	fallback := filepath.Join(modsPath, "..", "user_settings.config")
	app.appendLogToFile(fmt.Sprintf("user_settings.config not found, fallback: %s", fallback))
	return fallback
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
	return app.extractZipWithSTARRY(src, dst)
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
	if ext != "" {
		// Расширение файла — только если после точки есть хотя бы одна буква
		// или если это известное расширение (.zip, .rar, .7z, .mod).
		extLower := strings.ToLower(ext)
		knownExt := extLower == ".zip" || extLower == ".rar" ||
			extLower == ".7z" || extLower == ".mod"
		if knownExt {
			name = strings.TrimSuffix(name, ext)
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
	// Отделяем pre-release: "1.0.0-rc1" → "1.0.0" + "rc1"
	main1, pre1 := splitPrerelease(v1)
	main2, pre2 := splitPrerelease(v2)

	parts1 := strings.Split(main1, ".")
	parts2 := strings.Split(main2, ".")
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
	// Основные версии равны: pre-release < релиза
	if pre1 == "" && pre2 == "" {
		return 0
	}
	if pre1 == "" {
		return 1 // v1 — релиз, v2 — pre-release
	}
	if pre2 == "" {
		return -1
	}
	return strings.Compare(pre1, pre2)
}

func splitPrerelease(v string) (main, pre string) {
	for _, sep := range []string{"-", "+"} {
		if idx := strings.Index(v, sep); idx != -1 {
			return v[:idx], v[idx+1:]
		}
	}
	return v, ""
}

func getPart(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}
	return "0"
}

// / systemLocale возвращает предпочтительную системную локаль в формате
// BCP 47 ("ru-RU", "en-US", "zh-Hans-CN"). Пустая строка — если
// определить не удалось или библиотека вернула ошибку.
//
// go-locale — тот же пакет, что использует Fyne внутри updateLocalizer.
// Отдельный вызов безопасен: он просто читает env/системные API, ничего
// не кэширует между вызовами.
func systemLocale() string {
	locales, err := golocale.GetLocales()
	if err != nil || len(locales) == 0 {
		return ""
	}
	return locales[0]
}

// mapSystemLocaleToSupported пытается сопоставить системную локаль
// (например, "ru-RU", "en_US", "zh-Hans-CN") с одним из кодов,
// поддерживаемых приложением. Возвращает "" если совпадения нет —
// вызывающий оставит текущий выбор.
//
// Формат входа: BCP 47 ("ru-RU") или POSIX ("ru_RU"). Оба варианта
// нормализуются к нижнему регистру с дефисом.
func mapSystemLocaleToSupported(locale string) string {
	if locale == "" {
		return ""
	}
	s := strings.ToLower(locale)
	s = strings.ReplaceAll(s, "_", "-")

	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return ""
	}
	base := parts[0]

	// Особые случаи: регион/скрипт значим.
	switch base {
	case "pt":
		// У нас только pt-BR. pt-PT тоже мапим на pt-BR — лучше, чем
		// ничего, раз всё равно перевод близкий.
		return "pt-BR"
	case "zh":
		if len(parts) > 1 {
			switch parts[1] {
			case "hans", "cn", "sg":
				return "zh-hans"
			case "hant", "tw", "hk", "mo":
				return "zh-hant"
			}
		}
		return "zh-hans" // fallback
	}

	// Простые двухбуквенные коды — совпадают с нашими 1:1.
	switch base {
	case "en", "ru", "de", "es", "fr", "it", "ja", "ko", "pl":
		return base
	}
	return ""
}

// waitFor периодически опрашивает cond до true или таймаута.
// Возвращает true, если cond стал true, false если таймаут.
func waitFor(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return cond()
}

// openPathInFileManager открывает папку в системном файловом менеджере.
//
// На Linux одного xdg-open недостаточно: он может не найти обработчик
// (минимальные DE, отсутствие файлового менеджера, поломанный
// mimeapps.list) и молча упасть — тогда пользователь видит «ничего не
// происходит» без единой строчки в логе. Перебираем несколько
// бэкендов и логируем каждый провал, чтобы при разборе проблем было
// видно, что именно не сработало.
func (app *App) openPathInFileManager(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		app.appendLogToFile(fmt.Sprintf(
			"openPathInFileManager: path not found %q: %v", path, err))
		return
	}

	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("explorer", path)
		if err := cmd.Start(); err != nil {
			app.appendLogToFile(fmt.Sprintf(
				"explorer failed for %q: %v", path, err))
		}
		return

	case "linux":
		candidates := [][]string{
			{"xdg-open", path},
			{"gio", "open", path},
			{"kde-open5", path},
			{"kde-open", path},
			{"exo-open", path},
			{"nautilus", path},
			{"dolphin", path},
			{"thunar", path},
			{"pcmanfm", path},
		}
		for _, c := range candidates {
			bin, err := exec.LookPath(c[0])
			if err != nil {
				continue // утилита не установлена, пробуем следующую
			}
			cmd := exec.Command(bin, c[1:]...)
			if err := cmd.Start(); err != nil {
				app.appendLogToFile(fmt.Sprintf(
					"%s failed for %q: %v", c[0], path, err))
				continue
			}
			// Start() вернул nil — exec прошёл. Не ждём Wait:
			// xdg-open / gio отдают процесс менеджеру и завершаются сами.
			app.appendLogToFile(fmt.Sprintf("Opened %q via %s", path, c[0]))

			// Фоновый Wait, чтобы собрать зомби и увидеть ненулевой exit.
			go func(name string, cmd *exec.Cmd) {
				if err := cmd.Wait(); err != nil {
					app.appendLogToFile(fmt.Sprintf(
						"%s exited for %q: %v", name, path, err))
				}
			}(c[0], cmd)
			return
		}
		app.appendLog(fmt.Sprintf(
			"Could not open %q: no file manager backend found", path))

	default:
		// darwin и прочее — не целевая платформа, но пусть будет.
		if err := exec.Command("open", path).Start(); err != nil {
			app.appendLogToFile(fmt.Sprintf("open failed for %q: %v", path, err))
		}
	}
}
