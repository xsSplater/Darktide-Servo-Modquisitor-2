// Servo-Modquisitor-2/custom_button.go
package main

import (
	"Servo-Modquisitor/themes"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type _CustomButtonRenderer struct {
	btn     *CustomButton
	shadow  *canvas.Rectangle
	bg      *canvas.Rectangle
	bgImage *canvas.Image
	text    *canvas.Text
	icon    *canvas.Image
}

func (r *_CustomButtonRenderer) Layout(size fyne.Size) {
	shadowPad := float32(1)
	r.shadow.Resize(fyne.NewSize(size.Width+shadowPad, size.Height+shadowPad))
	r.shadow.Move(fyne.NewPos(shadowPad, shadowPad+1))

	r.bg.Resize(size)
	if r.bgImage != nil {
		r.bgImage.Resize(size)
	}

	textMin := r.text.MinSize()
	if r.btn.text == "" {
		textMin = fyne.NewSize(0, 0)
	}

	iconSize := r.btn.iconSize
	if r.icon != nil && r.btn.hovered && !r.btn.disabled {
		iconSize = iconSize * 1.35 // Увеличение при наведении
	}

	if r.icon != nil && r.btn.text != "" {
		// Есть И иконка, И текст: центрируем их вместе как единый блок
		totalWidth := iconSize + theme.Padding() + textMin.Width
		startX := (size.Width - totalWidth) / 2

		// Иконка (слева)
		r.icon.Resize(fyne.NewSize(iconSize, iconSize))
		r.icon.Move(fyne.NewPos(startX, (size.Height-iconSize)/2))
		r.icon.Refresh()

		// Текст (справа от иконки)
		r.text.Resize(fyne.NewSize(textMin.Width, textMin.Height))
		r.text.Move(fyne.NewPos(startX+iconSize+theme.Padding(), (size.Height-textMin.Height)/2))
		r.text.Alignment = fyne.TextAlignLeading
	} else if r.icon != nil {
		// Только иконка: центрируем
		r.icon.Resize(fyne.NewSize(iconSize, iconSize))
		r.icon.Move(fyne.NewPos((size.Width-iconSize)/2, (size.Height-iconSize)/2))
		r.icon.Refresh()
	} else {
		// Только текст: центрируем
		r.text.Resize(size)
		r.text.Alignment = fyne.TextAlignCenter
	}

	r.updateColors()
}

func (r *_CustomButtonRenderer) MinSize() fyne.Size {
	textMin := r.text.MinSize()
	if r.btn.text == "" {
		textMin = fyne.NewSize(0, 0)
	}
	iconMin := fyne.NewSize(0, 0)
	if r.icon != nil {
		iconMin = fyne.NewSize(r.btn.iconSize, r.btn.iconSize)
	}

	var w, h float32
	if r.icon != nil && r.btn.text != "" {
		// Иконка + текст: складываем их ширину + отступ
		w = iconMin.Width + theme.Padding() + textMin.Width
		h = fyne.Max(iconMin.Height, textMin.Height)
	} else if r.icon != nil {
		// Только иконка
		w = iconMin.Width
		h = iconMin.Height
	} else {
		// Только текст
		w = textMin.Width
		h = textMin.Height
	}

	// Добавляем стандартные отступы по краям.
	// +8 было слишком много — раздувало высоту кнопок до ~46-50px.
	// +4 даёт комфортные ~36-40px.
	return fyne.NewSize(w+2*theme.Padding()+4, h+2*theme.Padding()+4)
}

func (r *_CustomButtonRenderer) updateIcon() {
	if r.icon == nil {
		return
	}
	if r.btn.icon != nil {
		r.icon.Resource = r.btn.icon
		r.icon.Refresh()
	}
}

func (r *_CustomButtonRenderer) Refresh() {
	r.text.Text = r.btn.text
	r.updateIcon()
	// Принудительно пересчитываем Layout, чтобы изменить размер иконки
	r.Layout(r.btn.Size())
	r.updateColors()
	r.text.Refresh()
	r.bg.Refresh()
	if r.bgImage != nil {
		r.bgImage.Refresh()
	}
	if r.icon != nil {
		r.icon.Refresh()
	}
	r.shadow.Refresh()
}

func (r *_CustomButtonRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.shadow}
	if r.bgImage != nil {
		objs = append(objs, r.bgImage)
	}
	objs = append(objs, r.bg)
	if r.icon != nil {
		objs = append(objs, r.icon)
	}
	objs = append(objs, r.text)
	return objs
}

