package supabase

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

const fixtureKey = "synthetic-service-key-for-tests"

// storageServer is a tiny storage-api: buckets avatars (a.png, dir/b.png) and docs (empty).
func storageServer(t *testing.T, seen *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fixtureKey || r.Header.Get("apikey") != fixtureKey {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		*seen = append(*seen, r.Method+" "+r.URL.EscapedPath())
		switch {
		case r.URL.Path == "/storage/v1/bucket":
			_, _ = w.Write([]byte(`[{"id":"avatars"},{"id":"docs"}]`))
		case r.URL.Path == "/storage/v1/object/list/avatars":
			var body struct{ Prefix string }
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Prefix == "" {
				_, _ = w.Write([]byte(`[{"name":"a.png","id":"1"},{"name":"dir","id":null}]`))
			} else {
				_, _ = w.Write([]byte(`[{"name":"b.png","id":"2"}]`))
			}
		case r.URL.Path == "/storage/v1/object/list/docs":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/storage/v1/object/avatars/a.png":
			_, _ = io.WriteString(w, "AAA")
		case r.URL.Path == "/storage/v1/object/avatars/dir/b.png":
			_, _ = io.WriteString(w, "BBBB")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAPIListsAndStreamsObjects(t *testing.T) {
	var seen []string
	srv := storageServer(t, &seen)
	a, err := newAPI(srv.URL, fixtureKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	buckets, err := a.buckets(ctx)
	if err != nil || strings.Join(buckets, ",") != "avatars,docs" {
		t.Fatalf("buckets %v %v", buckets, err)
	}
	keys, err := a.keys(ctx, "avatars")
	if err != nil || strings.Join(keys, ",") != "a.png,dir/b.png" {
		t.Fatalf("keys %v %v", keys, err)
	}
	body, err := a.get(ctx, "avatars", "dir/b.png")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(body)
	_ = body.Close()
	if string(got) != "BBBB" {
		t.Fatalf("object bytes %q", got)
	}
	if _, err = a.get(ctx, "avatars", "missing.png"); err == nil || strings.Contains(err.Error(), srv.URL) || strings.Contains(err.Error(), fixtureKey) {
		t.Fatalf("error must be fixed text: %v", err)
	}
}

func TestNewAPIRefusesUnsafeURLs(t *testing.T) {
	for _, raw := range []string{"http://example.com", "ftp://127.0.0.1", "https://user:pw@example.com", "https://example.com/path", "https://example.com?x=1", "", "://bad"} {
		if _, err := newAPI(raw, fixtureKey); err == nil {
			t.Errorf("%q accepted", raw)
		}
	}
	for _, raw := range []string{"https://abc.supabase.co", "http://127.0.0.1:54321", "http://localhost:8000", "http://[::1]:9"} {
		if _, err := newAPI(raw, fixtureKey); err != nil {
			t.Errorf("%q refused: %v", raw, err)
		}
	}
}

func TestAPINeverFollowsRedirects(t *testing.T) {
	hit := false
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer srv.Close()
	a, _ := newAPI(srv.URL, fixtureKey)
	if _, err := a.buckets(context.Background()); err == nil || hit {
		t.Fatalf("redirect followed (hit=%v err=%v)", hit, err)
	}
}

func finishWith(t *testing.T, catalog map[string]int64) (portable.Manifest, error) {
	t.Helper()
	var seen []string
	srv := storageServer(t, &seen)
	a, _ := newAPI(srv.URL, fixtureKey)
	r := &run{api: a, catalog: catalog}
	for _, n := range catalog {
		r.total += n
	}
	w, err := portable.NewWriter(filepath.Join(t.TempDir(), "stage"))
	if err != nil {
		t.Fatal(err)
	}
	m := portable.Manifest{}
	return m, r.finish(context.Background(), w, &m)
}

func TestFinishWritesObjectsAndChecksCatalog(t *testing.T) {
	m, err := finishWith(t, map[string]int64{"avatars": 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Storage.Objects) != 2 || m.Storage.Objects[0].Bytes != 3 || len(m.Storage.Buckets) != 2 || m.Storage.Buckets[0].Bytes != 7 {
		t.Fatalf("manifest: %+v", m.Storage)
	}
}

func TestFinishRefusesListingThatDiffersFromCatalog(t *testing.T) {
	if _, err := finishWith(t, map[string]int64{"avatars": 3}); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("want catalog mismatch, got %v", err)
	}
}

func TestFinishNeedsAPIWhenObjectsExist(t *testing.T) {
	r := &run{catalog: map[string]int64{"avatars": 1}, total: 1}
	err := r.finish(context.Background(), nil, &portable.Manifest{})
	if err == nil || !strings.Contains(err.Error(), "--service-key-env") {
		t.Fatalf("got %v", err)
	}
	if err = (&run{}).finish(context.Background(), nil, &portable.Manifest{}); err != nil {
		t.Fatalf("no objects must pass: %v", err)
	}
}
