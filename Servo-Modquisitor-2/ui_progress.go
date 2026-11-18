// Servo-Modquisitor-2/ui_progress.go
package main

import (
	"context"
	"fmt"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// downloadProgressDialog — общая обвязка для скачивания с прогрессом.
func (app *App) downloadProgressDialog(title, filename string) (
	bar *widget.ProgressBar,
	ctx context.Context,
	hide func(),
	ready <-chan struct{},
) {
	ctx, cancel := context.WithCancel(context.Background())
	var dlg *dialog.CustomDialog
	var once sync.Once // + "sync" в импорты
	readyCh := make(chan struct{})

	// DoAndWait, а не Do: bar обязан быть присвоен ДО return.
	fyne.DoAndWait(func() {
		bar = widget.NewProgressBar()
		lbl := widget.NewLabel(fmt.Sprintf(app.msg("downloading"), filename))
		content := container.NewVBox(lbl, bar)
		dlg = dialog.NewCustom(title, app.msg("btn_cancel"), content, app.mainWindow)
		dlg.SetOnClosed(func() { once.Do(cancel) })
		dlg.Show()
	})
	close(readyCh) // совместимость: вызывающие всё ещё делают <-ready

	hide = func() {
		once.Do(cancel) // страховка от утечки контекста
		if dlg != nil {
			fyne.Do(func() { dlg.Hide() })
		}
	}
	return bar, ctx, hide, readyCh
}
