// Servo-Modquisitor-2/ui_build.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"Servo-Modquisitor/themes"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// createTableRow - пустая строка с заданной минимальной высотой.
// Используется и для системной таблицы, и как шаблон ячейки основной
// таблицы: Fyne в templateSize() создаёт свежий экземпляр через
// CreateCell и берёт его MinSize - здесь эту высоту задаёт пустая
// widget.Label (её MinSize ≈ высота строки текста), а spacer задаёт
// нижнюю границу TableRowHeight.
func createTableRow(height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(1, height))
	lbl := widget.NewLabel("")
	return container.NewStack(spacer, lbl)
}

// VBoxWithSpacing - вертикальный layout с заданным отступом между элементами.
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

// buildUI - оркестратор сборки главного окна. Вся конкретика разложена
// по build-методам ниже: каждый отвечает за одну область UI и
// устанавливает соответствующие поля App.
func (app *App) buildUI() {
	app.buildControlButtons()
	app.buildConsole()
	filterSelectWithSize, searchBar := app.buildSearchAndFilter()
	app.buildManagePanel()
	topPanelWithBg := app.buildTopPanel(filterSelectWithSize, searchBar)
	app.buildHeaderTable()
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
	// Иконки ────────────────────────────────────────────────────
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
	// updateAllRes := app.loadIconResource("update_all", "assets/buttons/update_all_mods_blue_p.png")
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

	// Перемещение / выбор ──────────────────────────────────────
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

	// Удаление ─────────────────────────────────────────────────
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

	// Версия / порядок ─────────────────────────────────────────
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

	// Сохранение / обновление ──────────────────────────────────
	app.btnSaveOrder = NewIconButton(saveRes, func() {
		app.saveOrderFromUI(nil)
	})
	app.btnSaveOrder.SetToolTip(app.msg("btn_save_order_tooltip"))

	app.btnRefresh = NewIconButton(refreshRes, func() {
		go app.refreshWithDirtyCheck()
	})
	app.btnRefresh.SetToolTip(app.msg("btn_refresh_tooltip"))

	app.btnToggle = NewIconButton(onRes, func() { app.toggleGlobalMods() })
	app.btnToggle.SetToolTip(app.msg("btn_toggle_tooltip"))
	app.updateToggleButtonText(app.btnToggle)

	// Массовые операции ────────────────────────────────────────
	app.enableSelectedBtn = NewIconButton(checkedBoxRes, func() { app.setSelectedActive(true) })
	app.enableSelectedBtn.SetToolTip(app.msg("btn_enable_selected_tooltip"))

	app.disableSelectedBtn = NewIconButton(UncheckedBoxRedRes, func() { app.setSelectedActive(false) })
	app.disableSelectedBtn.SetToolTip(app.msg("btn_disable_selected_tooltip"))

	app.enableAllBtn = NewIconButton(enableAllRes, func() { app.setAllModsActive(true) })
	app.enableAllBtn.SetToolTip(app.msg("btn_enable_all_tooltip"))

	app.disableAllBtn = NewIconButton(disableAllRes, func() { app.setAllModsActive(false) })
	app.disableAllBtn.SetToolTip(app.msg("btn_disable_all_tooltip"))

	// Управление, установка, обновления ───────────────────────
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

	// app.btnUpdateAll = NewIconButton(updateAllRes, func() {
	// 	go app.updateAllModsFromNexus()
	// })
	// app.btnUpdateAll.SetToolTip(app.msg("btn_update_all_premium_only"))

	// Запуск игры ──────────────────────────────────────────────
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

	// Кнопка удаления мода в карточке описания ────────────────
	app.btnRemove = NewIconButton(trashRes, func() { app.confirmRemoveSelectedDescription() })
	app.btnRemove.SetToolTip(app.msg("btn_remove_tooltip"))

	// Кнопка "обновить мод" в карточке ────────────────────────
	app.btnUpdateMod = NewIconButton(updateRes, func() { app.updateModFromDescription() })
	app.btnUpdateMod.SetToolTip(app.msg("btn_update_mod_premium_only"))

	// Кнопка "открыть папку" в карточке ───────────────────────
	app.openFolderBtn = NewIconButton(folderRes, func() { app.openSelectedModFolder() })
	app.openFolderBtn.Importance = widget.MediumImportance
	app.openFolderBtn.SetToolTip(app.msg("open_mod_folder_tooltip"))
}

// loadIconResource читает иконку из embed, логирует ошибку и возвращает
// StaticResource. При ошибке возвращает nil - Fyne корректно обрабатывает
// nil-иконки (кнопка останется без картинки).
func (app *App) loadIconResource(name, path string) fyne.Resource {
	data, err := embeddedFiles.ReadFile(path)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("Could not load %s icon (%s): %v", name, path, err))
		return nil
	}
	return fyne.NewStaticResource(name, data)
}

// statusIconCache хранит уже загруженные иконки статусов. Заполняется
// только из UI-потока (updateCell, updateDescriptionForMod), поэтому
// без мьютекса.
var statusIconCache = map[string]fyne.Resource{}

// statusIcon возвращает canvas.Image с иконкой статуса. Иконка берётся
// из assets/buttons/<filename>.png и кэшируется между вызовами, чтобы
// не читать файл на каждую ячейку таблицы.
func (app *App) statusIcon(filename string) *canvas.Image {
	res, ok := statusIconCache[filename]
	if !ok {
		res = app.loadIconResource(filename, "assets/buttons/"+filename+".png")
		statusIconCache[filename] = res
	}
	img := canvas.NewImageFromResource(res)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(20, 20))
	return img
}

// statusRow — строка секции в правой панели: иконка слева, текст
// справа с переносом. Используется и в секции «Статус», и в «Проблемы».
func (app *App) statusRow(iconName, text string) fyne.CanvasObject {
	icon := app.statusIcon(iconName)
	label := widget.NewLabel(text)
	label.Wrapping = fyne.TextWrapWord
	return container.NewBorder(nil, nil, icon, nil, label)
}

// ─────────────────────────────────────────────────────────────────
// Мелкие обработчики кнопок (вынесены из closures для читаемости)
// ─────────────────────────────────────────────────────────────────

