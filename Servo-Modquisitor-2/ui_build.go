// Servo-Modquisitor-2/ui_build.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/themes"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// createTableRow — пустая строка с заданной минимальной высотой.
// Используется и для системной таблицы, и как шаблон ячейки основной
// таблицы: Fyne в templateSize() создаёт свежий экземпляр через
// CreateCell и берёт его MinSize — здесь эту высоту задаёт пустая
// widget.Label (её MinSize ≈ высота строки текста), а spacer задаёт
// нижнюю границу TableRowHeight.
func createTableRow(height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(1, height))
	lbl := widget.NewLabel("")
	return container.NewStack(spacer, lbl)
}

// VBoxWithSpacing — вертикальный layout с заданным отступом между элементами.
type VBoxWithSpacing struct {
	Spacing float32
}

func (v VBoxWithSpacing) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	if len(objects) == 0 {
		return
	}
	var totalHeight float32
	for _, obj := range objects {
		totalHeight += obj.MinSize().Height
	}
	totalHeight += v.Spacing * float32(len(objects)-1)

	y := (size.Height - totalHeight) / 2
	if y < 0 {
		y = 0
	}
	for _, obj := range objects {
		minSize := obj.MinSize()
		obj.Resize(fyne.NewSize(size.Width, minSize.Height))
		obj.Move(fyne.NewPos(0, y))
		y += minSize.Height + v.Spacing
	}
}

func (v VBoxWithSpacing) MinSize(objects []fyne.CanvasObject) fyne.Size {
	if len(objects) == 0 {
		return fyne.NewSize(0, 0)
	}
	var maxWidth float32
	var totalHeight float32
	for _, obj := range objects {
		min := obj.MinSize()
		if min.Width > maxWidth {
			maxWidth = min.Width
		}
		totalHeight += min.Height
	}
	totalHeight += v.Spacing * float32(len(objects)-1)
	return fyne.NewSize(maxWidth, totalHeight)
}

// buildUI — оркестратор сборки главного окна. Вся конкретика разложена
// по build-методам ниже: каждый отвечает за одну область UI и
// устанавливает соответствующие поля App.
func (app *App) buildUI() {
	app.buildControlButtons()
	app.buildConsole()
	filterSelectWithSize, searchBar := app.buildSearchAndFilter()
	app.buildManagePanel()
	topPanelWithBg := app.buildTopPanel(filterSelectWithSize, searchBar)
	app.buildHeaderTable()
	app.buildSystemModsTable()
	app.buildModsTable()
	bottomPanel := app.buildBottomPanel()
	rightContent := app.buildDescriptionCard()

	app.assembleMainLayout(topPanelWithBg, bottomPanel, rightContent)

	app.appendCenteredLog(app.msg("log_start0"))
	app.filterModList()
	app.updateTableBorder()
	app.setupShortcuts()
}

// ─────────────────────────────────────────────────────────────────
// Иконки и кнопки
// ─────────────────────────────────────────────────────────────────

// buildControlButtons создаёт все CustomButton, используемые в верхней
// панели, панели массовых операций и в карточке описания. Устанавливает
// их в поля App.
//
// Иконки для toggle-кнопки (on/off) сохраняются в App, потому что нужны
// в updateToggleButtonText при смене темы или состояния.
func (app *App) buildControlButtons() {
	// ─── Иконки ────────────────────────────────────────────────────
	upRes := app.loadIconResource("up", "assets/buttons/up.png")
	downRes := app.loadIconResource("down", "assets/buttons/down.png")
	topRes := app.loadIconResource("top", "assets/buttons/top.png")
	bottomRes := app.loadIconResource("bottom", "assets/buttons/bottom.png")
	selTrashRes := app.loadIconResource("trash_sel", "assets/buttons/trashcan_red_sel.png")
	trashRes := app.loadIconResource("trash", "assets/buttons/trashcan_red.png")
	trashXRes := app.loadIconResource("trash_x", "assets/buttons/trashcan_red_x.png")
	editVersionRes := app.loadIconResource("edit_version", "assets/buttons/edit_version.png")
	saveRes := app.loadIconResource("save", "assets/buttons/save.png")
	refreshRes := app.loadIconResource("refresh", "assets/buttons/refresh.png")
	addRes := app.loadIconResource("add", "assets/buttons/add.png")
	autosortRes := app.loadIconResource("autosort", "assets/buttons/sort.png")
	checkUpdatesRes := app.loadIconResource("check_updates", "assets/buttons/check_updates_blue.png")
	updateAllRes := app.loadIconResource("update_all", "assets/buttons/update_all_mods_blue_p.png")
	updateSelRes := app.loadIconResource("update_sel", "assets/buttons/update_selected_blue_p.png")
	playRes := app.loadIconResource("play", "assets/buttons/play.png")
	playFastRes := app.loadIconResource("play_fast", "assets/buttons/play_fast.png")
	updateRes := app.loadIconResource("update", "assets/buttons/upd_download_blue_p.png")
	folderRes := app.loadIconResource("folder", "assets/buttons/folder_open.png")
	checkedBoxRes := app.loadIconResource("checked_box", "assets/buttons/checked_box.png")
	UncheckedBoxRedRes := app.loadIconResource("unchecked_box_red", "assets/buttons/unchecked_box_red.png")
	enableAllRes := app.loadIconResource("enable_all", "assets/buttons/enable_all.png")
	disableAllRes := app.loadIconResource("disable_all", "assets/buttons/disable_all.png")
	cogRes := app.loadIconResource("cog", "assets/buttons/cog_check.png")
	selectAllRes := app.loadIconResource("select_all", "assets/buttons/select_all.png")
	selectAllDeRes := app.loadIconResource("select_all_de", "assets/buttons/select_all_de.png")
	onRes := app.loadIconResource("on", "assets/buttons/on.png")
	offRes := app.loadIconResource("off", "assets/buttons/off_red.png")

	app.toggleOnIcon = onRes
	app.toggleOffIcon = offRes

	if colImgData, _ := embeddedFiles.ReadFile(ColBackgroundImage); colImgData != nil {
		app.selectColumnBgRes = fyne.NewStaticResource("Yellow_BG_col", colImgData)
	}

	// ─── Перемещение / выбор ──────────────────────────────────────
	app.moveToTopBtn = NewIconButton(topRes, func() { app.moveSelectedToTop() })
	app.moveToTopBtn.SetToolTip(app.msg("btn_move_to_top_tooltip"))

	app.moveToBottomBtn = NewIconButton(bottomRes, func() { app.moveSelectedToBottom() })
	app.moveToBottomBtn.SetToolTip(app.msg("btn_move_to_bottom_tooltip"))

	app.moveToEntry = widget.NewEntry()
	app.moveToEntry.SetPlaceHolder(app.msg("col_number"))
	app.moveToEntry.OnSubmitted = func(text string) { app.moveSelectedToPosition() }
	app.moveLabel = widget.NewLabel(app.msg("lbl_move_to"))

	app.selectAllBtn = NewIconButton(selectAllRes, func() { app.selectAllMods(true) })
	app.selectAllBtn.SetToolTip(app.msg("btn_select_all_tooltip"))

	app.deselectAllBtn = NewIconButton(selectAllDeRes, func() { app.selectAllMods(false) })
	app.deselectAllBtn.SetToolTip(app.msg("btn_deselect_all_tooltip"))

	// ─── Удаление ─────────────────────────────────────────────────
	app.btnRemoveAll = NewIconButton(trashXRes, func() {
		app.showConfirmDialog(
			app.msg("confirm_remove_all_title"),
			app.msg("confirm_remove_all_text"),
			func() { app.removeAllMods() },
		)
	})
	app.btnRemoveAll.SetToolTip(app.msg("btn_remove_all_tooltip"))

	app.btnRemoveSelected = NewIconButton(selTrashRes, func() {
		sel := app.selectedMods()
		if len(sel) == 0 {
			app.appendLog(app.msg("no_mods_selected"))
			return
		}
		app.showConfirmDialog(
			app.msg("confirm_remove_selected_title"),
			fmt.Sprintf(app.msg("confirm_remove_selected_text"), len(sel)),
			func() { app.removeSelectedMods() },
		)
	})
	app.btnRemoveSelected.SetToolTip(app.msg("btn_remove_selected_tooltip"))

	// ─── Версия / порядок ─────────────────────────────────────────
	app.btnEditVersion = NewIconButton(editVersionRes, func() {
		if app.selectedModName == "" {
			return
		}
		mod, ok := app.findModByName(app.selectedModName)
		if !ok {
			return
		}
		app.showEditVersionDialog(&mod)
	})
	app.btnEditVersion.SetToolTip(app.msg("btn_edit_version_tooltip"))

	app.btnUp = NewIconButton(upRes, func() { app.moveSelected(-1) })
	app.btnUp.SetToolTip(app.msg("btn_up_tooltip"))

	app.btnDown = NewIconButton(downRes, func() { app.moveSelected(1) })
	app.btnDown.SetToolTip(app.msg("btn_down_tooltip"))

	// ─── Сохранение / обновление ──────────────────────────────────
	app.btnSaveOrder = NewIconButton(saveRes, func() {
		if app.orderDirty {
			app.saveCurrentOrder()
			app.orderDirty = false
			app.refreshModList()
			app.appendLog(app.msg("log_order_saved"))
			app.stopBlinkSaveButton()
			app.updateTableBorder()
		} else {
			app.appendLog(app.msg("log_order_unchanged"))
		}
	})
	app.btnSaveOrder.SetToolTip(app.msg("btn_save_order_tooltip"))

	app.btnRefresh = NewIconButton(refreshRes, func() {
		go app.refreshWithDirtyCheck()
	})
	app.btnRefresh.SetToolTip(app.msg("btn_refresh_tooltip"))

	app.btnToggle = NewIconButton(onRes, func() { app.toggleGlobalMods() })
	app.btnToggle.SetToolTip(app.msg("btn_toggle_tooltip"))
	app.updateToggleButtonText(app.btnToggle)

	// ─── Массовые операции ────────────────────────────────────────
	app.enableSelectedBtn = NewIconButton(checkedBoxRes, func() { app.setSelectedActive(true) })
	app.enableSelectedBtn.SetToolTip(app.msg("btn_enable_selected_tooltip"))

	app.disableSelectedBtn = NewIconButton(UncheckedBoxRedRes, func() { app.setSelectedActive(false) })
	app.disableSelectedBtn.SetToolTip(app.msg("btn_disable_selected_tooltip"))

	app.enableAllBtn = NewIconButton(enableAllRes, func() { app.setAllModsActive(true) })
	app.enableAllBtn.SetToolTip(app.msg("btn_enable_all_tooltip"))

	app.disableAllBtn = NewIconButton(disableAllRes, func() { app.setAllModsActive(false) })
	app.disableAllBtn.SetToolTip(app.msg("btn_disable_all_tooltip"))

	// ─── Управление, установка, обновления ───────────────────────
	app.manageBtn = NewIconButton(cogRes, func() { app.toggleManagePanel() })
	app.manageBtn.SetToolTip(app.msg("btn_manage_mods_tooltip"))

	if btnImgData, _ := embeddedFiles.ReadFile(ButtonBackgroundImage); btnImgData != nil {
		img := canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG_button", btnImgData))
		img.FillMode = canvas.ImageFillStretch
		img.Translucency = 0.8
		app.manageBtn.SetBackgroundImage(img)
	}

	app.btnUpdateSelected = NewIconButton(updateSelRes, func() {
		go app.updateSelectedMods()
	})
	app.btnUpdateSelected.SetToolTip(app.msg("btn_update_selected_tooltip"))

	app.btnAMLConfig = NewCustomButton(app.msg("btn_aml_config"), func() { app.showAMLConfigWindow() })
	app.btnAMLConfig.SetToolTip(app.msg("btn_aml_config_tooltip"))

	app.btnInstall = NewIconButton(addRes, func() { app.pickAndInstallArchive() })
	app.btnInstall.SetToolTip(app.msg("btn_install_tooltip"))

	app.btnSortChecks = NewIconButton(autosortRes, func() { go app.runAllChecks() })
	app.btnSortChecks.SetIconSize(32)
	app.btnSortChecks.SetToolTip(app.msg("btn_sort_checks_tooltip"))

	if app.amlDetected.Load() {
		app.btnSaveOrder.SetToolTip(app.msg("aml_save_warning_tooltip"))
		app.btnSortChecks.SetToolTip(app.msg("aml_sort_warning_tooltip"))
	}

	app.btnCheckUpdates = NewIconButton(checkUpdatesRes, func() {
		go app.checkNexusUpdates()
	})
	app.btnCheckUpdates.SetToolTip(app.msg("btn_check_updates_tooltip"))

	app.btnUpdateAll = NewIconButton(updateAllRes, func() {
		go app.updateAllModsFromNexus()
	})
	app.btnUpdateAll.SetToolTip(app.msg("btn_update_all_premium_only"))

	// ─── Запуск игры ──────────────────────────────────────────────
	gameRoot, _ := app.getGameState()
	gameVer := detectGameVersion(gameRoot)
	if gameVer == VersionUnknown {
		app.btnLaunchNormal = NewIconButton(playRes, func() { app.launchGameFromUI(false) })
		app.btnLaunchNoLauncher = NewIconButton(playFastRes, func() { app.launchGameFromUI(true) })
		app.btnLaunchNormal.Hide()
		app.btnLaunchNoLauncher.Hide()
	} else {
		app.btnLaunchNormal = NewIconButton(playRes, func() { app.launchGameFromUI(false) })
		app.btnLaunchNormal.SetToolTip(app.msg("btn_launch_game_tooltip"))

		app.btnLaunchNoLauncher = NewIconButton(playFastRes, func() { app.launchGameFromUI(true) })
		app.btnLaunchNoLauncher.SetIconSize(32)
		app.btnLaunchNoLauncher.SetToolTip(app.msg("btn_launch_nolauncher_long_tooltip"))
	}

	// ─── Кнопка удаления мода в карточке описания ────────────────
	app.btnRemove = NewIconButton(trashRes, func() { app.confirmRemoveSelectedDescription() })
	app.btnRemove.SetToolTip(app.msg("btn_remove_tooltip"))

	// ─── Кнопка "обновить мод" в карточке ────────────────────────
	app.btnUpdateMod = NewIconButton(updateRes, func() { app.updateModFromDescription() })
	app.btnUpdateMod.SetToolTip(app.msg("btn_update_mod_premium_only"))

	// ─── Кнопка "открыть папку" в карточке ───────────────────────
	app.openFolderBtn = NewIconButton(folderRes, func() { app.openSelectedModFolder() })
	app.openFolderBtn.Importance = widget.MediumImportance
	app.openFolderBtn.SetToolTip(app.msg("open_mod_folder_tooltip"))
}

