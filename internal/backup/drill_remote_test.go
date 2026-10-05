package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
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
	if hb.Result != "failed" || len(hb.Mismatches) != 1 || hb.Mismatches[0] != "public.orders" || res.Heartbeat.Result != "failed" {
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
		if _, err := DrillRemote(context.Background(), o); !errors.Is(err, errs.ErrBackupDecryptFailed) {
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
	if _, err := DrillRemote(context.Background(), o); err != nil {
		t.Fatal(err)
	}
	if hb := e.drillHeartbeat(t); hb.Encrypted || hb.BackupKey != "proj_stream_20261006_020000.sql" {
		t.Fatalf("%+v", hb)
	}
}

func TestDrillRemoteWithoutEstimatesNeedsRows(t *testing.T) {
	e := newDrillEnv(t)
	o := e.opts()
	o.HeartbeatTo = "" // no heartbeat remote: nothing to compare, nothing written
	res, err := DrillRemote(context.Background(), o)
	if err != nil || res.Estimated || res.HeartbeatWritten {
		t.Fatalf("%+v %v", res, err)
	}
	t.Setenv("FAKE_USERS", "0")
	t.Setenv("FAKE_ORDERS", "0")
	if _, err := DrillRemote(context.Background(), o); codesOf(t, err) != "E218" {
		t.Fatalf("an empty restore passed: %v", err)
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
	est := map[string]int64{"a.x": 5, "a.y": 0, "a.z": 9}
	if m := compareRows(est, map[string]int64{"a.x": 1, "a.y": 0, "a.z": 0}); len(m) != 1 || m[0] != "a.z" {
		t.Errorf("empty table not flagged: %v", m)
	}
	if m := compareRows(est, map[string]int64{"a.x": 1}); len(m) != 1 || m[0] != "a.z" {
		t.Errorf("missing table not flagged: %v", m)
	}
	if m := compareRows(est, map[string]int64{"a.x": 1, "a.z": 2}); len(m) != 0 || m == nil {
		t.Errorf("clean restore: %#v", m)
	}
	if m := compareRows(nil, map[string]int64{}); len(m) != 1 {
		t.Errorf("an empty restore without estimates passed: %v", m)
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