// refreshWithDirtyCheck - логика кнопки "обновить список": если есть
// несохранённые изменения, спрашивает пользователя, что делать.
// Использует асинхронный диалог, чтобы не блокировать горутину.
func (app *App) refreshWithDirtyCheck() {
	if app.orderDirty.Load() {
		app.showChoiceDialog(
			app.mainWindow,
			app.msg("warning_title"),
			app.msg("refresh_discard_changes"),
			func(choice int) {
				// callback выполняется в UI-потоке, поэтому fyne.Do не нужен
				switch choice {
				case 0:
					app.saveOrderFromUI(func() {
						// saveOrderFromUI с onDone != nil НЕ вызывает
						// refreshModList (он это делает только при
						// onDone == nil). А нам здесь нужен именно
						// пересбор списка — это же «сохранить и
						// обновить».
						app.refreshModList()
						app.appendLog(app.msg("log_list_refreshed"))
					})
				case 1:
					// Отмена - ничего не делаем
				case 2:
					app.orderDirty.Store(false)
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
		// Если изменений нет - просто обновляем список.
		// Функция может вызываться из горутины, поэтому используем fyne.Do.
		fyne.Do(func() {
			app.refreshModList()
			app.appendLog(app.msg("log_list_refreshed"))
		})
	}
}

// saveOrderFromUI — вызывается из UI-потока. Само сохранение уходит
// в горутину, чтобы UI не блокировался на loadOrderMutex (его может
// держать фон, например runAllChecks → sorter.CreateLoadOrderFromActive).
//
// onDone == nil: после сохранения вызывается refreshModList().
// onDone != nil: вызывающий сам решает, что делать (в UI-потоке).
func (app *App) saveOrderFromUI(onDone func()) {
	if !app.orderDirty.Load() {
		app.appendLog(app.msg("log_order_unchanged"))
		if onDone != nil {
			onDone()
		}
		return
	}
	go func() {
		app.saveCurrentOrder()
		fyne.Do(func() {
			app.orderDirty.Store(false)
			app.stopBlinkSaveButton()
			app.updateTableBorder()
			app.appendLog(app.msg("log_order_saved"))
			if onDone != nil {
				onDone()
			} else {
				app.refreshModList()
			}
		})
	}()
}

// toggleManagePanel - логика кнопки-шестерёнки: показать/скрыть панель
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

// pickAndInstallArchive - логика кнопки "установить мод": открывает
// диалог выбора архива и запускает установку.
func (app *App) pickAndInstallArchive() {
	fd := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil || reader == nil {
			return
		}
		defer reader.Close()
		path := reader.URI().Path()
		// Фильтр диалога уже разрешает .zip/.rar/.7z, поэтому проверка
		// должна совпадать с ним. Раньше здесь оставалось только
		// strings.HasSuffix(".zip") — пользователь мог выбрать .rar,
		// но получал «please select a ZIP archive» и вынужден был
		// повторять выбор. handleDrop при этом .rar принимает — вот
		// и расхождение в поведении.
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".zip" && ext != ".rar" && ext != ".7z" {
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

// launchGameFromUI - логика кнопок запуска игры (обычной и без лаунчера).
//
// Если включена настройка AutoSortBeforeLaunch — сначала выполняем
// полную проверку (как по кнопке «Автосортировка»): obsolete, malformed,
// empty, incompatible, dependencies, broken, плюс пересортировка
// mod_load_order.txt. Диалоги показываются; пользователь может
// отменить шаг (например, открыть страницу required-мода) — тогда
// игра не запустится. Файл mod_load_order.txt в конце не открывается,
// чтобы не мешать старту.
func (app *App) launchGameFromUI(skipLauncher bool) {
	gameRoot, _ := app.getGameState()
	go func(root string) {
		if isDarktideRunning() {
			app.appendLog(app.msg("game_already_running"))
			return
		}
		ver := detectGameVersion(root)

		// Предполётная проверка + сортировка.
		app.cfgMutex.RLock()
		autoSort := app.cfg.AutoSortBeforeLaunch
		app.cfgMutex.RUnlock()
		if autoSort {
			if !app.runAllChecksInternal(false) {
				app.appendLogToFile("launch: aborted by user during pre-launch checks")
				return
			}
		}

		// Steam-версии нужен запущенный Steam в обоих режимах:
		//  - через steam://rungameid, чтобы клиент подхватил игру;
		//  - при прямом запуске через Proton, потому что игра зовёт
		//    SteamAPI_Init() и без клиента не инициализируется.
		// Xbox-версия Steam не использует, для неё проверку пропускаем.
		if ver == VersionSteam && !app.ensureSteamRunning() {
			return
		}

		if err := app.launchGameFunc(ver, root, skipLauncher); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("launch_error"), err))
		}
	}(gameRoot)
}

// ensureSteamRunning проверяет, запущен ли Steam, и если нет —
// предлагает пользователю варианты. Возвращает true, если можно
// продолжать запуск игры.
//
// ВАЖНО: вызывать ТОЛЬКО из фоновой горутины (использует
// showChoiceDialogSync, который блокируется до ответа пользователя).
func (app *App) ensureSteamRunning() bool {
	if isSteamRunning() {
		return true
	}

	choice := app.showChoiceDialogSync(
		app.mainWindow,
		app.msg("steam_not_running_title"),
		app.msg("steam_not_running_message"),
		app.msg("btn_start_steam"),
		app.msg("btn_launch_anyway"),
		app.msg("btn_cancel"),
	)

	switch choice {
	case 0:
		if err := startSteam(); err != nil {
			app.appendLog(fmt.Sprintf(app.msg("steam_start_failed"), err))
			return false
		}
		app.appendLog(app.msg("steam_start_waiting"))

		// Ждём только появления процесса — это гарантирует, что
		// xdg-open/rundll32 сработал. Дальше (логин, выбор аккаунта,
		// обновления клиента) — забота пользователя.
		if !waitFor(isSteamRunning, 20*time.Second) {
			app.appendLog(app.msg("steam_start_timeout"))
			return false
		}

		// Даём пользователю явно подтвердить готовность. Модальный
		// диалог сам поднимется поверх окна Steam, пользователь залогинится,
		// вернётся в наш диалог и нажмёт «Продолжить».
		return app.confirmSteamLoggedIn()

	case 1:
		app.appendLog(app.msg("steam_launch_anyway"))
		return true

	default:
		// Отмена — молча выходим.
		return false
	}
}

// confirmSteamLoggedIn показывает модальный диалог и блокирует
// вызывающую горутину до тех пор, пока пользователь не подтвердит,
// что залогинился в Steam (или не отменит запуск).
//
// Нужен потому, что определить программно момент «Steam готов
// принимать игру» невозможно: steamwebhelper поднимается ещё на
// экране выбора аккаунта, а до логина SteamAPI не работает.
// Вызывать ТОЛЬКО из фоновой горутины.
func (app *App) confirmSteamLoggedIn() bool {
	resultChan := make(chan bool, 1)

	fyne.Do(func() {
		app.bringToFront()

		var popUp *widget.PopUp

		okBtn := widget.NewButton(app.msg("btn_continue"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- true
		})
		cancelBtn := widget.NewButton(app.msg("btn_cancel"), func() {
			if popUp != nil {
				popUp.Hide()
			}
			resultChan <- false
		})

		msg := widget.NewLabel(app.msg("steam_login_prompt"))
		msg.Wrapping = fyne.TextWrapWord

		content := container.NewVBox(
			widget.NewLabelWithStyle(
				app.msg("steam_not_running_title"),
				fyne.TextAlignCenter,
				fyne.TextStyle{Bold: true},
			),
			widget.NewSeparator(),
			msg,
			widget.NewSeparator(),
			container.NewCenter(container.NewHBox(okBtn, cancelBtn)),
		)

		popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
		popUp.Resize(fyne.NewSize(450, 220))
		popUp.Show()
	})

	return <-resultChan
}

// confirmRemoveSelectedDescription - логика кнопки удаления в карточке
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

	// Снимок displayedMods под RLock — параллельная фоновая
	// перестройка списка не должна гоняться с этим чтением.
	app.modsMutex.RLock()
	displayedNames := make([]string, len(app.displayedMods))
	for i := range app.displayedMods {
		displayedNames[i] = app.displayedMods[i].Name
	}
	app.modsMutex.RUnlock()

	var nextModName string
	for i, n := range displayedNames {
		if n == modName {
			if i+1 < len(displayedNames) {
				nextModName = displayedNames[i+1]
			} else if i-1 >= 0 {
				nextModName = displayedNames[i-1]
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
			app.modTable.Length = func() (int, int) {
				app.modsMutex.RLock()
				n := len(app.displayedMods)
				app.modsMutex.RUnlock()
				return n, TableColumnCount
			}
			app.modTable.Refresh()
			app.updateTableBorder()
			app.appendLog(fmt.Sprintf(app.msg("log_deleted"), modName))

			app.saveCurrentOrder()
			if err := app.syncActiveProfileFromGame(); err != nil {
				app.appendLogToFile(fmt.Sprintf("confirmRemoveSelectedDescription: sync failed: %v", err))
			}
			app.orderDirty.Store(false)
			app.updateTableBorder()

			// Обновляем снимок после удаления.
			app.modsMutex.RLock()
			displayedNames = displayedNames[:0]
			for i := range app.displayedMods {
				displayedNames = append(displayedNames, app.displayedMods[i].Name)
			}
			app.modsMutex.RUnlock()

			if nextModName != "" {
				for i, n := range displayedNames {
					if n == nextModName {
						app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, 0)
						app.modTable.ScrollTo(widget.TableCellID{Row: i, Col: 0})
						break
					}
				}
			} else if len(displayedNames) > 0 {
				newIndex := oldIndex
				if newIndex >= len(displayedNames) {
					newIndex = len(displayedNames) - 1
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

// updateModFromDescription - логика кнопки "обновить мод" в карточке:
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

// openSelectedModFolder - логика кнопки "открыть папку мода" в карточке.
func (app *App) openSelectedModFolder() {
	if app.selectedModName == "" {
		return
	}
	mod, ok := app.findModByName(app.selectedModName)
	if !ok || mod.MissingFolder {
		return
	}
	app.cfgMutex.RLock()
	modsPath := app.cfg.ModsPath
	app.cfgMutex.RUnlock()
	if modsPath == "" {
		return
	}
	modPath := filepath.Join(modsPath, mod.Name)
	app.openPathInFileManager(modPath)
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
		// После смены поиска — скролл наверх: пользователь ожидает
		// увидеть новую выборку с начала.
		app.modTable.ScrollToTop()
	}

	searchBar = container.NewBorder(nil, nil, nil, app.searchClearBtn, searchEntryBox)

	app.filterSelect = widget.NewSelect(app.filterOptions(), nil)
	app.filterSelect.SetSelected(app.msg("filter_all"))
	app.filterSelect.OnChanged = func(s string) {
		app.filterModList()
		// После смены фильтра — скролл наверх по той же причине.
		app.modTable.ScrollToTop()
	}

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
		// app.btnUpdateAll,
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
			if !mod.IsSystem && app.showSelectColumn {
				cont.Add(app.buildSelectCheckboxColumn(id.Row, mod))
			} else {
				cont.Add(widget.NewLabel(""))
			}
		case 1:
			cont.Add(app.buildActiveCheckboxColumn(id.Row, mod))
		case 2:
			cont.Add(app.buildNumberColumn(id.Row, th, variant))
		case 3:
			cont.Add(app.buildNameColumn(id.Row, mod))
		case 4:
			cont.Add(app.buildDateColumn(mod, th, variant))
		case 5:
			cont.Add(app.buildStatusIconsColumn(mod))
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

	// AML-рамка: 3px, чтобы при одновременном показе с dirty-рамкой
	// (2px, сверху) жёлтая торчала тонкой полоской снаружи. Порядок
	// в стеке ниже: amlTableBorder снизу, tableBorder сверху.
	// Так при AML+dirty видны обе — жёлтый контур снаружи, оранжевая
	// обводка внутри. При только AML — жёлтая 3px. При только dirty —
	// оранжевая 2px.
	app.amlTableBorder = canvas.NewRectangle(color.Transparent)
	app.amlTableBorder.StrokeWidth = 1
	app.amlTableBorder.StrokeColor = th.Color(themes.ColorAMLWarning, variant)
	app.amlTableBorder.FillColor = color.Transparent
	app.amlTableBorder.Hide()

	mechData, _ := embeddedFiles.ReadFile(TableBackgroundImage)
	var mechBg *canvas.Image
	if mechData != nil {
		mechBg = canvas.NewImageFromResource(fyne.NewStaticResource(TableBackgroundImage, mechData))
		mechBg.FillMode = canvas.ImageFillContain
		mechBg.Translucency = TableBackgroundOpacity
		app.tableBorderContainer = container.NewStack(
			mechBg,
			app.modTable,
			app.amlTableBorder,
			app.tableBorder,
		)
	} else {
		app.tableBorderContainer = container.NewStack(
			app.modTable,
			app.amlTableBorder,
			app.tableBorder,
		)
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

// buildStatusIconsColumn — колонка 5 основной таблицы: горизонтальный
// ряд иконок статуса.
//
// Порядок подобран так, чтобы два «всегда присутствующих» статуса
// (состояние и источник) занимали фиксированные позиции слева — по ним
// глаз сканирует вертикаль. Вариативная часть (особенности, проблемы,
// обновление) идёт справа и уже не мешает сравнению строк между собой.
//
//  1. Состояние  — on / off_red  (всегда есть)
//  2. Источник   — hand_vulcan / nexus_orange  (всегда есть)
//  3. Особенности — pin, vortex, sys, symlink
//  4. Проблемы    — folder_missing, folder_broken, conflict, skull
//  5. Обновление  — update
func (app *App) buildStatusIconsColumn(mod *checks.ModInfo) fyne.CanvasObject {
	row := container.NewHBox()

	// 1. Состояние (УДАЛЕНО! Теперь это в отдельной колонке с иконками)

	// 2. Источник
	switch mod.Source {
	case "manual":
		row.Add(app.statusIcon("hand_vulcan_yellow"))
	case "nexus":
		row.Add(app.statusIcon("nexus_orange"))
	}

	// 3. Особенности
	if mod.Mandatory {
		row.Add(app.statusIcon("pin_bir"))
	}
	if mod.VortexDeployed {
		row.Add(app.statusIcon("vortex"))
	}
	if mod.IsSystem {
		row.Add(app.statusIcon("sys"))
	}
	if mod.IsSymlink {
		row.Add(app.statusIcon("symlink"))
	}

	// 4. Проблемы
	if mod.MissingFolder {
		row.Add(app.statusIcon("folder_missing"))
	}
	if mod.Broken {
		row.Add(app.statusIcon("folder_broken"))
	}
	if mod.Incompatible {
		row.Add(app.statusIcon("conflict"))
	}
	if mod.Obsolete {
		row.Add(app.statusIcon("skull"))
	}

	// 5. Обновление
	if mod.HasUpdate {
		row.Add(app.statusIcon("update"))
	}

	// Спейсер слева — чтобы иконки не прилипали к границе ячейки.
	pad := canvas.NewRectangle(color.Transparent)
	pad.SetMinSize(fyne.NewSize(theme.Padding()/2, 1))

	return container.NewHBox(pad, row)
}

// buildSelectCheckboxColumn - ячейка колонки 0: чекбокс выделения поверх
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
	// OnChanged ставим ПОСЛЕ SetChecked, чтобы избежать ложных срабатываний при перерисовке
	check.OnChanged = func(b bool) {
		app.updateModSelected(name, b)
		if b {
			// KeyModifierControl — обязательно. Без модификатора
			// Select в Fyne ЗАМЕНЯЕТ выделение (снимает со всех
			// строк и выделяет одну). Это ломало множественное
			// выделение галочками: после клика на второй чекбокс
			// syncSelectionToCheckboxes видел IsRowSelected(first)
			// == false и сбрасывал Selected[first] в модели —
			// и первая галочка визуально снималась.
			//
			// С Control выделение ДОБАВЛЯЕТСЯ к существующему,
			// IsRowSelected для ранее выбранных строк остаётся
			// true, sync ничего не сбрасывает.
			app.modTable.Select(widget.TableCellID{Row: row, Col: 0}, fyne.KeyModifierControl)
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

// reassignSelectionAfterUncheck - если сняли галку с текущего выбранного
// мода, переносим выделение на первую оставшуюся выделенную строку.
func (app *App) reassignSelectionAfterUncheck(name string) {
	app.modsMutex.RLock()
	names := make([]string, len(app.displayedMods))
	selected := make([]bool, len(app.displayedMods))
	for i := range app.displayedMods {
		names[i] = app.displayedMods[i].Name
		selected[i] = app.displayedMods[i].Selected
	}
	app.modsMutex.RUnlock()

	newSelRow := -1
	for i := range names {
		if selected[i] && names[i] != name {
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

// buildActiveCheckboxColumn - колонка 1: чекбокс активности мода.
func (app *App) buildActiveCheckboxColumn(row int, mod *checks.ModInfo) fyne.CanvasObject {
	if mod.IsSystem {
		// Системные моды всегда включены, просто показываем иконку "on"
		img := app.statusIcon("on")
		return container.NewCenter(img)
	}

	name := mod.Name
	// Безопасно копируем текущее состояние в локальную переменную.
	// Если этого не сделать, кнопка при клике может прочитать старое значение,
	// если список уже успел перестроиться (filterModList).
	isActive := mod.Active

	var icon fyne.Resource
	if isActive {
		icon = app.toggleOnIcon
	} else {
		icon = app.toggleOffIcon
	}

	// Создаем кнопку-иконку
	btn := NewIconButton(icon, func() {
		app.toggleModActive(name, !isActive)
	})
	btn.SetIconSize(24)
	btn.Importance = widget.LowImportance
	return container.NewCenter(btn)
}

// buildNumberColumn - колонка 2: порядковый номер строки.
func (app *App) buildNumberColumn(row int, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	t := canvas.NewText(fmt.Sprintf("%2d", row+1), th.Color(theme.ColorNameForeground, variant))
	t.Alignment = fyne.TextAlignCenter
	return t
}

// buildNameColumn - колонка 3: имя мода.
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

// buildDateColumn - колонка 4: дата установки.
func (app *App) buildDateColumn(mod *checks.ModInfo, th fyne.Theme, variant fyne.ThemeVariant) fyne.CanvasObject {
	t := canvas.NewText(app.formatDate(mod.ModTime, app.cfg.DateFormat), th.Color(theme.ColorNameForeground, variant))
	t.Alignment = fyne.TextAlignCenter
	return t
}

// buildNoteColumn - колонка 6: примечание в горизонтальном скролле.
func (app *App) buildNoteColumn(mod *checks.ModInfo) fyne.CanvasObject {
	label := widget.NewLabel(mod.Note)
	label.Wrapping = fyne.TextWrapOff
	scroll := container.NewScroll(label)
	scroll.SetMinSize(fyne.NewSize(0, 35))
	return scroll
}

// onModRowSelected - логика OnSelected основной таблицы.
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
		app.cfgMutex.RLock()
		active := app.cfg.ActiveProfile
		app.cfgMutex.RUnlock()
		if s != active {
			app.switchProfile(s)
		}
	})
	app.profileSelect.PlaceHolder = app.msg("profile_select_placeholder")

	syncRes := app.loadIconResource("profile_sync", "assets/buttons/sync_n.png")
	app.profileSyncBtn = NewIconButton(syncRes, func() { app.SyncProfileManually() })
	app.profileSyncBtn.SetToolTip(app.msg("profile_sync_tooltip"))

	content := container.NewHBox(
		app.counterLabel,
		layout.NewSpacer(),
		app.profileLabel,
		app.profileSelect,
		app.profileSyncBtn,
	)
	return container.NewBorder(nil, nil, nil, nil, content)
}

// ─────────────────────────────────────────────────────────────────
// Карточка описания
// ─────────────────────────────────────────────────────────────────

// thumbnailOverlay - прозрачный слой, реагирующий на клики и меняющий курсор.
type thumbnailOverlay struct {
	widget.BaseWidget
	onTapped func()
}

func newThumbnailOverlay(onTapped func()) *thumbnailOverlay {
	t := &thumbnailOverlay{onTapped: onTapped}
	t.ExtendBaseWidget(t)
	return t
}

func (t *thumbnailOverlay) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

func (t *thumbnailOverlay) Tapped(_ *fyne.PointEvent) {
	if t.onTapped != nil {
		t.onTapped()
	}
}

func (t *thumbnailOverlay) Cursor() desktop.Cursor {
	return desktop.PointerCursor // Курсор-указатель (рука)
}

// buildDescriptionCard собирает правую верхнюю карточку с описанием мода.
// Устанавливает поля descTitle, descAuthor, descBody, descURL, sourceCodeLink,
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
	app.descURL.OnTapped = func() {
		if app.descURL.URL != nil {
			if err := app.myApp.OpenURL(app.descURL.URL); err != nil {
				app.appendLogToFile(fmt.Sprintf("descURL open failed: %v", err))
			} else {
				app.appendLogToFile(fmt.Sprintf("descURL opened: %s", app.descURL.URL))
			}
		} else {
			app.appendLogToFile("descURL tapped, but URL is nil")
		}
	}

	// Thumbnail в правом верхнем углу карточки. Контейнер фиксированного
	// размера; внутри — фон-заглушка и картинка. Пока URL
	// картинки нет или пока она грузится, видна только заглушка.
	//
	// SetMinSize на обоих элементах задаёт MinSize контейнера. Border
	// в metaRow ниже даст этому боксу именно его min-size, а остальное
	// уйдёт метаданным слева.
	// Фон-подложка. Полупрозрачный чёрный, скруглённый — виден
	// только когда картинка ещё грузится или не вписалась в бокс
	// целиком (Contain оставляет поля по краям у неквадратных
	// картинок).
	app.descThumbnailBg = canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 15})
	app.descThumbnailBg.CornerRadius = CornerRadius15
	app.descThumbnailBg.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	app.descThumbnail = canvas.NewImageFromResource(nil)
	app.descThumbnail.FillMode = canvas.ImageFillContain
	app.descThumbnail.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))
	// CornerRadius совпадает с CornerRadius рамки и фона, чтобы
	// скругления на всех трёх слоях смотрелись как одно целое.
	app.descThumbnail.CornerRadius = CornerRadius15

	// Рамка — прозрачная заливка + тонкая обводка. Цвет берём из
	// ColorCRTScreenStroke: это тот же неоновый зелёный, что рисует
	// CRT-экран, и он уже присутствует в остальной карточке.
	// Если захочется нейтральную рамку — ColorDescCardStroke;
	// если ярче — ColorInputBorder.
	app.descThumbnailBorder = canvas.NewRectangle(color.Transparent)
	app.descThumbnailBorder.CornerRadius = CornerRadius15
	app.descThumbnailBorder.StrokeWidth = 1
	// ColorTableBorderDirty - рыжий
	app.descThumbnailBorder.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
	app.descThumbnailBorder.SetMinSize(fyne.NewSize(sizeThumbnailX, sizeThumbnailY))

	// Создаем прозрачный слой для кликов
	clickOverlay := newThumbnailOverlay(func() {
		if app.descThumbnail.Resource != nil {
			app.showEnlargedImage(app.descThumbnail.Resource)
		}
	})

	// Создаем иконку лупы для визуальной подсказки.
	// Правильное имя в стандартной теме Fyne - theme.ZoomInIcon()
	zoomIcon := widget.NewIcon(theme.ZoomInIcon())
	zoomIconBg := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 40}) // Полупрозрачный чёрный фон
	zoomIconBg.CornerRadius = 10

	// Контейнер для иконки с фоном (фиксированный размер 32x32)
	zoomBtn := container.NewStack(zoomIconBg, container.NewPadded(zoomIcon))
	zoomBtn.Resize(fyne.NewSize(32, 32))
	// Позиционируем в правом нижнем углу (отступ 8px)
	zoomBtn.Move(fyne.NewPos(sizeThumbnailX-32-320, sizeThumbnailY-32+40))

	// Оборачиваем в контейнер без лэйаута, чтобы внутренний Stack не перецентрировал иконку
	zoomOverlay := container.NewWithoutLayout(zoomBtn)

	app.descThumbnailBox = container.NewStack(
		app.descThumbnailBg,
		app.descThumbnail,
		clickOverlay, // Прозрачный слой для кликов
		app.descThumbnailBorder,
		zoomOverlay, // Иконка лупы в правом нижнем углу
	)

	app.sourceCodeLink = widget.NewHyperlink("", nil)
	app.sourceCodeLink.Alignment = fyne.TextAlignLeading
	app.sourceCodeLink.OnTapped = func() {
		if app.sourceCodeLink.URL != nil {
			if err := app.myApp.OpenURL(app.sourceCodeLink.URL); err != nil {
				app.appendLogToFile(fmt.Sprintf("sourceCodeLink open failed: %v", err))
			} else {
				app.appendLogToFile(fmt.Sprintf("sourceCodeLink opened: %s", app.sourceCodeLink.URL))
			}
		} else {
			app.appendLogToFile("sourceCodeLink tapped, but URL is nil")
		}
	}

	// Блок версий и дат — таблицей. Формируется один раз, значения
	// обновляются из updateDescriptionForMod через SetText у каждой
	// ячейки. FormLayout даёт выравнивание подписей и значений в две
	// колонки, как в классической «карточке товара».
	//
	// Подписи берём из messages.json; original_upload_label и
	// last_updated_label уже есть и приходят без двоеточия — приклеим
	// его здесь, чтобы не плодить по два ключа на каждую строку.
	app.descVersionLblVersion = widget.NewLabel("")
	app.descVersionValVersion = widget.NewLabel("")
	app.descVersionLblLatest = widget.NewLabel("")
	app.descVersionValLatest = widget.NewLabel("")
	app.descVersionLblOrig = widget.NewLabel("")
	app.descVersionValOrig = widget.NewLabel("")
	app.descVersionLblUpd = widget.NewLabel("")
	app.descVersionValUpd = widget.NewLabel("")
	app.descCategoryLbl = widget.NewLabel("")
	app.descCategoryVal = widget.NewLabel("")
	// Локализованные подписи ставим через refresh — так же, как это
	// будет делаться при смене языка.
	app.refreshDescStaticLabels()

	// Каждая строка — Border с фиксированной колонкой подписей слева
	// и значением справа. Фиксированная ширина через Spacer нужна,
	// чтобы значения начинались в одной вертикали (как в FormLayout),
	// но без его внутренних theme.Padding() — их мы не контролируем.
	//
	// Ширина 160 подобрана по самой длинной подписи ("Original upload:")
	// в 12 локалях с запасом. Если менять — держать синхронно во всех
	// строках, иначе колонка значений «съедет».
	const versionLabelColWidth float32 = 160

	makeVersionRow := func(lbl, val *widget.Label) fyne.CanvasObject {
		lblSpacer := canvas.NewRectangle(color.Transparent)
		lblSpacer.SetMinSize(fyne.NewSize(versionLabelColWidth, 1))
		lblCol := container.NewStack(lblSpacer, lbl)
		return container.NewBorder(nil, nil, lblCol, nil, val)
	}

	app.descVersionGrid = container.New(
		&VBoxWithSpacing{Spacing: SpacingText15},
		makeVersionRow(app.descVersionLblVersion, app.descVersionValVersion),
		makeVersionRow(app.descVersionLblLatest, app.descVersionValLatest),
		makeVersionRow(app.descVersionLblOrig, app.descVersionValOrig),
		makeVersionRow(app.descVersionLblUpd, app.descVersionValUpd),
		makeVersionRow(app.descCategoryLbl, app.descCategoryVal),
	)

	app.descUserNote = widget.NewLabel("")
	app.descUserNote.Wrapping = fyne.TextWrapWord

	app.descCardBgRect = canvas.NewRectangle(th.Color(themes.ColorDescCardBg, variant))
	app.descCardBgRect.CornerRadius = 12
	app.descCardBgRect.StrokeWidth = 0.5
	app.descCardBgRect.StrokeColor = th.Color(themes.ColorDescCardStroke, variant)

	editNoteRes := app.loadIconResource("edit_note", "assets/buttons/edit_sv.png")
	app.btnEditNote = NewIconButton(editNoteRes, func() {
		if app.selectedModName == "" {
			return
		}
		mod, ok := app.findModByName(app.selectedModName)
		if !ok {
			return
		}
		display := mod.DisplayName
		if display == "" {
			display = mod.Name
		}
		app.showEditUserNoteDialog(mod.Name, display)
	})
	app.btnEditNote.SetToolTip(app.msg("btn_edit_note_tooltip"))

	app.descExtraContainer = container.NewVBox()

	leftPadding := canvas.NewRectangle(color.Transparent)
	leftPadding.SetMinSize(fyne.NewSize(30, 1))

	headerRow := container.NewHBox(
		leftPadding,
		app.descTitle,
		layout.NewSpacer(),
		widget.NewSeparator(),
		app.btnEditNote,
		app.btnUpdateMod,
		app.openFolderBtn,
		app.btnRemove,
	)

	titleSep := canvas.NewRectangle(th.Color(themes.ColorCRTScreenStroke, variant))
	titleSep.SetMinSize(fyne.NewSize(0, 3))

	// titleSep — зелёная линия под заголовком карточки. Прячем
	// вместе с шапкой, когда мод не выбран, чтобы линия не висела
	// в пустой карточке.
	app.descTitleSep = container.NewVBox(titleSep)

	// Левый столбец метаданных: автор + оригинальная дата загрузки,
	// ссылки, версии. Три смысловые группы, разделённые линиями —
	// чтобы глаз сразу находил нужное.
	//
	// Nexus и Source стоят друг под другом: так лучше сканируется
	// (левый край обеих ссылок выровнен по одной вертикали), и
	// длинный текст "Open mod page [Nexus]" не сталкивается с
	// "Source Code" при узкой карточке.
	linksColumn := container.New(
		&VBoxWithSpacing{Spacing: SpacingLinks},
		app.descURL,
		app.sourceCodeLink,
	)

	metaLeft := container.NewVBox(
		app.descAuthor,
		widget.NewSeparator(),
		linksColumn,
		widget.NewSeparator(),
		app.descVersionGrid,
	)

	// Строка «метаданные | thumbnail». Border отдаёт правому элементу
	// его MinSize (230×150), остальное забирает metaLeft. Если картинки
	// нет, descThumbnailBox скрыт — metaLeft растягивается на всю
	// ширину карточки.
	metaRow := container.NewBorder(nil, nil, nil, app.descThumbnailBox, metaLeft)

	// Обёртки-секции. Каждая хранит свой separator ПЕРЕД контентом и
	// прячется целиком, когда контента нет — так в пустой карточке
	// (мод не выбран) не остаётся «висячих» линий.
	//
	// metaSection — без верхнего separator'а: его роль играет зелёный
	// titleSep выше. У metaLeft свои внутренние разделители, они
	// прячутся вместе с ним.
	app.descMetaSection = container.NewVBox(metaRow)

	app.descStatusTitle = widget.NewLabel("")
	app.descStatusList = container.New(&VBoxWithSpacing{Spacing: SpacingText11})
	// Заголовок + список — в одном VBoxWithSpacing с тем же отступом,
	// что и внутри списка. Тогда «Status:» и первая строка статуса
	// стоят так же плотно, как строки между собой. Внешний
	// container.NewVBox оставляем: он отделяет секцию от соседей
	// (от предыдущего блока и от следующей секции).
	app.descStatusSection = container.NewVBox(
		widget.NewSeparator(),
		container.New(&VBoxWithSpacing{Spacing: SpacingText13},
			app.descStatusTitle,
			app.descStatusList,
		),
	)
	app.descStatusSection.Hide()

	app.descProblemsTitle = widget.NewLabel("")
	app.descProblemsList = container.New(&VBoxWithSpacing{Spacing: SpacingText11})
	app.descProblemsSection = container.NewVBox(
		widget.NewSeparator(),
		container.New(&VBoxWithSpacing{Spacing: SpacingText13},
			app.descProblemsTitle,
			app.descProblemsList,
		),
	)
	app.descProblemsSection.Hide()

	app.descUserNoteSection = container.NewVBox(
		widget.NewSeparator(),
		app.descUserNote,
	)
	app.descUserNoteSection.Hide()

	descHeader := container.NewBorder(
		nil, nil, nil, nil,
		container.NewVBox(
			headerRow,
			app.descTitleSep,
			app.descMetaSection,
			app.descStatusSection,
			app.descProblemsSection,
			app.descUserNoteSection,
		),
	)

	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, 20))

	// Серый separator между шапкой и описанием тоже прячем в пустом
	// состоянии — обёртка descBodySep хранит сам разделитель и тело
	// описания, чтобы не висел в воздухе без содержимого.
	app.descBodySep = container.NewVBox(
		widget.NewSeparator(),
		app.descBody,
	)

	// Размеры placeholder'а. Лого и glow имеют фиксированные размеры —
	// scale-анимация через canvas.Image.Resize в Fyne пересчитывает
	// текстуру на каждом кадре и даёт видимые «ступеньки». Поэтому
	// пульсирует только прозрачность glow: смена Translucency —
	// это обновление одного uniform'а в шейдере, без пересборки
	// геометрии и без re-upload текстуры.
	const (
		placeholderBoxSize  = 210.0
		placeholderLogoSize = 200.0
		placeholderGlowSize = 215.0
	)

	// Лого-заглушка вместо лупы: два PNG в стеке. Передний —
	// mechanicus.png, задний — mechanicus_glow.png (размытая светящаяся
	// версия, подготовленная в графическом редакторе). Такой подход
	// даёт настоящее мягкое свечение, недостижимое двойной копией того
	// же PNG в Fyne.
	//
	// Анимация идёт через прямые Resize/Move на картинках (см.
	// startPlaceholderPulse). Поэтому обе картинки лежат в
	// container.NewWithoutLayout — этот контейнер НЕ трогает
	// layout-систему Fyne, что убирает рывки, которые были при
	// SetMinSize (тот пересчитывал layout всего дерева до меток).
	logoData, _ := embeddedFiles.ReadFile("assets/mechanicus.png")
	glowData, _ := embeddedFiles.ReadFile("assets/mechanicus_glow.png")

	logoRes := fyne.NewStaticResource("placeholder-logo.png", logoData)
	glowRes := fyne.NewStaticResource("placeholder-glow.png", glowData)

	// Задний слой — свечение.
	app.descPlaceholderGlow = canvas.NewImageFromResource(glowRes)
	app.descPlaceholderGlow.FillMode = canvas.ImageFillStretch
	app.descPlaceholderGlow.Translucency = 0.25

	app.descPlaceholderLogo = canvas.NewImageFromResource(logoRes)
	app.descPlaceholderLogo.FillMode = canvas.ImageFillStretch

	// Размеры фиксированы, позиции не меняются — обычный Stack.
	// Лого и glow выровнены по центру через GridWrap.
	app.descPlaceholderLogo.SetMinSize(fyne.NewSize(placeholderLogoSize, placeholderLogoSize))
	app.descPlaceholderGlow.SetMinSize(fyne.NewSize(placeholderGlowSize, placeholderGlowSize))

	logoStack := container.NewStack(
		container.NewCenter(app.descPlaceholderGlow),
		container.NewCenter(app.descPlaceholderLogo),
	)
	logoWithSize := container.NewGridWrap(
		fyne.NewSize(placeholderBoxSize, placeholderBoxSize),
		logoStack,
	)

	app.descPlaceholderTitle = canvas.NewText("", th.Color(theme.ColorNameForeground, variant))
	app.descPlaceholderTitle.Alignment = fyne.TextAlignCenter
	app.descPlaceholderTitle.TextSize = theme.TextSize() + 14
	app.descPlaceholderTitle.TextStyle = fyne.TextStyle{Bold: true}

	app.descPlaceholderHint = canvas.NewText("", th.Color(theme.ColorNamePlaceHolder, variant))
	app.descPlaceholderHint.Alignment = fyne.TextAlignCenter
	app.descPlaceholderHint.TextSize = theme.TextSize() + 8

	placeholderTopSpacer := canvas.NewRectangle(color.Transparent)
	placeholderTopSpacer.SetMinSize(fyne.NewSize(0, 40))

	placeholderMidSpacer := canvas.NewRectangle(color.Transparent)
	placeholderMidSpacer.SetMinSize(fyne.NewSize(0, 16))

	placeholderBottomSpacer := canvas.NewRectangle(color.Transparent)
	placeholderBottomSpacer.SetMinSize(fyne.NewSize(0, 16))

	app.descPlaceholder = container.NewVBox(
		placeholderTopSpacer,
		container.NewCenter(logoWithSize),
		placeholderMidSpacer,
		container.NewCenter(app.descPlaceholderTitle),
		placeholderBottomSpacer,
		container.NewCenter(app.descPlaceholderHint),
	)

	// descModContent — вся карточка мода. Внутри те же элементы, что
	// и раньше, но теперь они прячутся одним махом вместе с обёрткой.
	app.descModContent = container.NewVBox(
		descHeader,
		app.descBodySep,
		spacer,
		app.descExtraContainer,
	)
	app.descModContent.Hide() // изначально мод не выбран

	app.descCardContent = container.NewVBox(
		app.descPlaceholder,
		app.descModContent,
	)

	app.descCardScroll = container.NewScroll(app.descCardContent)
	app.descCardScroll.SetMinSize(fyne.NewSize(DescScrollMinWidth, DescScrollMinHeight))

	// Оверлей поверх фона, но ПОД содержимым. Полупрозрачный чёрный:
	// затемняет карточку в пустом состоянии. Скрыт при выбранном моде.
	//
	// 30/255 альфы — лёгкое притемнение. Достаточно, чтобы «пустая»
	// карточка отличалась от «рабочей», но не настолько, чтобы
	// потерялись цвета фона или рамки.
	app.descCardOverlay = canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 30})
	app.descCardOverlay.Hide()

	descCard := container.NewStack(
		app.descCardBgRect,
		app.descCardOverlay,
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
		app.tableBorderContainer,
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
	if app.descThumbnailBorder != nil {
		app.descThumbnailBorder.StrokeColor = th.Color(theme.ColorNameSeparator, variant)
		app.descThumbnailBorder.Refresh()
	}
	if app.tableBorder != nil {
		app.tableBorder.StrokeColor = th.Color(themes.ColorTableBorderDirty, variant)
		app.tableBorder.Refresh()
	}
	if app.amlTableBorder != nil {
		app.amlTableBorder.StrokeColor = th.Color(themes.ColorAMLWarning, variant)
		app.amlTableBorder.Refresh()
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

	if app.descPlaceholderTitle != nil {
		app.descPlaceholderTitle.Color = th.Color(theme.ColorNameForeground, variant)
		app.descPlaceholderTitle.Refresh()
	}
	if app.descPlaceholderHint != nil {
		app.descPlaceholderHint.Color = th.Color(theme.ColorNameDisabled, variant)
		app.descPlaceholderHint.Refresh()
	}

	if app.headerTable != nil {
		app.headerTable.Refresh()
	}
	if app.modTable != nil {
		app.modTable.Refresh()
	}

	for _, tbl := range []*widget.Table{app.headerTable, app.modTable} {
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
		app.manageBtn, app.searchClearBtn, app.btnRemoveAll, app.btnRemoveSelected, app.btnEditNote,
	} {
		if btn != nil {
			btn.Refresh()
		}
	}
}

// ─────────────────────────────────────────────────────────────────
// Логирование
// ─────────────────────────────────────────────────────────────────

// maxLogSegments - верхняя граница числа сегментов в лог-виджете.
// Без неё Segments растёт монотонно за всё время работы приложения
// (а appendLog вызывается в т.ч. на каждую операцию с модом), и через
// несколько часов работы UI начинает тормозить из-за отрисовки
// огромного RichText. (#20)
const maxLogSegments = 500

func (app *App) appendLog(text string) {
	if app.logWindow == nil {
		app.appendLogToFile(text)
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
		// перерисовка тысяч сегментов; делаем это батчами - только
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
	app.appendLogToFile(text)
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
	app.refreshDescStaticLabels()

	if name == "" {
		app.descTitle.Text = app.msg("select_mod")
		app.descTitle.Refresh()

		// Тексты placeholder'а пересобираются здесь же — так они
		// автоматически подхватывают смену языка (changeLanguage
		// вызывает updateDescriptionForMod("")).
		if app.descPlaceholderTitle != nil {
			app.descPlaceholderTitle.Text = app.msg("select_mod")
			app.descPlaceholderTitle.Refresh()
		}
		if app.descPlaceholderHint != nil {
			app.descPlaceholderHint.Text = app.msg("desc_placeholder")
			app.descPlaceholderHint.Refresh()
		}

		// Показываем placeholder, всю остальную карточку прячем
		// целиком — вместе с заголовком, кнопками, метаданными и
		// сепараторами.
		if app.descPlaceholder != nil {
			app.descPlaceholder.Show()
		}
		if app.descModContent != nil {
			app.descModContent.Hide()
		}
		if app.descCardOverlay != nil {
			app.descCardOverlay.Show()
			app.descCardOverlay.Refresh()
		}

		// Небольшая уборка, чтобы при следующем выборе мода описание
		// не «мигнуло» старым содержимым.
		app.descURL.SetURL(nil)
		app.descURL.SetText("")
		app.descBody.SetText("")
		if app.descThumbnailBox != nil {
			app.descThumbnailBox.Hide()
			app.descThumbnailURL = ""
		}
		if app.descStatusSection != nil {
			app.descStatusSection.Hide()
		}
		if app.descProblemsSection != nil {
			app.descProblemsSection.Hide()
		}
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

	// Скрываем placeholder, показываем всю карточку мода. Всё
	// остальное (metaSection, conflictSection, userNoteSection,
	// titleSep, bodySep) уже внутри descModContent — им не нужны
	// отдельные Show()/Hide() в этой ветке.
	if app.descPlaceholder != nil {
		app.descPlaceholder.Hide()
	}
	if app.descModContent != nil {
		app.descModContent.Show()
	}
	if app.descCardOverlay != nil {
		app.descCardOverlay.Hide()
		app.descCardOverlay.Refresh()
	}

	if app.descExtraContainer != nil {
		app.descExtraContainer.Show()
	}

	if app.descUserNote != nil {
		un := app.userNotes.Get(mod.Name)
		if un != "" {
			app.descUserNote.SetText("📝 " + un)
			if app.descUserNoteSection != nil {
				app.descUserNoteSection.Show()
			}
		} else {
			if app.descUserNoteSection != nil {
				app.descUserNoteSection.Hide()
			}
		}
	}

	app.cfgMutex.RLock()
	dateFormat := app.cfg.DateFormat
	app.cfgMutex.RUnlock()

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
	// Author — одной строкой. Original upload переехал ниже, в блок
	// версий: там он стоит парой с Last updated, обе даты описывают
	// «когда мод появился» и «когда обновлялся».
	app.descAuthor.SetText(fmt.Sprintf(app.msg("author_label"), author))

	app.descInstalled.SetText(fmt.Sprintf(app.msg("installed_label"),
		app.formatDate(mod.ModTime, dateFormat)))

	// Блок версий/дат. Каждое значение ставится отдельной SetText
	// в свою ячейку FormLayout'а. Если данных нет — оставляем пустую
	// строку: FormLayout всё равно рисует ячейку (она пустая), но
	// подпись слева видна, и «дырка» в таблице читается как «данные
	// ещё не подтянуты», а не как баг верстки.
	//
	// Пустая строка вместо «—» потому, что мод без nexus_versions
	// и без даты — редкий случай, а прочерк на каждой второй строке
	// выглядел бы шумно.

	// Version / Latest — только если есть cacheKey и кэш.
	// Обнуляем на каждом входе, чтобы не «протухли» с прошлого мода.
	if app.descVersionValVersion != nil {
		app.descVersionValVersion.SetText("")
	}
	if app.descVersionValLatest != nil {
		app.descVersionValLatest.SetText("")
	}

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
			app.descVersionValVersion.SetText(info.Version)
			if latest, ok := app.getLatestVersion(cacheKey); ok && latest != "" {
				app.descVersionValLatest.SetText(latest)
			}
		}
	}

	// Original upload / Last updated — независимо от cacheKey.
	// Даты приходят с Nexus в enrichModFromNexus и лежат прямо в ModInfo.
	if app.descVersionValOrig != nil {
		if !mod.OriginalUpload.IsZero() {
			app.descVersionValOrig.SetText(app.formatDate(mod.OriginalUpload, dateFormat))
		} else {
			app.descVersionValOrig.SetText("")
		}
	}
	if app.descVersionValUpd != nil {
		if !mod.LastUpdated.IsZero() {
			app.descVersionValUpd.SetText(app.formatDate(mod.LastUpdated, dateFormat))
		} else {
			app.descVersionValUpd.SetText("")
		}
	}

	// Обновляем категорию с локализацией
	if app.descCategoryVal != nil {
		if mod.Category != "" {
			// Ищем перевод по ключу "category_НазваниеКатегории"
			locKey := "category_" + mod.Category
			translated := app.msg(locKey)
			if translated != "" {
				app.descCategoryVal.SetText(translated)
			} else {
				// Если перевода нет, показываем оригинал
				app.descCategoryVal.SetText(mod.Category)
			}
		} else {
			app.descCategoryVal.SetText("-")
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

	if app.sourceCodeLink != nil {
		if mod.SourceCodeURL != "" {
			if u, err := url.Parse(mod.SourceCodeURL); err == nil {
				app.sourceCodeLink.SetURL(u)
				app.sourceCodeLink.SetText(app.msg("source_code_url"))
			} else {
				app.sourceCodeLink.SetURL(nil)
				app.sourceCodeLink.SetText("")
			}
		} else {
			app.sourceCodeLink.SetURL(nil)
			app.sourceCodeLink.SetText("")
		}
	}

	// ─── Секция «Статус» ──────────────────────────────────────────
	// Самое актуальное — сверху: обновление, потом источник, потом
	// особенности. Вкл/Выкл и Mandatory здесь НЕ показываем — они
	// дублируют чекбокс и позицию в списке.
	if app.descStatusList != nil {
		app.descStatusList.Objects = nil

		// Обновление — самое актуальное событие.
		if mod.HasUpdate {
			text := app.msg("status_update_available")

			var statusKey string
			switch mod.Name {
			case "dmf":
				statusKey = "8:dmf"
			case "base":
				statusKey = "19:base"
			case "autopatch":
				statusKey = "709:autopatch"
			default:
				if mod.URL != "" {
					if id := helpers.ExtractModIDFromURL(mod.URL); id != 0 {
						statusKey = fmt.Sprintf("%d:%s", id, mod.Name)
					}
				}
			}
			cur, latest := "", ""
			if statusKey != "" {
				if info, ok := app.getCachedVersion(statusKey); ok {
					cur = info.Version
				}
				if l, ok := app.getLatestVersion(statusKey); ok {
					latest = l
				}
			}
			if cur != "" && latest != "" {
				text = fmt.Sprintf("%s: %s → %s", text, cur, latest)
			} else if latest != "" {
				text = fmt.Sprintf("%s: %s", text, latest)
			}
			app.descStatusList.Add(app.statusRow("update", text))
		}

		// Источник
		switch mod.Source {
		case "manual":
			app.descStatusList.Add(app.statusRow("hand_vulcan", app.msg("status_manual")))
		case "nexus":
			app.descStatusList.Add(app.statusRow("nexus_orange", app.msg("status_nexus")))
		}

		// Особенности
		if mod.VortexDeployed {
			app.descStatusList.Add(app.statusRow("vortex", app.msg("status_vortex")))
		}
		if mod.IsSymlink {
			app.descStatusList.Add(app.statusRow("symlink", app.msg("status_symlink")))
		}
		if mod.IsSystem {
			app.descStatusList.Add(app.statusRow("sys", app.msg("status_system")))
		}
		if mod.Obsolete {
			app.descStatusList.Add(app.statusRow("skull", app.msg("desc_obsolete")))
		}

		if len(app.descStatusList.Objects) > 0 {
			app.descStatusSection.Show()
		} else {
			app.descStatusSection.Hide()
		}
		app.descStatusList.Refresh()
	}

	// ─── Секция «Проблемы» ────────────────────────────────────────
	// Три источника: отсутствует папка, повреждён .mod, конфликтует
	// с другим модом. Порядок — по убыванию критичности.
	if app.descProblemsList != nil {
		app.descProblemsList.Objects = nil

		if mod.MissingFolder {
			app.descProblemsList.Add(app.statusRow(
				"folder_missing", app.msg("status_missing_folder")))
		}
		if mod.Broken {
			app.descProblemsList.Add(app.statusRow(
				"folder_broken", app.msg("desc_broken")))
		}
		if mod.Incompatible {
			desc := ""
			for _, pair := range checks.GetIncompatiblePairs() {
				if pair.Mod1 != mod.Name && pair.Mod2 != mod.Name {
					continue
				}
				other := pair.Mod1
				if other == mod.Name {
					other = pair.Mod2
				}
				if !checks.FolderExists(other) {
					continue
				}
				desc = checks.GetIncompatibleDesc(pair.Mod1, pair.Mod2)
				break
			}
			if desc == "" {
				desc = app.msg("desc_conflict")
			}
			app.descProblemsList.Add(app.statusRow("conflict", desc))
		}

		if len(app.descProblemsList.Objects) > 0 {
			app.descProblemsSection.Show()
		} else {
			app.descProblemsSection.Hide()
		}
		app.descProblemsList.Refresh()
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

	// Thumbnail обновляем последним — когда все метки уже расставлены.
	// Это позволяет layout сразу знать финальный размер metaLeft.
	app.updateDescriptionThumbnail(mod)

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
// в переданный указатель из горутины - это была гонка с UI-потоком,
// который читал эти же поля для отрисовки. Теперь значения собираются
// в локальные переменные и переносятся в allMods под modsMutex.Lock.
//
// Параметр mod - это снимок ModInfo (не указатель), поэтому поля можно
// безопасно читать из горутины.
func (app *App) enrichModFromNexus(mod *checks.ModInfo) {
	if app.getAuthToken() == "" || mod.URL == "" {
		return
	}
	modID := helpers.ExtractModIDFromURL(mod.URL)
	if modID == 0 {
		return
	}

	// Снимок имени - mod может быть указателем в displayedMods, которую
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

		// Локальные переменные - не пишем в mod (это поле displayedMods).
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
					updated := *entry
					updated.NexusFilePattern = pattern
					checks.UpdateModDBEntry(updated)
					checks.SaveModDatabase()
					app.appendLog(fmt.Sprintf(app.msg("log_autosaved_stable_pattern"), modName, pattern))
				}
			}
		}

		// Ленивое дозаполнение NexusPictureURL для модов, установленных
		// до появления этого поля (или для случаев, когда первичная
		// установка шла не через autoAddModToDatabase).
		//
		// Дополнительный API-запрос делается ОДИН раз на мод — после
		// сохранения URL в базу условие entry.NexusPictureURL == ""
		// больше не выполняется.
		if entry := checks.GetModDBEntry(modName); entry != nil && entry.NexusPictureURL == "" {
			info, err := app.FetchNexusModInfo(modID, app.getAuthToken())
			if err == nil && info != nil && info.PictureURL != "" {
				updated := *entry
				updated.NexusPictureURL = info.PictureURL
				checks.UpdateModDBEntry(updated)
				checks.SaveModDatabase()

				fyne.Do(func() {
					// Синхронизируем allMods, чтобы UI сразу увидел URL.
					app.modsMutex.Lock()
					for i := range app.allMods {
						if app.allMods[i].Name == modName {
							app.allMods[i].NexusPictureURL = info.PictureURL
							break
						}
					}
					app.modsMutex.Unlock()

					// Если этот же мод всё ещё выбран — обновляем картинку.
					if app.selectedModName == modName {
						if m, ok := app.findModByName(modName); ok {
							app.updateDescriptionThumbnail(m)
						}
					}
				})
			}
		}

		fyne.Do(func() {
			// (#4) Запись в allMods - под modsMutex.
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
	if patcher == PatcherNone || gameRoot == "" {
		btn.icon = app.toggleOffIcon
		btn.Disable()
		btn.Refresh()
		return
	}

	// DLL установлена: состояние берём из флага DISABLE_AUTOPATCHER.
	// Bundle мог быть пропатчен самой DLL — это не «чужой патч»,
	// а штатное поведение.
	if AutopatcherDLLInstalled(gameRoot) {
		if AutopatcherDisabled(gameRoot) {
			btn.icon = app.toggleOffIcon
		} else {
			btn.icon = app.toggleOnIcon
		}
		btn.SetToolTip(app.msg("btn_toggle_tooltip"))
		btn.Enable()
		btn.Refresh()
		return
	}

	// DLL нет — смотрим на bundle, как раньше.
	patchedBy := BundlePatchedBy(gameRoot)
	switch patchedBy {
	case "SMQ":
		btn.icon = app.toggleOnIcon
		btn.SetToolTip(app.msg("btn_toggle_tooltip"))
		btn.Enable()
	case "unknown":
		// Действительно чужой патч: ни .patch_999, ни boot_bundle_next_patch,
		// а что-то ещё.
		btn.icon = app.toggleOnIcon
		btn.SetToolTip(app.msg("bundle_foreign_patch_warning"))
		btn.Enable()
	default:
		btn.icon = app.toggleOffIcon
		btn.SetToolTip(app.msg("btn_toggle_tooltip"))
		btn.Enable()
	}
	btn.Refresh()
}

func (app *App) updateUpDownButtons() {
	// Снимок selectedModName — читается в UI-потоке, но фоновая
	// removeModFromData теперь пишет его через fyne.Do, т.е. тоже
	// в UI-потоке. Прямое чтение безопасно, потому что мы сами в UI.
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

	app.modsMutex.RLock()
	idx := -1
	for i, m := range app.displayedMods {
		if m.Name == app.selectedModName {
			idx = i
			break
		}
	}
	total := len(app.displayedMods)
	app.modsMutex.RUnlock()

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
		if idx == total-1 {
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
// (#4) Раньше функция читала app.allMods без modsMutex - это была гонка
// с фоновыми горутинами (removeSelectedMods → removeModFromData, которая
// держит Lock). Теперь весь блок построения нового списка идёт под
// modsMutex.Lock - UI-обновления (Refresh, Scroll) вынесены за скобки.
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

	// (#4) Построение displayedMods - под modsMutex.Lock, потому что
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

	// Пересчитываем selectedModIndex из selectedModName — после
	// перестройки displayedMods индекс мог измениться (при смене
	// фильтра/поиска/порядка). Иначе ScrollTo уедет на другой мод.
	if app.selectedModName != "" {
		newIdx := -1
		for i := range app.displayedMods {
			if app.displayedMods[i].Name == app.selectedModName {
				newIdx = i
				break
			}
		}
		app.selectedModIndex.Store(int32(newIdx))
	} else {
		app.selectedModIndex.Store(-1)
	}

	app.modTable.Length = func() (int, int) { return len(app.displayedMods), TableColumnCount }
	app.modTable.Refresh()
	if selIdx := app.selectedModIndex.Load(); selIdx >= 0 {
		app.modTable.ScrollTo(widget.TableCellID{Row: int(selIdx), Col: 0})
	}
	// ScrollToTop здесь намеренно НЕ вызывается: при операциях без
	// явной смены фильтра (toggle, refresh, install) список не должен
	// прыгать в начало. Явный скролл наверх делают обработчики
	// filterSelect.OnChanged и searchEntry.OnChanged.

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
		// applySelectionFromMods — не только перерисовывает таблицу,
		// но и переносит Selected из модели в визуальное выделение
		// строк через modTable.Select(..., fyne.KeyModifierControl).
		//
		// Раньше здесь стоял filterModList: он обновлял Selected в
		// модели и Refresh-ил таблицу, но НЕ вызывал Select для строк.
		// Из-за этого Ctrl+A / кнопки «выделить всё» / «снять всё»
		// давали невидимое состояние — Selected в модели true, а
		// строки в таблице не подсвечены. Первый же клик без
		// модификатора по любой строке снимал визуальное выделение
		// со всех и через syncSelectionToCheckboxes сбрасывал
		// Selected у невидимых в false, теряя только что сделанный
		// выбор.
		previous := app.selectedModName

		app.applySelectionFromMods()

		// applySelectionFromMods выставляет selectedModName на первый
		// выделенный. Для Ctrl+A это логично: «текущий» становится
		// первый. Но если пользователь до Ctrl+A смотрел на конкретный
		// мод и тот всё ещё видим — оставляем фокус на нём, чтобы
		// описание не перескочило на другой мод из-за массового
		// выделения.
		//
		// Восстанавливаем только при selected == true. При снятии
		// (selected == false) пустое состояние с пустым описанием —
		// это корректно и ожидаемо.
		if selected && previous != "" && app.selectedModName != previous {
			app.modsMutex.RLock()
			stillVisible := false
			for i := range app.displayedMods {
				if app.displayedMods[i].Name == previous {
					stillVisible = true
					break
				}
			}
			app.modsMutex.RUnlock()
			if stillVisible {
				app.selectedModName = previous
				app.updateDescriptionForMod(previous)
				app.updateUpDownButtons()
			}
		}
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
		app.orderDirty.Store(true)
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
			// app.forceRefreshTable()
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
		app.orderDirty.Store(true)
		fyne.Do(func() {
			app.updateTableBorder()
			app.filterModList()
		})
	}
}

func (app *App) startBlink(btn *CustomButton, activeFlag *atomic.Bool, condition func() bool) {
	if activeFlag.Load() {
		return
	}
	activeFlag.Store(true)
	go func() {
		for activeFlag.Load() && condition() {
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
		return app.orderDirty.Load()
	})
}

func (app *App) stopBlinkSaveButton() {
	app.blinkSaveOrderActive.Store(false)
}

func (app *App) updateTableBorder() {
	if app.tableBorder == nil {
		return
	}
	if app.orderDirty.Load() {
		app.tableBorder.Show()
		if !app.blinkSaveOrderActive.Load() {
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
			app.modsMutex.RLock()
			idx := -1
			for i := range app.displayedMods {
				if app.displayedMods[i].Name == modName {
					idx = i
					break
				}
			}
			app.modsMutex.RUnlock()

			if idx < 0 {
				return
			}
			app.modTable.Select(widget.TableCellID{Row: idx, Col: 0}, 0)
			app.modTable.ScrollTo(widget.TableCellID{Row: idx, Col: 0})
		})
	})
}

// syncSelectionToCheckboxes переносит выделение строк таблицы в поля
// Selected моделей.
//
// (#4) Было две ошибки:
//  1. Индексы. Таблица построена по displayedMods, а синхронизация
//     шла по allMods[i] - при активном фильтре/поиске индексы не
//     совпадали, и флаг Selected улетал не тому моду.
//  2. Мьютекс. allMods защищён modsMutex, а функция писала в него
//     под Lock (это правильно), но читала displayedMods по позиции
//     - а displayedMods перестраивается filterModList'ом из UI-потока,
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

	// Форсируем пересчёт размера скролла Fyne Table. Трюк:
	// временно отдаём другое число, Refresh, потом возвращаем
	// ЖИВУЮ функцию, читающую len(app.displayedMods) — не
	// зафиксированное значение. Иначе Length навсегда останется
	// числом на момент вызова, и таблица будет рисовать неверное
	// количество строк после того, как displayedMods изменится
	// (вплоть до того, что часть модов просто пропадёт из вида).
	rows, cols := app.modTable.Length()
	app.modTable.Length = func() (int, int) { return rows + 1, cols }
	app.modTable.Refresh()

	app.modTable.Length = func() (int, int) {
		return len(app.displayedMods), TableColumnCount
	}
	app.modTable.Refresh()
}

func (app *App) applySelectionFromMods() {
	if app.modTable == nil {
		return
	}
	app.suppressSelectionEvents = true
	defer func() { app.suppressSelectionEvents = false }()

	app.modTable.ClearSelection()

	app.modsMutex.RLock()
	names := make([]string, len(app.displayedMods))
	selectedFlags := make([]bool, len(app.displayedMods))
	for i := range app.displayedMods {
		names[i] = app.displayedMods[i].Name
		selectedFlags[i] = app.displayedMods[i].Selected
	}
	app.modsMutex.RUnlock()

	var selectedNames []string
	for i := range names {
		if selectedFlags[i] {
			selectedNames = append(selectedNames, names[i])
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

	for i := range names {
		if selectedFlags[i] {
			app.modTable.Select(widget.TableCellID{Row: i, Col: 0}, fyne.KeyModifierControl)
		}
	}

	app.selectedModName = selectedNames[0]
	for i := range names {
		if names[i] == app.selectedModName {
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

	// Сохранение списка - CTRL+S
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		app.saveOrderFromUI(nil)
		// app.appendLog("CTRL+S")
	})

	// Поиск - Ctrl+F
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyF,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		if app.searchEntry != nil {
			canvas.Focus(app.searchEntry)
			// app.appendLog("Ctrl+F")
		}
	})

	// Выделить всё / снять выделение - Ctrl+A
	//
	// Если фокус сейчас в текстовом поле (searchEntry, moveToEntry или Entry внутри диалога) - Ctrl+A должен работать как «выделить весь текст», а не как «выделить все моды».
	// Canvas-level shortcuts перехватывают событие до того, как его получит виджет, поэтому проверяем фокус и сами вызываем SelectAll на Entry.
	canvas.AddShortcut(&fyne.ShortcutSelectAll{}, func(shortcut fyne.Shortcut) {
		if _, ok := canvas.Focused().(*widget.Entry); ok {
			return
		}
		app.selectAllModsToggle()
		// app.appendLog("Ctrl+A")
	})

	// Автосортировка - Ctrl+T
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyT,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		go app.runAllChecks()
		// app.appendLog("Ctrl+T")
	})

	// Обновить список - Ctrl+R
	// refreshWithDirtyCheck сам покажет диалог, если есть несохранённые изменения.
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyR,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		go app.refreshWithDirtyCheck()
		// app.appendLog("Ctrl+R")
	})

	// Проверить обновления - Ctrl+U
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyU,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		go app.checkNexusUpdates()
		// app.appendLog("Ctrl+U")
	})

	// Вкл/выкл модов глобально - Ctrl+O
	// Вызываем напрямую, как кнопка в верхней панели. toggleGlobalMods сам логирует результат.
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		app.toggleGlobalMods()
		// app.appendLog("Ctrl+O")
	})

	// Панель управления модами - Ctrl+M
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyM,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		app.toggleManagePanel()
		// app.appendLog("Ctrl+M")
	})

	// Редактирование заметки к моду - Ctrl+N
	canvas.AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyN,
		Modifier: fyne.KeyModifierControl,
	}, func(shortcut fyne.Shortcut) {
		// Не перехватываем клавишу, если пользователь вводит текст в поле
		if f := canvas.Focused(); f != nil {
			if _, ok := f.(*widget.Entry); ok {
				return
			}
		}

		// Если мод не выбран - ничего не делаем
		if app.selectedModName == "" {
			return
		}

		mod, ok := app.findModByName(app.selectedModName)
		if !ok {
			return
		}

		display := mod.DisplayName
		if display == "" {
			display = mod.Name
		}

		// Открываем диалог заметки
		app.showEditUserNoteDialog(mod.Name, display)
	})

	// Запуск игры без модификатора - F5 (быстрый), F6 (обычный).
	// rawShortcut возвращает ровно то ShortcutName, которое Fyne
	// генерирует для чистых функциональных клавиш.
	canvas.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF5, Modifier: 0}, func(_ fyne.Shortcut) {
		app.launchGameFromUI(true)
		// app.appendLog("F5")
	})
	canvas.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyF6, Modifier: 0}, func(_ fyne.Shortcut) {
		app.launchGameFromUI(false)
		// app.appendLog("F6")
	})

	// Удаление выделенных модов без модификатора - Delete.
	//
	// ВАЖНО: Delete используется в Entry для удаления символов. Чтобы
	// не мешать вводу, проверяем фокус: если он в текстовом поле —
	// не перехватываем.
	canvas.AddShortcut(&desktop.CustomShortcut{KeyName: fyne.KeyDelete, Modifier: 0}, func(_ fyne.Shortcut) {
		if f := canvas.Focused(); f != nil {
			if _, ok := f.(*widget.Entry); ok {
				f.TypedKey(&fyne.KeyEvent{Name: fyne.KeyDelete})
				return
			}
		}
		app.deleteSelectedModsWithConfirm()
		// app.appendLog("Delete")
	})
}