func (r *_CustomButtonRenderer) Destroy() {}

func (r *_CustomButtonRenderer) updateColors() {
	btn := r.btn
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	if btn.disabled {
		r.shadow.FillColor = th.Color(themes.ColorButtonShadowDisabled, variant)
	} else {
		r.shadow.FillColor = th.Color(themes.ColorButtonShadow, variant)
	}

	// Рамка: если warningFrame — жёлтая, толще; иначе обычная.
	// Значение применяется ко всем ветвям ниже, потому что жёлтая
	// рамка должна быть видна всегда, независимо от hover/pressed/
	// disabled-состояния и наличия bgImage.
	if btn.warningFrame {
		r.bg.StrokeColor = th.Color(themes.ColorAMLWarning, variant)
		r.bg.StrokeWidth = 1
	} else {
		r.bg.StrokeColor = th.Color(themes.ColorButtonStroke, variant)
		r.bg.StrokeWidth = 1
	}

	if btn.disabled {
		r.bg.FillColor = th.Color(theme.ColorNameDisabledButton, variant)
		r.text.Color = th.Color(theme.ColorNameDisabled, variant)
		return
	}

	if r.bgImage != nil {
		r.bg.FillColor = color.Transparent
		// Рамка уже назначена выше. Если warningFrame не установлен —
		// используем обычный цвет для кнопки с фоном-картинкой.
		if !btn.warningFrame {
			r.bg.StrokeColor = th.Color(themes.ColorButtonStrokeImage, variant)
		}
		if btn.Importance == widget.WarningImportance {
			r.text.Color = th.Color(theme.ColorNameForegroundOnWarning, variant)
		} else {
			r.text.Color = th.Color(theme.ColorNameForeground, variant)
		}
		return
	}

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
	icon       fyne.Resource
	OnTapped   func()
	Importance widget.Importance

	hovered  bool
	pressed  bool
	focused  bool
	disabled bool

	warningFrame bool

	OnMouseIn    func()
	OnMouseOut   func()
	OnMouseMoved func(*desktop.MouseEvent)

	bgImage *canvas.Image

	iconSize float32 // размер иконки

	// iconImage — ссылка на canvas.Image внутри рендерера.
	// Создаётся в CreateRenderer при первом показе кнопки.
	// Нужна для анимации поворота иконки (SetIconRotation).
	//
	// До первого CreateRenderer — nil. SetIconRotation в этом
	// состоянии запоминает значение в pendingRotation и
	// применяет его, когда рендерер будет создан.
	iconImage        *canvas.Image
	pendingRotation  float32
	hasPendingRotate bool
}

// SetWarningFrame включает/выключает постоянную жёлтую обводку.
// Не путать с Importance=WarningImportance — та меняет фон, а
// warningFrame только рамку. Обе могут сосуществовать.
func (b *CustomButton) SetWarningFrame(on bool) {
	if b.warningFrame == on {
		return
	}
	b.warningFrame = on
	b.Refresh()
}

func NewCustomButton(label string, tapped func()) *CustomButton {
	b := &CustomButton{
		text:     label,
		OnTapped: tapped,
	}
	b.ExtendBaseWidget(b)
	return b
}

func NewIconButton(icon fyne.Resource, tapped func()) *CustomButton {
	b := &CustomButton{
		icon:     icon,
		OnTapped: tapped,

		iconSize: 24, // по умолчанию
	}
	b.ExtendBaseWidget(b)
	return b
}

// Метод для установки размера иконки
func (b *CustomButton) SetIconSize(size float32) {
	b.iconSize = size
	b.Refresh()
}

func (b *CustomButton) SetBackgroundImage(img *canvas.Image) {
	b.bgImage = img
	b.Refresh()
}

func (b *CustomButton) SetText(text string) {
	b.text = text
	b.Refresh()
}