// loadIconResource читает иконку из embed, логирует ошибку и возвращает
// StaticResource. При ошибке возвращает nil — Fyne корректно обрабатывает
// nil-иконки (кнопка останется без картинки).
func (app *App) loadIconResource(name, path string) fyne.Resource {
	data, err := embeddedFiles.ReadFile(path)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Could not load %s icon (%s): %v", name, path, err))
		return nil
	}
	return fyne.NewStaticResource(name, data)
}

// ─────────────────────────────────────────────────────────────────
// Мелкие обработчики кнопок (вынесены из closures для читаемости)
// ─────────────────────────────────────────────────────────────────

// refreshWithDirtyCheck — логика кнопки "обновить список": если есть
// несохранённые изменения, спрашивает пользователя, что делать.
// Использует асинхронный диалог, чтобы не блокировать горутину.
func (app *App) refreshWithDirtyCheck() {
	if app.orderDirty {
		app.showChoiceDialog(
			app.mainWindow,
			app.msg("warning_title"),
			app.msg("refresh_discard_changes"),
			func(choice int) {
				// callback выполняется в UI-потоке, поэтому fyne.Do не нужен
				switch choice {
				case 0:
					app.saveCurrentOrder()
					app.orderDirty = false
					app.stopBlinkSaveButton()
					app.updateTableBorder()
					app.appendLog(app.msg("log_order_saved"))
					app.refreshModList()
					app.appendLog(app.msg("log_list_refreshed"))
				case 1:
					// Отмена — ничего не делаем
				case 2:
					app.orderDirty = false
					app.stopBlinkSaveButton()
					app.updateTableBorder()
					app.refreshModList()
					app.appendLog(app.msg("log_list_refreshed"))
				}
			},
			app.msg("btn_save_and_refresh"),
			app.msg("btn_cancel"),
			app.msg("btn_refresh_anyway"),
		)
	} else {
		// Если изменений нет — просто обновляем список.
		// Функция может вызываться из горутины, поэтому используем fyne.Do.
		fyne.Do(func() {
			app.refreshModList()
			app.appendLog(app.msg("log_list_refreshed"))
		})
	}
}

// toggleManagePanel — логика кнопки-шестерёнки: показать/скрыть панель
// массовых операций и колонку чекбоксов.
func (app *App) toggleManagePanel() {
	if app.managePanel.Visible() {
		app.managePanel.Hide()
		app.showSelectColumn = false
		app.headerTable.SetColumnWidth(0, 0)
		app.modTable.SetColumnWidth(0, 0)
		app.modTable.Refresh()
		app.headerTable.Refresh()
	} else {
		app.managePanel.Show()
		app.showSelectColumn = true
		app.headerTable.SetColumnWidth(0, ColSelectWidth)
		app.modTable.SetColumnWidth(0, ColSelectWidth)
		app.modTable.Refresh()
		app.headerTable.Refresh()
	}
	app.managePanel.Refresh()
}

// pickAndInstallArchive — логика кнопки "установить мод": открывает
// диалог выбора архива и запускает установку.
func (app *App) pickAndInstallArchive() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		path := reader.URI().Path()
		if !strings.HasSuffix(strings.ToLower(path), ".zip") {
			app.appendLogToFile(app.msg("log_zip_only"))
			return
		}
		go func(p string) {
			installedName, _, err := app.InstallModFromArchive(p, true, "", "")
			fyne.Do(func() {
				if err != nil {
					app.appendLogToFile(fmt.Sprintf(app.msg("log_extract_error"), err))
					return
				}
				checks.AutoFixMalformed()
				app.refreshModList()
				app.selectAndScrollToMod(installedName)
				app.appendLog(fmt.Sprintf(app.msg("log_installed"), filepath.Base(p)))
			})
		}(path)
	}, app.mainWindow)
	fd.SetFilter(storage.NewExtensionFileFilter([]string{".zip", ".rar", ".7z"}))
	fd.Show()
	fd.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
}

// launchGameFromUI — логика кнопок запуска игры (обычной и без лаунчера).
func (app *App) launchGameFromUI(skipLauncher bool) {
	gameRoot, _ := app.getGameState()
	go func(root string) {
		if isDarktideRunning() {
			app.appendLog(app.msg("game_already_running"))
			return
		}
		ver := detectGameVersion(root)
		if err := app.launchGameFunc(ver, root, skipLauncher); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("launch_error"), err))
		}
	}(gameRoot)
}

// confirmRemoveSelectedDescription — логика кнопки удаления в карточке
// описания: подтверждение + удаление + восстановление выделения.
func (app *App) confirmRemoveSelectedDescription() {
	if app.selectedModName == "" {
		return
	}
	modName := app.selectedModName
	mod, ok := app.findModByName(modName)
	if !ok || mod.IsSystem {
		app.appendLog(app.msg("log_cannot_delete_system"))
		return
	}

	var nextModName string
	for i, m := range app.displayedMods {
		if m.Name == modName {
			if i+1 < len(app.displayedMods) {
				nextModName = app.displayedMods[i+1].Name
			} else if i-1 >= 0 {
				nextModName = app.displayedMods[i-1].Name
			}
			break
		}
	}

	app.showConfirmDialog(
		app.msg("confirm_delete_title"),
		fmt.Sprintf(app.msg("confirm_delete_text"), mod.Name),
		func() {
			checks.RemoveMod(modName)
			app.removeModFromCache(modName)
			oldIndex, _ := app.removeModFromData(modName)

			app.updateModCounter()
			app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
			app.modTable.Refresh()
			app.updateTableBorder()
			app.appendLog(fmt.Sprintf(app.msg("log_deleted"), modName))

			app.saveCurrentOrder()
			app.syncProfileFromGame()
			app.orderDirty = false
			app.updateTableBorder()

			if nextModName != "" {
				for i, m := range app.displayedMods {
					if m.Name == nextModName {
						app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
						app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
						break
					}
				}
			} else if len(app.displayedMods) > 0 {
				newIndex := oldIndex
				if newIndex >= len(app.displayedMods) {
					newIndex = len(app.displayedMods) - 1
				}
				if newIndex >= 0 {
					app.modTable.Select(widget.TableCellID{Row: newIndex, Col: 0}, 0)
					app.modTable.ScrollTo(widget.TableCellID{Row: newIndex, Col: 0})
				}
			} else {
				app.selectedModName = ""
				app.selectedModIndex.Store(-1)
				app.updateDescriptionForMod("")
				app.updateUpDownButtons()
			}
		},
	)
}

