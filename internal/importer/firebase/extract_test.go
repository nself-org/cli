package firebase

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/scratch"
	"github.com/nself-org/cli/internal/portable"
)

// stubScratch stands in for the container build: it writes the two db members and a manifest.
func stubScratch(t *testing.T) {
	t.Helper()
	old := scratchBuild
	t.Cleanup(func() { scratchBuild = old })
	scratchBuild = func(_ context.Context, _ *config.Config, ddl string, copies []scratch.CopyFile, stage string) (portable.Manifest, error) {
		if !strings.Contains(ddl, "CREATE SCHEMA firebase;") || len(copies) != 3 {
			t.Fatalf("scratch input: %d copies\n%s", len(copies), ddl)
		}
		w, err := portable.NewWriter(stage)
		if err != nil {
			return portable.Manifest{}, err
		}
		for _, name := range []string{"db/data.dump", "db/schema.sql"} {
			if _, err = w.WriteFile(name, strings.NewReader("stub "+name)); err != nil {
				return portable.Manifest{}, err
			}
		}
		m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "postgres"}, DB: portable.DB{Schemas: []string{"firebase"}}}
		for _, c := range copies {
			m.DB.Tables = append(m.DB.Tables, portable.Table{Schema: c.Schema, Name: c.Table, Rows: 1, Hash: "1", PK: []string{}})
		}
		return w.Finish(m)
	}
}

func extractInput(t *testing.T, srvURL string, withBucket bool) importer.Input {
	t.Helper()
	host := strings.TrimPrefix(srvURL, "http://")
	values := map[string]string{"project": "demo-nself", "auth-export": authFixture}
	envs := map[string]string{"FIRESTORE_EMULATOR_HOST": host}
	if withBucket {
		values["bucket"] = "demo-bucket"
		envs["STORAGE_EMULATOR_HOST"] = host
	}
	return importer.Input{Values: values, Env: func(k string) string { return envs[k] }, Target: &config.Config{}}
}

func TestExtractBuildsPortableBundle(t *testing.T) {
	stubScratch(t)
	_, _, srv := fakeClient(t, fixtureDB())
	// fakeClient serves the first DB; serve a fresh one for the extract run.
	stage := filepath.Join(t.TempDir(), "stage")
	if _, err := (Source{}).Extract(context.Background(), extractInput(t, srv.URL, true), stage); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(stage)
	if err != nil {
		t.Fatal(err)
	}
	m := r.Manifest()
	if m.Producer.Source != "firebase" || m.Auth.Users != 3 || len(m.Auth.ResetRequired) != 3 || m.Auth.HashAlgorithms["unsupported"] != 3 {
		t.Fatalf("auth/producer %+v %+v", m.Producer, m.Auth)
	}
	if len(m.Storage.Objects) != 2 || len(m.Storage.Buckets) != 1 || m.Storage.Buckets[0].Objects != 2 {
		t.Fatalf("storage %+v", m.Storage)
	}
	counts := map[string]int64{}
	for _, c := range m.SourceCounts {
		counts[c.Kind+":"+c.ID] = c.Count
	}
	want := map[string]int64{"table:firebase.users": 3, "table:firebase.notes": 4, "table:firebase.users__orders": 5, "users:": 3, "objects:demo-bucket": 2}
	for k, v := range want {
		if counts[k] != v {
			t.Fatalf("source count %s = %d, want %d (all %v)", k, counts[k], v, counts)
		}
	}
	if err = r.Verify(); err != nil {
		t.Fatal(err)
	}
	users, _ := os.ReadFile(filepath.Join(stage, "auth", "users.jsonl"))
	if strings.Contains(string(users), "passwordHash") || strings.Contains(string(users), "ZmFrZS1oYXNo") || !strings.Contains(string(users), `"hash_algorithm":"unsupported"`) {
		t.Fatalf("users file leaks or lacks the algorithm: %s", users)
	}
	entries, _ := os.ReadDir(filepath.Dir(stage))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".extract-") {
			t.Fatalf("work dir left behind: %s", e.Name())
		}
	}
}

func TestExtractWithoutBucketSkipsStorage(t *testing.T) {
	stubScratch(t)
	_, _, srv := fakeClient(t, fixtureDB())
	stage := filepath.Join(t.TempDir(), "stage")
	if _, err := (Source{}).Extract(context.Background(), extractInput(t, srv.URL, false), stage); err != nil {
		t.Fatal(err)
	}
	r, _ := portable.Open(stage)
	if len(r.Manifest().Storage.Objects) != 0 {
		t.Fatal("objects without a bucket")
	}
}

func TestDetectReportsCollectionsAndUsers(t *testing.T) {
	_, _, srv := fakeClient(t, fixtureDB())
	d, err := (Source{}).Detect(context.Background(), extractInput(t, srv.URL, false))
	if err != nil || d.Kind != "firebase" || d.SourceUsers != 3 || len(d.Schemas) != 1 || d.Schemas[0] != "firebase" || !strings.Contains(strings.Join(d.Notes, "|"), "2 top-level collection(s)") {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestExtractRefusesEmptyProject(t *testing.T) {
	stubScratch(t)
	d := fixtureDB()
	d.subs[""] = nil
	_, _, srv := fakeClient(t, d)
	if _, err := (Source{}).Extract(context.Background(), extractInput(t, srv.URL, false), filepath.Join(t.TempDir(), "stage")); err == nil {
		t.Fatal("empty project accepted")
	}
}

func TestSourceRegistered(t *testing.T) {
	found := false
	for _, s := range importer.Sources() {
		found = found || s.Name() == "firebase"
	}
	if !found {
		t.Fatal("firebase not registered")
	}
}
