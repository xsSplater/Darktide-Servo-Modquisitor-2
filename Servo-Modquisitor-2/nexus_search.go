// Servo-Modquisitor-2/nexus_search.go
package main

import (
	"Servo-Modquisitor/checks"
	"Servo-Modquisitor/helpers"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ModBrowserItem — объединенная структура для браузера.
type ModBrowserItem struct {
	IsObsolete      bool
	IsInstalled     bool
	ModID           int
	Downloads       int
	UniqueDownloads int
	Endorsements    int
	Views           int
	Folder          string
	Name            string
	Summary         string
	Author          string
	Category        string
	PictureURL      string
	SourceCodeURL   string
	Version         string
	LatestVersion   string
	OriginalUpload  time.Time
	LastUpdated     time.Time
	Source          string
	Note            string
}

// SearchLocalDatabase ищет моды в локальной базе.
func (app *App) SearchLocalDatabase(query string, lang string, onlyNames bool) ([]ModBrowserItem, bool) {
	var exact []ModBrowserItem
	queryLower := strings.ToLower(strings.TrimSpace(query))
	if queryLower == "" {
		return nil, false
	}

	// Читаем настройку
	app.cfgMutex.RLock()
	forceEn := app.cfg.ForceEnglishModNames
	app.cfgMutex.RUnlock()

	// Язык для названий (может быть "en", если включена галочка)
	nameLang := lang
	if forceEn {
		nameLang = "en"
	}
	// Язык для описаний и заметок — всегда язык интерфейса
	descLang := lang

	app.modsMutex.RLock()
	db := app.modDatabase
	app.modsMutex.RUnlock()

	obsoleteMods := checks.GetObsoleteMods()

	// Шаг 1. Точный поиск
	for _, entry := range db {
		nameEn := strings.ToLower(checks.PickLocalized(entry.Name, "en"))
		nameLoc := strings.ToLower(checks.PickLocalized(entry.Name, nameLang))
		descLoc := strings.ToLower(checks.PickLocalized(entry.Description, descLang))
		authorLoc := strings.ToLower(entry.Author)

		// Ищем строго в названиях
		match := strings.Contains(nameEn, queryLower) || strings.Contains(nameLoc, queryLower)

		// Если галочка "только имена" снята — ищем ещё в авторе и описании
		if !onlyNames && !match {
			match = strings.Contains(authorLoc, queryLower) || strings.Contains(descLoc, queryLower)
		}

		if match {
			exact = append(exact, app.makeBrowserItem(entry, lang, forceEn, obsoleteMods))
		}
	}

	if len(exact) > 0 {
		return exact, false
	}

	// Шаг 2. Fuzzy-поиск
	if len(queryLower) < 4 {
		return nil, false
	}

	type fuzzyHit struct {
		item ModBrowserItem
		dist int
	}
	var hits []fuzzyHit

	for _, entry := range db {
		nameLoc := strings.ToLower(checks.PickLocalized(entry.Name, nameLang))
		nameEn := strings.ToLower(checks.PickLocalized(entry.Name, "en"))

		dist := levenshtein(queryLower, nameLoc)
		if d := levenshtein(queryLower, nameEn); d < dist {
			dist = d
		}

		// Fuzzy-поиск по автору и описанию только если галочка снята
		if !onlyNames {
			authorLoc := strings.ToLower(entry.Author)
			descLoc := strings.ToLower(checks.PickLocalized(entry.Description, descLang))
			if d := levenshtein(queryLower, authorLoc); d < dist {
				dist = d
			}
			if d := levenshtein(queryLower, descLoc); d < dist {
				dist = d
			}
		}

		threshold := len(queryLower) / 2
		if threshold < 2 {
			threshold = 2
		}

		if dist <= threshold {
			hits = append(hits, fuzzyHit{
				item: app.makeBrowserItem(entry, lang, forceEn, obsoleteMods),
				dist: dist,
			})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].dist < hits[j].dist
	})
	if len(hits) > 10 {
		hits = hits[:10]
	}

	var fuzzy []ModBrowserItem
	for _, h := range hits {
		fuzzy = append(fuzzy, h.item)
	}
	return fuzzy, len(fuzzy) > 0
}

func (app *App) makeBrowserItem(entry checks.ModDBEntry, lang string, forceEn bool, obsolete []string) ModBrowserItem {
	modID := helpers.ExtractModIDFromURL(entry.URL)

	isObsolete := false
	for _, obs := range obsolete {
		if strings.EqualFold(obs, entry.Folder) {
			isObsolete = true
			break
		}
	}

	// Определяем язык для названия
	nameLang := lang
	if forceEn {
		nameLang = "en"
	}

	item := ModBrowserItem{
		ModID:         modID,
		Folder:        entry.Folder,
		Name:          checks.PickLocalized(entry.Name, nameLang),    // Название по настройке
		Summary:       checks.PickLocalized(entry.Description, lang), // Описание на языке интерфейса
		Author:        entry.Author,
		Category:      entry.Category,
		PictureURL:    entry.NexusPictureURL,
		SourceCodeURL: entry.SourceCodeURL,
		Note:          checks.PickLocalized(entry.Note, lang), // Заметки на языке интерфейса
		IsObsolete:    isObsolete,
		IsInstalled:   checks.FolderExists(entry.Folder),
	}

	// Ищем установленную версию в кэше nexus_versions.json
	if modID != 0 {
		cacheKey := fmt.Sprintf("%d:%s", modID, entry.Folder)
		if info, ok := app.getCachedVersion(cacheKey); ok {
			item.Version = info.Version
			item.Source = info.Source
		}
	}

	return item
}

