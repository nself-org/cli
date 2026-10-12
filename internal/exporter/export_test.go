package exporter

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestExportPreflightNoWrites(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle")
	if _, err := Export(context.Background(), nil, Options{Out: out}); err == nil {
		t.Fatal("nil config accepted")
	}
	if _, err := Export(context.Background(), &config.Config{}, Options{}); err == nil {
		t.Fatal("empty output accepted")
	}
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "keep"), []byte("fake original content"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Export(context.Background(), &config.Config{}, Options{Out: out})
	assertCode(t, err, "E527")
	data, err := os.ReadFile(filepath.Join(out, "keep"))
	if err != nil || string(data) != "fake original content" {
		t.Fatal("occupied output changed")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("preflight created a stage")
	}
	if strings.Contains(safeFailure("postgres tool").Error(), "password") {
		t.Fatal("unsafe failure")
	}
}
