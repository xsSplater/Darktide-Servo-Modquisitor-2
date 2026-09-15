//go:build linux

// Servo-Modquisitor-2/launcher_linux.go

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────
// VDF: токенизатор и парсер
// ─────────────────────────────────────────────────────────────────────

// vdfValue — один токен VDF. quoted == true для содержимого кавычек
// (ключи и строковые значения), false для `{`, `}` и (на всякий) для
// неожиданных безкавычечных значений.
type vdfValue struct {
	val    string
	quoted bool
}

// tokenizeVDF читает поток и режет его на токены. Комментарии `//`
// пропускаются. Пробелы вне кавычек игнорируются, внутри — часть строки.
func tokenizeVDF(r io.Reader) ([]vdfValue, error) {
	br := bufio.NewReader(r)
	var tokens []vdfValue
	var sb strings.Builder
	inQuotes := false

	flushQuoted := func() {
		if sb.Len() > 0 {
			tokens = append(tokens, vdfValue{val: sb.String(), quoted: true})
			sb.Reset()
		}
	}
	flushBare := func() {
		if sb.Len() > 0 {
			tokens = append(tokens, vdfValue{val: sb.String(), quoted: false})
			sb.Reset()
		}
	}

	for {
		ch, _, err := br.ReadRune()
		if err == io.EOF {
			if inQuotes {
				flushQuoted()
			} else {
				flushBare()
			}
			break
		}
		if err != nil {
			return nil, err
		}

		switch ch {
		case '"':
			if inQuotes {
				flushQuoted()
				inQuotes = false
			} else {
				flushBare()
				inQuotes = true
			}
		case '{', '}':
			if inQuotes {
				sb.WriteRune(ch)
			} else {
				flushBare()
				tokens = append(tokens, vdfValue{val: string(ch), quoted: false})
			}
		case ' ', '\t', '\r', '\n':
			if inQuotes {
				sb.WriteRune(ch)
			} else {
				flushBare()
			}
		case '/':
			if inQuotes {
				sb.WriteRune(ch)
			} else {
				next, _ := br.Peek(1)
				if len(next) > 0 && next[0] == '/' {
					// Комментарий до конца строки.
					if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
						return nil, err
					}
				} else {
					sb.WriteRune(ch)
				}
			}
		default:
			sb.WriteRune(ch)
		}
	}
	return tokens, nil
}

// parseVDFTokens рекурсивно разбирает поток токенов в дерево map'ов.
func parseVDFTokens(tokens []vdfValue, idx int) (map[string]interface{}, int, error) {
	result := make(map[string]interface{})
	for idx < len(tokens) {
		tok := tokens[idx]
		if !tok.quoted && tok.val == "}" {
			return result, idx + 1, nil
		}
		if !tok.quoted {
			// Неожиданный безкавычечный токен на месте ключа.
			idx++
			continue
		}

		key := tok.val
		idx++
		if idx >= len(tokens) {
			return result, idx, nil
		}

		valTok := tokens[idx]
		if !valTok.quoted && valTok.val == "{" {
			sub, newIdx, err := parseVDFTokens(tokens, idx+1)
			if err != nil {
				return nil, idx, err
			}
			result[key] = sub
			idx = newIdx
		} else {
			result[key] = valTok.val
			idx++
		}
	}
	return result, idx, nil
}

// parseVDF парсит VDF из reader'а.
func parseVDF(r io.Reader) (map[string]interface{}, error) {
	tokens, err := tokenizeVDF(r)
	if err != nil {
		return nil, err
	}
	obj, _, err := parseVDFTokens(tokens, 0)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// parseVDFFile открывает файл и парсит его как VDF.
func parseVDFFile(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseVDF(f)
}

// ─────────────────────────────────────────────────────────────────────
// Утилиты доступа к дереву
// ─────────────────────────────────────────────────────────────────────

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	if sub, ok := m[key].(map[string]interface{}); ok {
		return sub
	}
	return nil
}

func getString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if s, ok := m[key].(string); ok {
		return s
	}
	return ""
}

func getKeys(m map[string]interface{}) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// ─────────────────────────────────────────────────────────────────────
// Поиск корня Steam и библиотек
// ─────────────────────────────────────────────────────────────────────

// findSteamRoot ищет корень Steam среди стандартных путей .deb / .rpm /
// Flatpak / Snap. Проверяет наличие config/config.vdf, чтобы отсеять
// пустые/устаревшие каталоги.
func findSteamRoot() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".steam", "root"),
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".steam", "steam"),
		filepath.Join(home, "snap", "steam", "common", ".local", "share", "Steam"),
		filepath.Join(home, "Steam"),
		"/usr/share/steam",
		"/usr/local/share/steam",
	}
	for _, p := range candidates {
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(p, "config", "config.vdf")); err == nil {
			return p
		}
	}
	return ""
}

