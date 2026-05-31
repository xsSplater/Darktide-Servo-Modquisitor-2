// config.go
package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const AppVersion = "1.0.0"

// ────────────────────────── ID ИГРЫ ──────────────────────────
const DarktideAppID = "1361210"

// ────────────────────────── ФАЙЛЫ ──────────────────────────
const (
	FileNameLoadOrder      = "mod_load_order.txt"
	FileNameMandatoryRules = "mandatory_obsolete_incompatible_dependencies.json"
	FileNameModDatabase    = "mod_database.json"

	gitHubRepoURL = "https://api.github.com/repos/xsSplater/Darktide-Servo-Modquisitor-2/releases/latest"

	modDatabaseURL  = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mod_database.json"
	modMandatoryURL = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mandatory_obsolete_incompatible_dependencies.json"
)

// ────────────────────────── ФОРМАТ ВРЕМЕНИ ──────────────────────────
const LogTimeFormat = "15:04:05"

// ────────────────────────── ОКНО ПРОГРАММЫ ──────────────────────────
const (
	MainWindowWidth  float32 = 900
	MainWindowHeight float32 = 700
)

// ────────────────────────── ПАНЕЛИ ──────────────────────────
const (
	LeftPanelMinWidth  float32 = 400
	RightPanelMinWidth float32 = 300
	SplitOffset        float64 = 0.625
)

// ────────────────────────── КОНСОЛЬ ──────────────────────────
const (
	ConsoleWidth  float32 = 500
	ConsoleHeight float32 = 300
)

const (
	ConsoleBackgroundImage = "assets/CRT_BlackBG.jpg"
	ConsoleGradientOpacity = 0.4
)

// ────────────────────────── ДИАЛОГИ ──────────────────────────
type DialogType int

const (
	DialogTypeInfo  DialogType = iota // зелёный
	DialogTypeWarn                    // красный
	DialogTypeError                   // красный
)

const (
	FileDialogWidth  float32 = 800
	FileDialogHeight float32 = 600
)

const (
	DialogMinWidth  float32 = 400
	DialogMinHeight float32 = 300
)

const (
	DialogGradientWidth  = 400
	DialogGradientHeight = 50
)

const (
	DialogButtonMinWidth float32 = 120
	DialogButtonHeight   float32 = 36
)

// ────────────────────────── ОПИСАНИЕ ──────────────────────────
const (
	DescScrollMinWidth  float32 = 200
	DescScrollMinHeight float32 = 250
)

// ────────────────────────── ТАБЛИЦЫ ──────────────────────────
const TableColumnCount = 7

const (
	ColCheckboxWidth float32 = 30
	ColSelectWidth   float32 = 30
	ColNumberWidth   float32 = 40
	ColNameWidth     float32 = 350
	ColDateWidth     float32 = 100
	ColStatusWidth   float32 = 120
	ColNoteWidth     float32 = 510
)

const (
	TableRowHeight    = 6
	SystemTableHeight = 75
)

// ────────────────────────── ПОИСК ──────────────────────────
const SearchMinWidth = 350

// ────────────────────────── ЗАДЕРЖКИ ──────────────────────────
const (
	TooltipHideDelay    = 10 * time.Second
	WindowMaximizeDelay = 200 * time.Millisecond
	BlinkOnDuration     = 600 * time.Millisecond
	BlinkOffDuration    = 1000 * time.Millisecond
	BlinkCheckInterval  = 2 * time.Second
)

// ────────────────────────── ВСПОМОГАТЕЛЬНЫЕ ФУНКЦИИ ──────────────────────────

func ApplyWindowSettings(window fyne.Window) {
	window.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
}

func ApplyTableColumnWidths(table *widget.Table) {
	table.SetColumnWidth(0, ColSelectWidth)
	table.SetColumnWidth(1, ColCheckboxWidth)
	table.SetColumnWidth(2, ColNumberWidth)
	table.SetColumnWidth(3, ColNameWidth)
	table.SetColumnWidth(4, ColDateWidth)
	table.SetColumnWidth(5, ColStatusWidth)
	table.SetColumnWidth(6, ColNoteWidth)
}

func ApplyDialogSettings(dialog fyne.Window) {
	dialog.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
}
