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
		return color.NRGBA{R: 240, G: 240, B: 245, A: 255}
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
	case theme.ColorNameInputBackground:
		return color.Transparent // return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 240, G: 240, B: 245, A: 255}
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 200, G: 20, B: 20, A: 255}

	// ================== КНОПКИ ======================
	case theme.ColorNameButton:
		return color.NRGBA{R: 210, G: 210, B: 218, A: 255}
	case theme.ColorNameHover:
		return color.NRGBA{R: 190, G: 190, B: 200, A: 255}
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 225, G: 225, B: 230, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 60, G: 100, B: 200, A: 128}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 170, G: 170, B: 180, A: 255}

	// ================== ТЕКСТ =======================
	case theme.ColorNameForeground:
		return color.NRGBA{R: 40, G: 40, B: 45, A: 255}
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 150, G: 150, B: 155, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 200, G: 40, B: 40, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 160, G: 160, B: 168, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 180, G: 140, B: 60, A: 255}
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 40, G: 80, B: 200, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 190, G: 190, B: 195, A: 200}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 245, G: 245, B: 248, A: 200}

	// ================== ВЫДЕЛЕНИЕ ===================
	case theme.ColorNameSelection:
		return color.NRGBA{R: 60, G: 100, B: 200, A: 80}

	// ================== РАЗДЕЛИТЕЛИ =================
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 200, G: 200, B: 208, A: 255}

	// ================== ТЕНИ ========================
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 80}

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 230, G: 230, B: 236, A: 255}

	// ================== ИНПУТ =======================
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 180, G: 180, B: 190, A: 255}

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
