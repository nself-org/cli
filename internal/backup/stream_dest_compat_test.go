package backup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// Purpose: tests for D-0676. A bare absolute path given to backup stream
// (--to, BACKUP_REMOTE, the heartbeat remote) or backup schedule streams as
// path://<dir> with one notice on v1.4, as it did through rclone's local
// backend on v1.4.13, and is refused under compat.V15 before any process
// runs. Every test selects its mode with compattest; none reads the ambient
// NSELF_V15.

// captureBareNotice sends the notice to a buffer and forgets earlier notices.
func captureBareNotice(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	oldOut := bareNoticeOut
	bareNoticeMu.Lock()
	oldSeen := bareNoticed
	bareNoticed = map[string]bool{}
	bareNoticeMu.Unlock()
	bareNoticeOut = &buf
	t.Cleanup(func() {
		bareNoticeOut = oldOut
		bareNoticeMu.Lock()
		bareNoticed = oldSeen
		bareNoticeMu.Unlock()
	})
	return &buf
}

func skipBarePathOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a bare path is one starting with /; Windows temp dirs start with a drive letter")
	}
}

// noticeLines counts the notice lines in buf.
func noticeLines(buf *bytes.Buffer) int {
	return strings.Count(buf.String(), "notice: backup destination")
}

func TestSettleBarePath(t *testing.T) {
	skipBarePathOnWindows(t)
	cases := []struct{ in, v14 string }{
		{"/mnt/backups", "path:///mnt/backups"},
		{"/mnt/backups/", "path:///mnt/backups"},
		{"/mnt/a/../backups", "path:///mnt/backups"},
		{"//mnt/backups", "path:///mnt/backups"},
		{"path:///mnt/backups", "path:///mnt/backups"},
		{"r2:bkt/x", "r2:bkt/x"},
		{"s3://bucket/p", "s3://bucket/p"},
		{"relative/dir", "relative/dir"},
		{"./dir", "./dir"},
		{"", ""},
	}
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		buf := captureBareNotice(t)
		for _, c := range cases {
			got, err := settleBarePath(c.in)
			bare := strings.HasPrefix(c.in, "/")
			switch {
			case v15 && bare:
				if err == nil || got != "" {
					t.Errorf("%q: got %q, %v; want a refusal", c.in, got, err)
					continue
				}
				if !strings.Contains(err.Error(), "path:///") || strings.Contains(err.Error(), c.in) {
					t.Errorf("%q: error %q must hint path:/// and not echo the value", c.in, err)
				}
			case err != nil || got != c.v14:
				t.Errorf("%q: got %q, %v; want %q", c.in, got, err, c.v14)
			}
		}
		if v15 && buf.Len() != 0 {
			t.Errorf("v1.5 printed a notice: %q", buf)
		}
	})
}

// TestSettleBarePath_NoticeOncePerValue: one line per value per process.
func TestSettleBarePath_NoticeOncePerValue(t *testing.T) {
	skipBarePathOnWindows(t)
	compattest.Set(t, false)
	buf := captureBareNotice(t)
	for i := 0; i < 3; i++ {
		if _, err := settleBarePath("/mnt/backups"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := settleBarePath("/mnt/other"); err != nil {
		t.Fatal(err)
	}
	if n := noticeLines(buf); n != 2 {
		t.Errorf("notice lines = %d, want 2: %q", n, buf)
	}
	if !strings.Contains(buf.String(), "Write path:///mnt/backups instead") {
		t.Errorf("notice does not say what to write: %q", buf)
	}
}

// TestSettleBarePath_SymlinkRoot: on v1.4 a bare path whose directory is a
// symlink streams into the target (rclone followed it); path:// itself still
// refuses a symlinked root.
func TestSettleBarePath_SymlinkRoot(t *testing.T) {
	skipBarePathOnWindows(t)
	compattest.Set(t, false)
	captureBareNotice(t)
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(real)
	got, err := settleBarePath(link)
	if err != nil || got != "path://"+want {
		t.Errorf("got %q, %v; want path://%s", got, err, want)
	}
}

// TestStreamBarePath_Sources: --to, BACKUP_REMOTE (cfg.Backup.Remote) and the
// heartbeat remote (flag and NSELF_BACKUP_HEARTBEAT_REMOTE). v1.4: the object
// lands in the directory, rclone never runs, one notice per value. v1.5:
// refused up front, nothing runs, nothing is written.
func TestStreamBarePath_Sources(t *testing.T) {
	skipBarePathOnWindows(t)
	sources := []string{"to", "config", "heartbeat-to", "heartbeat-env"}
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		for _, src := range sources {
			t.Run(src, func(t *testing.T) {
				store, log := fakeTools(t)
				buf := captureBareNotice(t)
				dir := filepath.Join(t.TempDir(), "dest")
				cfg := minimalConfig()
				opts := StreamOptions{To: "r2:bkt/x", AllowUnencrypted: true}
				switch src {
				case "to":
					opts.To = dir
				case "config":
					opts.To = ""
					cfg.Backup.Remote = dir
				case "heartbeat-to":
					opts.HeartbeatTo = dir
				case "heartbeat-env":
					t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", dir)
				}
				res, err := Stream(context.Background(), cfg, opts)
				if v15 {
					if err == nil || res != nil {
						t.Fatalf("Stream = %v, %v; want a refusal", res, err)
					}
					if !strings.Contains(err.Error(), "path:///") || strings.Contains(err.Error(), dir) {
						t.Errorf("error %q must hint path:/// and not echo the value", err)
					}
					assertNoProcessRan(t, store, log)
					if _, serr := os.Stat(dir); serr == nil {
						t.Error("the bare directory was created")
					}
					return
				}
				if err != nil {
					t.Fatalf("Stream: %v", err)
				}
				obj := filepath.Join(dir, res.BackupID)
				if strings.HasPrefix(src, "heartbeat") {
					obj = filepath.Join(dir, "testproject", "backup.json")
				}
				if _, err := os.Stat(obj); err != nil {
					t.Errorf("object not in the bare directory: %v", err)
				}
				for _, c := range readLog(t, log) {
					if strings.HasPrefix(c, "upload:") && strings.Contains(c, dir) {
						t.Errorf("rclone was called for the bare path: %q", c)
					}
				}
				if n := noticeLines(buf); n != 1 {
					t.Errorf("notice lines = %d, want 1: %q", n, buf)
				}
			})
		}
	})
}

