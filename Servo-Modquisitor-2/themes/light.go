// light.go
package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type ForcedLightTheme struct{}

func (t ForcedLightTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	// ================== ФОНЫ ========================
	case theme.ColorNameBackground:
		// Лёгкий зеленоватый оттенок, напоминающий бумагу или старый экран
		return color.NRGBA{R: 240, G: 244, B: 232, A: 255} // #F0F4E8
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // чисто белый
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // белый для полей ввода
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 240} // полупрозрачный белый для оверлеев
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // белый текст на警告

	// ================== КНОПКИ ======================
	case theme.ColorNameButton:
		return color.NRGBA{R: 240, G: 241, B: 243, A: 255} // светло-серый фон кнопки
	case theme.ColorNameHover:
		return color.NRGBA{R: 220, G: 225, B: 230, A: 255} // чуть темнее при наведении
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 235, G: 236, B: 240, A: 255} // очень светлый для disabled
	case theme.ColorNameFocus:
		return color.NRGBA{R: 76, G: 175, B: 80, A: 80} // зелёное свечение (прозрачное)
	case theme.ColorNamePressed:
		return color.NRGBA{R: 200, G: 205, B: 210, A: 255} // тёмный оттенок при нажатии

	// ================== ТЕКСТ =======================
	case theme.ColorNameForeground:
		return color.NRGBA{R: 33, G: 33, B: 33, A: 255} // почти чёрный (#212121)
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 180, G: 180, B: 180, A: 255} // светло-серый
	case theme.ColorNameError:
		return color.NRGBA{R: 211, G: 47, B: 47, A: 255} // красный (#D32F2F)
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 160, G: 160, B: 160, A: 255} // серый плейсхолдер
	case theme.ColorNamePrimary:
		// Тёмно-зелёный, как цвет символов на старом CRT-мониторе
		return color.NRGBA{R: 46, G: 125, B: 50, A: 255} // #2E7D32
	case theme.ColorNameHyperlink:
		// Оставляем синий для привычности, но можно заменить на #2E7D32 при желании
		return color.NRGBA{R: 33, G: 150, B: 243, A: 255} // синий (#2196F3)

	// ================== СКРОЛЛБАРЫ ==================
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255} // светло-серый
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 240, G: 240, B: 240, A: 255} // очень светлый

	// ================== ВЫДЕЛЕНИЕ ===================
	case theme.ColorNameSelection:
		return color.NRGBA{R: 76, G: 175, B: 80, A: 80} // полупрозрачный зелёный

	// ================== РАЗДЕЛИТЕЛИ =================
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 224, G: 224, B: 224, A: 255} // светло-серый

	// ================== ТЕНИ ========================
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 30} // лёгкая тень

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
		// Слегка зеленоватый фон заголовков, перекликающийся с основным фоном
		return color.NRGBA{R: 232, G: 240, B: 220, A: 255} // #E8F0DC

	// ================== ВВОД ========================
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255} // серая рамка

	// ──────────────── Статусы (кастомные) ────────────────
	case ColorStatusSystem:
		return color.NRGBA{R: 100, G: 180, B: 255, A: 255}
	case ColorStatusBroken:
		return color.NRGBA{R: 255, G: 80, B: 80, A: 255}
	case ColorStatusConflict:
		return color.NRGBA{R: 255, G: 140, B: 0, A: 255}
	case ColorStatusObsolete:
		return color.NRGBA{R: 180, G: 180, B: 0, A: 255}
	case ColorStatusMandatory:
		return color.NRGBA{R: 250, G: 120, B: 120, A: 255}
	case ColorStatusActive:
		return color.NRGBA{R: 100, G: 200, B: 100, A: 255}
	case ColorStatusInactive:
		return color.NRGBA{R: 140, G: 140, B: 140, A: 255}
	case ColorStatusVortex:
		return color.NRGBA{R: 100, G: 200, B: 255, A: 255}
	case ColorStatusMissing:
		return color.NRGBA{R: 255, G: 100, B: 100, A: 255} // красный
	case ColorStatusSymlink:
		return color.NRGBA{R: 255, G: 110, B: 255, A: 255} // фиолетовый
	case ColorStatusManual:
		return color.NRGBA{R: 20, G: 155, B: 250, A: 255} //
	case ColorStatusNexus:
		return color.NRGBA{R: 255, G: 200, B: 80, A: 255} // золотистый

	// ──────────────── Таблица ────────────────
	case ColorTableRowEven:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // белый
	case ColorTableRowOdd:
		return color.NRGBA{R: 245, G: 246, B: 248, A: 255} // очень светлый серый
	case ColorTableBorderDirty:
		return color.NRGBA{R: 255, G: 152, B: 0, A: 255} // оранжевая рамка
	case ColorTableHeaderBg:
		// Заголовок таблицы с лёгким зелёным оттенком
		return color.NRGBA{R: 225, G: 235, B: 215, A: 255} // #E1EBD7
	case ColorSystemTableBg:
		return color.NRGBA{R: 245, G: 246, B: 248, A: 200} // с лёгкой прозрачностью
	case ColorTableRowSelected:
		return color.NRGBA{R: 60, G: 222, B: 30, A: 55} // Выделенная строка
	case ColorTableRowConflict:
		return color.NRGBA{R: 255, G: 77, B: 0, A: 55} // Mod Conflict background
	case ColorTableObsoleteMod:
		return color.NRGBA{R: 150, G: 150, B: 0, A: 55} // Obsolete Mod background
	case ColorTableHasUpdateMod:
		return color.NRGBA{R: 0, G: 222, B: 255, A: 55} // Mod Has Update background
	case ColorTableMissingFolder:
		return color.NRGBA{R: 255, G: 0, B: 0, A: 55} // Missing Folder background
	case ColorStatusSymlinkBg:
		return color.NRGBA{R: 255, G: 55, B: 255, A: 55} // Symlink background

	// ──────────────── Консоль ────────────────
	case ColorConsoleText:
		return color.NRGBA{R: 0, G: 200, B: 0, A: 255} // зелёный текст (как терминал)
	case ColorCRTScreenFill:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 10} // очень лёгкая заливка
	case ColorCRTScreenStroke:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 30} // тень или обводка
	case ColorCRTHeaderBg:
		return color.NRGBA{R: 240, G: 241, B: 243, A: 200} // фон заголовка консоли

	// ──────────────── Панели/карточки ────────
	case ColorDescCardStroke:
		return color.NRGBA{R: 224, G: 224, B: 224, A: 255} // рамка карточки
	case ColorDescCardBg:
		// Фон карточки с лёгким зеленоватым оттенком
		return color.NRGBA{R: 248, G: 250, B: 240, A: 255} // #F8FAF0
	case ColorManagePanelBg:
		// Панель управления - тот же оттенок, что и основной фон
		return color.NRGBA{R: 240, G: 244, B: 232, A: 255} // #F0F4E8
	case ColorTopPanelBg:
		// Верхняя панель - единый с фоном цвет
		return color.NRGBA{R: 240, G: 244, B: 232, A: 255} // #F0F4E8
	case ColorTipBg:
		return color.NRGBA{R: 255, G: 248, B: 225, A: 255} // светло-жёлтый (подсказка)

	// ──────────────── Кастомные кнопки ───────
	case ColorButtonShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 20} // тень кнопки
	case ColorButtonShadowDisabled:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 8} // тень для disabled
	case ColorButtonStroke:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255} // обводка кнопки
	case ColorButtonStrokeImage:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 150} // обводка для иконок

	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t ForcedLightTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}
func (t ForcedLightTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (t ForcedLightTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
