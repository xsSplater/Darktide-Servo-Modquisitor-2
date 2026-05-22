package main

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/widget"
)

const nexusAPIBase = "https://api.nexusmods.com/v1"
const (
    appName    = "Servo-Modquisitor"
    appVersion = AppVersion   // берётся из config.go
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
    Version string `json:"version"`
}

// FetchNexusModInfo получает информацию о моде по числовому ID.
func (app *App) FetchNexusModInfo(modID int, apiKey string) (*NexusModInfo, error) {
    url := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d.json", nexusAPIBase, modID)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("apikey", apiKey)
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
func (app *App) DownloadFileWithProgress(url, destPath, apiKey string, bar *widget.ProgressBar, win fyne.Window) error {
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("apikey", apiKey)
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

// fetchDirectDownloadLink получает прямую ссылку для скачивания файла через внутренний API Nexus.
func (app *App) fetchDirectDownloadLink(modID, fileID, apiKey string) (string, string, error) {
    apiURL := fmt.Sprintf("https://api.nexusmods.com/v1/games/warhammer40kdarktide/mods/%s/files/%s/download_link.json", modID, fileID)
    req, _ := http.NewRequest("GET", apiURL, nil)
    req.Header.Set("apikey", apiKey)
    req.Header.Set("Application-Name", appName)
    req.Header.Set("Application-Version", appVersion)
    req.Header.Set("Referer", "https://www.nexusmods.com/")
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return "", "", err
    }
    defer resp.Body.Close()
    bodyBytes, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != http.StatusOK {
        return "", "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
    }
    // массив
    var links []struct {
        URI  string `json:"URI"`
        URL  string `json:"url"`
        Name string `json:"file_name"`
    }
    if err := json.Unmarshal(bodyBytes, &links); err == nil && len(links) > 0 {
        for _, l := range links {
            uri := l.URI
            if uri == "" {
                uri = l.URL
            }
            if uri != "" {
                fn := l.Name
                if fn == "" {
                    parts := strings.Split(uri, "/")
                    fn = parts[len(parts)-1]
                }
                if idx := strings.Index(fn, "?"); idx != -1 {
                    fn = fn[:idx]
                }
                return uri, fn, nil
            }
        }
    }
    // одиночный объект
    var single struct {
        URL  string `json:"url"`
        Name string `json:"file_name"`
    }
    if err := json.Unmarshal(bodyBytes, &single); err == nil && single.URL != "" {
        fn := single.Name
        if fn == "" {
            parts := strings.Split(single.URL, "/")
            fn = parts[len(parts)-1]
        }
        if idx := strings.Index(fn, "?"); idx != -1 {
            fn = fn[:idx]
        }
        return single.URL, fn, nil
    }
    return "", "", fmt.Errorf("unexpected response format: %s", string(bodyBytes))
}

// getLatestFileID получает ID самого нового файла мода по числовому ID мода.
func getLatestFileID(modID int, apiKey string) (int, error) {
    url := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d/files.json", nexusAPIBase, modID)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("apikey", apiKey)
    req.Header.Set("Application-Name", appName)
    req.Header.Set("Application-Version", appVersion)
    req.Header.Set("Referer", "https://www.nexusmods.com/")
    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    if resp.StatusCode != http.StatusOK {
        return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
    }
    var files struct {
        Files []struct {
            FileID int `json:"file_id"`
        } `json:"files"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
        return 0, err
    }
    if len(files.Files) == 0 {
        return 0, fmt.Errorf("no files found for mod %d", modID)
    }
    // Последний в ответе обычно самый свежий
    return files.Files[len(files.Files)-1].FileID, nil
}

func (app *App) checkNexusUpdates() {
    if app.cfg.NexusAPIKey == "" {
        app.appendLog(app.messages["nexus_api_key_missing"])
        return
    }
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
        info, err := app.FetchNexusModInfo(modID, app.cfg.NexusAPIKey)
        if err != nil {
            app.appendLog(fmt.Sprintf(app.messages["log_update_check_failed_for"], mod.Name, err))
            continue
        }
        modIDStr := fmt.Sprintf("%d", modID)
        if cached, exists := app.nexusVersionCache[modIDStr]; exists {
            if cached != info.Version {
                app.appendLog(fmt.Sprintf(app.messages["log_update_available"], mod.Name, cached, info.Version))
                updatesFound++
            }
        } else {
            // Первое обнаружение версии (только лог)
            app.appendLog(fmt.Sprintf(app.messages["log_nexus_version_found"], mod.Name, info.Version))
        }
        // app.nexusVersionCache[modIDStr] = info.Version // Пока выключен кэш
    }
    app.saveNexusVersionCache()
    if updatesFound == 0 {
        app.appendLog(app.messages["no_updates_found"])
    } else {
        app.appendLog(fmt.Sprintf(app.messages["updates_found_count"], updatesFound))
    }
}

func (app *App) FetchFileInfo(modID, fileID int, apiKey string) (*FileInfo, error) {
    url := fmt.Sprintf("%s/games/warhammer40kdarktide/mods/%d/files/%d.json",
        nexusAPIBase, modID, fileID)
    req, _ := http.NewRequest("GET", url, nil)
    req.Header.Set("apikey", apiKey)
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
    var info FileInfo
    if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
        return nil, err
    }
    return &info, nil
}
