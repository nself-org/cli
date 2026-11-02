package repoqa

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A quiet grep may close its input before printf finishes. Under pipefail that
// must not turn a documented schema into a spurious missing-row failure.
func TestContractWikiLargeSchemaTable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("contract wiki check requires POSIX paths")
	}
	root := repoRoot(t)
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("contract wiki check requires jq")
	}
	fixture := t.TempDir()
	for _, name := range []string{
		".github/wiki/JSON-Output.md", ".github/wiki/Exit-Codes.md",
		".github/wiki/error-codes.md", ".github/command-registry.json", "schemas/index.json",
	} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		if name == ".github/wiki/JSON-Output.md" {
			// Valid duplicate rows make the producer exceed the pipe buffer,
			// reproducing the early-reader-exit race deterministically.
			data = append(data, []byte(strings.Repeat("\n| `schemas/envelope.v1.schema.json` | Envelope |", 16384)+"\n")...)
		}
		dest := filepath.Join(fixture, name)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	// The check verifies that each documented schema exists, without reading it.
	err := filepath.WalkDir(filepath.Join(root, "schemas"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(fixture, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		if rel == filepath.Join("schemas", "index.json") {
			return nil
		}
		return os.WriteFile(dest, nil, 0644)
	})
	if err != nil {
		t.Fatal(err)
	}
	run := func() ([]byte, error) {
		return exec.Command("bash", filepath.Join(root, "scripts/contract-check-wiki.sh"), "--root", fixture).CombinedOutput()
	}
	if out, err := run(); err != nil {
		t.Fatalf("valid schema table rejected: %v\n%s", err, out)
	}
	page := filepath.Join(fixture, ".github/wiki/JSON-Output.md")
	data, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "| `schemas/lockfile.v1.schema.json` |") {
			lines = append(lines, line)
		}
	}
	if err := os.WriteFile(page, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := run(); err == nil || !strings.Contains(string(out), "schemas/lockfile.v1.schema.json is in schemas/index.json but has no row") {
		t.Fatalf("missing schema row was not diagnosed: %v\n%s", err, out)
	}
}
