//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package invoke

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestAuditWritableTakesTheLock pins that the pre-run probe refuses when another
// process holds the audit flock, and passes (releasing its own lock) otherwise.
func TestAuditWritableTakesTheLock(t *testing.T) {
	old := lockWait
	lockWait = 100 * time.Millisecond
	t.Cleanup(func() { lockWait = old })
	path := filepath.Join(t.TempDir(), "machine-audit.jsonl")
	g := NewGuard(path)
	if err := g.auditWritable(); err != nil {
		t.Fatalf("positive control: %v", err)
	}
	if err := g.auditWritable(); err != nil {
		t.Fatalf("the probe must release its lock: %v", err)
	}
	holder, err := os.OpenFile(path, os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := g.auditWritable(); err == nil {
		t.Fatal("auditWritable passed while another descriptor held the lock")
	}
	_ = syscall.Flock(int(holder.Fd()), syscall.LOCK_UN)
	if err := g.auditWritable(); err != nil {
		t.Errorf("after release: %v", err)
	}
}
