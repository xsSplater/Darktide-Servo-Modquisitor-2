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
	case theme.ColorNameBackground:
		return color.NRGBA{R: 32, G: 32, B: 32, A: 255}
	// Фон выпадающих меню (строки меню, контекстные меню)
	case theme.ColorNameMenuBackground:
		return color.NRGBA{R: 42, G: 42, B: 42, A: 255} // чуть светлее основного фона
	// Фон полей ввода (например, текстовые поля) – у нас прозрачный, чтобы CRT-фон был виден
	case theme.ColorNameInputBackground:
		return color.Transparent // return color.NRGBA{R: 22, G: 22, B: 22, A: 255}
	// Фон всплывающих окон (диалогов) – как основной фон
	case theme.ColorNameOverlayBackground:
		return color.NRGBA{R: 32, G: 32, B: 32, A: 255}
	// Цвет текста на предупреждающем/ошибочном фоне (используется редко, в основном для обозначения опасных действий)
	case theme.ColorNameForegroundOnWarning:
		return color.NRGBA{R: 255, G: 32, B: 32, A: 255}

	// ================== КНОПКИ ======================
	// Основной цвет обычной кнопки
	case theme.ColorNameButton:
		return color.NRGBA{R: 50, G: 50, B: 55, A: 255}
	// Цвет кнопки, на которую навели мышь (hover)
	case theme.ColorNameHover:
		return color.NRGBA{R: 70, G: 150, B: 38, A: 255}
	// Цвет заблокированной (неактивной) кнопки
	case theme.ColorNameDisabledButton:
		return color.NRGBA{R: 20, G: 20, B: 20, A: 255}
	// Индикатор фокуса (обводка активного элемента) – обычно едва заметный
	case theme.ColorNameFocus:
		return color.NRGBA{R: 80, G: 130, B: 200, A: 128}
	case theme.ColorNamePressed:
		return color.NRGBA{R: 60, G: 60, B: 68, A: 255}

	// ================== ТЕКСТ =======================
	// Основной цвет текста
	case theme.ColorNameForeground:
		return color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	// Цвет текста неактивных (заблокированных) виджетов (например, disabled‑кнопок)
	case theme.ColorNameDisabled:
		return color.NRGBA{R: 190, G: 255, B: 110, A: 255} // салатовый (CRT‑стиль)
		// return color.NRGBA{R: 100, G: 100, B: 100, A: 255}
	// Цвет сообщения об ошибке (красный текст)
	case theme.ColorNameError:
		return color.NRGBA{R: 220, G: 60, B: 60, A: 255}
	// Цвет текста-подсказки (placeholder) в полях ввода
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 100, G: 100, B: 110, A: 255}
	// Основной акцентный цвет темы (используется для заголовков, индикаторов важности)
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 60, G: 160, B: 30, A: 255}
	// Цвет гиперссылок (в Fyne они могут отображаться как текст)
	case theme.ColorNameHyperlink:
		return color.NRGBA{R: 80, G: 140, B: 220, A: 255}

	// ================== СКРОЛЛБАРЫ ==================
	// Цвет ползунка скроллбара
	case theme.ColorNameScrollBar:
		return color.NRGBA{R: 60, G: 150, B: 65, A: 200}
	// Цвет дорожки (фона) скроллбара
	case theme.ColorNameScrollBarBackground:
		return color.NRGBA{R: 22, G: 22, B: 22, A: 200}

	// ================== ВЫДЕЛЕНИЕ ===================
	// Цвет выделения текста (например, при выделении мышью)
	case theme.ColorNameSelection:
		return color.NRGBA{R: 80, G: 140, B: 220, A: 100}

	// ================== РАЗДЕЛИТЕЛИ =================
	// Цвет разделительных линий (например, widget.NewSeparator())
	case theme.ColorNameSeparator:
		return color.NRGBA{R: 5, G: 5, B: 5, A: 255}

	// ================== ТЕНИ ========================
	// Цвет тени окон и всплывающих элементов
	case theme.ColorNameShadow:
		return color.NRGBA{R: 0, G: 0, B: 0, A: 180}

	// ================== ЗАГОЛОВОК ===================
	case theme.ColorNameHeaderBackground:
		return color.NRGBA{R: 40, G: 40, B: 44, A: 255}

	// ================== ИНПУТ =======================
	case theme.ColorNameInputBorder:
		return color.NRGBA{R: 190, G: 255, B: 110, A: 255} // салатовый (CRT‑стиль)
		// return color.NRGBA{R: 70, G: 70, B: 78, A: 255}

	// Если вдруг запрошен неизвестный цвет – отдаём белый (заглушка)
	default:
		return theme.DefaultTheme().Color(name, variant)
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
