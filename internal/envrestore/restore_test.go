package envrestore

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestRestoreBringsBackCapturedBytes: a write after Capture is undone, and a
// file the write created is removed again.
func TestRestoreBringsBackCapturedBytes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &File{Path: path}
	if err := f.Capture(); err != nil {
		t.Fatal(err)
	}
	f.ReleaseSignals()
	if err := os.WriteFile(path, []byte("A=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.Restore(); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "A=1\n" {
		t.Fatalf("not restored: %q", got)
	}

	fresh := &File{Path: filepath.Join(dir, ".env.new")}
	if err := fresh.Capture(); err != nil {
		t.Fatal(err)
	}
	fresh.ReleaseSignals()
	if err := os.WriteFile(fresh.Path, []byte("B=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Restore(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fresh.Path); !os.IsNotExist(err) {
		t.Fatalf("created file must be removed: %v", err)
	}
}

// TestRemoveLineUsesInjectedGrammar: the captured bytes go through without,
// and an absent file is left absent.
func TestRemoveLineUsesInjectedGrammar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("A=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := &File{Path: path}
	if err := f.Capture(); err != nil {
		t.Fatal(err)
	}
	f.ReleaseSignals()
	drop := func(prev []byte, key string) ([]byte, error) {
		return bytes.ReplaceAll(prev, []byte(key+"=1\n"), nil), nil
	}
	if err := f.RemoveLine("A", drop); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "B=2\n" {
		t.Fatalf("line not removed: %q", got)
	}

	absent := &File{Path: filepath.Join(dir, "none.env")}
	if err := absent.Capture(); err != nil {
		t.Fatal(err)
	}
	absent.ReleaseSignals()
	if err := absent.RemoveLine("A", drop); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(absent.Path); !os.IsNotExist(err) {
		t.Fatalf("absent file must stay absent: %v", err)
	}
}
