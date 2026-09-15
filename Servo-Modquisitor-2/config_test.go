// Servo-Modquisitor-2/config_test.go
package main

import (
	"encoding/json"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Language != "en" {
		t.Errorf("Language = %s, want en", cfg.Language)
	}
	if cfg.Theme != "dark" {
		t.Errorf("Theme = %s, want dark", cfg.Theme)
	}
	if cfg.UpdateCheckFrequency != "every_start" {
		t.Errorf("UpdateCheckFrequency = %s, want every_start", cfg.UpdateCheckFrequency)
	}
	if !cfg.ShowSystemMods {
		t.Error("ShowSystemMods should be true by default")
	}
	if !cfg.ShowModListAfterSort {
		t.Error("ShowModListAfterSort should be true by default")
	}
}

func TestConfigMarshalUnmarshal(t *testing.T) {
	cfg := &Config{
		Language:             "ru",
		Theme:                "custom",
		DateFormat:           "yyyy-mm-dd",
		UpdateCheckFrequency: "weekly",
		ShowSystemMods:       false,
		ShowModListAfterSort: true,
		WindowWidth:          1024,
		WindowHeight:         768,
	}
	data, err := json.MarshalIndent(cfg, "", "	")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg2.Language != cfg.Language {
		t.Errorf("Language = %s, want %s", cfg2.Language, cfg.Language)
	}
	if cfg2.Theme != cfg.Theme {
		t.Errorf("Theme = %s, want %s", cfg2.Theme, cfg.Theme)
	}
	if cfg2.DateFormat != cfg.DateFormat {
		t.Errorf("DateFormat = %s, want %s", cfg2.DateFormat, cfg.DateFormat)
	}
	if cfg2.WindowWidth != cfg.WindowWidth {
		t.Errorf("WindowWidth = %d, want %d", cfg2.WindowWidth, cfg.WindowWidth)
	}
}
