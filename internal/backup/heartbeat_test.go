package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// Purpose: tests for the backup heartbeat (P7-PROD-71, contract:cli.backup-heartbeat).
// The stream tests run the real pipeline against test doubles of pg_dump, age,
// psql and rclone placed first on PATH; the doubles log every call so the
// tests can prove ordering and that a failed backup writes no heartbeat. No
// real storage is ever contacted.

const fakeDumpBytes = "PGDMP-fake-custom-dump" // 22 bytes; the age double is a pass-through

// fakeTools installs the doubles and returns the store dir (uploaded objects,
// one file per remote path) and the call log path.
func fakeTools(t *testing.T) (store, log string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("PATH doubles are POSIX shell scripts")
	}
	dir := t.TempDir()
	store = filepath.Join(dir, "store")
	log = filepath.Join(dir, "calls.log")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	scripts := map[string]string{
		"pg_dump": `echo pg_dump >> "$FAKE_LOG"
if [ -n "$FAKE_PGDUMP_FAIL" ]; then echo "pg_dump: connection refused" >&2; exit 1; fi
if [ -n "$FAKE_PGDUMP_EMPTY" ]; then exit 0; fi
printf '` + fakeDumpBytes + `'`,
		"age": `cat`,
		"psql": `echo psql >> "$FAKE_LOG"
echo "args: $*" > "$FAKE_STORE/../psql.args"
echo "pgpassword: $PGPASSWORD" >> "$FAKE_STORE/../psql.args"
if [ -n "$FAKE_PSQL_FAIL" ]; then echo "psql: failed for $*" >&2; exit 1; fi
printf 'public.users\t42\npublic.orders\t7\n'`,
		"rclone": `[ "$1" = rcat ] || exit 2
if [ -n "$FAKE_RCLONE_FAIL" ]; then
  case "$2" in *"$FAKE_RCLONE_FAIL"*) cat >/dev/null; echo "rclone: fake upload failure" >&2; exit 1;; esac
fi
name=$(printf %s "$2" | tr '/:' '__')
cat > "$FAKE_STORE/$name"
echo "upload $2" >> "$FAKE_LOG"`,
	}
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range scripts {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_STORE", store)
	t.Setenv("FAKE_LOG", log)
	for _, k := range []string{"FAKE_PGDUMP_FAIL", "FAKE_PGDUMP_EMPTY", "FAKE_PSQL_FAIL", "FAKE_RCLONE_FAIL", "NSELF_BACKUP_HEARTBEAT_REMOTE"} {
		t.Setenv(k, "")
	}
	return store, log
}

func readLog(t *testing.T, log string) []string {
	t.Helper()
	b, _ := os.ReadFile(log)
	return strings.Fields(strings.ReplaceAll(string(b), "upload ", "upload:"))
}

func streamOpts() StreamOptions {
	return StreamOptions{
		To:          "r2:bkt/nself-web",
		Recipients:  []string{"age1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqsxxxxxxxx"},
		HeartbeatTo: "hbremote:hb",
	}
}

// TestHeartbeatGoldenShape pins the object: field set, sorted keys, 2-space
// indent, trailing newline, null/[] rules.
func TestHeartbeatGoldenShape(t *testing.T) {
	hb := newBackupHeartbeat("nself-web", "nself-web_stream_20261005_023000.sql.age", 1048576, true,
		map[string]int64{"public.users": 42, "public.orders": 7},
		time.Date(2026, 10, 5, 2, 30, 41, 0, time.UTC), "1.4.12")
	got, err := hb.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "heartbeat", "backup.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("heartbeat drifted from the golden.\n--- got ---\n%s--- want ---\n%s", got, want)
	}
	assertSortedKeys(t, got)
}

// assertSortedKeys fails when the top-level JSON keys are not in sorted order.
func assertSortedKeys(t *testing.T, raw []byte) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	prev := ""
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		key := k.(string)
		if key <= prev {
			t.Errorf("key %q is not after %q: keys must be sorted", key, prev)
		}
		prev = key
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
}

