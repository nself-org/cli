package entitlement

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteNewRenameFailureCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "occupied")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeNew(target, []byte("private-token")); err == nil {
		t.Fatal("rename over a directory must report failure")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != "occupied" || !entries[0].IsDir() {
		t.Fatalf("failed publication changed target or leaked temporary token: %v %v", entries, err)
	}
}

func TestWriteExistingTruncatesLongerToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	for _, token := range []string{"long-original-token", "short", ""} {
		if err := writeAtomic(path, []byte(token)); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != token {
			t.Fatalf("refresh left stale token bytes: got %q want %q: %v", data, token, err)
		}
	}
}
