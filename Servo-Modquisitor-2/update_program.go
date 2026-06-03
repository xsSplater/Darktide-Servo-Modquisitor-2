// update_program.go
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func (app *App) updateProgramFromGitHub() {
	app.appendLog(app.messages["checking_github_update"])

	latestVersion, downloadURL, err := getLatestReleaseInfo()
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["version_check_failed_program"], err))
		return
	}

	if compareVersions(latestVersion, AppVersion) <= 0 {
		app.appendLog(app.messages["github_already_latest"])
		return
	}

	fyne.Do(func() {
		dialog.ShowConfirm(
			app.messages["github_update_title"],
			fmt.Sprintf(app.messages["github_update_message"], latestVersion),
			func(ok bool) {
				if ok {
					go app.downloadAndReplace(downloadURL, latestVersion)
				}
			},
			app.mainWindow,
		)
	})
}

func getLatestReleaseInfo() (version, downloadURL string, err error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(GitHubReleaseAPI)
	if err != nil {
		return "", "", fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	tagPrefix := `"tag_name":"`
	start := strings.Index(string(body), tagPrefix)
	if start == -1 {
		return "", "", fmt.Errorf("tag_name not found in release info")
	}
	start += len(tagPrefix)
	end := strings.Index(string(body)[start:], `"`)
	if end == -1 {
		return "", "", fmt.Errorf("malformed tag_name")
	}
	version = string(body)[start : start+end]
	version = strings.TrimPrefix(version, "v")

	browserDownloadURL := `"browser_download_url":"`
	start = strings.Index(string(body), browserDownloadURL)
	if start == -1 {
		return "", "", fmt.Errorf("no download URL found")
	}
	start += len(browserDownloadURL)
	end = strings.Index(string(body)[start:], `"`)
	if end == -1 {
		return "", "", fmt.Errorf("malformed download URL")
	}
	downloadURL = string(body)[start : start+end]

	return version, downloadURL, nil
}

func (app *App) downloadAndReplace(downloadURL, newVersion string) {
	exePath, err := os.Executable()
	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("cannot locate executable: %v", err), app.mainWindow)
		})
		return
	}
	exeDir := filepath.Dir(exePath)

	// Создаём временный файл в папке с программой
	tmpFile, err := os.CreateTemp(exeDir, "servo-update-*.exe")
	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("cannot create temp file: %v", err), app.mainWindow)
		})
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	// defer os.Remove(tmpPath) не нужен - удалим вручную после использования

	bar := widget.NewProgressBar()
	lbl := widget.NewLabel(app.messages["downloading_update"])
	var dlg dialog.Dialog
	fyne.Do(func() {
		dlg = dialog.NewCustom(app.messages["btn_downloading"], app.messages["btn_cancel"], container.NewVBox(lbl, bar), app.mainWindow)
		dlg.Show()
	})

	err = app.DownloadFileWithProgress(downloadURL, tmpPath, bar)
	fyne.Do(func() {
		if dlg != nil {
			dlg.Hide()
		}
	})
	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("download failed: %v", err), app.mainWindow)
		})
		os.Remove(tmpPath)
		return
	}

	newExePath := exePath + ".new"
	os.Remove(newExePath) // удаляем старый .new, если остался

	if isZipFile(tmpPath) {
		err = extractExeFromZip(tmpPath, newExePath)
		os.Remove(tmpPath) // временный архив больше не нужен
	} else {
		// Просто переименовываем скачанный exe в .new
		err = os.Rename(tmpPath, newExePath)
	}

	if err != nil {
		fyne.Do(func() {
			dialog.ShowError(fmt.Errorf("cannot prepare new executable: %v", err), app.mainWindow)
		})
		os.Remove(tmpPath)
		return
	}

	fyne.Do(func() {
		dialog.ShowConfirm(
			app.messages["update_ready"],
			app.messages["update_ready_restart"],
			func(ok bool) {
				if ok {
					replaceAndRestart(exePath, newExePath)
				} else {
					os.Remove(newExePath)
				}
			},
			app.mainWindow,
		)
	})
}

func extractExeFromZip(zipPath, destExe string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if strings.HasSuffix(name, ".exe") || (runtime.GOOS == "linux" && strings.Contains(name, "servo")) {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			defer rc.Close()
			out, err := os.Create(destExe)
			if err != nil {
				return err
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			return err
		}
	}
	return fmt.Errorf("executable not found in zip")
}

// isZipFile проверяет, является ли файл ZIP-архивом (по сигнатуре)
func isZipFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 4)
	_, err = io.ReadFull(f, buf)
	return err == nil && buf[0] == 0x50 && buf[1] == 0x4B && buf[2] == 0x03 && buf[3] == 0x04
}