// updateModFromDescription — логика кнопки "обновить мод" в карточке:
// определяет системный/обычный мод, запускает нужный сценарий в фоне.
func (app *App) updateModFromDescription() {
	if app.selectedModName == "" {
		return
	}
	mod, ok := app.findModByName(app.selectedModName)
	if !ok {
		return
	}
	if mod.URL == "" {
		app.appendLog(app.msg("update_no_url"))
		return
	}
	go func() {
		switch mod.Name {
		case "base":
			app.updateDML()
		case "dmf":
			app.updateDMF()
		case "autopatch":
			app.updateAutopatcher()
		default:
			m := mod
			app.updateModFromNexus(&m, false)
		}
	}()
}

// openSelectedModFolder — логика кнопки "открыть папку мода" в карточке.
func (app *App) openSelectedModFolder() {
	if app.selectedModName == "" {
		return
	}
	mod, ok := app.findModByName(app.selectedModName)
	if !ok || mod.MissingFolder {
		return
	}
	modPath := filepath.Join(app.cfg.ModsPath, mod.Name)
	if _, err := os.Stat(modPath); err != nil {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", modPath)
	case "linux":
		cmd = exec.Command("xdg-open", modPath)
	case "darwin":
		cmd = exec.Command("open", modPath)
	default:
		u, _ := url.Parse("file://" + filepath.ToSlash(modPath))
		_ = app.myApp.OpenURL(u)
		return
	}
	if cmd != nil {
		cmd.Start()
	}
}

// ─────────────────────────────────────────────────────────────────
// Консоль / лог
// ─────────────────────────────────────────────────────────────────

// buildConsole собирает левую-нижнюю область с CRT-консолью и логом.
// Устанавливает app.logWindow, app.screenBgRect, app.headerBoxBgRect,
// app.logHeaderText, app.consoleScroll.
func (app *App) buildConsole() {
	app.logWindow = widget.NewRichText(
		&widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: theme.ColorNameForegroundOnWarning,
				TextStyle: fyne.TextStyle{},
			},
		},
	)
	app.logWindow.Wrapping = fyne.TextWrapWord

	crtData, _ := embeddedFiles.ReadFile(ConsoleBackgroundImage)
	var crtImg *canvas.Image
	var grad *canvas.Image
	if crtData != nil {
		crtImg = canvas.NewImageFromResource(fyne.NewStaticResource("CRT_BlackBG", crtData))
		crtImg.FillMode = canvas.ImageFillStretch
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
		grad.Translucency = ConsoleGradientOpacity
	} else {
		grad = canvas.NewImageFromImage(app.makeCRTGradient(1000, 800))
		grad.FillMode = canvas.ImageFillStretch
	}

	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	app.screenBgRect = canvas.NewRectangle(th.Color(themes.ColorCRTScreenFill, variant))
	app.screenBgRect.CornerRadius = 22
	app.screenBgRect.StrokeWidth = 2
	app.screenBgRect.StrokeColor = th.Color(themes.ColorCRTScreenStroke, variant)

	app.logHeaderText = canvas.NewText("", th.Color(themes.ColorConsoleText, variant))
	app.logHeaderText.TextStyle = fyne.TextStyle{Bold: true}
	app.logHeaderText.Alignment = fyne.TextAlignCenter
	app.logHeaderText.TextSize = theme.TextSize()

	logStack := container.NewStack()
	if crtImg != nil {
		logStack.Add(crtImg)
	}
	logStack.Add(grad)
	logStack.Add(app.screenBgRect)
	logStack.Add(container.NewPadded(app.logWindow))

	app.headerBoxBgRect = canvas.NewRectangle(th.Color(themes.ColorCRTHeaderBg, variant))
	headerBox := container.NewStack(
		app.headerBoxBgRect,
		container.NewCenter(app.logHeaderText),
	)

	logPanel := container.NewBorder(headerBox, nil, nil, nil, logStack)

	app.consoleScroll = container.NewScroll(logPanel)
	app.consoleScroll.SetMinSize(fyne.NewSize(ConsoleWidth, ConsoleHeight))
}

// ─────────────────────────────────────────────────────────────────
// Поиск и фильтр
// ─────────────────────────────────────────────────────────────────

// buildSearchAndFilter создаёт поле поиска и выпадающий список фильтров.
// Устанавливает app.searchEntry, app.searchClearBtn, app.filterSelect.
//
// Возвращает готовые к размещению в верхней панели контейнеры
// (фильтр с фиксированной минимальной шириной и панель поиска с кнопкой
// очистки), чтобы buildTopPanel не собирал их заново.
func (app *App) buildSearchAndFilter() (filterSelectWithSize fyne.CanvasObject, searchBar fyne.CanvasObject) {
	app.searchEntry = widget.NewEntry()
	app.searchEntry.SetEscapeHandler(func() {
		if app.modTable != nil {
			app.mainWindow.Canvas().Focus(app.modTable)
		} else {
			app.mainWindow.Canvas().Unfocus()
		}
		app.appendLogToFile("Поиск закрыт по Esc (стандартный Entry)")
	})
	app.searchEntry.SetPlaceHolder(app.msg("search_placeholder"))

	searchSpacer := canvas.NewRectangle(color.Transparent)
	searchSpacer.SetMinSize(fyne.NewSize(SearchMinWidth, 1))
	searchEntryBox := container.NewStack(searchSpacer, app.searchEntry)

	app.searchClearBtn = NewCustomButton("✕", func() {
		app.searchEntry.SetText("")
	})
	app.searchClearBtn.Importance = widget.DangerImportance
	app.searchClearBtn.Hide()

	app.searchEntry.OnChanged = func(s string) {
		if s != "" {
			app.searchClearBtn.Show()
		} else {
			app.searchClearBtn.Hide()
		}
		app.filterModList()
	}

	searchBar = container.NewBorder(nil, nil, nil, app.searchClearBtn, searchEntryBox)

	app.filterSelect = widget.NewSelect(app.filterOptions(), nil)
	app.filterSelect.SetSelected(app.msg("filter_all"))
	app.filterSelect.OnChanged = func(s string) { app.filterModList() }

	filterSpacer := canvas.NewRectangle(color.Transparent)
	filterSpacer.SetMinSize(fyne.NewSize(FilterMinWidth, 1))
	filterSelectWithSize = container.NewStack(filterSpacer, app.filterSelect)

	return filterSelectWithSize, searchBar
}

// ─────────────────────────────────────────────────────────────────
// Панель массовых операций
// ─────────────────────────────────────────────────────────────────

// buildManagePanel создаёт панель с кнопками массовых операций и
// скрывает её по умолчанию. Устанавливает app.managePanelBgRect,
// app.managePanel.
func (app *App) buildManagePanel() {
	singleRow := container.NewHBox(
		app.moveLabel,
		app.moveToEntry,
		widget.NewSeparator(),
		app.btnUp,
		app.btnDown,
		app.moveToTopBtn,
		app.moveToBottomBtn,
		widget.NewSeparator(),
		app.selectAllBtn,
		app.deselectAllBtn,
		widget.NewSeparator(),
		app.enableSelectedBtn,
		app.disableSelectedBtn,
		app.enableAllBtn,
		app.disableAllBtn,
		widget.NewSeparator(),
		app.btnEditVersion,
		widget.NewSeparator(),
		app.btnUpdateSelected,
		widget.NewSeparator(),
		app.btnRemoveSelected,
		app.btnRemoveAll,
	)

	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	yellowData, _ := embeddedFiles.ReadFile(HeaderBackgroundImage)
	var yellowBg *canvas.Image
	if yellowData != nil {
		yellowBg = canvas.NewImageFromResource(fyne.NewStaticResource("Yellow_BG", yellowData))
		yellowBg.FillMode = canvas.ImageFillStretch
		yellowBg.Translucency = 0.9
	}

	app.managePanelBgRect = canvas.NewRectangle(th.Color(themes.ColorManagePanelBg, variant))
	panelContent := container.NewVBox(singleRow)
	if yellowBg != nil {
		app.managePanel = container.NewStack(app.managePanelBgRect, yellowBg, panelContent)
	} else {
		app.managePanel = container.NewStack(app.managePanelBgRect, panelContent)
	}
	app.managePanel.Hide()
}

// ─────────────────────────────────────────────────────────────────
// Верхняя панель
// ─────────────────────────────────────────────────────────────────

// buildTopPanel собирает верхнюю панель с основными кнопками, фильтром
// и поиском. Устанавливает app.topPanelBgRect и возвращает готовый
// контейнер с фоном.
func (app *App) buildTopPanel(filterSelectWithSize, searchBar fyne.CanvasObject) fyne.CanvasObject {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	app.topPanelBgRect = canvas.NewRectangle(th.Color(themes.ColorTopPanelBg, variant))
	content := container.NewHBox(
		app.btnInstall,
		app.btnRefresh,
		app.btnSaveOrder,
		widget.NewSeparator(),
		app.btnSortChecks,
		widget.NewSeparator(),
		app.manageBtn,
		widget.NewSeparator(),
		filterSelectWithSize,
		searchBar,
		widget.NewSeparator(),
		app.btnCheckUpdates,
		app.btnUpdateAll,
		widget.NewSeparator(),
		app.btnToggle,
		widget.NewSeparator(),
		app.btnLaunchNormal,
		app.btnLaunchNoLauncher,
	)
	return container.NewStack(app.topPanelBgRect, content)
}

// ─────────────────────────────────────────────────────────────────
// Таблицы
// ─────────────────────────────────────────────────────────────────

// buildHeaderTable создаёт таблицу-заголовок над основной таблицей модов.
func (app *App) buildHeaderTable() {
	createCell := func() fyne.CanvasObject {
		return container.NewStack(
			canvas.NewRectangle(color.Transparent),
			widget.NewLabel(""),
		)
	}
	updateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		th := fyne.CurrentApp().Settings().Theme()
		variant := fyne.CurrentApp().Settings().ThemeVariant()
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		bg := canvas.NewRectangle(th.Color(themes.ColorTableHeaderBg, variant))
		cont.Add(bg)
		label := widget.NewLabel("")
		label.TextStyle = fyne.TextStyle{Bold: true}
		label.Alignment = fyne.TextAlignCenter
		switch id.Col {
		case 0:
			if app.showSelectColumn {
				label.SetText("⚙")
			}
		case 1:
			label.SetText(app.msg("col_checkbox"))
		case 2:
			label.SetText(app.msg("col_number"))
		case 3:
			label.SetText(app.msg("col_name"))
		case 4:
			label.SetText(app.msg("col_date"))
		case 5:
			label.SetText(app.msg("col_status"))
		case 6:
			label.SetText(app.msg("col_note"))
		}
		cont.Add(label)
	}
	app.headerTable = widget.NewTable(
		func() (int, int) { return 1, TableColumnCount },
		createCell,
		updateCell,
	)
	ApplyTableColumnWidths(app.headerTable)
	app.headerTable.SetColumnWidth(0, 0)
	app.headerTable.OnSelected = nil
}