// TestHeartbeatParseApproxRows covers psql output parsing.
func TestHeartbeatParseApproxRows(t *testing.T) {
	got, err := parseApproxRows("public.a\t10\n\npublic.b c\t0\r\n")
	if err != nil || got["public.a"] != 10 || got["public.b c"] != 0 || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
	if empty, err := parseApproxRows(""); err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("empty output must give an empty non-nil map, got %v, %v", empty, err)
	}
	for _, bad := range []string{"nocount", "public.a\tabc"} {
		if _, err := parseApproxRows(bad); err == nil {
			t.Errorf("parseApproxRows(%q) should fail", bad)
		}
	}
}

// TestStreamHeartbeat_WrittenAfterUpload runs the full pipeline against the
// doubles: estimates are read before the dump, the heartbeat is uploaded after
// the backup, and its content follows the contract with no secret or remote.
func TestStreamHeartbeat_WrittenAfterUpload(t *testing.T) {
	store, log := fakeTools(t)
	cfg := minimalConfig()
	cfg.Postgres.Password = "s3cretpw"
	res, err := Stream(context.Background(), cfg, streamOpts())
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	calls := readLog(t, log)
	want := []string{"psql", "pg_dump", "upload:r2:bkt/nself-web/" + res.BackupID, "upload:hbremote:hb/testproject/backup.json"}
	if strings.Join(calls, " ") != strings.Join(want, " ") {
		t.Fatalf("call order = %v, want %v (estimates before the dump, heartbeat after the upload)", calls, want)
	}
	raw, err := os.ReadFile(filepath.Join(store, "hbremote_hb_testproject_backup.json"))
	if err != nil {
		t.Fatalf("heartbeat object missing: %v", err)
	}
	assertSortedKeys(t, raw)
	if !bytes.HasSuffix(raw, []byte("}\n")) || !bytes.HasPrefix(raw, []byte("{\n  \"approx_rows\": {\n")) {
		t.Errorf("not 2-space indented with a trailing newline:\n%s", raw)
	}
	var hb map[string]any
	if err := json.Unmarshal(raw, &hb); err != nil {
		t.Fatal(err)
	}
	golden, _ := os.ReadFile(filepath.Join("testdata", "heartbeat", "backup.golden.json"))
	var shape map[string]any
	_ = json.Unmarshal(golden, &shape)
	for k := range shape {
		if _, ok := hb[k]; !ok {
			t.Errorf("heartbeat lacks field %q", k)
		}
	}
	if len(hb) != len(shape) {
		t.Errorf("heartbeat has %d fields, golden has %d", len(hb), len(shape))
	}
	rows, _ := hb["approx_rows"].(map[string]any)
	if rows["public.users"] != float64(42) || rows["public.orders"] != float64(7) {
		t.Errorf("approx_rows = %v", rows)
	}
	checks := map[string]any{
		"schema_version": "1", "kind": "backup", "project": "testproject", "result": "ok",
		"backup_key": res.BackupID, "bytes": float64(len(fakeDumpBytes)), "encrypted": true,
		"restored_rows": nil,
	}
	for k, v := range checks {
		if hb[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, hb[k], hb[k], v)
		}
	}
	if _, err := time.Parse(time.RFC3339, hb["at"].(string)); err != nil || !strings.HasSuffix(hb["at"].(string), "Z") {
		t.Errorf("at = %v is not RFC 3339 UTC", hb["at"])
	}
	for _, leak := range []string{"s3cretpw", "r2:", "bkt", "hbremote", "postgresql://", "localhost"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("heartbeat contains %q", leak)
		}
	}
}

// TestStreamHeartbeat_FailedBackupWritesNone: whichever leg fails, no
// heartbeat object exists afterwards and Stream reports the failure.
func TestStreamHeartbeat_FailedBackupWritesNone(t *testing.T) {
	cases := map[string]string{
		"pg_dump fails":       "FAKE_PGDUMP_FAIL",
		"backup upload fails": "FAKE_RCLONE_FAIL",
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			store, _ := fakeTools(t)
			val := "1"
			if env == "FAKE_RCLONE_FAIL" {
				val = "bkt/nself-web" // fails only the backup object, not the heartbeat remote
			}
			t.Setenv(env, val)
			res, err := Stream(context.Background(), minimalConfig(), streamOpts())
			if err == nil || res != nil {
				t.Fatalf("Stream = %v, %v; want an error and no result", res, err)
			}
			if _, statErr := os.Stat(filepath.Join(store, "hbremote_hb_testproject_backup.json")); statErr == nil {
				t.Error("a failed backup wrote a heartbeat")
			}
		})
	}
}

