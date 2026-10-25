// Servo-Modquisitor-2/ui_batch_dialog.go
package main

import (
	"fmt"
	"image/color"
	"net/url"
	"sync"

	"Servo-Modquisitor/themes"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

type BatchDialog struct {
	window fyne.Window
	items  []*BatchItem
	mode   BatchMode
	app    *App

	// Control
	startChan  chan struct{}
	skipChan   chan string
	cancelChan chan struct{}
	onceStart  sync.Once
	onceCancel sync.Once

	// UI: Таблица
	headerTable *widget.Table
	table       *widget.Table

	// UI: Детали выбранного мода
	detailsTitle     *widget.Label
	detailsAuthor    *widget.Label
	detailsDesc      *widget.Label
	detailsChangelog *widget.Label
	detailsLink      *widget.Hyperlink
	detailsImage     *canvas.Image
	detailsImageBox  *fyne.Container
	detailsCard      *canvas.Rectangle
	detailsScroll    *container.Scroll
	currentDetailID  string

	// UI: Прогресс и кнопки
	statusLabel   *widget.Label
	progressBar   *widget.ProgressBar
	btnStart      *CustomButton
	btnSkip       *CustomButton
	btnCancel     *CustomButton
	autoOpenCheck *widget.Check

	btnSelectAll   *CustomButton
	btnDeselectAll *CustomButton

	currentWaitID string
}

func (app *App) showBatchDialog(title string, items []*BatchItem, mode BatchMode) {
	// Защита от пустого заголовка
	if title == "" {
		title = "Servo-Modquisitor"
	}

	d := &BatchDialog{
		items:      items,
		mode:       mode,
		app:        app,
		startChan:  make(chan struct{}),
		skipChan:   make(chan string, 4),
		cancelChan: make(chan struct{}),
	}

	d.window = app.myApp.NewWindow(title)
	d.window.Resize(fyne.NewSize(1200, 800))
	d.window.CenterOnScreen()

	d.buildTable()
	d.buildDetails()

	autoOpenLabel := app.msg("batch_auto_open")
	if autoOpenLabel == "" {
		autoOpenLabel = "Auto-open mod pages"
	}
	d.autoOpenCheck = widget.NewCheck(autoOpenLabel, nil)
	d.autoOpenCheck.SetChecked(true)

	top := container.NewVBox(
		widget.NewSeparator(),
		d.headerTable,
	)

	bottom := d.buildBottom()

	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	var tableBg *canvas.Image
	var tableContent fyne.CanvasObject = d.table
	if mechData != nil {
		tableBg = canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		tableBg.FillMode = canvas.ImageFillContain
		tableBg.Translucency = TableBackgroundOpacity
		tableContent = container.NewStack(tableBg, d.table)
	}

	split := container.NewHSplit(tableContent, d.buildDetailsContainer())
	split.Offset = 0.60

	content := container.NewBorder(top, bottom, nil, nil, split)
	d.window.SetContent(container.NewPadded(content))

	d.window.SetCloseIntercept(func() {
		d.onceCancel.Do(func() { close(d.cancelChan) })
		d.window.Close()
	})

	if len(items) > 0 {
		d.table.Select(widget.TableCellID{Row: 0, Col: 0}, 0)
		d.selectDetail(items[0].ID)
	}

	d.window.Show()
	go app.runBatchRunner(d, d.cancelChan)
}

// ─── Таблица ──────────────────────────────────────────────────────

func (d *BatchDialog) buildTable() {
	d.headerTable = widget.NewTable(
		func() (int, int) { return 1, 6 },
		func() fyne.CanvasObject {
			th := d.app.myApp.Settings().Theme()
			variant := d.app.myApp.Settings().ThemeVariant()
			return container.NewStack(
				canvas.NewRectangle(th.Color(themes.ColorTableHeaderBg, variant)),
				widget.NewLabelWithStyle("", fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
			)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cont := cell.(*fyne.Container)
			lbl := cont.Objects[1].(*widget.Label)
			switch id.Col {
			case 0:
				lbl.SetText(d.app.msg("col_checkbox"))
			case 1:
				// Используем локализацию
				lbl.SetText(d.app.msg("col_status"))
			case 2:
				lbl.SetText(d.app.msg("col_name"))
			case 3:
				lbl.SetText(d.app.msg("col_version"))
			case 4:
				lbl.SetText(d.app.msg("col_author"))
			case 5:
				lbl.SetText(d.app.msg("category_label"))
			}
		},
	)
	d.headerTable.SetColumnWidth(0, 30)
	d.headerTable.SetColumnWidth(1, 50)
	d.headerTable.SetColumnWidth(2, 360)
	d.headerTable.SetColumnWidth(3, 90)
	d.headerTable.SetColumnWidth(4, 200)
	d.headerTable.SetColumnWidth(5, 150)

	d.table = widget.NewTable(
		func() (int, int) { return len(d.items), 6 },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			bg.SetMinSize(fyne.NewSize(1, 28))
			return container.NewStack(bg, widget.NewLabel(""), widget.NewCheck("", nil))
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			d.updateCell(id, cell)
		},
	)
	d.table.SetColumnWidth(0, 30)
	d.table.SetColumnWidth(1, 50)
	d.table.SetColumnWidth(2, 360)
	d.table.SetColumnWidth(3, 90)
	d.table.SetColumnWidth(4, 200)
	d.table.SetColumnWidth(5, 150)

	d.table.OnSelected = func(id widget.TableCellID) {
		if id.Row >= 0 && id.Row < len(d.items) {
			d.selectDetail(d.items[id.Row].ID)
		}
	}
}

func (d *BatchDialog) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
	cont := cell.(*fyne.Container)
	if id.Row >= len(d.items) {
		return
	}
	item := d.items[id.Row]

	th := d.app.myApp.Settings().Theme()
	variant := d.app.myApp.Settings().ThemeVariant()

	bg := cont.Objects[0].(*canvas.Rectangle)
	lbl := cont.Objects[1].(*widget.Label)
	chk := cont.Objects[2].(*widget.Check)

	if d.table.IsRowSelected(id.Row) {
		bg.FillColor = th.Color(themes.ColorTableRowSelected, variant)
	} else if id.Row%2 == 0 {
		bg.FillColor = th.Color(themes.ColorTableRowEven, variant)
	} else {
		bg.FillColor = th.Color(themes.ColorTableRowOdd, variant)
	}
	bg.Refresh()

	lbl.Hide()
	chk.Hide()

	switch id.Col {
	case 0:
		chk.Show()
		chk.SetChecked(item.Selected)
		chk.OnChanged = func(b bool) {
			item.Selected = b
		}
		if item.Status == BatchInstalled || item.Status == BatchSkipped {
			chk.Disable()
		} else {
			chk.Enable()
		}
	case 1:
		lbl.Show()
		lbl.SetText(item.Status.Icon())
		lbl.Alignment = fyne.TextAlignCenter
	case 2:
		lbl.Show()
		lbl.SetText(item.Name)
		lbl.Alignment = fyne.TextAlignLeading
	case 3:
		lbl.Show()
		lbl.SetText(item.VersionLabel())
		lbl.Alignment = fyne.TextAlignLeading
	case 4:
		lbl.Show()
		lbl.SetText(item.Author)
		lbl.Alignment = fyne.TextAlignLeading
	case 5:
		lbl.Show()
		lbl.SetText(item.Category)
		lbl.Alignment = fyne.TextAlignLeading
	}
	lbl.Refresh()
	chk.Refresh()
}

