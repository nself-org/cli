package ledger

// Store tests (P7-PLUG-18): atomic replace, crash safety, writer lock, modes.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s := NewStore(t.TempDir())
	s.Sources = fixtureSources
	return s
}

func addPlugin(slug string) func(*Ledger) error {
	return func(l *Ledger) error {
		l.Plugins[slug] = PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: TierFree, Version: "1.0.0"}
		return nil
	}
}

// TestLedgerCrashKeepsPrevious: a failure between the temp write and the
// rename leaves the previous ledger byte-identical and no temp file behind.
func TestLedgerCrashKeepsPrevious(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update(addPlugin("first")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}

	reached := false
	s.beforeRename = func() error { reached = true; return errors.New("injected crash") }
	err = s.Update(addPlugin("second"))
	if err == nil || !strings.Contains(err.Error(), "injected crash") {
		t.Fatalf("Update did not surface the injected failure: %v", err)
	}
	if !reached {
		t.Fatal("the crash hook never ran: the test proved nothing")
	}
	after, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("ledger changed across a failed write:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(string(after), "second") {
		t.Fatal("the failed update leaked into the ledger")
	}
	left, _ := filepath.Glob(filepath.Join(s.Dir(), "*.tmp"))
	if len(left) != 0 {
		t.Fatalf("temp file left behind: %v", left)
	}

	// The lock was released: the next write works.
	s.beforeRename = nil
	if err := s.Update(addPlugin("third")); err != nil {
		t.Fatalf("write after a failed write: %v", err)
	}
	l, persisted, err := s.Load()
	if err != nil || !persisted {
		t.Fatalf("load: persisted=%v err=%v", persisted, err)
	}
	if _, ok := l.Plugins["third"]; !ok || l.Plugins["second"].Tier != "" {
		t.Fatalf("unexpected ledger after recovery: %+v", l.Plugins)
	}
}

// TestLedgerFnErrorWritesNothing: an fn error aborts before any file exists.
func TestLedgerFnErrorWritesNothing(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update(func(*Ledger) error { return errors.New("no") }); err == nil {
		t.Fatal("fn error was swallowed")
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("ledger written despite fn error: %v", err)
	}
	// An fn that produces an invalid ledger is refused before the rename.
	err := s.Update(func(l *Ledger) error {
		l.Plugins["orphan"] = PluginRecord{InstalledBy: []string{}, Tier: TierFree}
		return nil
	})
	if err == nil {
		t.Fatal("invalid ledger was written")
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatal("ledger file exists after an invalid update")
	}
}

// TestLedgerConcurrentWriters: many writers, each in its own Store (its own
// file descriptor), all read-modify-write the same ledger. Every change must
// survive; fn sleeps inside the critical section so a missing lock loses updates.
func TestLedgerConcurrentWriters(t *testing.T) {
	root := t.TempDir()
	const n = 24
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := NewStore(root)
			errs <- s.Update(func(l *Ledger) error {
				time.Sleep(2 * time.Millisecond)
				return addPlugin(fmt.Sprintf("p%02d", i))(l)
			})
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("writer failed: %v", err)
		}
	}
	l, persisted, err := NewStore(root).Load()
	if err != nil || !persisted {
		t.Fatalf("load: persisted=%v err=%v", persisted, err)
	}
	if len(l.Plugins) != n {
		t.Fatalf("lost updates: %d of %d plugins survived", len(l.Plugins), n)
	}
}

// TestLedgerConcurrentProcesses: the same property across real processes.
func TestLedgerConcurrentProcesses(t *testing.T) {
	root := t.TempDir()
	const n = 6
	var cmds []*exec.Cmd
	for i := 0; i < n; i++ {
		c := exec.Command(os.Args[0], "-test.run=^TestLedgerHelperWriter$")
		c.Env = append(os.Environ(), "LEDGER_HELPER_ROOT="+root, fmt.Sprintf("LEDGER_HELPER_SLUG=proc%d", i))
		if err := c.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, c)
	}
	for i, c := range cmds {
		if err := c.Wait(); err != nil {
			t.Fatalf("helper %d: %v", i, err)
		}
	}
	l, _, err := NewStore(root).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Plugins) != n {
		t.Fatalf("lost updates across processes: %d of %d", len(l.Plugins), n)
	}
}

// TestLedgerHelperWriter is the child of TestLedgerConcurrentProcesses; it is
// a no-op in a normal run.
func TestLedgerHelperWriter(t *testing.T) {
	root, slug := os.Getenv("LEDGER_HELPER_ROOT"), os.Getenv("LEDGER_HELPER_SLUG")
	if root == "" {
		t.Skip("helper process only")
	}
	err := NewStore(root).Update(func(l *Ledger) error {
		time.Sleep(20 * time.Millisecond)
		return addPlugin(slug)(l)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestLedgerLoadIsReadOnly: Load of a missing ledger bootstraps in memory and
// writes nothing, not even the directory; EnsureBootstrapped persists once.
func TestLedgerLoadIsReadOnly(t *testing.T) {
	s := newTestStore(t)
	l, persisted, err := s.Load()
	if err != nil || persisted || len(l.Bundles) != 2 {
		t.Fatalf("Load = persisted %v err %v bundles %d", persisted, err, len(l.Bundles))
	}
	if _, err := os.Stat(filepath.Join(s.root, ".nself")); !os.IsNotExist(err) {
		t.Fatal("Load created .nself")
	}
	created, err := s.EnsureBootstrapped()
	if err != nil || !created {
		t.Fatalf("first EnsureBootstrapped: created=%v err=%v", created, err)
	}
	if err := s.Update(addPlugin("later")); err != nil {
		t.Fatal(err)
	}
	created, err = s.EnsureBootstrapped()
	if err != nil || created {
		t.Fatalf("second EnsureBootstrapped: created=%v err=%v", created, err)
	}
	l2, persisted, _ := s.Load()
	if !persisted {
		t.Fatal("ledger not persisted")
	}
	if _, ok := l2.Plugins["later"]; !ok {
		t.Fatal("EnsureBootstrapped overwrote an existing ledger")
	}
	if st, err := os.Stat(s.Path()); err != nil || st.Mode().Perm() != 0o644 {
		t.Fatalf("ledger mode = %v, %v; want 0644", st.Mode(), err)
	}
}

// TestLedgerDamagedFileIsAnError: a damaged or hand-edited ledger is reported,
// never replaced by a default.
func TestLedgerDamagedFileIsAnError(t *testing.T) {
	for name, content := range map[string]string{
		"garbage":     "not json",
		"empty":       "",
		"hand-edited": `{"_generated":"x","schema_version":1,"bundles":{},"plugins":{}}`,
		"future":      `{"_generated":"` + Generated + `","schema_version":2,"bundles":{},"plugins":{}}`,
	} {
		s := newTestStore(t)
		if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.Path(), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Load(); err == nil {
			t.Errorf("%s: Load accepted a damaged ledger", name)
		}
		if err := s.Update(addPlugin("x")); err == nil {
			t.Errorf("%s: Update overwrote a damaged ledger", name)
		}
		if got, _ := os.ReadFile(s.Path()); string(got) != content {
			t.Errorf("%s: damaged file was modified", name)
		}
	}
}
