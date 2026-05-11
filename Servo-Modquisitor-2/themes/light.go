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
		return color.NRGBA{R: 222, G: 225, B: 222, A: 200}  // тёплый светло‑серый
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 250, G: 250, B: 252, A: 225} // чуть светлее, для выпадающих меню
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 200} // полупрозрачный белый
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 237, G: 237, B: 242, A: 255}
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 200, G: 40, B: 40, A: 255}

	// ================== КНОПКИ ======================
	case theme.ColorNameButton:
		return color.NRGBA{R: 215, G: 215, B: 222, A: 255} // светлая кнопка
	case theme.ColorNameHover:
		return color.NRGBA{R: 180, G: 210, B: 160, A: 255} // мягкий зелёный hover
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 225, G: 225, B: 230, A: 255}
	case theme.ColorNameFocus:
		return color.NRGBA{R: 100, G: 140, B: 100, A: 100} // зелёный оттенок фокуса
	case theme.ColorNamePressed:
		return color.NRGBA{R: 190, G: 190, B: 200, A: 255}

	// ================== ТЕКСТ =======================
	case theme.ColorNameForeground:
		return color.NRGBA{R: 45, G: 45, B: 50, A: 255} // тёмно‑серый, мягкий
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 150, G: 150, B: 155, A: 255}
	case theme.ColorNameError:
		return color.NRGBA{R: 200, G: 40, B: 40, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 160, G: 160, B: 168, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 90, G: 135, B: 60, A: 255} // тёмно‑зелёный акцент
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 60, G: 100, B: 180, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 190, G: 190, B: 195, A: 200}
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 245, G: 245, B: 248, A: 200}

	// ================== ВЫДЕЛЕНИЕ ===================
	case theme.ColorNameSelection:
		return color.NRGBA{R: 100, G: 150, B: 100, A: 120} // зелёное выделение

	// ================== РАЗДЕЛИТЕЛИ =================
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 200, G: 200, B: 208, A: 255}

	// ================== ТЕНИ ========================
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 60} // лёгкая тень

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 227, G: 227, B: 234, A: 255}

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
