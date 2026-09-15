// Servo-Modquisitor-2/nexus_api.go
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
//
// Параметр apiKey — это OAuth access token. Раньше функция игнорировала
// его и вызывала app.getAuthToken() внутри (лишний поход в OS keyring,
// который недешёвый: ~1-3 мс на вызов через Win Credential Manager /
// libsecret). Теперь используем то, что передали.
func (app *App) FetchNexusModInfo(modID int, apiKey string) (*NexusModInfo, error) {
	urlStr := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d.json", nexusAPIBase, modID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	var info NexusModInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// DownloadFileWithProgress скачивает файл и обновляет прогресс-бар.
//
// UI обновляется не на каждый прочитанный чанк (32 КБ), а только при
// смене целого процента. При быстрой сети старый код вызвал бы
// fyne.Do сотни раз в секунду и забил UI-очередь. Финальные 100%
// выставляются явно, чтобы троттлинг не оставил прогресс на 99%.
func (app *App) DownloadFileWithProgress(ctx context.Context, url, destPath string, bar *widget.ProgressBar) error {
	// app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: creating request to %s", url))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: NewRequest failed: %v", err))
		return err
	}
	req.Header.Set("User-Agent", СonfigFolderSMQ)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: Timeout30Minutes}
	app.appendLogToFile("DownloadFileWithProgress: sending request")
	resp, err := client.Do(req)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: Do failed: %v", err))
		return err
	}
	defer resp.Body.Close()
	app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: response status %d", resp.StatusCode))

	if resp.StatusCode != http.StatusOK {
		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: bad status %d", resp.StatusCode))
		return fmt.Errorf("HTTP error: %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: content length %d", totalSize))

	out, err := os.Create(destPath)
	if err != nil {
		app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: Create file failed: %v", err))
		return err
	}
	defer out.Close()

	var downloaded int64
	buf := make([]byte, 32*1024)
	lastPercent := -1

loop:
	for {
		select {
		case <-ctx.Done():
			app.appendLogToFile("DownloadFileWithProgress: context cancelled")
			return ctx.Err()
		default:
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, writeErr := out.Write(buf[:n]); writeErr != nil {
					app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: write error: %v", writeErr))
					return writeErr
				}
				downloaded += int64(n)
				if totalSize > 0 {
					percent := int(float64(downloaded) / float64(totalSize) * 100)
					if percent > lastPercent {
						lastPercent = percent
						fyne.Do(func() {
							bar.SetValue(float64(downloaded) / float64(totalSize))
						})
					}
				}
			}
			if err == io.EOF {
				app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: EOF reached, downloaded %d bytes", downloaded))
				break loop
			}
			if err != nil {
				app.appendLogToFile(fmt.Sprintf("DownloadFileWithProgress: read error: %v", err))
				return err
			}
		}
	}
	// Гарантируем 100% даже если последний шаг прогресса был на 99.x%.
	if totalSize > 0 {
		fyne.Do(func() { bar.SetValue(1.0) })
	}
	app.appendLogToFile("DownloadFileWithProgress: completed successfully")
	return nil
}