// TestStreamHeartbeat_UploadFailure: a heartbeat outage keeps the backup and
// is an error only with HeartbeatRequired.
func TestStreamHeartbeat_UploadFailure(t *testing.T) {
	fakeTools(t)
	t.Setenv("FAKE_RCLONE_FAIL", "backup.json")
	res, err := Stream(context.Background(), minimalConfig(), streamOpts())
	if err != nil || res == nil {
		t.Fatalf("default: Stream = %v, %v; the backup must succeed", res, err)
	}
	opts := streamOpts()
	opts.HeartbeatRequired = true
	res, err = Stream(context.Background(), minimalConfig(), opts)
	if err == nil || res == nil {
		t.Fatalf("required: Stream = %v, %v; want the result and an error", res, err)
	}
}

// TestStreamHeartbeat_RemoteSources: no remote, no heartbeat and no psql call;
// the environment variable works when the flag is empty.
func TestStreamHeartbeat_RemoteSources(t *testing.T) {
	store, log := fakeTools(t)
	opts := streamOpts()
	opts.HeartbeatTo = ""
	if _, err := Stream(context.Background(), minimalConfig(), opts); err != nil {
		t.Fatal(err)
	}
	for _, c := range readLog(t, log) {
		if c == "psql" || strings.Contains(c, "backup.json") {
			t.Fatalf("no remote configured, but %q ran", c)
		}
	}
	t.Setenv("NSELF_BACKUP_HEARTBEAT_REMOTE", "envhb:b")
	if _, err := Stream(context.Background(), minimalConfig(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store, "envhb_b_testproject_backup.json")); err != nil {
		t.Errorf("environment remote not used: %v", err)
	}
}

