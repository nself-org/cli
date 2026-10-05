package postgres

// generator_test.go — tests for the init-script renderer and writer
// (P7-LIVE-21: RenderInitScript is the render half shared with plan mode).
// Inputs: temp project dirs. Outputs: pass/fail. Constraints: no network, no
// docker; error paths use directories where files are expected.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderInitScriptReturnsBothFiles(t *testing.T) {
	files, err := RenderInitScript(t.TempDir())
	if err != nil {
		t.Fatalf("RenderInitScript: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d", len(files))
	}
	if !bytes.Equal(files["postgres/init/01-init.sql"], []byte(initScriptSQL)) {
		t.Error("01-init.sql does not match initScriptSQL")
	}
	if !bytes.Equal(files["postgres/init/04-np-plugins.sql"], []byte(npPluginsInitSQL)) {
		t.Error("04-np-plugins.sql does not match npPluginsInitSQL")
	}
	if !strings.Contains(string(files["postgres/init/01-init.sql"]), "CREATE SCHEMA IF NOT EXISTS auth;") {
		t.Error("01-init.sql lacks the auth schema")
	}
}

func TestRenderInitScriptDoesNotTouchDisk(t *testing.T) {
	dir := t.TempDir()
	if _, err := RenderInitScript(dir); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("render wrote %d entries into the workdir", len(entries))
	}
}

// Render and write must stay byte-identical: plan mode relies on it.
func TestGenerateInitScriptMatchesRender(t *testing.T) {
	dir := t.TempDir()
	if err := GenerateInitScript(dir); err != nil {
		t.Fatalf("GenerateInitScript: %v", err)
	}
	files, err := RenderInitScript(dir)
	if err != nil {
		t.Fatal(err)
	}
	for rel, want := range files {
		got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s: written bytes differ from rendered bytes", rel)
		}
		if runtimeUnixModes() {
			info, _ := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
			if info.Mode().Perm() != 0644 {
				t.Errorf("%s mode = %v, want 0644", rel, info.Mode().Perm())
			}
		}
	}
}

func TestGenerateInitScriptIdempotent(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := GenerateInitScript(dir); err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

func TestEnsureNpPluginsTableIsIdempotentSQL(t *testing.T) {
	sql := EnsureNpPluginsTable()
	if sql != npPluginsInitSQL {
		t.Error("EnsureNpPluginsTable must return npPluginsInitSQL")
	}
	if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS np_plugins") {
		t.Error("np_plugins creation must be idempotent")
	}
}

func TestGenerateInitScriptErrorPaths(t *testing.T) {
	t.Run("init dir blocked by a file", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "postgres"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		err := GenerateInitScript(dir)
		if err == nil || !strings.Contains(err.Error(), "creating postgres init dir") {
			t.Fatalf("want init dir error, got %v", err)
		}
	})
	t.Run("01-init.sql is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "postgres", "init", "01-init.sql"), 0755); err != nil {
			t.Fatal(err)
		}
		err := GenerateInitScript(dir)
		if err == nil || !strings.Contains(err.Error(), "writing postgres init script") {
			t.Fatalf("want init script error, got %v", err)
		}
	})
	t.Run("04-np-plugins.sql is a directory", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, "postgres", "init", "04-np-plugins.sql"), 0755); err != nil {
			t.Fatal(err)
		}
		err := GenerateInitScript(dir)
		if err == nil || !strings.Contains(err.Error(), "writing np_plugins init script") {
			t.Fatalf("want np_plugins error, got %v", err)
		}
	})
}

// runtimeUnixModes reports whether file modes are meaningful on this OS.
func runtimeUnixModes() bool { return filepath.Separator == '/' }
