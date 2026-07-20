// nexus_api.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const (
	appName    = AppName
	appVersion = AppVersion
)

type NexusModInfo struct {
	Name         string `json:"name"`
	Summary      string `json:"summary"`
	Author       string `json:"author"`
	Version      string `json:"version"`
	Downloads    int    `json:"downloads"`
	Endorsements int    `json:"endorsements"`
	PictureURL   string `json:"picture_url"`
}

type FileInfo struct {
	ID                int
	FileUUID          string // UUID файла (Group ID) для v3
	Version           string
	UploadedTimestamp int64
	FileName          string
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// FetchNexusModInfo получает информацию о моде по числовому ID.
func (app *App) FetchNexusModInfo(modID int, apiKey string) (*NexusModInfo, error) {
	url := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d.json", nexusAPIBase, modID)
	req, _ := http.NewRequest("GET", url, nil)
	token := app.getAuthToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)
	client := &http.Client{Timeout: Timeout10Seconds}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var info NexusModInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// DownloadFileWithProgress скачивает файл и обновляет прогресс‑бар.
func (app *App) DownloadFileWithProgress(ctx context.Context, url, destPath string, bar *widget.ProgressBar) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", СonfigFolderSMQ)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: Timeout30Minutes}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	var lastLoggedPercent int = -1

loop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				_, writeErr := out.Write(buf[:n])
				if writeErr != nil {
					return writeErr
				}
				downloaded += int64(n)
				if totalSize > 0 {
					percent := int(float64(downloaded) / float64(totalSize) * 100)
					if percent > lastLoggedPercent {
						lastLoggedPercent = percent
					}
					fyne.Do(func() {
						bar.SetValue(float64(downloaded) / float64(totalSize))
					})
				}
			}
			if err == io.EOF {
				break loop
			}
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (app *App) checkNexusUpdates() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}

	app.appendLog(app.messages["log_checking_updates"])
	total := len(app.allMods)
	processed := 0
	updatesFound := 0

	for i := range app.allMods {
		mod := &app.allMods[i]
		if mod.URL == "" {
			continue
		}
		modID := helpers.ExtractModIDFromURL(mod.URL)
		if modID == 0 {
			continue
		}

		// Проверка на симлинк
		if app.isSymlinkFolder(mod.Name) {
			app.appendLog(fmt.Sprintf(app.messages["log_skipping_update_check_symlink"], mod.Name))
			processed++
			continue
		}

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
			continue
		}

		modIDStr := fmt.Sprintf("%d", modID)
		cacheKey := modIDStr + ":" + mod.Name
		app.setLatestVersion(cacheKey, fileInfo.Version)

		saved, exists := app.getCachedVersion(cacheKey)
		if !exists || saved.Source == "manual" {
			processed++
			continue
		}

		if fileInfo.UploadedTimestamp > saved.Timestamp {
			app.appendLog(fmt.Sprintf(app.messages["log_update_available"], mod.Name, saved.Version, fileInfo.Version))
			updatesFound++
		}

		processed++
		if processed%10 == 0 {
			app.appendLog(fmt.Sprintf(app.messages["log_mods_checked_progress"], processed, total))
		}
	}

	if updatesFound == 0 {
		app.appendLog(app.messages["no_updates_found"])
	} else {
		app.appendLog(fmt.Sprintf(app.messages["updates_found_count"], updatesFound))
	}

	app.checkSpecialUpdates()
	app.appendLog(app.messages["log_update_check_completed"])

	fyne.Do(func() {
		app.refreshModList()
	})
}

// extractFileNameFromURL извлекает имя файла из последнего сегмента пути URL.
func extractFileNameFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := u.Path
	segments := strings.Split(strings.TrimLeft(path, "/"), "/")
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}