// SetIcon меняет иконку кнопки. Если иконка не менялась — no-op.
//
// Специально НЕ вызывает Refresh, если значение идентично: внутри
// Refresh пересчитывается Layout (изменение размера иконки при hover,
// центрирование иконки и текста), и лишний пересчёт на каждое
// «обновить состояние» (а updateDescriptionForMod / FilterPicker.updateButton
// вызываются часто) даёт заметный оверхед на ровном месте.
//
// nil допустим — иконка просто исчезнет.
func (b *CustomButton) SetIcon(icon fyne.Resource) {
	if b.icon == icon {
		return
	}
	b.icon = icon
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
	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()

	shadow := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 100})
	shadow.CornerRadius = 5

	bg := canvas.NewRectangle(th.Color(theme.ColorNameButton, variant))
	bg.CornerRadius = 5

	txt := canvas.NewText(b.text, th.Color(theme.ColorNameForeground, variant))
	txt.Alignment = fyne.TextAlignCenter
	txt.TextStyle.Bold = true

	var iconImg *canvas.Image
	if b.icon != nil {
		iconImg = canvas.NewImageFromResource(b.icon)
		iconImg.FillMode = canvas.ImageFillContain
		iconImg.SetMinSize(fyne.NewSize(24, 24))
		// Применяем «отложенный» поворот, если SetIconRotation
		// вызывали до первого рендера (например, при восстановлении
		// состояния спойлера из кэша).
		if b.hasPendingRotate {
			iconImg.Rotation = b.pendingRotation
			b.hasPendingRotate = false
		}
		b.iconImage = iconImg
	}

	renderer := &_CustomButtonRenderer{
		btn:    b,
		shadow: shadow,
		bg:     bg,
		text:   txt,
		icon:   iconImg,
	}
	if b.bgImage != nil {
		renderer.bgImage = b.bgImage
	}
	return renderer
}

var _ fyne.Tappable = (*CustomButton)(nil)
var _ desktop.Hoverable = (*CustomButton)(nil)
var _ fyne.Focusable = (*CustomButton)(nil)
var _ fyne.Disableable = (*CustomButton)(nil)

func NewCustomIconButton(label string, icon fyne.Resource, tapped func()) *CustomButton {
	b := &CustomButton{
		text:     label,
		icon:     icon,
		OnTapped: tapped,
		iconSize: 24, // стандартный размер иконки в текстовой кнопке
	}
	b.ExtendBaseWidget(b)
	return b
}

// SetIconRotation задаёт угол поворота иконки в градусах по часовой
// стрелке. Не перерисовывает, если значение совпадает.
//
// Если рендерер ещё не создан (кнопка ни разу не показывалась) —
// значение запоминается и применяется, когда рендерер будет создан.
// Это позволяет восстанавливать состояние (например, «спойлер
// раскрыт») из кэша до первого показа кнопки на экране.
func (b *CustomButton) SetIconRotation(degrees float32) {
	if b.iconImage != nil {
		b.iconImage.SetRotation(degrees)
		return
	}
	if b.hasPendingRotate && b.pendingRotation == degrees {
		return
	}
	b.pendingRotation = degrees
	b.hasPendingRotate = true
}

// IconRotation возвращает текущий угол поворота иконки.
// 0 — если иконка не была повёрнута или рендерер ещё не создан.
func (b *CustomButton) IconRotation() float32 {
	if b.iconImage == nil {
		if b.hasPendingRotate {
			return b.pendingRotation
		}
		return 0
	}
	return b.iconImage.Rotation
}

// AnimateIconRotation плавно поворачивает иконку кнопки от from до to
// градусов за duration.
//
// Если btn == nil или iconImage == nil — no-op. В последнем случае
// (кнопка ещё не показана) можно передать желаемое конечное значение
// через btn.SetIconRotation(to) — оно запомнится в pendingRotation
// и применится при первом рендере.
//
// Пример: раскрытие спойлера — from=0, to=90; закрытие — from=90, to=0.
func AnimateIconRotation(btn *CustomButton, from, to float32, duration time.Duration) {
	if btn == nil {
		return
	}
	if btn.iconImage == nil {
		// Рендерер не готов — запоминаем конечное значение.
		btn.SetIconRotation(to)
		return
	}
	if from == to {
		btn.SetIconRotation(to)
		return
	}
	anim := fyne.NewAnimation(duration, func(v float32) {
		btn.SetIconRotation(from + (to-from)*v)
	})
	anim.Start()
}
