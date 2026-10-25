package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

// etagServer serves bodies[i] with ETag etags[i] on the i-th unconditional
// request and records every If-None-Match it sees. A request whose
// If-None-Match equals the current ETag gets a 304.
type etagServer struct {
	*httptest.Server
	mu     sync.Mutex
	cur    int
	bodies [][]byte
	etags  []string
	inm    []string
}

func newETagServer(t *testing.T, bodies [][]byte, etags []string) *etagServer {
	t.Helper()
	s := &etagServer{bodies: bodies, etags: etags}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.inm = append(s.inm, r.Header.Get("If-None-Match"))
		w.Header().Set("ETag", s.etags[s.cur])
		if r.Header.Get("If-None-Match") == s.etags[s.cur] {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(s.bodies[s.cur])
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *etagServer) advance() { s.mu.Lock(); s.cur++; s.mu.Unlock() }

func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestFetchETag304(t *testing.T) {
	if got, want := newClient().Timeout, httptimeout.Plugin.Timeout; got != want || got == 0 {
		t.Fatalf("client timeout %v, want httptimeout.Plugin's %v", got, want)
	}
	srv := newETagServer(t, [][]byte{sampleCatalog(1)}, []string{`"e1"`})
	dir := t.TempDir()

	first, err := Fetch(context.Background(), srv.URL, dir)
	if err != nil || first.Source != SourceNetwork {
		t.Fatalf("first fetch: %v %v", first.Source, err)
	}
	second, err := Fetch(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if got := srv.inm; len(got) != 2 || got[0] != "" || got[1] != `"e1"` {
		t.Fatalf("If-None-Match sequence = %q, want [\"\" \"e1\"]", got)
	}
	if second.Source != SourceNetwork || len(second.Catalog.Plugins) != len(first.Catalog.Plugins) {
		t.Fatalf("304 should reuse the cached catalog, got %v with %d plugins", second.Source, len(second.Catalog.Plugins))
	}
	body, _ := os.ReadFile(filepath.Join(dir, bodyFile))
	if string(body) != string(sampleCatalog(1)) {
		t.Fatal("cached body changed on a 304")
	}
}

func TestFetchNewETagReplaces(t *testing.T) {
	srv := newETagServer(t, [][]byte{sampleCatalog(1), sampleCatalog(2)}, []string{`"e1"`, `"e2"`})
	dir := t.TempDir()
	if _, err := Fetch(context.Background(), srv.URL, dir); err != nil {
		t.Fatal(err)
	}
	srv.advance()
	res, err := Fetch(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatal(err)
	}
	if srv.inm[1] != `"e1"` {
		t.Fatalf("second request If-None-Match = %q", srv.inm[1])
	}
	e, _ := res.Catalog.Lookup("traefik")
	if !strings.HasSuffix(e.Description, "2") {
		t.Fatalf("fetched description %q, want the new body", e.Description)
	}
	body, _ := os.ReadFile(filepath.Join(dir, bodyFile))
	etag, _ := os.ReadFile(filepath.Join(dir, etagFile))
	if string(body) != string(sampleCatalog(2)) || strings.TrimSpace(string(etag)) != `"e2"` {
		t.Fatalf("cache not replaced: etag %q", etag)
	}
}

func TestFetchOfflineUsesCache(t *testing.T) {
	srv := newETagServer(t, [][]byte{sampleCatalog(1)}, []string{`"e1"`})
	dir := t.TempDir()
	if _, err := Fetch(context.Background(), srv.URL, dir); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local)
	if err := os.Chtimes(filepath.Join(dir, bodyFile), when, when); err != nil {
		t.Fatal(err)
	}
	srv.Close() // unreachable from here on

	res, err := Fetch(context.Background(), srv.URL, dir)
	if err != nil {
		t.Fatalf("offline fetch with a cache should succeed: %v", err)
	}
	if res.Source != SourceCache || res.Note() != "using cached catalog from 2026-10-01" {
		t.Fatalf("source %v note %q", res.Source, res.Note())
	}
	if _, ok := res.Catalog.FindByCommand("traefik"); !ok {
		t.Fatal("cached catalog lost its entries")
	}
	if n := (Result{Source: SourceNetwork}).Note(); n != "" {
		t.Fatalf("network result has note %q", n)
	}
}

func TestFetchNoCacheE115(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	for name, u := range map[string]string{"unreachable": url, "plain http to a remote host": "http://plugins.example.invalid/catalog.json"} {
		if _, err := Fetch(context.Background(), u, t.TempDir()); codeOf(err) != "E115" {
			t.Errorf("%s: got %v, want E115", name, err)
		}
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer bad.Close()
	if _, err := Fetch(context.Background(), bad.URL, t.TempDir()); codeOf(err) != "E115" {
		t.Errorf("502 with no cache: got %v, want E115", err)
	}
	if _, err := LoadCached(t.TempDir()); codeOf(err) != "E115" {
		t.Errorf("LoadCached with no cache: got %v, want E115", err)
	}
}

func TestFetchInvalidBodyKeepsCache(t *testing.T) {
	good := newETagServer(t, [][]byte{sampleCatalog(1)}, []string{`"e1"`})
	dir := t.TempDir()
	if _, err := Fetch(context.Background(), good.URL, dir); err != nil {
		t.Fatal(err)
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"schema_version": 9}`)) }))
	defer broken.Close()
	if _, err := Fetch(context.Background(), broken.URL, dir); codeOf(err) != "E116" {
		t.Fatalf("invalid body: got %v, want E116", err)
	}
	body, _ := os.ReadFile(filepath.Join(dir, bodyFile))
	if string(body) != string(sampleCatalog(1)) {
		t.Fatal("an invalid body replaced the cache")
	}
}

func TestCommandHintReadsCacheOnly(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_CACHE", dir)
	if _, ok := CommandHint("traefik"); ok {
		t.Fatal("hint without a cache")
	}
	srv := newETagServer(t, [][]byte{sampleCatalog(1)}, []string{`"e1"`})
	if _, err := Fetch(context.Background(), srv.URL, dir); err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if slug, ok := CommandHint("traefik"); !ok || slug != "traefik" {
		t.Fatalf("CommandHint(traefik) = %q %v", slug, ok)
	}
	if _, ok := CommandHint("ghost"); ok {
		t.Fatal("non-installable plugin offered as a hint")
	}
	if _, ok := CommandHint("nope"); ok {
		t.Fatal("unknown command offered as a hint")
	}
}

// TestWriteCacheConcurrentKeepsPair proves racing writers and readers never see
// one response's body with another's ETag (D-0875). Without the cache lock a
// later 304 would keep the wrong body.
func TestWriteCacheConcurrentKeepsPair(t *testing.T) {
	dir := t.TempDir()
	const writers, rounds = 8, 60
	pairOK := func(c *cached) bool {
		if c.etag == "" {
			return true // a body with no ETag is a safe crash window
		}
		for n := 0; n < writers; n++ {
			if c.etag == fmt.Sprintf(`"e%d"`, n) {
				return strings.Contains(string(c.body), fmt.Sprintf("Reverse proxy %d", n))
			}
		}
		return false
	}
	var wg sync.WaitGroup
	var bad atomic.Int32
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				writeCache(dir, sampleCatalog(w), fmt.Sprintf(`"e%d"`, w))
				if c, _, ok := readCache(dir); ok && !pairOK(c) {
					bad.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	c, _, ok := readCache(dir)
	if !ok || !pairOK(c) || bad.Load() != 0 {
		t.Fatalf("body and ETag diverged: final ok=%v pair=%v, %d bad reads", ok, ok && pairOK(c), bad.Load())
	}
}
