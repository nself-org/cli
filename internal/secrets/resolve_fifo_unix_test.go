//go:build unix

package secrets

// Purpose: prove a FIFO identity path is refused without blocking.
// Inputs: SECRETS_AGE_KEY_PATH naming a FIFO with no writer.
// Outputs: E543 within a short deadline.
// Constraints: unix only (mkfifo). SPORT: P7-TRUST-03 (Codex pre-merge).

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestReadableIdentityRefusesFIFO proves the identity check stats before it opens.
func TestReadableIdentityRefusesFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "key.fifo")
	if err := syscall.Mkfifo(p, 0600); err != nil {
		t.Skip("mkfifo unsupported:", err)
	}
	t.Setenv("SECRETS_AGE_KEY_PATH", p)
	done := make(chan error, 1)
	go func() {
		_, err := readableIdentity()
		done <- err
	}()
	select {
	case err := <-done:
		trustCode(t, err, "E543")
	case <-time.After(3 * time.Second):
		t.Fatal("readableIdentity blocked on a FIFO")
	}
}
