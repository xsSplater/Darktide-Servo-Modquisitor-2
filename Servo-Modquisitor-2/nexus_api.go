// nexus_api.go
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

const nexusAPIBase = "https://api.nexusmods.com/v1"
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
	Version           string
	UploadedTimestamp int64
}

// FetchNexusModInfo получает информацию о моде по числовому ID.
func (app *App) FetchNexusModInfo(modID int, apiKey string) (*NexusModInfo, error) {
	url := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d.json", nexusAPIBase, modID)
	req, _ := http.NewRequest("GET", url, nil)
	// Если apiKey похож на OAuth-токен (длинный или содержит точки), используем Bearer,
	// иначе считаем, что это старый API-ключ.
	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	} else {
		req.Header.Set("apikey", apiKey)
	}
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")
	client := &http.Client{Timeout: 10 * time.Second}
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

// extractModIDFromURL пытается извлечь числовой ID из ссылки Nexus Mods.
func extractModIDFromURL(rawURL string) int {
	parts := strings.Split(strings.TrimRight(rawURL, "/"), "/")
	if len(parts) < 2 {
		return 0
	}
	id, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return id
}

// DownloadFileWithProgress скачивает файл и обновляет прогресс‑бар.
func (app *App) DownloadFileWithProgress(url, destPath string, bar *widget.ProgressBar) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Servo-Modquisitor")
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	totalSize := resp.ContentLength
	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			out.Write(buf[:n])
			downloaded += int64(n)
			if totalSize > 0 {
				fyne.Do(func() {
					bar.SetValue(float64(downloaded) / float64(totalSize))
				})
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (app *App) checkNexusUpdates() {
	if app.getAuthToken() == "" {
		app.appendLog(app.messages["nexus_api_key_missing"])
		return
	}

	app.appendLog("🔍 Checking for updates...")
	total := len(app.allMods)
	processed := 0
	updatesFound := 0

	for i := range app.allMods {
		mod := &app.allMods[i]
		if mod.URL == "" {
			continue
		}
		modID := extractModIDFromURL(mod.URL)
		if modID == 0 {
			continue
		}

		fileInfo, err := app.getLatestFileInfo(modID)
		if err != nil {
			// Проверяем, не является ли ошибка 403 (мод недоступен)
			errMsg := err.Error()
			if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Mod not available") {
				app.appendLog(fmt.Sprintf("⚠️ Mod %s is not available on Nexus (removed or hidden). Skipping.", mod.Name))
			} else {
				app.appendLog(fmt.Sprintf(app.messages["log_update_check_failed_for"], mod.Name, err))
			}
			continue
		}

		modIDStr := fmt.Sprintf("%d", modID)
		var saved ModVersionInfo
		if info, exists := app.nexusVersionCache[modIDStr]; exists {
			saved = info
		}

		// Если нет сохранённой информации (первый запуск) - сохраняем текущую и не считаем обновлением
		if saved.Timestamp == 0 {
			app.nexusVersionCache[modIDStr] = ModVersionInfo{
				Timestamp: fileInfo.UploadedTimestamp,
				Version:   fileInfo.Version,
				Folder:    mod.Name,
			}
			app.saveNexusVersionCache()
			processed++
			continue
		}

		if fileInfo.UploadedTimestamp > saved.Timestamp {
			app.appendLog(fmt.Sprintf(app.messages["log_update_available"], mod.Name, saved.Version, fileInfo.Version))
			updatesFound++
		}

		processed++
		// Логируем прогресс каждые 10 модов (не чаще, чтобы не засорять лог)
		if processed%10 == 0 {
			app.appendLog(fmt.Sprintf("  Progress: %d of %d mods checked", processed, total))
		}
	}

	if updatesFound == 0 {
		app.appendLog(app.messages["no_updates_found"])
	} else {
		app.appendLog(fmt.Sprintf(app.messages["updates_found_count"], updatesFound))
	}
	app.appendLog("✅ Update check completed")
}

