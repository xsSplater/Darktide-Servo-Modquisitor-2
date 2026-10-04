// Servo-Modquisitor-2/shortcut.go
package main

import (
	"bytes"
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fyne-io/image/ico"
)

// createDesktopShortcut создаёт ярлык быстрого запуска игры на рабочем
// столе пользователя. Возвращает путь к созданному ярлыку.
//
// Поддерживаемые ОС: Windows (.lnk через PowerShell + WScript.Shell),
// Linux (.desktop в ~/Desktop или локализованный эквивалент).
func (app *App) createDesktopShortcut() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot resolve executable path: %w", err)
	}

	switch runtime.GOOS {
	case "windows":
		return createWindowsShortcut(exePath)
	case "linux":
		return createLinuxShortcut(exePath)
	default:
		return "", fmt.Errorf("desktop shortcut not supported on %s", runtime.GOOS)
	}
}

// createLinuxShortcut пишет .desktop-файл с Exec=".../exe" --play.
//
// Пути с пробелами оборачиваем в двойные кавычки по спецификации
// Desktop Entry (пустая строка вместо экранирования backslash — так
// короче и читаемее, парсеры это принимают).
//
// Иконку берём из assets/icon.png, кладём в конфиг-папку и ссылаемся
// на файл — иначе на некоторых DE .desktop показывается без иконки.
func createLinuxShortcut(exePath string) (string, error) {
	desktopDir := linuxDesktopDir()
	if desktopDir == "" {
		return "", fmt.Errorf("desktop directory not found")
	}

	iconPath := ""
	// Иконка ярлыка — play_fast (символ «быстрого запуска»),
	// а не AppIcon: на рабочем столе пользователь видит именно
	// ярлык игры, и пиктограмма должна это отражать.
	if data, err := embeddedFiles.ReadFile("assets/buttons/play_fast.png"); err == nil && len(data) > 0 {
		dir := filepath.Dir(configFilePath())
		_ = os.MkdirAll(dir, 0755)
		iconPath = filepath.Join(dir, "quick_launch.png")
		if err := os.WriteFile(iconPath, data, 0644); err != nil {
			iconPath = "" // не критично, ярлык создадим без иконки
		}
	}

	execQuoted := `"` + strings.ReplaceAll(exePath, `"`, `\"`) + `"`
	iconLine := ""
	if iconPath != "" {
		iconLine = "Icon=" + iconPath + "\n"
	}

	content := fmt.Sprintf(`[Desktop Entry]
Version=1.0
Type=Application
Name=Darktide Quick Launch
Comment=Launch Darktide with mods, skipping the mod manager UI
Exec=%s --play
%sTerminal=false
Categories=Game;
StartupNotify=false
`, execQuoted, iconLine)

	shortcutPath := filepath.Join(desktopDir, "Servo-Modquisitor (Quick Launch).desktop")
	if err := os.WriteFile(shortcutPath, []byte(content), 0755); err != nil {
		return "", fmt.Errorf("write .desktop: %w", err)
	}
	return shortcutPath, nil
}

// linuxDesktopDir ищет путь к рабочему столу: сначала XDG (user-dirs.dirs), потом типичные локализованные имена. Возвращает "" если ничего не нашли.
func linuxDesktopDir() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}

	configDir := os.Getenv("XDG_CONFIG_HOME")
	if configDir == "" {
		configDir = filepath.Join(home, ".config")
	}
	if data, err := os.ReadFile(filepath.Join(configDir, "user-dirs.dirs")); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "XDG_DESKTOP_DIR=") {
				continue
			}
			val := strings.TrimPrefix(line, "XDG_DESKTOP_DIR=")
			val = strings.Trim(val, `"`)
			val = strings.ReplaceAll(val, "$HOME", home)
			if info, err := os.Stat(val); err == nil && info.IsDir() {
				return val
			}
		}
	}

	for _, name := range []string{"Desktop", "Рабочий стол", "Schreibtisch", "Bureau", "Escritorio"} {
		p := filepath.Join(home, name)
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			return p
		}
	}
	return ""
}

// createWindowsShortcut создаёт .lnk через PowerShell + WScript.Shell.
//
// Иконка: Windows Shell не умеет читать PNG, только .ico/PE. Конвертируем
// PNG из embed в ICO на лету и сохраняем в конфиг-папку — оттуда
// IconLocation на неё и смотрит.
func createWindowsShortcut(exePath string) (string, error) {
	escapePS := func(s string) string {
		return strings.ReplaceAll(s, "'", "''")
	}

	iconPath := ensureWindowsShortcutIcon()
	iconLocation := exePath + ",0" // fallback: иконка самого exe
	if iconPath != "" {
		iconLocation = iconPath + ",0"
	}

	workDir := filepath.Dir(exePath)
	script := fmt.Sprintf(`
$desktop = [Environment]::GetFolderPath('Desktop')
$lnk = Join-Path $desktop 'Servo-Modquisitor (Quick Launch).lnk'
$s = (New-Object -COM WScript.Shell).CreateShortcut($lnk)
$s.TargetPath = '%s'
$s.Arguments = '--play'
$s.WorkingDirectory = '%s'
$s.IconLocation = '%s'
$s.Save()
Write-Output $lnk
`, escapePS(exePath), escapePS(workDir), escapePS(iconLocation))

	psExe := "powershell.exe"
	if _, err := exec.LookPath(psExe); err != nil {
		if _, err2 := exec.LookPath("pwsh.exe"); err2 == nil {
			psExe = "pwsh.exe"
		} else {
			return "", fmt.Errorf("neither powershell.exe nor pwsh.exe found")
		}
	}

	cmd := exec.Command(psExe, "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("powershell failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// ensureWindowsShortcutIcon декодирует PNG из embed и сохраняет его
// как .ico в конфиг-папке. Возвращает путь к .ico или "" при ошибке.
//
// ICO-файл один на всю программу (перезаписывается при каждом вызове) —
// терять на нём смысла нет.
func ensureWindowsShortcutIcon() string {
	data, err := embeddedFiles.ReadFile("assets/buttons/play_fast.png")
	if err != nil || len(data) == 0 {
		return ""
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	dir := filepath.Dir(configFilePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ""
	}
	iconPath := filepath.Join(dir, "quick_launch.ico")

	var buf bytes.Buffer
	if err := ico.Encode(&buf, img); err != nil {
		return ""
	}
	if err := os.WriteFile(iconPath, buf.Bytes(), 0644); err != nil {
		return ""
	}
	return iconPath
}
