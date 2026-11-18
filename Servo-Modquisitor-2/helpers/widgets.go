// Servo-Modquisitor-2/helpers/widgets.go
package helpers

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// NewFixedSpacer возвращает прозрачный прямоугольник фиксированного
// размера.
func NewFixedSpacer(w, h float32) *canvas.Rectangle {
	r := canvas.NewRectangle(color.Transparent)
	r.SetMinSize(fyne.NewSize(w, h))
	return r
}

// BoldLabel — widget.Label с жирным шрифтом, выравнивание по левому краю.
func BoldLabel(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
}

// BoldCenterLabel — widget.Label с жирным шрифтом, по центру.
func BoldCenterLabel(text string) *widget.Label {
	return widget.NewLabelWithStyle(text, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
}
