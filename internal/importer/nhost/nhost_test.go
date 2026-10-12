package nhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/portable"
)

func TestRegisteredWithEnvNameFlags(t *testing.T) {
	var found importer.Source
	for _, s := range importer.Sources() {
		if s.Name() == "nhost" {
			found = s
		}
	}
	if found == nil {
		t.Fatal("nhost not registered")
	}
	env := map[string]bool{}
	for _, f := range found.Flags() {
		env[f.Name] = f.EnvVarName
	}
	for _, name := range []string{"db-url-env", "admin-secret-env", "s3-access-key-env", "s3-secret-env"} {
		if !env[name] {
			t.Errorf("secret flag %s must name an environment variable", name)
		}
	}
	for _, name := range []string{"hasura-url", "storage-endpoint", "bucket"} {
		if v, ok := env[name]; !ok || v {
			t.Errorf("flag %s missing or marked env-name", name)
		}
	}
	got := pgInput(importer.Input{Values: map[string]string{"db-url-env": "SRC"}})
	if got.Values["from-env"] != "SRC" {
		t.Fatalf("pgInput %+v", got)
	}
	if n := keepNotes([]string{"looks like nhost", "other"}); len(n) != 1 || n[0] != "other" {
		t.Fatalf("keepNotes %v", n)
	}
}

func s3(t *testing.T, objects map[string]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := objects[strings.TrimPrefix(r.URL.Path, "/files/")]
		if !ok || !strings.HasPrefix(r.URL.Path, "/files/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func input(srv *httptest.Server) importer.Input {
	env := map[string]string{"S3_ACCESS": "fixture-access", "S3_SECRET": "fixture-secret-key"}
	return importer.Input{
		Values: map[string]string{"storage-endpoint": srv.URL, "s3-access-key-env": "S3_ACCESS", "s3-secret-env": "S3_SECRET", "bucket": "files"},
		Env:    func(k string) string { return env[k] },
	}
}

func i64(n int64) *int64 { return &n }

func build(t *testing.T, in importer.Input, count int64, rows []fileRow) (portable.Manifest, *portable.Reader, error) {
	t.Helper()
	old := storageState
	storageState = func(context.Context, importer.Input) (int64, []fileRow, error) { return count, rows, nil }
	t.Cleanup(func() { storageState = old })
	dir := filepath.Join(t.TempDir(), "stage")
	w, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "nhost"}}
	if err = addStorage(context.Background(), in)(w, &m); err != nil {
		return m, nil, err
	}
	if _, err = w.Finish(m); err != nil {
		t.Fatal(err)
	}
	r, err := portable.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r.Manifest(), r, nil
}

func TestAddStorageWritesEveryObjectWithSha(t *testing.T) {
	objects := map[string]string{"11111111-1111-1111-1111-111111111111": "alpha", "22222222-2222-2222-2222-222222222222": "bravo-bytes"}
	srv := s3(t, objects)
	rows := []fileRow{{"11111111-1111-1111-1111-111111111111", i64(5)}, {"22222222-2222-2222-2222-222222222222", i64(11)}}
	m, r, err := build(t, input(srv), 2, rows)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Storage.Objects) != 2 || len(m.Storage.Buckets) != 1 || m.Storage.Buckets[0].Name != "files" || m.Storage.Buckets[0].Objects != 2 || m.Storage.Buckets[0].Bytes != 16 {
		t.Fatalf("storage %+v", m.Storage)
	}
	for _, o := range m.Storage.Objects {
		sum := sha256.Sum256([]byte(objects[o.Key]))
		if o.Bucket != "files" || o.SHA256 != hex.EncodeToString(sum[:]) {
			t.Errorf("object %+v", o)
		}
		rc, err := r.Open(o.Member)
		if err != nil {
			t.Fatal(err)
		}
		_ = rc.Close()
	}
	if len(m.SourceCounts) != 1 || m.SourceCounts[0].Kind != "objects" || m.SourceCounts[0].ID != "files" || m.SourceCounts[0].Count != 2 {
		t.Fatalf("source counts %+v", m.SourceCounts)
	}
}

func TestAddStorageRefusals(t *testing.T) {
	srv := s3(t, map[string]string{"aaaaaaaa-0000-0000-0000-000000000000": "four"})
	good := fileRow{"aaaaaaaa-0000-0000-0000-000000000000", i64(4)}
	cases := []struct {
		name  string
		edit  func(*importer.Input)
		count int64
		rows  []fileRow
		want  string
	}{
		{"missing object", nil, 1, []fileRow{{"bbbbbbbb-0000-0000-0000-000000000000", i64(4)}}, "source object unavailable"},
		{"size differs", nil, 1, []fileRow{{good.id, i64(99)}}, "size differs"},
		{"no endpoint flag", func(in *importer.Input) { delete(in.Values, "storage-endpoint") }, 1, []fileRow{good}, "--storage-endpoint"},
		{"no bucket flag", func(in *importer.Input) { delete(in.Values, "bucket") }, 1, []fileRow{good}, "--bucket"},
		{"remote http endpoint", func(in *importer.Input) { in.Values["storage-endpoint"] = "http://s3.example.com" }, 1, []fileRow{good}, "https"},
		{"empty secret env", func(in *importer.Input) { in.Values["s3-secret-env"] = "UNSET_NAME" }, 1, []fileRow{good}, "E517"},
	}
	for _, c := range cases {
		in := input(srv)
		if c.edit != nil {
			c.edit(&in)
		}
		_, _, err := build(t, in, c.count, c.rows)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		for _, secret := range []string{"fixture-secret-key", "fixture-access", c.rows[0].id} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: error leaks %q: %v", c.name, secret, err)
			}
		}
	}
}

func TestAddStorageWithNoFilesNeedsNoFlags(t *testing.T) {
	m, _, err := build(t, importer.Input{Values: map[string]string{}}, 0, nil)
	if err != nil || len(m.Storage.Objects) != 0 || len(m.SourceCounts) != 0 {
		t.Fatalf("no files: %+v %v", m, err)
	}
}
