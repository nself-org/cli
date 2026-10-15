package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestDownloadTierPair sends the resolved catalogue tier over HTTP, so the
// server cannot select a licensed row for an unlicensed tier-pair install.
func TestDownloadTierPair(t *testing.T) {
	for _, tc := range []struct {
		name     string
		override string
		entitled EntitlementFunc
		key      string
		wantTier string
		wantKey  string
	}{
		{"unlicensed", "", neverEntitled, "", "free", ""},
		{"entitled", "", alwaysEntitled, "test-license", "licensed", "test-license"},
		{"explicit free", "free", alwaysEntitled, "test-license", "free", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("NSELF_PLUGIN_LICENSE_KEY", tc.key)
			for i := 1; i <= 10; i++ {
				t.Setenv(fmt.Sprintf("NSELF_LICENSE_KEY_%d", i), "")
			}
			var requests []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests = append(requests, r.URL.RequestURI())
				if r.URL.Query().Get("tier") != tc.wantTier {
					http.Error(w, "wrong tier selected", http.StatusForbidden)
					return
				}
				if got := r.Header.Get("X-License-Key"); got != tc.wantKey {
					t.Errorf("license header = %q, want %q", got, tc.wantKey)
				}
				_, _ = w.Write([]byte("selected-tier-archive"))
			}))
			defer srv.Close()
			t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
			t.Setenv("NSELF_PING_API_URL", srv.URL)

			manifest, err := ResolvePlugin(context.Background(), fixtureRegistry(), "cron", tc.override, tc.entitled)
			if err != nil {
				t.Fatal(err)
			}
			path, kind, err := downloadPluginPackageForTier(context.Background(), manifest.Name, manifest.Version, srv.URL, "", isPaidPluginManifest(manifest))
			if err != nil {
				t.Fatalf("download resolved %s tier: %v (requests: %v)", manifest.Tier, err, requests)
			}
			t.Cleanup(func() { _ = os.Remove(path) })
			if kind != ArtifactKindSource {
				t.Fatalf("artifact kind = %q, want source", kind)
			}
			body, err := os.ReadFile(path)
			if err != nil || string(body) != "selected-tier-archive" {
				t.Fatalf("downloaded archive = %q, err = %v", body, err)
			}
			if len(requests) != 1 || !strings.HasPrefix(requests[0], "/plugins/cron/") {
				t.Fatalf("expected one primary download, got %v", requests)
			}
		})
	}
}
