// Servo-Modquisitor-2/batch_runner.go
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

type waitOutcome int

const (
	outcomeInstalled waitOutcome = iota
	outcomeSkipped
	outcomeFailed
	outcomeCancelled
)

// runBatchRunner — фоновая горутина, обрабатывающая элементы по одному.
func (app *App) runBatchRunner(d *BatchDialog, cancelChan <-chan struct{}) {
	// Отложенная функция, которая сработает даже при отмене или ошибке
	defer func() {
		if d.mode == BatchModeInstall || d.mode == BatchModeUpdate {
			// Принудительно включаем все не-системные моды
			app.modsMutex.Lock()
			for i := range app.allMods {
				if !app.allMods[i].IsSystem {
					app.allMods[i].Active = true
				}
			}
			app.modsMutex.Unlock()

			app.saveCurrentOrder()
			if err := app.syncActiveProfileFromGame(); err != nil {
				app.appendLogToFile(fmt.Sprintf("Batch: sync failed: %v", err))
			}
			fyne.Do(func() {
				app.refreshModList()
				app.orderDirty.Store(false)
				app.updateTableBorder()
			})
		}
	}()

	select {
	case <-cancelChan:
		return
	case <-d.startChan:
	}

	var itemsToProcess []*BatchItem
	for _, item := range d.items {
		if item.Selected {
			itemsToProcess = append(itemsToProcess, item)
		} else {
			d.SetStatus(item, BatchSkipped)
		}
	}

	total := len(itemsToProcess)
	if total == 0 {
		d.Finish(0, 0)
		return
	}

	success := 0
	loggedIn := app.isLoggedIn()

	// Создаем контекст для отмены скачивания
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Слушаем кнопку "Отмена" и отменяем контекст
	go func() {
		select {
		case <-cancelChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	for i, item := range itemsToProcess {
		select {
		case <-cancelChan:
			d.UpdateProgress(app.msg("batch_cancelled"))
			return // defer сработает и сохранит то, что уже установилось
		default:
		}

		d.UpdateProgress(fmt.Sprintf(app.msg("batch_processing"), i+1, total, item.Name))
		d.SetStatus(item, BatchWaiting)

		// ─── Premium ──────────────────────────────────────────────
		if loggedIn && item.FileID != 0 && item.NexusModID != 0 {
			d.SetStatus(item, BatchDownloading)
			err := app.downloadAndInstallBatchItem(ctx, item, d)
			if err == nil {
				d.SetStatus(item, BatchInstalled)
				success++
				continue
			} else if errors.Is(err, context.Canceled) {
				d.UpdateProgress(app.msg("batch_cancelled"))
				return // defer сработает
			} else if !isAuthError(err) {
				d.SetStatus(item, BatchFailed, err.Error())
				continue
			}
			app.appendLogToFile(fmt.Sprintf(
				"Batch: premium failed for %s, switching to manual: %v", item.Name, err))
		}

		// ─── Manual ───────────────────────────────────────────────
		d.SetStatus(item, BatchWaiting)

		if d.autoOpenCheck.Checked {
			if urlStr := item.NexusURL(); urlStr != "" {
				if u, err := url.Parse(urlStr); err == nil {
					fyne.Do(func() { fyne.CurrentApp().OpenURL(u) })
				}
			}
		}

		app.drainBatchSignals()

		switch d.waitForItem(item, cancelChan) {
		case outcomeInstalled:
			success++
		case outcomeSkipped:
			// уже помечен
		case outcomeFailed, outcomeCancelled:
			return // defer сработает
		}
	}

	d.Finish(success, total)
}

// downloadAndInstallBatchItem — скачивает и устанавливает один мод.
func (app *App) downloadAndInstallBatchItem(ctx context.Context, item *BatchItem, d *BatchDialog) error {
	directURL, filename, err := app.getPremiumDownloadURL(
		fmt.Sprintf("%d", item.NexusModID), fmt.Sprintf("%d", item.FileID))
	if err != nil {
		return fmt.Errorf("download link: %w", err)
	}

	tempDir := app.getProgramTempDir()
	destPath := filepath.Join(tempDir, filename)

	// Передаем ctx в функцию скачивания
	if err := app.DownloadFileWithProgress(ctx, directURL, destPath, nil); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("download: %w", err)
	}

	installedName, installedVersion, err := app.InstallModFromArchive(destPath, true, item.Version, item.ID)
	os.Remove(destPath)
	if err != nil {
		return fmt.Errorf("install: %w", err)
	}

	ver := installedVersion
	if ver == "" {
		ver = item.Version
	}
	if installedName != "" && ver != "" {
		cacheKey := fmt.Sprintf("%d:%s", item.NexusModID, installedName)
		app.cacheModVersion(cacheKey, installedName, ver, time.Now().Unix(), "nexus", 0)
	}
	return nil
}

func (d *BatchDialog) waitForItem(item *BatchItem, cancelChan <-chan struct{}) waitOutcome {
	for {
		select {
		case <-cancelChan:
			return outcomeCancelled

		case id := <-d.skipChan:
			if id == item.ID {
				d.SetStatus(item, BatchSkipped)
				return outcomeSkipped
			}

		case sig := <-d.app.batchInstallChan:
			if batchSignalMatches(sig, item) {
				d.SetStatus(item, BatchInstalled)
				return outcomeInstalled
			}

		case <-time.After(10 * time.Minute):
			d.SetStatus(item, BatchFailed, d.app.msg("batch_timeout"))
			return outcomeFailed
		}
	}
}

func batchSignalMatches(sig BatchSignal, item *BatchItem) bool {
	if sig.Folder != "" && sig.Folder == item.ID {
		return true
	}
	if sig.NexusModID != 0 && item.NexusModID != 0 && sig.NexusModID == item.NexusModID {
		return true
	}
	return false
}

func (app *App) drainBatchSignals() {
	for {
		select {
		case <-app.batchInstallChan:
		default:
			return
		}
	}
}

func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "401") || strings.Contains(s, "403")
}
