// aml.go
//
// Parsing and (careful) rewriting of Darktide ".mod" files for the AML
// ("Auto Mod Loading and Ordering") configuration feature.
//
// A ".mod" file is a small Lua script that returns a table, e.g.:
//
//	return {
//	  run = function() ... end,
//	  load_after  = { "weapon_customization", "for_the_drip" },
//	  load_before = { "some_mod" },
//	  require     = { "weapon_customization" },
//	  version     = "24.07.05",
//	  packages    = {},
//	}
//
// When the AML mod is installed it ignores mod_load_order.txt and instead orders
// mods using the top-level `load_after`, `load_before` and `require` tables read
// straight from these files. This file lets the program read those tables and
// write them back without disturbing the rest of the script (the `run` function,
// `packages`, comments, formatting elsewhere).
//
// The Lua "parser" here is intentionally minimal: it only understands enough to
// find the outer `return { ... }` table and the top-level array/string keys we
// care about. It correctly skips strings and comments so braces inside them are
// never miscounted.
package checks

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// AMLModConfig holds the AML ordering metadata parsed from a single mod's
// ".mod" file.
type AMLModConfig struct {
	Folder      string   // mod folder name
	ModFilePath string   // absolute path to the .mod file (empty if none found)
	LoadAfter   []string // top-level load_after = { ... }
	LoadBefore  []string // top-level load_before = { ... }
	Require     []string // top-level require = { ... }
	Version     string   // top-level version = "..."
	HasConfig   bool     // true if any of the three arrays is non-empty
	ParseErr    string   // non-fatal note: no .mod file / unreadable / unparseable
}

// amlFields are the editable top-level array keys, in display order.
var amlFields = []string{"load_after", "load_before", "require"}

// findModFile locates the ".mod" file for a mod folder. It prefers
// <folder>/<folder>.mod and falls back to the single *.mod file in the folder.
// Returns "" if none (or more than one ambiguous candidate) is found.
func findModFile(folder string) string {
	dir := filepath.Join(modsDir, folder)
	primary := filepath.Join(dir, folder+".mod")
	if fileExists(primary) {
		return primary
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	found := ""
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".mod") {
			if found != "" {
				return "" // ambiguous: more than one .mod file
			}
			found = filepath.Join(dir, e.Name())
		}
	}
	return found
}

// ReadAMLConfig reads and parses the AML metadata for a single mod folder.
// On any problem it returns a config with ParseErr set rather than failing.
func ReadAMLConfig(folder string) AMLModConfig {
	cfg := AMLModConfig{Folder: folder}
	path := findModFile(folder)
	if path == "" {
		cfg.ParseErr = "no_modfile"
		return cfg
	}
	cfg.ModFilePath = path
	data, err := os.ReadFile(path)
	if err != nil {
		cfg.ParseErr = "unreadable"
		return cfg
	}
	content := string(data)

	open, ok := findLuaOuterBrace(content)
	if !ok {
		cfg.ParseErr = "no_table"
		return cfg
	}
	closeIdx, ok := matchLuaBrace(content, open)
	if !ok {
		cfg.ParseErr = "unbalanced"
		return cfg
	}

	cfg.LoadAfter = readArrayKey(content, open, closeIdx, "load_after")
	cfg.LoadBefore = readArrayKey(content, open, closeIdx, "load_before")
	cfg.Require = readArrayKey(content, open, closeIdx, "require")
	cfg.Version = readStringKey(content, open, closeIdx, "version")
	cfg.HasConfig = len(cfg.LoadAfter) > 0 || len(cfg.LoadBefore) > 0 || len(cfg.Require) > 0
	return cfg
}

// ListAMLConfigs returns the AML metadata for every installed mod folder that
// actually has a ".mod" file (system folders such as base/dmf are skipped, as
// they have no .mod file).
func ListAMLConfigs() []AMLModConfig {
	var out []AMLModConfig
	for _, f := range ListModFolders() {
		path := findModFile(f)
		if path == "" {
			continue
		}
		out = append(out, ReadAMLConfig(f))
	}
	return out
}

