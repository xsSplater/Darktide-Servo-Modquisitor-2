// config.go
package main

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ───────────── Программа ─────────────────────────────────────────
const (
	AppName         = "Servo-Modquisitor-2"
	AppVersion      = "1.1.1"
	AppID           = "com.xssplater.servo-modquisitor"
	AppIcon         = "assets/icon.png"
	СonfigFolderSMQ = "Servo-Modquisitor"
	DarktideAppID   = "1361210"
)

// ───────────── Нексус ────────────────────────────────────────────
const (
	clientID          = "servomodquisitor2"
	NexusMainURL      = "https://www.nexusmods.com/"
	nexusAPIBase      = "https://api.nexusmods.com/v1"
	oauthAuthorizeURL = "https://users.nexusmods.com/oauth/authorize"
	oauthTokenURL     = "https://users.nexusmods.com/oauth/token"
	redirectURI       = "http://localhost:31337/callback" // (не менять!)
	NexusV1Files      = "https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%d/files.json"
	NexusV1Filess     = "https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s.json"
	NexusV1DownLink   = "https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s/download_link.json"
)

// ───────────── Файлы ─────────────────────────────────────────────
const (
	// Файл проверки модов на несовместимости, устаревание, зависимости
	FileNameMandatoryRules = "mandatory_obsolete_incompatible_dependencies.json"
	FileNameLoadOrder      = "mod_load_order.txt"  // Файл список модов
	FileNameModDatabase    = "mod_database.json"   // Файл базы модов
	FileNameMessages       = "lang/messages.json"  // Файл сообщений
	FileNameNexusVersions  = "nexus_versions.json" // Файл версий модов
	FileNameConfig         = "config.json"         // Файл конфига
	FileNameLog            = "app.log"             // Файл лога
)

// ───────────── Ссылки ────────────────────────────────────────────
const (
	GitHubRepoSMQ = "https://github.com/xsSplater/Darktide-Servo-Modquisitor-2"
	gitHubRepoURL = "https://api.github.com/repos/xsSplater/Darktide-Servo-Modquisitor-2/releases/latest"

	modDatabaseURL  = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mod_database.json"
	modMandatoryURL = "https://raw.githubusercontent.com/xsSplater/Darktide-Servo-Modquisitor-2/main/SortingRules_and_ModDatabase/mandatory_obsolete_incompatible_dependencies.json"

	DarktideModDML = "https://www.nexusmods.com/warhammer40kdarktide/mods/19"

	DiscordDTModders = "https://discord.com/channels/1048312349867646996/1506507675976859679"
	DiscordDTMy      = "https://discord.gg/BGZagw3xnz"
)

// ───────────── Сетевой слушатель и аргументы ─────────────────────
const (
	NXMProtocol     = "tcp"
	NXMAddress      = "localhost:31338" // порт для приёма nxm-ссылок
	OAuthListenAddr = "localhost:31337" // порт для OAuth-колбэка (не менять!)
	NXMCommLine     = "--nxm"
)

// ───────────── Форматы времени ───────────────────────────────────
const (
	LogTimeFormat       = "02-01-2006 15:04:05"
	YYYYMMDD_TimeFormat = "2006-01-02"
	MMDDYYYY_TimeFormat = "01-02-2006"
	DDMMYYYY_TimeFormat = "02-01-2006"
)

// ───────────── Окно программы ────────────────────────────────────
const (
	MainWindowWidth  float32 = 900
	MainWindowHeight float32 = 700
)

// ───────────── Панели ────────────────────────────────────────────
const (
	LeftPanelMinWidth  float32 = 400
	RightPanelMinWidth float32 = 300
	SplitOffset        float64 = 0.625
)

// ───────────── Консоль ───────────────────────────────────────────
const (
	ConsoleWidth  float32 = 500
	ConsoleHeight float32 = 300
)

const (
	ConsoleBackgroundImage = "assets/CRT_BlackBG.jpg"
	ConsoleGradientOpacity = 0.4 // 1.00 - невидимый, 0 - видимый
)

// ───────────── Таблица ───────────────────────────────────────────
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

const (
	TableBackgroundImage   = "assets/mechanicus.png"
	TableBackgroundOpacity = 0.98 // 1.00 - невидимый, 0 - видимый
)

const (
	HeaderBackgroundImage = "assets/Yellow_BG.jpg"
	ButtonBackgroundImage = "assets/Yellow_BG_button.jpg"
	ColBackgroundImage    = "assets/Yellow_BG_col.jpg"
)

// ───────────── Диалоги ───────────────────────────────────────────
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

// ───────────── Описание ──────────────────────────────────────────
const (
	DescScrollMinWidth  float32 = 200
	DescScrollMinHeight float32 = 250
)

// ───────────── Поиск ─────────────────────────────────────────────
const SearchMinWidth = 350

// ───────────── Задержки ──────────────────────────────────────────
const (
	WindowMaximizeDelay = 200 * time.Millisecond
	BlinkOnDuration     = 600 * time.Millisecond
	BlinkOffDuration    = 1000 * time.Millisecond
	BlinkCheckInterval  = 2 * time.Second
	TooltipHideDelay    = 10 * time.Second
	Timeout10Seconds    = 10 * time.Second
	Timeout30Minutes    = 30 * time.Minute
)

// ───────────── Вспомогательные функции ───────────────────────────

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
