// Servo-Modquisitor-2/ui_batch_dialog.go
package main

import (
	"fmt"
	"image/color"
	"net/url"
	"strings"
	"sync"
	"time"

	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/themes"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
	onceFinish sync.Once

	// UI: Таблица
	headerTable *widget.Table
	table       *widget.Table
	rows        []batchRow // строки таблицы (включая заголовки групп)

	// Ссылки — как descURL / sourceCodeLink в главном окне
	detailsSourceLink *widget.Hyperlink

	// Блок «версия + даты + категория» (аналог descVersionGrid)
	detailsVersionGrid *fyne.Container
	detailsValVersion  *widget.Label
	detailsValLatest   *widget.Label
	detailsValOrig     *widget.Label
	detailsValUpd      *widget.Label
	detailsValCategory *widget.Label

	// Секция «Статус» — иконка + текст, без дерева
	detailsStatusList    *fyne.Container
	detailsStatusSection *fyne.Container

	// Секция «Проблемы» — конфликты, broken, missing, ошибки батча
	detailsProblemsList    *fyne.Container
	detailsProblemsSection *fyne.Container

	// Пользовательская заметка
	detailsUserNote        *widget.Label
	detailsUserNoteSection *fyne.Container

	// UI: Детали выбранного мода
	detailsTitle    *widget.Label
	detailsTitleSep *canvas.Rectangle // зелёная полоска под заголовком
	detailsAuthor   *widget.Label
	detailsDesc     *widget.Label
	detailsLink     *widget.Hyperlink
	detailsImage    *canvas.Image
	detailsImageBox *fyne.Container
	detailsCard     *canvas.Rectangle
	detailsScroll   *container.Scroll
	currentDetailID string

	// Спойлер «Последние изменения»
	changelogLabel     *widget.Label
	changelogContainer *fyne.Container
	btnChangelog       *CustomButton
	changelogExpanded  bool
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

	// onFinish — вызывается, когда runBatchRunner завершил работу.
	// Нужен там, где вызывающий код должен дождаться конца
	// (например, runAllChecksInternal — прежде чем сортировать).
	onFinish func(success, total int)
}

func (app *App) showBatchDialog(title string, items []*BatchItem, mode BatchMode) {
	app.showBatchDialogInternal(title, items, mode, nil)
}

func (app *App) showBatchDialogSync(title string, items []*BatchItem, mode BatchMode) (int, int) {
	result := make(chan struct{ ok, total int }, 1)
	app.showBatchDialogInternal(title, items, mode, func(ok, total int) {
		result <- struct{ ok, total int }{ok, total}
	})
	r := <-result
	return r.ok, r.total
}

func (app *App) showBatchDialogInternal(title string, items []*BatchItem, mode BatchMode, onFinish func(int, int)) {
	if title == "" {
		title = ConfigFolderSMQ
	}

	d := &BatchDialog{
		items:      items,
		mode:       mode,
		app:        app,
		startChan:  make(chan struct{}),
		skipChan:   make(chan string, 4),
		cancelChan: make(chan struct{}),
		onFinish:   onFinish,
	}

	// Гарантируем выполнение инициализации UI в главном потоке.
	// Это предотвращает ошибку "Error in Fyne call thread", если функция
	// вызвана из фоновой горутины (например, после завершения проверок).
	fyne.Do(func() {
		d.window = app.myApp.NewWindow(title)
		d.window.Resize(fyne.NewSize(MainWindowWidth, MainWindowHeight))
		d.window.CenterOnScreen()

		d.buildRows()
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

		// Первая строка может быть заголовком группы — берём первую
		// строку с реальным модом.
		for i, r := range d.rows {
			if r.item != nil {
				d.table.Select(widget.TableCellID{Row: i, Col: 0}, 0)
				d.selectDetail(r.item.ID)
				break
			}
		}

		d.window.Show()
	})

	// Запускаем обработчик батча в отдельной горутине, как и было задумано
	go app.runBatchRunner(d, d.cancelChan)
}

// batchRow — строка таблицы. В BatchModeDelete конфликты идут
// группами: заголовок (item == nil) + дочерние строки с веткой
// дерева, как в редакторе правил сортировки.
type batchRow struct {
	item    *BatchItem
	title   string
	isGroup bool
	inGroup bool
	isLast  bool
}

