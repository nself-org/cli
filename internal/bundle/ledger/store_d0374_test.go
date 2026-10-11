package ledger

// Store tests for D-0374: the write size cap, directory fsync errors and a
// bootstrap scan that could not read every plugin manifest.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLedgerWriteRefusesOversize: a ledger that would serialize over MaxBytes is
// refused before the temp file exists, and the previous file stays loadable.
func TestLedgerWriteRefusesOversize(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update(addPlugin("first")); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	huge := strings.Repeat("9", MaxBytes)
	err = s.Update(func(l *Ledger) error {
		l.Plugins["big"] = PluginRecord{InstalledBy: []string{}, Explicit: true, Tier: TierFree, Version: huge}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to write") {
		t.Fatalf("oversize Update = %v, want a refusal", err)
	}
	if after, _ := os.ReadFile(s.Path()); string(after) != string(before) {
		t.Fatal("refused write changed the ledger")
	}
	if tmps, _ := filepath.Glob(filepath.Join(s.Dir(), ".bundles.json.*.tmp")); len(tmps) != 0 {
		t.Fatalf("temp files left behind: %v", tmps)
	}
	if _, _, err := s.Load(); err != nil {
		t.Fatalf("ledger no longer loads after a refused write: %v", err)
	}
}

// TestLedgerUnreadableBootstrapNotPersisted: a bootstrap scan that could not read
// a manifest is never written (Update, EnsureBootstrapped), while Load still
// shows it and warns.
func TestLedgerUnreadableBootstrapNotPersisted(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Sources = func() (Sources, error) {
		src := fixtureSources()
		src.Unreadable = []string{"broken: plugin.json: invalid character"}
		return src, nil
	}
	var warned []string
	s.Warn = func(m string) { warned = append(warned, m) }

	if err := s.Update(addPlugin("x")); err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("Update = %v, want an error naming the unreadable manifest", err)
	}
	if _, err := s.EnsureBootstrapped(); err == nil {
		t.Fatal("EnsureBootstrapped persisted an incomplete scan")
	}
	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("a ledger file was written: %v", err)
	}
	l, persisted, err := s.Load()
	if err != nil || persisted || len(l.Bundles) == 0 {
		t.Fatalf("Load = persisted %v err %v bundles %d", persisted, err, len(l.Bundles))
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "broken") {
		t.Fatalf("warnings = %q", warned)
	}
}