// buildSystemModsTable создаёт таблицу системных модов (base/dmf/autopatch).
// Устанавливает app.systemModsTable, app.systemModsTableContainer.
func (app *App) buildSystemModsTable() {
	updateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.systemMods) {
			return
		}
		th := fyne.CurrentApp().Settings().Theme()
		variant := fyne.CurrentApp().Settings().ThemeVariant()
		mod := &app.systemMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil
		cont.Add(canvas.NewRectangle(th.Color(themes.ColorSystemTableBg, variant)))

		switch id.Col {
		case 0, 1:
			cont.Add(widget.NewLabel(""))
		case 2:
			t := widget.NewLabel("[ ]")
			t.Alignment = fyne.TextAlignCenter
			cont.Add(t)
		case 3:
			display := mod.DisplayName
			if display == "" {
				display = mod.Name
			}
			label := widget.NewLabel(display)
			label.TextStyle = fyne.TextStyle{Bold: true}
			cont.Add(label)
		case 4:
			t := canvas.NewText(app.formatDate(mod.ModTime, app.cfg.DateFormat),
				th.Color(theme.ColorNameForeground, variant))
			t.Alignment = fyne.TextAlignCenter
			cont.Add(t)
		case 5:
			cont.Add(app.buildSystemStatusLabel(mod, th, variant))
		case 6:
			label := widget.NewLabel(mod.Note)
			label.Wrapping = fyne.TextWrapWord
			cont.Add(label)
		}
	}

	app.systemModsTable = widget.NewTable(
		func() (int, int) { return len(app.systemMods), TableColumnCount },
		func() fyne.CanvasObject { return createTableRow(TableRowHeight) },
		updateCell,
	)
	ApplyTableColumnWidths(app.systemModsTable)
	app.systemModsTable.SetColumnWidth(0, 0)

	app.systemModsTable.OnSelected = func(id widget.TableCellID) {
		if id.Row < len(app.systemMods) {
			mod := &app.systemMods[id.Row]
			app.selectedModName = mod.Name
			app.selectedModIndex.Store(-1)
			app.updateDescriptionForMod(mod.Name)
			app.scheduleEnrich(mod)
			app.updateUpDownButtons()
			app.systemModsTable.Refresh()
			app.modTable.UnselectAll()
		}
	}

	// Spacer определяет высоту контейнера; пересчитывается в
	// updateSystemModsTable при изменении числа системных модов.
	app.systemModsTableSpacer = canvas.NewRectangle(color.Transparent)
	app.systemModsTableSpacer.SetMinSize(fyne.NewSize(
		1, SystemTableRowHeight*float32(len(app.systemMods))))

	container := container.NewStack(app.systemModsTableSpacer, app.systemModsTable)

	app.cfgMutex.RLock()
	showSys := app.cfg.ShowSystemMods
	app.cfgMutex.RUnlock()
	if !showSys {
		container.Hide()
	}
	app.systemModsTableContainer = container
}

// buildSystemStatusLabel собирает двухстрочный статус для системной
// таблицы (frameworks + подстатус).
func (app *App) buildSystemStatusLabel(mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	mainLabel := canvas.NewText(app.msg("status_system"), th.Color(themes.ColorStatusSystem, variant))
	mainLabel.TextSize = StatusFontSize + 2
	mainLabel.Alignment = fyne.TextAlignCenter
	mainLabel.TextStyle = fyne.TextStyle{Bold: true}

	var subText string
	var subColor color.Color
	switch {
	case mod.MissingFolder:
		subText = app.msg("status_missing_folder")
		subColor = th.Color(themes.ColorStatusMissing, variant)
	case mod.VortexDeployed:
		subText = app.msg("status_vortex")
		subColor = th.Color(themes.ColorStatusVortex, variant)
	case mod.IsSymlink:
		subText = app.msg("status_symlink")
		subColor = th.Color(themes.ColorStatusSymlink, variant)
	case mod.Source == "manual":
		subText = app.msg("status_manual")
		subColor = th.Color(themes.ColorStatusManual, variant)
	case mod.Source == "nexus":
		subText = app.msg("status_nexus")
		subColor = th.Color(themes.ColorStatusNexus, variant)
	}

	if subText == "" {
		return mainLabel
	}

	subLabel := canvas.NewText(subText, subColor)
	subLabel.TextSize = StatusFontSize
	subLabel.Alignment = fyne.TextAlignCenter

	box := container.NewWithoutLayout(mainLabel, subLabel)
	box.Layout = &VBoxWithSpacing{Spacing: StatusRowSpacing}
	return box
}

// buildModsTable создаёт основную таблицу модов. Устанавливает
// app.modTable, app.tableBorder, app.tableBorderContainer.
func (app *App) buildModsTable() {
	updateCell := func(id widget.TableCellID, cell fyne.CanvasObject) {
		if id.Row >= len(app.displayedMods) {
			return
		}
		th := fyne.CurrentApp().Settings().Theme()
		variant := fyne.CurrentApp().Settings().ThemeVariant()
		mod := &app.displayedMods[id.Row]
		cont := cell.(*fyne.Container)
		cont.Objects = nil

		bgColor := app.modRowBackgroundColor(id.Row, mod, th, variant)
		cont.Add(canvas.NewRectangle(bgColor))

		switch id.Col {
		case 0:
			cont.Objects = nil
			if !mod.IsSystem {
				cont.Add(app.buildSelectCheckboxColumn(id.Row, mod))
			} else {
				cont.Add(widget.NewLabel(""))
			}
		case 1:
			if !mod.IsSystem {
				cont.Add(app.buildActiveCheckboxColumn(id.Row, mod))
			}
		case 2:
			if mod.IsSystem {
				cont.Add(widget.NewLabel(""))
			} else {
				cont.Add(app.buildNumberColumn(id.Row, th, variant))
			}
		case 3:
			cont.Add(app.buildNameColumn(id.Row, mod))
		case 4:
			cont.Add(app.buildDateColumn(mod, th, variant))
		case 5:
			cont.Add(app.buildStatusColumn(mod, th, variant))
		case 6:
			cont.Add(app.buildNoteColumn(mod))
		}
	}

	app.modTable = widget.NewTable(
		func() (int, int) { return len(app.displayedMods), TableColumnCount },
		func() fyne.CanvasObject { return createTableRow(TableRowHeight) },
		updateCell,
	)
	ApplyTableColumnWidths(app.modTable)
	app.modTable.SetColumnWidth(0, 0)

	app.modTable.OnDoubleTapped = func(id widget.TableCellID) {
		if !app.managePanel.Visible() {
			app.managePanel.Show()
			app.showSelectColumn = true
			app.headerTable.SetColumnWidth(0, ColSelectWidth)
			app.modTable.SetColumnWidth(0, ColSelectWidth)
			app.headerTable.Refresh()
			app.modTable.Refresh()
			app.managePanel.Refresh()
			app.modTable.Select(id, 0)
		}
		app.syncSelectionToCheckboxes()
	}

	app.modTable.OnSelected = func(id widget.TableCellID) {
		app.onModRowSelected(id)
	}

	th := fyne.CurrentApp().Settings().Theme()
	variant := fyne.CurrentApp().Settings().ThemeVariant()
	app.tableBorder = canvas.NewRectangle(color.Transparent)
	app.tableBorder.StrokeWidth = 2
	app.tableBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
	app.tableBorder.FillColor = color.Transparent
	app.tableBorder.Hide()

	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	var mechBg *canvas.Image
	if mechData != nil {
		mechBg = canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechBg.FillMode = canvas.ImageFillContain
		mechBg.Translucency = TableBackgroundOpacity
		app.tableBorderContainer = container.NewStack(mechBg, app.modTable, app.tableBorder)
	} else {
		app.tableBorderContainer = container.NewStack(app.modTable, app.tableBorder)
	}
}

// modRowBackgroundColor выбирает цвет фона строки по её состоянию.
func (app *App) modRowBackgroundColor(row int, mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) color.Color {
	base := th.Color(themes.ColorTableRowEven, variant)
	if row%2 == 1 {
		base = th.Color(themes.ColorTableRowOdd, variant)
	}
	switch {
	case app.modTable != nil && app.modTable.IsRowSelected(row):
		return th.Color(themes.ColorTableRowSelected, variant)
	case row == int(app.selectedModIndex.Load()):
		return th.Color(themes.ColorTableRowSelected, variant)
	case mod.HasUpdate:
		return th.Color(themes.ColorTableHasUpdateMod, variant)
	case mod.Obsolete:
		return th.Color(themes.ColorTableObsoleteMod, variant)
	case mod.MissingFolder:
		return th.Color(themes.ColorTableMissingFolder, variant)
	case mod.Incompatible:
		return th.Color(themes.ColorTableRowConflict, variant)
	case mod.IsSymlink:
		return th.Color(themes.ColorStatusSymlinkBg, variant)
	default:
		return base
	}
}

// buildSelectCheckboxColumn — ячейка колонки 0: чекбокс выделения поверх
// опционального фонового изображения.
func (app *App) buildSelectCheckboxColumn(row int, mod *checks.ModInfo) fyne.CanvasObject {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	bgStack := []fyne.CanvasObject{}
	if app.selectColumnBgRes != nil {
		img := canvas.NewImageFromResource(app.selectColumnBgRes)
		img.FillMode = canvas.ImageFillStretch
		img.Translucency = 0.8
		bgStack = append(bgStack, img)
	} else {
		bgStack = append(bgStack, canvas.NewRectangle(th.Color(themes.ColorButtonShadow, variant)))
	}

	name := mod.Name
	check := widget.NewCheck("", nil)
	check.SetChecked(mod.Selected)
	check.OnChanged = func(b bool) {
		app.updateModSelected(name, b)
		if b {
			app.modTable.Select(widget.TableCellID{Row: row, Col: 0}, 0)
		} else if app.selectedModName == name {
			app.reassignSelectionAfterUncheck(name)
		}
		app.modTable.Refresh()
	}
	if app.showSelectColumn {
		check.Show()
	} else {
		check.Hide()
	}
	check.Refresh()
	bgStack = append(bgStack, check)
	return container.NewStack(bgStack...)
}

// reassignSelectionAfterUncheck — если сняли галку с текущего выбранного
// мода, переносим выделение на первую оставшуюся выделенную строку.
func (app *App) reassignSelectionAfterUncheck(name string) {
	newSelRow := -1
	for i, dm := range app.displayedMods {
		if dm.Selected && dm.Name != name {
			newSelRow = i
			break
		}
	}
	if newSelRow >= 0 {
		app.modTable.Select(widget.TableCellID{Row: newSelRow, Col: 0}, 0)
		return
	}
	app.modTable.UnselectAll()
	app.selectedModName = ""
	app.selectedModIndex.Store(-1)
	app.updateDescriptionForMod("")
	app.updateUpDownButtons()
}

