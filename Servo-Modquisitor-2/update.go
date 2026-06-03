// update.go
package main

import (
	"Servo-Modquisitor/checks"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2/dialog"
)

func (app *App) downloadSortFiles() error {
	files := []struct {
		remote string
		local  string
	}{
		{modDatabaseURL, FileNameModDatabase},
		{modMandatoryURL, FileNameMandatoryRules},
	}
	for _, f := range files {
		dest := filepath.Join(app.cfg.ModsPath, f.local)
		if err := app.downloadFile(f.remote, dest); err != nil {
			return fmt.Errorf("%s: %w", f.local, err)
		}
	}
	return nil
}

func (app *App) ensureSortFiles() {
	missing := false
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameMandatoryRules)); os.IsNotExist(err) {
		missing = true
	}
	if _, err := os.Stat(filepath.Join(app.cfg.ModsPath, FileNameModDatabase)); os.IsNotExist(err) {
		missing = true
	}
	if !missing {
		return
	}

	app.appendLog(app.messages["sort_files_missing_short"])

	if app.cfg.SkipSortFilesPrompt {
		app.appendLog(app.messages["download_skip_forever"])
		return
	}

	choice := app.showChoiceDialog(
		app.mainWindow,
		app.messages["sort_files_missing"],
		app.messages["download_sort_files_question"],
		app.messages["yes"],
		app.messages["skip"],
		app.messages["download_skip_forever"],
	)
	switch choice {
	case 0: // Да - скачать
		if err := app.downloadSortFiles(); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
			dialog.ShowInformation(app.messages["sort_files_missing"], fmt.Sprintf(app.messages["download_failed"], err), app.mainWindow)
		} else {
			app.appendLog(app.messages["sort_files_updated"])
			// Перезагрузить базы
			if err := app.loadModDatabase(FileNameModDatabase); err == nil {
				checks.SetModDatabase(app.modDatabase)
			}
			if err := checks.LoadExternalLists(FileNameMandatoryRules); err == nil {
				app.cfg.LastMandatoryRulesVersion = checks.GetExternalVersion()
				saveConfig(app.cfg)
			}
			app.refreshModList()
		}
	case 2: // Пропустить и больше не спрашивать
		app.cfg.SkipSortFilesPrompt = true
		saveConfig(app.cfg)
		fallthrough
	case 1: // Пропустить
		app.appendLog(app.messages["download_skipped"])
	}
}

func (app *App) shouldCheckUpdates() bool {
	if app.cfg.UpdateCheckFrequency == "never" {
		return false
	}
	if app.cfg.UpdateCheckFrequency == "every_start" {
		return true
	}
	if app.cfg.LastUpdateCheck == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339, app.cfg.LastUpdateCheck)
	if err != nil {
		return true
	}
	now := time.Now()
	switch app.cfg.UpdateCheckFrequency {
	case "weekly":
		return now.Sub(last) >= 7*24*time.Hour
	case "monthly":
		return now.After(last.AddDate(0, 1, 0))
	case "yearly":
		return now.After(last.AddDate(1, 0, 0))
	}
	return false
}

func (app *App) downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		out.Close()
		os.Remove(dest)
		return fmt.Errorf("copy failed: %w", err)
	}
	return nil
}

func (app *App) checkForProgramUpdate() {
	u, _ := url.Parse(ServoMQModPage)
	_ = app.myApp.OpenURL(u)
	app.appendLog(app.messages["open_download_page"])
}

func (app *App) updateSortFiles() {
	updates := []string{}
	if need, newVer, err := app.checkVersion(modDatabaseURL, app.cfg.LastModDatabaseVersion); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_update_check_error_db"], err))
	} else if need {
		updates = append(updates, FileNameModDatabase)
		app.cfg.LastModDatabaseVersion = newVer
	}
	if need, newVer, err := app.checkVersion(modMandatoryURL, app.cfg.LastMandatoryRulesVersion); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_update_check_error_mandatory"], err))
	} else if need {
		updates = append(updates, FileNameMandatoryRules)
		app.cfg.LastMandatoryRulesVersion = newVer
	}

	if len(updates) == 0 {
		app.appendLog(app.messages["no_updates_found"])
		return
	}

	choice := app.showChoiceDialog(app.mainWindow,
		app.messages["update_title"],
		fmt.Sprintf(app.messages["update_message"], strings.Join(updates, ", ")),
		app.messages["yes"],
		app.messages["skip"],
	)
	if choice == 0 {
		if err := app.downloadSortFiles(); err != nil {
			app.appendLog(fmt.Sprintf(app.messages["download_failed"], err))
			dialog.ShowInformation(app.messages["update_title"], fmt.Sprintf(app.messages["download_failed"], err), app.mainWindow)
		} else {
			app.appendLog(app.messages["sort_files_updated"])
			app.loadModDatabase(FileNameModDatabase)
			checks.SetModDatabase(app.modDatabase)
			checks.LoadExternalLists(FileNameMandatoryRules)
			app.refreshModList()
			saveConfig(app.cfg)
		}
	}
}