// selectAllModsToggle вызывается по Ctrl+A. Если хоть один мод уже
// выделен - снимаем выделение. Иначе выделяем все видимые моды и
// показываем панель массовых операций.
func (app *App) selectAllModsToggle() {
	app.modsMutex.RLock()
	anySelected := false
	for _, m := range app.displayedMods {
		if m.Selected {
			anySelected = true
			break
		}
	}
	app.modsMutex.RUnlock()

	if !anySelected {
		// Показываем панель безусловно — не полагаемся на Visible(),
		// который в Fyne может врать для контейнеров внутри VBox.
		app.showManagePanelForce()
	}

	app.selectAllMods(!anySelected)
}

// showManagePanelForce включает панель массовых операций и колонку
// чекбоксов, не проверяя текущее состояние. Идемпотентна — повторный
// вызов ничего не сломает.
func (app *App) showManagePanelForce() {
	if app.managePanel == nil {
		return
	}
	app.managePanel.Show()
	app.showSelectColumn = true

	if app.headerTable != nil {
		app.headerTable.SetColumnWidth(0, ColSelectWidth)
		app.headerTable.Refresh()
	}
	if app.modTable != nil {
		app.modTable.SetColumnWidth(0, ColSelectWidth)
		app.modTable.Refresh()
	}
	app.managePanel.Refresh()

	// Show() меняет флаг Visible, но родительский VBox не пересчитывает
	// layout сам по себе. Без перерисовки всего дерева панель остаётся
	// в VBox с нулевой высотой и визуально не появляется. Canvas.Refresh
	// обходит дерево рекурсивно и вызывает Layout там, где нужно.
	if app.mainWindow != nil && app.mainWindow.Canvas() != nil {
		app.mainWindow.Canvas().Refresh(app.mainWindow.Content())
	}
}

