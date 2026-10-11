package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Purpose: tests for D-0257. backup stream, the heartbeat and (in
// stream_schedule_dest_test.go) backup schedule accept the destinations create
// accepts, and refuse the option-shaped and credential-carrying ones before any
// process starts. The PATH doubles come from fakeTools (heartbeat_test.go). All
// credential-looking values are obviously fake.

const (
	fakeKeyID  = "FAKEKEYID000000000000"
	fakeSecret = "fake-secret-not-real"
)

// hostileRemotes are values no destination may accept for stream or heartbeat.
func hostileRemotes() []string {
	return []string{
		"-x",
		"--config=/tmp/x",
		":s3:b",
		":s3,access_key_id=" + fakeKeyID + ",secret_access_key=" + fakeSecret + ":bucket",
		"myr,pass=" + fakeSecret + ":p",
		"s3://" + fakeKeyID + ":" + fakeSecret + "@bucket/p",
		"file:///etc/x",
		"http://h/x",
		"path://relative/dir",
	}
}

func dirEntries(t *testing.T, dir string) int {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	return len(entries)
}

// TestStreamToPathDestination: path:// works for stream; rclone is never run.
func TestStreamToPathDestination(t *testing.T) {
	store, log := fakeTools(t)
	dir := filepath.Join(t.TempDir(), "dest")
	res, err := Stream(context.Background(), minimalConfig(), StreamOptions{To: "path://" + dir, AllowUnencrypted: true})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	obj := filepath.Join(dir, res.BackupID)
	b, err := os.ReadFile(obj)
	if err != nil || string(b) != fakeDumpBytes {
		t.Fatalf("object %q: %q %v", obj, b, err)
	}
	if fi, _ := os.Stat(obj); fi.Mode().Perm() != 0o600 {
		t.Errorf("object mode %v, want 0600", fi.Mode().Perm())
	}
	if res.Bytes != int64(len(fakeDumpBytes)) {
		t.Errorf("Bytes = %d", res.Bytes)
	}
	for _, c := range readLog(t, log) {
		if strings.HasPrefix(c, "upload:") {
			t.Errorf("rclone was called: %q", c)
		}
	}
	if dirEntries(t, store) != 0 {
		t.Error("the rclone store should be empty")
	}
}

// TestStreamHeartbeatToPath: the heartbeat lands in a path:// remote.
func TestStreamHeartbeatToPath(t *testing.T) {
	fakeTools(t)
	hbDir := filepath.Join(t.TempDir(), "hb")
	opts := streamOpts()
	opts.HeartbeatTo = "path://" + hbDir
	if _, err := Stream(context.Background(), minimalConfig(), opts); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(hbDir, "testproject", "backup.json"))
	if err != nil || !strings.Contains(string(b), `"project": "testproject"`) {
		t.Fatalf("heartbeat: %q %v", b, err)
	}
}

// TestStreamRefusesHostileRemotes: a hostile --to or --heartbeat-to fails the
// run up front, dry-run included: no pg_dump, no rclone, nothing stored, and
// the error never echoes the value.
func TestStreamRefusesHostileRemotes(t *testing.T) {
	store, log := fakeTools(t)
	for _, v := range hostileRemotes() {
		for _, dry := range []bool{false, true} {
			for _, field := range []string{"to", "heartbeat-to"} {
				opts := streamOpts()
				opts.DryRun = dry
				if field == "to" {
					opts.To = v
				} else {
					opts.HeartbeatTo = v
				}
				res, err := Stream(context.Background(), minimalConfig(), opts)
				if err == nil || res != nil {
					t.Errorf("%s=%q dry=%v: Stream = %v, %v; want an error", field, v, dry, res, err)
					continue
				}
				if strings.Contains(err.Error(), fakeSecret) || strings.Contains(err.Error(), fakeKeyID) {
					t.Errorf("%s=%q: the error echoes the value: %v", field, v, err)
				}
			}
		}
	}
	if calls := readLog(t, log); len(calls) != 0 {
		t.Errorf("a process ran: %v", calls)
	}
	if dirEntries(t, store) != 0 {
		t.Error("something was stored")
	}
}

// TestWriteHeartbeatRefusesHostileRemotes: WriteHeartbeat parses its remote
// with the same grammar.
func TestWriteHeartbeatRefusesHostileRemotes(t *testing.T) {
	store, log := fakeTools(t)
	for _, v := range hostileRemotes() {
		err := WriteHeartbeat(context.Background(), v, Heartbeat{Project: "p"})
		if err == nil {
			t.Errorf("%q: accepted", v)
			continue
		}
		if strings.Contains(err.Error(), fakeSecret) || strings.Contains(err.Error(), fakeKeyID) {
			t.Errorf("%q: the error echoes the value: %v", v, err)
		}
	}
	if calls := readLog(t, log); len(calls) != 0 {
		t.Errorf("a process ran: %v", calls)
	}
	if dirEntries(t, store) != 0 {
		t.Error("something was stored")
	}
}
