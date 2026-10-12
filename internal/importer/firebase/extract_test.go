package firebase

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestCheckObjectCatchesTruncationAndRecoding(t *testing.T) {
	sum := md5.Sum([]byte("hello"))
	good := objectInfo{Name: "a", Size: "5", MD5Hash: base64.StdEncoding.EncodeToString(sum[:])}
	if err := checkObject(good, 5, sum[:]); err != nil {
		t.Fatal(err)
	}
	if checkObject(good, 4, sum[:]) == nil {
		t.Fatal("short object accepted")
	}
	other := md5.Sum([]byte("HELLO"))
	if checkObject(good, 5, other[:]) == nil {
		t.Fatal("different bytes accepted")
	}
	if err := checkObject(objectInfo{Name: "composite", Size: "5"}, 5, other[:]); err != nil {
		t.Fatalf("an object without MD5 is judged by size only: %v", err)
	}
}

// TestObjectDownloadsAskForStoredBytes: the bulk client sends Accept-Encoding gzip and does not
// decode, so an object stored gzip-encoded is copied as stored.
func TestObjectDownloadsAskForStoredBytes(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Accept-Encoding")
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write([]byte("STORED-GZIP-BYTES"))
	}))
	defer srv.Close()
	c := newClient(ownerToken{})
	g := &gcs{c: c, base: srv.URL, bucket: "b"}
	rc, err := g.open(context.Background(), "x.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, _ := io.ReadAll(rc)
	if seen != "gzip" || string(b) != "STORED-GZIP-BYTES" {
		t.Fatalf("accept-encoding %q body %q", seen, b)
	}
}

func TestRequestsRetryTransientFailures(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			if calls == 1 {
				hj, _ := w.(http.Hijacker)
				conn, _, _ := hj.Hijack()
				_ = conn.Close() // transport reset
				return
			}
			w.Header().Set("Retry-After", "0")
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"collectionIds":["a"]}`))
	}))
	defer srv.Close()
	c := newClient(ownerToken{})
	c.backoff = func(int) time.Duration { return 0 }
	f := &firestore{c: c, project: "demo-nself", base: srv.URL}
	ids, err := f.collectionIDs(context.Background(), "")
	if err != nil || len(ids) != 1 || calls != 3 {
		t.Fatalf("ids %v err %v calls %d", ids, err, calls)
	}
}
