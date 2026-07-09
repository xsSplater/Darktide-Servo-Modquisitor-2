// disk_search.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// GameSearchResult содержит результат поиска
type GameSearchResult struct {
	Path    string
	Success bool
}

// getSearchRoots возвращает список корневых путей для поиска (диски на Windows, корневые каталоги на Linux)
func getSearchRoots() []string {
	if runtime.GOOS == "windows" {
		var drives []string
		for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
			path := string(drive) + ":\\"
			if _, err := os.Stat(path); err == nil {
				drives = append(drives, path)
			}
		}
		return drives
	}
	// Linux / Unix
	var roots []string
	for _, p := range []string{"/", "/home", "/mnt", "/media", "/run/media"} {
		if _, err := os.Stat(p); err == nil {
			roots = append(roots, p)
		}
	}
	return roots
}

// showDiskSearchDialog показывает диалог выбора корневых путей и запускает поиск
func (app *App) showDiskSearchDialog() chan GameSearchResult {
	resultChan := make(chan GameSearchResult, 1)

	roots := getSearchRoots()

	// Чекбоксы для корневых путей (в одну строку с переносом, максимум 4 колонки)
	var checkboxes []fyne.CanvasObject
	for _, r := range roots {
		cb := widget.NewCheck(r, nil)
		cb.SetChecked(true)
		checkboxes = append(checkboxes, cb)
	}

	// Определяем количество колонок: не более 4, если больше - 4
	cols := len(checkboxes)
	if cols > 4 {
		cols = 4
	} else if cols == 0 {
		cols = 1
	}
	grid := container.NewGridWithColumns(cols, checkboxes...)

	// Заголовок
	labelText := app.messages["disk_search_label"]
	if runtime.GOOS != "windows" {
		labelText = app.messages["disk_search_label_linux"]
	}
	label := widget.NewLabelWithStyle(labelText, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// Прогресс и статус
	progress := widget.NewProgressBar()
	progress.Hide()
	statusLabel := widget.NewLabel(app.messages["disk_search_status_ready"])
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.Wrapping = fyne.TextWrapWord

	// Кнопки
	searchBtn := widget.NewButtonWithIcon(app.messages["disk_search_btn_start"], theme.SearchIcon(), nil)
	searchBtn.Importance = widget.HighImportance
	manualBtn := widget.NewButtonWithIcon(app.messages["disk_search_btn_manual"], theme.FolderOpenIcon(), nil)
	cancelBtn := widget.NewButton(app.messages["btn_cancel"], nil)

	var popUp *widget.PopUp
	content := container.NewVBox(
		container.NewPadded(
			container.NewVBox(
				label,
				grid,
				widget.NewSeparator(),
				statusLabel,
				progress,
				container.NewHBox(
					container.NewCenter(searchBtn),
					container.NewCenter(manualBtn),
					container.NewCenter(cancelBtn),
				),
			),
		),
	)

	popUp = widget.NewModalPopUp(content, app.mainWindow.Canvas())
	popUp.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight200))
	popUp.Show()

	// Переменные для управления поиском
	var cancelCtx context.Context
	var cancelFunc context.CancelFunc
	var searchRunning bool
	var mu sync.Mutex

	updateStatus := func(text string) {
		fyne.Do(func() { statusLabel.SetText(text) })
	}

	updateProgress := func(current, total int) {
		fyne.Do(func() {
			if total > 0 {
				progress.SetValue(float64(current) / float64(total))
			}
		})
	}

	searchBtn.OnTapped = func() {
		mu.Lock()
		if searchRunning {
			if cancelFunc != nil {
				cancelFunc()
			}
			searchBtn.SetText(app.messages["disk_search_btn_start"])
			searchRunning = false
			mu.Unlock()
			return
		}
		searchRunning = true
		mu.Unlock()

		// Собираем выбранные корни
		var selected []string
		for _, obj := range checkboxes {
			if cb, ok := obj.(*widget.Check); ok && cb.Checked {
				selected = append(selected, cb.Text)
			}
		}
		if len(selected) == 0 {
			updateStatus(app.messages["disk_search_status_no_selection"])
			mu.Lock()
			searchRunning = false
			mu.Unlock()
			return
		}

		searchBtn.SetText(app.messages["disk_search_btn_stop"])
		progress.Show()
		updateStatus(fmt.Sprintf(app.messages["disk_search_status_searching"], 0))
		progress.SetValue(0)

		cancelCtx, cancelFunc = context.WithCancel(context.Background())

		go func() {
			defer func() {
				mu.Lock()
				searchRunning = false
				searchBtn.SetText(app.messages["disk_search_btn_start"])
				progress.Hide()
				mu.Unlock()
			}()

			found, err := app.searchGameOnDrives(selected, cancelCtx, func(current, total int, currentPath string) {
				updateProgress(current, total)
				if current%100 == 0 {
					updateStatus(fmt.Sprintf(app.messages["disk_search_status_searching"], current))
				}
			})

			if err != nil {
				updateStatus(fmt.Sprintf(app.messages["disk_search_status_error"], err.Error()))
				return
			}

			if cancelCtx.Err() != nil {
				updateStatus(app.messages["disk_search_status_cancelled"])
				return
			}

			if len(found) == 0 {
				updateStatus(app.messages["disk_search_status_not_found"])
				return
			}

			if len(found) == 1 {
				updateStatus(fmt.Sprintf(app.messages["disk_search_status_found"], found[0]))
				time.Sleep(500 * time.Millisecond)
				fyne.Do(func() {
					popUp.Hide()
					resultChan <- GameSearchResult{Path: found[0], Success: true}
				})
				return
			}

			// Несколько кандидатов - показываем список выбора
			fyne.Do(func() {
				items := found
				list := widget.NewList(
					func() int { return len(items) },
					func() fyne.CanvasObject { return widget.NewLabel("") },
					func(id widget.ListItemID, obj fyne.CanvasObject) {
						obj.(*widget.Label).SetText(items[id])
					},
				)
				list.OnSelected = func(id widget.ListItemID) {
					popUp.Hide()
					resultChan <- GameSearchResult{Path: items[id], Success: true}
				}

				var newPopUp *widget.PopUp
				newPopUp = widget.NewModalPopUp(
					container.NewVBox(
						widget.NewLabelWithStyle(app.messages["disk_search_multiple_found_title"], fyne.TextAlignCenter, fyne.TextStyle{Bold: true}),
						container.NewVScroll(list),
						widget.NewButton(app.messages["btn_cancel"], func() {
							newPopUp.Hide()
							popUp.Hide()
							resultChan <- GameSearchResult{Success: false}
						}),
					),
					app.mainWindow.Canvas(),
				)
				newPopUp.Resize(fyne.NewSize(DialogMinWidth, DialogMinHeight))
				newPopUp.Show()
				popUp.Hide()
			})
		}()
	}

	manualBtn.OnTapped = func() {
		popUp.Hide()
		fyne.Do(func() {
			dlg := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
				if err == nil && uri != nil {
					path := filepath.FromSlash(uri.Path())
					resultChan <- GameSearchResult{Path: path, Success: true}
				} else {
					resultChan <- GameSearchResult{Success: false}
				}
			}, app.mainWindow)
			dlg.Resize(fyne.NewSize(FileDialogWidth, FileDialogHeight))
			dlg.Show()
		})
	}

	cancelBtn.OnTapped = func() {
		mu.Lock()
		if cancelFunc != nil {
			cancelFunc()
		}
		mu.Unlock()
		popUp.Hide()
		resultChan <- GameSearchResult{Success: false}
	}

	return resultChan
}

