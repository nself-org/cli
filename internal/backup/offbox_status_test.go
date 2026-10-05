package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

// Purpose: tests for the off-box status read (P7-PROD-07, contract:cli.backup-status).
// Heartbeats come from path:// directories, so no network or tool is involved.

var fixtureBackupAt = time.Date(2026, 10, 5, 2, 31, 0, 0, time.UTC)

func fixtureRemote(t *testing.T) string {
	t.Helper()
	abs, err := filepath.Abs(filepath.Join("testdata", "offbox", "hb"))
	if err != nil {
		t.Fatal(err)
	}
	return "path://" + abs
}

// writeObject writes <dir>/proj/<kind>.json with the given raw content.
func writeObject(t *testing.T, dir, kind, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proj", kind+".json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// hbJSON renders a heartbeat object with the given overrides applied.
func hbJSON(t *testing.T, kind string, at time.Time, mut func(*Heartbeat)) string {
	t.Helper()
	hb := newBackupHeartbeat("proj", "proj_stream_1.sql.age", 10, true, map[string]int64{"public.a": 1}, at, "1.4.12")
	hb.Kind = kind
	if kind == "drill" {
		hb.ApproxRows, hb.RestoredRows = nil, map[string]int64{"public.a": 1}
	}
	if mut != nil {
		mut(&hb)
	}
	data, err := hb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func codeOf(t *testing.T, err error) []string {
	t.Helper()
	var codes []string
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range j.Unwrap() {
			codes = append(codes, codeOf(t, e)...)
		}
		return codes
	}
	var ce *errs.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("error is not coded: %v", err)
	}
	return []string{ce.Code}
}

func TestParseAge(t *testing.T) {
	good := map[string]time.Duration{
		"26h": 26 * time.Hour, "35d": 35 * 24 * time.Hour, "1d12h": 36 * time.Hour, "90m": 90 * time.Minute,
	}
	for in, want := range good {
		got, err := ParseAge(in)
		if err != nil || got != want {
			t.Errorf("ParseAge(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "0h", "-5h", "d", "abc", "5x", "1d-2h"} {
		if _, err := ParseAge(in); err == nil {
			t.Errorf("ParseAge(%q) accepted", in)
		}
	}
}

func TestOffboxFreshGolden(t *testing.T) {
	now := fixtureBackupAt.Add(2 * time.Hour)
	st, err := ReadOffbox(context.Background(), fixtureRemote(t), "proj", OffboxOptions{MaxAge: 26 * time.Hour, MaxDrillAge: 35 * 24 * time.Hour}, now)
	if err != nil {
		t.Fatalf("fresh fixture failed: %v", err)
	}
	if st.MaxAgeExceeded || st.MaxDrillAgeExceeded || st.Source != "heartbeat" {
		t.Fatalf("%+v", st)
	}
	out, err := FormatStatusOffbox(nil, st, "json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "offbox", "status_fresh.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if out+"\n" != string(want) {
		t.Errorf("offbox JSON drifted from the golden.\n--- got ---\n%s\n--- want ---\n%s", out, want)
	}
}

func TestOffboxThresholds(t *testing.T) {
	remote := fixtureRemote(t)
	ctx := context.Background()
	cases := []struct {
		name  string
		now   time.Time
		opts  OffboxOptions
		codes []string
		text  string
	}{
		{"backup 27h old", fixtureBackupAt.Add(27 * time.Hour), OffboxOptions{MaxAge: 26 * time.Hour}, []string{"E217"}, "27h0m0s old"},
		{"backup exactly at the limit", fixtureBackupAt.Add(26 * time.Hour), OffboxOptions{MaxAge: 26 * time.Hour}, nil, ""},
		{"drill 36 days old", time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC), OffboxOptions{MaxDrillAge: 35 * 24 * time.Hour}, []string{"E218"}, "old (limit 840h0m0s"},
		{"both stale", time.Date(2026, 10, 26, 3, 0, 0, 0, time.UTC), OffboxOptions{MaxAge: 26 * time.Hour, MaxDrillAge: 10 * 24 * time.Hour}, []string{"E217", "E218"}, ""},
		{"no thresholds never fails on age", time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), OffboxOptions{}, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, err := ReadOffbox(ctx, remote, "proj", c.opts, c.now)
			if got := strings.Join(codeOf(t, err), ","); got != strings.Join(c.codes, ",") {
				t.Fatalf("codes = %q, want %v (err %v)", got, c.codes, err)
			}
			if c.text != "" && !strings.Contains(err.Error(), c.text) {
				t.Errorf("error lacks %q: %v", c.text, err)
			}
			if st.BackupAgeSeconds == nil || *st.BackupAgeSeconds != int64(c.now.Sub(fixtureBackupAt).Seconds()) {
				t.Errorf("age not shown: %v", st.BackupAgeSeconds)
			}
			if err != nil && errs.ExitCodeFor(err) != 2 {
				t.Errorf("exit = %d, want 2", errs.ExitCodeFor(err))
			}
		})
	}
}

