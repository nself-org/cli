package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// TestDownloadPluginPackagePrefersPlatformAsset pins which archive the
// installer asks for.
//
// A service plugin's package is source and works anywhere, so the generic
// endpoint is right. A CLI plugin's package has to contain a compiled binary
// for the machine doing the installing — a Linux build is useless on macOS — so
// it must ask for the per-platform release asset, and only fall back to the
// generic one for a release published before those existed.
//
// This distinction did not exist until the whole path was run end to end: the
// release pipeline shipped source only, so every extracted command installed
// "successfully" and then did not exist.
func TestDownloadPluginPackagePrefersPlatformAsset(t *testing.T) {
	platform, err := PlatformArch()
	if err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}

	tests := []struct {
		name       string
		binaryName string
		wantPath   string
		wantKind   string
	}{
		{
			name:       "cli plugin asks for its platform build",
			binaryName: "nself-example",
			wantPath:   "/releases/download/v1.0.0/example-1.0.0-" + platform + ".tar.gz",
			wantKind:   platform,
		},
		{
			name:       "service plugin uses the generic package",
			binaryName: "",
			wantPath:   "/plugins/example/tarball",
			wantKind:   ArtifactKindSource,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mu sync.Mutex
			var seen []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				seen = append(seen, r.URL.Path)
				mu.Unlock()
				// Any 200 with a body is enough; this test is about which URL
				// is requested, not about the archive's contents.
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("not-a-real-tarball"))
			}))
			defer srv.Close()

			t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)

			path, kind, err := downloadPluginPackage(context.Background(), "example", "1.0.0", srv.URL, tt.binaryName)
			if err != nil {
				t.Fatalf("download: %v", err)
			}
			_ = path

			if kind != tt.wantKind {
				t.Errorf("artifact kind = %q, want %q", kind, tt.wantKind)
			}

			mu.Lock()
			defer mu.Unlock()
			if len(seen) == 0 {
				t.Fatal("no request was made")
			}
			if seen[0] != tt.wantPath {
				t.Errorf("first request was %q, want %q (requests: %v)", seen[0], tt.wantPath, seen)
			}
		})
	}
}

// TestDownloadPluginPackageFallsBackWhenNoPlatformAsset covers a CLI plugin
// whose release predates per-platform assets: the platform URL 404s and the
// generic package is tried, rather than the install failing outright.
func TestDownloadPluginPackageFallsBackWhenNoPlatformAsset(t *testing.T) {
	if _, err := PlatformArch(); err != nil {
		t.Skipf("unsupported test platform: %v", err)
	}

	var mu sync.Mutex
	var seen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.URL.Path)
		mu.Unlock()
		if strings.Contains(r.URL.Path, "/releases/download/") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("source-tarball"))
	}))
	defer srv.Close()

	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)

	_, kind, err := downloadPluginPackage(context.Background(), "example", "1.0.0", srv.URL, "nself-example")
	if err != nil {
		t.Fatalf("expected fallback to succeed, got: %v", err)
	}
	if kind != ArtifactKindSource {
		t.Errorf("artifact kind = %q, want %q (fell back to the generic package)", kind, ArtifactKindSource)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) < 2 {
		t.Fatalf("expected a platform attempt then a fallback, saw %v", seen)
	}
	if !strings.Contains(seen[0], "/releases/download/") {
		t.Errorf("first attempt was not the platform asset: %v", seen)
	}
	if seen[len(seen)-1] != "/plugins/example/tarball" {
		t.Errorf("did not fall back to the generic package: %v", seen)
	}
}
