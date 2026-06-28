// aml_test.go
package checks

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// writeMod creates <dir>/<folder>/<folder>.mod with the given content and points
// the package modsDir at dir.
func writeMod(t *testing.T, dir, folder, content string) string {
	t.Helper()
	modsDir = dir
	md := filepath.Join(dir, folder)
	if err := os.MkdirAll(md, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(md, folder+".mod")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

const sampleMod = `return {
  run = function()
    new_mod("testmod", {
      mod_script = "scripts/mods/testmod/testmod",
    })
  end,

  load_after = {
    "weapon_customization",
    "for_the_drip",
  },
  require = {
    "weapon_customization",
  },
  version = "24.07.05",

  packages = {},
}
`

func TestReadAMLConfig(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "testmod", sampleMod)

	cfg := ReadAMLConfig("testmod")
	if cfg.ParseErr != "" {
		t.Fatalf("unexpected ParseErr: %s", cfg.ParseErr)
	}
	if want := []string{"weapon_customization", "for_the_drip"}; !reflect.DeepEqual(cfg.LoadAfter, want) {
		t.Errorf("LoadAfter = %v, want %v", cfg.LoadAfter, want)
	}
	if want := []string{"weapon_customization"}; !reflect.DeepEqual(cfg.Require, want) {
		t.Errorf("Require = %v, want %v", cfg.Require, want)
	}
	if len(cfg.LoadBefore) != 0 {
		t.Errorf("LoadBefore = %v, want empty", cfg.LoadBefore)
	}
	if cfg.Version != "24.07.05" {
		t.Errorf("Version = %q, want 24.07.05", cfg.Version)
	}
	if !cfg.HasConfig {
		t.Errorf("HasConfig = false, want true")
	}
}

func TestWriteAMLConfigReplaceInsertClear(t *testing.T) {
	dir := t.TempDir()
	path := writeMod(t, dir, "testmod", sampleMod)

	cfg := ReadAMLConfig("testmod")
	cfg.LoadAfter = []string{"weapon_customization"} // replace existing array
	cfg.LoadBefore = []string{"some_hud_mod"}         // insert new key (was absent)
	cfg.Require = nil                                  // clear existing -> `require = {}`

	if err := WriteAMLConfig(cfg); err != nil {
		t.Fatalf("WriteAMLConfig: %v", err)
	}

	// Backup must exist and equal the original.
	bak, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	if string(bak) != sampleMod {
		t.Errorf("backup content does not match original")
	}

	out, _ := os.ReadFile(path)
	got := string(out)

	// The run function and packages must be preserved untouched.
	if !strings.Contains(got, `new_mod("testmod"`) {
		t.Errorf("run function body was altered:\n%s", got)
	}
	if !strings.Contains(got, "packages = {}") {
		t.Errorf("packages was altered:\n%s", got)
	}
	if !luaBracesBalanced(got) {
		t.Errorf("result has unbalanced braces:\n%s", got)
	}

	re := ReadAMLConfig("testmod")
	if want := []string{"weapon_customization"}; !reflect.DeepEqual(re.LoadAfter, want) {
		t.Errorf("after write LoadAfter = %v, want %v", re.LoadAfter, want)
	}
	if want := []string{"some_hud_mod"}; !reflect.DeepEqual(re.LoadBefore, want) {
		t.Errorf("after write LoadBefore = %v, want %v", re.LoadBefore, want)
	}
	if len(re.Require) != 0 {
		t.Errorf("after write Require = %v, want empty", re.Require)
	}
	if re.Version != "24.07.05" {
		t.Errorf("after write Version = %q, want unchanged 24.07.05", re.Version)
	}
}

const minimalMod = `return {
	run = function()
		new_mod("minimal", {})
	end,
	packages = {
		"packages/mods/minimal/minimal",
	},
}
`

func TestWriteAMLConfigInsertIntoMinimal(t *testing.T) {
	dir := t.TempDir()
	path := writeMod(t, dir, "minimal", minimalMod)

	cfg := ReadAMLConfig("minimal")
	if cfg.HasConfig {
		t.Fatalf("minimal mod should have no AML config")
	}
	cfg.LoadAfter = []string{"a_mod", "b_mod"}
	if err := WriteAMLConfig(cfg); err != nil {
		t.Fatalf("WriteAMLConfig: %v", err)
	}

	out, _ := os.ReadFile(path)
	got := string(out)
	if !luaBracesBalanced(got) {
		t.Fatalf("unbalanced braces after insert:\n%s", got)
	}
	if !strings.Contains(got, "packages = {") {
		t.Errorf("packages dropped:\n%s", got)
	}
	re := ReadAMLConfig("minimal")
	if want := []string{"a_mod", "b_mod"}; !reflect.DeepEqual(re.LoadAfter, want) {
		t.Errorf("LoadAfter = %v, want %v", re.LoadAfter, want)
	}
}

// A `load_after` that lives inside the run function (at brace-depth 0, since a
// function body is not brace-delimited) must NOT be mistaken for the top-level
// metadata key.
func TestFunctionBodyNotMatched(t *testing.T) {
	const tricky = `return {
  run = function()
    local load_after = { "inside_run" }
    return load_after
  end,
  load_after = { "real_one" },
}
`
	dir := t.TempDir()
	writeMod(t, dir, "tricky", tricky)

	cfg := ReadAMLConfig("tricky")
	if want := []string{"real_one"}; !reflect.DeepEqual(cfg.LoadAfter, want) {
		t.Errorf("LoadAfter = %v, want %v (must ignore the one inside run)", cfg.LoadAfter, want)
	}
}

// A '}' inside a comment must not break brace matching.
func TestCommentBraceNotCounted(t *testing.T) {
	const withComment = `return {
  -- this closing brace } should be ignored
  load_after = { "x" },
}
`
	dir := t.TempDir()
	writeMod(t, dir, "cmt", withComment)

	cfg := ReadAMLConfig("cmt")
	if want := []string{"x"}; !reflect.DeepEqual(cfg.LoadAfter, want) {
		t.Errorf("LoadAfter = %v, want %v", cfg.LoadAfter, want)
	}
}

func TestCleanList(t *testing.T) {
	got := cleanList([]string{" a ", "a", "", "b", "b ", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cleanList = %v, want %v", got, want)
	}
}