// TestStreamBarePath_DryRunOutput: the v1.4 dry run reports the destination as
// written, the line v1.4.13 printed, and creates nothing.
func TestStreamBarePath_DryRunOutput(t *testing.T) {
	skipBarePathOnWindows(t)
	compattest.Set(t, false)
	store, log := fakeTools(t)
	captureBareNotice(t)
	dir := filepath.Join(t.TempDir(), "dest")
	res, err := Stream(context.Background(), minimalConfig(), StreamOptions{To: dir, AllowUnencrypted: true, DryRun: true})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if want := dir + "/" + res.BackupID; res.Destination != want {
		t.Errorf("Destination = %q, want %q", res.Destination, want)
	}
	assertNoProcessRan(t, store, log)
	if _, serr := os.Stat(dir); serr == nil {
		t.Error("a dry run created the directory")
	}
}

// TestWriteHeartbeatBarePath: WriteHeartbeat settles its remote the same way.
func TestWriteHeartbeatBarePath(t *testing.T) {
	skipBarePathOnWindows(t)
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		captureBareNotice(t)
		dir := filepath.Join(t.TempDir(), "hb")
		err := WriteHeartbeat(context.Background(), dir, newBackupHeartbeat("p", "k", 1, false, nil, time.Date(2026, 10, 5, 2, 30, 41, 0, time.UTC), "1.4.13"))
		if v15 != (err != nil) {
			t.Fatalf("v15=%v: WriteHeartbeat err = %v", v15, err)
		}
		_, serr := os.Stat(filepath.Join(dir, "p", "backup.json"))
		if v15 == (serr == nil) {
			t.Errorf("v15=%v: heartbeat object stat = %v", v15, serr)
		}
	})
}

// TestScheduleStreamBarePath: backup schedule accepts a bare --to and
// --heartbeat-to on v1.4 (the unit keeps the value as written, as v1.4.13
// did) and refuses them under v1.5, naming the flag.
func TestScheduleStreamBarePath(t *testing.T) {
	skipUnitTestOnWindows(t)
	compattest.Both(t, func(t *testing.T) {
		v15 := os.Getenv("NSELF_V15") == "1"
		for _, flag := range []string{"--to", "--heartbeat-to"} {
			buf := captureBareNotice(t)
			var out bytes.Buffer
			opts := scheduleOpts(t, true, &out)
			if flag == "--to" {
				opts.To = "/mnt/backups"
			} else {
				opts.HeartbeatTo = "/mnt/backups"
			}
			err := ScheduleStream(minimalConfig(), opts)
			if v15 {
				if err == nil || !strings.Contains(err.Error(), flag+":") || !strings.Contains(err.Error(), "path:///") {
					t.Errorf("%s: err = %v; want a refusal naming the flag", flag, err)
				}
				continue
			}
			if err != nil {
				t.Fatalf("%s: %v", flag, err)
			}
			if !strings.Contains(out.String(), "/mnt/backups") {
				t.Errorf("%s: the unit does not carry the value as written: %q", flag, out.String())
			}
			if noticeLines(buf) != 1 {
				t.Errorf("%s: want one notice, got %q", flag, buf)
			}
		}
	})
}
