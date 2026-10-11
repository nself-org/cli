package main

// Purpose: verify hermetic OpenAPI generation and drift detection.
// Inputs: an isolated working directory and fake user-state paths.
// Outputs: generated document assertions and run exit-code checks.
// Constraints: no network or repository artifact writes; process state is restored.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunWriteAndCheck(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", filepath.Join(dir, "fake-home"))
	t.Setenv("NSELF_PLUGIN_DIR", filepath.Join(dir, "fake-plugins"))
	t.Setenv("NSELF_V15", "0")
	if err := os.Mkdir(".github", 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(".github", "openapi.json")
	if code, err := run(true); code != 1 || err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("missing artifact: run(true) = (%d, %v), want (1, missing error)", code, err)
	}
	if code, err := run(false); code != 0 || err != nil {
		t.Fatalf("write: run(false) = (%d, %v), want (0, nil)", code, err)
	}
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		OpenAPI string                     `json:"openapi"`
		Paths   map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(document, &spec); err != nil {
		t.Fatalf("generated document is invalid JSON: %v", err)
	}
	if spec.OpenAPI != "3.1.0" || len(spec.Paths) == 0 {
		t.Fatalf("generated document has version %q and %d paths", spec.OpenAPI, len(spec.Paths))
	}
	if code, err := run(true); code != 0 || err != nil {
		t.Fatalf("clean check: run(true) = (%d, %v), want (0, nil)", code, err)
	}
	if err := os.WriteFile(path, append(document, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if code, err := run(true); code != 1 || err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("modified artifact: run(true) = (%d, %v), want (1, stale error)", code, err)
	}
	if os.Getenv("HOME") != filepath.Join(dir, "fake-home") ||
		os.Getenv("NSELF_PLUGIN_DIR") != filepath.Join(dir, "fake-plugins") || os.Getenv("NSELF_V15") != "0" {
		t.Fatal("run did not restore the caller's environment")
	}
}
