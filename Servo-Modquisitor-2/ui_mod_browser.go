// Servo-Modquisitor-2/ui_mod_browser.go
package main

import (
	"fmt"
	"image/color"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/themes"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// activeBrowserUpdate хранит функцию для перерисовки открытого окна поиска при смене языка.
var activeBrowserUpdate func()

type StatBox struct {
	Value *widget.Label
	Item  fyne.CanvasObject
}

func (app *App) newStatBox(emoji, label string) *StatBox {
	valLbl := widget.NewLabelWithStyle("-", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	lbl := widget.NewLabelWithStyle(label, fyne.TextAlignCenter, fyne.TextStyle{})
	lbl.Importance = widget.LowImportance

	cont := container.NewVBox(
		widget.NewLabelWithStyle(emoji, fyne.TextAlignCenter, fyne.TextStyle{}),
		valLbl,
		lbl,
	)
	return &StatBox{Value: valLbl, Item: cont}
}

// modBrowserTheme - кастомная тема для изменения цвета рамки поля ввода.
type modBrowserTheme struct {
	fyne.Theme
	primaryColor color.Color
}

func (m *modBrowserTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	if name == theme.ColorNamePrimary || name == theme.ColorNameInputBorder {
		return m.primaryColor
	}
	return m.Theme.Color(name, variant)
}

func (app *App) showModBrowser() {
	win := app.myApp.NewWindow(app.msg("browser_title"))
	win.Resize(fyne.NewSize(1300, 800))
	win.CenterOnScreen()

	var results []ModBrowserItem
	var selectedMod *ModBrowserItem
	var searchTimer *time.Timer
	var blinkSearchActive atomic.Bool
	var currentSearchQuery string

	// Хелпер для локализации категорий (как в ui_build.go)
	getLocalizedCategory := func(cat string) string {
		if cat == "" {
			return "-"
		}
		locKey := "category_" + cat
		if translated := app.msg(locKey); translated != "" {
			return translated
		}
		return cat
	}

	// Состояние спойлера и кнопок (локально для этого окна)
	var (
		changelogExpanded bool
		changelogFetched  bool
		changelogModID    int
		btnChangelog      *widget.Button
		detailsBox        *fyne.Container
		btnDownload       *widget.Button
	)

	// --- 1. Таблица и Заголовки ---
	var table *widget.Table
	var headerTable *widget.Table

	headerTable = widget.NewTable(
		func() (int, int) { return 1, 5 }, // 5 колонок
		func() fyne.CanvasObject {
			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()
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
				lbl.SetText(app.msg("col_status"))
			case 1:
				lbl.SetText(app.msg("col_name"))
			case 2:
				lbl.SetText(app.msg("col_author"))
			case 3:
				lbl.SetText(app.msg("category_label"))
			case 4:
				lbl.SetText(app.msg("col_total_dls"))
			}
		},
	)
	headerTable.SetColumnWidth(0, ColStatusSmWidth)
	headerTable.SetColumnWidth(1, ColNameWidth)
	headerTable.SetColumnWidth(2, ColAuthorWidth)
	headerTable.SetColumnWidth(3, ColCategoryWidth)
	headerTable.SetColumnWidth(4, ColDLsWidth)
	headerTable.OnSelected = nil

	table = widget.NewTable(
		func() (int, int) { return len(results), 5 },
		func() fyne.CanvasObject {
			spacer := canvas.NewRectangle(color.Transparent)
			spacer.SetMinSize(fyne.NewSize(1, 28))
			return container.NewStack(spacer)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			cont := cell.(*fyne.Container)
			cont.Objects = nil

			if id.Row >= len(results) {
				return
			}
			mod := results[id.Row]

			th := app.myApp.Settings().Theme()
			variant := app.myApp.Settings().ThemeVariant()

			bgColor := th.Color(themes.ColorTableRowEven, variant)
			if table.IsRowSelected(id.Row) {
				bgColor = th.Color(themes.ColorTableRowSelected, variant)
			} else if id.Row%2 == 1 {
				bgColor = th.Color(themes.ColorTableRowOdd, variant)
			}
			bg := canvas.NewRectangle(bgColor)

			var content fyne.CanvasObject

			switch id.Col {
			case 0:
				row := container.NewHBox()
				if mod.IsInstalled {
					if mod.Source == "manual" {
						row.Add(app.statusIcon("hand_vulcan_yellow"))
					} else {
						row.Add(app.statusIcon("nexus_orange"))
					}
				} else if mod.IsObsolete {
					row.Add(app.statusIcon("skull"))
				}

				hasReqs := false
				for _, dep := range checks.GetDependencies() {
					if dep.Dependent == mod.Folder || dep.Dependent == mod.Name {
						hasReqs = true
						break
					}
				}
				if hasReqs {
					row.Add(app.statusIcon("warn_yellow_romb"))
				}

				pad := canvas.NewRectangle(color.Transparent)
				pad.SetMinSize(fyne.NewSize(theme.Padding()/2, 1))
				content = container.NewHBox(pad, row)

			case 1: // Название
				segs := createHighlightedSegments(mod.Name, currentSearchQuery, widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, Inline: true})
				rt := widget.NewRichText(segs...)
				rt.Wrapping = fyne.TextWrapOff
				rt.Truncation = fyne.TextTruncateEllipsis
				content = rt
			case 2: // Автор
				segs := createHighlightedSegments(mod.Author, currentSearchQuery, widget.RichTextStyle{Inline: true})
				rt := widget.NewRichText(segs...)
				rt.Wrapping = fyne.TextWrapOff
				rt.Truncation = fyne.TextTruncateEllipsis
				content = rt
			case 3:
				content = widget.NewLabel(getLocalizedCategory(mod.Category))
			case 4:
				if mod.Downloads > 0 {
					content = widget.NewLabel(fmt.Sprintf("%d", mod.Downloads))
				} else {
					content = widget.NewLabel("-")
				}
			}

			cont.Add(bg)
			if content != nil {
				cont.Add(content)
			} else {
				spacer := canvas.NewRectangle(color.Transparent)
				spacer.SetMinSize(fyne.NewSize(1, 28))
				cont.Add(spacer)
			}
		},
	)
	table.SetColumnWidth(0, ColStatusSmWidth)
	table.SetColumnWidth(1, ColNameWidth)
	table.SetColumnWidth(2, ColAuthorWidth)
	table.SetColumnWidth(3, ColCategoryWidth)
	table.SetColumnWidth(4, ColDLsWidth)

	// --- 2. Правая панель (Карточка) ---
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	titleLbl := widget.NewRichText()
	titleLbl.Wrapping = fyne.TextWrapOff
	titleLbl.Truncation = fyne.TextTruncateEllipsis

	authorLbl := widget.NewRichText()
	authorLbl.Wrapping = fyne.TextWrapOff
	authorLbl.Truncation = fyne.TextTruncateEllipsis

	summaryLbl := widget.NewRichText()
	summaryLbl.Wrapping = fyne.TextWrapWord

	descURL := widget.NewHyperlink("", nil)
	sourceCodeLink := widget.NewHyperlink("", nil)

	descVersionLblVersion := widget.NewLabel("")
	descVersionValVersion := widget.NewLabel("")
	descVersionLblLatest := widget.NewLabel("")
	descVersionValLatest := widget.NewLabel("")
	descVersionLblOrig := widget.NewLabel("")
	descVersionValOrig := widget.NewLabel("")
	descVersionLblUpd := widget.NewLabel("")
	descVersionValUpd := widget.NewLabel("")
	descCategoryLbl := widget.NewLabel("")
	descCategoryVal := widget.NewLabel("")

	makeRow := func(lbl, val *widget.Label) fyne.CanvasObject {
		lblSpacer := canvas.NewRectangle(color.Transparent)
		lblSpacer.SetMinSize(fyne.NewSize(versionLabelColWidth, 1))
		lblCol := container.NewStack(lblSpacer, lbl)
		return container.NewBorder(nil, nil, lblCol, nil, val)
	}

	versionRow := makeRow(descVersionLblVersion, descVersionValVersion)

	descVersionGrid := container.New(
		&VBoxWithSpacing{Spacing: SpacingText15},
		versionRow,
		makeRow(descVersionLblLatest, descVersionValLatest),
		makeRow(descVersionLblOrig, descVersionValOrig),
		makeRow(descVersionLblUpd, descVersionValUpd),
		makeRow(descCategoryLbl, descCategoryVal),
	)

	// Секция статуса
	descStatusList := container.New(&VBoxWithSpacing{Spacing: SpacingText11})
	descStatusSection := container.NewVBox(
		widget.NewSeparator(),
		container.New(&VBoxWithSpacing{Spacing: SpacingText13},
			widget.NewLabelWithStyle(
				app.msg("desc_section_status")+":",
				fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
			),
			descStatusList,
		),
	)
	descStatusSection.Hide()

	// Картинка
	imgBg := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 15})
	imgBg.CornerRadius = CornerRadius15
	imgBg.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	thumbnail := canvas.NewImageFromResource(nil)
	thumbnail.FillMode = canvas.ImageFillContain
	thumbnail.CornerRadius = CornerRadius15
	thumbnail.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	thumbnailBorder := canvas.NewRectangle(color.Transparent)
	thumbnailBorder.CornerRadius = CornerRadius15
	thumbnailBorder.StrokeWidth = 1
	thumbnailBorder.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
	thumbnailBorder.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	thumbOverlay := newThumbnailOverlay(func() {
		if thumbnail.Resource != nil {
			app.showEnlargedImage(thumbnail.Resource)
		}
	})
	thumbBox := container.NewStack(imgBg, thumbnail, thumbOverlay, thumbnailBorder)
	thumbBox.Hide()

	// Кнопка установки
	btnDownload = widget.NewButton(app.msg("browser_btn_install"), func() {
		if selectedMod == nil {
			return
		}
		btnDownload.Disable()

		go func(mID int, mName string) {
			fi, err := app.getLatestFileInfo(mID)
			if err != nil {
				fyne.Do(func() {
					app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to get file info: %v", err))
					btnDownload.Enable()
				})
				return
			}

			directURL, filename, err := app.getPremiumDownloadURL(strconv.Itoa(mID), strconv.Itoa(fi.ID))
			if err != nil {
				if isAuthError(err) || strings.Contains(err.Error(), "403") {
					fyne.Do(func() {
						app.appendLog(app.msg("premium_required_manual"))
						if u, uerr := url.Parse(fmt.Sprintf("https://www.nexusmods.com/warhammer40kdarktide/mods/%d", mID)); uerr == nil {
							fyne.CurrentApp().OpenURL(u)
						}
						btnDownload.Enable()
					})
					return
				}
				fyne.Do(func() {
					app.showInfoDialog(app.msg("error_title"), fmt.Sprintf("Failed to get download link: %v", err))
					btnDownload.Enable()
				})
				return
			}

			fyne.Do(func() {
				btnDownload.Enable()
			})
			app.showDownloadDialog(directURL, filename, mName, fi, strconv.Itoa(mID))
		}(selectedMod.ModID, selectedMod.Name)
	})
	btnDownload.Importance = widget.HighImportance
	btnDownload.Disable()

	// Спойлер «Последние изменения»
	changelogLabel := widget.NewLabel(app.msg("downloading_changelog"))
	changelogLabel.Wrapping = fyne.TextWrapWord
	changelogContainer := container.NewVBox(changelogLabel)
	changelogContainer.Hide()

	btnChangelog = widget.NewButton(app.msg("btn_show_changelog"), func() {
		if selectedMod == nil {
			return
		}
		if !changelogExpanded {
			if !changelogFetched || changelogModID != selectedMod.ModID {
				changelogLabel.SetText(app.msg("downloading_changelog"))
				changelogModID = selectedMod.ModID
				mID := selectedMod.ModID
				go func() {
					fi, err := app.getLatestFileInfo(mID)
					var text string
					if err == nil {
						cl, _ := app.FetchChangelog(mID, fi.ID)
						text = stripHTML(cl)
					}
					if text == "" {
						text = app.msg("changelog_unavailable")
					}
					fyne.Do(func() {
						if selectedMod != nil && selectedMod.ModID == mID {
							changelogLabel.SetText(text)
							changelogFetched = true
						}
					})
				}()
			}
			changelogExpanded = true
			changelogContainer.Show()
			btnChangelog.SetText(app.msg("btn_hide_changelog"))
			detailsBox.Refresh()
		} else {
			changelogExpanded = false
			changelogContainer.Hide()
			btnChangelog.SetText(app.msg("btn_show_changelog"))
			detailsBox.Refresh()
		}
	})
	btnChangelog.Hide()

	descCardBg := canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	descCardBg.CornerRadius = 12
	descCardBg.StrokeWidth = 0.5
	descCardBg.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)

	linksCol := container.New(&VBoxWithSpacing{Spacing: SpacingLinks},
		descURL,
		sourceCodeLink,
	)

	metaLeft := container.NewVBox(
		authorLbl,
		widget.NewSeparator(),
		linksCol,
		widget.NewSeparator(),
		descVersionGrid,
	)
	metaRow := container.NewBorder(nil, nil, nil, thumbBox, metaLeft)

	titleSep := canvas.NewRectangle(th.Color(themes.ColorCRTScreenStroke, variant))
	titleSep.SetMinSize(fyne.NewSize(0, 3))

	detailsBox = container.NewVBox(
		titleLbl,
		titleSep,
		metaRow,
		descStatusSection,
		widget.NewSeparator(),
		summaryLbl,
		widget.NewSeparator(),
		btnChangelog,
		changelogContainer,
		layout.NewSpacer(),
		btnDownload,
	)

	detailsScroll := container.NewVScroll(detailsBox)
	detailsScroll.SetMinSize(fyne.NewSize(DescScrollMinWidth, DescScrollMinHeight))
	detailsCard := container.NewStack(descCardBg, container.NewPadded(detailsScroll))

	// --- 3. Обработка выбора в таблице ---
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row < 0 || id.Row >= len(results) {
			return
		}
		mod := &results[id.Row]
		selectedMod = mod

		changelogExpanded = false
		changelogFetched = false
		changelogModID = 0
		changelogLabel.SetText(app.msg("downloading_changelog"))
		changelogContainer.Hide()
		btnChangelog.SetText(app.msg("btn_show_changelog"))
		if mod.ModID != 0 {
			btnChangelog.Show()
		} else {
			btnChangelog.Hide()
		}

		titleLbl.Segments = createHighlightedSegments(mod.Name, currentSearchQuery, widget.RichTextStyle{TextStyle: fyne.TextStyle{Bold: true}, Inline: true})
		authorLbl.Segments = createHighlightedSegments(fmt.Sprintf(app.msg("author_label"), mod.Author), currentSearchQuery, widget.RichTextStyle{Inline: true})

		if mod.ModID != 0 {
			if u, err := url.Parse(fmt.Sprintf("https://www.nexusmods.com/warhammer40kdarktide/mods/%d", mod.ModID)); err == nil {
				descURL.SetURL(u)
				descURL.SetText(app.msg("mod_url_label"))
				descURL.Show()
			}
		} else {
			descURL.Hide()
		}

		if mod.SourceCodeURL != "" {
			if u, err := url.Parse(mod.SourceCodeURL); err == nil {
				sourceCodeLink.SetURL(u)
				sourceCodeLink.SetText(app.msg("source_code_url"))
				sourceCodeLink.Show()
			}
		} else {
			sourceCodeLink.Hide()
		}

		descVersionLblVersion.SetText(app.msg("table_lbl_version"))
		if mod.IsInstalled && mod.Version != "" {
			descVersionValVersion.SetText(mod.Version)
		} else {
			descVersionValVersion.SetText("-")
		}

		descVersionLblLatest.SetText(app.msg("table_lbl_latest"))
		descVersionValLatest.SetText(mod.LatestVersion)

		descVersionLblOrig.SetText(app.msg("original_upload_label") + ":")
		if !mod.OriginalUpload.IsZero() {
			descVersionValOrig.SetText(app.formatDate(mod.OriginalUpload, "dd-mm-yyyy"))
		} else {
			descVersionValOrig.SetText("-")
		}

		descVersionLblUpd.SetText(app.msg("last_updated_label") + ":")
		if !mod.LastUpdated.IsZero() {
			descVersionValUpd.SetText(app.formatDate(mod.LastUpdated, "dd-mm-yyyy"))
		} else {
			descVersionValUpd.SetText("-")
		}

		descCategoryLbl.SetText(app.msg("category_label") + ":")
		descCategoryVal.SetText(getLocalizedCategory(mod.Category))

		// Статус
		descStatusList.Objects = nil

		if mod.IsInstalled {
			switch mod.Source {
			case "manual":
				descStatusList.Add(app.statusRow("hand_vulcan", app.msg("status_manual")))
			default:
				descStatusList.Add(app.statusRow("nexus_orange", app.msg("status_nexus")))
			}
		}

		// ДОБАВЛЕНО: Отображение статуса устаревшего мода
		if mod.IsObsolete {
			descStatusList.Add(app.statusRow("skull", app.msg("desc_obsolete")))
		}

		app.cfgMutex.RLock()
		lang := app.cfg.Language
		app.cfgMutex.RUnlock()

		if row := app.createRequiredModsRow(mod.Folder, lang); row != nil {
			descStatusList.Add(row)
		}

		if len(descStatusList.Objects) > 0 {
			descStatusSection.Show()
		} else {
			descStatusSection.Hide()
		}
		descStatusList.Refresh()

		summaryLbl.Segments = createHighlightedSegments(mod.Summary, currentSearchQuery, widget.RichTextStyle{Inline: true})

		thumbBox.Hide()
		thumbnail.Resource = nil
		thumbnail.Image = nil
		thumbnail.Refresh()
		if mod.PictureURL != "" {
			thumbBox.Show()
			go func(u, expectedName string) {
				res, err := fetchNexusThumbnail(u)
				if err == nil {
					fyne.Do(func() {
						if selectedMod != nil && selectedMod.Name == expectedName {
							thumbnail.Resource = res
							thumbnail.Refresh()
						}
					})
				}
			}(mod.PictureURL, mod.Name)
		}

		btnDownload.Enable()

		// Принудительно обновляем контейнер и скролл, чтобы Fyne пересчитал высоту блоков после Show()/Hide() секции Required
		detailsBox.Refresh()
		detailsScroll.Refresh()

		if mod.ModID != 0 && app.getAuthToken() != "" {
			go func(m *ModBrowserItem, rowID int) {
				fi, err := app.getLatestFileInfo(m.ModID)
				if err != nil {
					return
				}
				oldest, _ := app.getOldestFileInfo(m.ModID)

				// Запрашиваем информацию о моде для получения количества скачиваний
				info, _ := app.FetchNexusModInfo(m.ModID, app.getAuthToken())

				fyne.Do(func() {
					if selectedMod == m {
						if fi != nil && fi.UploadedTimestamp > 0 {
							m.LastUpdated = time.Unix(fi.UploadedTimestamp, 0)
							descVersionValUpd.SetText(app.formatDate(m.LastUpdated, "dd-mm-yyyy"))
						}
						if oldest != nil && oldest.UploadedTimestamp > 0 {
							m.OriginalUpload = time.Unix(oldest.UploadedTimestamp, 0)
							descVersionValOrig.SetText(app.formatDate(m.OriginalUpload, "dd-mm-yyyy"))
						}

						if fi != nil {
							m.LatestVersion = fi.Version
							descVersionValLatest.SetText(m.LatestVersion)
						}

						// Обновляем количество скачиваний
						if info != nil && info.Downloads > 0 {
							m.Downloads = info.Downloads
						}

						// Обновляем таблицу (чтобы отобразилось Total DLs)
						table.Refresh()
					}
				})
			}(mod, id.Row)
		}
	}

	// --- 4. Поиск и Фильтры ---
	searchEntry := widget.NewEntry()

	// Обработка Esc: сброс поиска или снятие фокуса
	searchEntry.SetEscapeHandler(func() {
		if searchEntry.Text != "" {
			searchEntry.SetText("")
		} else {
			win.Canvas().Focus(table)
		}
	})

	// Определяем цвета для режимов
	colorPink := color.NRGBA{R: 255, G: 105, B: 180, A: 255}
	colorBlue := color.NRGBA{R: 0, G: 205, B: 255, A: 255}

	// Используем кастомную тему для Entry, чтобы избежать двойных рамок
	searchTheme := &modBrowserTheme{Theme: fyne.CurrentApp().Settings().Theme(), primaryColor: colorPink}
	searchEntryWithTheme := container.NewThemeOverride(searchEntry, searchTheme)

	// Кнопка очистки
	searchClearBtn := NewCustomButton("✕", func() {
		searchEntry.SetText("")
	})
	searchClearBtn.Importance = widget.DangerImportance
	searchClearBtn.Hide()

	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(100, 1))
	searchEntryBox := container.NewStack(searchSpacer, searchEntryWithTheme)

	searchBar := container.NewBorder(nil, nil, nil, searchClearBtn, searchEntryBox)

	statusLabel := widget.NewLabel("")
	statusLabel.Alignment = fyne.TextAlignCenter

	nexusStatusIcon := app.statusIcon("nexus_orange")
	nexusStatusIcon.Hide()

	statusBox := container.NewCenter(
		container.NewHBox(nexusStatusIcon, statusLabel),
	)

	// Галочка с текстом
	checkBox := widget.NewCheck(app.msg("setting_search_only_names"), nil)
	checkBox.SetChecked(false) // По умолчанию ищем по всей базе (описания, авторы)

	// Функция обновления UI при смене режима
	updateSearchModeUI := func(onlyNames bool) {
		phNames := app.msg("browser_search_ph_names")
		if phNames == "" {
			phNames = "Начните вводить название мода..."
		}

		phAll := app.msg("browser_search_ph_all")
		if phAll == "" {
			phAll = "Начните вводить название, описание или имя автора..."
		}

		if onlyNames {
			searchEntry.SetPlaceHolder(phNames)
			searchTheme.primaryColor = colorPink
		} else {
			searchEntry.SetPlaceHolder(phAll)
			searchTheme.primaryColor = colorBlue
		}
		searchEntryWithTheme.Refresh()
	}

	// Вызываем при старте (false = искать везде)
	updateSearchModeUI(false)

	var (
		btnSearchNexus   *CustomButton
		nexusTableBorder *canvas.Rectangle
		leftPanel        fyne.CanvasObject
	)

	stopSearchBlink := func() {
		if blinkSearchActive.Swap(false) {
			btnSearchNexus.Importance = widget.MediumImportance
			btnSearchNexus.Refresh()
		}
	}

	btnSearchNexus = NewCustomButton(app.msg("btn_browser_search_nexus"), func() {
		stopSearchBlink()

		q := searchEntry.Text
		currentSearchQuery = q
		if q == "" {
			return
		}
		statusLabel.SetText(app.msg("browser_search_nexus_api"))
		btnSearchNexus.Disable()

		// Показываем индикаторы Nexus
		nexusStatusIcon.Show()
		nexusStatusIcon.Refresh()
		nexusTableBorder.Show()
		nexusTableBorder.Refresh()
		statusBox.Refresh()
		leftPanel.Refresh()

		go func() {
			res, total, err := app.SearchNexusMods(q, "downloads", 0, checkBox.Checked)
			fyne.Do(func() {
				btnSearchNexus.Enable()
				if err != nil {
					statusLabel.SetText(fmt.Sprintf("Error: %v", err))
					return
				}
				results = res
				table.Refresh()
				if total > len(results) {
					statusLabel.SetText(fmt.Sprintf(
						app.msg("browser_found_nexus_total"),
						len(results), total,
					))
				} else {
					statusLabel.SetText(fmt.Sprintf(app.msg("browser_found_nexus"), len(results)))
				}
				selectedMod = nil
				btnDownload.Disable()
				descStatusSection.Hide()
			})
		}()
	})

	// Обработка Enter в поле поиска (запускает поиск по Nexus)
	searchEntry.OnSubmitted = func(s string) {
		if !btnSearchNexus.Disabled() {
			btnSearchNexus.OnTapped()
		}
	}

	// Выносим логику поиска в отдельную функцию, чтобы вызывать её из двух мест
	doSearch := func(s string) {
		currentSearchQuery = s
		if s == "" {
			if searchTimer != nil {
				searchTimer.Stop()
			}
			stopSearchBlink()
			results = nil
			table.Refresh()
			statusLabel.SetText("")
			selectedMod = nil
			btnDownload.Disable()
			descStatusSection.Hide()

			// Прячем индикаторы Nexus
			nexusStatusIcon.Hide()
			nexusTableBorder.Hide()
			statusBox.Refresh()
			leftPanel.Refresh()
			return
		}

		if searchTimer != nil {
			searchTimer.Stop()
		}
		statusLabel.SetText(app.msg("browser_typing_"))

		// При начале ввода локального поиска сразу убираем оранжевую рамку
		nexusStatusIcon.Hide()
		nexusTableBorder.Hide()
		statusBox.Refresh()
		leftPanel.Refresh()

		searchTimer = time.AfterFunc(300*time.Millisecond, func() {
			app.cfgMutex.RLock()
			lang := app.cfg.Language
			app.cfgMutex.RUnlock()

			res, isFuzzy := app.SearchLocalDatabase(s, lang, checkBox.Checked)

			fyne.Do(func() {
				if searchEntry.Text != s {
					return
				}

				if len(res) == 0 {
					results = nil
					table.Refresh()
					statusLabel.SetText(app.msg("browser_found_not_local"))
					selectedMod = nil
					btnDownload.Disable()
					descStatusSection.Hide()

					descStatusSection.Hide()

					app.startBlink(btnSearchNexus, &blinkSearchActive, func() bool { return true })
					return
				}

				stopSearchBlink()
				results = res
				table.UnselectAll()
				selectedMod = nil
				btnDownload.Disable()
				descStatusSection.Hide()
				table.Refresh()

				if isFuzzy {
					statusLabel.SetText(fmt.Sprintf(app.msg("browser_fuzzy_hint"), len(res)))
				} else {
					statusLabel.SetText(fmt.Sprintf(app.msg("browser_found_local"), len(res)))
				}
			})
		})
	}

	searchEntry.OnChanged = func(s string) {
		if s != "" {
			searchClearBtn.Show()
		} else {
			searchClearBtn.Hide()
		}
		searchBar.Refresh()
		doSearch(s)
	}

	// При переключении галочки перезапускаем поиск и обновляем цвета/плейсхолдер
	checkBox.OnChanged = func(b bool) {
		updateSearchModeUI(b)
		doSearch(searchEntry.Text)
	}

	// --- 5. Сборка лэйаута ---
	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	var tableContent fyne.CanvasObject = table
	if mechData != nil {
		mechBg := canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechBg.FillMode = canvas.ImageFillContain
		mechBg.Translucency = TableBackgroundOpacity
		tableContent = container.NewStack(mechBg, table)
	}

	// Оранжевая рамка для результатов Nexus API
	nexusTableBorder = canvas.NewRectangle(color.Transparent)
	nexusTableBorder.StrokeWidth = 2
	nexusTableBorder.StrokeColor = color.NRGBA{R: 255, G: 140, B: 0, A: 255}
	nexusTableBorder.Hide()

	// Строка поиска остается наверху, а сплиттер разделяет Таблицу+Заголовки и Карточку
	topPanelBg := canvas.NewRectangle(th.Color(themes.ColorTopPanelBg, variant))
	topBarContent := container.NewVBox(
		container.NewPadded(container.NewBorder(nil, nil, btnSearchNexus, checkBox, searchBar)),
		statusBox,
		widget.NewSeparator(),
	)
	topBar := container.NewStack(topPanelBg, topBarContent)

	// Оборачиваем левую панель в Stack, чтобы рамка рисовалась ПОВЕРХ таблицы и заголовков
	leftPanelInner := container.NewBorder(headerTable, nil, nil, nil, tableContent)
	leftPanel = container.NewStack(leftPanelInner, nexusTableBorder)

	split := container.NewHSplit(leftPanel, detailsCard)
	split.Offset = 0.55

	content := container.NewBorder(topBar, nil, nil, nil, split)
	win.SetContent(content)

	// Обработчик смены языка
	activeBrowserUpdate = func() {
		headerTable.Refresh()
		table.Refresh()
		updateSearchModeUI(checkBox.Checked)
		checkBox.SetText(app.msg("setting_search_only_names"))
		win.SetTitle(app.msg("browser_title"))

		if searchEntry.Text != "" {
			doSearch(searchEntry.Text)
		}
	}

	// Горячие клавиши: Ctrl+F для фокуса на поиске
	win.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyF,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		win.Canvas().Focus(searchEntry)
	})

	win.Show()
}