// deleteSelectedModsWithConfirm вызывается по Ctrl+Delete. Использует
// тот же диалог, что и кнопка массового удаления - единообразно и
// безопасно (случайное нажатие Ctrl+Delete ничего не удалит без
// подтверждения).
func (app *App) deleteSelectedModsWithConfirm() {
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
}

// updateDescriptionThumbnail обновляет картинку в правой карточке.
//
// Вызывается из UI-потока (updateDescriptionForMod). Загрузка идёт
// асинхронно; пока она идёт, видна только заглушка — layout не «прыгает»
// при появлении картинки, потому что descThumbnailBox имеет
// фиксированный MinSize.
//
// Если у мода нет NexusPictureURL — прячем весь box; метаданные слева
// растягиваются на всю ширину карточки.
func (app *App) updateDescriptionThumbnail(mod checks.ModInfo) {
	if app.descThumbnailBox == nil || app.descThumbnail == nil {
		return
	}

	url := mod.NexusPictureURL
	app.descThumbnailURL = url

	if url == "" {
		app.descThumbnailBox.Hide()
		app.descThumbnail.Resource = nil
		app.descThumbnail.Image = nil // ← важно: без этого остаётся старая текстура
		app.descThumbnail.Refresh()
		return
	}

	app.descThumbnailBox.Show()
	// Сбрасываем ОБА поля. Resource — это источник данных, Image —
	// уже декодированная картинка. Fyne в Refresh'е смотрит и на то, и на
	// другое: если оставить Image, рендер останется прежним, даже если
	// Resource = nil.
	app.descThumbnail.Resource = nil
	app.descThumbnail.Image = nil
	app.descThumbnail.Refresh()

	app.thumbnailCacheMu.Lock()
	if res, ok := app.thumbnailCache[url]; ok {
		app.thumbnailCacheMu.Unlock()
		app.descThumbnail.Resource = res
		app.descThumbnail.Refresh()
		return
	}
	app.thumbnailCacheMu.Unlock()

	go func(expectedURL string) {
		defer func() { recover() }()

		res, err := fetchNexusThumbnail(expectedURL)
		if err != nil {
			app.appendLogToFile(fmt.Sprintf(
				"thumbnail fetch failed for %q: %v", expectedURL, err))
			fyne.Do(func() {
				if app.descThumbnailURL == expectedURL {
					app.descThumbnailBox.Hide()
					app.descThumbnail.Resource = nil
					app.descThumbnail.Image = nil
					app.descThumbnail.Refresh()
				}
			})
			return
		}

		// Проверяем только заголовок — DecodeConfig дешёвый и не
		// разворачивает кадры. Для анимированного WebP (VP8X+ANIM)
		// он может вернуть ошибку, если версия x/image/webp старая.
		// В этом случае НЕ прячем бокс: пусть Fyne декодирует картинку
		// своим путём (в форке добавлен deanimator).
		if cfg, _, cfgErr := image.DecodeConfig(bytes.NewReader(res.Content())); cfgErr == nil {
			if cfg.Width < 10 || cfg.Height < 10 {
				app.appendLogToFile(fmt.Sprintf(
					"thumbnail too small for %q: %dx%d, skipping",
					expectedURL, cfg.Width, cfg.Height))
				fyne.Do(func() {
					if app.descThumbnailURL == expectedURL {
						app.descThumbnailBox.Hide()
						app.descThumbnail.Resource = nil
						app.descThumbnail.Image = nil
						app.descThumbnail.Refresh()
					}
				})
				return
			}
		}

		app.thumbnailCacheMu.Lock()
		app.thumbnailCache[expectedURL] = res
		app.thumbnailCacheMu.Unlock()

		fyne.Do(func() {
			if app.descThumbnailURL != expectedURL {
				return
			}
			app.descThumbnail.Resource = res
			app.descThumbnail.Refresh()
		})
	}(url)
}