// ─── Детали мода (правая панель) ──────────────────────────────────

func (d *BatchDialog) buildDetails() {
	th := d.app.myApp.Settings().Theme()
	variant := d.app.myApp.Settings().ThemeVariant()

	d.detailsTitle = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	d.detailsAuthor = widget.NewLabel("")
	d.detailsDesc = widget.NewLabel("")
	d.detailsDesc.Wrapping = fyne.TextWrapWord

	d.detailsChangelog = widget.NewLabel("")
	d.detailsChangelog.Wrapping = fyne.TextWrapWord

	d.detailsLink = widget.NewHyperlink(d.app.msg("mod_url_label"), nil)

	// Создаем картинку и рамку для нее (как в главном окне)
	imgBg := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 15})
	imgBg.CornerRadius = CornerRadius15
	imgBg.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	d.detailsImage = canvas.NewImageFromResource(nil)
	d.detailsImage.FillMode = canvas.ImageFillContain
	d.detailsImage.CornerRadius = CornerRadius15
	d.detailsImage.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	imgBorder := canvas.NewRectangle(color.Transparent)
	imgBorder.CornerRadius = CornerRadius15
	imgBorder.StrokeWidth = 1
	imgBorder.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)
	imgBorder.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	d.detailsImageBox = container.NewStack(
		imgBg,
		d.detailsImage,
		imgBorder)

	d.detailsCard = canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	d.detailsCard.CornerRadius = 12
	d.detailsCard.StrokeWidth = 1
	d.detailsCard.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)

	// Левая колонка с текстом
	metaLeft := container.NewVBox(
		d.detailsTitle,
		widget.NewSeparator(),
		d.detailsAuthor,
		widget.NewSeparator(),
		d.detailsLink,
		widget.NewSeparator(),
		d.detailsDesc,
		widget.NewSeparator(),
		d.detailsChangelog,
	)

	// Оборачиваем картинку в VBox со Spacer, чтобы она не растягивалась по высоте
	imageCol := container.NewVBox(d.detailsImageBox, layout.NewSpacer())

	// Используем Border, чтобы колонка с картинкой ушла вправо
	content := container.NewBorder(nil, nil, nil, imageCol, metaLeft)

	d.detailsScroll = container.NewScroll(content)
}

func (d *BatchDialog) buildDetailsContainer() fyne.CanvasObject {
	return container.NewStack(
		d.detailsCard,
		container.NewPadded(d.detailsScroll),
	)
}

