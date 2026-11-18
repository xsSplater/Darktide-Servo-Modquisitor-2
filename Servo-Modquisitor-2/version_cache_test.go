package main

import (
	"testing"
)

func TestParseVersionCacheJSON(t *testing.T) {
	// Старый формат (map[string]string)
	oldData := []byte(`{"123:modA": "1600000000", "456:modB": "1600000001"}`)
	resOld, err := parseVersionCacheJSON(oldData)
	if err != nil {
		t.Fatalf("Failed to parse old format: %v", err)
	}
	if len(resOld) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(resOld))
	}
	itemA, ok := resOld["123:modA"]
	if !ok {
		t.Fatalf("Item modA not found")
	}
	if itemA.Timestamp != 1600000000 {
		t.Errorf("Wrong timestamp for modA: %d", itemA.Timestamp)
	}
	if itemA.Source != "nexus" {
		t.Errorf("Wrong source: %s", itemA.Source)
	}

	// Новый формат
	newData := []byte(`{"123:modA": {"timestamp": 1600000000, "version": "1.0", "folder": "modA", "source": "manual"}}`)
	resNew, err := parseVersionCacheJSON(newData)
	if err != nil {
		t.Fatalf("Failed to parse new format: %v", err)
	}
	if resNew["123:modA"].Version != "1.0" {
		t.Errorf("Wrong version for modA")
	}
	if resNew["123:modA"].Source != "manual" {
		t.Errorf("Wrong source for modA")
	}

	// Отсеивание мусора (слишком большие ID)
	badData := []byte(`{"9999999:modBad": {"timestamp": 1600000000, "version": "1.0", "folder": "modBad"}}`)
	resBad, err := parseVersionCacheJSON(badData)
	if err != nil {
		t.Fatalf("Failed to parse bad format: %v", err)
	}
	if _, ok := resBad["9999999:modBad"]; ok {
		t.Errorf("Bad ID should have been filtered out")
	}
}
