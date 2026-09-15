// Servo-Modquisitor-2/disk_search_test.go

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGetSearchRoots(t *testing.T) {
	roots := getSearchRoots()
	if len(roots) == 0 {
		t.Error("getSearchRoots returned empty slice")
	}
	// Не можем проверить конкретные значения, т.к. зависит от системы
}

func TestSearchGameOnDrives(t *testing.T) {
	// Создаём временную структуру папок, имитирующую Darktide
	dir := t.TempDir()
	gameRoot := filepath.Join(dir, "Warhammer 40,000 DARKTIDE")
	if err := os.MkdirAll(filepath.Join(gameRoot, "binaries"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Создаём ещё одну папку с похожим именем, но без binaries
	otherDir := filepath.Join(dir, "Warhammer 40,000 DARKTIDE_backup")
	if err := os.Mkdir(otherDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	app := &App{}
	roots := []string{dir}
	ctx := context.Background()
	results, err := app.searchGameOnDrives(roots, ctx, nil)
	if err != nil {
		t.Fatalf("searchGameOnDrives error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if len(results) > 0 && results[0] != gameRoot {
		t.Errorf("expected %s, got %s", gameRoot, results[0])
	}
}
