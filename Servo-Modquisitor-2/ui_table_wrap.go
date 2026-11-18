// Servo-Modquisitor-2/ui_table_wrap.go
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

// newTableBackground создаёт слой с фоновым изображением (mechanicus),
// на который затем накладываются переданные объекты (таблица, рамки и т.д.).
//
// Если изображение не найдено, возвращает обычный стек без фона.
func (app *App) newTableBackground(opacity float64, objects ...fyne.CanvasObject) *fyne.Container {
	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	if mechData == nil {
		return container.NewStack(objects...)
	}

	mechBg := canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
	mechBg.FillMode = canvas.ImageFillContain
	mechBg.Translucency = opacity // Теперь типы совпадают

	layers := make([]fyne.CanvasObject, 0, len(objects)+1)
	layers = append(layers, mechBg)
	layers = append(layers, objects...)

	return container.NewStack(layers...)
}
