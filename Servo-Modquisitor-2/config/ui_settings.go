package config

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ──────────────── Размеры главного окна ────────────────
const (
	MainWindowWidth  float32 = 900
	MainWindowHeight float32 = 700
)

// ──────────────── Размеры панелей ────────────────
const (
	LeftPanelMinWidth  float32 = 400
	RightPanelMinWidth float32 = 300
	SplitOffset        float64 = 0.625
)

// ──────────────── Размеры консоли ────────────────
const (
	ConsoleWidth  float32 = 800
	ConsoleHeight float32 = 160
)

// ──────────────── Размеры диалогов ────────────────
const (
	FileDialogWidth  float32 = 800
	FileDialogHeight float32 = 600
)

// ──────────────── Ширины колонок таблицы модов ────────────────
const (
	ColCheckboxWidth float32 = 30  // Галочка
	ColNumberWidth   float32 = 30  // Номер
	ColNameWidth     float32 = 300 // Название мода
	ColDateWidth     float32 = 90  // Дата
	ColStatusWidth   float32 = 100 // Статус
	ColNoteWidth     float32 = 550 // Примечание
)

// ──────────────── Количество колонок ────────────────
const TableColumnCount = 6

// ──────────────── Настройки описания ────────────────
const (
	DescScrollMinWidth  float32 = 200
	DescScrollMinHeight float32 = 150
)

// ──────────────── Логирование ────────────────
const (
	LogCharWidth    = 4
	LogDefaultWidth = 900
)

// ApplyWindowSettings применяет размеры к окну
func ApplyWindowSettings(window fyne.Window) {
	window.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
}

// ApplyTableColumnWidths задаёт ширину всем колонкам таблицы
func ApplyTableColumnWidths(table *widget.Table) {
	table.SetColumnWidth(0, ColCheckboxWidth)
	table.SetColumnWidth(1, ColNumberWidth)
	table.SetColumnWidth(2, ColNameWidth)
	table.SetColumnWidth(3, ColDateWidth)
	table.SetColumnWidth(4, ColStatusWidth)
	table.SetColumnWidth(5, ColNoteWidth)
}

// ──────────────── Настройки консоли ────────────────
const (
	ConsoleBackgroundImage = "assets/CRT_BlackBG.jpg" // путь к фоновой картинке
	ConsoleGradientOpacity = 0.4                      // прозрачность градиента (0.0 – 1.0)
	ConsoleFontSize        = 12                       // не используется напрямую, но можно добавить
)

// ──────────────── Настройки диалогов ────────────────
const (
	DialogMinWidth  float32 = 400
	DialogMinHeight float32 = 300
)

// ApplyDialogSettings применяет стандартные размеры к диалогу
func ApplyDialogSettings(dialog fyne.Window) {
	dialog.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
}

// ──────────────── Диалоги (градиент) ────────────────
const (
	DialogGradientWidth  = 600
	DialogGradientHeight = 50
)
