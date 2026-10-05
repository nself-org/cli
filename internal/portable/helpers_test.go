package portable

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

var fixedNow = func() time.Time { return time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC) }

// baseManifest is a small valid manifest body (Files is filled by Finish).
func baseManifest() Manifest {
	return Manifest{
		Producer: Producer{Tool: "nself", Version: "1.0.0", Source: "nself"},
		Postgres: PostgresInfo{ServerVersion: "16.4", Major: 16},
		DB: DB{Schemas: []string{"public"}, Tables: []Table{
			{Schema: "public", Name: "notes", Rows: 2, Hash: "123", PK: []string{"id"}}}},
	}
}

// makeBundle writes files through the Writer and returns the bundle dir.
func makeBundle(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bundle")
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Now = fixedNow
	for rel, body := range files {
		if _, err := w.WriteFile(rel, strings.NewReader(body)); err != nil {
			t.Fatalf("WriteFile(%q): %v", rel, err)
		}
	}
	if _, err := w.Finish(baseManifest()); err != nil {
		t.Fatal(err)
	}
	return dir
}

// editManifest rewrites manifest.json through a generic map.
func editManifest(t *testing.T, dir string, edit func(m map[string]any)) {
	t.Helper()
	p := filepath.Join(dir, ManifestName)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	edit(m)
	out, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(p, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// setFiles replaces files[] with the given entries (path, sha256, bytes).
func setFiles(m map[string]any, entries ...map[string]any) {
	list := make([]any, len(entries))
	for i, e := range entries {
		list[i] = e
	}
	m["files"] = list
}

func fileEntry(path string, sum string, n int64) map[string]any {
	return map[string]any{"path": path, "sha256": sum, "bytes": n}
}

const zeroSum = "0000000000000000000000000000000000000000000000000000000000000000"

// wantCode asserts err is a CLIError with the code and wraps the sentinel.
func wantCode(t *testing.T, err error, code string, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil error", code)
	}
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != code {
		t.Fatalf("want code %s, got %v", code, err)
	}
	if sentinel != nil && !errors.Is(err, sentinel) {
		t.Fatalf("want %v wrapped, got %v", sentinel, err)
	}
}
