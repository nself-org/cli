package ledger

// Remove-intent tests (D-0368): round trip, strict parse, lock-only access.

import (
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

var intentStart = time.Date(2026, 10, 11, 9, 8, 7, 0, time.UTC)

func TestIntentRoundTripSortedAndCleared(t *testing.T) {
	s := newTestStore(t)
	err := s.Locked(func(tx *Tx, _ Ledger) error {
		if _, ok, err := tx.RemoveIntent("chat"); err != nil || ok {
			t.Fatalf("empty store: ok=%v err=%v", ok, err)
		}
		if err := tx.ClearRemoveIntent("chat"); err != nil {
			t.Fatalf("clear with no file: %v", err)
		}
		if _, err := os.Stat(s.IntentPath()); !os.IsNotExist(err) {
			t.Fatalf("clear created a file: %v", err)
		}
		if err := tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart, Remaining: []string{"b", "a", "b"}}); err != nil {
			t.Fatal(err)
		}
		if err := tx.SetRemoveIntent("claw", RemoveIntent{StartedAt: intentStart, Remaining: []string{"z"}}); err != nil {
			t.Fatal(err)
		}
		in, ok, err := tx.RemoveIntent("chat")
		if err != nil || !ok {
			t.Fatalf("read back: ok=%v err=%v", ok, err)
		}
		if strings.Join(in.Remaining, ",") != "a,b" || !in.StartedAt.Equal(intentStart) {
			t.Errorf("got %+v, want sorted unique [a b] at %v", in, intentStart)
		}
		if err := tx.ClearRemoveIntent("chat"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(s.IntentPath()); err != nil {
			t.Fatalf("claw's intent must keep the file: %v", err)
		}
		if _, ok, _ := tx.RemoveIntent("claw"); !ok {
			t.Error("clearing chat dropped claw")
		}
		if err := tx.ClearRemoveIntent("claw"); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(s.IntentPath()); !os.IsNotExist(err) {
			t.Fatalf("the file must go when the last intent is cleared: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntentFileShape(t *testing.T) {
	s := newTestStore(t)
	_ = s.Locked(func(tx *Tx, _ Ledger) error {
		return tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart.In(time.FixedZone("x", 3600)), Remaining: []string{"a"}})
	})
	data, err := os.ReadFile(s.IntentPath())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{\n  \"_generated\": ") || !strings.Contains(string(data), `"started_at": "2026-10-11T09:08:07Z"`) {
		t.Errorf("unexpected file:\n%s", data)
	}
	if st, err := os.Stat(s.IntentPath()); err != nil || (runtime.GOOS != "windows" && st.Mode().Perm() != 0o644) {
		t.Errorf("mode = %v, %v; want 0644", st.Mode(), err)
	}
}

func TestIntentStrictParse(t *testing.T) {
	head := `{"_generated":"` + Generated + `","schema_version":1,"bundles":`
	ok := head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":["a"]}}}`
	if _, err := parseIntents([]byte(ok)); err != nil {
		t.Fatalf("valid file rejected: %v", err)
	}
	cases := []struct{ name, doc, want string }{
		{"unknown root key", head + `{},"extra":1}`, "unknown key"},
		{"unknown record key", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":[],"x":1}}}`, "unknown key"},
		{"wrong-case key", head + `{"chat":{"STARTED_AT":"2026-10-11T09:08:07Z","remaining":[]}}}`, "unknown key"},
		{"duplicate key", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":[],"remaining":[]}}}`, "duplicate"},
		{"bad bundle slug", head + `{"Bad Slug":{"started_at":"2026-10-11T09:08:07Z","remaining":[]}}}`, "slug"},
		{"bad member slug", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":["../x"]}}}`, "remaining"},
		{"repeated member", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":["a","a"]}}}`, "twice"},
		{"bad time", head + `{"chat":{"started_at":"yesterday","remaining":[]}}}`, "RFC 3339"},
		{"missing remaining", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z"}}}`, "needs"},
		{"null remaining", head + `{"chat":{"started_at":"2026-10-11T09:08:07Z","remaining":null}}}`, "needs"},
		{"null bundles", `{"_generated":"` + Generated + `","schema_version":1,"bundles":null}`, "bundles"},
		{"wrong generated", `{"_generated":"hand","schema_version":1,"bundles":{}}`, "_generated"},
		{"wrong version", `{"_generated":"` + Generated + `","schema_version":2,"bundles":{}}`, "schema_version"},
		{"generated not first", `{"schema_version":1,"_generated":"` + Generated + `","bundles":{}}`, "_generated"},
		{"trailing data", ok + `{}`, "trailing"},
		{"not an object", `[]`, "object"},
		{"empty", ``, "object"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseIntents([]byte(c.doc))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want it to mention %q", err, c.want)
			}
		})
	}
}

func TestIntentOversizeAndBadFileNameTheFile(t *testing.T) {
	s := newTestStore(t)
	if err := s.Update(addPlugin("a")); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"oversize": strings.Repeat(" ", MaxBytes+1),
		"garbage":  "not json",
	} {
		if err := os.WriteFile(s.IntentPath(), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		err := s.Locked(func(tx *Tx, _ Ledger) error {
			_, _, err := tx.RemoveIntent("chat")
			return err
		})
		if err == nil || !strings.Contains(err.Error(), s.IntentPath()) {
			t.Errorf("%s: err = %v, want it to name %s", name, err, s.IntentPath())
		}
		// A write must not paper over a damaged file either.
		err = s.Locked(func(tx *Tx, _ Ledger) error {
			return tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart})
		})
		if err == nil {
			t.Errorf("%s: SetRemoveIntent replaced a damaged file", name)
		}
	}
}

func TestSetRemoveIntentRefusesBadNames(t *testing.T) {
	s := newTestStore(t)
	_ = s.Locked(func(tx *Tx, _ Ledger) error {
		if err := tx.SetRemoveIntent("Bad Slug", RemoveIntent{StartedAt: intentStart}); err == nil {
			t.Error("bad bundle slug accepted")
		}
		if err := tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart, Remaining: []string{"a/b"}}); err == nil {
			t.Error("bad member accepted")
		}
		return nil
	})
	if _, err := os.Stat(s.IntentPath()); !os.IsNotExist(err) {
		t.Errorf("a refused set wrote a file: %v", err)
	}
}

func TestIntentWriteFailureLeavesOldFile(t *testing.T) {
	s := newTestStore(t)
	_ = s.Locked(func(tx *Tx, _ Ledger) error {
		return tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart, Remaining: []string{"a", "b"}})
	})
	before, _ := os.ReadFile(s.IntentPath())
	s.SetWriteHookForTest(func(path string) error {
		if path == s.IntentPath() {
			return os.ErrPermission
		}
		return nil
	})
	err := s.Locked(func(tx *Tx, _ Ledger) error {
		return tx.SetRemoveIntent("chat", RemoveIntent{StartedAt: intentStart, Remaining: []string{"b"}})
	})
	if err == nil {
		t.Fatal("hook error swallowed")
	}
	after, _ := os.ReadFile(s.IntentPath())
	if string(before) != string(after) {
		t.Error("a failed write changed the intent file")
	}
}

func TestLedgerFormatUnchangedByIntent(t *testing.T) {
	// The ledger Parse still rejects an "intent" key: the ledger format is v1.
	doc := `{"_generated":"` + Generated + `","schema_version":1,"bundles":{},"plugins":{},"remove_intent":{}}`
	if _, err := Parse([]byte(doc)); err == nil {
		t.Error("ledger Parse accepted an unknown key")
	}
}
