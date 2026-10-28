//go:build !windows

package oplock

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestReleaseInvalidatesHolderToken(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(context.Background(), dir, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	l.Release()
	raw, err := os.ReadFile(LockPath(dir))
	if err != nil {
		t.Fatal(err)
	}
	var h Holder
	if err := json.Unmarshal(raw, &h); err != nil {
		t.Fatal(err)
	}
	if h.Token != "" {
		t.Fatal("released holder retains its re-entry token")
	}
}

func TestStderrNullIsNotTerminal(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := Stderr
	Stderr = f
	t.Cleanup(func() { Stderr = old })
	if stderrIsTerminal() {
		t.Fatal("null device is not a terminal")
	}
}
