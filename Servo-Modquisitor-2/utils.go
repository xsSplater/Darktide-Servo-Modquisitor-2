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
)

func SetLauncherMessages(verUnknown, exeNotFound, rootNotFound string) {
	errGameVersionUnknown = verUnknown
	errDarktideExeNotFound = exeNotFound
	errGameRootNotFound = rootNotFound
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
	dir, _ := os.Getwd()
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
	return PatcherNone
}

func isModsEnabledAutoPatch() bool {
	_, err := os.Stat("DISABLE_AUTOPATCHER")
	return os.IsNotExist(err)
}

func toggleModsAutoPatch() error {
	if isModsEnabledAutoPatch() {
		f, _ := os.Create("DISABLE_AUTOPATCHER")
		if f != nil {
			f.Close()
		}
		if _, err := os.Stat("../bundle/bundle_database.data.bak"); err == nil {
			os.Rename("../bundle/bundle_database.data.bak", "../bundle/bundle_database.data")
		}
	} else {
		os.Remove("DISABLE_AUTOPATCHER")
	}
	return nil
}

func toggleModsLegacy() error {
	gameRoot := getGameRoot()
	if gameRoot == "" {
		return fmt.Errorf("%s", errGameRootNotFound)
	}
	cmd := exec.Command(filepath.Join(gameRoot, "tools", "dtkit-patch"), "--toggle", "bundle")
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
	app.appendLog("// " + app.messages["log_start"]) // Запуск диагностики...

	checks.CheckInstallation(app.mainWindow) // Ищем папки base и dmf

	checks.EnsureModLoadOrder(app.mainWindow) // Ищем список модов mod_load_order.txt

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

	// Первое обновление списка (после удалений) – должно быть в главном потоке
	fyne.Do(func() { app.refreshModList() })

	// Собираем активные моды (можно без UI)
	var activeNames []string
	for _, mod := range app.allMods {
		if mod.Active && checks.FolderExists(mod.Name) {
			activeNames = append(activeNames, mod.Name)
		}
	}
	sorter.CreateLoadOrderFromActive(activeNames, app.cfg.Language)
	app.appendLog(app.messages["done"])

	// Финальное обновление UI и открытие файла – в главном потоке
	fyne.Do(func() {
		app.refreshModList()
		absPath, _ := filepath.Abs(filepath.Join(app.cfg.ModsPath, "mod_load_order.txt"))
		if _, err := os.Stat(absPath); err == nil {
			go func() {
				if err := openFileWithDefaultApp(absPath); err != nil {
					app.appendLog(fmt.Sprintf("Failed to open file: %v", err))
				}
			}()
		}
	})
}
