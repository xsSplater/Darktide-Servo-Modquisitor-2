package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type ForcedDarkTheme struct{}

func (t ForcedDarkTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	// ================== ФОНЫ ========================
	// Основной фон всего приложения (окна, пустые области)
	case theme.ColorNameBackground:		return color.NRGBA{R: 32,  G: 32,  B: 32,  A: 225}
	// Фон выпадающих меню (строки меню, контекстные меню)
	case theme.ColorNameMenuBackground:
										return color.NRGBA{R: 52,  G: 55,  B: 52,  A: 240}
	// Фон полей ввода - прозрачный для видимости CRT-фона
	case theme.ColorNameInputBackground:
										return color.NRGBA{R: 5,   G: 6,   B: 5,   A: 155}
	// Фон всплывающих окон (диалогов) - как основной фон
	case theme.ColorNameOverlayBackground:
										return color.NRGBA{R: 32,  G: 32,  B: 32,  A: 255}
	// Цвет текста предупреждений или ошибок
	case theme.ColorNameForegroundOnWarning:
										return color.NRGBA{R: 192, G: 255, B: 26,  A: 255}

	// ================== КНОПКИ ======================
	// Основной цвет обычной кнопки
	case theme.ColorNameButton:			return color.NRGBA{R: 55,  G: 60,  B: 55,  A: 240}
	// Цвет кнопки, на которую навели мышь (hover)
	case theme.ColorNameHover:			return color.NRGBA{R: 70,  G: 120, B: 38,  A: 222}
	// Цвет неактивной кнопки
	case theme.ColorNameDisabledButton:	return color.NRGBA{R: 30,  G: 33,  B: 30,  A: 111}
	// Индикатор фокуса (обводка активного элемента) - обычно едва заметный
	case theme.ColorNameFocus:			return color.NRGBA{R: 80,  G: 130, B: 200, A: 128}
	case theme.ColorNamePressed:		return color.NRGBA{R: 60,  G: 60,  B: 68,  A: 255}

	// ================== ТЕКСТ =======================
	// Основной цвет текста
	case theme.ColorNameForeground:		return color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	// Цвет текста неактивных виджетов (например, disabled-кнопок)
	case theme.ColorNameDisabled:		return color.NRGBA{R: 111, G: 115, B: 111, A: 111} 
	// Цвет сообщения об ошибке (красный текст)
	case theme.ColorNameError:			return color.NRGBA{R: 220, G: 60,  B: 60,  A: 255}
	// Цвет текста-подсказки (placeholder) в полях ввода
	case theme.ColorNamePlaceHolder:	return color.NRGBA{R: 100, G: 100, B: 110, A: 255}
	// Основной акцентный цвет темы (используется для заголовков, индикаторов важности)
	case theme.ColorNamePrimary:		return color.NRGBA{R: 60,  G: 160, B: 30,  A: 255}
	// Цвет гиперссылок (в Fyne они могут отображаться как текст)
	case theme.ColorNameHyperlink:		return color.NRGBA{R: 80,  G: 140, B: 220, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	// Цвет ползунка скроллбара
	case theme.ColorNameScrollBar:		return color.NRGBA{R: 60,  G: 150, B: 65,  A: 200}
	// Цвет дорожки (фона) скроллбара
	case theme.ColorNameScrollBarBackground:
										return color.NRGBA{R: 22,  G: 22,  B: 22,  A: 200}

	// ================== ВЫДЕЛЕНИЕ ===================
	// Цвет выделения текста (например, при выделении мышью)
	case theme.ColorNameSelection:		return color.NRGBA{R: 80,  G: 140, B: 220, A: 100}

	// ================== РАЗДЕЛИТЕЛИ =================
	// Цвет разделительных линий (например, widget.NewSeparator())
	case theme.ColorNameSeparator:		return color.NRGBA{R: 65,  G: 75,  B: 55,  A: 155}

	// ================== ТЕНИ ========================
	// Цвет тени окон и всплывающих элементов
	case theme.ColorNameShadow:			return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 180}

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
										return color.NRGBA{R: 40,  G: 40,  B: 44,  A: 255}

	// ================== ИНПУТ =======================
	case theme.ColorNameInputBorder:	return color.NRGBA{R: 190, G: 255, B: 110, A: 255}

    // ──────────────── Статусы ────────────────
    case ColorStatusSystem:				return color.NRGBA{R: 100, G: 180, B: 255, A: 255}
    case ColorStatusBroken:				return color.NRGBA{R: 255, G: 80,  B: 80,  A: 255}
    case ColorStatusConflict:			return color.NRGBA{R: 255, G: 140, B: 0,   A: 255}
    case ColorStatusObsolete:			return color.NRGBA{R: 180, G: 180, B: 0,   A: 255}
    case ColorStatusMandatory:			return color.NRGBA{R: 0,   G: 180, B: 0,   A: 255}
    case ColorStatusActive:				return color.NRGBA{R: 100, G: 200, B: 100, A: 255}
    case ColorStatusInactive:			return color.NRGBA{R: 140, G: 140, B: 140, A: 255}

    // ──────────────── Таблица ────────────────
    case ColorTableRowEven:				return color.NRGBA{R: 38,  G: 38,  B: 42,  A: 155}
    case ColorTableRowOdd:				return color.NRGBA{R: 34,  G: 34,  B: 38,  A: 5  }
    case ColorTableRowSelected:			return color.NRGBA{R: 60,  G: 160, B: 30,  A: 80 }
    case ColorTableRowConflict:			return color.NRGBA{R: 80,  G: 40,  B: 0,   A: 120}
    case ColorTableBorderDirty:			return color.NRGBA{R: 200, G: 100, B: 0,   A: 255}
    case ColorTableHeaderBg:			return color.NRGBA{R: 20,  G: 20,  B: 20,  A: 255}
    case ColorSystemTableBg:			return color.NRGBA{R: 20,  G: 20,  B: 20,  A: 150}

    // ──────────────── Консоль ────────────────
	case ColorConsoleText:				return color.NRGBA{R: 192, G: 255, B: 26,  A: 255}
    case ColorCRTScreenFill:			return color.NRGBA{R: 192, G: 255, B: 26,  A: 15 }
    case ColorCRTScreenStroke:			return color.NRGBA{R: 192, G: 255, B: 26,  A: 111}
    case ColorCRTHeaderBg:				return color.NRGBA{R: 10,  G: 10,  B: 10,  A: 175}

    // ──────────────── Панели/карточки ────────
	case ColorDescCardStroke:			return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 155}
    case ColorDescCardBg:				return color.NRGBA{R: 45,  G: 45,  B: 50,  A: 255}
    case ColorManagePanelBg:			return color.NRGBA{R: 10,  G: 10,  B: 10,  A: 255}
    case ColorTopPanelBg:				return color.NRGBA{R: 22,  G: 22,  B: 22,  A: 255}
    case ColorTipBg:					return color.NRGBA{R: 10,  G: 10,  B: 10,  A: 200}

    // ──────────────── Кастомные кнопки ───────
    case ColorButtonShadow:				return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 85 }
    case ColorButtonShadowDisabled:		return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 40 }
    case ColorButtonStroke:				return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 155}
    case ColorButtonStrokeImage:		return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 85 }

	// Если вдруг запрошен неизвестный цвет - отдаём белый (заглушка)
	default:							return theme.DefaultTheme().Color(name, variant)
	}
}

// Остальные методы темы: шрифт, иконки и размеры берём из стандартной тёмной темы,
// чтобы не менять привычный вид элементов интерфейса.
func (t ForcedDarkTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}
func (t ForcedDarkTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (t ForcedDarkTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
