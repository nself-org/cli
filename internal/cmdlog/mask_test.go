package cmdlog

// Purpose: prove secret values never reach the command log file (D-0473).
// Inputs: planted fake values. Outputs: pass/fail.
// Constraints: reads the real log file in a temp dir; positive controls prove
// the scan could have found the value.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const planted = "FAKE-planted-0468-value"

func resetMask(t *testing.T) {
	t.Helper()
	maskMu.Lock()
	maskValues = nil
	maskMu.Unlock()
	t.Cleanup(func() {
		maskMu.Lock()
		maskValues = nil
		maskMu.Unlock()
	})
}

func readEntries(t *testing.T, dir string) (string, []Entry) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "nself.log"))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	var out []Entry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		out = append(out, e)
	}
	return string(data), out
}

func TestPlantedValueNeverInLogFile(t *testing.T) {
	resetMask(t)
	dir := t.TempDir()
	l := New(dir)
	// Via the variadic argument.
	l.Begin([]string{"nself", "secrets", "set", "FAKEKEY", planted, "--note=" + planted}, planted)(0, nil)
	// Via MaskValues, with the value also echoed in the error text.
	MaskValues(planted + "-two")
	l.Begin([]string{"nself", "config", "set", "somekey", planted + "-two"})(1, errors.New("bad value "+planted+"-two"))

	raw, entries := readEntries(t, dir)
	if strings.Contains(raw, planted) {
		t.Fatalf("planted value reached the log file:\n%s", raw)
	}
	if !strings.Contains(raw, "[REDACTED]") {
		t.Fatalf("no [REDACTED] marker in the log:\n%s", raw)
	}
	if len(entries) != 2 || !strings.Contains(entries[0].Command, "secrets set FAKEKEY [REDACTED] --note=[REDACTED]") {
		t.Fatalf("non-secret arg missing or wrong shape (positive control): %+v", entries)
	}
	if entries[1].Error != "bad value [REDACTED]" {
		t.Errorf("error = %q, want it scrubbed", entries[1].Error)
	}
}

func TestShortSecretExactMatchOnly(t *testing.T) {
	resetMask(t)
	got := redactArgsWith([]string{"nself", "set", "ab", "--x=ab", "cabd"}, []string{"ab"})
	if got != "nself set [REDACTED] --x=[REDACTED] cabd" {
		t.Errorf("got %q", got)
	}
	if got := scrubText("ab", secretSet([]string{"ab"})); got != "[REDACTED]" {
		t.Errorf("whole short error = %q", got)
	}
	if got := scrubText("cabd", secretSet([]string{"ab"})); got != "cabd" {
		t.Errorf("short value must not substring-match, got %q", got)
	}
}

func TestEmptySecretIgnored(t *testing.T) {
	resetMask(t)
	MaskValues("")
	if got := redactArgsWith([]string{"nself", "start"}, []string{""}); got != "nself start" {
		t.Errorf("got %q", got)
	}
}

func TestFlagNameRulesStillApply(t *testing.T) {
	resetMask(t)
	got := redactArgsWith([]string{"nself", "x", "--token", planted, "--password=" + planted}, nil)
	if strings.Contains(got, planted) {
		t.Errorf("flag-name rule lost: %q", got)
	}
}

// TestEntryCarriesNoOutput pins the shape: argv and error only, so command
// output (for example a --reveal result) has no path into the log.
func TestEntryCarriesNoOutput(t *testing.T) {
	b, err := json.Marshal(Entry{})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ts", "cmd", "user", "dur_s", "exit"} {
		delete(m, k)
	}
	delete(m, "error")
	if len(m) != 0 {
		t.Errorf("Entry has fields beyond argv and error: %v", m)
	}
}

func TestLogFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix modes only")
	}
	resetMask(t)
	dir := filepath.Join(t.TempDir(), "logs")
	l := New(dir)
	l.Begin([]string{"nself", "start"})(0, nil)
	fi, err := os.Stat(filepath.Join(dir, "nself.log"))
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("log mode = %v (err %v), want 0600", fi.Mode().Perm(), err)
	}
	di, _ := os.Stat(dir)
	if di.Mode().Perm() != 0o700 {
		t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
	}

	// An older world-readable log is narrowed on open, and a 0400 one is not widened.
	path := filepath.Join(dir, "nself.log")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	l.Begin([]string{"nself", "stop"})(0, nil)
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("existing log mode = %v, want 0600", fi.Mode().Perm())
	}
	if err := os.Chmod(path, 0o200); err != nil {
		t.Fatal(err)
	}
	l.Begin([]string{"nself", "stop"})(0, nil)
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o200 {
		t.Errorf("narrow log mode = %v, want 0200 unchanged", fi.Mode().Perm())
	}
}
