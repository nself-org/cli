package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

// rcloneStub puts an rclone double first on PATH. It appends its argv to the
// returned log and exits with $RCLONE_EXIT after printing $RCLONE_OUT to
// stderr; RCLONE_SLEEP makes it hang.
func rcloneStub(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH doubles are POSIX shell scripts")
	}
	dir := t.TempDir()
	body := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"" + filepath.Join(dir, "calls.log") + "\"\n" +
		"[ -n \"$RCLONE_SLEEP\" ] && exec sleep 30\n[ -n \"$RCLONE_OUT\" ] && printf '%s\\n' \"$RCLONE_OUT\" >&2\nexit ${RCLONE_EXIT:-0}\n"
	if err := os.WriteFile(filepath.Join(dir, "rclone"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(dir, "calls.log")
}

// Hostile --from / --heartbeat-to values are refused before any exec: the stub
// rclone must never run, for the status and the drill paths alike.
func TestHostileRemotesRefusedBeforeExec(t *testing.T) {
	calls := rcloneStub(t)
	for _, u := range []string{
		"--config=/etc/passwd", "-v", "--sftp-ssh=evil", ":sftp,host=127.0.0.1,user=x:/",
		"file:///etc", "http://127.0.0.1:1/x",
	} {
		st, err := ReadOffbox(context.Background(), u, "proj", OffboxOptions{MaxAge: time.Hour}, time.Now())
		if strings.Join(codeOf(t, err), ",") != "E219" || !strings.Contains(err.Error(), "cannot be opened") {
			t.Errorf("status %q: %v", u, err)
		}
		if len(st.Problems) != 1 {
			t.Errorf("status %q problems: %v", u, st.Problems)
		}
		e := newDrillEnv(t)
		for _, o := range []DrillRemoteOptions{
			{Project: "proj", From: u, Identity: e.identity},
			{Project: "proj", From: "path://" + e.src, Identity: e.identity, HeartbeatTo: u},
		} {
			if _, err := DrillRemote(context.Background(), o); !errors.Is(err, errs.ErrBackupRemoteFailed) {
				t.Errorf("drill %+v: %v", o, err)
			}
		}
	}
	if b, err := os.ReadFile(calls); err == nil {
		t.Fatalf("rclone was executed: %s", b)
	}
}

// A missing heartbeat is told apart from an unreachable remote by rclone's exit
// code, never by words in its output (the review's "Config file not found"
// NOTICE must not turn a dead remote into "no backup").
func TestRcloneFailuresClassifiedByExitCode(t *testing.T) {
	rcloneStub(t)
	notice := "NOTICE: Config file \"/home/u/.config/rclone/rclone.conf\" not found - using defaults\nCouldn't find section in config file: no such remote"
	cases := []struct {
		name, exit string
		want       []string
		text       string
	}{
		{"directory not found (3) is a missing object", "3", []string{"E217", "E218"}, "no backup heartbeat found"},
		{"file not found (4) is a missing object", "4", []string{"E217", "E218"}, "no backup heartbeat found"},
		{"exit 1 with not-found words is an unreadable remote", "1", []string{"E219"}, "rclone exited with status 1"},
		{"temporary error (5) is an unreadable remote", "5", []string{"E219"}, "status 5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("RCLONE_EXIT", c.exit)
			t.Setenv("RCLONE_OUT", notice)
			st, err := ReadOffbox(context.Background(), "s3:nonexistent-bucket/hb", "proj", OffboxOptions{MaxAge: time.Hour, MaxDrillAge: time.Hour}, time.Now())
			codes := codeOf(t, err)
			if c.want[0] == "E219" {
				// Both heartbeats unreadable: two E219 problems.
				codes = codes[:1]
			}
			if strings.Join(codes, ",") != strings.Join(c.want, ",") || !strings.Contains(err.Error(), c.text) {
				t.Fatalf("codes %v, err %v", codes, err)
			}
			if len(st.Problems) == 0 {
				t.Error("problems is empty")
			}
		})
	}
}

// A remote that never answers is cut off by the bounded context.
func TestRemoteFetchIsBounded(t *testing.T) {
	rcloneStub(t)
	t.Setenv("RCLONE_SLEEP", "1")
	old := remoteFetchTimeout
	remoteFetchTimeout = 300 * time.Millisecond
	defer func() { remoteFetchTimeout = old }()
	start := time.Now()
	_, err := ReadOffbox(context.Background(), "s3:bucket/hb", "proj", OffboxOptions{MaxAge: time.Hour}, time.Now())
	if time.Since(start) > 10*time.Second {
		t.Fatalf("status took %s", time.Since(start))
	}
	if !strings.Contains(codesOf(t, err), "E219") || !strings.Contains(err.Error(), "did not answer within the time limit") {
		t.Fatalf("err = %v", err)
	}
	// The drill's List is bounded the same way.
	e := newDrillEnv(t)
	t.Setenv("PATH", filepath.Dir(mustLookPath(t, "rclone"))+string(os.PathListSeparator)+os.Getenv("PATH"))
	start = time.Now()
	_, err = DrillRemote(context.Background(), DrillRemoteOptions{Project: "proj", From: "s3:bucket/src", Identity: e.identity, HeartbeatTo: "path://" + e.hb})
	if time.Since(start) > 10*time.Second || !errors.Is(err, errs.ErrBackupRemoteFailed) || !strings.Contains(err.Error(), "time limit") {
		t.Fatalf("drill list: %v after %s", err, time.Since(start))
	}
}

// --format json carries a reason for every missing or unreadable heartbeat.
func TestOffboxProblemsArray(t *testing.T) {
	dir := t.TempDir()
	writeObject(t, dir, "backup", hbJSON(t, "backup", time.Now().Add(-48*time.Hour), nil))
	writeObject(t, dir, "drill", "{not json")
	st, err := ReadOffbox(context.Background(), "path://"+dir, "proj", OffboxOptions{MaxAge: 26 * time.Hour}, time.Now())
	if err == nil || len(st.Problems) != 2 {
		t.Fatalf("problems = %v, err %v", st.Problems, err)
	}
	if !strings.HasPrefix(st.Problems[0], "[E219] drill heartbeat is unreadable") || !strings.HasPrefix(st.Problems[1], "[E217]") {
		t.Errorf("problems = %q", st.Problems)
	}
	out, _ := FormatStatusOffbox(nil, st, "json")
	var v struct {
		Offbox struct {
			Problems []string `json:"problems"`
		} `json:"offbox"`
	}
	if json.Unmarshal([]byte(out), &v) != nil || len(v.Offbox.Problems) != 2 {
		t.Errorf("json: %s", out)
	}
	// A clean status has an empty array, never null.
	st, _ = ReadOffbox(context.Background(), "", "proj", OffboxOptions{}, time.Now())
	if out, _ = FormatStatusOffbox(nil, st, "json"); !strings.Contains(out, `"problems": []`) {
		t.Errorf("no-remote status: %s", out)
	}
}
