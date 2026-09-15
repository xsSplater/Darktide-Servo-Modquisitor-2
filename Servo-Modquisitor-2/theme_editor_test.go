// Servo-Modquisitor-2/theme_editor_test.go
package main

import (
	"image/color"
	"testing"
)

func TestHexFromColor(t *testing.T) {
	c := color.NRGBA{R: 0x12, G: 0x34, B: 0x56, A: 0x78}
	hex := hexFromColor(c)
	if hex != "#12345678" {
		t.Errorf("hexFromColor(%v) = %s, want #12345678", c, hex)
	}
	// Проверка с альфа 0xFF
	c = color.NRGBA{R: 0xAB, G: 0xCD, B: 0xEF, A: 0xFF}
	hex = hexFromColor(c)
	if hex != "#ABCDEFFF" {
		t.Errorf("hexFromColor(%v) = %s, want #ABCDEFFF", c, hex)
	}
	// Проверка с прозрачностью 0
	c = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x00}
	hex = hexFromColor(c)
	if hex != "#00000000" {
		t.Errorf("hexFromColor(%v) = %s, want #00000000", c, hex)
	}
}

func TestColorFromHex(t *testing.T) {
	tests := []struct {
		hex string
		r   uint8
		g   uint8
		b   uint8
		a   uint8
		err bool
	}{
		{"#12345678", 0x12, 0x34, 0x56, 0x78, false},
		{"#ABCDEF", 0xAB, 0xCD, 0xEF, 0xFF, false},
		{"#00000000", 0x00, 0x00, 0x00, 0x00, false},
		{"#1234567", 0, 0, 0, 0, true}, // неверная длина
		{"12345678", 0, 0, 0, 0, true}, // без #
		{"#GHIJKL", 0, 0, 0, 0, true},  // не hex
	}
	for _, tt := range tests {
		c, err := colorFromHex(tt.hex)
		if tt.err {
			if err == nil {
				t.Errorf("colorFromHex(%s) expected error, got nil", tt.hex)
			}
			continue
		}
		if err != nil {
			t.Errorf("colorFromHex(%s) error: %v", tt.hex, err)
			continue
		}
		if c.R != tt.r || c.G != tt.g || c.B != tt.b || c.A != tt.a {
			t.Errorf("colorFromHex(%s) = %v, want {R:%d G:%d B:%d A:%d}", tt.hex, c, tt.r, tt.g, tt.b, tt.a)
		}
	}
}
