// Servo-Modquisitor-2/ui_tree_branch.go
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

const (
	treeIndent     float32 = 36 // Отступ от левого края до вертикальной линии
	treeLineWidth  float32 = 2  // Толщина линий
	treeLineLength float32 = 16 // Длина горизонтальной линии
)

// makeTreeBranch создаёт ячейку таблицы с нарисованной веткой дерева.
// isLast == true — рисует уголок (└), иначе перекрестие (├).
func (app *App) makeTreeBranch(isLast bool, content fyne.CanvasObject) fyne.CanvasObject {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	// Цвет линий — берём из темы (Primary — это акцентный цвет, обычно зелёный)
	lineColor := th.Color(theme.ColorNamePrimary, variant)

	vLine := canvas.NewRectangle(lineColor)
	hLine := canvas.NewRectangle(lineColor)

	// Кастомный layout для конкретной ячейки
	layout := &treeRowLayout{
		isLast: isLast,
	}

	return container.New(layout, vLine, hLine, content)
}

// treeRowLayout — layout с учётом того, последний ли это элемент.
type treeRowLayout struct {
	isLast bool
}

func (t *treeRowLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	// Минимальная ширина — отступ + линия + отступ до текста.
	// Высота — не меньше 30, но растёт под многострочный контент
	// (имя партнёра + описание конфликта в батч-диалоге).
	h := float32(30)
	if len(objects) >= 3 {
		if ch := objects[2].MinSize().Height; ch > h {
			h = ch
		}
	}
	return fyne.NewSize(treeIndent+treeLineLength+8, h)
}

func (t *treeRowLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	vLine := objects[0]
	hLine := objects[1]
	content := objects[2]

	// Позиция середины по вертикали
	midY := size.Height / 2

	// Вертикальная линия
	if t.isLast {
		// Последний элемент: линия идёт до середины.
		// Прибавляем половину толщины линии, чтобы она идеально слилась
		// с горизонтальной линией и не было зазора в уголке.
		vLine.Resize(fyne.NewSize(treeLineWidth, midY+(treeLineWidth)))
		vLine.Move(fyne.NewPos(treeIndent, -2))
	} else {
		// Обычный элемент: растягиваем линию на 1 пиксель вверх и вниз
		// и смещаем на полпикселя, чтобы перекрыть зазоры между строками.
		vLine.Resize(fyne.NewSize(treeLineWidth, size.Height+4))
		vLine.Move(fyne.NewPos(treeIndent, -2)) //-0.5
	}

	// Горизонтальная линия
	hLine.Resize(fyne.NewSize(treeLineLength, treeLineWidth))
	hLine.Move(fyne.NewPos(treeIndent, midY-(treeLineWidth/2)))

	// Контент (текст)
	textX := treeIndent + treeLineLength
	content.Resize(fyne.NewSize(size.Width-textX, size.Height))
	content.Move(fyne.NewPos(textX, 0))
}
