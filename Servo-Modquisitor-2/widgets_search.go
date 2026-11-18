// Servo-Modquisitor-2/widgets_search.go
//
// Единый конструктор поля поиска. Устраняет дублирование
// (spacer + entry + clear + debounce + escape), которое раньше было
// раскидано по ui_build.go, aml_config.go, sorting_rules_editor.go
// и ui_mod_browser.go с расхождениями в мелочах (разная иконка
// очистки, разная реакция на Escape, разные debounce).
package main

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SearchFieldOptions — параметры конструктора поля поиска.
type SearchFieldOptions struct {
	// Placeholder — текст-заглушка. Пустая строка — берётся из
	// messages.json по ключу "search_placeholder".
	Placeholder string

	// Width — минимальная ширина поля (spacer). 0 — без spacer'а:
	// поле сжимается до Entry.MinSize. Для popup'ов удобно 0.
	Width float32

	// DebounceDelay — задержка перед OnFilter. 0 — OnFilter
	// вызывается сразу на каждое изменение.
	DebounceDelay time.Duration

	// OnFilter — вызывается при изменении текста (с debounce,
	// если DebounceDelay > 0).
	OnFilter func(query string)

	// OnSubmitted — вызывается по Enter. nil — Enter ничего не делает.
	OnSubmitted func(query string)

	// OnEscape — вызывается по Escape. nil — стандартное
	// поведение: очистить текст; если уже пусто — снять фокус
	// с активного окна.
	OnEscape func()

	// ShowClear — показывать кнопку очистки (крестик).
	ShowClear bool

	// ClearIcon — иконка кнопки очистки. nil — стандартная
	// "assets/buttons/backsp.png".
	ClearIcon fyne.Resource

	// Theme — тема для всего поля (в т.ч. Entry). nil — тема
	// приложения. Используется мод-браузером для розовой/синей
	// рамки.
	Theme fyne.Theme
}

// SearchField — готовый набор: Entry, кнопка очистки, layout-контейнер.
type SearchField struct {
	// Entry — внутренний Entry. Всегда не nil.
	Entry *widget.Entry

	// ClearBtn — кнопка очистки. nil, если ShowClear == false.
	ClearBtn *CustomButton

	// Container — готовый узел для вставки в layout.
	Container fyne.CanvasObject
}

// NewSearchField собирает поиск с debounce, кнопкой очистки, spacer'ом
// под минимальную ширину и опциональной кастомной темой.
//
// Единая реализация для главного окна, AML-редактора, редактора правил
// и мод-браузера. Поведение:
//   - clear-кнопка автоматически показывается/скрывается по
//     непустому тексту;
//   - debounce-таймер локальный для каждого поля, окна не мешают
//     друг другу;
//   - OnFilter вызывается через DebounceDelay после последнего
//     изменения (или сразу, если 0);
//   - Escape без явного OnEscape чистит текст; если уже пусто —
//     снимает фокус с mainWindow.
func (app *App) NewSearchField(opts SearchFieldOptions) *SearchField {
	entry := widget.NewEntry()

	placeholder := opts.Placeholder
	if placeholder == "" {
		placeholder = app.msg("search_placeholder")
	}
	entry.SetPlaceHolder(placeholder)

	// ── Spacer под минимальную ширину ─────────────────────────
	var entryDisplay fyne.CanvasObject = entry
	if opts.Width > 0 {
		spacer := canvas.NewRectangle(color.Transparent)
		spacer.SetMinSize(fyne.NewSize(opts.Width, 1))
		entryDisplay = container.NewStack(spacer, entry)
	}

	// ── Кнопка очистки ────────────────────────────────────────
	var clearBtn *CustomButton
	if opts.ShowClear {
		icon := opts.ClearIcon
		if icon == nil {
			icon = app.loadIconResource("clear", "assets/buttons/backsp.png")
		}
		clearBtn = NewIconButton(icon, func() {
			entry.SetText("")
		})
		clearBtn.Importance = widget.DangerImportance
		clearBtn.Hide()
	}

	// ── Debounce + show/hide clear-кнопки ─────────────────────
	var debounce *time.Timer
	entry.OnChanged = func(s string) {
		if clearBtn != nil {
			if s != "" {
				clearBtn.Show()
			} else {
				clearBtn.Hide()
			}
			clearBtn.Refresh()
		}
		if opts.OnFilter == nil {
			return
		}
		if opts.DebounceDelay <= 0 {
			opts.OnFilter(s)
			return
		}
		if debounce != nil {
			debounce.Stop()
		}
		query := s
		debounce = time.AfterFunc(opts.DebounceDelay, func() {
			fyne.Do(func() { opts.OnFilter(query) })
		})
	}

	// ── Enter ─────────────────────────────────────────────────
	if opts.OnSubmitted != nil {
		entry.OnSubmitted = opts.OnSubmitted
	}

	// ── Escape ────────────────────────────────────────────────
	entry.SetEscapeHandler(func() {
		if opts.OnEscape != nil {
			opts.OnEscape()
			return
		}
		if entry.Text != "" {
			entry.SetText("")
			return
		}
		if app.mainWindow != nil && app.mainWindow.Canvas() != nil {
			app.mainWindow.Canvas().Unfocus()
		}
	})

	// ── Layout ────────────────────────────────────────────────
	var inner fyne.CanvasObject
	if clearBtn != nil {
		inner = container.NewBorder(nil, nil, nil, clearBtn, entryDisplay)
	} else {
		inner = entryDisplay
	}
	if opts.Theme != nil {
		inner = container.NewThemeOverride(inner, opts.Theme)
	}

	return &SearchField{
		Entry:     entry,
		ClearBtn:  clearBtn,
		Container: inner,
	}
}