// getPremiumDownloadURL - для Premium-пользователей (key и expires не нужны).
// Требует обязательный OAuth-токен (Bearer) или API-ключ (apikey).
func (app *App) getPremiumDownloadURL(modID, fileID string) (string, string, error) {
	token := app.getAuthToken()
	if token == "" {
		return "", "", fmt.Errorf("Premium download requires authentication. Please log in via OAuth (menu Nexus → Login) or set your API key.")
	}

	// Принудительное обновление OAuth-токена, если истёк
	if app.cfg.OAuthRefreshToken != "" && time.Now().After(app.cfg.OAuthExpiry) {
		if err := app.refreshAccessToken(); err != nil {
			return "", "", fmt.Errorf("OAuth token expired and could not be refreshed: %w. Please re-login.", err)
		}
		token = app.getAuthToken()
	}

	urlStr := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s/download_link.json", modID, fileID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", "", err
	}

	// Авторизация: предпочитаем Bearer, если токен похож на JWT
	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
	}

	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")
	req.Header.Set("User-Agent", "Servo-Modquisitor")

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
	// app.appendLog("Premium download response received (content omitted for privacy)")

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API error %d: %s", resp.StatusCode, snippet)
	}

	// Разбор ответа (массив зеркал)
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

// getFreeDownloadURL - для бесплатных пользователей (требует key и expires).
// Авторизация не обязательна, но если есть токен - добавим.
func (app *App) getFreeDownloadURL(modID, fileID, key, expires string) (string, string, error) {
	urlStr := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s/download_link.json", modID, fileID)

	// Добавляем параметры key и expires
	params := url.Values{}
	params.Set("key", key)
	params.Set("expires", expires)
	urlStr += "?" + params.Encode()

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", "", err
	}

	token := app.getAuthToken()
	if token != "" {
		// Необязательная авторизация (может помочь в некоторых случаях)
		if app.cfg.OAuthAccessToken != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		} else {
			req.Header.Set("apikey", token)
		}
	}

	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")
	req.Header.Set("User-Agent", "Servo-Modquisitor")

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
	// app.appendLog(fmt.Sprintf("Free download response: %s", snippet))

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

// parseDownloadResponse общая обработка ответа от download_link.json
func (app *App) parseDownloadResponse(resp *http.Response, err error) (string, string, error) {
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	snippet := string(respBody)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}
	app.appendLog(fmt.Sprintf("Download API response: %s", snippet))

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("API error %d: %s", resp.StatusCode, snippet)
	}

	var mirrors []struct {
		Name      string `json:"name"`
		ShortName string `json:"short_name"`
		URI       string `json:"URI"`
	}
	if err := json.Unmarshal(respBody, &mirrors); err != nil || len(mirrors) == 0 {
		var single struct {
			Name      string `json:"name"`
			ShortName string `json:"short_name"`
			URI       string `json:"URI"`
		}
		if err2 := json.Unmarshal(respBody, &single); err2 == nil && single.URI != "" {
			mirrors = append(mirrors, single)
		} else {
			return "", "", fmt.Errorf("unexpected response format: %s", snippet)
		}
	}
	downloadURL := mirrors[0].URI
	fileName := extractFileNameFromURL(downloadURL)
	return downloadURL, fileName, nil
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

// getLatestFileID возвращает ID самого свежего файла мода через REST API
func (app *App) getLatestFileID(modID int) (int, error) {
	token := app.getAuthToken()
	if token == "" {
		return 0, fmt.Errorf("no authentication token")
	}

	url := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%d/files.json", modID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
	}
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Files []struct {
			FileID int `json:"file_id"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}
	if len(result.Files) == 0 {
		return 0, fmt.Errorf("no files found for mod %d", modID)
	}
	// Файлы приходят в порядке убывания даты (новые первыми)
	return result.Files[0].FileID, nil
}

func (app *App) getLatestFileInfo(modID int) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}

	url := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%d/files.json", modID)
	req, _ := http.NewRequest("GET", url, nil)
	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
	}
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")

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
	}, nil
}

// getFileInfoByID получает информацию о конкретном файле (версию, timestamp) по его ID
func (app *App) getFileInfoByID(modID, fileID string) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}

	url := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s.json", modID, fileID)
	req, _ := http.NewRequest("GET", url, nil)

	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
	}
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", "https://www.nexusmods.com/")

	client := &http.Client{Timeout: 10 * time.Second}
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
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return &FileInfo{
		ID:                result.FileID,
		Version:           result.Version,
		UploadedTimestamp: result.UploadedTimestamp,
	}, nil
}
