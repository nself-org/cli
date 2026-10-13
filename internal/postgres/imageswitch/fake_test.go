package imageswitch

// fake_test.go — an in-memory docker, stack and applier for the engine tests.
//
// The fake recognises the one-shot scripts by name and writes the stage files the
// real scripts would write, from a small model of the old cluster. Knobs inject
// the faults the tests need; a kill is simulated by a panic that the test recovers
// (nothing after it runs, as after SIGKILL).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// killed is the panic value of a simulated SIGKILL.
type killed struct{ at string }

const (
	oldImage = "postgres:16-alpine"
	newImage = "pgvector/pgvector:pg16"
	// A fake password and password hash: they must never reach a log, state or error.
	fakePassword = "FAKE-TEST-PASSWORD-0000"
	fakeHash     = "SCRAM-SHA-256$4096:fake-salt$fake-stored:fake-server"
)

// fakeEnv is the whole fake world of one test.
type fakeEnv struct {
	t   *testing.T
	mu  sync.Mutex
	dir string

	pgRunning  bool
	pgvector   bool // the running postgres is a pgvector image
	volumes    map[string]bool
	labels     map[string]map[string]string
	removed    []string // containers removed by name
	running    map[string]bool
	calls      []string // ordered log: "sql: ...", "oneshot: <name>", "stack.up: ...", ...
	envApplied []map[string]string
	roSet      map[string]bool // databases with default_transaction_read_only on

	// knobs
	dbs       []string
	cluster   string // cluster document served by capcluster.sql
	ext       string // extensions in the target image ("a\nb")
	locales   string
	dfKB      int64
	failStage string // script name whose one-shot fails: dump.sh, restore.sh, recheck.sh
	failErr   string
	mutate    string // "restore": the restored capture differs; "recheck": the original changed
	killAt    string // "restore", "apply", "start"
	applyErr  error
	stopPGErr error // returned when postgres alone is stopped
	healthErr error // returned by WaitHealthy
	// a database appears when the writers stop / while the dump runs
	newDBOnStop, newDBAtDump bool
	onDump                   func() // called when the dump one-shot runs
	sessions                 string // answer to the session count query
	instExt                  string // extensions installed in every database
}

// newFake returns a world with a running alpine postgres, two databases and a stack.
func newFake(t *testing.T) (*fakeEnv, *Switch) {
	t.Helper()
	was := onWindows
	onWindows = false
	t.Cleanup(func() { onWindows = was })
	f := &fakeEnv{
		t: t, dir: t.TempDir(), pgRunning: true, volumes: map[string]bool{"proj_postgres_data": true}, labels: map[string]map[string]string{},
		running: map[string]bool{"postgres": true, "hasura": true, "auth": true, "nginx": true}, roSet: map[string]bool{},
		dbs: []string{"app", "postgres"}, ext: "plpgsql\npgcrypto\nvector", locales: "C\nC.utf8\nen_US.utf8\nPOSIX", dfKB: 50 << 20, sessions: "0", instExt: "plpgsql",
	}
	f.cluster = f.clusterJSON(f.dbs)
	s := New(Config{
		ProjectDir: f.dir, ProjectName: "proj", Service: "postgres", Container: "proj_postgres", User: "postgres", Password: fakePassword,
		TargetFor: func(major int) (string, error) {
			if major != 16 {
				return "", fmt.Errorf("no pgvector image for %d", major)
			}
			return newImage, nil
		},
		PrevEnv: map[string]string{EnvImage: "", EnvDataVolume: ""},
	}, Deps{Runtime: f, Stack: f, Applier: f, Now: func() time.Time { return time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC) },
		FreeBytes: func(string) (uint64, error) { return 1 << 40, nil }})
	return f, s
}

// clusterJSON is a cluster document for the given databases.
func (f *fakeEnv) clusterJSON(dbs []string) string {
	var parts []string
	for _, d := range dbs {
		parts = append(parts, fmt.Sprintf(`{"name":%q,"owner":"postgres","provider":"c","allow_connections":true,"collate":"en_US.utf8","ctype":"en_US.utf8","create":"CREATE DATABASE %s WITH TEMPLATE = template0 OWNER = postgres ENCODING = 'UTF8' LC_COLLATE = 'en_US.utf8' LC_CTYPE = 'en_US.utf8' CONNECTION LIMIT = -1","explicit_acl":false,"acl":[],"settings":[]}`, d, d))
	}
	return fmt.Sprintf(`{"major":16,"bootstrap":"postgres","tablespaces":[],"databases":[%s]}`, strings.Join(parts, ","))
}

