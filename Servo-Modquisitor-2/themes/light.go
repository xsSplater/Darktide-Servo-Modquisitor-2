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
	case theme.ColorNameBackground:		return color.NRGBA{R: 222, G: 225, B: 222, A: 200}
	case theme.ColorNameMenuBackground:	return color.NRGBA{R: 250, G: 250, B: 252, A: 225}
	case theme.ColorNameInputBackground:
										return color.NRGBA{R: 255, G: 255, B: 255, A: 200} 
	case theme.ColorNameOverlayBackground:
										return color.NRGBA{R: 237, G: 237, B: 242, A: 255}
	case theme.ColorNameForegroundOnWarning:
										return color.NRGBA{R: 200, G: 40,  B: 40,  A: 255}

	// ================== КНОПКИ ======================
	case theme.ColorNameButton:			return color.NRGBA{R: 215, G: 215, B: 222, A: 255}
	case theme.ColorNameHover:			return color.NRGBA{R: 180, G: 210, B: 160, A: 255}
	case theme.ColorNameDisabledButton:
										return color.NRGBA{R: 225, G: 225, B: 230, A: 255}
	case theme.ColorNameFocus:			return color.NRGBA{R: 100, G: 140, B: 100, A: 100}
	case theme.ColorNamePressed:		return color.NRGBA{R: 190, G: 190, B: 200, A: 255}

	// ================== ТЕКСТ =======================
	case theme.ColorNameForeground:		return color.NRGBA{R: 45,  G: 45,  B: 50,  A: 255}
	case theme.ColorNameDisabled:		return color.NRGBA{R: 150, G: 150, B: 155, A: 255}
	case theme.ColorNameError:			return color.NRGBA{R: 200, G: 40,  B: 40,  A: 255}
	case theme.ColorNamePlaceHolder:	return color.NRGBA{R: 160, G: 160, B: 168, A: 255}
	case theme.ColorNamePrimary:		return color.NRGBA{R: 90,  G: 135, B: 60,  A: 255}
	case theme.ColorNameHyperlink:		return color.NRGBA{R: 60,  G: 100, B: 180, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	case theme.ColorNameScrollBar:		return color.NRGBA{R: 190, G: 190, B: 195, A: 200}
	case theme.ColorNameScrollBarBackground:
										return color.NRGBA{R: 245, G: 245, B: 248, A: 200}

	// ================== ВЫДЕЛЕНИЕ ===================
	case theme.ColorNameSelection:		return color.NRGBA{R: 100, G: 150, B: 100, A: 120} 

	// ================== РАЗДЕЛИТЕЛИ =================
	case theme.ColorNameSeparator:		return color.NRGBA{R: 200, G: 200, B: 208, A: 255}

	// ================== ТЕНИ ========================
	case theme.ColorNameShadow:			return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 60 }

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
										return color.NRGBA{R: 227, G: 227, B: 234, A: 255}

	// ================== ИНПУТ =======================
	case theme.ColorNameInputBorder:	return color.NRGBA{R: 180, G: 180, B: 190, A: 255}

    // Статусы (яркость снижена для светлого фона)
    case ColorStatusSystem:   			return color.NRGBA{R: 20,  G: 100, B: 180, A: 255}
    case ColorStatusBroken:   			return color.NRGBA{R: 200, G: 40,  B: 40,  A: 255}
    case ColorStatusConflict: 			return color.NRGBA{R: 220, G: 120, B: 0,   A: 255}
    case ColorStatusObsolete: 			return color.NRGBA{R: 140, G: 140, B: 0,   A: 255}
    case ColorStatusMandatory:			return color.NRGBA{R: 0,   G: 130, B: 0,   A: 255}
    case ColorStatusActive:   			return color.NRGBA{R: 70,  G: 150, B: 70,  A: 255}
    case ColorStatusInactive: 			return color.NRGBA{R: 140, G: 140, B: 145, A: 255}

    // Таблица
    case ColorTableRowEven:     		return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
    case ColorTableRowOdd:      		return color.NRGBA{R: 240, G: 240, B: 245, A: 255}
    case ColorTableRowSelected:			return color.NRGBA{R: 160, G: 210, B: 130, A: 120}
    case ColorTableRowConflict:			return color.NRGBA{R: 255, G: 230, B: 200, A: 180}
    case ColorTableBorderDirty:			return color.NRGBA{R: 200, G: 0,   B: 0,   A: 255}
    case ColorTableHeaderBg:    		return color.NRGBA{R: 227, G: 227, B: 234, A: 255}
    case ColorSystemTableBg:    		return color.NRGBA{R: 235, G: 235, B: 240, A: 200}

    // Консоль
	case ColorConsoleText:				return color.NRGBA{R: 192, G: 255, B: 26,  A: 255}
    case ColorCRTScreenFill:			return color.NRGBA{R: 40,  G: 40,  B: 40,  A: 20 }
    case ColorCRTScreenStroke:			return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 40 }
    case ColorCRTHeaderBg:				return color.NRGBA{R: 245, G: 245, B: 248, A: 200}

    // Панели
	case ColorDescCardStroke:			return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 40 }
    case ColorDescCardBg:				return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
    case ColorManagePanelBg:			return color.NRGBA{R: 245, G: 245, B: 248, A: 255}
    case ColorTopPanelBg:				return color.NRGBA{R: 237, G: 237, B: 242, A: 255}
    case ColorTipBg:					return color.NRGBA{R: 245, G: 245, B: 248, A: 230}

    // Кастомные кнопки
    case ColorButtonShadow:         	return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 25 }
    case ColorButtonShadowDisabled: 	return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 10 }
    case ColorButtonStroke:         	return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 50 }
    case ColorButtonStrokeImage:    	return color.NRGBA{R: 0,   G: 0,   B: 0,   A: 30 }

	default:							return theme.DefaultTheme().Color(name, variant)
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