func (app *App) checkNexusUpdates() {
	if app.getAuthToken() == "" {
		app.appendLog(app.msg("nexus_api_key_missing"))
		return
	}

	app.appendLog(app.msg("log_checking_updates"))

	// Показываем прогресс
	bar, label, cancelChan, closeDialog := app.showProgressDialog(
		app.msg("update_title"),
		app.msg("collecting_mods_for_update"),
	)
	defer closeDialog()

	totalMods := 0
	processed := 0
	updatesFound := 0

	app.modsMutex.RLock()
	allModsCopy := make([]checks.ModInfo, len(app.allMods))
	copy(allModsCopy, app.allMods)
	app.modsMutex.RUnlock()

	for _, mod := range allModsCopy {
		if mod.URL != "" && !mod.IsSystem {
			totalMods++
		}
	}

	for i := range allModsCopy {
		select {
		case <-cancelChan:
			app.appendLog(app.msg("collecting_mods_cancelled"))
			return
		default:
		}

		mod := &allModsCopy[i]
		if mod.URL == "" || mod.IsSystem {
			continue
		}
		modID := helpers.ExtractModIDFromURL(mod.URL)
		if modID == 0 {
			processed++
			continue
		}
		if app.isSymlinkFolder(mod.Name) {
			app.appendLog(fmt.Sprintf(app.msg("log_skipping_update_check_symlink"), mod.Name))
			processed++
			continue
		}

		fileInfo, err := app.getLatestFileInfoForMod(modID, mod.Name)
		if err != nil {
			app.logNexusError(err, mod.Name)
			processed++
			continue
		}

		cacheKey := fmt.Sprintf("%d:%s", modID, mod.Name)
		app.setLatestVersion(cacheKey, fileInfo.Version)

		saved, exists := app.getCachedVersion(cacheKey)
		if !exists || saved.Source == "manual" || saved.Version == "" {
			processed++
			continue
		}

		if fileInfo.UploadedTimestamp > saved.Timestamp {
			app.appendLog(fmt.Sprintf(app.msg("update_available_x_a_b"), mod.Name, saved.Version, fileInfo.Version))
			updatesFound++
		}

		processed++
		if processed%5 == 0 || processed == totalMods {
			fyne.Do(func() {
				bar.SetValue(float64(processed) / float64(totalMods))
				label.SetText(fmt.Sprintf(app.msg("checked_x_y"), processed, totalMods))
			})
		}
	}

	if updatesFound == 0 {
		app.appendLog(app.msg("no_updates_found"))
	} else {
		app.appendLog(fmt.Sprintf(app.msg("updates_found_count"), updatesFound))
	}

	app.checkSpecialUpdates()
	app.appendLog(app.msg("log_update_check_completed"))

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
	urlStr := fmt.Sprintf(NexusV1Files, modID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
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
	urlStr := fmt.Sprintf(NexusV1Filess, modID, fileID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
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
	urlStr := fmt.Sprintf(NexusV1Files, modID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
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

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
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
		fileNameNorm := normalizeForPattern(f.FileName)
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
				app.versionCache.Delete(oldCacheKey)
				app.saveNexusVersionCache()
				app.appendLogToFile(fmt.Sprintf(app.msg("removed_cache_for"), folderName, existingModID, modID))
				// Далее создадим новую запись с нуля
				existing = nil // чтобы не копировать старую
			} else {
				app.appendLog(fmt.Sprintf(app.msg("mod_already_has_id"), folderName, existingModID, modID))
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
			app.appendLog(fmt.Sprintf(app.msg("save_mod_db_failed"), err))
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
		app.appendLogToFile(fmt.Sprintf(app.msg("log_autosaved_stable_pattern"), folderName, pattern))
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
		app.appendLogToFile(fmt.Sprintf(app.msg("log_failed_to_save_mod_db"), err))
	} else {
		app.appendLog(fmt.Sprintf(app.msg("log_mod_db_updated"), folderName))
		// Забираем свежий список из modDBMap (внутри GetModDBList уже
		// взята блокировка modDBMutex в пакете checks).
		newDB := checks.GetModDBList()
		app.modsMutex.Lock()
		app.modDatabase = newDB
		app.modsMutex.Unlock()
		checks.SetModDatabase(newDB)
		fyne.Do(func() {
			app.refreshModList()
		})
	}
}

// getPremiumDownloadURL - for Premium users (v1, used for nxm links of regular mods)
func (app *App) getPremiumDownloadURL(modID, fileID string) (string, string, error) {
	token := app.getAuthToken()
	if token == "" {
		return "", "", errors.New(app.msg("log_error_prem_download_oauth"))
	}
	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}
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
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}
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
//
// 401/403 обрабатываем отдельно от общего случая: на этой ручке они
// означают истёкшую nxm-ссылку. Раньше проверка 401/403 шла после
// общего if и была мёртвым кодом — пользователь видел сухое
// "API error 401: {...}" вместо понятного сообщения.
func (app *App) getFreeDownloadURL(modID, fileID, key, expires string) (string, string, error) {
	if key == "" || expires == "" {
		return "", "", fmt.Errorf("missing key or expires in nxm link")
	}

	urlStr := fmt.Sprintf(NexusV1DownLink, modID, fileID) +
		"?key=" + url.QueryEscape(key) +
		"&expires=" + url.QueryEscape(expires)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", "", fmt.Errorf("build request: %w", err)
	}

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

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read body: %w", err)
	}
	snippet := string(respBody)
	if len(snippet) > 500 {
		snippet = snippet[:500] + "..."
	}

	switch resp.StatusCode {
	case http.StatusOK:
		// продолжаем ниже
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", "", fmt.Errorf(
			"download link expired or invalid (HTTP %d). Please generate a new link on Nexus.",
			resp.StatusCode)
	default:
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

// FetchChangelog получает список изменений для конкретного файла мода
func (app *App) FetchChangelog(modID int, fileID int) (string, error) {
	if modID == 0 || fileID == 0 {
		return "", fmt.Errorf("invalid modID or fileID: %d, %d", modID, fileID)
	}
	token := app.getAuthToken()
	if token == "" {
		return "", fmt.Errorf("no authentication token")
	}
	urlStr := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d/files/%d.json", nexusAPIBase, modID, fileID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	req.Header.Set("Referer", NexusMainURL)

	client := &http.Client{Timeout: Timeout10Seconds}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var result struct {
		ChangelogHTML string `json:"changelog_html"`
		Changelog     string `json:"changelog"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.ChangelogHTML != "" {
		return result.ChangelogHTML, nil
	}
	return result.Changelog, nil
}

// getOldestFileInfo возвращает информацию о самом старом файле мода (по дате загрузки).
func (app *App) getOldestFileInfo(modID int) (*FileInfo, error) {
	token := app.getAuthToken()
	if token == "" {
		return nil, fmt.Errorf("no authentication token")
	}
	urlStr := fmt.Sprintf(NexusV1Files, modID)
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
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
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
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
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if len(result.Files) == 0 {
		return nil, fmt.Errorf("no files found for mod %d", modID)
	}
	oldest := result.Files[0]
	for _, f := range result.Files {
		if f.UploadedTimestamp < oldest.UploadedTimestamp {
			oldest = f
		}
	}
	return &FileInfo{
		ID:                oldest.FileID,
		Version:           oldest.Version,
		UploadedTimestamp: oldest.UploadedTimestamp,
		FileName:          oldest.FileName,
	}, nil
}