// WriteAMLConfig writes cfg.LoadAfter / LoadBefore / Require back into the mod's
// ".mod" file, preserving everything else. Before writing it validates that the
// resulting file still has balanced braces, backs the original up to
// "<file>.mod.bak", and writes atomically. The version field is never modified.
func WriteAMLConfig(cfg AMLModConfig) error {
	path := cfg.ModFilePath
	if path == "" {
		path = findModFile(cfg.Folder)
	}
	if path == "" {
		return &amlError{"no_modfile"}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	working := string(data)

	updates := map[string][]string{
		"load_after":  cleanList(cfg.LoadAfter),
		"load_before": cleanList(cfg.LoadBefore),
		"require":     cleanList(cfg.Require),
	}
	// Apply in a fixed order, re-parsing the working string each time so byte
	// offsets stay valid after each edit.
	for _, key := range amlFields {
		working, err = applyArrayKey(working, key, updates[key])
		if err != nil {
			return err
		}
	}

	if !luaBracesBalanced(working) {
		return &amlError{"unbalanced_result"}
	}

	// Back up the original (overwrite any previous backup).
	if err := os.WriteFile(path+".bak", data, 0644); err != nil {
		return err
	}
	// Atomic write: temp file in the same dir, then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(working), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// applyArrayKey replaces (or inserts/removes) the top-level `key = { ... }`
// array in content with items, returning the modified content.
func applyArrayKey(content, key string, items []string) (string, error) {
	open, ok := findLuaOuterBrace(content)
	if !ok {
		return content, &amlError{"no_table"}
	}
	closeIdx, ok := matchLuaBrace(content, open)
	if !ok {
		return content, &amlError{"unbalanced"}
	}

	keyStart, valStart, found := findLuaTopLevelKey(content, open, closeIdx, key)
	serialized := serializeArray(key, items)

	if found {
		if content[valStart] != '{' {
			// Existing value isn't an array literal — leave it alone to be safe.
			return content, nil
		}
		valClose, ok := matchLuaBrace(content, valStart)
		if !ok {
			return content, &amlError{"unbalanced"}
		}
		valEnd := valClose + 1 // just past the closing '}' (any trailing comma stays)
		return content[:keyStart] + serialized + content[valEnd:], nil
	}

	// Not present: only insert when there is something to write.
	if len(items) == 0 {
		return content, nil
	}
	ins := ""
	if closeIdx > 0 && content[closeIdx-1] != '\n' {
		ins = "\n"
	}
	ins += "  " + serialized + ",\n"
	return content[:closeIdx] + ins + content[closeIdx:], nil
}

// serializeArray renders a Lua `key = { ... }` assignment (no leading indent on
// the first line, no trailing comma). An empty list renders as `key = {}`.
func serializeArray(key string, items []string) string {
	if len(items) == 0 {
		return key + " = {}"
	}
	var b strings.Builder
	b.WriteString(key)
	b.WriteString(" = {\n")
	for _, it := range items {
		b.WriteString("    ")
		b.WriteString(strconv.Quote(it)) // ASCII mod names: Lua-compatible quoting
		b.WriteString(",\n")
	}
	b.WriteString("  }")
	return b.String()
}

// cleanList trims entries, drops blanks, and removes duplicates (keeping order).
func cleanList(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// ───────────────────────── minimal Lua scanning ─────────────────────────

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func isLuaSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// skipLuaString assumes content[i] is a quote (" or ') and returns the index
// just past the closing quote.
func skipLuaString(content string, i int) int {
	quote := content[i]
	i++
	for i < len(content) {
		c := content[i]
		if c == '\\' {
			i += 2
			continue
		}
		if c == quote {
			return i + 1
		}
		i++
	}
	return i
}

// luaLongBracketLevel reports whether content[i] starts a long bracket
// ("[[", "[=[", "[==[", ...) and, if so, its level (number of '=').
func luaLongBracketLevel(content string, i int) (level int, ok bool) {
	if i >= len(content) || content[i] != '[' {
		return 0, false
	}
	j := i + 1
	for j < len(content) && content[j] == '=' {
		j++
	}
	if j < len(content) && content[j] == '[' {
		return j - (i + 1), true
	}
	return 0, false
}

// skipLuaLongBracket returns the index just past the matching close of a long
// bracket that opens at content[i] with the given level.
func skipLuaLongBracket(content string, i, level int) int {
	closer := "]" + strings.Repeat("=", level) + "]"
	openLen := 2 + level
	if i+openLen > len(content) {
		return len(content)
	}
	idx := strings.Index(content[i+openLen:], closer)
	if idx < 0 {
		return len(content)
	}
	return i + openLen + idx + len(closer)
}

// skipCommentOrString advances past a comment or string literal starting at i,
// returning the new index and whether anything was skipped. If content[i] does
// not start a comment/string it returns (i, false).
func skipCommentOrString(content string, i int) (int, bool) {
	n := len(content)
	c := content[i]
	if c == '-' && i+1 < n && content[i+1] == '-' {
		if lvl, isLong := luaLongBracketLevel(content, i+2); isLong {
			return skipLuaLongBracket(content, i+2, lvl), true
		}
		nl := strings.IndexByte(content[i:], '\n')
		if nl < 0 {
			return n, true
		}
		return i + nl + 1, true
	}
	if c == '"' || c == '\'' {
		return skipLuaString(content, i), true
	}
	if c == '[' {
		if lvl, isLong := luaLongBracketLevel(content, i); isLong {
			return skipLuaLongBracket(content, i, lvl), true
		}
	}
	return i, false
}

// findLuaOuterBrace returns the index of the outer table's opening '{' (the
// first '{' encountered when skipping comments and strings).
func findLuaOuterBrace(content string) (int, bool) {
	i, n := 0, len(content)
	for i < n {
		if ni, skipped := skipCommentOrString(content, i); skipped {
			i = ni
			continue
		}
		if content[i] == '{' {
			return i, true
		}
		i++
	}
	return 0, false
}

// matchLuaBrace returns the index of the '}' matching the '{' at open.
func matchLuaBrace(content string, open int) (int, bool) {
	depth := 0
	i, n := open, len(content)
	for i < n {
		if ni, skipped := skipCommentOrString(content, i); skipped {
			i = ni
			continue
		}
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
		i++
	}
	return 0, false
}

// findLuaTopLevelKey finds a depth-1 (directly inside the table) `key =` within
// the table whose braces span [open, closeIdx]. It returns the index where the
// key identifier starts and the index where its value begins (first non-space
// char after '='). A `key` appearing inside a nested table or the run function
// is never matched.
func findLuaTopLevelKey(content string, open, closeIdx int, key string) (keyStart, valStart int, found bool) {
	depth := 0 // 0 == directly inside the outer table
	i := open + 1
	for i < closeIdx {
		if ni, skipped := skipCommentOrString(content, i); skipped {
			i = ni
			continue
		}
		c := content[i]
		switch {
		case c == '{':
			depth++
			i++
		case c == '}':
			depth--
			i++
		case depth == 0 && isIdentStart(c):
			j := i
			for j < closeIdx && isIdentChar(content[j]) {
				j++
			}
			ident := content[i:j]
			// A function value (e.g. `run = function() ... end`) is not brace
			// delimited, so skip its whole block — otherwise tokens inside it
			// would be scanned as if they were top-level table keys.
			if ident == "function" {
				i = skipLuaBlock(content, j, closeIdx)
				continue
			}
			k := j
			for k < closeIdx && isLuaSpace(content[k]) {
				k++
			}
			if ident == key && k < closeIdx && content[k] == '=' && (k+1 >= closeIdx || content[k+1] != '=') {
				v := k + 1
				for v < closeIdx && isLuaSpace(content[v]) {
					v++
				}
				return i, v, true
			}
			i = j
		default:
			i++
		}
	}
	return 0, 0, false
}

// skipLuaBlock skips a `function`/`if`/`do`/`repeat` block, returning the index
// just past its matching `end` (or `until`). pos must be the index right after
// the opening keyword that started the block (depth is taken as 1 on entry).
// Strings and comments are skipped so keywords inside them are not counted.
func skipLuaBlock(content string, pos, limit int) int {
	depth := 1
	i := pos
	for i < limit {
		if ni, skipped := skipCommentOrString(content, i); skipped {
			i = ni
			continue
		}
		c := content[i]
		if isIdentStart(c) {
			j := i
			for j < limit && isIdentChar(content[j]) {
				j++
			}
			switch content[i:j] {
			case "function", "if", "do", "repeat":
				// `for`/`while` are not counted: they are always paired with a
				// `do` that opens the block, so counting only `do` keeps balance.
				depth++
			case "end", "until":
				depth--
				if depth == 0 {
					return j
				}
			}
			i = j
			continue
		}
		i++
	}
	return i
}

// readArrayKey returns the string entries of a top-level array key, or nil.
func readArrayKey(content string, open, closeIdx int, key string) []string {
	_, valStart, found := findLuaTopLevelKey(content, open, closeIdx, key)
	if !found || valStart >= len(content) || content[valStart] != '{' {
		return nil
	}
	valClose, ok := matchLuaBrace(content, valStart)
	if !ok {
		return nil
	}
	return extractLuaStrings(content[valStart : valClose+1])
}

// readStringKey returns the value of a top-level string key (e.g. version), or "".
func readStringKey(content string, open, closeIdx int, key string) string {
	_, valStart, found := findLuaTopLevelKey(content, open, closeIdx, key)
	if !found || valStart >= len(content) {
		return ""
	}
	c := content[valStart]
	if c != '"' && c != '\'' {
		return ""
	}
	end := skipLuaString(content, valStart)
	if end <= valStart+1 {
		return ""
	}
	return unquoteLua(content[valStart:end])
}

// extractLuaStrings returns every quoted string literal found in s, in order.
func extractLuaStrings(s string) []string {
	var out []string
	i, n := 0, len(s)
	for i < n {
		c := s[i]
		if c == '-' && i+1 < n && s[i+1] == '-' {
			ni, _ := skipCommentOrString(s, i)
			i = ni
			continue
		}
		if c == '"' || c == '\'' {
			end := skipLuaString(s, i)
			out = append(out, unquoteLua(s[i:end]))
			i = end
			continue
		}
		i++
	}
	return out
}

// unquoteLua strips surrounding quotes and unescapes a simple Lua string literal.
func unquoteLua(s string) string {
	if len(s) < 2 {
		return ""
	}
	inner := s[1 : len(s)-1]
	if !strings.ContainsRune(inner, '\\') {
		return inner
	}
	var b strings.Builder
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\\' && i+1 < len(inner) {
			i++
			switch inner[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			default:
				b.WriteByte(inner[i])
			}
			continue
		}
		b.WriteByte(inner[i])
	}
	return b.String()
}

// luaBracesBalanced reports whether '{' and '}' are balanced across the whole
// content, ignoring braces inside strings and comments.
func luaBracesBalanced(content string) bool {
	depth := 0
	i, n := 0, len(content)
	for i < n {
		if ni, skipped := skipCommentOrString(content, i); skipped {
			i = ni
			continue
		}
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth < 0 {
				return false
			}
		}
		i++
	}
	return depth == 0
}

// amlError is a small error type with a stable code, so callers/tests can match
// on the reason without string fragility.
type amlError struct{ Code string }

func (e *amlError) Error() string { return "aml: " + e.Code }