// buildActiveCheckboxColumn — колонка 1: чекбокс активности мода.
func (app *App) buildActiveCheckboxColumn(row int, mod *checks.ModInfo) fyne.CanvasObject {
	name := mod.Name
	check := widget.NewCheck("", nil)
	check.SetChecked(mod.Active)
	if mod.MissingFolder {
		check.Disable()
	}
	check.OnChanged = func(b bool) {
		app.toggleModActive(name, b)
		app.modTable.Select(widget.TableCellID{Row: row, Col: 0}, 0)
	}
	return check
}

// buildNumberColumn — колонка 2: порядковый номер строки.
func (app *App) buildNumberColumn(row int, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	t := canvas.NewText(fmt.Sprintf("%2d", row+1), th.Color(theme.ColorNameForeground, variant))
	t.Alignment = fyne.TextAlignCenter
	return t
}

// buildNameColumn — колонка 3: имя мода.
func (app *App) buildNameColumn(row int, mod *checks.ModInfo) fyne.CanvasObject {
	display := mod.DisplayName
	if display == "" {
		display = mod.Name
	}
	label := widget.NewLabel(display)
	if row == int(app.selectedModIndex.Load()) {
		label.TextStyle = fyne.TextStyle{Bold: true}
	}
	return label
}

// buildDateColumn — колонка 4: дата установки.
func (app *App) buildDateColumn(mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	t := canvas.NewText(app.formatDate(mod.ModTime, app.cfg.DateFormat), th.Color(theme.ColorNameForeground, variant))
	t.Alignment = fyne.TextAlignCenter
	return t
}

// buildStatusColumn — колонка 5: основной и дополнительный статус.
func (app *App) buildStatusColumn(mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	mainText, mainColor, subText, subColor := app.computeStatusTexts(mod, th, variant)

	mainLabel := canvas.NewText(mainText, mainColor)
	mainLabel.TextSize = StatusFontSize + 2
	mainLabel.Alignment = fyne.TextAlignCenter
	mainLabel.TextStyle = fyne.TextStyle{Bold: true}

	if subText == "" {
		return mainLabel
	}

	subLabel := canvas.NewText(subText, subColor)
	subLabel.TextSize = StatusFontSize
	subLabel.Alignment = fyne.TextAlignCenter

	box := container.NewWithoutLayout(mainLabel, subLabel)
	box.Layout = &VBoxWithSpacing{Spacing: StatusRowSpacing}
	return box
}

// computeStatusTexts — определяет тексты и цвета для колонки статуса.
func (app *App) computeStatusTexts(mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) (string, color.Color, string, color.Color) {
	var mainText string
	var mainColor color.Color
	if mod.Active {
		mainText = app.msg("status_active")
		mainColor = th.Color(themes.ColorStatusActive, variant)
	} else {
		mainText = app.msg("status_inactive")
		mainColor = th.Color(themes.ColorStatusInactive, variant)
	}

	if mod.HasUpdate {
		return mainText, mainColor,
			app.msg("status_update_available"), th.Color(theme.ColorNamePrimary, variant)
	}

	switch {
	case mod.MissingFolder:
		return mainText, mainColor, app.msg("status_missing_folder"), th.Color(themes.ColorStatusMissing, variant)
	case mod.VortexDeployed:
		return mainText, mainColor, app.msg("status_vortex"), th.Color(themes.ColorStatusVortex, variant)
	case mod.IsSymlink:
		return mainText, mainColor, app.msg("status_symlink"), th.Color(themes.ColorStatusSymlink, variant)
	case mod.IsSystem:
		return mainText, mainColor, app.msg("status_system"), th.Color(themes.ColorStatusSystem, variant)
	case mod.Broken:
		return mainText, mainColor, app.msg("desc_broken"), th.Color(themes.ColorStatusBroken, variant)
	case mod.Incompatible:
		return mainText, mainColor, app.msg("desc_conflict"), th.Color(themes.ColorStatusConflict, variant)
	case mod.Obsolete:
		return mainText, mainColor, app.msg("desc_obsolete"), th.Color(themes.ColorStatusObsolete, variant)
	case mod.Mandatory && mod.Active:
		return mainText, mainColor, app.msg("status_mandatory"), th.Color(themes.ColorStatusMandatory, variant)
	case mod.Source == "manual":
		return mainText, mainColor, app.msg("status_manual"), th.Color(themes.ColorStatusManual, variant)
	case mod.Source == "nexus":
		return mainText, mainColor, app.msg("status_nexus"), th.Color(themes.ColorStatusNexus, variant)
	default:
		return mainText, mainColor, "", color.Transparent
	}
}

// buildNoteColumn — колонка 6: примечание в горизонтальном скролле.
func (app *App) buildNoteColumn(mod *checks.ModInfo) fyne.CanvasObject {
	label := widget.NewLabel(mod.Note)
	label.Wrapping = fyne.TextWrapOff
	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(0, 35))
	return scroll
}

// onModRowSelected — логика OnSelected основной таблицы.
func (app *App) onModRowSelected(id widget.TableCellID) {
	if app.suppressSelectionEvents {
		return
	}
	if id.Row >= len(app.displayedMods) {
		return
	}
	if !app.modTable.IsRowSelected(id.Row) {
		app.selectedModIndex.Store(-1)
		app.selectedModName = ""
		app.updateDescriptionForMod("")
		app.updateUpDownButtons()
		app.modTable.Refresh()
		app.syncSelectionToCheckboxes()
		return
	}

	app.syncSelectionToCheckboxes()

	selectedCount := 0
	for i := 0; i < len(app.displayedMods); i++ {
		if app.modTable.IsRowSelected(i) {
			selectedCount++
		}
	}
	if selectedCount > 1 && !app.managePanel.Visible() {
		app.managePanel.Show()
		app.showSelectColumn = true
		app.headerTable.SetColumnWidth(0, ColSelectWidth)
		app.modTable.SetColumnWidth(0, ColSelectWidth)
		app.headerTable.Refresh()
		app.modTable.Refresh()
		app.managePanel.Refresh()
	}

	app.selectedModName = app.displayedMods[id.Row].Name
	app.selectedModIndex.Store(int32(id.Row))
	app.updateDescriptionForMod(app.selectedModName)
	app.scheduleEnrich(&app.displayedMods[id.Row])
	app.updateUpDownButtons()
	app.modTable.Refresh()
}

// ─────────────────────────────────────────────────────────────────
// Нижняя панель
// ─────────────────────────────────────────────────────────────────

// buildBottomPanel создаёт нижнюю панель со счётчиком модов и
// выпадающим списком профилей. Устанавливает app.counterLabel,
// app.profileLabel, app.profileSelect.
func (app *App) buildBottomPanel() fyne.CanvasObject {
	app.counterLabel = widget.NewLabel("")
	app.profileLabel = widget.NewLabel(app.msg("profile_label"))
	app.profileLabel.TextStyle = fyne.TextStyle{Bold: true}

	app.profileSelect = widget.NewSelect([]string{}, func(s string) {
		if s != app.cfg.ActiveProfile {
			app.switchProfile(s)
		}
	})
	app.profileSelect.PlaceHolder = app.msg("profile_select_placeholder")

	content := container.NewHBox(
		app.counterLabel,
		layout.NewSpacer(),
		app.profileLabel,
		app.profileSelect,
	)
	return container.NewBorder(nil, nil, nil, nil, content)
}

// ─────────────────────────────────────────────────────────────────
// Карточка описания
// ─────────────────────────────────────────────────────────────────

// buildDescriptionCard собирает правую верхнюю карточку с описанием мода.
// Устанавливает поля descTitle, descAuthor, descBody, descURL, githubLink,
// descConflict, descCardBgRect, descExtraContainer, descCardContent,
// descCardScroll.
//
// Возвращает готовый rightContent (карточка + консоль) для сборки split'а.
func (app *App) buildDescriptionCard() fyne.CanvasObject {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	app.descTitle = canvas.NewText(app.msg("select_mod"), th.Color(theme.ColorNameForeground, variant))
	app.descTitle.TextSize = theme.TextSize() + 2
	app.descTitle.TextStyle = fyne.TextStyle{Bold: true}

	app.descAuthor = widget.NewLabel("-")
	app.descInstalled = widget.NewLabel("")
	app.descBody = widget.NewLabel(app.msg("desc_placeholder"))
	app.descBody.Wrapping = fyne.TextWrapWord
	app.descURL = widget.NewHyperlink("", nil)
	app.githubLink = widget.NewHyperlink("", nil)
	app.githubLink.Alignment = fyne.TextAlignLeading

	app.descLocalVersion = widget.NewLabel("")
	app.descLatestVersion = widget.NewLabel("")
	app.descLastUpdated = widget.NewLabel("")
	app.descOriginalUpload = widget.NewLabel("")
	app.descConflict = widget.NewLabel("")
	app.descConflict.Wrapping = fyne.TextWrapWord
	app.descConflict.Hide()

	app.descCardBgRect = canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	app.descCardBgRect.CornerRadius = 12
	app.descCardBgRect.StrokeWidth = 0.5
	app.descCardBgRect.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)

	app.descExtraContainer = container.NewVBox()

	leftPadding := canvas.NewRectangle(color.Transparent)
	leftPadding.SetMinSize(fyne.NewSize(30, 1))

	headerRow := container.NewHBox(
		leftPadding,
		app.descTitle,
		layout.NewSpacer(),
		widget.NewSeparator(),
		app.btnUpdateMod,
		app.openFolderBtn,
		app.btnRemove,
	)

	titleSep := canvas.NewRectangle(th.Color(themes.ColorCRTScreenStroke, variant))
	titleSep.SetMinSize(fyne.NewSize(0, 3))

	descHeader := container.NewBorder(
		nil, nil, nil, nil,
		container.NewVBox(
			headerRow,
			titleSep,
			app.descAuthor,
			widget.NewSeparator(),
			container.NewHBox(widget.NewLabel(""), app.descURL, widget.NewLabel("  "), app.githubLink),
			widget.NewSeparator(),
			container.NewHBox(widget.NewLabel(""), app.descLocalVersion),
			widget.NewSeparator(),
			app.descConflict,
		),
	)

	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, 20))

	app.descCardContent = container.NewVBox(
		descHeader,
		widget.NewSeparator(),
		app.descBody,
		spacer,
		app.descExtraContainer,
	)

	app.descCardScroll = container.NewScroll(app.descCardContent)
	app.descCardScroll.SetMinSize(fyne.NewSize(DescScrollMinWidth, DescScrollMinHeight))

	descCard := container.NewStack(
		app.descCardBgRect,
		container.NewPadded(app.descCardScroll),
	)

	rightContent := container.NewVSplit(descCard, app.consoleScroll)
	rightContent.Offset = 0.65
	return container.NewBorder(nil, nil, nil, nil, rightContent)
}

