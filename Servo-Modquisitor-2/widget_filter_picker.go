// Servo-Modquisitor-2/widget_filter_picker.go
package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// FilterPickerOption — одна опция фильтра.
type FilterPickerOption struct {
	Label string
	Icon  fyne.Resource // PNG из assets/buttons/. nil допустим (без иконки).
}

// FilterPicker — замена widget.Select для фильтра с PNG-иконками.
//
// Устроен так:
//   - Кнопка (CustomButton) показывает иконку+текст текущего выбора.
//     Клик открывает немодальный popup со списком, спозиционированный
//     прямо под кнопкой (как обычный выпадающий список).
//   - Список — widget.List с ленивой отрисовкой и переиспользованием
//     строк. Hover-подсветку и обработку клика делает List, наша
//     filterPickerRow — только layout (иконка + текст).
//   - Выбор закрывает popup и вызывает OnChanged(idx).
//   - Клик мимо popup закрывает его (встроенное поведение Fyne PopUp).
//
// Не пытайтесь использовать Selected() для получения строки —
// возвращается индекс. Если нужна строка — передайте её через
// замыкание OnChanged.
type FilterPicker struct {
	widget.BaseWidget

	options   []FilterPickerOption
	selected  int
	OnChanged func(int) // индекс выбранной опции

	btn       *CustomButton
	parentWin fyne.Window
	pop       *widget.PopUp
}

// NewFilterPicker создаёт пикер с заданными опциями.
//
// parentWin — окно, в котором живёт виджет (нужно для показа popup).
// onChanged может быть nil.
func (app *App) NewFilterPicker(parentWin fyne.Window, options []FilterPickerOption, onChanged func(int)) *FilterPicker {
	fp := &FilterPicker{
		options:   options,
		parentWin: parentWin,
		OnChanged: onChanged,
		selected:  0,
	}
	fp.ExtendBaseWidget(fp)

	// Кнопка без иконки и текста — заполним через updateButton.
	fp.btn = NewCustomIconButton("", nil, func() { fp.showPopUp() })
	fp.btn.SetIconSize(20) // меньше стандартных 24, чтобы не раздувать панель
	fp.updateButton()

	return fp
}

func (fp *FilterPicker) updateButton() {
	if fp.selected < 0 || fp.selected >= len(fp.options) {
		return
	}
	opt := fp.options[fp.selected]
	fp.btn.SetText(opt.Label)
	fp.btn.SetIcon(opt.Icon)
	fp.btn.Refresh()
}

// SetLabels обновляет тексты опций (после смены языка). Сами
// иконки не трогаем — они не зависят от языка.
func (fp *FilterPicker) SetLabels(labels []string) {
	for i := range fp.options {
		if i < len(labels) {
			fp.options[i].Label = labels[i]
		}
	}
	fp.updateButton()
}

// Select программно выбирает опцию и вызывает OnChanged.
// Как widget.Select.SetSelected.
func (fp *FilterPicker) Select(idx int) {
	if idx < 0 || idx >= len(fp.options) || idx == fp.selected {
		return
	}
	fp.selected = idx
	fp.updateButton()
	if fp.OnChanged != nil {
		fp.OnChanged(idx)
	}
}

// Selected возвращает индекс текущей опции.
func (fp *FilterPicker) Selected() int {
	return fp.selected
}

func (fp *FilterPicker) showPopUp() {
	if fp.parentWin == nil || fp.parentWin.Canvas() == nil {
		return
	}
	if fp.pop != nil {
		fp.pop.Hide()
		fp.pop = nil
	}

	list := widget.NewList(
		func() int { return len(fp.options) },
		func() fyne.CanvasObject { return newFilterPickerRow() },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			row, ok := o.(*filterPickerRow)
			if !ok || id < 0 || id >= len(fp.options) {
				return
			}
			opt := fp.options[id]
			row.icon.Resource = opt.Icon
			row.icon.Refresh()
			row.label.SetText(opt.Label)
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		if id < 0 || id >= len(fp.options) {
			return
		}
		fp.Select(id)
		if fp.pop != nil {
			fp.pop.Hide()
			fp.pop = nil
		}
	}

	// Выясняем реальную высоту одной строки через MinSize шаблона.
	// CreateRenderer() обязателен: без него BaseWidget.MinSize()
	// возвращает 0, так как рендерер создаётся лениво.
	template := newFilterPickerRow()
	template.CreateRenderer()
	rowHeight := template.MinSize().Height
	if rowHeight <= 0 {
		rowHeight = 44 // fallback
	}

	// Ширина = ширина кнопки, но не меньше 240px.
	btnSize := fp.btn.Size()
	popupWidth := fyne.Max(btnSize.Width, 240)

	// Высота = все строки + отступы. Потолок — 400px, чтобы
	// длинный список не растянулся на весь экран (тогда появится
	// вертикальный скролл).
	popupHeight := float32(len(fp.options))*rowHeight + 25
	if popupHeight > 400 {
		popupHeight = 400
	}

	listScroll := container.NewVScroll(list)
	listScroll.SetMinSize(fyne.NewSize(popupWidth, popupHeight))

	btnPos := fyne.CurrentApp().Driver().AbsolutePositionForObject(fp.btn)
	pos := fyne.NewPos(btnPos.X, btnPos.Y+btnSize.Height)

	canvasSize := fp.parentWin.Canvas().Size()
	if pos.Y+popupHeight > canvasSize.Height {
		if above := btnPos.Y - popupHeight; above >= 0 {
			pos.Y = above
		}
	}
	if pos.X+popupWidth > canvasSize.Width {
		pos.X = canvasSize.Width - popupWidth
	}
	if pos.X < 0 {
		pos.X = 0
	}

	fp.pop = widget.NewPopUp(listScroll, fp.parentWin.Canvas())
	fp.pop.SetSize(fyne.NewSize(popupWidth, popupHeight))
	fp.pop.ShowAtPosition(pos)
}

// CreateRenderer — FilterPicker сам не рисует, всё делает вложенная
// кнопка. Рендерер-обёртка нужна, чтобы FilterPicker был полноценным
// widget'ом (иначе ExtendBaseWidget работать не будет).
func (fp *FilterPicker) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(fp.btn)
}

// ─────────────────────────────────────────────────────────────────
// filterPickerRow — строка внутри popup: иконка + текст.
// Hover-подсветку и обработку клика делает сам widget.List.
// ─────────────────────────────────────────────────────────────────

type filterPickerRow struct {
	widget.BaseWidget

	icon  *canvas.Image
	label *widget.Label
}

func newFilterPickerRow() *filterPickerRow {
	icon := canvas.NewImageFromResource(nil)
	icon.FillMode = canvas.ImageFillContain
	icon.SetMinSize(fyne.NewSize(20, 20))

	row := &filterPickerRow{
		icon:  icon,
		label: widget.NewLabel(""),
	}
	row.ExtendBaseWidget(row)
	return row
}

func (r *filterPickerRow) CreateRenderer() fyne.WidgetRenderer {
	// Иконка слева с отступом, текст растягивается. Border гарантирует,
	// что содержимое заполнит высоту строки.
	content := container.NewBorder(
		nil, nil,
		container.NewPadded(r.icon),
		nil,
		container.NewPadded(r.label),
	)
	// Spacer задаёт высоту 36 px — List использует MinSize() элемента
	// для вычисления высоты строки. Без него строка схлопнется до
	// высоты текста (~20 px), и иконка будет «прилипать».
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, 36))
	return widget.NewSimpleRenderer(container.NewStack(spacer, content))
}