func (app *App) checkVersion(url, localVersion string) (bool, string, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", url, nil)
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var wrapper struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&wrapper); err != nil {
		return false, "", err
	}
	// Сравниваем версии: обновление нужно только если удалённая версия НОВЕЕ локальной
	if compareVersions(wrapper.Version, localVersion) > 0 {
		return true, wrapper.Version, nil
	}
	return false, localVersion, nil
}

// compareVersions сравнивает две версии (например, "1.3.3" и "1.1.1").
// Возвращает 1, если a > b; -1, если a < b; 0, если равны.
func compareVersions(a, b string) int {
	// убираем 'v' в начале, если есть
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	// Добиваем нулями до одинаковой длины, чтобы корректно сравнивать, например, "1.2" и "1.2.0"
	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}
	for len(partsA) < maxLen {
		partsA = append(partsA, "0")
	}
	for len(partsB) < maxLen {
		partsB = append(partsB, "0")
	}

	for i := 0; i < maxLen; i++ {
		numA, errA := strconv.Atoi(partsA[i])
		numB, errB := strconv.Atoi(partsB[i])
		if errA != nil || errB != nil {
			// Если не удалось распарсить, сравниваем как строки
			if partsA[i] < partsB[i] {
				return -1
			} else if partsA[i] > partsB[i] {
				return 1
			}
			continue
		}
		if numA < numB {
			return -1
		} else if numA > numB {
			return 1
		}
	}
	return 0
}

func (app *App) checkForUpdates() {
	app.cfg.LastUpdateCheck = time.Now().Format(time.RFC3339)
	saveConfig(app.cfg)

	updates := []string{}
	if need, newVer, err := app.checkVersion(modDatabaseURL, app.cfg.LastModDatabaseVersion); err == nil && need {
		updates = append(updates, FileNameModDatabase)
		_ = newVer
	}
	if need, newVer, err := app.checkVersion(modMandatoryURL, app.cfg.LastMandatoryRulesVersion); err == nil && need {
		updates = append(updates, FileNameMandatoryRules)
		_ = newVer
	}
	if len(updates) > 0 {
		app.appendLog(fmt.Sprintf(app.messages["log_new_sorting_files_available"], strings.Join(updates, ", ")))
	}
	// go app.checkForProgramUpdateGitHub() // Раньше открывалась страница релизов на Гитхабе
}

func (app *App) checkForProgramUpdateGitHub() {
	// Показываем, что начали проверку
	app.appendLog(app.messages["checking_github_update"])

	// Запрос к GitHub API
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", GitHubReleaseAPI, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "token "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		app.appendLog(fmt.Sprintf(app.messages["github_api_error"], err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		app.appendLog(fmt.Sprintf(app.messages["github_api_error_status"], resp.StatusCode))
		return
	}

	// Разбор JSON ответа
	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["github_api_error"], err))
		return
	}

	// Сравниваем версии (AppVersion имеет формат "0.9.9", tag_name — "v0.9.9")
	latestVer := strings.TrimPrefix(release.TagName, "v")
	if compareVersions(latestVer, AppVersion) > 0 {
		app.appendLog(fmt.Sprintf(app.messages["github_update_available"], AppVersion, release.TagName))
		// Предлагаем открыть страницу релиза
		choice := app.showChoiceDialog(app.mainWindow,
			app.messages["github_update_title"],
			fmt.Sprintf(app.messages["github_update_message"], release.TagName),
			app.messages["yes_open_github"],
			app.messages["btn_cancel"],
		)
		if choice == 0 {
			u, _ := url.Parse(release.HTMLURL)
			app.myApp.OpenURL(u)
		}
	} else {
		app.appendLog(app.messages["github_already_latest"])
	}
}
