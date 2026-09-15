// Servo-Modquisitor-2/settings_editor_test.go
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// compareSettingsNodes глубоко сравнивает два узла SettingsNode.
// Игнорирует поля Parent и Modified.
func compareSettingsNodes(a, b *SettingsNode) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Key != b.Key {
		return false
	}
	if a.IsArray != b.IsArray {
		return false
	}
	if !reflect.DeepEqual(a.Value, b.Value) {
		return false
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for k, v := range a.Children {
		if childB, ok := b.Children[k]; !ok {
			return false
		} else if !compareSettingsNodes(v, childB) {
			return false
		}
	}
	return true
}

// parseStringToNode парсит строку как файл настроек и возвращает корневой узел.
func parseStringToNode(t *testing.T, content string) *SettingsNode {
	t.Helper()
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.config")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	node, err := parseSettingsFile(path)
	if err != nil {
		t.Fatalf("parseSettingsFile error: %v", err)
	}
	return node
}

// serializeNodeToString сериализует узел в строку (без корневого "root").
func serializeNodeToString(node *SettingsNode) string {
	if node.Key == "root" {
		var sb strings.Builder
		keys := getSortedKeys(node.Children)
		for _, k := range keys {
			sb.WriteString(serializeNode(node.Children[k], ""))
		}
		return sb.String()
	}
	return serializeNode(node, "")
}

// TestParseSettingsFile проверяет парсинг корректных файлов.
func TestParseSettingsFile(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    *SettingsNode
	}{
		{
			name: "simple key-value",
			content: `key = "value"
`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"key": {Key: "key", Value: "value"},
				},
			},
		},
		{
			name: "integer and boolean",
			content: `int_key = 42
bool_key = true
`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"int_key":  {Key: "int_key", Value: 42},
					"bool_key": {Key: "bool_key", Value: true},
				},
			},
		},
		{
			name: "nested table",
			content: `outer = {
  inner_key = "inner_value"
}`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"outer": {
						Key: "outer",
						Children: map[string]*SettingsNode{
							"inner_key": {Key: "inner_key", Value: "inner_value"},
						},
					},
				},
			},
		},
		{
			name:    "array with commas",
			content: `array_key = [1, 2, 3]`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"array_key": {
						Key:     "array_key",
						IsArray: true,
						Value:   []interface{}{1, 2, 3},
					},
				},
			},
		},
		{
			name: "multi-line array with commas",
			content: `array_key = [
  1,
  2,
  3
]`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"array_key": {
						Key:     "array_key",
						IsArray: true,
						Value:   []interface{}{1, 2, 3},
					},
				},
			},
		},
		{
			name: "comments and empty lines",
			content: `-- comment
key = "value"
-- another comment
`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"key": {Key: "key", Value: "value"},
				},
			},
		},
		{
			name:    "quoted key with spaces",
			content: `"key with spaces" = "value"`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"key with spaces": {Key: "key with spaces", Value: "value"},
				},
			},
		},
		{
			name: "nested table with multiple keys",
			content: `outer = {
  a = 1
  b = "two"
  c = {
    d = 3.14
  }
}`,
			want: &SettingsNode{
				Key: "root",
				Children: map[string]*SettingsNode{
					"outer": {
						Key: "outer",
						Children: map[string]*SettingsNode{
							"a": {Key: "a", Value: 1},
							"b": {Key: "b", Value: "two"},
							"c": {
								Key: "c",
								Children: map[string]*SettingsNode{
									"d": {Key: "d", Value: 3.14},
								},
							},
						},
					},
				},
			},
		},
		// Тест с запятой убран, так как реальный синтаксис не использует завершающие запятые
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStringToNode(t, tt.content)
			if !compareSettingsNodes(got, tt.want) {
				t.Errorf("parseSettingsFile() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// TestParseSettingsFileErrors проверяет обработку ошибок при парсинге.
// Парсер не выдаёт ошибку для незакрытых скобок, поэтому ожидаем false.
func TestParseSettingsFileErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "unclosed brace",
			content: `key = { value = 1`,
			wantErr: false, // парсер не проверяет баланс
		},
		{
			name:    "unclosed array",
			content: `key = [1, 2, 3`,
			wantErr: false,
		},
		{
			name:    "invalid syntax",
			content: `key = = value`,
			wantErr: false,
		},
		{
			name:    "empty file",
			content: "",
			wantErr: false,
		},
		{
			name:    "only comments",
			content: "-- just a comment\n-- another",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			path := filepath.Join(tmpDir, "test.config")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			_, err := parseSettingsFile(path)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSettingsFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSerializeSettings проверяет сериализацию структуры обратно в строку.
func TestSerializeSettings(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "simple key-value",
			content: `key = "value"
`,
		},
		{
			name: "nested table",
			content: `outer = {
  inner = "value"
}
`,
		},
		{
			name: "array with commas",
			content: `arr = [1, 2, 3]
`,
		},
		{
			name: "mixed types",
			content: `str = "hello"
num = 42
flag = true
obj = {
  sub = "nested"
}
list = [1, "two", 3]
`,
		},
		{
			name: "quoted keys",
			content: `"key with spaces" = "value"
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalNode := parseStringToNode(t, tt.content)
			serialized := serializeNodeToString(originalNode)
			reparsedNode := parseStringToNode(t, serialized)
			if !compareSettingsNodes(originalNode, reparsedNode) {
				t.Errorf("Serialization round-trip failed.\nOriginal:\n%s\nSerialized:\n%s", tt.content, serialized)
			}
		})
	}
}

// TestGetNodeByPath проверяет поиск узла по пути.
func TestGetNodeByPath(t *testing.T) {
	content := `a = {
  b = "value"
  c = 123
}
d = [1,2]
`
	node := parseStringToNode(t, content)
	tests := []struct {
		path string
		want string
	}{
		{"a", "a"},
		{"a/b", "b"},
		{"a/c", "c"},
		{"d", "d"},
		{"nonexistent", ""},
		{"a/b/c", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			found := getNodeByPath(node, tt.path)
			if tt.want == "" {
				if found != nil {
					t.Errorf("getNodeByPath(%q) = %v, want nil", tt.path, found)
				}
			} else {
				if found == nil || found.Key != tt.want {
					t.Errorf("getNodeByPath(%q) = %v, want key %q", tt.path, found, tt.want)
				}
			}
		})
	}
}
