// nexus_api.go
package main

import (
	"Servo-Modquisitor/checks"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
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
	Version           string
	UploadedTimestamp int64
	FileName          string
}

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

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

	app.appendLog(app.messages["log_checking_updates"])
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

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			// Проверяем, не является ли ошибка 403 (мод недоступен)
			errMsg := err.Error()
			if strings.Contains(errMsg, "403") || strings.Contains(errMsg, "Mod not available") {
				app.appendLog(fmt.Sprintf(app.messages["log_error_mod_not_available"], mod.Name))
			} else {
				app.appendLog(fmt.Sprintf(app.messages["log_update_check_failed_for"], mod.Name, err))
			}
			continue
		}

		modIDStr := fmt.Sprintf("%d", modID)
		cacheKey := modIDStr + ":" + mod.Name
		var saved ModVersionInfo
		if info, exists := app.nexusVersionCache[cacheKey]; exists {
			saved = info
		}

		// Если нет сохранённой информации (первый запуск) - сохраняем текущую и не считаем обновлением
		if saved.Timestamp == 0 {
			app.nexusVersionCache[cacheKey] = ModVersionInfo{
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
			app.appendLog(fmt.Sprintf(app.messages["log_mods_checked_progress"], processed, total))
		}
	}

	if updatesFound == 0 {
		app.appendLog(app.messages["no_updates_found"])
	} else {
		app.appendLog(fmt.Sprintf(app.messages["updates_found_count"], updatesFound))
	}
	app.appendLog(app.messages["log_update_check_completed"])
}

// getPremiumDownloadURL - для Premium-пользователей (key и expires не нужны).
// Требует обязательный OAuth-токен (Bearer) или API-ключ (apikey).
func (app *App) getPremiumDownloadURL(modID, fileID string) (string, string, error) {
	token := app.getAuthToken()
	if token == "" {
		return "", "", errors.New(app.messages["log_error_prem_download_oauth"])
	}

	// Принудительное обновление OAuth-токена, если истёк
	if app.cfg.OAuthRefreshToken != "" && time.Now().After(app.cfg.OAuthExpiry) {
		if err := app.refreshAccessToken(); err != nil {
			return "", "", fmt.Errorf(app.messages["log_error_token_expired"], err)
		}
		token = app.getAuthToken()
	}

	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID)
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
	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID)

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
	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
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

	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
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

func (app *App) getFileInfoByFolderPattern(modID int, folderName string) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}

	url := fmt.Sprintf(NexusV1Files, modID)
	req, _ := http.NewRequest("GET", url, nil)
	if app.cfg.OAuthAccessToken != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	} else {
		req.Header.Set("apikey", token)
	}
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
	normalizedPattern := strings.ToLower(strings.ReplaceAll(pattern, " ", "_"))

	// Сортируем файлы по убыванию даты (новые первыми)
	files := result.Files
	sort.Slice(files, func(i, j int) bool {
		return files[i].UploadedTimestamp > files[j].UploadedTimestamp
	})

	for _, f := range files {
		fileNameLower := strings.ToLower(strings.ReplaceAll(f.FileName, " ", "_"))
		if strings.Contains(fileNameLower, normalizedPattern) {
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
	// Проверяем, есть ли уже запись в памяти
	existing := checks.GetModDBEntry(folderName)
	needUpdate := existing == nil
	if existing != nil {
		// Проверяем, каких полей не хватает
		if existing.Name == nil || checks.PickLocalized(existing.Name, "en") == "" ||
			existing.Description == nil || checks.PickLocalized(existing.Description, "en") == "" ||
			existing.Author == "" {
			needUpdate = true
		}
	}
	if !needUpdate {
		return // всё уже есть
	}

	// Получаем данные с Nexus
	info, err := app.FetchNexusModInfo(modID, app.getAuthToken())
	if err != nil {
		app.appendLog(fmt.Sprintf("Auto-add to database failed for %s: %v", folderName, err))
		return
	}

	// Создаём или обновляем запись
	var entry checks.ModDBEntry
	if existing != nil {
		entry = *existing // копируем существующие данные
	} else {
		entry = checks.ModDBEntry{
			Folder: folderName,
			URL:    fmt.Sprintf("https://www.nexusmods.com/warhammer40kdarktide/mods/%d", modID),
		}
	}
	// Если передан fileName и в базе ещё нет nexus_file_pattern — заполняем
	if len(fileName) > 0 && fileName[0] != "" {
		if entry.NexusFilePattern == "" {
			// Вместо полного имени файла, сохраняем обрезанный стабильный паттерн
			pattern := makeStablePattern(fileName[0], modID)
			entry.NexusFilePattern = pattern
			app.appendLog(fmt.Sprintf("Auto-saved stable file pattern for %s: %s", folderName, pattern))
		}
	}

	// Заполняем переводами
	if entry.Name == nil {
		entry.Name = make(map[string]string)
	}
	if entry.Description == nil {
		entry.Description = make(map[string]string)
	}
	if entry.Note == nil {
		entry.Note = make(map[string]string)
	}
	// Устанавливаем английские значения только если они пустые
	if entry.Name["en"] == "" {
		entry.Name["en"] = info.Name
	}
	if entry.Description["en"] == "" {
		entry.Description["en"] = cleanDescription(info.Summary)
	}
	if entry.Author == "" {
		entry.Author = info.Author
	}
	// Гарантируем наличие ключа "ru" (пустого)
	if entry.Name["ru"] == "" {
		entry.Name["ru"] = ""
	}
	if entry.Description["ru"] == "" {
		entry.Description["ru"] = ""
	}
	// Гарантируем наличие ключа "en"
	if _, ok := entry.Note["en"]; !ok {
		entry.Note["en"] = ""
	}
	// Гарантируем наличие ключа "ru"
	if _, ok := entry.Note["ru"]; !ok {
		entry.Note["ru"] = ""
	}

	// Обновляем память и сохраняем
	checks.UpdateModDBEntry(entry)
	if err := checks.SaveModDatabase(); err != nil {
		app.appendLog(fmt.Sprintf("Failed to save mod database: %v", err))
	} else {
		app.appendLog(fmt.Sprintf("Mod database updated for %s", folderName))
		// Перечитываем базу, чтобы UI увидел изменения
		app.modDatabase = checks.GetModDBList()
		checks.SetModDatabase(app.modDatabase)
		app.refreshModList()
	}
}
