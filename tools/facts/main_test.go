package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scratch builds a repository root holding the three sources.
func scratch(t *testing.T, version string, openapi bool) string {
	t.Helper()
	root := t.TempDir()
	gh := filepath.Join(root, ".github")
	if err := os.MkdirAll(gh, 0o755); err != nil {
		t.Fatal(err)
	}
	put := func(name, body string) {
		if err := os.WriteFile(filepath.Join(gh, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("VERSION", version+"\n")
	put("command-registry.json", testRegistry)
	if openapi {
		put("openapi.json", `{"openapi":"3.1.0"}`)
	}
	return root
}

func exec(root string, check bool, readme string) (int, string) {
	var out, errb bytes.Buffer
	code := run(root, check, readme, &out, &errb)
	return code, out.String() + errb.String()
}

func TestRunWriteThenCheck(t *testing.T) {
	root := scratch(t, "1.4.14", true)
	if code, msg := exec(root, true, ""); code != 1 || !strings.Contains(msg, "facts.json") {
		t.Fatalf("missing facts.json must fail naming it: %d %s", code, msg)
	}
	if code, msg := exec(root, false, ""); code != 0 {
		t.Fatalf("write failed: %d %s", code, msg)
	}
	if code, msg := exec(root, true, ""); code != 0 {
		t.Fatalf("check after write: %d %s", code, msg)
	}
	if code, _ := exec(root, false, ""); code != 0 {
		t.Fatal("rewrite must succeed")
	}
}

func TestRunDetectsSourceChange(t *testing.T) {
	root := scratch(t, "1.4.14", false)
	exec(root, false, "")
	gh := filepath.Join(root, ".github")
	for name, body := range map[string]string{
		"VERSION":               "1.4.15\n",
		"command-registry.json": strings.Replace(testRegistry, `"commands":5`, `"commands":6`, 1),
		"openapi.json":          `{}`,
	} {
		path := filepath.Join(gh, name)
		old, _ := os.ReadFile(path)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, msg := exec(root, true, ""); code != 1 || !strings.Contains(msg, "facts.json") {
			t.Errorf("%s change: want exit 1 naming facts.json, got %d %s", name, code, msg)
		}
		if old == nil {
			_ = os.Remove(path)
		} else {
			_ = os.WriteFile(path, old, 0o644)
		}
	}
}

func TestRunReadme(t *testing.T) {
	root := scratch(t, "1.4.14", false)
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	exec(root, false, "")
	if code, msg := exec(root, true, readme); code != 1 || !strings.Contains(msg, "README.md") {
		t.Fatalf("stale block must fail naming the README: %d %s", code, msg)
	}
	if code, msg := exec(root, false, readme); code != 0 {
		t.Fatalf("fill: %d %s", code, msg)
	}
	filled, _ := os.ReadFile(readme)
	if code, _ := exec(root, false, readme); code != 0 {
		t.Fatal("second fill failed")
	}
	if again, _ := os.ReadFile(readme); !bytes.Equal(again, filled) {
		t.Fatal("second fill changed the README")
	}
	if code, msg := exec(root, true, readme); code != 0 {
		t.Fatalf("check after fill: %d %s", code, msg)
	}
}

func TestRunErrors(t *testing.T) {
	root := t.TempDir()
	if code, _ := exec(root, true, ""); code != 2 {
		t.Fatalf("no sources: want 2, got %d", code)
	}
	root = scratch(t, "nope", false)
	if code, _ := exec(root, true, ""); code != 2 {
		t.Fatalf("bad version: want 2, got %d", code)
	}
	root = scratch(t, "1.4.14", false)
	if code, _ := exec(root, true, filepath.Join(root, "absent.md")); code != 2 {
		t.Fatalf("missing README: want 2, got %d", code)
	}
}