// getSteamLibraries возвращает список путей к steamapps: основной плюс
// все дополнительные из libraryfolders.vdf. Поддерживает оба формата
// файла и оба варианта регистра ключа.
func getSteamLibraries(steamRoot string) []string {
	primary := filepath.Join(steamRoot, "steamapps")
	libs := []string{primary}

	lfPath := filepath.Join(primary, "libraryfolders.vdf")
	doc, err := parseVDFFile(lfPath)
	if err != nil {
		return libs
	}

	// Новый Steam: "libraryfolders", старый: "LibraryFolders".
	lf := getMap(doc, "libraryfolders")
	if lf == nil {
		lf = getMap(doc, "LibraryFolders")
	}
	if lf == nil {
		return libs
	}

	seen := map[string]bool{primary: true}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		libs = append(libs, p)
	}

	for _, key := range getKeys(lf) {
		// Старый формат: "1" "D:\\Steam"
		if s, ok := lf[key].(string); ok {
			add(filepath.Join(s, "steamapps"))
			continue
		}
		// Новый формат: "0" { "path" "..." ... }
		if entry := getMap(lf, key); entry != nil {
			if path := getString(entry, "path"); path != "" {
				add(filepath.Join(path, "steamapps"))
			}
		}
	}

	return libs
}

// ─────────────────────────────────────────────────────────────────────
// Определение выбранного Proton
// ─────────────────────────────────────────────────────────────────────

// getCompatToolName читает config.vdf и возвращает ID Proton для
// Darktide (или, если не задан — ID, назначенный на «0» = default для
// всех игр). Возвращает "" если ничего не найдено.
func getCompatToolName(config map[string]interface{}) string {
	software := getMap(config, "Software")
	if software == nil {
		return ""
	}
	valve := getMap(software, "Valve")
	if valve == nil {
		return ""
	}
	steam := getMap(valve, "Steam")
	if steam == nil {
		return ""
	}
	mapping := getMap(steam, "CompatToolMapping")
	if mapping == nil {
		return ""
	}

	if entry := getMap(mapping, DarktideAppID); entry != nil {
		if name := getString(entry, "name"); name != "" {
			return name
		}
	}
	if entry := getMap(mapping, "0"); entry != nil {
		if name := getString(entry, "name"); name != "" {
			return name
		}
	}
	return ""
}

// collectCompatManifests собирает пути ко всем compatibilitytool.vdf,
// которые удалось найти:
//   - <lib>/steamapps/common/<ProtonFolder>/compatibilitytool.vdf
//   - <steamRoot>/compatibilitytools.d/<CustomFolder>/compatibilitytool.vdf
//   - ~/.steam/steam/compatibilitytools.d/...
//   - ~/.local/share/Steam/compatibilitytools.d/...
//   - ~/.var/app/com.valvesoftware.Steam/... (Flatpak)
func collectCompatManifests(steamRoot string, libs []string) []string {
	seen := map[string]bool{}
	var result []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	// Манифесты в библиотеках.
	for _, lib := range libs {
		common := filepath.Join(lib, "common")
		entries, err := os.ReadDir(common)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			m := filepath.Join(common, e.Name(), "compatibilitytool.vdf")
			if _, err := os.Stat(m); err == nil {
				add(m)
			}
		}
	}

	// Кастомные Proton'ы в compatibilitytools.d.
	home, _ := os.UserHomeDir()
	customDirs := []string{
		filepath.Join(steamRoot, "compatibilitytools.d"),
		filepath.Join(home, ".steam", "steam", "compatibilitytools.d"),
		filepath.Join(home, ".local", "share", "Steam", "compatibilitytools.d"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam", "compatibilitytools.d"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".steam", "steam", "compatibilitytools.d"),
	}
	for _, dir := range customDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			m := filepath.Join(dir, e.Name(), "compatibilitytool.vdf")
			if _, err := os.Stat(m); err == nil {
				add(m)
			}
		}
	}
	return result
}

// parseCompatManifest читает compatibilitytool.vdf и, если внутри
// есть запись с именем toolName, возвращает путь к proton-бинарнику.
func parseCompatManifest(manifestPath, toolName string) string {
	doc, err := parseVDFFile(manifestPath)
	if err != nil {
		return ""
	}
	compatTools := getMap(doc, "compat_tools")
	if compatTools == nil {
		return ""
	}
	entry := getMap(compatTools, toolName)
	if entry == nil {
		return ""
	}
	installPath := getString(entry, "install_path")
	if installPath == "" {
		return ""
	}

	manifestDir := filepath.Dir(manifestPath)
	var resolved string
	switch {
	case installPath == "." || installPath == "":
		resolved = manifestDir
	case filepath.IsAbs(installPath):
		resolved = installPath
	default:
		resolved = filepath.Join(manifestDir, installPath)
	}

	proton := filepath.Join(resolved, "proton")
	if _, err := os.Stat(proton); err == nil {
		return proton
	}
	return ""
}

