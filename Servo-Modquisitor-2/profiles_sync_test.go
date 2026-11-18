package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeModsSignature(t *testing.T) {
	dir := t.TempDir()

	// Пустая папка
	sig1, err := ComputeModsSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sig1 == "" {
		t.Error("signature for empty dir should not be empty")
	}

	// Добавляем файл
	file1 := filepath.Join(dir, "file1.txt")
	if err := os.WriteFile(file1, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	sig2, err := ComputeModsSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sig1 == sig2 {
		t.Error("signature should change after adding file")
	}

	// Меняем содержимое
	if err := os.WriteFile(file1, []byte("world"), 0644); err != nil {
		t.Fatal(err)
	}
	sig3, err := ComputeModsSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sig2 == sig3 {
		t.Error("signature should change after modifying file content")
	}

	// Меняем время модификации (не содержимое)
	newTime := time.Now().Add(1 * time.Hour)
	if err := os.Chtimes(file1, newTime, newTime); err != nil {
		t.Fatal(err)
	}
	sig4, err := ComputeModsSignature(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sig3 == sig4 {
		t.Error("signature should change after touching file")
	}
}
