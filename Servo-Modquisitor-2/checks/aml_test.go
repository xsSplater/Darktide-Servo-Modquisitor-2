package checks

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteAMLConfig(t *testing.T) {
	tmpDir := t.TempDir()
	SetModsDir(tmpDir)
	modFolder := "TestMod"
	os.MkdirAll(filepath.Join(tmpDir, modFolder), 0755)

	modContent := `return {
		run = function()
			-- function body
		end,
		load_after = { "mod1" },
		version = "1.0.0",
	}`

	modFilePath := filepath.Join(tmpDir, modFolder, modFolder+".mod")
	if err := os.WriteFile(modFilePath, []byte(modContent), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := ReadAMLConfig(modFolder)
	if cfg.ParseErr != "" {
		t.Fatalf("ParseErr: %s", cfg.ParseErr)
	}
	if len(cfg.LoadAfter) != 1 || cfg.LoadAfter[0] != "mod1" {
		t.Errorf("LoadAfter wrong: %v", cfg.LoadAfter)
	}
	if cfg.Version != "1.0.0" {
		t.Errorf("Version wrong: %s", cfg.Version)
	}

	cfg.LoadAfter = []string{"mod2"}
	cfg.LoadBefore = []string{"mod3"}
	cfg.Require = []string{"mod4"}
	cfg.Version = "2.0.0"
	cfg.Author = "NewAuthor"

	if err := WriteAMLConfig(cfg); err != nil {
		t.Fatalf("WriteAMLConfig failed: %v", err)
	}

	cfg2 := ReadAMLConfig(modFolder)
	if len(cfg2.LoadAfter) != 1 || cfg2.LoadAfter[0] != "mod2" {
		t.Errorf("LoadAfter wrong: %v", cfg2.LoadAfter)
	}
	if len(cfg2.LoadBefore) != 1 || cfg2.LoadBefore[0] != "mod3" {
		t.Errorf("LoadBefore wrong: %v", cfg2.LoadBefore)
	}
	if len(cfg2.Require) != 1 || cfg2.Require[0] != "mod4" {
		t.Errorf("Require wrong: %v", cfg2.Require)
	}
	if cfg2.Version != "2.0.0" {
		t.Errorf("Version wrong: %s", cfg2.Version)
	}
	if cfg2.Author != "NewAuthor" {
		t.Errorf("Author wrong: %s", cfg2.Author)
	}

	data, _ := os.ReadFile(modFilePath)
	if !strings.Contains(string(data), "function body") {
		t.Errorf("Function body was lost!")
	}
}
