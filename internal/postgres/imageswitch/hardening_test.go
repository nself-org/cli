package imageswitch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/postgres/tablehash"
)

// specRecorder wraps the fake runtime and keeps every one-shot spec.
type specRecorder struct {
	*fakeEnv
	specs []docker.RunSpec
}

func (r *specRecorder) RunOneShot(ctx context.Context, spec docker.RunSpec) (string, string, error) {
	r.specs = append(r.specs, spec)
	return r.fakeEnv.RunOneShot(ctx, spec)
}

func (r *specRecorder) byName(suffix string) docker.RunSpec {
	for _, s := range r.specs {
		if strings.HasSuffix(s.Name, suffix) {
			return s
		}
	}
	return docker.RunSpec{}
}

func TestOneShotSpecsAreHardened(t *testing.T) {
	f, s := newFake(t)
	rec := &specRecorder{fakeEnv: f}
	s.d.Runtime = rec
	if _, err := s.Run(context.Background(), RunOptions{}); err != nil {
		t.Fatal(err)
	}
	dump, restore := rec.byName("-dump"), rec.byName("-restore")
	if dump.Image != newImage || restore.Image != newImage {
		t.Errorf("both tools run in the target image, got %q and %q", dump.Image, restore.Image)
	}
	if dump.EnvPass["PGPASSWORD"] != fakePassword || dump.EnvPass["PGHOST"] != "proj_postgres" {
		t.Errorf("dump env = %v", dump.EnvPass)
	}
	if dump.Network != "proj_network" || restore.Network != "none" {
		t.Errorf("networks: dump %q restore %q (the restore server must have none)", dump.Network, restore.Network)
	}
	if len(dump.CapDrop) == 0 || dump.CapDrop[0] != "ALL" {
		t.Error("the dump one-shot drops every capability")
	}
	// D-0803: the restore one-shot runs as root but only with the five capabilities restore.sh needs.
	if len(restore.CapDrop) != 1 || restore.CapDrop[0] != "ALL" {
		t.Errorf("the restore one-shot drops every capability, got %v", restore.CapDrop)
	}
	if got, want := strings.Join(restore.CapAdd, ","), "CHOWN,SETUID,SETGID,DAC_OVERRIDE,FOWNER"; got != want {
		t.Errorf("restore CapAdd = %q, want %q", got, want)
	}
	for _, spec := range rec.specs {
		for _, a := range spec.Args {
			if strings.Contains(a, fakePassword) || strings.Contains(a, fakeHash) {
				t.Errorf("a secret reached argv of %s", spec.Name)
			}
		}
		for _, m := range spec.Mounts {
			if strings.Contains(m.Source, "docker.sock") || strings.Contains(m.Source, "postgres/init") || strings.Contains(m.Destination, "initdb.d") {
				t.Errorf("%s mounts %s:%s", spec.Name, m.Source, m.Destination)
			}
		}
	}
	if _, ok := restore.EnvPass["PGPASSWORD"]; ok {
		t.Error("the restore server needs no password")
	}
	mounted := false
	for _, m := range restore.Mounts {
		if m.Destination == "/var/lib/postgresql/data" && strings.Contains(m.Source, "pgvector") {
			mounted = true
		}
		if m.Source == "proj_postgres_data" {
			t.Error("the restore must never mount the old volume")
		}
	}
	if !mounted {
		t.Errorf("the restore mounts the new volume at the data directory: %v", restore.Mounts)
	}
	for _, c := range f.calls {
		if strings.Contains(c, fakePassword) || strings.Contains(c, fakeHash) {
			t.Errorf("a secret reached a call: %s", c)
		}
	}
}

func TestStateAndStageAreOwnerOnlyAndHoldNoSecret(t *testing.T) {
	f, s := newFake(t)
	f.killAt = "restore"
	runKilled(func() { _, _ = s.Run(context.Background(), RunOptions{}) })
	raw, err := os.ReadFile(StatePath(f.dir))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{fakePassword, fakeHash, "PASSWORD"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("state holds %q", secret)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	for path, want := range map[string]os.FileMode{
		filepath.Join(f.dir, ".nself", "image-switch"): 0o700,
		StatePath(f.dir): 0o600,
		filepath.Join(f.dir, ".nself", "image-switch", "stage"):               0o700,
		filepath.Join(f.dir, ".nself", "image-switch", "stage", "restore.sh"): 0o600,
	} {
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s is %o, want %o", path, fi.Mode().Perm(), want)
		}
	}
}

