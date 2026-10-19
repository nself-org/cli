//go:build !windows

package verify

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestPromoteReleaseFIFOTrustFile: a FIFO at a trust path must not block the
// verifier; it refuses with E493.
func TestPromoteReleaseFIFOTrustFile(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	dir := filepath.Join(r.home, ".nself", "ci", "trust")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, keysName), 0o600); err != nil {
		t.Skipf("no FIFO support: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := Verify(context.Background(), r.input()); done <- err }()
	select {
	case err := <-done:
		if codeOf(err) != "E493" {
			t.Fatalf("FIFO trust file: %v, want E493", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a FIFO at the trust path blocked the verifier")
	}
}

func TestPromoteReleaseFIFOReleaseFile(t *testing.T) {
	r := newRelease(t)
	r.write(PayloadType, r.signer)
	path := filepath.Join(r.dir, EvidenceFile)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("no FIFO support: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := Verify(context.Background(), r.input()); done <- err }()
	select {
	case err := <-done:
		if codeOf(err) != "E497" {
			t.Fatalf("FIFO evidence: %v, want E497", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a FIFO release file blocked the verifier")
	}
}
