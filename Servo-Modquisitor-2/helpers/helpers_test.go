// Servo-Modquisitor-2/helpers/helpers_test.go
package helpers

import "testing"

func TestExtractModIDFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want int
	}{
		{"https://nexusmods.com/warhammer40kdarktide/mods/123", 123},
		{"https://nexusmods.com/warhammer40kdarktide/mods/789?tab=files", 789},
		{"https://nexusmods.com/warhammer40kdarktide/mods/456#comments", 456},
		{"https://nexusmods.com/warhammer40kdarktide/mods/789/?tab=files", 789},
		{"https://nexusmods.com/warhammer40kdarktide/mods/", 0},
		{"", 0},
	}
	for _, tt := range tests {
		if got := ExtractModIDFromURL(tt.url); got != tt.want {
			t.Errorf("ExtractModIDFromURL(%q) = %d, want %d", tt.url, got, tt.want)
		}
	}
}
func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}
	if !ContainsString(slice, "b") {
		t.Error("ContainsString returned false for existing item")
	}
	if ContainsString(slice, "d") {
		t.Error("ContainsString returned true for non-existing item")
	}
	if ContainsString(nil, "a") {
		t.Error("ContainsString returned true for nil slice")
	}
}