func (app *App) getLatestFileInfo(modID int) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}
	url := fmt.Sprintf(NexusV1Files, modID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token) // токен уже гарантированно не пустой
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: Timeout10Seconds}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Files []struct {
			FileID            int    `json:"file_id"`
			Version           string `json:"version"`
			UploadedTimestamp int64  `json:"uploaded_timestamp"`
			FileName          string `json:"file_name"` // важно!
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Files) == 0 {
		return nil, fmt.Errorf("no files found for mod %d", modID)
	}
	newest := result.Files[0]
	for _, f := range result.Files {
		if f.UploadedTimestamp > newest.UploadedTimestamp {
			newest = f
		}
	}
	return &FileInfo{
		ID:                newest.FileID,
		Version:           newest.Version,
		UploadedTimestamp: newest.UploadedTimestamp,
		FileName:          newest.FileName,
	}, nil
}

// getFileInfoByID получает информацию о конкретном файле (версию, timestamp) по его ID
func (app *App) getFileInfoByID(modID, fileID string) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}
	url := fmt.Sprintf(NexusV1Filess, modID, fileID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: Timeout10Seconds}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		FileID            int    `json:"file_id"`
		Version           string `json:"version"`
		UploadedTimestamp int64  `json:"uploaded_timestamp"`
		FileName          string `json:"file_name"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &FileInfo{
		ID:                result.FileID,
		Version:           result.Version,
		UploadedTimestamp: result.UploadedTimestamp,
		FileName:          result.FileName,
	}, nil
}

func normalizeForPattern(s string) string {
	var b strings.Builder
	for _, ch := range s {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			b.WriteRune(ch)
		}
	}
	return strings.ToLower(b.String())
}

func (app *App) getFileInfoByFolderPattern(modID int, folderName string) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}
	url := fmt.Sprintf(NexusV1Files, modID)
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Files []struct {
			FileID            int    `json:"file_id"`
			Version           string `json:"version"`
			UploadedTimestamp int64  `json:"uploaded_timestamp"`
			FileName          string `json:"file_name"`
			Name              string `json:"name"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Files) == 0 {
		return nil, fmt.Errorf("no files found for mod %d", modID)
	}

	// Определяем шаблон поиска: если в базе есть nexus_file_pattern - используем его, иначе имя папки
	pattern := folderName
	if p := checks.GetNexusFilePattern(folderName); p != "" {
		pattern = p
	}
	normalizedPattern := normalizeForPattern(pattern)

	// Сортируем файлы по убыванию даты (новые первыми)
	files := result.Files
	sort.Slice(files, func(i, j int) bool {
		return files[i].UploadedTimestamp > files[j].UploadedTimestamp
	})

	for _, f := range files {
		fileNameNorm := normalizeForPattern(f.FileName) // <-- нормализуем имя файла
		if strings.Contains(fileNameNorm, normalizedPattern) {
			return &FileInfo{
				ID:                f.FileID,
				Version:           f.Version,
				UploadedTimestamp: f.UploadedTimestamp,
				FileName:          f.FileName,
			}, nil
		}
	}
	return nil, fmt.Errorf("no file matching pattern '%s' for folder '%s'", pattern, folderName)
}

// .
func (app *App) getLatestFileInfoForMod(modID int, folderName string) (*FileInfo, error) {
	return app.getFileInfoByFolderPattern(modID, folderName)
}

func cleanDescription(desc string) string {
	// заменяем различные варианты <br> на перевод строки
	desc = strings.ReplaceAll(desc, "<br />", "\n")
	desc = strings.ReplaceAll(desc, "<br/>", "\n")
	desc = strings.ReplaceAll(desc, "<br>", "\n")
	// удаляем оставшиеся HTML-теги
	desc = htmlTagRe.ReplaceAllString(desc, "")
	return strings.TrimSpace(desc)
}