// searchGameOnDrives выполняет поиск Darktide в указанных корневых путях
func (app *App) searchGameOnDrives(roots []string, ctx context.Context, progress func(current, total int, currentPath string)) ([]string, error) {
	var results []string
	var mu sync.Mutex
	var totalChecked int

	// Папки, которые игнорируем для ускорения
	ignoreDirs := map[string]bool{
		"Windows": true, "Program Files": true, "Program Files (x86)": true,
		"System32": true, "System": true, "AppData": true,
		"$Recycle.Bin": true, "System Volume Information": true,
		"boot": true, "Users": true, "ProgramData": true,
		"Microsoft": true, "WindowsApps": true, "WpSystem": true,
		"proc": true, "sys": true, "dev": true, "run": true, "tmp": true, "var": true, "etc": true,
	}

	const maxDepth = 5 // /Steam/steamapps/common/Warhammer... 4 уровня, запас 5

	var walkDir func(path string, depth int) error
	walkDir = func(path string, depth int) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if depth > maxDepth {
			return nil
		}

		entries, err := os.ReadDir(path)
		if err != nil {
			return nil
		}

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "$") {
				continue
			}
			if ignoreDirs[name] {
				continue
			}

			fullPath := filepath.Join(path, name)

			// Проверяем, не является ли папка корнем Darktide
			if strings.EqualFold(name, "Warhammer 40,000 DARKTIDE") {
				if _, err := os.Stat(filepath.Join(fullPath, "binaries")); err == nil {
					mu.Lock()
					results = append(results, fullPath)
					mu.Unlock()
					continue
				}
				if _, err := os.Stat(filepath.Join(fullPath, "content")); err == nil {
					mu.Lock()
					results = append(results, fullPath)
					mu.Unlock()
					continue
				}
			}

			if depth+1 <= maxDepth {
				totalChecked++
				if progress != nil {
					progress(totalChecked, 0, fullPath)
				}
				walkDir(fullPath, depth+1)
			}
		}
		return nil
	}

	for _, root := range roots {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		walkDir(root, 0)
	}

	return results, nil
}
