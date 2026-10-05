package manifestv2_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// mutate decodes a fixture, lets fn edit the generic document, and re-encodes it.
func mutate(t *testing.T, rel string, fn func(doc map[string]any)) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(readFixture(t, rel), &doc); err != nil {
		t.Fatal(err)
	}
	fn(doc)
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// codeOf returns the errs code of err, or "" for an uncoded error.
func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func wantCode(t *testing.T, err error, code string, mentions ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s, got nil error", code)
	}
	if got := codeOf(err); got != code {
		t.Fatalf("want %s, got %q: %v", code, got, err)
	}
	for _, m := range mentions {
		if !strings.Contains(err.Error(), m) {
			t.Fatalf("error does not name %q: %v", m, err)
		}
	}
}

func fixtures(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("testdata", dir, "*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures in testdata/%s", dir)
	}
	return files
}
