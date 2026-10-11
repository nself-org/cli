package vectorsearch

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

// update rewrites the golden files: go test ./internal/database/vectorsearch -update
var update = flag.Bool("update", false, "rewrite golden files")

// assertGolden compares got with testdata/golden/<name> byte for byte.
func assertGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create it)", path, err)
	}
	if string(want) != string(got) {
		t.Errorf("%s differs from the golden file\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