// autoAddModToDatabase добавляет информацию о моде в базу mod_database.json,
// если её там ещё нет, или дополняет отсутствующие поля.
func (app *App) autoAddModToDatabase(modID int, folderName string, fileName ...string) {
	if modID == 0 || modID > MaxModsID {
		return
	}
	// Игнорируем системные папки
	if folderName == "binaries" || folderName == "bundle" || folderName == "tools" || folderName == "mods" {
		return
	}

	existing := checks.GetModDBEntry(folderName)

	// Проверяем, не привязан ли уже этот мод к другому ID
	if existing != nil && existing.URL != "" {
		existingModID := helpers.ExtractModIDFromURL(existing.URL)
		if existingModID != 0 && existingModID != modID {
			// Если запись неполная (поля пустые) — разрешаем перезапись, удалив старую запись из кэша
			if existing.Name == nil || checks.PickLocalized(existing.Name, "en") == "" ||
				existing.Description == nil || checks.PickLocalized(existing.Description, "en") == "" ||
				existing.Author == "" {
				// Удаляем старую запись из кэша
				oldCacheKey := fmt.Sprintf("%d:%s", existingModID, folderName)
				app.cacheMutex.Lock()
				delete(app.nexusVersionCache, oldCacheKey)
				app.cacheMutex.Unlock()
				app.saveNexusVersionCache()
				app.appendLog(fmt.Sprintf("ℹ️ Removed stale cache entry for %s (old ID: %d), will save with new ID: %d", folderName, existingModID, modID))
				// Далее создадим новую запись с нуля
				existing = nil // чтобы не копировать старую
			} else {
				app.appendLog(fmt.Sprintf("⚠️ Mod %s already has a different Nexus ID (%d) and is complete, skipping update with ID %d", folderName, existingModID, modID))
				return
			}
		}
	}

	// Определяем, нужно ли обновлять запись
	needUpdate := existing == nil
	if existing != nil {
		// Проверяем наличие обязательных полей
		if existing.Name == nil || checks.PickLocalized(existing.Name, "en") == "" ||
			existing.Description == nil || checks.PickLocalized(existing.Description, "en") == "" ||
			existing.Author == "" {
			needUpdate = true
		}
	}
	if !needUpdate {
		// Запись уже полная, но добавим недостающие языковые ключи (если они отсутствуют)
		ensureAllLanguageKeys(existing)
		if err := checks.SaveModDatabase(); err != nil {
			app.appendLog(fmt.Sprintf("Failed to save mod database (language keys): %v", err))
		}
		return
	}

	// Получаем информацию с Nexus
	info, err := app.FetchNexusModInfo(modID, app.getAuthToken())
	if err != nil {
		app.logNexusError(err, folderName)
		return
	}

	var entry checks.ModDBEntry
	if existing != nil {
		entry = *existing
		// Убедимся, что URL правильный
		entry.URL = fmt.Sprintf(NexusModIDLink, modID)
	} else {
		entry = checks.ModDBEntry{
			Folder: folderName,
			URL:    fmt.Sprintf(NexusModIDLink, modID),
		}
	}

	// Извлекаем паттерн из имени файла, если передан
	pattern := ""
	if len(fileName) > 0 && fileName[0] != "" {
		pattern = extractPatternFromFilename(fileName[0])
	}
	if pattern == "" {
		// fallback: используем первое слово из folderName
		parts := strings.Fields(folderName)
		if len(parts) > 0 {
			pattern = strings.ToLower(parts[0])
		}
	}
	if pattern != "" {
		entry.NexusFilePattern = pattern
		app.appendLog(fmt.Sprintf(app.messages["log_autosaved_stable_pattern"], folderName, pattern))
	}

	// Инициализируем карты, если nil
	if entry.Name == nil {
		entry.Name = make(map[string]string)
	}
	if entry.Description == nil {
		entry.Description = make(map[string]string)
	}
	if entry.Note == nil {
		entry.Note = make(map[string]string)
	}

	// Заполняем английский, если пусто
	if entry.Name["en"] == "" {
		entry.Name["en"] = info.Name
	}
	if entry.Description["en"] == "" {
		entry.Description["en"] = cleanDescription(info.Summary)
	}
	if entry.Author == "" {
		entry.Author = info.Author
	}

	// Гарантируем наличие всех языковых ключей (пустых)
	ensureAllLanguageKeys(&entry)

	// Сохраняем
	checks.UpdateModDBEntry(entry)
	if err := checks.SaveModDatabase(); err != nil {
		app.appendLog(fmt.Sprintf(app.messages["log_failed_to_save_mod_db"], err))
	} else {
		app.appendLog(fmt.Sprintf(app.messages["log_mod_db_updated"], folderName))
		app.modDatabase = checks.GetModDBList()
		checks.SetModDatabase(app.modDatabase)
		fyne.Do(func() {
			app.refreshModList()
		})
	}
}