func (d *BatchDialog) buildRows() {
	d.rows = nil
	if d.mode != BatchModeDelete {
		for _, it := range d.items {
			d.rows = append(d.rows, batchRow{item: it})
		}
		return
	}

	hint := d.app.msg("conflict_group_hint")
	if hint == "" {
		hint = "keep one"
	}

	prevGroup := -1
	for i, it := range d.items {
		if it.Kind != ProblemConflict || it.ConflictGroup < 0 {
			d.rows = append(d.rows, batchRow{item: it})
			prevGroup = -1
			continue
		}
		g := it.ConflictGroup
		if g != prevGroup {
			n := 0
			for j := i; j < len(d.items); j++ {
				if d.items[j].Kind == ProblemConflict && d.items[j].ConflictGroup == g {
					n++
				}
			}
			d.rows = append(d.rows, batchRow{
				isGroup: true,
				title:   fmt.Sprintf("%s (%d) · %s", d.app.msg("sort_conflict"), n, hint),
			})
			prevGroup = g
		}
		isLast := i+1 >= len(d.items) ||
			d.items[i+1].Kind != ProblemConflict ||
			d.items[i+1].ConflictGroup != g
		d.rows = append(d.rows, batchRow{item: it, inGroup: true, isLast: isLast})
	}
}

func (d *BatchDialog) itemByID(id string) (*BatchItem, bool) {
	for _, it := range d.items {
		if it.ID == id {
			return it, true
		}
	}
	return nil, false
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
				lbl.SetText(d.app.msg("col_status"))
			case 2:
				lbl.SetText(d.app.msg("col_name"))
			case 3:
				if d.mode == BatchModeDelete {
					lbl.SetText(d.app.msg("sort_mod_type"))
				} else {
					lbl.SetText(d.app.msg("col_version"))
				}
			case 4:
				lbl.SetText(d.app.msg("col_author"))
			case 5:
				lbl.SetText(d.app.msg("category_label"))
			}
		},
	)
	d.headerTable.SetColumnWidth(0, ColCheckboxWidth)
	d.headerTable.SetColumnWidth(1, ColNumberWidth)
	d.headerTable.SetColumnWidth(2, ColNameWidth)
	d.headerTable.SetColumnWidth(3, ColVersionWidth)
	d.headerTable.SetColumnWidth(4, ColAuthorWidth)
	d.headerTable.SetColumnWidth(5, ColCategoryWidth)

	d.table = widget.NewTable(
		func() (int, int) { return len(d.rows), 6 },
		func() fyne.CanvasObject {
			bg := canvas.NewRectangle(color.Transparent)
			bg.SetMinSize(fyne.NewSize(1, 28))
			statusBox := container.NewCenter() // <- ОБЯЗАТЕЛЬНО!
			// Пятый слот — под ветку дерева. Держим его всегда,
			// чтобы индексы cont.Objects[0..4] не разъезжались
			// при переиспользовании ячейки.
			treeBox := container.NewStack()
			return container.NewStack(bg, widget.NewLabel(""), widget.NewCheck("", nil), statusBox, treeBox)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			d.updateCell(id, cell)
		},
	)
	d.table.SetColumnWidth(0, ColCheckboxWidth)
	d.table.SetColumnWidth(1, ColNumberWidth)
	d.table.SetColumnWidth(2, ColNameWidth)
	d.table.SetColumnWidth(3, ColVersionWidth)
	d.table.SetColumnWidth(4, ColAuthorWidth)
	d.table.SetColumnWidth(5, ColCategoryWidth)

	d.table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(d.rows) {
			return
		}
		if r := d.rows[id.Row]; r.item != nil {
			d.selectDetail(r.item.ID)
			return
		}
		// Клик по заголовку группы — показываем первого её члена.
		for i := id.Row + 1; i < len(d.rows); i++ {
			if d.rows[i].item != nil {
				d.selectDetail(d.rows[i].item.ID)
				return
			}
		}
	}
}

