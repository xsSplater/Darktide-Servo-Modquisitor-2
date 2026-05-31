// utils.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/sorter"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

func (app *App) containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (app *App) checkIncompatible(name string) bool {
	for _, pair := range checks.IncompatiblePairs {
		if (pair.Mod1 == name || pair.Mod2 == name) &&
			checks.FolderExists(pair.Mod1) && checks.FolderExists(pair.Mod2) {
			return true
		}
	}
	return false
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

func getGameRoot() string {
	exePath, _ := os.Executable()
	dir := filepath.Dir(exePath) // папка, где лежит exe (mods)
	for {
		if detectGameVersion(dir) != VersionUnknown {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// Патчеры toggle_darktide_mods.bat и toggle_dt_mod_autopatch.cmd
func detectPatcherType() PatcherType {
	gameRoot := getGameRoot()
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
		return PatcherLegacy // старый патчер
	}
	return PatcherNone
}

func isModsEnabledAutoPatch() bool {
	gameRoot := getGameRoot()
	if gameRoot == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(gameRoot, "DISABLE_AUTOPATCHER"))
	return os.IsNotExist(err)
}

func toggleModsAutoPatch() error {
	gameRoot := getGameRoot()
	if gameRoot == "" {
		return fmt.Errorf("game root not found")
	}
	if isModsEnabledAutoPatch() {
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

func toggleModsLegacy() error {
	gameRoot := getGameRoot()
	if gameRoot == "" {
		return fmt.Errorf("%s", errGameRootNotFound)
	}
	bat := filepath.Join(gameRoot, "toggle_darktide_mods.bat")
	if _, err := os.Stat(bat); err != nil {
		// если bat нет, пробуем dtkit-patch
		dtkit := filepath.Join(gameRoot, "tools", "dtkit-patch")
		if _, err := os.Stat(dtkit); err == nil {
			cmd := exec.Command(dtkit, "--toggle", "bundle")
			return cmd.Run()
		}
		return fmt.Errorf("no supported patcher found")
	}
	cmd := exec.Command(bat)
	return cmd.Run()
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
	app.appendLog("// " + app.messages["log_start"])

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
	app.refreshModList()

	// 2. Собираем активные моды для сортировки
	var activeNames []string
	notActive := make(map[string]bool)
	for _, mod := range app.allMods {
		if mod.Active && checks.FolderExists(mod.Name) {
			activeNames = append(activeNames, mod.Name)
		} else if checks.FolderExists(mod.Name) && !mod.IsSystem {
			notActive[mod.Name] = true
		}
	}

	// Если активных модов нет - просто завершаем
	if len(activeNames) == 0 {
		app.appendLog(app.messages["done"])
		return
	}

	sorter.CreateLoadOrderFromActive(activeNames, app.cfg.Language)

	// Дописываем неактивные моды, чтобы сохранить их состояние
	loadOrderPath := filepath.Join(app.cfg.ModsPath, FileNameLoadOrder)
	f, err := os.OpenFile(loadOrderPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		existing := make(map[string]bool)
		data, _ := os.ReadFile(loadOrderPath)
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
	}
	app.appendLog(app.messages["done"])

	// Финальное обновление UI и открытие файла
	fyne.Do(func() {
		app.refreshModList()
		absPath, _ := filepath.Abs(filepath.Join(app.cfg.ModsPath, FileNameLoadOrder))
		if _, err := os.Stat(absPath); err == nil {
			go func() {
				if err := openFileWithDefaultApp(absPath); err != nil {
					app.appendLog(fmt.Sprintf(app.messages["log_failed_open_file"], err))
				}
			}()
		}
	})
}
