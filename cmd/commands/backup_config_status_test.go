package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/pflag"
)

func TestWithDestinationKindsText(t *testing.T) {
	out, err := withDestinationKinds("Backup Configuration:\n", "table", "path:///mnt/b")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"configured: path", "* path", "  rclone", "  host"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	out, _ = withDestinationKinds("x\n", "table", "")
	if !strings.Contains(out, "configured: none") {
		t.Errorf("unset remote should read none:\n%s", out)
	}
}

func TestWithDestinationKindsJSON(t *testing.T) {
	out, err := withDestinationKinds(`{"remote": "s3://b/p"}`, "json", "s3://b/p")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Remote      string `json:"remote"`
		Destination string `json:"destination"`
		Kinds       []struct {
			Kind string `json:"kind"`
		} `json:"destination_kinds"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if v.Remote != "s3://b/p" || v.Destination != "rclone" || len(v.Kinds) != 3 {
		t.Fatalf("%+v", v)
	}
	if _, err := withDestinationKinds("not json", "json", ""); err == nil {
		t.Fatal("bad json accepted")
	}
}

// Purpose: flag and run tests for `nself backup status --heartbeat-to` (P7-PROD-07).
// Inputs: the package-level backupStatusCmd with flags set programmatically,
// path:// heartbeat directories and a temp directory that is NOT a project.
// Outputs: none (t.Error on wrong JSON, exit class or code).

// writeStatusHB writes <dir>/proj/<kind>.json with the given age and result.
func writeStatusHB(t *testing.T, dir, kind string, age time.Duration, result string) {
	t.Helper()
	hb := backup.Heartbeat{
		At: time.Now().Add(-age).UTC().Format(time.RFC3339), BackupKey: "proj_stream_1.sql.age", Bytes: 10, CLIVersion: "1.4.12",
		Encrypted: true, Kind: kind, Mismatches: []string{}, Project: "proj", Result: result, SchemaVersion: "1",
	}
	if kind == "backup" {
		hb.ApproxRows = map[string]int64{"public.users": 5}
	} else {
		hb.RestoredRows = map[string]int64{"public.users": 5}
	}
	data, err := hb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "proj"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "proj", kind+".json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func runStatusFlags(t *testing.T, flags map[string]string) (string, error) {
	t.Helper()
	fs := backupStatusCmd.Flags()
	resetBackupFlagSet(t, fs)
	fs.VisitAll(func(f *pflag.Flag) { // each call starts from the defaults
		_ = f.Value.Set(f.DefValue)
		f.Changed = false
	})
	for k, v := range flags {
		if err := fs.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return captureStdout(t, func() error { return runBackupStatus(backupStatusCmd, nil) })
}

func TestBackupStatusOffboxFlags(t *testing.T) {
	fs := backupStatusCmd.Flags()
	for _, name := range []string{"heartbeat-to", "max-age", "max-drill-age", "project", "format"} {
		if f := fs.Lookup(name); f == nil || f.Value.Type() != "string" {
			t.Errorf("flag --%s is missing or not a string on `backup status`", name)
		}
	}
	hb := t.TempDir()
	t.Chdir(t.TempDir()) // not a project: --project with --heartbeat-to must not load one
	remote := "path://" + hb
	base := map[string]string{"project": "proj", "heartbeat-to": remote, "format": "json"}
	with := func(kv ...string) map[string]string {
		m := map[string]string{}
		for k, v := range base {
			m[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			m[kv[i]] = kv[i+1]
		}
		return m
	}
	codeIn := func(err error, code string) bool { return err != nil && strings.Contains(err.Error(), "["+code+"]") }

	writeStatusHB(t, hb, "backup", 2*time.Hour, "ok")
	writeStatusHB(t, hb, "drill", 3*24*time.Hour, "ok")
	out, err := runStatusFlags(t, with("max-age", "26h", "max-drill-age", "35d"))
	if err != nil {
		t.Fatalf("fresh: %v", err)
	}
	var v map[string]json.RawMessage
	if jerr := json.Unmarshal([]byte(out), &v); jerr != nil || len(v) != 1 || v["offbox"] == nil {
		t.Fatalf("outside a project only offbox is printed: %v\n%s", jerr, out)
	}
	var off struct {
		Source   string `json:"source"`
		Exceeded bool   `json:"max_age_exceeded"`
		Age      int64  `json:"backup_age_seconds"`
	}
	_ = json.Unmarshal(v["offbox"], &off)
	if off.Source != "heartbeat" || off.Exceeded || off.Age < 7190 || off.Age > 7300 {
		t.Errorf("%+v", off)
	}

	writeStatusHB(t, hb, "backup", 27*time.Hour, "ok")
	out, err = runStatusFlags(t, with("max-age", "26h"))
	if !codeIn(err, "E217") || errs.ExitCodeFor(err) == 0 || !strings.Contains(out, `"max_age_exceeded": true`) {
		t.Fatalf("27 h old backup: err=%v out=%s", err, out)
	}
	writeStatusHB(t, hb, "drill", 36*24*time.Hour, "ok")
	if _, err = runStatusFlags(t, with("max-drill-age", "35d")); !codeIn(err, "E218") {
		t.Fatalf("36 day old drill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(hb, "proj", "backup.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = runStatusFlags(t, with()); !codeIn(err, "E219") {
		t.Fatalf("unreadable heartbeat: %v", err)
	}
	if _, err = runStatusFlags(t, with("max-age", "soon")); err == nil || !strings.Contains(err.Error(), "--max-age") {
		t.Fatalf("bad --max-age: %v", err)
	}
	text, err := runStatusFlags(t, map[string]string{"project": "proj", "heartbeat-to": remote, "max-drill-age": "90d"})
	if !codeIn(err, "E219") || !strings.Contains(text, "Off-box drill:") {
		t.Errorf("text form: err=%v\n%s", err, text)
	}
}

// TestBackupStatusInProjectKeepsExistingFields: inside a project without a
// heartbeat remote the JSON gains only offbox.source = none.
func TestBackupStatusInProjectKeepsExistingFields(t *testing.T) {
	proj := t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=proj\nENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(proj)
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "")
	out, err := runStatusFlags(t, map[string]string{"format": "json"})
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]json.RawMessage
	if jerr := json.Unmarshal([]byte(out), &v); jerr != nil {
		t.Fatal(jerr, out)
	}
	for _, k := range []string{"last_run", "next_run", "health", "total_size", "backup_count", "retention_daily", "offbox"} {
		if v[k] == nil {
			t.Errorf("field %q missing:\n%s", k, out)
		}
	}
	if !strings.Contains(string(v["offbox"]), `"source": "none"`) {
		t.Errorf("offbox: %s", v["offbox"])
	}
	text, err := runStatusFlags(t, nil)
	if err != nil || strings.Contains(text, "Off-box") {
		t.Errorf("text output must not change without a heartbeat remote: %v\n%s", err, text)
	}
}
