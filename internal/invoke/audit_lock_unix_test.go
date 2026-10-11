//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package invoke

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestAuditLockWaitBounded pins that a lock held elsewhere delays an audit
// write by at most lockWait and then fails, instead of hanging every audited
// call behind the process-wide mutex.
func TestAuditLockWaitBounded(t *testing.T) {
	old := lockWait
	lockWait = 150 * time.Millisecond
	t.Cleanup(func() { lockWait = old })
	path := filepath.Join(t.TempDir(), "machine-audit.jsonl")
	holder, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	err = NewGuard(path).appendLine([]byte("{}\n"))
	if err == nil {
		t.Fatal("appendLine wrote while another descriptor held the lock")
	}
	if waited := time.Since(start); waited > 3*time.Second {
		t.Errorf("waited %v for a held lock", waited)
	}
	_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)
	if err := NewGuard(path).appendLine([]byte("{}\n")); err != nil {
		t.Errorf("after release: %v", err)
	}
}
