package backup

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// Purpose: tests for D-0467 and the schedule half of D-0257: backup schedule
// refuses inline credentials and hostile remotes before it renders or writes
// anything, and writes the service unit 0600. Values are obviously fake.

const (
	inlineS3Conn   = ":s3,access_key_id=" + fakeKeyID + ",secret_access_key=" + fakeSecret + ":bucket"
	inlineS3Scheme = "s3://" + fakeKeyID + ":" + fakeSecret + "@b/p"
)

func scheduleOpts(t *testing.T, dry bool, out *bytes.Buffer) ScheduleOptions {
	t.Helper()
	setScheduleProcessEnv(t, "")
	return ScheduleOptions{
		Cron: "0 3 * * *", To: "r2:b/p", BinaryPath: "/bin/sh", ProjectDir: t.TempDir(),
		UnitDir: t.TempDir(), DryRun: dry, Out: out,
	}
}

func TestScheduleStreamRefusesInlineCredentials(t *testing.T) {
	skipUnitTestOnWindows(t)
	for _, v := range []string{inlineS3Conn, inlineS3Scheme} {
		for _, field := range []string{"to", "heartbeat-to"} {
			for _, dry := range []bool{true, false} {
				var out bytes.Buffer
				opts := scheduleOpts(t, dry, &out)
				if field == "to" {
					opts.To = v
				} else {
					opts.HeartbeatTo = v
				}
				err := ScheduleStream(minimalConfig(), opts)
				if err == nil {
					t.Errorf("%s=%q dry=%v: accepted", field, v, dry)
					continue
				}
				if !strings.Contains(err.Error(), "inline") || !strings.Contains(err.Error(), "--env-file") {
					t.Errorf("%s dry=%v: error lacks the advice: %v", field, dry, err)
				}
				for _, s := range []string{err.Error(), out.String()} {
					if strings.Contains(s, fakeSecret) || strings.Contains(s, fakeKeyID) {
						t.Errorf("%s dry=%v: the secret leaked: %q", field, dry, s)
					}
				}
				if out.Len() != 0 || dirEntries(t, opts.UnitDir) != 0 {
					t.Errorf("%s dry=%v: something was rendered or written", field, dry)
				}
			}
		}
	}
}

func TestScheduleStreamRefusesHostileRemote(t *testing.T) {
	skipUnitTestOnWindows(t)
	for _, v := range []string{"-x", "file:///etc/x", "host://nosuch/srv/b"} {
		for _, dry := range []bool{true, false} {
			var out bytes.Buffer
			opts := scheduleOpts(t, dry, &out)
			opts.To = v
			if err := ScheduleStream(minimalConfig(), opts); err == nil {
				t.Errorf("--to %q dry=%v: accepted", v, dry)
			}
			opts.To, opts.HeartbeatTo = "r2:b/p", v
			if err := ScheduleStream(minimalConfig(), opts); err == nil {
				t.Errorf("--heartbeat-to %q dry=%v: accepted", v, dry)
			}
			if out.Len() != 0 || dirEntries(t, opts.UnitDir) != 0 {
				t.Errorf("%q dry=%v: something was rendered or written", v, dry)
			}
		}
	}
}

// TestScheduleStreamRefusesHostileRemotesQuietly: every hostile --to and
// --heartbeat-to is refused in both modes, with no rendered unit, no written
// file, and no credential in the error, the output or the stderr notice (D-1157).
func TestScheduleStreamRefusesHostileRemotesQuietly(t *testing.T) {
	skipUnitTestOnWindows(t)
	t.Chdir(t.TempDir())
	compattest.Both(t, func(t *testing.T) {
		notice := captureBareNotice(t)
		for _, v := range hostileRemotes() {
			for _, field := range []string{"to", "heartbeat-to"} {
				for _, dry := range []bool{true, false} {
					var out bytes.Buffer
					opts := scheduleOpts(t, dry, &out)
					if field == "to" {
						opts.To = v
					} else {
						opts.HeartbeatTo = v
					}
					err := ScheduleStream(minimalConfig(), opts)
					if err == nil {
						t.Errorf("%s=%q dry=%v: accepted", field, v, dry)
						continue
					}
					assertNoLeak(t, field+" error", err.Error())
					assertNoLeak(t, field+" output", out.String())
					if out.Len() != 0 || dirEntries(t, opts.UnitDir) != 0 {
						t.Errorf("%s=%q dry=%v: something was rendered or written", field, v, dry)
					}
				}
			}
		}
		assertNoLeak(t, "stderr notice", notice.String())
	})
}

// TestScheduleStreamAcceptsNativeDestinations: path:// and name:path pass.
func TestScheduleStreamAcceptsNativeDestinations(t *testing.T) {
	skipUnitTestOnWindows(t)
	for _, v := range []string{"path:///mnt/backups", "s3://bucket/p", "r2:bkt/x"} {
		var out bytes.Buffer
		opts := scheduleOpts(t, true, &out)
		opts.To = v
		if err := ScheduleStream(minimalConfig(), opts); err != nil {
			t.Errorf("--to %q: %v", v, err)
		}
	}
}

// TestScheduleStreamServiceUnitMode: the service unit (it holds --to) is 0600
// even over a pre-existing 0644 file; the timer is 0644.
func TestScheduleStreamServiceUnitMode(t *testing.T) {
	skipUnitTestOnWindows(t)
	dir := filepath.Join(t.TempDir(), "units")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	svc := filepath.Join(dir, "nself-backup-stream.service")
	if err := os.WriteFile(svc, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(svc, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeUnitFiles(dir, "[Service]\n", "[Timer]\n"); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]os.FileMode{
		"nself-backup-stream.service": 0o600,
		"nself-backup-stream.timer":   0o644,
	} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil || fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v (want %v) err %v", name, fi.Mode().Perm(), want, err)
		}
	}
	if b, _ := os.ReadFile(svc); string(b) != "[Service]\n" {
		t.Errorf("service content %q", b)
	}
}
