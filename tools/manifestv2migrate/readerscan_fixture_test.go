package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := dir; d != filepath.Dir(d); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("go.mod not found")
	return ""
}

// Purpose: scan the immutable released readers without git, tags or network.
// Inputs: complete source files at v1.4.12 commit
// 1c1962daad278076f8125833f40b191e7eb237d5. Outputs: isolated Go source tree.
// Constraints: .txt suffix keeps historical source outside the current build.
// SPORT: manifest v2 compatibility coverage.
func readerFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join("testdata", "v1412-source")
	dst := t.TempDir()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out := filepath.Join(dst, strings.TrimSuffix(rel, ".txt"))
		if err := os.MkdirAll(filepath.Dir(out), 0700); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

func TestReaderScanWithoutGit(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "not-a-repository"))
	TestReaderScan(t)
}

func TestReaderScanDetectsAdditionalDecodedKey(t *testing.T) {
	dir := readerFixture(t)
	path := filepath.Join(dir, "internal", "plugin", "interfaces.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	marker := "type PluginManifest struct {"
	changed := strings.Replace(string(data), marker, marker+"\n DebtRegression string `json:\"debt_regression\"`", 1)
	if changed == string(data) {
		t.Fatal("fixture no longer contains PluginManifest")
	}
	if err := os.WriteFile(path, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	got := renderReaders(readerFields(t, dir))
	if !strings.Contains(got, "key debt_regression PluginManifest\n") {
		t.Fatal("scan missed the additional decoded key")
	}
}
