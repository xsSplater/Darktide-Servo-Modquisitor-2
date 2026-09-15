// Servo-Modquisitor-2/themes/custom.go
package themes

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// CustomTheme позволяет пользователю задать цвета индивидуально.
//
// Base — тема-основа, к которой откатываемся для всех имён, которых нет
// в Colors. Если Base == nil — используется ForcedDarkTheme. Это важно:
// switch по именам здесь больше не нужен, потому что новые цвета,
// добавленные в xssyne (menuBarAccent, menuItemHeaderBg и т.д.),
// автоматически попадают в Base и не превращаются в «белый винегрет»
// из theme.DefaultTheme().
type CustomTheme struct {
	Colors map[string]color.Color
	Base   fyne.Theme
}

func (t CustomTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if c, ok := t.Colors[string(name)]; ok {
		return c
	}
	base := t.Base
	if base == nil {
		base = ForcedDarkTheme{}
	}
	return base.Color(name, variant)
}

func (t CustomTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.Base != nil {
		return t.Base.Font(style)
	}
	return theme.DefaultTheme().Font(style)
}

func (t CustomTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	if t.Base != nil {
		return t.Base.Icon(name)
	}
	return theme.DefaultTheme().Icon(name)
}

func (t CustomTheme) Size(name fyne.ThemeSizeName) float32 {
	if t.Base != nil {
		return t.Base.Size(name)
	}
	return theme.DefaultTheme().Size(name)
}