// getPremiumDownloadURL - for Premium users (v1, used for nxm links of regular mods)
func (app *App) getPremiumDownloadURL(modID, fileID string) (string, string, error) {
	token := app.getAuthToken()
	if token == "" {
		return "", "", errors.New(app.messages["log_error_prem_download_oauth"])
	}
	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID)
	req, _ := http.NewRequest("GET", urlStr, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)
	req.Header.Set("User-Agent", СonfigFolderSMQ)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	snippet := string(respBody)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API error %d: %s", resp.StatusCode, snippet)
	}

	var mirrors []struct {
		URI string `json:"URI"`
	}
	if err := json.Unmarshal(respBody, &mirrors); err != nil || len(mirrors) == 0 {
		return "", "", fmt.Errorf("unexpected response format: %s", snippet)
	}
	downloadURL := mirrors[0].URI
	fileName := extractFileNameFromURL(downloadURL)
	return downloadURL, fileName, nil
}

// getFreeDownloadURL - for FREE users (v1, used for nxm links of regular mods)
func (app *App) getFreeDownloadURL(modID, fileID, key, expires string) (string, string, error) {
	// Checking for the presence of required parameters
	if key == "" || expires == "" {
		return "", "", fmt.Errorf("missing key or expires in nxm link")
	}

	// Forming a URL with the key and expires parameters
	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID) +
		"?key=" + url.QueryEscape(key) +
		"&expires=" + url.QueryEscape(expires)

	req, _ := http.NewRequest("GET", urlStr, nil)

	token := app.getAuthToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)
	req.Header.Set("User-Agent", СonfigFolderSMQ)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	snippet := string(respBody)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API error %d: %s", resp.StatusCode, snippet)
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return "", "", fmt.Errorf("download link expired or invalid (HTTP %d). Please generate a new link on Nexus.", resp.StatusCode)
	}

	var mirrors []struct {
		URI string `json:"URI"`
	}
	if err := json.Unmarshal(respBody, &mirrors); err != nil || len(mirrors) == 0 {
		return "", "", fmt.Errorf("unexpected response format: %s", snippet)
	}
	downloadURL := mirrors[0].URI
	fileName := extractFileNameFromURL(downloadURL)
	return downloadURL, fileName, nil
}

// ensureAllLanguageKeys добавляет отсутствующие языковые ключи в карты entry.Name, entry.Description, entry.Note.
func ensureAllLanguageKeys(entry *checks.ModDBEntry) {
	langKeys := []string{"en", "ru", "de", "es", "fr", "it", "ja", "ko", "pl", "pt-BR", "zh-hans", "zh-hant"}
	if entry.Name == nil {
		entry.Name = make(map[string]string)
	}
	for _, lang := range langKeys {
		if _, ok := entry.Name[lang]; !ok {
			entry.Name[lang] = ""
		}
	}
	if entry.Description == nil {
		entry.Description = make(map[string]string)
	}
	for _, lang := range langKeys {
		if _, ok := entry.Description[lang]; !ok {
			entry.Description[lang] = ""
		}
	}
	if entry.Note == nil {
		entry.Note = make(map[string]string)
	}
	for _, lang := range langKeys {
		if _, ok := entry.Note[lang]; !ok {
			entry.Note[lang] = ""
		}
	}
}
