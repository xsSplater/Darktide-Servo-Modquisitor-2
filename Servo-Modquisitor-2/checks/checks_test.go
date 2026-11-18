package checks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWriteLoadOrder(t *testing.T) {
	tmpDir := t.TempDir()
	// Инициализируем глобалы, чтобы msgGetter не был nil
	InitGlobals(
		func(string) {},
		func(string) string { return "" },
		nil, nil, tmpDir, nil, nil, nil,
	)
	SetProfileDataDir(tmpDir)
	os.MkdirAll(filepath.Join(tmpDir, "mods"), 0755)

	entries := []LoadOrderEntry{
		{Name: "ModA", Active: true},
		{Name: "ModB", Active: false},
	}

	if err := WriteLoadOrder(entries); err != nil {
		t.Fatalf("WriteLoadOrder: %v", err)
	}

	read := ReadLoadOrder()
	if len(read) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(read))
	}
	if read[0].Name != "ModA" || !read[0].Active {
		t.Errorf("ModA wrong")
	}
	if read[1].Name != "ModB" || read[1].Active {
		t.Errorf("ModB wrong")
	}
}