// ─────────────────────────────────────────────────────────────────
// Финальная сборка
// ─────────────────────────────────────────────────────────────────

// assembleMainLayout собирает все панели в финальный layout и
// устанавливает его в главное окно.
func (app *App) assembleMainLayout(topPanelWithBg, bottomPanel, rightContent fyne.CanvasObject) {
	modsArea := container.NewBorder(
		container.NewVBox(
			topPanelWithBg,
			app.managePanel,
			app.headerTable,
		),
		nil, nil, nil,
		container.NewBorder(
			container.NewVBox(app.systemModsTableContainer),
			nil, nil, nil,
			app.tableBorderContainer,
		),
	)

	leftPanel := container.NewBorder(
		nil,
		bottomPanel,
		nil, nil,
		modsArea,
	)

	split := container.NewHSplit(leftPanel, rightContent)
	split.Offset = SplitOffset
	content := container.NewBorder(nil, nil, nil, nil, split)
	app.mainWindow.SetContent(content)
}

func (app *App) refreshThemeColors() {
	th := app.myApp.Settings().Theme()
	variant := app.myApp.Settings().ThemeVariant()

	if app.screenBgRect != nil {
		app.screenBgRect.FillColor = th.Color(themes.ColorCRTScreenFill, variant)
		app.screenBgRect.StrokeColor = th.Color(themes.ColorCRTScreenStroke, variant)
		app.screenBgRect.Refresh()
	}
	if app.headerBoxBgRect != nil {
		app.headerBoxBgRect.FillColor = th.Color(themes.ColorCRTHeaderBg, variant)
		app.headerBoxBgRect.Refresh()
	}
	if app.logHeaderText != nil {
		app.logHeaderText.Color = th.Color(themes.ColorConsoleText, variant)
		app.logHeaderText.Refresh()
	}
	if app.tipBgRect != nil {
		app.tipBgRect.FillColor = th.Color(themes.ColorTipBg, variant)
		app.tipBgRect.Refresh()
	}
	if app.topPanelBgRect != nil {
		app.topPanelBgRect.FillColor = th.Color(themes.ColorTopPanelBg, variant)
		app.topPanelBgRect.Refresh()
	}
	if app.managePanelBgRect != nil {
		app.managePanelBgRect.FillColor = th.Color(themes.ColorManagePanelBg, variant)
		app.managePanelBgRect.Refresh()
	}
	if app.descCardBgRect != nil {
		app.descCardBgRect.FillColor = th.Color(themes.ColorDescCardBg, variant)
		app.descCardBgRect.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)
		app.descCardBgRect.Refresh()
	}
	if app.tableBorder != nil {
		app.tableBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
		app.tableBorder.Refresh()
	}
	if app.descTitle != nil {
		app.descTitle.Color = th.Color(theme.ColorNameForeground, variant)
		app.descTitle.Refresh()
	}
	if app.logWindow != nil {
		app.logWindow.Refresh()
	}
	if app.consoleScroll != nil {
		app.consoleScroll.Refresh()
	}
	if app.descCardScroll != nil {
		app.descCardScroll.Refresh()
	}
	if app.descCardContent != nil {
		app.descCardContent.Refresh()
	}
	if app.descExtraContainer != nil {
		app.descExtraContainer.Refresh()
	}

	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.systemModsTable != nil {
		app.systemModsTable.Refresh()
	}
	if app.modTable != nil {
		app.modTable.Refresh()
	}

	for _, tbl := range []*widget.Table{app.headerTable, app.systemModsTable, app.modTable} {
		if tbl == nil {
			continue
		}
		tbl.SetColumnWidth(0, 1)
		tbl.SetColumnWidth(0, 0)
		tbl.Refresh()
	}

	app.updateTooltipStyle()

	for _, btn := range []*CustomButton{
		app.btnSaveOrder, app.btnRefresh, app.btnInstall, app.btnRemove,
		app.btnUp, app.btnDown, app.btnSortChecks, app.btnToggle,
		app.btnLaunchNormal, app.btnLaunchNoLauncher,
		app.moveToTopBtn, app.moveToBottomBtn, app.btnAMLConfig,
		app.selectAllBtn, app.deselectAllBtn, app.enableSelectedBtn,
		app.disableSelectedBtn, app.enableAllBtn, app.disableAllBtn, app.btnEditVersion,
		app.manageBtn, app.searchClearBtn, app.btnRemoveAll, app.btnRemoveSelected,
	} {
		if btn != nil {
			btn.Refresh()
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// Логирование
// ─────────────────────────────────────────────────────────────────

// maxLogSegments — верхняя граница числа сегментов в лог-виджете.
// Без неё Segments растёт монотонно за всё время работы приложения
// (а appendLog вызывается в т.ч. на каждую операцию с модом), и через
// несколько часов работы UI начинает тормозить из-за отрисовки
// огромного RichText. (#20)
const maxLogSegments = 500

func (app *App) appendLog(text string) {
	if app.logWindow == nil {
		if app.logFile != nil {
			fmt.Fprintln(app.logFile, time.Now().Format(LogTimeFormat), text)
		}
		return
	}
	fyne.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				if app.logFile != nil {
					fmt.Fprintf(app.logFile, "PANIC in appendLog: %v\n", r)
				}
			}
		}()
		seg := &widget.TextSegment{
			Style: widget.RichTextStyle{
				ColorName: themes.ColorConsoleText,
				TextStyle: fyne.TextStyle{},
			},
			Text: text,
		}
		app.logWindow.Segments = append(app.logWindow.Segments, seg)
		// (#20) Обрезаем буфер. Копирование слайса дешевле, чем
		// перерисовка тысяч сегментов; делаем это батчами — только
		// когда превысили лимит, оставляя ровно maxLogSegments.
		if len(app.logWindow.Segments) > maxLogSegments {
			tail := make([]widget.RichTextSegment, maxLogSegments)
			copy(tail, app.logWindow.Segments[len(app.logWindow.Segments)-maxLogSegments:])
			app.logWindow.Segments = tail
		}
		app.logWindow.Refresh()
		if app.consoleScroll != nil {
			app.consoleScroll.ScrollToBottom()
		}
	})
	if app.logFile != nil {
		fmt.Fprintln(app.logFile, time.Now().Format(LogTimeFormat), text)
	}
}

func (app *App) appendCenteredLog(text string) {
	fyne.Do(func() {
		if app.logHeaderText != nil {
			app.logHeaderText.Text = text
			app.logHeaderText.Refresh()
		}
	})
}

