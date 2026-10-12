package ledger

// Tx tests (D-0368): Locked takes the lock and loads, writes only on request,
// Write is durable while the lock is held, Update keeps its behaviour, and the
// write hook and stale-temp sweep cover the second file.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockedWritesNothingUnlessAsked(t *testing.T) {
	s := newTestStore(t)
	called := false
	err := s.Locked(func(tx *Tx, l Ledger) error {
		called = true
		if l.Bundles == nil || l.Plugins == nil {
			t.Error("Locked passed an unusable ledger")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("Locked: called=%v err=%v", called, err)
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("Locked wrote the ledger without Write: %v", err)
	}
}

func TestLockedWriteIsDurableBeforeFnReturns(t *testing.T) {
	s := newTestStore(t)
	err := s.Locked(func(tx *Tx, l Ledger) error {
		l.Plugins["first"] = PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: TierFree}
		if err := tx.Write(l); err != nil {
			return err
		}
		// A reader (no lock needed) already sees the checkpoint.
		got, persisted, err := s.Load()
		if err != nil || !persisted {
			t.Fatalf("Load inside Locked: persisted=%v err=%v", persisted, err)
		}
		if _, ok := got.Plugins["first"]; !ok {
			t.Error("checkpoint not visible to a reader")
		}
		l.Plugins["second"] = PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: TierFree}
		return errors.New("stop after the checkpoint")
	})
	if err == nil {
		t.Fatal("fn error was swallowed")
	}
	got, _, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Plugins["first"]; !ok {
		t.Error("the checkpoint was lost when fn failed")
	}
	if _, ok := got.Plugins["second"]; ok {
		t.Error("an unwritten change reached the disk")
	}
}

func TestLockedSerialisesWithUpdate(t *testing.T) {
	needsFileLock(t)
	s := newTestStore(t)
	inside := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.Locked(func(tx *Tx, l Ledger) error {
			close(inside)
			<-release
			l.Plugins["held"] = PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: TierFree}
			return tx.Write(l)
		})
	}()
	<-inside
	updated := make(chan error, 1)
	go func() { updated <- s.Update(addPlugin("other")) }()
	select {
	case <-updated:
		t.Fatal("Update ran while Locked held the lock")
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Load()
	if _, ok := got.Plugins["held"]; !ok {
		t.Error("lost the Locked write")
	}
	if _, ok := got.Plugins["other"]; !ok {
		t.Error("lost the Update write")
	}
}

func TestWriteHookFailsOnlyTheTargetedWrite(t *testing.T) {
	s := newTestStore(t)
	boom := errors.New("disk full")
	n := 0
	s.SetWriteHookForTest(func(path string) error {
		n++
		if n == 2 {
			return boom
		}
		return nil
	})
	err := s.Locked(func(tx *Tx, l Ledger) error {
		if err := tx.Write(l); err != nil {
			return err
		}
		return tx.Write(l)
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the hook error", err)
	}
	if _, _, err := s.Load(); err != nil {
		t.Fatalf("the first write must have landed intact: %v", err)
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatal(err)
	}
}

func TestStaleIntentTempIsSwept(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update(addPlugin("a")); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(s.Dir(), ".remove-intent.json.123.tmp")
	if err := os.WriteFile(stale, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	if err := s.Update(addPlugin("b")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale intent temp survived: %v", err)
	}
}