// createHighlightedSegments разбивает текст на сегменты, подсвечивая
// найденный запрос жирным шрифтом и цветом.
func createHighlightedSegments(text, query string, baseStyle widget.RichTextStyle) []widget.RichTextSegment {
	if query == "" {
		return []widget.RichTextSegment{
			&widget.TextSegment{Text: text, Style: baseStyle},
		}
	}

	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)

	var segments []widget.RichTextSegment
	start := 0

	for {
		idx := strings.Index(lowerText[start:], lowerQuery)
		if idx == -1 {
			// Добавляем оставшийся хвост текста
			if start < len(text) {
				segments = append(segments, &widget.TextSegment{
					Text:  text[start:],
					Style: baseStyle,
				})
			}
			break
		}

		actualIdx := start + idx

		// Текст ДО совпадения
		if actualIdx > start {
			segments = append(segments, &widget.TextSegment{
				Text:  text[start:actualIdx],
				Style: baseStyle,
			})
		}

		// Сам НАЙДЕННЫЙ текст (подсвечиваем фоном)
		highlightStyle := baseStyle
		highlightStyle.TextStyle.Bold = true

		// Белый текст на цветной подложке.
		// В стандартной теме белый даёт, например, ColorNameForegroundOnError
		// (или ForegroundOnPrimary). Подставьте тот, что в вашей теме — белый.
		highlightStyle.ColorName = theme.ColorNameForegroundOnError

		// Подложка цвета, как вы просили — #c0ff1a в вашей теме
		highlightStyle.BackgroundColorName = theme.ColorNameForegroundOnWarning

		segments = append(segments, &widget.TextSegment{
			Text:  text[actualIdx : actualIdx+len(query)],
			Style: highlightStyle,
		})

		start = actualIdx + len(query)
	}

	return segments
}