func (f *fakeEnv) log(format string, a ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, fmt.Sprintf(format, a...))
}

// called reports whether a logged call contains substr.
func (f *fakeEnv) called(substr string) bool {
	for _, c := range f.calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// Inspect implements Runtime.
func (f *fakeEnv) Inspect(_ context.Context, name string) (ContainerInfo, error) {
	img := oldImage
	if f.pgvector {
		img = newImage
	}
	return ContainerInfo{Image: img, Running: f.pgRunning, DataVolume: "proj_postgres_data", Networks: []string{"bridge", "proj_network"}, ComposeProject: "proj"}, nil
}

// ExecSQL implements Runtime. It answers the queries preflight and quiesce ask.
func (f *fakeEnv) ExecSQL(_ context.Context, _, _, db, sql string) (string, error) {
	f.log("sql(%s): %s", db, strings.Join(strings.Fields(sql), " "))
	switch {
	case strings.Contains(sql, "'major'"):
		return f.cluster + "\n", nil
	case strings.Contains(sql, "pg_database_size"):
		return "0|1048576|\n", nil
	case strings.Contains(sql, "FROM pg_extension"):
		return f.instExt + "\n", nil
	case strings.Contains(sql, "SET default_transaction_read_only = on"):
		f.roSet[strings.Split(sql, `"`)[1]] = true
	case strings.Contains(sql, "RESET default_transaction_read_only"):
		delete(f.roSet, strings.Split(sql, `"`)[1])
	case strings.Contains(sql, "SELECT count(*) FROM pg_stat_activity"):
		return f.sessions + "\n", nil
	}
	return "", nil
}

// RunOneShot implements Runtime: it writes what the real script would write.
func (f *fakeEnv) RunOneShot(_ context.Context, spec docker.RunSpec) (string, string, error) {
	f.log("oneshot: %s %v", spec.Name, spec.Args)
	if len(spec.Args) > 0 && spec.Args[0] == "sh" { // the probe
		return fmt.Sprintf("/dev/x 1 1 %d 1%% /probe\n---EXT\n%s\n---LOC\n%s\n", f.dfKB, f.ext, f.locales), "", nil
	}
	script := filepath.Base(spec.Args[len(spec.Args)-1])
	if f.failStage == script {
		return "", "psql:/stage/globals.restore.sql:4: ERROR: " + f.failErr + " PASSWORD '" + fakeHash + "'", errors.New("exit status 3")
	}
	stage := ""
	for _, m := range spec.Mounts {
		if m.Destination == "/stage" {
			stage = m.Source
		}
	}
	switch script {
	case "dump.sh":
		if f.killAt == "dump" {
			f.killAt = ""
			panic(killed{"dump"})
		}
		if f.onDump != nil {
			f.onDump()
		}
		if f.newDBAtDump {
			f.newDBAtDump = false
			f.dbs = append(f.dbs, "late")
			f.cluster = f.clusterJSON(f.dbs)
		}
		f.writeSet(stage, "old", "")
		for i := range f.dbs {
			f.put(stage, dbFile(i, "dump"), "PGDMP")
		}
	case "restore.sh":
		if f.killAt == "restore" {
			f.killAt = ""
			panic(killed{"restore"})
		}
		f.writeSet(stage, "new", f.mutate)
		f.volumes["used"] = true
	case "recheck.sh":
		if f.killAt == "recheck" {
			f.killAt = ""
			panic(killed{"recheck"})
		}
		f.writeSet(stage, "chk", f.mutate)
	}
	return "", "", nil
}

// writeSet writes the capture files of one suffix; mutate "restore" or "recheck" changes one table hash.
func (f *fakeEnv) writeSet(stage, suffix, mutate string) {
	f.put(stage, "cluster."+suffix+".json", f.cluster)
	f.put(stage, "globals."+suffix+".sql", "-- dump\n\\restrict tok"+suffix+"\nCREATE ROLE postgres;\nALTER ROLE postgres WITH SUPERUSER PASSWORD '"+fakeHash+"';\n")
	for i := range f.dbs {
		hash := "111"
		if (mutate == "restore" && suffix == "new") || (mutate == "recheck" && suffix == "chk") {
			hash = "222"
		}
		f.put(stage, dbFile(i, "cap."+suffix+".json"), fmt.Sprintf(`{"encoding":"UTF8","tables":[{"table":"public.t","count":"5","hash":"%s"}],"sequences":[["public","sq",1000]],"extensions":[["plpgsql","1.0"]]}`, hash))
		f.put(stage, dbFile(i, "schema."+suffix+".sql"), "-- Dumped "+suffix+"\nSET x = 1;\nCREATE TABLE public.t (a integer);\n")
	}
}

func (f *fakeEnv) put(stage, name, body string) {
	if err := os.WriteFile(filepath.Join(stage, name), []byte(body), 0o600); err != nil {
		f.t.Fatalf("fake stage write: %v", err)
	}
}

// RemoveContainer implements Runtime.
func (f *fakeEnv) RemoveContainer(_ context.Context, name string) error {
	f.removed = append(f.removed, name)
	return nil
}

// VolumeCreate implements Runtime.
func (f *fakeEnv) VolumeCreate(_ context.Context, name string, labels map[string]string) error {
	f.volumes[name], f.labels[name] = true, labels
	f.log("volume.create: %s", name)
	return nil
}

// VolumeExists implements Runtime.
func (f *fakeEnv) VolumeExists(_ context.Context, name string) (bool, error) {
	return f.volumes[name], nil
}

// VolumeRemove implements Runtime.
func (f *fakeEnv) VolumeRemove(_ context.Context, name string) error {
	delete(f.volumes, name)
	f.log("volume.rm: %s", name)
	return nil
}

// Running implements Stack.
func (f *fakeEnv) Running(context.Context) ([]string, error) {
	var out []string
	for _, n := range []string{"auth", "hasura", "nginx", "postgres"} {
		if f.running[n] {
			out = append(out, n)
		}
	}
	return out, nil
}

// Stop implements Stack.
func (f *fakeEnv) Stop(_ context.Context, services ...string) error {
	f.log("stack.stop: %s", strings.Join(services, ","))
	if f.stopPGErr != nil && len(services) == 1 && services[0] == "postgres" {
		return f.stopPGErr
	}
	if f.newDBOnStop && len(services) > 1 {
		f.newDBOnStop = false
		f.dbs = append(f.dbs, "late")
		f.cluster = f.clusterJSON(f.dbs)
	}
	if f.killAt == "stop" && len(services) > 0 && services[0] == "auth" {
		f.killAt = ""
		panic(killed{"stop"})
	}
	for _, s := range services {
		f.running[s] = false
		if s == "postgres" {
			f.pgRunning = false
		}
	}
	return nil
}

// Up implements Stack.
func (f *fakeEnv) Up(_ context.Context, services ...string) error {
	f.log("stack.up: %s", strings.Join(services, ","))
	if f.killAt == "start" && f.pgvector {
		f.killAt = ""
		panic(killed{"start"})
	}
	if len(services) == 0 {
		services = []string{"auth", "hasura", "nginx", "postgres"}
	}
	for _, s := range services {
		f.running[s] = true
		if s == "postgres" {
			f.pgRunning = true
		}
	}
	return nil
}

// WaitHealthy implements Stack.
func (f *fakeEnv) WaitHealthy(context.Context, string) error { return f.healthErr }

// Plan implements Applier.
func (f *fakeEnv) Plan(_ context.Context, env map[string]string) (any, error) {
	return map[string]any{"env": env}, nil
}

// Apply implements Applier.
func (f *fakeEnv) Apply(_ context.Context, env map[string]string) error {
	f.log("apply: %v", env)
	if f.killAt == "apply" {
		f.killAt = ""
		panic(killed{"apply"})
	}
	if f.applyErr != nil && env[EnvImage] == newImage { // only the forward rewrite fails; putting the old env back works
		return f.applyErr
	}
	f.envApplied = append(f.envApplied, env)
	f.pgvector = env[EnvImage] == newImage
	return nil
}

// runKilled runs fn and reports whether it was "killed" at the given point.
func runKilled(fn func()) (k *killed) {
	defer func() {
		if r := recover(); r != nil {
			kv, ok := r.(killed)
			if !ok {
				panic(r)
			}
			k = &kv
		}
	}()
	fn()
	return nil
}
