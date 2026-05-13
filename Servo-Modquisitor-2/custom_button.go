// custom_button.go
package main

import (
	"Servo-Modquisitor/themes"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type _CustomButtonRenderer struct {
	btn     *CustomButton
	shadow  *canvas.Rectangle // тень
	bg      *canvas.Rectangle // основной фон
	bgImage *canvas.Image     // фоновое изображение (опционально)
	text    *canvas.Text
}

func (r *_CustomButtonRenderer) Layout(size fyne.Size) {
	// Тень: чуть смещена вниз‑вправо, чуть меньше фона
	shadowPad := float32(1)
	r.shadow.Resize(fyne.NewSize(size.Width+shadowPad, size.Height+shadowPad))
	r.shadow.Move(fyne.NewPos(shadowPad, shadowPad+1))

	r.bg.Resize(size)
	if r.bgImage != nil {
		r.bgImage.Resize(size)
	}
	r.text.Resize(size)
	r.text.Alignment = fyne.TextAlignCenter
	r.updateColors()
}

func (r *_CustomButtonRenderer) MinSize() fyne.Size {
	min := r.text.MinSize()
	min.Width += 2*theme.Padding() + 8
	min.Height += 2*theme.Padding() + 8
	return min
}

func (r *_CustomButtonRenderer) Refresh() {
	r.text.Text = r.btn.text
	r.updateColors()
	r.text.Refresh()
	r.bg.Refresh()
	if r.bgImage != nil {
		r.bgImage.Refresh()
	}
	r.shadow.Refresh()
}

func (r *_CustomButtonRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.shadow}
	if r.bgImage != nil {
		objs = append(objs, r.bgImage)
	}
	objs = append(objs, r.bg, r.text)
	return objs
}

func (r *_CustomButtonRenderer) Destroy() {}

func (r *_CustomButtonRenderer) updateColors() {
    btn := r.btn
    th := fyne.CurrentApp().Settings().Theme()
    variant := fyne.CurrentApp().Settings().ThemeVariant()

    // Тень
    if btn.disabled {
        r.shadow.FillColor = th.Color(themes.ColorButtonShadowDisabled, variant)
    } else {
        r.shadow.FillColor = th.Color(themes.ColorButtonShadow, variant)
    }

    r.bg.StrokeColor = th.Color(themes.ColorButtonStroke, variant)

    if btn.disabled {
        r.bg.FillColor = th.Color(theme.ColorNameDisabledButton, variant)
        r.text.Color = th.Color(theme.ColorNameDisabled, variant)
        return
    }

    // Если есть фоновое изображение
    if r.bgImage != nil {
        r.bg.FillColor = color.Transparent
        r.bg.StrokeColor = th.Color(themes.ColorButtonStrokeImage, variant)
        if btn.Importance == widget.WarningImportance {
            r.text.Color = th.Color(theme.ColorNameForegroundOnWarning, variant)
        } else {
            r.text.Color = th.Color(theme.ColorNameForeground, variant)
        }
        return
    }

    // Обычные состояния
    switch {
    case btn.pressed && btn.hovered:
        r.bg.FillColor = th.Color(theme.ColorNamePressed, variant)
        r.text.Color = th.Color(theme.ColorNameForeground, variant)
    case btn.hovered:
        r.bg.FillColor = th.Color(theme.ColorNameHover, variant)
        r.text.Color = th.Color(theme.ColorNameForeground, variant)
    default:
        if btn.Importance == widget.WarningImportance {
            r.bg.FillColor = th.Color(theme.ColorNamePrimary, variant)
            r.text.Color = th.Color(theme.ColorNameForegroundOnWarning, variant)
        } else {
            r.bg.FillColor = th.Color(theme.ColorNameButton, variant)
            r.text.Color = th.Color(theme.ColorNameForeground, variant)
        }
    }
}

type CustomButton struct {
	widget.BaseWidget
	text       string
	OnTapped   func()
	Importance widget.Importance

	hovered  bool
	pressed  bool
	focused  bool
	disabled bool

	OnMouseIn    func()
	OnMouseOut   func()
	OnMouseMoved func(*desktop.MouseEvent)

	bgImage *canvas.Image // необязательное фоновое изображение
}

func NewCustomButton(label string, tapped func()) *CustomButton {
	b := &CustomButton{
		text:     label,
		OnTapped: tapped,
	}
	b.ExtendBaseWidget(b)
	return b
}

// SetBackgroundImage задаёт фоновое изображение для кнопки.
func (b *CustomButton) SetBackgroundImage(img *canvas.Image) {
	b.bgImage = img
	b.Refresh()
}

func (b *CustomButton) SetText(text string) {
	b.text = text
	b.Refresh()
}

func (b *CustomButton) Tapped(e *fyne.PointEvent) {
	if b.disabled {
		return
	}
	if b.OnTapped != nil {
		b.OnTapped()
	}
}

func (b *CustomButton) MouseIn(event *desktop.MouseEvent) {
	b.hovered = true
	b.Refresh()
	if b.OnMouseIn != nil {
		b.OnMouseIn()
	}
}

func (b *CustomButton) MouseOut() {
	b.hovered = false
	b.pressed = false
	b.Refresh()
	if b.OnMouseOut != nil {
		b.OnMouseOut()
	}
}

func (b *CustomButton) MouseMoved(event *desktop.MouseEvent) {
	if b.OnMouseMoved != nil {
		b.OnMouseMoved(event)
	}
}

func (b *CustomButton) FocusGained() {
	b.focused = true
	b.Refresh()
}

func (b *CustomButton) FocusLost() {
	b.focused = false
	b.Refresh()
}

func (b *CustomButton) TypedKey(event *fyne.KeyEvent) {}
func (b *CustomButton) TypedRune(r rune)              {}

func (b *CustomButton) Disabled() bool { return b.disabled }
func (b *CustomButton) Enable()        { b.disabled = false; b.Refresh() }
func (b *CustomButton) Disable()       { b.disabled = true; b.Refresh() }

func (b *CustomButton) CreateRenderer() fyne.WidgetRenderer {
	shadow := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 100})
	shadow.CornerRadius = 5

	bg := canvas.NewRectangle(theme.ButtonColor())
	bg.CornerRadius = 5

	txt := canvas.NewText(b.text, theme.ForegroundColor())
	txt.Alignment = fyne.TextAlignCenter
	txt.TextStyle.Bold = true

	renderer := &_CustomButtonRenderer{
		btn:    b,
		shadow: shadow,
		bg:     bg,
		text:   txt,
	}
	// Если у кнопки уже установлено изображение, передаём его в рендерер
	if b.bgImage != nil {
		renderer.bgImage = b.bgImage
	}
	return renderer
}

var _ fyne.Tappable = (*CustomButton)(nil)
var _ desktop.Hoverable = (*CustomButton)(nil)
var _ fyne.Focusable = (*CustomButton)(nil)
var _ fyne.Disableable = (*CustomButton)(nil)
