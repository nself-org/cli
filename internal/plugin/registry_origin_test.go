package plugin

// registry_origin_test.go — D-0849: the registry cache is scoped to the
// registry it came from, and the exclusive client never leaves the configured
// registry.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func originServer(t *testing.T, version string, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits != nil {
			atomic.AddInt32(hits, 1)
		}
		_, _ = w.Write([]byte(`{"plugins":[{"name":"ai","version":"` + version + `"}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func downServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(hits, 1)
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func pluginVersion(t *testing.T, reg *Registry, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(reg.Plugins) != 1 {
		t.Fatalf("plugins = %+v", reg.Plugins)
	}
	return reg.Plugins[0].Version
}

// A cache written for registry A must not answer for registry B.
func TestFetchRegistryIgnoresCacheOfAnotherRegistry(t *testing.T) {
	a, b := originServer(t, "1.0.0", nil), originServer(t, "2.0.0", nil)
	dir := t.TempDir()
	ctx := context.Background()
	reg, err := FetchRegistry(ctx, a.URL, dir)
	if v := pluginVersion(t, reg, err); v != "1.0.0" {
		t.Fatalf("registry A served %s", v)
	}
	reg, err = FetchRegistry(ctx, b.URL, dir) // within the TTL: A's cache is still fresh
	if v := pluginVersion(t, reg, err); v != "2.0.0" {
		t.Fatalf("registry B answered from A's cache: got %s, want 2.0.0", v)
	}
	reg, err = FetchRegistry(ctx, b.URL, dir) // B's own cache now serves
	if v := pluginVersion(t, reg, err); v != "2.0.0" {
		t.Fatalf("registry B's own cache: got %s", v)
	}
}

// The last-resort stale cache is scoped too: A's file cannot answer for B when
// both endpoints are down, but B's own stale file can.
func TestStaleCacheIsScopedToTheRegistry(t *testing.T) {
	var hits int32
	a := originServer(t, "1.0.0", nil)
	dead, deadFallback := downServer(t, &hits), downServer(t, &hits)
	dir := t.TempDir()
	if _, err := FetchRegistry(context.Background(), a.URL, dir); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(dir+"/"+registryCacheFile, old, old)
	c := &registryHTTPClient{primaryURL: dead.URL, fallbackURL: deadFallback.URL, cacheDir: dir, cacheTTL: time.Minute, httpClient: http.DefaultClient}
	if reg, err := c.Fetch(context.Background()); err == nil {
		t.Fatalf("a stale cache of another registry answered: %+v", reg.Plugins)
	}
	c.primaryURL = a.URL
	reg, err := c.Fetch(context.Background())
	if v := pluginVersion(t, reg, err); v != "1.0.0" {
		t.Fatalf("own registry: got %s", v)
	}
}

// The exclusive client asks only the configured registry: no fallback host and
// no stale cache, and a failure is an error.
func TestExclusiveFetchNeverFallsBack(t *testing.T) {
	var primaryHits, fallbackHits int32
	primary := downServer(t, &primaryHits)
	fallback := originServer(t, "9.9.9", &fallbackHits)
	dir := t.TempDir()
	seed := &registryHTTPClient{primaryURL: fallback.URL, fallbackURL: fallback.URL, cacheDir: dir, cacheTTL: time.Minute, httpClient: http.DefaultClient}
	if _, err := seed.Fetch(context.Background()); err != nil { // a cache file for some other origin
		t.Fatal(err)
	}
	atomic.StoreInt32(&fallbackHits, 0)
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(dir+"/"+registryCacheFile, old, old)
	c := &registryHTTPClient{primaryURL: primary.URL, fallbackURL: fallback.URL, cacheDir: dir, cacheTTL: time.Minute, httpClient: http.DefaultClient, exclusive: true}
	if reg, err := c.Fetch(context.Background()); err == nil {
		t.Fatalf("exclusive fetch fell back: %+v", reg.Plugins)
	}
	if got := atomic.LoadInt32(&fallbackHits); got != 0 {
		t.Fatalf("fallback host asked %d times", got)
	}
	if atomic.LoadInt32(&primaryHits) == 0 {
		t.Fatal("the configured registry was never asked")
	}
}

// An exclusive fetch refuses a cache with no recorded origin, accepts its own
// cache, and the pin it writes is readable by the normal client (the installer).
func TestExclusiveFetchCacheOrigin(t *testing.T) {
	var hits int32
	a := originServer(t, "1.0.0", &hits)
	dir := t.TempDir()
	t.Setenv("NSELF_PLUGIN_REGISTRY", a.URL)
	if err := WriteRegistryCache(dir, &Registry{Plugins: []PluginManifest{{Name: "ai", Version: "7.0.0"}}}); err != nil {
		t.Fatal(err)
	}
	reg, err := FetchRegistryExclusive(context.Background(), "", dir)
	if v := pluginVersion(t, reg, err); v != "1.0.0" || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("an unscoped cache answered an exclusive fetch: %s (hits %d)", v, hits)
	}
	reg, err = FetchRegistryExclusive(context.Background(), "", dir)
	if v := pluginVersion(t, reg, err); v != "1.0.0" || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("own cache not used: %s (hits %d)", v, hits)
	}
	if err := WriteRegistryCache(dir, reg); err != nil { // the installer's pin
		t.Fatal(err)
	}
	reg, err = FetchRegistry(context.Background(), "", dir)
	if v := pluginVersion(t, reg, err); v != "1.0.0" || atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("pin not readable by the normal client: %s (hits %d)", v, hits)
	}
}