// refreshDescStaticLabels обновляет локализованные подписи карточки
// описания: версии/даты и заголовки секций «Статус», «Проблемы».
//
// Раньше эти подписи создавались один раз в buildDescriptionCard и не
// переиспользовались при changeLanguage — из-за этого после смены
// языка они оставались на старом. Теперь вызывается и при первом
// построении карточки, и на каждом updateDescriptionForMod, который
// changeLanguage тоже дёргает.
func (app *App) refreshDescStaticLabels() {
	if app.descVersionLblVersion != nil {
		app.descVersionLblVersion.SetText(app.msg("table_lbl_version"))
	}
	if app.descVersionLblLatest != nil {
		app.descVersionLblLatest.SetText(app.msg("table_lbl_latest"))
	}
	if app.descVersionLblOrig != nil {
		app.descVersionLblOrig.SetText(app.msg("original_upload_label") + ":")
	}
	if app.descVersionLblUpd != nil {
		app.descVersionLblUpd.SetText(app.msg("last_updated_label") + ":")
	}
	if app.descStatusTitle != nil {
		app.descStatusTitle.SetText(app.msg("desc_section_status") + ":")
	}
	if app.descProblemsTitle != nil {
		app.descProblemsTitle.SetText(app.msg("desc_section_problems") + ":")
	}
	if app.descCategoryLbl != nil {
		app.descCategoryLbl.SetText(app.msg("category_label") + ":")
	}
}
