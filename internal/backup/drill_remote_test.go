package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: tests for `backup drill --from` (P7-PROD-07). The flow tests use the
// docker and age doubles of restore_container_test.go over real path://
// destinations. TestDrillRemotePostgres runs a real encrypted stream backup
// through a real postgres container; it needs docker and age and skips
// without them (pack verification line 5 runs it in a Linux container).

const newestKey = "proj_stream_20261005_023000.sql.age"
const olderKey = "proj_stream_20261001_020000.sql.age"

type drillEnv struct {
	dir, src, hb, identity, logs, tmp string
}

func newDrillEnv(t *testing.T) *drillEnv {
	t.Helper()
	e := &drillEnv{logs: fakeDocker(t)}
	e.dir = t.TempDir()
	e.src, e.hb = filepath.Join(e.dir, "src"), filepath.Join(e.dir, "hb")
	e.tmp = filepath.Join(e.dir, "tmp")
	e.identity = filepath.Join(e.dir, "identity.key")
	for _, d := range []string{e.src, e.hb, e.tmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(e.identity, []byte("AGE-SECRET-KEY-FAKE-IDENTITY-DO-NOT-PRINT\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", e.tmp)
	t.Setenv("HOME", e.dir)
	for _, k := range []string{olderKey, newestKey} {
		if err := os.WriteFile(filepath.Join(e.src, k), []byte("PGDMP-fake-encrypted-stream"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A backup heartbeat for the newest object: users and orders estimated non-empty.
	hb := newBackupHeartbeat("proj", newestKey, 27, true, map[string]int64{"public.users": 1000, "public.orders": 25, "public.empty": 0}, time.Now().Add(-time.Hour), "1.4.12")
	data, _ := hb.Marshal()
	writeObject(t, e.hb, "backup", string(data))
	return e
}

// writeBackupHB replaces the backup heartbeat with one describing key.
func (e *drillEnv) writeBackupHB(t *testing.T, key string, approx map[string]int64) {
	t.Helper()
	hb := newBackupHeartbeat("proj", key, 27, strings.HasSuffix(key, ".age"), approx, time.Now().Add(-time.Hour), "1.4.12")
	data, _ := hb.Marshal()
	writeObject(t, e.hb, "backup", string(data))
}

func (e *drillEnv) opts() DrillRemoteOptions {
	return DrillRemoteOptions{Project: "proj", From: "path://" + e.src, Identity: e.identity, HeartbeatTo: "path://" + e.hb}
}

func (e *drillEnv) drillHeartbeat(t *testing.T) *Heartbeat {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(e.hb, "proj", "drill.json"))
	if err != nil {
		t.Fatalf("drill.json not written: %v", err)
	}
	hb, err := ParseHeartbeat(data, "proj", "drill", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return hb
}

// assertClean checks the isolation guarantees of every drill, pass or fail.
func (e *drillEnv) assertClean(t *testing.T, started bool) {
	t.Helper()
	calls := readFake(t, e.logs, "calls.log")
	started2, removed := false, false
	for _, l := range strings.Split(calls, "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		var name string
		switch {
		case f[0] == "exec" && len(f) > 2:
			name = f[2]
		case f[0] == "rm" && len(f) > 3:
			name, removed = f[3], true
		case f[0] == "inspect":
			name = f[len(f)-1]
		case f[0] == "run":
			started2 = true
		}
		if name != "" && !throwawayNameRe.MatchString(name) {
			t.Errorf("drill touched container %q, which is not a throwaway: %s", name, l)
		}
	}
	if started2 != started {
		t.Errorf("container started = %v, want %v", started2, started)
	}
	if started2 && !removed {
		t.Error("the throwaway container was not removed")
	}
	if left, _ := os.ReadDir(e.tmp); len(left) != 0 {
		t.Errorf("temp files left behind (plaintext dump?): %v", left)
	}
	if out := readFake(t, e.logs, "age.log") + calls; strings.Contains(out, "DO-NOT-PRINT") {
		t.Error("the identity content reached a process argument")
	}
}

func codesOf(t *testing.T, err error) string { return strings.Join(codeOf(t, err), ",") }

func TestDrillRemoteOK(t *testing.T) {
	e := newDrillEnv(t)
	res, err := DrillRemote(context.Background(), e.opts())
	if err != nil {
		t.Fatal(err)
	}
	hb := e.drillHeartbeat(t)
	if hb.Result != "ok" || len(hb.Mismatches) != 0 || hb.BackupKey != newestKey || !hb.Encrypted || hb.ApproxRows != nil ||
		hb.RestoredRows["public.users"] != 1000 || hb.RestoredRows["public.orders"] != 25 || hb.Bytes != 27 {
		t.Fatalf("%+v", hb)
	}
	if !res.HeartbeatWritten || !res.Estimated || len(res.Tables) != 2 {
		t.Errorf("%+v", res)
	}
	if got := readFake(t, e.logs, "age.log"); !strings.Contains(got, "--decrypt -i "+e.identity) {
		t.Errorf("age args: %q", got)
	}
	e.assertClean(t, true)
}

func TestDrillRemoteMismatchFails(t *testing.T) {
	e := newDrillEnv(t)
	t.Setenv("FAKE_ORDERS", "0") // estimated 25 rows, restored empty
	res, err := DrillRemote(context.Background(), e.opts())
	if codesOf(t, err) != "E218" || !strings.Contains(err.Error(), "public.orders") {
		t.Fatalf("err = %v", err)
	}
	hb := e.drillHeartbeat(t)
	if hb.Result != "failed" || len(hb.Mismatches) != 1 || !strings.HasPrefix(hb.Mismatches[0], "public.orders:") || res.Heartbeat.Result != "failed" {
		t.Fatalf("%+v", hb)
	}
	e.assertClean(t, true)
}

func TestDrillRemoteRestoreFailureRecorded(t *testing.T) {
	e := newDrillEnv(t)
	t.Setenv("FAKE_RESTORE_FAIL", "1")
	_, err := DrillRemote(context.Background(), e.opts())
	if !errors.Is(err, errs.ErrBackupRestoreFailed) {
		t.Fatalf("err = %v", err)
	}
	if hb := e.drillHeartbeat(t); hb.Result != "failed" || hb.RestoredRows != nil {
		t.Fatalf("%+v", hb)
	}
	e.assertClean(t, true)
}

func TestDrillRemoteDecryptFailureRecorded(t *testing.T) {
	e := newDrillEnv(t)
	t.Setenv("FAKE_AGE_FAIL", "1")
	_, err := DrillRemote(context.Background(), e.opts())
	if !errors.Is(err, errs.ErrBackupDecryptFailed) {
		t.Fatalf("err = %v", err)
	}
	if hb := e.drillHeartbeat(t); hb.Result != "failed" {
		t.Fatalf("%+v", hb)
	}
	e.assertClean(t, false)
}

func TestDrillRemotePreflightErrors(t *testing.T) {
	t.Run("no docker", func(t *testing.T) {
		e := newDrillEnv(t)
		t.Setenv("FAKE_NO_DOCKER", "1")
		if _, err := DrillRemote(context.Background(), e.opts()); codesOf(t, err) != "E220" {
			t.Fatalf("err = %v", err)
		}
		e.assertClean(t, false)
	})
	t.Run("not enough disk", func(t *testing.T) {
		e := newDrillEnv(t)
		old := freeDiskBytes
		freeDiskBytes = func(string) (uint64, error) { return 53, nil } // needs 2 x 27
		defer func() { freeDiskBytes = old }()
		if _, err := DrillRemote(context.Background(), e.opts()); codesOf(t, err) != "E220" {
			t.Fatalf("err = %v", err)
		}
		if _, statErr := os.Stat(filepath.Join(e.hb, "proj", "drill.json")); statErr == nil {
			t.Error("a preflight failure must not write a drill heartbeat")
		}
		e.assertClean(t, false)
	})
	t.Run("exactly twice the size is enough", func(t *testing.T) {
		e := newDrillEnv(t)
		old := freeDiskBytes
		freeDiskBytes = func(string) (uint64, error) { return 54, nil }
		defer func() { freeDiskBytes = old }()
		if _, err := DrillRemote(context.Background(), e.opts()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("no identity", func(t *testing.T) {
		e := newDrillEnv(t)
		o := e.opts()
		o.Identity = ""
		if _, err := DrillRemote(context.Background(), o); codesOf(t, err) != "E223" {
			t.Fatalf("err = %v", err)
		}
		e.assertClean(t, false)
	})
	t.Run("no backup", func(t *testing.T) {
		e := newDrillEnv(t)
		o := e.opts()
		o.Project = "other"
		if _, err := DrillRemote(context.Background(), o); !errors.Is(err, errs.ErrBackupNotFound) {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("bad project", func(t *testing.T) {
		e := newDrillEnv(t)
		o := e.opts()
		o.Project = "../x"
		if _, err := DrillRemote(context.Background(), o); err == nil {
			t.Fatal("accepted")
		}
	})
}

func TestDrillRemoteNamedKeyAndPlain(t *testing.T) {
	e := newDrillEnv(t)
	o := e.opts()
	o.Key = olderKey
	e.writeBackupHB(t, olderKey, map[string]int64{"public.users": 1000, "public.orders": 25})
	if _, err := DrillRemote(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	hb := e.drillHeartbeat(t)
	if hb.BackupKey != olderKey || hb.Result != "ok" {
		t.Fatalf("%+v", hb)
	}
	// An unencrypted stream object needs neither age nor an identity.
	if err := os.WriteFile(filepath.Join(e.src, "proj_stream_20261006_020000.sql"), []byte("PGDMP-plain-stream"), 0o600); err != nil {
		t.Fatal(err)
	}
	o = e.opts()
	o.Identity = ""
	e.writeBackupHB(t, "proj_stream_20261006_020000.sql", map[string]int64{"public.users": 1000, "public.orders": 25})
	if _, err := DrillRemote(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if hb := e.drillHeartbeat(t); hb.Encrypted || hb.BackupKey != "proj_stream_20261006_020000.sql" {
		t.Fatalf("%+v", hb)
	}
}

// Without source counts the drill cannot verify: it fails with E218 and says so,
// even though the restore itself produced rows. It never passes weakly.
func TestDrillRemoteWithoutSourceCountsFails(t *testing.T) {
	cases := map[string]func(e *drillEnv, o *DrillRemoteOptions, t *testing.T){
		"no heartbeat remote": func(_ *drillEnv, o *DrillRemoteOptions, _ *testing.T) { o.HeartbeatTo = "" },
		"heartbeat of another object": func(e *drillEnv, _ *DrillRemoteOptions, t *testing.T) {
			e.writeBackupHB(t, olderKey, map[string]int64{"public.users": 1000})
		},
		"null approx_rows": func(e *drillEnv, _ *DrillRemoteOptions, t *testing.T) { e.writeBackupHB(t, newestKey, nil) },
		"no heartbeat object": func(e *drillEnv, _ *DrillRemoteOptions, t *testing.T) {
			_ = os.Remove(filepath.Join(e.hb, "proj", "backup.json"))
		},
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			e := newDrillEnv(t)
			o := e.opts()
			mut(e, &o, t)
			res, err := DrillRemote(context.Background(), o)
			if codesOf(t, err) != "E218" || !strings.Contains(err.Error(), "cannot verify") {
				t.Fatalf("err = %v", err)
			}
			if res.Heartbeat.Result != "failed" || len(res.Heartbeat.Mismatches) != 1 || res.Estimated || len(res.Warnings) == 0 {
				t.Fatalf("%+v", res)
			}
			if o.HeartbeatTo != "" {
				if hb := e.drillHeartbeat(t); hb.Result != "failed" {
					t.Fatalf("%+v", hb)
				}
			}
			e.assertClean(t, true)
		})
	}
}

func TestSelectBackupAndCompare(t *testing.T) {
	objs := []destinations.Object{
		{Key: "other_stream_20270101_000000.sql.age"}, {Key: "proj_stream_20261001_020000.sql.age"},
		{Key: "proj_stream_20261005_023000.sql"}, {Key: "proj/backup.json"}, {Key: "proj_stream_20260101_000000.txt"},
	}
	got, err := selectBackup(objs, "proj", "")
	if err != nil || got.Key != "proj_stream_20261005_023000.sql" {
		t.Fatalf("%v %v", got, err)
	}
	if got, err = selectBackup(objs, "proj", "proj_stream_20261001_020000.sql.age"); err != nil || got.Key != objs[1].Key {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = selectBackup(objs, "proj", "nope"); !errors.Is(err, errs.ErrBackupNotFound) {
		t.Fatal(err)
	}
	if _, err = selectBackup(nil, "proj", ""); !errors.Is(err, errs.ErrBackupNotFound) {
		t.Fatal(err)
	}
	est := map[string]int64{"a.x": 50, "a.y": 0, "a.z": 9}
	for name, c := range map[string]struct {
		restored map[string]int64
		want     int
	}{
		"clean":                {map[string]int64{"a.x": 50, "a.z": 9}, 0},
		"within 10% shortfall": {map[string]int64{"a.x": 45, "a.z": 9}, 0},
		"more than estimated":  {map[string]int64{"a.x": 60, "a.z": 9}, 0},
		"partial table":        {map[string]int64{"a.x": 10, "a.z": 9}, 1},
		"empty table":          {map[string]int64{"a.x": 50, "a.y": 0, "a.z": 0}, 1},
		"missing table":        {map[string]int64{"a.x": 50}, 1},
		"nothing restored":     {map[string]int64{}, 2},
	} {
		m := compareRows(est, c.restored)
		if m == nil || len(m) != c.want {
			t.Errorf("%s: %#v, want %d mismatches", name, m, c.want)
		}
	}
	for _, none := range []map[string]int64{nil, {}} {
		if m := compareRows(none, map[string]int64{"a.x": 5}); len(m) != 1 || m[0] != cannotVerify {
			t.Errorf("no source counts must not pass: %v", m)
		}
	}
}

// TestDrillRemotePostgres drills a real encrypted stream backup into a real
// throwaway postgres container. It needs docker (with a reachable daemon),
// age and age-keygen, and skips otherwise.
func TestDrillRemotePostgres(t *testing.T) {
	for _, bin := range []string{"docker", "age", "age-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	ctx := context.Background()
	if err := exec.CommandContext(ctx, "docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		t.Skip("docker daemon not reachable")
	}
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	// Source database: a sibling postgres container seeded with two tables.
	src := "nself-drilltest-src-" + strings.ReplaceAll(t.Name(), "/", "-")
	run := func(args ...string) string {
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	_ = exec.Command("docker", "rm", "-f", "-v", src).Run()
	run("run", "-d", "--name", src, "--network", "none", "-e", "POSTGRES_PASSWORD=pw-for-test-only", "postgres:16-alpine")
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", src).Run() })
	for i := 0; ; i++ {
		if exec.CommandContext(ctx, "docker", "exec", src, "pg_isready", "-h", "127.0.0.1").Run() == nil {
			break
		}
		if i > 60 {
			t.Fatal("source postgres not ready")
		}
		time.Sleep(time.Second)
	}
	seed := "CREATE TABLE users(id int, name text); INSERT INTO users SELECT g, 'u'||g FROM generate_series(1,1000) g;" +
		"CREATE SCHEMA app; CREATE TABLE app.orders(id int); INSERT INTO app.orders SELECT generate_series(1,25);" +
		"CREATE TABLE empty_t(id int); ANALYZE;"
	run("exec", src, "psql", "-U", "postgres", "-c", seed)
	dump := filepath.Join(dir, "plain.dump")
	dumpOut, err := exec.CommandContext(ctx, "docker", "exec", src, "pg_dump", "-U", "postgres", "-Fc").Output()
	if err != nil || len(dumpOut) == 0 {
		t.Fatalf("pg_dump: %v", err)
	}
	if err := os.WriteFile(dump, dumpOut, 0o600); err != nil {
		t.Fatal(err)
	}
	// Encrypt with a fresh age key; the object is a custom-format archive named .sql.age.
	identity := filepath.Join(dir, "id.key")
	keyOut, err := exec.CommandContext(ctx, "age-keygen", "-o", identity).CombinedOutput()
	if err != nil {
		t.Fatalf("age-keygen: %v %s", err, keyOut)
	}
	rcpt := strings.TrimSpace(strings.TrimPrefix(string(keyOut), "Public key:"))
	srcDir, hbDir := filepath.Join(dir, "src"), filepath.Join(dir, "hb")
	_ = os.MkdirAll(srcDir, 0o700)
	_ = os.MkdirAll(hbDir, 0o700)
	key := "proj_stream_20261005_023000.sql.age"
	if out, err := exec.CommandContext(ctx, "age", "-r", rcpt, "-o", filepath.Join(srcDir, key), dump).CombinedOutput(); err != nil {
		t.Fatalf("age: %v %s", err, out)
	}
	approx := map[string]int64{"public.users": 1000, "app.orders": 25, "public.empty_t": 0}
	hb := newBackupHeartbeat("proj", key, 1, true, approx, time.Now(), "test")
	data, _ := hb.Marshal()
	writeObject(t, hbDir, "backup", string(data))

	// Other tests may start containers meanwhile: only drill containers are compared.
	drillPS := func() string { return run("ps", "-a", "--filter", "name=nself-drill-", "--format", "{{.Names}}") }
	before := drillPS()
	res, err := DrillRemote(ctx, DrillRemoteOptions{Project: "proj", From: "path://" + srcDir, Identity: identity, HeartbeatTo: "path://" + hbDir})
	if err != nil {
		t.Fatalf("drill: %v", err)
	}
	if res.Heartbeat.Result != "ok" || res.Heartbeat.RestoredRows["public.users"] != 1000 || res.Heartbeat.RestoredRows["app.orders"] != 25 {
		t.Fatalf("%+v", res.Heartbeat)
	}
	if after := drillPS(); after != before {
		t.Errorf("containers left behind:\nbefore %q\nafter  %q", before, after)
	}
	// A backup missing a table the heartbeat estimated non-empty fails.
	hb.ApproxRows["public.gone"] = 5
	data, _ = hb.Marshal()
	writeObject(t, hbDir, "backup", string(data))
	if _, err := DrillRemote(ctx, DrillRemoteOptions{Project: "proj", From: "path://" + srcDir, Identity: identity, HeartbeatTo: "path://" + hbDir}); codesOf(t, err) != "E218" {
		t.Fatalf("a missing table did not fail the drill: %v", err)
	}
	dj, _ := os.ReadFile(filepath.Join(hbDir, "proj", "drill.json"))
	if !strings.Contains(string(dj), `"result": "failed"`) || !strings.Contains(string(dj), "public.gone") {
		t.Errorf("drill.json: %s", dj)
	}
}

// A cancelled context (SIGINT or SIGTERM through signal.NotifyContext) while
// pg_restore runs unwinds the drill: the container is removed, no decrypted
// file or temp directory is left, and no drill heartbeat claims a result.
func TestDrillRemoteCancelMidRestoreCleansUp(t *testing.T) {
	e := newDrillEnv(t)
	t.Setenv("FAKE_RESTORE_HANG", "1")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := DrillRemote(ctx, e.opts()); done <- err }()
	for i := 0; !strings.Contains(readFake(t, e.logs, "calls.log"), "pg_restore"); i++ {
		if i > 200 {
			t.Fatal("the restore never started")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled drill reported success")
		}
	case <-time.After(20 * time.Second):
		t.Fatal("the drill did not return after the cancel")
	}
	e.assertClean(t, true) // removed, nothing in TMPDIR, only throwaway names touched
	if _, err := os.Stat(filepath.Join(e.hb, "proj", "drill.json")); err == nil {
		t.Error("a cancelled drill wrote drill.json")
	}
}

// The docker stub lists three labelled containers: a stale throwaway, a young
// throwaway and a labelled container that is not a throwaway.
const sweepDockerScript = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_DIR/calls.log"
case "$1" in
  ps) printf 'nself-drill-aaaaaaaaaaaaaaaa\nnself-drill-bbbbbbbbbbbbbbbb\nmyproj_postgres\nmyproj_pg_restore_test\nnself-drill-notvalid\n';;
  inspect) case "$4" in
      nself-drill-aaaa*) echo 2020-01-01T00:00:00.123456789Z;;
      *) date -u +%Y-%m-%dT%H:%M:%S.000000000Z;;
    esac;;
esac
exit 0
`

func TestSweepRemovesOnlyStaleThrowawayContainers(t *testing.T) {
	logs := fakeDocker(t)
	bin := filepath.Dir(mustLookPath(t, "docker"))
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(sweepDockerScript), 0o755); err != nil {
		t.Fatal(err)
	}
	sweepDrillContainers(context.Background(), time.Hour)
	calls := readFake(t, logs, "calls.log")
	if !strings.Contains(calls, "ps -a --filter label=org.nself.drill") {
		t.Errorf("the sweep must filter by the drill label only:\n%s", calls)
	}
	var removed []string
	for _, l := range strings.Split(calls, "\n") {
		if strings.HasPrefix(l, "rm ") {
			removed = append(removed, l)
		}
	}
	if len(removed) != 1 || removed[0] != "rm -f -v nself-drill-aaaaaaaaaaaaaaaa" {
		t.Errorf("removed %v, want only the stale throwaway", removed)
	}
}

func mustLookPath(t *testing.T, name string) string {
	t.Helper()
	p, err := exec.LookPath(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSweepRemovesOnlyStaleOwnedDrillTempDirs(t *testing.T) {
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mode os.FileMode, age time.Time) string {
		p := filepath.Join(root, name)
		if err := os.Mkdir(p, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "backup.plain"), []byte("secret"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(p, age, age)
		return p
	}
	stale := mk("nself-drill-111", 0o700, old)
	young := mk("nself-drill-222", 0o700, time.Now())
	loose := mk("nself-drill-333", 0o755, old) // not the mode MkdirTemp gives
	other := mk("nself-other-444", 0o700, old) // not a drill directory
	target := mk("target-dir", 0o700, old)     // a symlink must never be followed
	link := filepath.Join(root, "nself-drill-555")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	sweepDrillTempDirs(root, time.Hour)
	if _, err := os.Stat(stale); err == nil {
		t.Error("the stale drill directory was not removed")
	}
	keep := []string{young, other, target, filepath.Join(target, "backup.plain")}
	if runtime.GOOS != "windows" { // Windows has no mode bits to tell a loose directory by
		keep = append(keep, loose)
	}
	for _, p := range keep {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", p, err)
		}
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("the symlink was removed: %v", err)
	}
}

// An E220 (Docker down) exits with its registered class 2: the exec.ExitError
// of `docker version` must not be wrapped as the cause.
func TestDrillRemoteDockerDownExitClass(t *testing.T) {
	e := newDrillEnv(t)
	t.Setenv("FAKE_NO_DOCKER", "1")
	_, err := DrillRemote(context.Background(), e.opts())
	if codesOf(t, err) != "E220" || errs.ExitCodeFor(err) != 2 {
		t.Fatalf("err = %v, exit class %d", err, errs.ExitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "Docker is not available for the drill container") {
		t.Errorf("message changed: %v", err)
	}
}

// TestDrillSignalCleanupAndSweepReal runs against a real Docker daemon: the
// sweep removes a stale throwaway but leaves a running unrelated container and
// a labelled container that is not a throwaway; a cancel in the middle of a
// restore then leaves no drill container and no temp directory behind.
func TestDrillSignalCleanupAndSweepReal(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not on PATH")
	}
	if err := exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run(); err != nil {
		t.Skip("docker daemon not reachable")
	}
	docker := func(args ...string) string {
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("docker %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	if runtime.GOOS == "windows" {
		t.Skip("needs Linux containers")
	}
	if exec.Command("docker", "image", "inspect", drillImage()).Run() != nil {
		if out, err := exec.Command("docker", "pull", drillImage()).CombinedOutput(); err != nil {
			t.Skipf("cannot pull %s: %v\n%s", drillImage(), err, out)
		}
	}
	id, _ := randHex(4)
	stale := throwawayPrefix + "0000" + id + "0000" // a throwaway name no live run uses
	fixtures := []string{"p07fix-plain-" + id, "p07fix-labelled-" + id}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", stale, fixtures[0], fixtures[1]).Run() })
	docker("run", "-d", "--name", stale, "--label", containerLabel+"=stale", "--entrypoint", "sleep", drillImage(), "300")
	docker("run", "-d", "--name", fixtures[0], "--entrypoint", "sleep", drillImage(), "300")
	docker("run", "-d", "--name", fixtures[1], "--label", containerLabel+"=fixture", "--entrypoint", "sleep", drillImage(), "300")

	old := drillStaleAfter
	drillStaleAfter = 0
	t.Cleanup(func() { drillStaleAfter = old })

	e := &drillEnv{}
	e.dir = t.TempDir()
	e.src, e.tmp = filepath.Join(e.dir, "src"), filepath.Join(e.dir, "tmp")
	for _, d := range []string{e.src, e.tmp} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("TMPDIR", e.tmp)
	if err := os.WriteFile(filepath.Join(e.src, "proj_stream_20261005_023000.sql"), []byte("SELECT pg_sleep(120);\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := DrillRemote(ctx, DrillRemoteOptions{Project: "proj", From: "path://" + e.src})
		done <- err
	}()
	var drill string
	for i := 0; drill == ""; i++ {
		if i > 300 {
			t.Fatal("no drill container appeared")
		}
		for _, n := range strings.Fields(docker("ps", "-a", "--filter", "label="+containerLabel, "--format", "{{.Names}}")) {
			if throwawayNameRe.MatchString(n) && n != stale {
				drill = n
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	for i := 0; exec.Command("docker", "exec", drill, "pg_isready", "-h", "127.0.0.1").Run() != nil && i < 120; i++ {
		time.Sleep(500 * time.Millisecond)
	}
	time.Sleep(2 * time.Second) // psql is inside pg_sleep
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("a cancelled drill reported success")
		}
	case <-time.After(60 * time.Second):
		t.Fatal("the drill did not return after the cancel")
	}
	if got := docker("ps", "-a", "--filter", "name="+drill, "--format", "{{.Names}}"); got != "" {
		t.Errorf("drill container %s left behind", drill)
	}
	if got := docker("ps", "-a", "--filter", "name="+stale, "--format", "{{.Names}}"); got != "" {
		t.Errorf("stale throwaway %s was not swept", stale)
	}
	for _, f := range fixtures {
		if got := docker("ps", "--filter", "name="+f, "--filter", "status=running", "--format", "{{.Names}}"); got != f {
			t.Errorf("unrelated container %s was touched (running list: %q)", f, got)
		}
	}
	if left, _ := os.ReadDir(e.tmp); len(left) != 0 {
		t.Errorf("temp files left behind: %v", left)
	}
}
