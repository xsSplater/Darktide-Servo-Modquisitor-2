package helpers

import "testing"

func TestExtractModIDFromURL(t *testing.T) {
	cases := []struct {
		url      string
		expected int
	}{
		{"https://www.nexusmods.com/warhammer40kdarktide/mods/123", 123},
		{"https://www.nexusmods.com/warhammer40kdarktide/mods/16?tab=files", 16},
		{"https://www.nexusmods.com/warhammer40kdarktide/mods/abc", 0},
		{"", 0},
		{"123", 0},
	}
	for _, c := range cases {
		res := ExtractModIDFromURL(c.url)
		if res != c.expected {
			t.Errorf("ExtractModIDFromURL(%q) = %d, expected %d", c.url, res, c.expected)
		}
	}
}
