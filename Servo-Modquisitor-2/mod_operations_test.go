// Servo-Modquisitor-2/mod_operations_test.go
package main

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	tests := []struct {
		destDir string
		name    string
		want    string
		err     bool
	}{
		{"/tmp", "file.txt", filepath.Join("/tmp", "file.txt"), false},
		{"/tmp", "sub/file.txt", filepath.Join("/tmp", "sub", "file.txt"), false},
		{"/tmp", "/etc/passwd", "", true},
		{"/tmp", "../etc/passwd", "", true},
		{"/tmp", "..\\etc\\passwd", "", true}, // Windows-стиль тоже должен быть отклонён
	}
	for _, tt := range tests {
		got, err := safeJoin(tt.destDir, tt.name)
		if tt.err {
			if err == nil {
				t.Errorf("safeJoin(%q, %q) expected error, got nil", tt.destDir, tt.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("safeJoin(%q, %q) error: %v", tt.destDir, tt.name, err)
			continue
		}
		// Нормализуем ожидаемый путь для текущей ОС
		expected := filepath.FromSlash(tt.want)
		if got != expected {
			t.Errorf("safeJoin(%q, %q) = %q, want %q", tt.destDir, tt.name, got, expected)
		}
	}
}

func TestNormalizeArchiveStructure(t *testing.T) {
	dir := t.TempDir()
	app := &App{} // создаём пустой App, для вызова метода нужен receiver

	// Создаём структуру: одна папка-обёртка с mods/ внутри
	outer := filepath.Join(dir, "outer")
	modsDir := filepath.Join(outer, "mods")
	modFolder := filepath.Join(modsDir, "testmod")
	if err := os.MkdirAll(modFolder, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Кладём .mod файл в папку мода (чтобы структура была валидной)
	if err := os.WriteFile(filepath.Join(modFolder, "testmod.mod"), []byte(""), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Вызываем normalizeArchiveStructure на временной папке, но она не знает о modsPath.
	// Это тест для функции, которая принимает tmpDir.
	err := app.normalizeArchiveStructure(dir) // но у нас tmpDir = dir (корень), а обёртка внутри
	if err != nil {
		t.Fatalf("normalizeArchiveStructure error: %v", err)
	}
	// После нормализации папка outer должна исчезнуть, а testmod подняться в корень
	if _, err := os.Stat(filepath.Join(dir, "testmod")); err != nil {
		t.Error("testmod folder not moved to root")
	}
	if _, err := os.Stat(outer); err == nil {
		t.Error("outer folder still exists")
	}
}

// --- Вспомогательная функция: собрать zip с заданными файлами. ---
func makeZip(t *testing.T, zipPath string, entries map[string][]byte) {
	t.Helper()
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for name, data := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %q: %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write %q: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}

// #3: path traversal внутри zip должен отвергаться.
func TestExtractZipLimitsRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "bomb.zip")
	makeZip(t, zipPath, map[string][]byte{
		"../escape.txt": []byte("evil"),
	})

	dest := filepath.Join(dir, "out")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	err := app.extractZipWithLimits(zipPath, dest, 1024, 4096)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !errors.Is(err, errArchiveSecurity) {
		t.Errorf("expected errArchiveSecurity, got %v", err)
	}
}

// #3: заявленный размер файла больше лимита — отвергаем до чтения.
func TestExtractZipLimitsRejectsOversizedFile(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "big.zip")
	makeZip(t, zipPath, map[string][]byte{
		"big.bin": make([]byte, 100),
	})

	dest := filepath.Join(dir, "out")
	os.MkdirAll(dest, 0755)

	app := &App{}
	err := app.extractZipWithLimits(zipPath, dest, 50, 1000)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errArchiveLimitExceeded) {
		t.Errorf("expected errArchiveLimitExceeded, got %v", err)
	}
}

// #3: суммарный объём больше MaxExtractedSize — отвергаем.
func TestExtractZipLimitsRejectsTotalOversize(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "many.zip")
	payload := make([]byte, 20)
	makeZip(t, zipPath, map[string][]byte{
		"a.bin": payload,
		"b.bin": payload,
		"c.bin": payload,
		"d.bin": payload,
	})

	dest := filepath.Join(dir, "out")
	os.MkdirAll(dest, 0755)

	app := &App{}
	// maxFile = 100 (каждый пройдёт), maxTotal = 50 (третий упадёт)
	err := app.extractZipWithLimits(zipPath, dest, 100, 50)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, errArchiveLimitExceeded) {
		t.Errorf("expected errArchiveLimitExceeded, got %v", err)
	}
}

// #3: валидный архив проходит и содержимое корректно.
func TestExtractZipLimitsSucceeds(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "ok.zip")
	makeZip(t, zipPath, map[string][]byte{
		"sub/a.txt": []byte("hello"),
		"b.txt":     []byte("world"),
	})

	dest := filepath.Join(dir, "out")
	os.MkdirAll(dest, 0755)

	app := &App{}
	err := app.extractZipWithLimits(zipPath, dest, 1024, 4096)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	gotA, err := os.ReadFile(filepath.Join(dest, "sub", "a.txt"))
	if err != nil || string(gotA) != "hello" {
		t.Errorf("sub/a.txt = %q (err=%v), want hello", gotA, err)
	}
	gotB, err := os.ReadFile(filepath.Join(dest, "b.txt"))
	if err != nil || string(gotB) != "world" {
		t.Errorf("b.txt = %q (err=%v), want world", gotB, err)
	}
}

// #3: пустой архив не паникует и не оставляет мусора.
func TestExtractZipLimitsEmpty(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "empty.zip")
	makeZip(t, zipPath, map[string][]byte{})

	dest := filepath.Join(dir, "out")
	os.MkdirAll(dest, 0755)

	app := &App{}
	if err := app.extractZipWithLimits(zipPath, dest, 1024, 4096); err != nil {
		t.Errorf("empty archive should not error, got %v", err)
	}
}