// TestStreamHeartbeat_UnreadableRows: when the estimates cannot be read the
// heartbeat is still written with approx_rows null (never {}), and the psql
// error never carries the connection password.
func TestStreamHeartbeat_UnreadableRows(t *testing.T) {
	store, _ := fakeTools(t)
	t.Setenv("FAKE_PSQL_FAIL", "1")
	cfg := minimalConfig()
	cfg.Postgres.Password = "s3cretpw"
	if _, err := Stream(context.Background(), cfg, streamOpts()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(store, "hbremote_hb_testproject_backup.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"approx_rows": null`) {
		t.Errorf("approx_rows should be null:\n%s", raw)
	}
	_, qerr := ReadApproxRows(context.Background(), buildPgURL(cfg))
	if qerr == nil || strings.Contains(qerr.Error(), "s3cretpw") {
		t.Errorf("psql error must exist and must not contain the password: %v", qerr)
	}
}

// TestHeartbeatRefuses covers the guards: nothing uploaded, a project name
// that would add a path segment, and an empty remote.
func TestHeartbeatRefuses(t *testing.T) {
	store, _ := fakeTools(t)
	ctx := context.Background()
	if err := publishHeartbeat(ctx, "p", "hb:b", &StreamResult{BackupID: "k", Bytes: 0}, nil); err == nil {
		t.Error("zero bytes must not produce a heartbeat")
	}
	for _, project := range []string{"a/b", "..", "", "-x"} {
		if err := WriteHeartbeat(ctx, "hb:b", Heartbeat{Project: project}); err == nil {
			t.Errorf("project %q must be refused", project)
		}
	}
	if err := WriteHeartbeat(ctx, " ", Heartbeat{Project: "p"}); err == nil {
		t.Error("empty remote must be refused")
	}
	if entries, _ := os.ReadDir(store); len(entries) != 0 {
		t.Errorf("nothing should have been uploaded, found %d objects", len(entries))
	}
}

// TestHeartbeatApproxRowsPostgres runs the contract's SQL on a real postgres:16
// container through the docker funnel and parses the result. Skipped without a
// reachable Docker daemon.
func TestHeartbeatApproxRowsPostgres(t *testing.T) {
	if testing.Short() {
		t.Skip("needs docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	if _, _, err := docker.RunOneShot(ctx, docker.RunSpec{Image: "postgres:16-alpine", Args: []string{"true"}}); err != nil {
		t.Skipf("docker not available: %v", err)
	}
	const script = `set -e
docker-entrypoint.sh postgres >/tmp/pg.log 2>&1 &
i=0
until [ "$(grep -c 'ready to accept connections' /tmp/pg.log 2>/dev/null)" -ge 2 ]; do
  i=$((i+1)); [ "$i" -gt 90 ] && { cat /tmp/pg.log; exit 1; }; sleep 1
done
psql -U postgres -v ON_ERROR_STOP=1 -q -c "CREATE TABLE public.users(id int); CREATE TABLE public.orders(id int); INSERT INTO public.users SELECT generate_series(1,1000); INSERT INTO public.orders SELECT generate_series(1,25);"
i=0
while :; do
  out=$(psql -U postgres -X -A -t -F "$(printf '\t')" -c "$APPROX_SQL")
  case "$out" in *"public.users$(printf '\t')1000"*) break;; esac
  i=$((i+1)); [ "$i" -gt 30 ] && break; sleep 1
done
echo BEGIN-ROWS; echo "$out"; echo END-ROWS`
	stdout, _, err := docker.RunOneShot(ctx, docker.RunSpec{
		Image:   "postgres:16-alpine",
		Args:    []string{"sh", "-c", script},
		EnvPass: map[string]string{"POSTGRES_PASSWORD": "unused-test-value", "APPROX_SQL": approxRowsSQL},
	})
	if err != nil {
		t.Fatalf("postgres container: %v", err)
	}
	i, j := strings.Index(stdout, "BEGIN-ROWS\n"), strings.Index(stdout, "END-ROWS")
	if i < 0 || j < i {
		t.Fatalf("no rows marker in output:\n%s", stdout)
	}
	rows, err := parseApproxRows(stdout[i+len("BEGIN-ROWS\n") : j])
	if err != nil {
		t.Fatal(err)
	}
	if rows["public.users"] != 1000 || rows["public.orders"] != 25 || len(rows) != 2 {
		t.Errorf("approx rows = %v, want users=1000 orders=25 only", rows)
	}
}

// TestStreamZeroBytesFails: an empty upload (only possible with --no-encrypt)
// is a failed backup: Stream returns an error wrapping ErrBackupFailed and
// writes no heartbeat, so the job cannot report green.
func TestStreamZeroBytesFails(t *testing.T) {
	store, _ := fakeTools(t)
	t.Setenv("FAKE_PGDUMP_EMPTY", "1")
	opts := streamOpts()
	opts.Recipients = nil
	opts.AllowUnencrypted = true
	res, err := Stream(context.Background(), minimalConfig(), opts)
	if err == nil || res != nil {
		t.Fatalf("Stream = %v, %v; want an error and no result", res, err)
	}
	if !errors.Is(err, errs.ErrBackupFailed) || !strings.Contains(err.Error(), "0 bytes") {
		t.Errorf("err = %v, want ErrBackupFailed mentioning 0 bytes", err)
	}
	if _, statErr := os.Stat(filepath.Join(store, "hbremote_hb_testproject_backup.json")); statErr == nil {
		t.Error("an empty backup wrote a heartbeat")
	}
}

// TestHeartbeatPasswordNotInArgv: the row-estimate psql call gets the DSN
// without its password and the password in PGPASSWORD.
func TestHeartbeatPasswordNotInArgv(t *testing.T) {
	store, _ := fakeTools(t)
	cfg := minimalConfig()
	cfg.Postgres.Password = "s3cretpw"
	if _, err := ReadApproxRows(context.Background(), buildPgURL(cfg)); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(store), "psql.args"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.SplitN(string(raw), "\n", 2)[0], "s3cretpw") {
		t.Errorf("password is in psql argv: %s", raw)
	}
	if !strings.Contains(string(raw), "pgpassword: s3cretpw") {
		t.Errorf("PGPASSWORD not set for psql: %s", raw)
	}
}

// TestSplitPgPassword covers the DSN split.
func TestSplitPgPassword(t *testing.T) {
	u, pw := splitPgPassword("postgresql://admin:p%40ss@localhost:5432/db?sslmode=disable")
	if pw != "p@ss" || strings.Contains(u, "p%40ss") || !strings.Contains(u, "admin@localhost:5432/db") {
		t.Errorf("got %q, %q", u, pw)
	}
	if u, pw := splitPgPassword("postgresql://admin@localhost/db"); pw != "" || u != "postgresql://admin@localhost/db" {
		t.Errorf("no-password DSN changed: %q, %q", u, pw)
	}
}