func (d *BatchDialog) updateCell(id widget.TableCellID, cell fyne.CanvasObject) {
	cont := cell.(*fyne.Container)
	if id.Row >= len(d.rows) {
		return
	}
	row := d.rows[id.Row]
	item := row.item // nil → заголовок группы

	th := d.app.myApp.Settings().Theme()
	variant := d.app.myApp.Settings().ThemeVariant()

	bg := cont.Objects[0].(*canvas.Rectangle)
	lbl := cont.Objects[1].(*widget.Label)
	chk := cont.Objects[2].(*widget.Check)
	statusBox := cont.Objects[3].(*fyne.Container)
	treeBox := cont.Objects[4].(*fyne.Container)

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
	statusBox.Objects = nil
	treeBox.Objects = nil
	treeBox.Hide()
	lbl.TextStyle = fyne.TextStyle{}
	lbl.Truncation = fyne.TextTruncateOff

	switch id.Col {
	case 0:
		if item == nil {
			break // у заголовка группы чекбокса нет
		}
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
		var iconName string
		switch {
		case item == nil:
			iconName = "conflict"
		case d.mode == BatchModeDelete &&
			(item.Status == BatchPending || item.Status == BatchSkipped):
			iconName = item.Kind.Icon()
		default:
			iconName = item.Status.Icon()
		}
		if iconName != "" {
			statusBox.Add(d.app.statusIcon(iconName))
		}
		statusBox.Refresh()
	case 2:
		switch {
		case row.isGroup:
			lbl.Show()
			lbl.SetText(row.title)
			lbl.TextStyle = fyne.TextStyle{Bold: true}
			lbl.Truncation = fyne.TextTruncateEllipsis
			lbl.Alignment = fyne.TextAlignLeading
		case row.inGroup:
			treeBox.Objects = []fyne.CanvasObject{
				d.app.makeTreeBranch(row.isLast, widget.NewLabel(item.Name)),
			}
			treeBox.Show()
			treeBox.Refresh()
		default:
			lbl.Show()
			lbl.SetText(item.Name)
			lbl.Alignment = fyne.TextAlignLeading
		}
	case 3:
		if item == nil {
			break
		}
		lbl.Show()
		if d.mode == BatchModeDelete {
			if item.Kind != ProblemNone {
				lbl.SetText(d.app.msg(item.Kind.String()))
			} else {
				lbl.SetText("")
			}
		} else {
			lbl.SetText(item.VersionLabel())
		}
		lbl.Alignment = fyne.TextAlignLeading
	case 4:
		if item != nil {
			lbl.Show()
			lbl.SetText(item.Author)
			lbl.Alignment = fyne.TextAlignLeading
		}
	case 5:
		if item != nil {
			lbl.Show()
			lbl.SetText(item.Category)
			lbl.Alignment = fyne.TextAlignLeading
		}
	}
	lbl.Refresh()
	chk.Refresh()
}

