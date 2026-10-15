//go:build !windows

package oplock

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestDebtReleaseInvalidatesToken(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(context.Background(), dir, Opts{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Release)
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
		t.Fatal("released holder retains re-entry token")
	}
	want := l.Holder()
	want.Token = ""
	if h != want {
		t.Fatalf("release changed holder history: got %+v, want %+v", h, want)
	}
}

func TestDebtOtherHostCannotReenter(t *testing.T) {
	host, _ := os.Hostname()
	h := Holder{Host: host + "-elsewhere", PID: os.Getpid(), Token: "old-token"}
	dir := heldWithRecord(t, h)
	l, err := Acquire(context.Background(), dir, Opts{Getenv: tokenEnv(h.Token)})
	if l != nil {
		l.Release()
	}
	if !errors.Is(err, ErrHeld) {
		t.Fatalf("other-host token re-entered: %v", err)
	}
}

func TestDebtNullIsNotTerminal(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := Stderr
	Stderr = f
	t.Cleanup(func() { Stderr = old })
	if stderrIsTerminal() {
		t.Fatal("null character device detected as a terminal")
	}
}
