// Servo-Modquisitor-2/ui_search_popup.go
package main

import (
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// SearchPopupOptions — параметры для универсального поиска мода.
type SearchPopupOptions struct {
	Title     string
	Source    []string // список имён папок
	Width     float32
	Height    float32
	FormatRow func(folder string) string // если nil, просто folder
	OnPick    func(string)
	Parent    fyne.Window
}

// ShowSearchPopup создаёт модальный список с поиском и выбором одного мода.
// Используется в AML-редакторе и редакторе правил сортировки.
//
// Производительность: в Source может быть 1500+ записей (полная база
// модов). Display-имена и lowercase-ключи для поиска считаются ОДИН
// раз при открытии — filterFn превращается в голый strings.Contains
// по уже готовым строкам. Debounce 300ms не даёт фильтровать на
// каждое нажатие клавиши при быстром вводе.
func (app *App) ShowSearchPopup(opts SearchPopupOptions) {
	if opts.Width == 0 {
		opts.Width = 400
	}
	if opts.Height == 0 {
		opts.Height = 450
	}
	if opts.FormatRow == nil {
		opts.FormatRow = func(folder string) string {
			return app.modDisplayName(folder)
		}
	}

	// Pre-compute display + lowercase-ключ для каждой записи.
	// lower содержит и имя папки, и display-имя, разделённые NUL
	// (\x00 — не встречается в пользовательском вводе, поэтому
	// коллизий между концом folderLower и началом displayLower нет).
	// Один strings.Contains по lower накрывает обе проверки,
	// которые раньше были раздельными.
	type entry struct {
		folder  string
		display string
		lower   string
	}
	entries := make([]entry, len(opts.Source))
	for i, f := range opts.Source {
		d := opts.FormatRow(f)
		entries[i] = entry{
			folder:  f,
			display: d,
			lower:   strings.ToLower(f) + "\x00" + strings.ToLower(d),
		}
	}

	var filtered []entry
	apply := func(q string) {
		q = strings.ToLower(strings.TrimSpace(q))
		filtered = filtered[:0]
		for _, e := range entries {
			if q == "" || strings.Contains(e.lower, q) {
				filtered = append(filtered, e)
			}
		}
	}
	apply("")

	var pop *widget.PopUp
	list := widget.NewList(
		func() int { return len(filtered) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= 0 && id < len(filtered) {
				o.(*widget.Label).SetText(filtered[id].display)
			}
		},
	)
	list.OnSelected = func(id widget.ListItemID) {
		if id >= 0 && id < len(filtered) {
			picked := filtered[id].folder
			if pop != nil {
				pop.Hide() // Сначала закрываем текущий
			}
			// Открываем следующий асинхронно, давая Fyne время убрать старый попап
			go func() {
				time.Sleep(100 * time.Millisecond)
				fyne.Do(func() {
					opts.OnPick(picked)
				})
			}()
		}
	}

	searchField := app.NewSearchField(SearchFieldOptions{
		DebounceDelay: 300 * time.Millisecond,
		ShowClear:     false,
		OnFilter: func(q string) {
			apply(q)
			list.UnselectAll()
			list.Refresh()
		},
	})
	search := searchField.Entry

	titleLbl := widget.NewLabelWithStyle(opts.Title, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	listScroll := container.NewVScroll(list)
	listScroll.SetMinSize(fyne.NewSize(opts.Width-40, opts.Height-100))

	cancel := NewCustomIconButton(app.msg("btn_cancel"), app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
		if pop != nil {
			pop.Hide()
		}
	})

	content := container.NewBorder(
		container.NewVBox(titleLbl, search, widget.NewSeparator()),
		container.NewCenter(cancel),
		nil, nil,
		listScroll,
	)

	pop = widget.NewModalPopUp(content, opts.Parent.Canvas())
	pop.Resize(fyne.NewSize(opts.Width, opts.Height))
	pop.Show()
	opts.Parent.Canvas().Focus(search)
}