// ─── Детали мода (правая панель) ──────────────────────────────────
func (d *BatchDialog) buildDetails() {
	th := d.app.myApp.Settings().Theme()
	variant := d.app.myApp.Settings().ThemeVariant()

	d.detailsTitle = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	d.detailsTitleSep = canvas.NewRectangle(th.Color(themes.ColorCRTScreenStroke, variant))
	d.detailsTitleSep.SetMinSize(fyne.NewSize(0, 3))

	d.detailsAuthor = widget.NewLabel("")
	d.detailsDesc = widget.NewLabel("")
	d.detailsDesc.Wrapping = fyne.TextWrapWord

	// ─── Ссылки: Nexus + исходники ───────────────────────────────
	d.detailsLink = widget.NewHyperlink(d.app.msg("mod_url_label"), nil)
	d.detailsLink.OnTapped = func() { d.openDetailURL(d.detailsLink.URL) }
	d.detailsSourceLink = widget.NewHyperlink(d.app.msg("source_code_url"), nil)
	d.detailsSourceLink.Alignment = fyne.TextAlignLeading
	d.detailsSourceLink.OnTapped = func() { d.openDetailURL(d.detailsSourceLink.URL) }
	linksColumn := container.New(&VBoxWithSpacing{Spacing: SpacingLinks},
		d.detailsLink, d.detailsSourceLink)

	// ─── Картинка с рамкой ───────────────────────────────────────
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
	imgBorder.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
	imgBorder.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	d.detailsImageBox = container.NewStack(imgBg, d.detailsImage, imgBorder)

	// ─── Версии / даты / категория — две колонки ─────────────────
	lblVersion := widget.NewLabel(d.app.msg("table_lbl_version"))
	lblLatest := widget.NewLabel(d.app.msg("table_lbl_latest"))
	lblOrig := widget.NewLabel(d.app.msg("original_upload_label") + ": ")
	lblUpd := widget.NewLabel(d.app.msg("last_updated_label") + ": ")
	lblCat := widget.NewLabel(d.app.msg("category_label") + ": ")

	d.detailsValVersion = widget.NewLabel("")
	d.detailsValLatest = widget.NewLabel("")
	d.detailsValOrig = widget.NewLabel("")
	d.detailsValUpd = widget.NewLabel("")
	d.detailsValCategory = widget.NewLabel("")

	// Фиксированная колонка подписей — значения в одной вертикали,
	// ровно как makeVersionRow в buildDescriptionCard.
	makeRow := func(lbl, val *widget.Label) fyne.CanvasObject {
		col := container.NewStack(helpers.NewFixedSpacer(versionLabelColWidth, 1), lbl)
		return container.NewBorder(nil, nil, col, nil, val)
	}
	d.detailsVersionGrid = container.New(&VBoxWithSpacing{Spacing: SpacingText15},
		makeRow(lblVersion, d.detailsValVersion),
		makeRow(lblLatest, d.detailsValLatest),
		makeRow(lblOrig, d.detailsValOrig),
		makeRow(lblUpd, d.detailsValUpd),
		makeRow(lblCat, d.detailsValCategory),
	)

	metaLeft := container.NewVBox(
		d.detailsAuthor,
		widget.NewSeparator(),
		linksColumn,
		widget.NewSeparator(),
		d.detailsVersionGrid,
	)
	metaRow := container.NewBorder(nil, nil, nil, d.detailsImageBox, metaLeft)

	// ─── Секция «Статус» ─────────────────────────────────────────
	d.detailsStatusList = container.New(&VBoxWithSpacing{Spacing: SpacingText11})
	d.detailsStatusSection = container.NewVBox(
		widget.NewSeparator(),
		container.New(&VBoxWithSpacing{Spacing: SpacingText13},
			widget.NewLabelWithStyle(d.app.msg("desc_section_status")+": ",
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			d.detailsStatusList,
		),
	)
	d.detailsStatusSection.Hide()

	// ─── Секция «Проблемы» ───────────────────────────────────────
	d.detailsProblemsList = container.New(&VBoxWithSpacing{Spacing: SpacingText11})
	d.detailsProblemsSection = container.NewVBox(
		widget.NewSeparator(),
		container.New(&VBoxWithSpacing{Spacing: SpacingText13},
			widget.NewLabelWithStyle(d.app.msg("desc_section_problems")+": ",
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			d.detailsProblemsList,
		),
	)
	d.detailsProblemsSection.Hide()

	// ─── Пользовательская заметка ────────────────────────────────
	d.detailsUserNote = widget.NewLabel("")
	d.detailsUserNote.Wrapping = fyne.TextWrapWord
	d.detailsUserNoteSection = container.NewVBox(widget.NewSeparator(), d.detailsUserNote)
	d.detailsUserNoteSection.Hide()

	// ─── Спойлер changelog ───────────────────────────────────────
	d.changelogLabel = widget.NewLabel("")
	d.changelogLabel.Wrapping = fyne.TextWrapWord
	d.changelogContainer = container.NewVBox(d.changelogLabel)
	d.changelogContainer.Hide()

	d.btnChangelog = NewCustomIconButton(
		d.app.msg("btn_show_changelog"),
		d.app.loadIconResource("arrow_right", "assets/buttons/spoiler.png"),
		func() {
			d.changelogExpanded = !d.changelogExpanded
			if d.changelogExpanded {
				d.changelogContainer.Show()
				d.btnChangelog.SetText(d.app.msg("btn_hide_changelog"))
				AnimateIconRotation(d.btnChangelog, 0, 90, 200*time.Millisecond)
			} else {
				d.changelogContainer.Hide()
				d.btnChangelog.SetText(d.app.msg("btn_show_changelog"))
				AnimateIconRotation(d.btnChangelog, 90, 0, 200*time.Millisecond)
			}
		},
	)
	d.btnChangelog.Hide()

	d.detailsCard = canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	d.detailsCard.CornerRadius = 12
	d.detailsCard.StrokeWidth = 1
	d.detailsCard.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)

	content := container.NewVBox(
		d.detailsTitle,
		d.detailsTitleSep,
		metaRow,
		d.detailsStatusSection,
		d.detailsProblemsSection,
		d.detailsUserNoteSection,
		widget.NewSeparator(),
		d.detailsDesc,
		widget.NewSeparator(),
		d.btnChangelog,
		d.changelogContainer,
		layout.NewSpacer(),
	)

	d.detailsScroll = container.NewScroll(content)
}

func (d *BatchDialog) buildDetailsContainer() fyne.CanvasObject {
	return container.NewStack(
		d.detailsCard,
		container.NewPadded(d.detailsScroll),
	)
}

// openDetailURL — открытие ссылки из карточки с логированием,
// как descURL.OnTapped в главном окне.
func (d *BatchDialog) openDetailURL(u *url.URL) {
	if u == nil {
		return
	}
	if err := d.app.myApp.OpenURL(u); err != nil {
		d.app.appendLogToFile(fmt.Sprintf("batch details: open %s failed: %v", u, err))
	}
}

// itemURLs — Nexus-страница и исходники. Источники по приоритету:
// установленный мод → mod_database.json → NexusModID элемента
// (для установки из коллекции, где мода ещё нет на диске).
func (d *BatchDialog) itemURLs(item *BatchItem, mod checks.ModInfo, hasMod bool) (string, string) {
	nexusURL, sourceURL := "", ""
	if hasMod {
		nexusURL, sourceURL = mod.URL, mod.SourceCodeURL
	}
	if e := checks.GetModDBEntry(item.ID); e != nil {
		if nexusURL == "" {
			nexusURL = e.URL
		}
		if sourceURL == "" {
			sourceURL = e.SourceCodeURL
		}
	}
	if nexusURL == "" {
		nexusURL = item.NexusURL()
	}
	return nexusURL, sourceURL
}

// fillVersionGrid — Version/Latest из кэша версий, даты из ModInfo,
// категория с локализацией. Пустые значения остаются пустыми, как
// в главном окне.
func (d *BatchDialog) fillVersionGrid(item *BatchItem, mod checks.ModInfo, hasMod bool, nexusURL, dateFormat string) {
	d.detailsValVersion.SetText("")
	d.detailsValLatest.SetText("")
	d.detailsValOrig.SetText("")
	d.detailsValUpd.SetText("")

	modID := item.NexusModID
	if modID == 0 {
		modID = helpers.ExtractModIDFromURL(nexusURL)
	}
	if modID != 0 {
		key := fmt.Sprintf("%d:%s", modID, item.ID)
		ver := ""
		if info, ok := d.app.getCachedVersion(key); ok {
			ver = info.Version
		}
		if ver == "" {
			ver = item.Version
		}
		d.detailsValVersion.SetText(ver)

		latest, _ := d.app.getLatestVersion(key)
		if latest == "" && d.mode == BatchModeUpdate {
			latest = item.Version // целевая версия обновления
		}
		d.detailsValLatest.SetText(latest)
	} else {
		d.detailsValVersion.SetText(item.Version)
	}

	if hasMod {
		if !mod.OriginalUpload.IsZero() {
			d.detailsValOrig.SetText(d.app.formatDate(mod.OriginalUpload, dateFormat))
		}
		if !mod.LastUpdated.IsZero() {
			d.detailsValUpd.SetText(d.app.formatDate(mod.LastUpdated, dateFormat))
		}
	}

	cat := item.Category
	if cat == "" && hasMod {
		cat = mod.Category
	}
	switch {
	case cat == "":
		d.detailsValCategory.SetText("-")
	case d.app.msg("category_"+cat) != "":
		d.detailsValCategory.SetText(d.app.msg("category_" + cat))
	default:
		d.detailsValCategory.SetText(cat)
	}
}

// fillStatusAndProblems — только на UI-потоке. Строки «иконка + текст»
// через app.statusRow, ровно как в секциях главного окна.
func (d *BatchDialog) fillStatusAndProblems(item *BatchItem, mod checks.ModInfo, hasMod bool, lang string) {
	d.detailsStatusList.Objects = nil
	d.detailsProblemsList.Objects = nil

	// ─── Статус ──────────────────────────────────────────────────
	if hasMod && mod.HasUpdate {
		d.detailsStatusList.Add(d.app.statusRow("update", d.app.msg("status_update_available")))
	}
	if hasMod && !mod.IsSymlink {
		switch mod.Source {
		case "manual":
			d.detailsStatusList.Add(d.app.statusRow("hand_vulcan", d.app.msg("status_manual")))
		case "nexus":
			d.detailsStatusList.Add(d.app.statusRow("nexus_orange", d.app.msg("status_nexus")))
		}
	}
	// Required — тот же виджет, что в главном окне: заголовок,
	// ✅/❌ и кликабельные ссылки на страницы модов.
	if row := d.app.createRequiredModsRow(item.ID, lang); row != nil {
		d.detailsStatusList.Add(row)
	} else if item.Name != item.ID {
		if row := d.app.createRequiredModsRow(item.Name, lang); row != nil {
			d.detailsStatusList.Add(row)
		}
	}
	if hasMod {
		if mod.VortexDeployed {
			d.detailsStatusList.Add(d.app.statusRow("vortex", d.app.msg("status_vortex")))
		}
		if mod.IsSymlink {
			d.detailsStatusList.Add(d.app.statusRow("symlink", d.app.msg("status_symlink")))
		}
		if mod.IsSystem {
			d.detailsStatusList.Add(d.app.statusRow("sys", d.app.msg("status_system")))
		}
	}
	if item.Kind == ProblemObsolete || (hasMod && mod.Obsolete) {
		d.detailsStatusList.Add(d.app.statusRow("skull", d.app.msg("desc_obsolete")))
	}

	// ─── Проблемы ────────────────────────────────────────────────
	if item.Kind == ProblemMissingFolder || (hasMod && mod.MissingFolder) {
		d.detailsProblemsList.Add(d.app.statusRow("folder_missing", d.app.msg("status_missing_folder")))
	}
	if item.Kind == ProblemBroken || (hasMod && mod.Broken) {
		d.detailsProblemsList.Add(d.app.statusRow("folder_broken", d.app.msg("desc_broken")))
	}

	// Конфликты: «Конфликт: Mod1 ⚔ Mod2» + причина из JSON.
	// Меток «удаляется / остаётся» нет — их показывает галка в таблице
	// и подсказка внизу окна.
	for _, p := range item.ConflictPartners {
		text := fmt.Sprintf("%s: %s ⚔ %s",
			d.app.msg("sort_conflict"),
			d.app.modDisplayName(item.ID),
			d.app.modDisplayName(p))
		if reason := conflictReason(checks.GetIncompatibleDesc(item.ID, p)); reason != "" {
			text += "\n" + reason
		}
		d.detailsProblemsList.Add(d.app.statusRow("conflict", text))
	}

	if item.Error != "" {
		d.detailsProblemsList.Add(d.app.statusRow("warn_red", item.Error))
	}

	if len(d.detailsStatusList.Objects) > 0 {
		d.detailsStatusSection.Show()
	} else {
		d.detailsStatusSection.Hide()
	}
	if len(d.detailsProblemsList.Objects) > 0 {
		d.detailsProblemsSection.Show()
	} else {
		d.detailsProblemsSection.Hide()
	}
	d.detailsStatusList.Refresh()
	d.detailsProblemsList.Refresh()
}

// conflictReason вырезает из локализованного описания пары строку-
// заголовок «Конфликт: X ⚔ Y» и возвращает только причину.
// Заголовок карточка строит сама — так формат един для всех пар,
// включая те, где desc в JSON отсутствует.
func conflictReason(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	head, tail := desc, ""
	if i := strings.IndexByte(desc, '\n'); i >= 0 {
		head, tail = desc[:i], strings.TrimSpace(desc[i+1:])
	}
	if strings.Contains(head, "⚔") {
		return tail // заголовок отбрасываем; если причины нет — вернём ""
	}
	return desc // desc без заголовка — считаем целиком причиной
}

// refreshDetailSections — пересборка «Статуса» и «Проблем» для
// текущего мода. Вызывается из SetStatus: у мода мог появиться
// item.Error, который надо показать в «Проблемах».
func (d *BatchDialog) refreshDetailSections() {
	if d.currentDetailID == "" {
		return
	}
	item, ok := d.itemByID(d.currentDetailID)
	if !ok {
		return
	}
	lang := "en"
	d.app.cfgMutex.RLock()
	if d.app.cfg.Language != "" {
		lang = d.app.cfg.Language
	}
	d.app.cfgMutex.RUnlock()
	mod, hasMod := d.app.findModByName(item.ID)
	d.fillStatusAndProblems(item, mod, hasMod, lang)
}

func (d *BatchDialog) selectDetail(id string) {
	if d.currentDetailID == id {
		return
	}
	d.currentDetailID = id

	// Сброс спойлера.
	d.changelogExpanded = false
	d.changelogContainer.Hide()
	d.btnChangelog.SetText(d.app.msg("btn_show_changelog"))
	d.btnChangelog.SetIconRotation(0)
	d.changelogLabel.SetText("")

	// Сброс секций.
	d.detailsStatusList.Objects = nil
	d.detailsStatusSection.Hide()
	d.detailsProblemsList.Objects = nil
	d.detailsProblemsSection.Hide()
	d.detailsUserNote.SetText("")
	d.detailsUserNoteSection.Hide()

	item, ok := d.itemByID(id)
	if !ok {
		return
	}

	lang, dateFormat := "en", ""
	d.app.cfgMutex.RLock()
	if d.app.cfg.Language != "" {
		lang = d.app.cfg.Language
	}
	dateFormat = d.app.cfg.DateFormat
	d.app.cfgMutex.RUnlock()

	// Для delete/update ID — имя папки, мод найдётся. Для установки
	// из коллекции ID = "mod_<id>" — мода ещё нет, hasMod == false.
	mod, hasMod := d.app.findModByName(item.ID)

	d.detailsTitle.SetText(item.Name)

	author := item.Author
	if author == "" && hasMod {
		author = mod.Author
	}
	if author == "" || author == "Unknown" {
		author = d.app.msg("author_unknown")
	}
	d.detailsAuthor.SetText(fmt.Sprintf(d.app.msg("author_label"), author))

	// Ссылки.
	nexusURL, sourceURL := d.itemURLs(item, mod, hasMod)
	setLink := func(h *widget.Hyperlink, raw, label string) {
		if raw == "" {
			h.SetURL(nil)
			h.SetText("")
			h.Hide()
			return
		}
		u, err := url.Parse(raw)
		if err != nil {
			h.SetURL(nil)
			h.SetText("")
			h.Hide()
			return
		}
		h.SetURL(u)
		h.SetText(label)
		h.Show()
	}
	setLink(d.detailsLink, nexusURL, d.app.msg("mod_url_label"))
	setLink(d.detailsSourceLink, sourceURL, d.app.msg("source_code_url"))

	d.fillVersionGrid(item, mod, hasMod, nexusURL, dateFormat)
	d.fillStatusAndProblems(item, mod, hasMod, lang)

	// Заметка пользователя.
	if d.app.userNotes != nil {
		if un := d.app.userNotes.Get(item.ID); un != "" {
			d.detailsUserNote.SetText("📝 " + un)
			d.detailsUserNoteSection.Show()
		}
	}

	// Описание.
	desc := strings.TrimSpace(item.Description)
	if desc == "" && hasMod {
		desc = strings.TrimSpace(mod.Description)
	}
	if desc == "" || desc == "OBSOLETE!" {
		desc = d.app.msg("desc_placeholder")
	}
	d.detailsDesc.SetText(desc)

	// Changelog — наполняем, но не раскрываем.
	cleanCL := stripHTML(item.Changelog)
	if cleanCL != "" && cleanCL != d.app.msg("changelog_unavailable") {
		d.changelogLabel.SetText(cleanCL)
		d.btnChangelog.Show()
	} else {
		d.btnChangelog.Hide()
	}

	// Картинка.
	d.detailsImage.Resource = nil
	d.detailsImage.Refresh()
	pic := item.PictureURL
	if pic == "" && hasMod {
		pic = mod.NexusPictureURL
	}
	if pic != "" {
		go func(u, expectID string) {
			res, err := fetchNexusThumbnail(u)
			if err == nil {
				fyne.Do(func() {
					if d.currentDetailID == expectID {
						d.detailsImage.Resource = res
						d.detailsImage.Refresh()
					}
				})
			}
		}(pic, id)
	}
}

// ─── Кнопки и Прогресс ────────────────────────────────────────────

func (d *BatchDialog) buildBottom() fyne.CanvasObject {
	startMsg := d.app.msg("batch_waiting_start")
	if d.mode == BatchModeDelete {
		startMsg = d.app.msg("problems_delete_hint")
	}
	d.statusLabel = widget.NewLabel(startMsg)
	d.statusLabel.Alignment = fyne.TextAlignCenter

	d.progressBar = widget.NewProgressBar()
	d.progressBar.SetValue(0)
	d.progressBar.Hide()

	// Локализованные кнопки
	btnStartText := d.app.msg("batch_btn_install")
	switch d.mode {
	case BatchModeUpdate:
		btnStartText = d.app.msg("btn_update_mods")
	case BatchModeDelete:
		btnStartText = d.app.msg("btn_delete_selected")
	}
	if btnStartText == "" {
		switch d.mode {
		case BatchModeUpdate:
			btnStartText = "Update selected"
		case BatchModeDelete:
			btnStartText = "Delete selected"
		default:
			btnStartText = "Install selected"
		}
	}

	var btnStartIcon fyne.Resource
	if d.mode == BatchModeDelete {
		btnStartIcon = d.app.loadIconResource("trash", "assets/buttons/trashcan_red.png")
	} else {
		btnStartIcon = d.app.loadIconResource("play", "assets/buttons/downl.png")
	}
	d.btnStart = NewCustomIconButton(btnStartText, btnStartIcon, func() {
		d.onceStart.Do(func() { close(d.startChan) })
		d.btnStart.Disable()
		d.btnSelectAll.Disable()
		d.btnDeselectAll.Disable()
		d.progressBar.Show()
		d.statusLabel.SetText(d.app.msg("batch_starting"))
	})
	d.btnStart.Importance = widget.HighImportance

	d.btnSkip = NewCustomIconButton(d.app.msg("batch_btn_skip"), d.app.loadIconResource("skip", "assets/buttons/skip.png"), func() {
		select {
		case d.skipChan <- d.currentWaitID:
		default:
		}
	})
	d.btnSkip.Disable()

	d.btnCancel = NewCustomIconButton(d.app.msg("btn_cancel"), d.app.loadIconResource("cancel", "assets/buttons/x_.png"), func() {
		d.onceCancel.Do(func() { close(d.cancelChan) })
		d.window.Close()
	})
	d.btnCancel.Importance = widget.DangerImportance

	d.btnSelectAll = NewCustomIconButton(d.app.msg("btn_select_all"), d.app.loadIconResource("select_all", "assets/buttons/select_all.png"), func() {
		for _, it := range d.items {
			if it.Status == BatchPending {
				it.Selected = true
			}
		}
		d.table.Refresh()
	})

	d.btnDeselectAll = NewCustomIconButton(d.app.msg("btn_deselect_all"), d.app.loadIconResource("select_all_de", "assets/buttons/select_all_de.png"), func() {
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
		d.refreshProgress()
		d.table.Refresh()
		// Ошибка/смена статуса видна в карточке только для
		// открытого мода — остальные не пересобираем.
		if item.ID == d.currentDetailID {
			d.refreshDetailSections()
		}
	})
}

// refreshProgress — только на UI-потоке (внутри fyne.Do).
func (d *BatchDialog) refreshProgress() {
	total := len(d.items)
	if total == 0 || d.progressBar == nil {
		return
	}
	done := 0
	for _, it := range d.items {
		switch it.Status {
		case BatchPending, BatchWaiting, BatchDownloading:
			// ещё в работе
		default:
			done++
		}
	}
	d.progressBar.SetValue(float64(done) / float64(total))
}
func (d *BatchDialog) UpdateProgress(msg string) {
	fyne.Do(func() { d.statusLabel.SetText(msg) })
}

// Finish идемпотентен: runBatchRunner зовёт его и явно, и через defer.
// onFinish гарантированно срабатывает ровно один раз — иначе
// showBatchDialogSync либо зависнет, либо получит два результата.
func (d *BatchDialog) Finish(success, total int) {
	d.onceFinish.Do(func() {
		fyne.Do(func() {
			d.statusLabel.SetText(fmt.Sprintf(d.app.msg("batch_finished"), success, total))
			d.progressBar.Show() // мог остаться скрытым при total == 0
			d.progressBar.SetValue(1)
			d.btnStart.Hide()
			d.btnSkip.Hide()
			d.btnCancel.SetText(d.app.msg("theme_editor_btn_close"))
			d.btnCancel.Importance = widget.MediumImportance
		})
		if d.onFinish != nil {
			d.onFinish(success, total)
		}
	})
}
