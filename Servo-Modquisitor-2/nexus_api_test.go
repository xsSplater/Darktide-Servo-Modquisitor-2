// Servo-Modquisitor-2/nexus_api_test.go
package main

import (
	"encoding/json"
	"testing"
)

func TestExtractFileNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/path/to/file.zip", "file.zip"},
		{"https://example.com/file", "file"},
		{"https://example.com/", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractFileNameFromURL(tt.url)
		if got != tt.want {
			t.Errorf("extractFileNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeForPattern(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World!", "helloworld"},
		{"123_abc", "123abc"},
		{"A.B.C", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeForPattern(tt.input)
		if got != tt.want {
			t.Errorf("normalizeForPattern(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestStripHTML(t *testing.T) {
	html := "<p>Hello <b>world</b></p><br/>"
	expected := "Hello world"
	got := stripHTML(html)
	if got != expected {
		t.Errorf("stripHTML(%q) = %q, want %q", html, got, expected)
	}
}

func TestParseFileInfoResponse(t *testing.T) {
	// Тестируем парсинг JSON ответа от Nexus
	jsonData := `{
		"files": [
			{"file_id": 123, "version": "1.0.0", "uploaded_timestamp": 1609459200, "file_name": "mod.zip"},
			{"file_id": 124, "version": "1.0.1", "uploaded_timestamp": 1609545600, "file_name": "mod_v1.0.1.zip"}
		]
	}`
	var result struct {
		Files []struct {
			FileID            int    `json:"file_id"`
			Version           string `json:"version"`
			UploadedTimestamp int64  `json:"uploaded_timestamp"`
			FileName          string `json:"file_name"`
		} `json:"files"`
	}
	if err := json.Unmarshal([]byte(jsonData), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(result.Files))
	}
	// Проверяем выбор самого нового
	newest := result.Files[0]
	for _, f := range result.Files {
		if f.UploadedTimestamp > newest.UploadedTimestamp {
			newest = f
		}
	}
	if newest.FileID != 124 {
		t.Errorf("newest file_id = %d, want 124", newest.FileID)
	}
	if newest.Version != "1.0.1" {
		t.Errorf("newest version = %s, want 1.0.1", newest.Version)
	}
}