func TestOffboxFailsClosed(t *testing.T) {
	now := fixtureBackupAt.Add(time.Hour)
	ok := func(kind string) string { return hbJSON(t, kind, now.Add(-time.Hour), nil) }
	bad := map[string]string{
		"not json":       "{nope",
		"empty":          "",
		"wrong version":  hbJSON(t, "backup", now, func(h *Heartbeat) { h.SchemaVersion = "2" }),
		"wrong kind":     hbJSON(t, "backup", now, func(h *Heartbeat) { h.Kind = "drill" }),
		"wrong project":  hbJSON(t, "backup", now, func(h *Heartbeat) { h.Project = "other" }),
		"bad time":       hbJSON(t, "backup", now, func(h *Heartbeat) { h.At = "yesterday" }),
		"future time":    hbJSON(t, "backup", now.Add(2*time.Hour), nil),
		"unknown result": hbJSON(t, "backup", now, func(h *Heartbeat) { h.Result = "maybe" }),
		"oversize":       "{\"x\": \"" + strings.Repeat("a", maxHeartbeatBytes) + "\"}",
	}
	for name, content := range bad {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeObject(t, dir, "backup", content)
			writeObject(t, dir, "drill", ok("drill"))
			// No thresholds: an unreadable heartbeat still fails.
			_, err := ReadOffbox(context.Background(), "path://"+dir, "proj", OffboxOptions{}, now)
			if got := strings.Join(codeOf(t, err), ","); got != "E219" {
				t.Fatalf("codes = %q, err %v", got, err)
			}
		})
	}
	t.Run("missing heartbeat is stale", func(t *testing.T) {
		dir := t.TempDir()
		st, err := ReadOffbox(context.Background(), "path://"+dir, "proj", OffboxOptions{MaxAge: time.Hour, MaxDrillAge: time.Hour}, now)
		if got := strings.Join(codeOf(t, err), ","); got != "E217,E218" || !st.MaxAgeExceeded || !st.MaxDrillAgeExceeded {
			t.Fatalf("codes = %q, %+v", got, st)
		}
		if st.LastBackup != nil || st.BackupAgeSeconds != nil {
			t.Errorf("a missing heartbeat must report no age: %+v", st)
		}
	})
	t.Run("failed drill is not ok even when young", func(t *testing.T) {
		dir := t.TempDir()
		writeObject(t, dir, "backup", ok("backup"))
		writeObject(t, dir, "drill", hbJSON(t, "drill", now.Add(-time.Minute), func(h *Heartbeat) { h.Result = "failed" }))
		_, err := ReadOffbox(context.Background(), "path://"+dir, "proj", OffboxOptions{MaxDrillAge: 35 * 24 * time.Hour}, now)
		if got := strings.Join(codeOf(t, err), ","); got != "E218" || !strings.Contains(err.Error(), "failed") {
			t.Fatalf("codes = %q, err %v", got, err)
		}
	})
	t.Run("a threshold without a remote cannot pass", func(t *testing.T) {
		_, err := ReadOffbox(context.Background(), "", "proj", OffboxOptions{MaxAge: time.Hour}, now)
		if got := strings.Join(codeOf(t, err), ","); got != "E219" {
			t.Fatalf("codes = %q", got)
		}
		st, err := ReadOffbox(context.Background(), "", "proj", OffboxOptions{}, now)
		if err != nil || st.Source != "none" {
			t.Fatalf("%+v %v", st, err)
		}
	})
	t.Run("bad project name", func(t *testing.T) {
		if _, err := ReadOffbox(context.Background(), "path://"+t.TempDir(), "../x", OffboxOptions{}, now); err == nil {
			t.Fatal("a project name with a slash was accepted")
		}
	})
}

func TestFormatStatusOffboxKeepsExistingFields(t *testing.T) {
	info := &StatusInfo{LastRun: "never", NextRun: "0 2 * * *", Health: "warning", TotalSize: "0 B", RetentionDaily: 7}
	old, err := FormatStatus(info, "json")
	if err != nil {
		t.Fatal(err)
	}
	off := &OffboxStatus{Source: "none"}
	out, err := FormatStatusOffbox(info, off, "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, strings.TrimSuffix(old, "\n}")+",\n  \"offbox\": {") {
		t.Errorf("existing fields changed or offbox is not appended:\n%s\n--- old ---\n%s", out, old)
	}
	var v map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &v); err != nil || v["offbox"] == nil || v["last_run"] == nil {
		t.Fatalf("%v %s", err, out)
	}
	// Text: unchanged without a remote, two lines with one.
	text, _ := FormatStatusOffbox(info, off, "")
	oldText, _ := FormatStatus(info, "")
	if text != oldText {
		t.Errorf("text output changed without a heartbeat remote:\n%s", text)
	}
	st, _ := ReadOffbox(context.Background(), fixtureRemote(t), "proj", OffboxOptions{}, fixtureBackupAt.Add(time.Hour))
	text, _ = FormatStatusOffbox(info, st, "")
	if !strings.HasPrefix(text, oldText) || strings.Count(strings.TrimPrefix(text, oldText), "\n") != 2 ||
		!strings.Contains(text, "Off-box backup:") || !strings.Contains(text, "1h0m0s ago") {
		t.Errorf("want the old text plus two off-box lines:\n%s", text)
	}
}