func (d *BatchDialog) selectDetail(id string) {
	if d.currentDetailID == id {
		return
	}
	d.currentDetailID = id
	for _, item := range d.items {
		if item.ID != id {
			continue
		}
		d.detailsTitle.SetText(item.Name)

		author := item.Author
		if author == "" || author == "Unknown" {
			author = d.app.msg("author_unknown")
		}
		d.detailsAuthor.SetText(fmt.Sprintf(d.app.msg("author_label"), author))

		desc := item.Description
		if desc == "" || desc == "OBSOLETE!" {
			desc = d.app.msg("desc_placeholder")
		}
		d.detailsDesc.SetText(desc)

		cleanCL := stripHTML(item.Changelog)
		if cleanCL != "" && cleanCL != d.app.msg("changelog_unavailable") {
			d.detailsChangelog.SetText(d.app.msg("btn_show_changelog") + ":\n" + cleanCL)
			d.detailsChangelog.Show()
		} else {
			d.detailsChangelog.Hide()
		}

		if item.NexusModID != 0 {
			if u, err := url.Parse(item.NexusURL()); err == nil {
				d.detailsLink.SetURL(u)
				d.detailsLink.Show()
			}
		} else {
			d.detailsLink.Hide()
		}

		d.detailsImage.Resource = nil
		d.detailsImage.Refresh()
		if item.PictureURL != "" {
			go func(u, expectId string) {
				res, err := fetchNexusThumbnail(u)
				if err == nil {
					fyne.Do(func() {
						if d.currentDetailID == expectId {
							d.detailsImage.Resource = res
							d.detailsImage.Refresh()
						}
					})
				}
			}(item.PictureURL, id)
		}
		return
	}
}

// ─── Кнопки и Прогресс ────────────────────────────────────────────

func (d *BatchDialog) buildBottom() fyne.CanvasObject {
	d.statusLabel = widget.NewLabel(d.app.msg("batch_waiting_start"))
	d.statusLabel.Alignment = fyne.TextAlignCenter

	d.progressBar = widget.NewProgressBar()
	d.progressBar.SetValue(0)
	d.progressBar.Hide()

	// Локализованные кнопки
	btnStartText := d.app.msg("batch_btn_install")
	if d.mode == BatchModeUpdate {
		btnStartText = d.app.msg("btn_update_mods")
	}
	// Fallback если переводы не добавлены
	if btnStartText == "" {
		if d.mode == BatchModeUpdate {
			btnStartText = "Update selected"
		} else {
			btnStartText = "Install selected"
		}
	}

	d.btnStart = NewCustomButton(btnStartText, func() {
		d.onceStart.Do(func() { close(d.startChan) })
		d.btnStart.Disable()
		d.btnSelectAll.Disable()
		d.btnDeselectAll.Disable()
		d.progressBar.Show()
		d.statusLabel.SetText(d.app.msg("batch_starting"))
	})
	d.btnStart.Importance = widget.HighImportance

	d.btnSkip = NewCustomButton(d.app.msg("batch_btn_skip"), func() {
		select {
		case d.skipChan <- d.currentWaitID:
		default:
		}
	})
	d.btnSkip.Disable()

	d.btnCancel = NewCustomButton(d.app.msg("btn_cancel"), func() {
		d.onceCancel.Do(func() { close(d.cancelChan) })
		d.window.Close()
	})
	d.btnCancel.Importance = widget.DangerImportance

	d.btnSelectAll = NewCustomButton(d.app.msg("btn_select_all"), func() {
		for _, it := range d.items {
			if it.Status == BatchPending {
				it.Selected = true
			}
		}
		d.table.Refresh()
	})

	d.btnDeselectAll = NewCustomButton(d.app.msg("btn_deselect_all"), func() {
		for _, it := range d.items {
			if it.Status == BatchPending {
				it.Selected = false
			}
		}
		d.table.Refresh()
	})

	return container.NewVBox(
		widget.NewSeparator(),
		container.NewHBox(d.btnSelectAll, d.btnDeselectAll, d.autoOpenCheck),
		widget.NewSeparator(),
		d.statusLabel,
		d.progressBar,
		container.NewCenter(
			container.NewHBox(d.btnStart, d.btnSkip, d.btnCancel)),
	)
}

func (d *BatchDialog) SetStatus(item *BatchItem, status BatchItemStatus, errMsg ...string) {
	item.Status = status
	if len(errMsg) > 0 {
		item.Error = errMsg[0]
	}

	if status == BatchWaiting || status == BatchDownloading {
		d.currentWaitID = item.ID
	}

	fyne.Do(func() {
		if status == BatchWaiting || status == BatchDownloading {
			d.btnSkip.Enable()
		} else {
			d.btnSkip.Disable()
		}
		d.table.Refresh()
	})
}

func (d *BatchDialog) UpdateProgress(msg string) {
	fyne.Do(func() { d.statusLabel.SetText(msg) })
}

func (d *BatchDialog) Finish(success, total int) {
	fyne.Do(func() {
		d.statusLabel.SetText(fmt.Sprintf(d.app.msg("batch_finished"), success, total))
		d.progressBar.SetValue(1)
		d.btnStart.Hide()
		d.btnSkip.Hide()
		d.btnCancel.SetText(d.app.msg("theme_editor_btn_close"))
		d.btnCancel.Importance = widget.MediumImportance
	})
}