// enrichModBrowserItems обогащает данные из Nexus локальными переводами и флагами
func (app *App) enrichModBrowserItems(items []ModBrowserItem) []ModBrowserItem {
	app.modsMutex.RLock()
	db := app.modDatabase
	app.modsMutex.RUnlock()

	app.cfgMutex.RLock()
	lang := app.cfg.Language
	forceEn := app.cfg.ForceEnglishModNames
	app.cfgMutex.RUnlock()

	if forceEn {
		lang = "en"
	}

	dbMap := make(map[int]checks.ModDBEntry)
	for _, entry := range db {
		if entry.URL != "" {
			if id := helpers.ExtractModIDFromURL(entry.URL); id != 0 {
				dbMap[id] = entry
			}
		}
	}

	obsoleteMods := checks.GetObsoleteMods()

	for i := range items {
		if entry, ok := dbMap[items[i].ModID]; ok {
			// Перезаписываем на локализованные данные
			if name := checks.PickLocalized(entry.Name, lang); name != "" {
				items[i].Name = name
			}
			if desc := checks.PickLocalized(entry.Description, lang); desc != "" {
				items[i].Summary = desc
			}
			if cat := entry.Category; cat != "" {
				items[i].Category = cat
			}
			items[i].Folder = entry.Folder
			items[i].SourceCodeURL = entry.SourceCodeURL
			items[i].IsInstalled = checks.FolderExists(entry.Folder)

			for _, obs := range obsoleteMods {
				if strings.EqualFold(obs, entry.Folder) {
					items[i].IsObsolete = true
					break
				}
			}

			// Подтягиваем установленную версию из кеша
			if items[i].IsInstalled {
				cacheKey := fmt.Sprintf("%d:%s", items[i].ModID, entry.Folder)
				if info, ok := app.getCachedVersion(cacheKey); ok {
					items[i].Version = info.Version
					items[i].Source = info.Source
				}
			}
		}
	}
	return items
}

// SearchNexusMods — поиск модов на Nexus через GraphQL v2.
func (app *App) SearchNexusMods(query string, sortBy string, offset int, onlyNames bool) ([]ModBrowserItem, int, error) {
	const (
		apiURL   = "https://api.nexusmods.com/v2/graphql"
		gameDom  = "warhammer40kdarktide"
		pageSize = 30
	)
	_ = onlyNames

	var sortField string
	switch sortBy {
	case "updated":
		sortField = "updatedAt"
	case "endorsements":
		sortField = "endorsements"
	default:
		sortField = "downloads"
	}
	sortParam := []map[string]interface{}{
		{sortField: map[string]interface{}{"direction": "DESC"}},
	}

	filterParam := map[string]interface{}{
		"gameDomainName": []map[string]interface{}{
			{"value": gameDom, "op": "EQUALS"},
		},
	}
	if query != "" {
		filterParam["name"] = []map[string]interface{}{
			{"value": query, "op": "WILDCARD"},
		}
	}

	graphqlQuery := `
query SearchMods(
  $filter: ModsFilter
  $sort: [ModsSort!]
  $offset: Int
  $count: Int
) {
  mods(
    filter: $filter
    sort: $sort
    offset: $offset
    count: $count
    viewUserBlockedContent: false
  ) {
    nodes {
      modId
      name
      summary
      author
      pictureUrl
      modCategory { name }
      downloads
      endorsements
      version
      adultContent
      createdAt
      updatedAt
    }
    nodesCount
    totalCount
  }
}`

	vars := map[string]interface{}{
		"filter": filterParam,
		"sort":   sortParam,
		"offset": offset,
		"count":  pageSize,
	}

	reqBody, err := json.Marshal(map[string]interface{}{
		"query":     graphqlQuery,
		"variables": vars,
	})
	if err != nil {
		return nil, 0, err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Application-Name", appName)
	req.Header.Set("Application-Version", appVersion)
	if token := app.getAuthToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{Timeout: Timeout10Seconds}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("GraphQL HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			Mods struct {
				Nodes []struct {
					ModID        int    `json:"modId"`
					Name         string `json:"name"`
					Summary      string `json:"summary"`
					Author       string `json:"author"`
					PictureURL   string `json:"pictureUrl"`
					Downloads    int    `json:"downloads"`
					Endorsements int    `json:"endorsements"`
					Version      string `json:"version"`
					AdultContent bool   `json:"adultContent"`
					CreatedAt    string `json:"createdAt"`
					UpdatedAt    string `json:"updatedAt"`
					ModCategory  struct {
						Name string `json:"name"`
					} `json:"modCategory"`
				} `json:"nodes"`
				NodesCount int `json:"nodesCount"`
				TotalCount int `json:"totalCount"`
			} `json:"mods"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, 0, err
	}
	if len(result.Errors) > 0 {
		return nil, 0, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	items := make([]ModBrowserItem, 0, len(result.Data.Mods.Nodes))
	for _, n := range result.Data.Mods.Nodes {
		var orig, upd time.Time
		if t, err := time.Parse(time.RFC3339, n.CreatedAt); err == nil {
			orig = t
		}
		if t, err := time.Parse(time.RFC3339, n.UpdatedAt); err == nil {
			upd = t
		}

		items = append(items, ModBrowserItem{
			ModID:          n.ModID,
			Name:           n.Name,
			Summary:        n.Summary,
			Author:         n.Author,
			PictureURL:     n.PictureURL,
			Category:       n.ModCategory.Name,
			Downloads:      n.Downloads,
			Endorsements:   n.Endorsements,
			LatestVersion:  n.Version,
			OriginalUpload: orig,
			LastUpdated:    upd,
		})
	}

	items = app.enrichModBrowserItems(items)
	return items, result.Data.Mods.TotalCount, nil
}

func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(
				cur[j-1]+1,
				prev[j]+1,
				prev[j-1]+cost,
			)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
