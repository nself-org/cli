package bundle

// Purpose: D-0546 guards for the bundles.json cache path: a cached catalog with
// an invalid member name is refused, and a fetched catalog that fails validation
// falls back to the validated cache without overwriting it.
// Constraints: httptest servers and temp dirs only; package state is restored
// after each test.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	badCatalogJSON  = `{"schema_version":"2.0.0","bundles":{"evil":{"display":"E","tier":"free","plugins":["ok","../x"]}}}`
	goodCatalogJSON = `{"schema_version":"2.0.0","bundles":{"cached-good":{"display":"CG","tier":"free","plugins":["alpha","beta"]}}}`
)

// isolateBundleState clears the loaded catalog for the test and restores it after.
func isolateBundleState(t *testing.T) {
	t.Helper()
	prev := currentBundles()
	setState(nil)
	t.Cleanup(func() { setState(prev) })
}

// writeCacheFile writes a cachedBundlesDoc holding catalogJSON and returns its path
// and raw bytes.
func writeCacheFile(t *testing.T, catalogJSON string) (string, []byte) {
	t.Helper()
	var doc bundlesDoc
	if err := json.Unmarshal([]byte(catalogJSON), &doc); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(cachedBundlesDoc{FetchedAt: time.Now().Unix(), Doc: doc}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := t.TempDir() + "/bundles-cache.json"
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data
}

// serveBody starts a server answering 200 with body; deadServer returns the URL of a
// closed one (connection refused).
func serveBody(t *testing.T, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func deadServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

// captureStderr runs fn and returns what it wrote to os.Stderr.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	fn()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	return string(out)
}

// TestLoad_FetchFailure_BadCacheRefused: fetch fails and the cache lists "../x":
// Load must error and must not install the catalog.
func TestLoad_FetchFailure_BadCacheRefused(t *testing.T) {
	isolateBundleState(t)
	cachePath, _ := writeCacheFile(t, badCatalogJSON)
	t.Setenv("NSELF_BUNDLES_CACHE_PATH", cachePath)
	t.Setenv("NSELF_BUNDLES_URL", deadServer(t))

	err := Load(context.Background())
	if err == nil {
		t.Fatal("Load accepted a cached catalog with an invalid member name")
	}
	if !strings.Contains(err.Error(), "cached bundles.json") {
		t.Errorf("error %q does not name the cache", err)
	}
	if _, ok := Get("evil"); ok {
		t.Error("a refused cached catalog was installed")
	}
}

func TestLoad_FetchedInvalid(t *testing.T) {
	tests := []struct {
		name      string
		cache     string // "" = no cache file
		wantErr   string // "" = Load must succeed
		wantSlug  string // slug that must be loaded on success
		wantNoGet string // slug that must NOT be loaded
	}{
		{name: "good cache is used", cache: goodCatalogJSON, wantSlug: "cached-good", wantNoGet: "evil"},
		{name: "no cache returns the validation error", wantErr: "validating bundles.json", wantNoGet: "evil"},
		{name: "bad cache returns an error", cache: badCatalogJSON, wantErr: "validating bundles.json", wantNoGet: "evil"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateBundleState(t)
			cachePath := t.TempDir() + "/missing-cache.json"
			var before []byte
			if tc.cache != "" {
				cachePath, before = writeCacheFile(t, tc.cache)
			}
			t.Setenv("NSELF_BUNDLES_CACHE_PATH", cachePath)
			t.Setenv("NSELF_BUNDLES_URL", serveBody(t, badCatalogJSON))

			var err error
			stderr := captureStderr(t, func() { err = Load(context.Background()) })

			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Load error = %v, want it to contain %q", err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("Load = %v, want the validated cache to be used", err)
				}
				if _, ok := Get(tc.wantSlug); !ok {
					t.Errorf("bundle %q from the cache is not loaded", tc.wantSlug)
				}
				if !strings.Contains(stderr, "failed validation") {
					t.Errorf("stderr %q does not name the validation failure", stderr)
				}
			}
			if _, ok := Get(tc.wantNoGet); ok {
				t.Errorf("bundle %q from the bad payload was installed", tc.wantNoGet)
			}
			after, rerr := os.ReadFile(cachePath)
			if tc.cache == "" {
				if rerr == nil {
					t.Errorf("Load wrote the bad payload to a new cache file: %s", after)
				}
			} else if string(after) != string(before) {
				t.Errorf("cache file was overwritten:\nbefore: %s\nafter:  %s", before, after)
			}
		})
	}
}
