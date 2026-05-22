package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const AppVersion = "0.9.5"

// ────────────────────────── ОКНО ──────────────────────────
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
const (
	FileDialogWidth  float32 = 800
	FileDialogHeight float32 = 600
)

const (
	DialogMinWidth  float32 = 400
	DialogMinHeight float32 = 300
)

const (
	DialogGradientWidth  = 600
	DialogGradientHeight = 50
)

// ────────────────────────── ОПИСАНИЕ ──────────────────────────
const (
	DescScrollMinWidth  float32 = 200
	DescScrollMinHeight float32 = 150
)

// ────────────────────────── ТАБЛИЦЫ ──────────────────────────
const TableColumnCount = 7

const (
	ColCheckboxWidth float32 = 30
	ColSelectWidth   float32 = 30
	ColNumberWidth   float32 = 30
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
const SearchMinWidth = 250

// ────────────────────────── ФАЙЛЫ ──────────────────────────
const (
	FileNameLoadOrder      = "mod_load_order.txt"
	FileNameMandatoryRules = "mandatory_obsolete_incompatible_dependencies.json"
	FileNameModDatabase    = "mod_database.json"
)

// ────────────────────────── ФОРМАТ ВРЕМЕНИ ──────────────────────────
const LogTimeFormat = "15:04:05"

// ────────────────────────── ЗАДЕРЖКИ ──────────────────────────
const (
	TooltipHideDelay    = 2 * time.Second
	WindowMaximizeDelay = 200 * time.Millisecond
	BlinkOnDuration     = 600 * time.Millisecond
	BlinkOffDuration    = 1000 * time.Millisecond
	BlinkCheckInterval  = 2 * time.Second
)

// ────────────────────────── ID ИГРЫ ──────────────────────────
const DarktideAppID = "1361210"

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