func (app *App) updateDescriptionForMod(name string) {
	if name == "" {
		app.descTitle.Text = app.msg("select_mod")
		app.descTitle.Refresh()
		app.descAuthor.SetText("-")
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
		app.descBody.SetText(app.msg("desc_placeholder"))
		if app.descExtraContainer != nil {
			app.descExtraContainer.Objects = nil
			app.descExtraContainer.Refresh()
		}
		if app.descCardContent != nil {
			app.descCardContent.Refresh()
		}
		if app.descCardScroll != nil {
			app.descCardScroll.Refresh()
		}
		return
	}

	mod, ok := app.findModByName(name)
	if !ok {
		return
	}

	if app.openFolderBtn != nil {
		if mod.MissingFolder || mod.Name == "" {
			app.openFolderBtn.Disable()
		} else {
			app.openFolderBtn.Enable()
		}
	}

	display := mod.DisplayName
	if display == "" {
		display = mod.Name
	}
	app.descTitle.Text = display
	app.descTitle.Refresh()

	author := mod.Author
	if author == "" {
		author = app.msg("author_unknown")
	}
	authorText := fmt.Sprintf(app.msg("author_label"), author)
	if !mod.OriginalUpload.IsZero() {
		authorText += fmt.Sprintf("          %s: %s", app.msg("original_upload_label"), app.formatDate(mod.OriginalUpload, app.cfg.DateFormat))
	}
	app.descAuthor.SetText(authorText)

	app.descInstalled.SetText(fmt.Sprintf(app.msg("installed_label"), app.formatDate(mod.ModTime, app.cfg.DateFormat)))

	if app.descLocalVersion != nil {
		var cacheKey string
		switch mod.Name {
		case "dmf":
			cacheKey = "8:dmf"
		case "base":
			cacheKey = "19:base"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if mod.URL != "" {
				modID := helpers.ExtractModIDFromURL(mod.URL)
				if modID != 0 {
					cacheKey = fmt.Sprintf("%d:%s", modID, mod.Name)
				}
			}
		}

		if cacheKey != "" {
			if info, ok := app.getCachedVersion(cacheKey); ok && info.Version != "" {
				localText := fmt.Sprintf(app.msg("nexus_local_version_label"), info.Version)

				if latest, ok := app.getLatestVersion(cacheKey); ok {
					localText += "          " + fmt.Sprintf(app.msg("nexus_latest_version_label"), latest)
				}

				if !mod.LastUpdated.IsZero() {
					localText += fmt.Sprintf("          %s: %s", app.msg("last_updated_label"), app.formatDate(mod.LastUpdated, app.cfg.DateFormat))
				}

				app.descLocalVersion.SetText(localText)
			} else {
				app.descLocalVersion.SetText(app.msg("nexus_local_version_unknown"))
			}
		} else {
			app.descLocalVersion.SetText("")
		}
	}

	if app.descLastUpdated != nil {
		if !mod.LastUpdated.IsZero() {
			app.descLastUpdated.SetText(fmt.Sprintf("Last updated: %s", app.formatDate(mod.LastUpdated, app.cfg.DateFormat)))
		} else {
			app.descLastUpdated.SetText("")
		}
	}

	if app.descOriginalUpload != nil {
		if !mod.OriginalUpload.IsZero() {
			app.descOriginalUpload.SetText(fmt.Sprintf("Original upload: %s", app.formatDate(mod.OriginalUpload, app.cfg.DateFormat)))
		} else {
			app.descOriginalUpload.SetText("")
		}
	}

	if app.descLatestVersion != nil {
		var cacheKey string
		switch mod.Name {
		case "dmf":
			cacheKey = "8:dmf"
		case "base":
			cacheKey = "19:base"
		case "autopatch":
			cacheKey = "709:autopatch"
		default:
			if mod.URL != "" {
				modID := helpers.ExtractModIDFromURL(mod.URL)
				if modID != 0 {
					cacheKey = fmt.Sprintf("%d:%s", modID, mod.Name)
				}
			}
		}

		if cacheKey != "" {
			if latest, ok := app.getLatestVersion(cacheKey); ok {
				app.descLatestVersion.SetText(fmt.Sprintf(app.msg("nexus_latest_version_label"), latest))
			} else {
				app.descLatestVersion.SetText(app.msg("nexus_latest_version_unknown"))
			}
		} else {
			app.descLatestVersion.SetText("")
		}
	}

	desc := strings.TrimSpace(mod.Description)
	if mod.MissingFolder {
		desc = app.msg("desc_missing") + desc
	}
	if desc == "" || desc == "{" || desc == "}" || desc == "[]" || desc == "()" {
		desc = app.msg("desc_placeholder")
	}
	app.descBody.SetText(desc)

	if mod.URL != "" {
		if u, err := url.Parse(mod.URL); err == nil {
			app.descURL.SetURL(u)
			app.descURL.SetText(app.msg("mod_url_label"))
		} else {
			app.descURL.SetURL(nil)
			app.descURL.SetText("")
		}
	} else {
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
	}

	if app.githubLink != nil {
		if mod.GitHubURL != "" {
			if u, err := url.Parse(mod.GitHubURL); err == nil {
				app.githubLink.SetURL(u)
				app.githubLink.SetText(app.msg("source_code_url"))
			} else {
				app.githubLink.SetURL(nil)
				app.githubLink.SetText("")
			}
		} else {
			app.githubLink.SetURL(nil)
			app.githubLink.SetText("")
		}
	}

	if mod.Incompatible {
		// Копия списка под checksDataMutex — без прямого чтения
		// глобального слайса, который может быть перезаписан в
		// checks.LoadExternalLists.
		for _, pair := range checks.GetIncompatiblePairs() {
			if pair.Mod1 == mod.Name || pair.Mod2 == mod.Name {
				other := pair.Mod1
				if other == mod.Name {
					other = pair.Mod2
				}
				if !checks.FolderExists(other) {
					continue
				}
				if desc := checks.GetIncompatibleDesc(pair.Mod1, pair.Mod2); desc != "" {
					app.descConflict.SetText(desc)
				} else {
					app.descConflict.SetText("")
				}
				app.descConflict.Show()
				break
			}
		}
	} else {
		app.descConflict.Hide()
		app.descConflict.SetText("")
	}

	if app.descExtraContainer != nil {
		app.descExtraContainer.Objects = nil
		if mod.URL != "" {
			modID := helpers.ExtractModIDFromURL(mod.URL)
			if modID != 0 {
				key := fmt.Sprintf("%d:%s", modID, mod.Name)
				savedText, hasText := app.changelog.Text(key)
				expanded, hasExpanded := app.changelog.Expanded(key)

				changelogLabel := widget.NewLabel(app.msg("downloading_changelog"))
				changelogLabel.Wrapping = fyne.TextWrapWord
				changelogContainer := container.NewVBox(changelogLabel)
				changelogContainer.Hide()

				if hasText && savedText != "" && savedText != app.msg("downloading_changelog") {
					changelogLabel.SetText(savedText)
				}
				if hasExpanded && expanded {
					changelogContainer.Show()
				}

				btnState := struct {
					expanded bool
					btn      *widget.Button
				}{}
				btnState.btn = widget.NewButton(app.msg("btn_show_changelog"), func() {
					if !btnState.expanded {
						if changelogLabel.Text == app.msg("downloading_changelog") {
							go func() {
								fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
								if err != nil {
									fyne.Do(func() {
										changelogLabel.SetText(app.msg("changelog_load_failed"))
									})
									return
								}
								changelog, err := app.FetchChangelog(modID, fileInfo.ID)
								clean := app.msg("changelog_unavailable")
								if err == nil && changelog != "" {
									clean = stripHTML(changelog)
								}
								fyne.Do(func() {
									app.changelog.SetText(key, clean)
									changelogLabel.SetText(clean)
								})
							}()
						}
						btnState.expanded = true
						changelogContainer.Show()
						btnState.btn.SetText(app.msg("btn_hide_changelog"))
						app.changelog.SetExpanded(key, true)
					} else {
						btnState.expanded = false
						changelogContainer.Hide()
						btnState.btn.SetText(app.msg("btn_show_changelog"))
						app.changelog.SetExpanded(key, false)
					}
				})

				if hasExpanded && expanded {
					btnState.btn.SetText(app.msg("btn_hide_changelog"))
					btnState.expanded = true
				}

				app.descExtraContainer.Objects = []fyne.CanvasObject{
					widget.NewSeparator(),
					btnState.btn,
					changelogContainer,
				}
				app.descExtraContainer.Refresh()
			}
		} else {
			app.descExtraContainer.Refresh()
		}
	}

	if app.descCardContent != nil {
		app.descCardContent.Refresh()
	}
	if app.descCardScroll != nil {
		app.descCardScroll.Refresh()
	}
}

// enrichModFromNexus асинхронно подтягивает метаданные с Nexus.
//
// (#4) Раньше функция писала mod.LastUpdated / mod.OriginalUpload прямо
// в переданный указатель из горутины — это была гонка с UI-потоком,
// который читал эти же поля для отрисовки. Теперь значения собираются
// в локальные переменные и переносятся в allMods под modsMutex.Lock.
//
// Параметр mod — это снимок ModInfo (не указатель), поэтому поля можно
// безопасно читать из горутины.
func (app *App) enrichModFromNexus(mod *checks.ModInfo) {
	if app.getAuthToken() == "" || mod.URL == "" {
		return
	}
	modID := helpers.ExtractModIDFromURL(mod.URL)
	if modID == 0 {
		return
	}

	// Снимок имени — mod может быть указателем в displayedMods, которую
	// перестроит UI. Работаем со строкой, а не с полем.
	modName := mod.Name

	go func() {
		defer func() { recover() }()
		var fileInfo *FileInfo
		var err error
		if modName == "base" || modName == "dmf" {
			fileInfo, err = app.getLatestFileInfo(modID)
		} else {
			fileInfo, err = app.getLatestFileInfoForMod(modID, modName)
		}
		if err != nil {
			app.appendLog(fmt.Sprintf(app.msg("log_cannot_get_file_info"), modName, err))
			return
		}
		cacheKey := fmt.Sprintf("%d:%s", modID, modName)
		app.setLatestVersion(cacheKey, fileInfo.Version)

		// Локальные переменные — не пишем в mod (это поле displayedMods).
		var newLastUpdated time.Time
		var newOriginalUpload time.Time

		if fileInfo != nil && fileInfo.UploadedTimestamp > 0 {
			newLastUpdated = time.Unix(fileInfo.UploadedTimestamp, 0)
		}

		oldestInfo, err := app.getOldestFileInfo(modID)
		if err == nil && oldestInfo != nil && oldestInfo.UploadedTimestamp > 0 {
			newOriginalUpload = time.Unix(oldestInfo.UploadedTimestamp, 0)
		}

		if fileInfo.FileName != "" {
			entry := checks.GetModDBEntry(modName)
			if entry != nil && entry.NexusFilePattern == "" {
				pattern := extractPatternFromFilename(fileInfo.FileName)
				if pattern != "" {
					entry.NexusFilePattern = pattern
					checks.UpdateModDBEntry(*entry)
					checks.SaveModDatabase()
					app.appendLog(fmt.Sprintf(app.msg("log_autosaved_stable_pattern"), modName, pattern))
				}
			}
		}

		fyne.Do(func() {
			// (#4) Запись в allMods — под modsMutex.
			app.modsMutex.Lock()
			for i := range app.allMods {
				if app.allMods[i].Name == modName {
					app.allMods[i].LastUpdated = newLastUpdated
					app.allMods[i].OriginalUpload = newOriginalUpload
					break
				}
			}
			app.modsMutex.Unlock()

			if app.selectedModName == modName {
				app.updateDescriptionForMod(modName)
			}
		})
	}()
}

func (app *App) updateToggleButtonText(btn *CustomButton) {
	gameRoot, patcher := app.getGameState()
	switch patcher {
	case PatcherAutoPatch:
		if isModsEnabledAutoPatch(gameRoot) {
			btn.icon = app.toggleOnIcon
		} else {
			btn.icon = app.toggleOffIcon
		}
	case PatcherLegacy:
		if isModsEnabledLegacy(gameRoot) {
			btn.icon = app.toggleOnIcon
		} else {
			btn.icon = app.toggleOffIcon
		}
	default:
		btn.icon = app.toggleOffIcon
		btn.Disable()
		return
	}
	btn.text = ""
	btn.Enable()
	btn.Refresh()
}

func (app *App) updateUpDownButtons() {
	if app.selectedModName == "" {
		app.btnUp.Disable()
		app.btnDown.Disable()
		app.btnUp.Refresh()
		app.btnDown.Refresh()
		return
	}
	if mod, ok := app.findModByName(app.selectedModName); ok && mod.IsSystem {
		app.btnUp.Disable()
		app.btnDown.Disable()
		app.moveToTopBtn.Disable()
		app.moveToBottomBtn.Disable()
		app.btnUp.Refresh()
		app.btnDown.Refresh()
		app.moveToTopBtn.Refresh()
		app.moveToBottomBtn.Refresh()
		return
	}
	idx := -1
	for i, m := range app.displayedMods {
		if m.Name == app.selectedModName {
			idx = i
			break
		}
	}
	app.selectedModIndex.Store(int32(idx))
	if idx < 0 {
		app.btnUp.Disable()
		app.btnDown.Disable()
	} else {
		app.btnUp.Enable()
		app.btnDown.Enable()
		if idx == 0 {
			app.btnUp.Disable()
		}
		if idx == len(app.displayedMods)-1 {
			app.btnDown.Disable()
		}
	}
	app.btnUp.Refresh()
	app.btnDown.Refresh()
}

type modFilterFunc func(checks.ModInfo) bool

