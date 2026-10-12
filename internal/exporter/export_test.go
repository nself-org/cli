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
	secretOut := filepath.Join(dir, "FAKE-OUTPUT-PATH-SECRET")
	if _, err := Export(context.Background(), &config.Config{}, Options{Out: secretOut, SecretValues: []string{"FAKE-OUTPUT-PATH-SECRET"}}); err == nil || strings.Contains(err.Error(), "FAKE-OUTPUT-PATH-SECRET") {
		t.Fatal("output path could expose a configured secret")
	}
	if _, err := os.Lstat(secretOut); !os.IsNotExist(err) {
		t.Fatal("unsafe output path was written")
	}
	t.Chdir(dir)
	if _, err := Export(context.Background(), &config.Config{}, Options{Out: "relative-bundle", SecretValues: []string{dir}}); err == nil || strings.Contains(err.Error(), dir) {
		t.Fatal("absolute output summary could expose a configured secret")
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

func TestExportParentMustExist(t *testing.T) {
	dir := t.TempDir()
	_, err := Export(context.Background(), &config.Config{}, Options{Out: filepath.Join(dir, "missing", "bundle")})
	if err == nil || !strings.Contains(err.Error(), "parent directory") {
		t.Fatal("missing parent not reported clearly", err)
	}
	if _, serr := os.Lstat(filepath.Join(dir, "missing")); !os.IsNotExist(serr) {
		t.Fatal("export created the parent")
	}
}

func TestPublishPreservesOccupiedOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out")
	stage := filepath.Join(dir, "stage")
	if err := os.Mkdir(stage, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(out, []byte("fake existing file"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertCode(t, publishBundle(stage, out), "E527")
	data, err := os.ReadFile(out)
	if err != nil || string(data) != "fake existing file" {
		t.Fatal("existing output file changed")
	}
	if err = os.Remove(out); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(out, "keep"), []byte("fake existing member"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertCode(t, publishBundle(stage, out), "E527")
	if _, err = os.Stat(filepath.Join(out, "keep")); err != nil {
		t.Fatal("existing output member changed")
	}
	if err = os.Remove(filepath.Join(out, "keep")); err != nil {
		t.Fatal(err)
	}
	if err = publishBundle(stage, out); err != nil {
		t.Fatal("empty destination rejected", err)
	}
}
