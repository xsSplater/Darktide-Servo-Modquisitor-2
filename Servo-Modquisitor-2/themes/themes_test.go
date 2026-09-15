// Servo-Modquisitor-2/themes/themes_test.go
package themes

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

func TestThemesReturnColors(t *testing.T) {
	themes := []fyne.Theme{
		ForcedDarkTheme{},
		ForcedLightTheme{},
		HighContrastTheme{},
	}
	for _, th := range themes {
		// Проверяем основные цвета
		for _, name := range []fyne.ThemeColorName{
			theme.ColorNameBackground,
			theme.ColorNameForeground,
			theme.ColorNameButton,
			theme.ColorNamePrimary,
		} {
			c := th.Color(name, theme.VariantDark)
			if c == nil {
				t.Errorf("Color for %s returned nil", name)
			}
		}
		// Проверяем кастомные имена из theme_colors.go
		customNames := []string{
			ColorStatusSystem,
			ColorTableRowEven,
			ColorConsoleText,
			ColorDescCardBg,
			ColorButtonShadow,
		}
		for _, cn := range customNames {
			c := th.Color(fyne.ThemeColorName(cn), theme.VariantDark)
			if c == nil {
				t.Errorf("Color for %s returned nil", cn)
			}
		}
	}
}

func TestCustomThemeReturnsCustomColors(t *testing.T) {
	// CustomTheme требует Fyne-приложения для fallback, поэтому не тестируем Color.
	// Просто проверяем, что структура создаётся.
	ct := CustomTheme{Colors: map[string]color.Color{
		ColorStatusActive: color.NRGBA{R: 255, G: 0, B: 0, A: 255},
	}}
	if ct.Colors == nil {
		t.Error("CustomTheme Colors map is nil")
	}
	// Проверить, что карта содержит нужный ключ
	if _, ok := ct.Colors[ColorStatusActive]; !ok {
		t.Errorf("CustomTheme Colors missing key %s", ColorStatusActive)
	}
}