func TestIncompleteFailsClosedOnUnreadableState(t *testing.T) {
	dir := t.TempDir()
	if _, bad := Incomplete(dir); bad {
		t.Error("no state is not incomplete")
	}
	if err := os.MkdirAll(filepath.Dir(StatePath(dir)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, bad := Incomplete(dir); !bad || st == nil {
		t.Error("an unreadable state must count as incomplete")
	}
	for _, p := range []Phase{PhaseDone, PhaseRolledBack} {
		if !p.Complete() {
			t.Errorf("%s is complete", p)
		}
	}
	for _, p := range []Phase{PhaseStarted, PhaseSwitching, PhaseSwitched, ""} {
		if p.Complete() {
			t.Errorf("%q is not complete", p)
		}
	}
}

// recordingDB is a tablehash.DB that records the hashing query.
type recordingDB struct{ queries []string }

func (r *recordingDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (r *recordingDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	r.queries = append(r.queries, sql)
	return emptyRow{}
}

type emptyRow struct{}

func (emptyRow) Scan(dest ...any) error {
	for _, d := range dest {
		switch p := d.(type) {
		case *string:
			*p = "UTF8"
		case *int64:
			*p = 0
		}
	}
	return nil
}

// TestCaptureSQLMatchesTablehash pins capdb.sql to the definition in internal/postgres/tablehash:
// the same session settings in the same order, and the same count and hash expression.
func TestCaptureSQLMatchesTablehash(t *testing.T) {
	sql, err := Asset("capdb.sql")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(sql), " ")
	prev := -1
	for _, kv := range tablehash.Settings {
		needle := "set_config('" + kv[0] + "', '" + kv[1] + "', false)"
		at := strings.Index(flat, needle)
		if at < 0 || at < prev {
			t.Errorf("capdb.sql lacks %s, or has it out of order", needle)
		}
		prev = at
	}
	db := &recordingDB{}
	if _, _, err := tablehash.Snapshot(context.Background(), db, []tablehash.Table{{Schema: "public", Name: "t"}}); err != nil {
		t.Fatal(err)
	}
	var expr string
	for _, q := range db.queries {
		if strings.Contains(q, "hashtextextended") {
			expr = q[strings.Index(q, "coalesce(") : strings.Index(q, "::text")+len("::text")]
		}
	}
	if expr == "" || !strings.Contains(flat, expr) {
		t.Errorf("tablehash hashes with %q, capdb.sql does not contain it", expr)
	}
	if !strings.Contains(flat, "count(*) AS c") {
		t.Error("capdb.sql counts rows with count(*)")
	}
}

func TestScriptsKeepTheirPromises(t *testing.T) {
	restore, _ := Asset("restore.sh")
	for _, want := range []string{"listen_addresses=''", "--exit-on-error", "set -euo pipefail", "chown -R", "gosu postgres initdb"} {
		if !strings.Contains(restore, want) {
			t.Errorf("restore.sh lacks %q", want)
		}
	}
	for _, bad := range []string{"initdb.d", "ON_ERROR_STOP=0", "fsync=off", "ON_ERROR_STOP 0"} {
		if strings.Contains(restore, bad) {
			t.Errorf("restore.sh must not contain %q", bad)
		}
	}
	dump, _ := Asset("dump.sh")
	if !strings.Contains(dump, "pg_export_snapshot") || !strings.Contains(dump, "--snapshot=") {
		t.Error("dump.sh dumps inside an exported snapshot")
	}
	for _, name := range []string{"dump.sh", "recheck.sh", "restore.sh", "common.sh", "capdb.sql", "capcluster.sql"} {
		body, err := Asset(name)
		if err != nil || body == "" {
			t.Errorf("asset %s: %v", name, err)
		}
		if strings.Contains(body, "pg_stat_") {
			t.Errorf("%s reads pg_stat counters, which the commands' own reads move", name)
		}
	}
}
