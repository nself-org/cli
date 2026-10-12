package nhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5"
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
	for _, name := range []string{"hasura-url", "storage-endpoint", "bucket", "s3-region", "s3-root-folder"} {
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
	files := portable.Table{Schema: "storage", Name: "files", Rows: count, Hash: "1"}
	return buildWith(t, in, storageSnapshot{uploaded: count, total: count, rows: rows}, []portable.Table{files})
}

func buildWith(t *testing.T, in importer.Input, snap storageSnapshot, tables []portable.Table) (portable.Manifest, *portable.Reader, error) {
	t.Helper()
	old := storageState
	storageState = func(context.Context, importer.Input) (storageSnapshot, error) { return snap, nil }
	t.Cleanup(func() { storageState = old })
	dir := filepath.Join(t.TempDir(), "stage")
	w, err := portable.NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "nhost"}, DB: portable.DB{Tables: tables}}
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
		{"count differs from list", nil, 2, []fileRow{good}, "source storage changed"},
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

func TestObjectKeyAndRegion(t *testing.T) {
	cases := map[string]string{"": "id1", "files": "files/id1", "/files/": "files/id1", "a/b": "a/b/id1"}
	for root, want := range cases {
		if got := objectKey(importer.Input{Values: map[string]string{"s3-root-folder": root}}, "id1"); got != want {
			t.Errorf("root %q: %q want %q", root, got, want)
		}
	}
	in := input(httptest.NewServer(http.NotFoundHandler()))
	in.Values["s3-region"] = "eu-west-1"
	c, err := s3Client(in)
	if err != nil || c.Region != "eu-west-1" {
		t.Fatalf("region: %+v %v", c, err)
	}
}

func TestAddStorageReadsUnderRootFolder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files/root/dir/cccccccc-0000-0000-0000-000000000000" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("rooted"))
	}))
	defer srv.Close()
	in := input(srv)
	in.Values["s3-root-folder"] = "root/dir"
	m, _, err := build(t, in, 1, []fileRow{{"cccccccc-0000-0000-0000-000000000000", i64(6)}})
	if err != nil || len(m.Storage.Objects) != 1 || m.Storage.Objects[0].Key != "cccccccc-0000-0000-0000-000000000000" {
		t.Fatalf("rooted read: %+v %v", m.Storage, err)
	}
}

func TestProbeObjectAndGetFailureClassification(t *testing.T) {
	status := http.StatusOK
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.WriteHeader(status)
	}))
	defer srv.Close()
	in := input(srv)
	in.Values["s3-root-folder"] = "root"
	if err := probeObject(context.Background(), in, "id1"); err != nil || path != "/files/root/id1" {
		t.Fatalf("reachable object refused: %v (path %s)", err, path)
	}
	status = http.StatusNotFound
	if err := probeObject(context.Background(), in, "id1"); err == nil || !strings.Contains(err.Error(), "unavailable for a storage.files row") {
		t.Errorf("404 must read as a missing object: %v", err)
	}
	for _, s := range []int{http.StatusForbidden, http.StatusInternalServerError} {
		status = s
		err := probeObject(context.Background(), in, "id1")
		if err == nil || !strings.Contains(err.Error(), "check --storage-endpoint") || strings.Contains(err.Error(), "fixture-secret-key") {
			t.Errorf("status %d: %v", s, err)
		}
	}
	srv.Close()
	if err := probeObject(context.Background(), in, "id1"); err == nil {
		t.Error("unreachable endpoint accepted")
	}
}

func TestStorageChangedAndLimits(t *testing.T) {
	srv := s3(t, map[string]string{"dddddddd-0000-0000-0000-000000000000": "data"})
	row := fileRow{"dddddddd-0000-0000-0000-000000000000", i64(4)}
	files := portable.Table{Schema: "storage", Name: "files", Rows: 3, Hash: "1"}
	// The dump saw 3 storage.files rows; the listing saw 4: a file was added after the dump.
	_, _, err := buildWith(t, input(srv), storageSnapshot{uploaded: 1, total: 4, rows: []fileRow{row}}, []portable.Table{files})
	if err == nil || !strings.Contains(err.Error(), "source storage changed") {
		t.Fatalf("an added file must stop the import: %v", err)
	}
	// Equal totals pass.
	if _, _, err = buildWith(t, input(srv), storageSnapshot{uploaded: 1, total: 3, rows: []fileRow{row}}, []portable.Table{files}); err != nil {
		t.Fatalf("equal totals refused: %v", err)
	}
	// storage.files missing from the dump fails closed.
	if _, _, err = buildWith(t, input(srv), storageSnapshot{uploaded: 1, total: 3, rows: []fileRow{row}}, nil); err == nil || !strings.Contains(err.Error(), "source storage changed") {
		t.Fatalf("a dump without storage.files must fail closed: %v", err)
	}
	// A row with no size cannot be checked.
	if _, _, err = buildWith(t, input(srv), storageSnapshot{uploaded: 1, total: 3, rows: []fileRow{{row.id, nil}}}, []portable.Table{files}); err == nil || !strings.Contains(err.Error(), "has no size") {
		t.Fatalf("a row with no size must be refused: %v", err)
	}
	_, _, err = buildWith(t, input(srv), storageSnapshot{uploaded: maxFiles + 1, total: maxFiles + 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "more than 100000 files") {
		t.Fatalf("too many files must stop before any download: %v", err)
	}
}

func TestObjectLongerThanItsRowIsCutAndRefused(t *testing.T) {
	const id = "eeeeeeee-0000-0000-0000-000000000000"
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chunk := make([]byte, 64<<10)
		for sent.Load() < 16<<20 {
			n, err := w.Write(chunk)
			sent.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	_, _, err := build(t, input(srv), 1, []fileRow{{id, i64(10)}})
	if err == nil || !strings.Contains(err.Error(), "size differs") {
		t.Fatalf("an object longer than storage.files.size must be refused: %v", err)
	}
	if got := sent.Load(); got >= 8<<20 {
		t.Fatalf("the endpoint streamed %d bytes for a 10 byte row: the read is not bounded", got)
	}
}

type fakeRow struct {
	v   any
	err error
}

func (r fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	switch d := dest[0].(type) {
	case *bool:
		*d = r.v.(bool)
	case *int64:
		*d = r.v.(int64)
	}
	return nil
}

type fakeDB map[string]any // SQL fragment -> value

func (f fakeDB) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	for frag, v := range f {
		if strings.Contains(sql, frag) {
			return fakeRow{v: v}
		}
	}
	return fakeRow{err: errors.New("unexpected query")}
}

func TestAuthExtrasCountsWhatIsNotCarried(t *testing.T) {
	none := fakeDB{"column_name='active_mfa_type'": false, "'auth.user_providers'": false, "'auth.user_security_keys'": false}
	got, err := authExtrasIn(context.Background(), none)
	if err != nil || got.nonzero() {
		t.Fatalf("older hasura-auth without those tables: %+v %v", got, err)
	}
	all := fakeDB{"column_name='active_mfa_type'": true, "active_mfa_type IS NOT NULL": int64(2), "'auth.user_providers'": true, "FROM auth.user_providers": int64(5), "'auth.user_security_keys'": true, "FROM auth.user_security_keys": int64(1)}
	got, err = authExtrasIn(context.Background(), all)
	if err != nil || got.mfa != 2 || got.providers != 5 || got.keys != 1 || !got.nonzero() {
		t.Fatalf("counts: %+v %v", got, err)
	}
	if _, err = authExtrasIn(context.Background(), fakeDB{}); err == nil {
		t.Fatal("a failed lookup must be an error, not zero")
	}
}