// filterModList строит app.displayedMods из app.allMods согласно
// текущему фильтру и поиску.
//
// (#4) Раньше функция читала app.allMods без modsMutex — это была гонка
// с фоновыми горутинами (removeSelectedMods → removeModFromData, которая
// держит Lock). Теперь весь блок построения нового списка идёт под
// modsMutex.Lock — UI-обновления (Refresh, Scroll) вынесены за скобки.
func (app *App) filterModList() {
	if app.modTable == nil {
		app.appendLogToFile("filterModList: modTable is nil, skipping")
		return
	}

	if app.filterSelect == nil {
		app.appendLogToFile("filterModList: filterSelect is nil, using all mods")
		app.modsMutex.Lock()
		app.displayedMods = make([]checks.ModInfo, len(app.allMods))
		copy(app.displayedMods, app.allMods)
		app.modsMutex.Unlock()

		app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
		if app.selectedModName != "" {
			for i, m := range app.displayedMods {
				if m.Name == app.selectedModName {
					app.selectedModIndex.Store(int32(i))
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
					break
				}
			}
		} else {
			app.selectedModIndex.Store(-1)
		}
		app.modTable.Refresh()
		activeCount := 0
		for _, m := range app.displayedMods {
			if m.Active {
				activeCount++
			}
		}
		if app.counterLabel != nil {
			app.counterLabel.SetText(fmt.Sprintf(app.msg("mods_counter"), len(app.displayedMods), len(app.allMods), activeCount))
		}
		app.forceRefreshTable()
		return
	}

	predicates := map[string]modFilterFunc{
		app.msg("filter_all"):        func(m checks.ModInfo) bool { return true },
		app.msg("filter_active"):     func(m checks.ModInfo) bool { return m.Active },
		app.msg("filter_inactive"):   func(m checks.ModInfo) bool { return !m.Active },
		app.msg("filter_obsolete"):   func(m checks.ModInfo) bool { return m.Obsolete },
		app.msg("filter_conflict"):   func(m checks.ModInfo) bool { return m.Incompatible },
		app.msg("filter_missing"):    func(m checks.ModInfo) bool { return m.MissingFolder },
		app.msg("filter_has_update"): func(m checks.ModInfo) bool { return m.HasUpdate },
	}
	filter := app.filterSelect.Selected
	if filter == "" {
		filter = app.msg("filter_all")
	}
	filterFn, ok := predicates[filter]
	if !ok {
		filterFn = predicates[app.msg("filter_all")]
	}
	search := strings.ToLower(app.searchEntry.Text)

	// (#4) Построение displayedMods — под modsMutex.Lock, потому что
	// allMods может писаться из фоновой горутины.
	app.modsMutex.Lock()
	newDisplayed := make([]checks.ModInfo, 0, len(app.allMods))
	for _, mod := range app.allMods {
		if search != "" {
			dn := strings.ToLower(mod.DisplayName)
			if !strings.Contains(strings.ToLower(mod.Name), search) && !strings.Contains(dn, search) {
				continue
			}
		}
		if filterFn(mod) {
			newDisplayed = append(newDisplayed, mod)
		}
	}
	app.displayedMods = newDisplayed
	totalCount := len(app.allMods)
	app.modsMutex.Unlock()

	app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
	app.modTable.Refresh()
	selIdx := app.selectedModIndex.Load()
	if selIdx >= 0 {
		app.modTable.ScrollTo(widget.TableCellID{Row: int(selIdx), Col: 0})
	} else {
		app.modTable.ScrollToTop()
	}
	app.updateUpDownButtons()
	activeCount := 0
	for _, m := range app.displayedMods {
		if m.Active {
			activeCount++
		}
	}
	if app.counterLabel != nil {
		app.counterLabel.SetText(fmt.Sprintf(app.msg("mods_counter"), len(app.displayedMods), totalCount, activeCount))
	}
	app.forceRefreshTable()
}

func (app *App) filterOptions() []string {
	return []string{
		app.msg("filter_all"),
		app.msg("filter_active"),
		app.msg("filter_inactive"),
		app.msg("filter_obsolete"),
		app.msg("filter_conflict"),
		app.msg("filter_missing"),
		app.msg("filter_has_update"),
	}
}

func (app *App) selectAllMods(selected bool) {
	app.modsMutex.Lock()
	visibleNames := make(map[string]bool)
	for _, mod := range app.displayedMods {
		visibleNames[mod.Name] = true
	}
	for i := range app.allMods {
		if visibleNames[app.allMods[i].Name] {
			app.allMods[i].Selected = selected
		}
	}
	for i := range app.displayedMods {
		if visibleNames[app.displayedMods[i].Name] {
			app.displayedMods[i].Selected = selected
		}
	}
	app.modsMutex.Unlock()

	fyne.Do(func() {
		app.filterModList()
	})
}

// updateModSelected обновляет Selected в allMods и displayedMods по имени.
// Безопасно вызывать из UI-потока (внутри держит modsMutex).
func (app *App) updateModSelected(name string, selected bool) {
	app.modsMutex.Lock()
	defer app.modsMutex.Unlock()
	for i := range app.allMods {
		if app.allMods[i].Name == name {
			app.allMods[i].Selected = selected
			break
		}
	}
	for i := range app.displayedMods {
		if app.displayedMods[i].Name == name {
			app.displayedMods[i].Selected = selected
			break
		}
	}
}

func (app *App) setSelectedActive(active bool) {
	app.modsMutex.Lock()
	changed := false
	for i := range app.allMods {
		if app.allMods[i].Selected && !app.allMods[i].IsSystem {
			if app.allMods[i].Active != active {
				app.allMods[i].Active = active
				changed = true
			}
		}
	}
	for i := range app.displayedMods {
		if app.displayedMods[i].Selected && !app.displayedMods[i].IsSystem {
			if app.displayedMods[i].Active != active {
				app.displayedMods[i].Active = active
			}
		}
	}
	app.modsMutex.Unlock()
	if changed {
		app.orderDirty = true
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
			app.forceRefreshTable()
		})
	}
}

func (app *App) setAllModsActive(active bool) {
	app.modsMutex.Lock()
	changed := false
	for i := range app.allMods {
		if !app.allMods[i].IsSystem {
			if app.allMods[i].Active != active {
				app.allMods[i].Active = active
				changed = true
			}
		}
	}
	for i := range app.displayedMods {
		if !app.displayedMods[i].IsSystem {
			app.displayedMods[i].Active = active
		}
	}
	app.modsMutex.Unlock()
	if changed {
		app.orderDirty = true
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
		})
	}
}

func (app *App) startBlink(btn *CustomButton, activeFlag *bool, condition func() bool) {
	if *activeFlag {
		return
	}
	*activeFlag = true
	go func() {
		for *activeFlag && condition() {
			fyne.Do(func() {
				btn.Importance = widget.WarningImportance
				btn.Refresh()
			})
			time.Sleep(BlinkOnDuration)
			fyne.Do(func() {
				btn.Importance = widget.MediumImportance
				btn.Refresh()
			})
			time.Sleep(BlinkOffDuration)
		}
		fyne.Do(func() {
			btn.Importance = widget.MediumImportance
			btn.Refresh()
		})
	}()
}

func (app *App) startBlinkSaveButton() {
	app.startBlink(app.btnSaveOrder, &app.blinkSaveOrderActive, func() bool {
		return app.orderDirty
	})
}

func (app *App) stopBlinkSaveButton() {
	app.blinkSaveOrderActive = false
}

func (app *App) updateTableBorder() {
	if app.tableBorder == nil {
		return
	}
	if app.orderDirty {
		app.tableBorder.Show()
		if !app.blinkSaveOrderActive {
			app.startBlinkSaveButton()
		}
	} else {
		app.tableBorder.Hide()
		app.stopBlinkSaveButton()
	}
}

func (app *App) scheduleEnrich(mod *checks.ModInfo) {
	if app.enrichDebounce != nil {
		app.enrichDebounce.Stop()
	}
	modCopy := *mod
	app.enrichDebounce = time.AfterFunc(1500*time.Millisecond, func() {
		app.enrichModFromNexus(&modCopy)
	})
}

func (app *App) selectAndScrollToMod(modName string) {
	if modName == "" {
		return
	}
	time.AfterFunc(50*time.Millisecond, func() {
		fyne.Do(func() {
			for i, m := range app.displayedMods {
				if m.Name == modName {
					app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
					app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
					return
				}
			}
		})
	})
}

// syncSelectionToCheckboxes переносит выделение строк таблицы в поля
// Selected моделей.
//
// (#4) Было две ошибки:
//  1. Индексы. Таблица построена по displayedMods, а синхронизация
//     шла по allMods[i] — при активном фильтре/поиске индексы не
//     совпадали, и флаг Selected улетал не тому моду.
//  2. Мьютекс. allMods защищён modsMutex, а функция писала в него
//     под Lock (это правильно), но читала displayedMods по позиции
//     — а displayedMods перестраивается filterModList'ом из UI-потока,
//     и чтение длины без Lock могло разъехаться с реальным размером.
//
// Теперь: снимок выделения берём по displayedMods, а обновляем
// allMods через map name→index, всё под modsMutex.
func (app *App) syncSelectionToCheckboxes() {
	if app.modTable == nil {
		return
	}

	app.modsMutex.RLock()
	n := len(app.displayedMods)
	app.modsMutex.RUnlock()

	selections := make([]bool, n)
	for i := 0; i < n; i++ {
		selections[i] = app.modTable.IsRowSelected(i)
	}

	app.modsMutex.Lock()
	if len(app.displayedMods) != n {
		app.modsMutex.Unlock()
		return
	}
	nameToAllIdx := make(map[string]int, len(app.allMods))
	for j := range app.allMods {
		nameToAllIdx[app.allMods[j].Name] = j
	}
	for i := 0; i < n; i++ {
		if app.displayedMods[i].Selected != selections[i] {
			app.displayedMods[i].Selected = selections[i]
		}
		if j, ok := nameToAllIdx[app.displayedMods[i].Name]; ok {
			app.allMods[j].Selected = selections[i]
		}
	}
	app.modsMutex.Unlock()

	rows, cols := app.modTable.Length()
	app.modTable.Length = func() (int, int) { return rows + 1, cols }
	app.modTable.Refresh()
	app.modTable.Length = func() (int, int) { return rows, cols }
	app.modTable.Refresh()
}

func (app *App) applySelectionFromMods() {
	if app.modTable == nil {
		return
	}
	app.suppressSelectionEvents = true
	defer func() { app.suppressSelectionEvents = false }()

	app.modTable.ClearSelection()

	var selectedNames []string
	for _, m := range app.displayedMods {
		if m.Selected {
			selectedNames = append(selectedNames, m.Name)
		}
	}

	if len(selectedNames) == 0 {
		app.selectedModName = ""
		app.selectedModIndex.Store(-1)
		app.updateDescriptionForMod("")
		app.updateUpDownButtons()
		app.modTable.Refresh()
		return
	}

	for i, m := range app.displayedMods {
		if m.Selected {
			app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, fyne.KeyModifierControl)
		}
	}

	app.selectedModName = selectedNames[0]
	for i, m := range app.displayedMods {
		if m.Name == app.selectedModName {
			app.selectedModIndex.Store(int32(i))
			break
		}
	}

	app.updateDescriptionForMod(app.selectedModName)
	app.updateUpDownButtons()
	app.modTable.Refresh()
}

func (app *App) setupShortcuts() {
	canvas := app.mainWindow.Canvas()

	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		if app.orderDirty {
			app.saveCurrentOrder()
			app.orderDirty = false
			app.refreshModList()
			app.appendLog(app.msg("log_order_saved"))
			app.stopBlinkSaveButton()
			app.updateTableBorder()
			app.appendLogToFile("Сохранено сочетанием CTRL+S")
		} else {
			app.appendLog(app.msg("log_order_unchanged"))
		}
	})

	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyF,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		if app.searchEntry != nil {
			canvas.Focus(app.searchEntry)
			app.appendLogToFile("Переход к поиску сочетанием Ctrl+F")
		}
	})
}
