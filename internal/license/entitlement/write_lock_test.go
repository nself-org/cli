package entitlement

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/fslock"
)

// D-0947: the timer and a build both rewrite the token in place; writeAtomic serialises them on
// <state>/entitlement.lock so one never truncates under the other.
func TestWriteAtomicWaitsForTheEntitlementLock(t *testing.T) {
	path := Path(t.TempDir())
	if err := writeAtomic(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	release, err := fslock.Acquire(filepath.Join(filepath.Dir(path), "entitlement.lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- writeAtomic(path, []byte("second")) }()
	select {
	case err := <-done:
		t.Fatalf("writeAtomic finished while another writer held the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if got, _ := os.ReadFile(path); string(got) != "first" {
		t.Fatalf("token changed under the lock: %q", got)
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "second" {
		t.Fatalf("token after release = %q, want second", got)
	}
}
