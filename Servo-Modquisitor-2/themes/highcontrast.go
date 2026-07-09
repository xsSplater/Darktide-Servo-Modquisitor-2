// highcontrast.go
package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type HighContrastTheme struct{}

func (t HighContrastTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	// ================== ФОНЫ ========================
	case theme.ColorNameBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255} // чисто белый
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 240}
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255} // чёрный текст на предупреждении

	// ================== КНОПКИ ======================
	case theme.ColorNameButton:
		return color.NRGBA{R: 245, G: 245, B: 255, A: 255} // Белые кнопки
	case theme.ColorNameHover:
		return color.NRGBA{R: 80, G: 80, B: 80, A: 255} // тёмно-серый при наведении
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 255, G: 165, B: 0, A: 200} // ярко-оранжевая обводка
	case theme.ColorNamePressed:
		return color.NRGBA{R: 50, G: 50, B: 50, A: 255}

	// ================== ТЕКСТ =======================
	case theme.ColorNameForeground:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255} // чёрный
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 150, G: 150, B: 150, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 255, G: 0, B: 0, A: 255} // ярко-красный
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 120, G: 120, B: 120, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 0, G: 0, B: 255, A: 255} // синий акцент
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 0, G: 0, B: 200, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 240, G: 240, B: 240, A: 255}

	// ================== ВЫДЕЛЕНИЕ ===================
	case theme.ColorNameSelection:
		return color.NRGBA{R: 0, G: 0, B: 255, A: 100}

	// ================== РАЗДЕЛИТЕЛИ =================
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}

	// ================== ТЕНИ ========================
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 80}

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255}

	// ================== ВВОД ========================
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}

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
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case ColorTableRowOdd:
		return color.NRGBA{R: 240, G: 240, B: 240, A: 255}
	case ColorTableBorderDirty:
		return color.NRGBA{R: 255, G: 140, B: 0, A: 255}
	case ColorTableHeaderBg:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 255}
	case ColorSystemTableBg:
		return color.NRGBA{R: 240, G: 240, B: 240, A: 200}
	case ColorTableRowSelected:
		return color.NRGBA{R: 60, G: 255, B: 30, A: 55} // Выделенная строка
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
		return color.NRGBA{R: 0, G: 255, B: 0, A: 255}
	case ColorCRTScreenFill:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 10}
	case ColorCRTScreenStroke:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 30}
	case ColorCRTHeaderBg:
		return color.NRGBA{R: 200, G: 200, B: 200, A: 200}

	// ──────────────── Панели/карточки ────────
	case ColorDescCardStroke:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	case ColorDescCardBg:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case ColorManagePanelBg:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case ColorTopPanelBg:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case ColorTipBg:
		return color.NRGBA{R: 255, G: 248, B: 225, A: 255}

	// ──────────────── Кастомные кнопки ───────
	case ColorButtonShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 40}
	case ColorButtonShadowDisabled:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 15}
	case ColorButtonStroke:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	case ColorButtonStrokeImage:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 150}

	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (t HighContrastTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}
func (t HighContrastTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}
func (t HighContrastTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