// resolveProtonPath возвращает путь к `proton` для Darktide. Если
// config.vdf не содержит записи, берётся proton_experimental (Steam
// выбирает его по умолчанию при первом запуске).
func resolveProtonPath(steamRoot string, libs []string) (string, error) {
	toolName := ""
	if cfg, err := parseVDFFile(filepath.Join(steamRoot, "config", "config.vdf")); err == nil {
		toolName = getCompatToolName(cfg)
	}
	if toolName == "" {
		toolName = "proton_experimental"
	}

	for _, m := range collectCompatManifests(steamRoot, libs) {
		if p := parseCompatManifest(m, toolName); p != "" {
			return p, nil
		}
	}

	// Последний шанс — если config.vdf ссылается на tool, которого
	// нет, пробуем любую папку Proton в common. Это спасает от случая,
	// когда Steam записал «proton_9» а реально установлен только
	// «Proton - Experimental».
	for _, lib := range libs {
		common := filepath.Join(lib, "common")
		entries, err := os.ReadDir(common)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, "Proton") && !strings.HasPrefix(name, "GE-Proton") {
				continue
			}
			proton := filepath.Join(common, name, "proton")
			if _, err := os.Stat(proton); err == nil {
				return proton, nil
			}
		}
	}

	return "", fmt.Errorf("Proton '%s' not found", toolName)
}

// findCompatDataPath находит путь к compatdata/<appID>. Если папка ещё
// не создана, возвращает путь в первой библиотеке — Proton создаст её
// при первом запуске.
func findCompatDataPath(libs []string, appID string) string {
	for _, lib := range libs {
		p := filepath.Join(lib, "compatdata", appID)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if len(libs) > 0 {
		return filepath.Join(libs[0], "compatdata", appID)
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────
// Запуск
// ─────────────────────────────────────────────────────────────────────

func launchGame(version GameVersion, gameRoot string, skipLauncher bool) error {
	switch version {
	case VersionSteam:
		if !skipLauncher {
			return exec.Command("xdg-open", "steam://rungameid/"+DarktideAppID).Start()
		}

		steamRoot := findSteamRoot()
		if steamRoot == "" {
			return fmt.Errorf("Steam root not found")
		}

		libs := getSteamLibraries(steamRoot)
		protonPath, err := resolveProtonPath(steamRoot, libs)
		if err != nil {
			// Старый текст ошибки про Wine — оставлен для обратной
			// совместимости с SetLinuxLauncherMessages.
			return fmt.Errorf("%s: %w", errWineNotFound, err)
		}

		exePath := filepath.Join(gameRoot, "binaries", "Darktide.exe")
		if _, err := os.Stat(exePath); os.IsNotExist(err) {
			return fmt.Errorf("%s: %s", errDarktideExeNotFound, exePath)
		}

		// steam_appid.txt нужен старому Steam API для корректной
		// инициализации внутри wine. Steam и сам его кладёт при
		// запуске через клиент, но при Quick Launch мы идём мимо.
		steamAppIDPath := filepath.Join(gameRoot, "binaries", "steam_appid.txt")
		if err := os.WriteFile(steamAppIDPath, []byte(DarktideAppID), 0644); err != nil {
			return fmt.Errorf("cannot write steam_appid.txt: %w", err)
		}

		compatData := findCompatDataPath(libs, DarktideAppID)
		if compatData == "" {
			return fmt.Errorf("compatdata path not found")
		}
		if err := os.MkdirAll(compatData, 0755); err != nil {
			return fmt.Errorf("cannot create compatdata dir: %w", err)
		}

		// Proton — это обёртка, которой нужна подкоманда `run` и
		// два обязательных пути. Без них он не запустит exe.
		args := []string{
			"run",
			exePath,
			"--bundle-dir", "../bundle",
			"--ini", "settings",
			"--backend-auth-service-url", "https://bsp-auth-prod.atoma.cloud",
			"--backend-title-service-url", "https://bsp-td-prod.atoma.cloud",
			"--lua-heap-mb-size", "2048",
		}

		cmd := exec.Command(protonPath, args...)
		cmd.Dir = filepath.Dir(exePath)
		cmd.Env = append(os.Environ(),
			"STEAM_COMPAT_DATA_PATH="+compatData,
			"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamRoot,
			"SteamAppId="+DarktideAppID,
			"SteamGameId="+DarktideAppID,
			"SteamPath="+steamRoot,
		)
		return cmd.Start()

	case VersionXbox:
		return fmt.Errorf("%s", errXboxOnLinux)

	default:
		return fmt.Errorf("%s", errGameVersionUnknown)
	}
}
