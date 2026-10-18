package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// downgradeFixture serves a registry whose only plugin, "oldplug", is at
// 1.0.0, and counts tarball requests. Signatures prove origin, not recency
// (D-0843): a registry that serves an older signed release as the latest must
// not move an installed plugin backwards.
func downgradeFixture(t *testing.T) *int32 {
	t.Helper()
	const body = `{"plugins":[{"name":"oldplug","version":"1.0.0","description":"fixture","category":"util","tier":"free","license":"MIT"}]}`
	var downloads int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/tarball") {
			atomic.AddInt32(&downloads, 1)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_ALLOW_DOWNGRADE", "")
	return &downloads
}

// installedAt writes an installed oldplug at version into dir (name or name.prev).
func installedAt(t *testing.T, pluginDir, dir, version string) {
	t.Helper()
	d := filepath.Join(pluginDir, dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	m := `{"name":"oldplug","version":"` + version + `","description":"fixture","category":"util","license":"MIT"}`
	if err := os.WriteFile(filepath.Join(d, "plugin.json"), []byte(m), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestInstallRefusesDowngradeInV15 pins the D-0843 rule: in v1.5 an install
// whose registry version is below the installed one is refused before any
// download, whether the installed copy sits in place or parked as .prev by
// Update; --allow-downgrade (NSELF_PLUGIN_ALLOW_DOWNGRADE=1) opts back in; v1.4
// still only warns; an upgrade or an equal version is never refused.
func TestInstallRefusesDowngradeInV15(t *testing.T) {
	cases := []struct {
		name      string
		v15       bool
		allow     bool
		dir       string
		installed string
		refused   bool
	}{
		{"v1.5 installed copy", true, false, "oldplug", "2.0.0", true},
		{"v1.5 update backup", true, false, "oldplug.prev", "2.0.0", true},
		{"v1.5 allow-downgrade", true, true, "oldplug", "2.0.0", false},
		{"v1.4 warns only", false, false, "oldplug", "2.0.0", false},
		{"v1.5 upgrade", true, false, "oldplug", "0.9.0", false},
		{"v1.5 same version", true, false, "oldplug", "1.0.0", false},
		{"v1.5 fresh install", true, false, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			downloads := downgradeFixture(t)
			if tc.v15 {
				t.Setenv("NSELF_V15", "1")
			} else {
				t.Setenv("NSELF_V15", "")
			}
			if tc.allow {
				t.Setenv("NSELF_PLUGIN_ALLOW_DOWNGRADE", "1")
			}
			pluginDir := t.TempDir()
			if tc.dir != "" {
				installedAt(t, pluginDir, tc.dir, tc.installed)
			}
			err := installLocked(context.Background(), &config.Config{ProjectName: "t"}, "oldplug", pluginDir)
			if tc.refused {
				if err == nil || !strings.Contains(err.Error(), "downgrade") || !strings.Contains(err.Error(), "--allow-downgrade") {
					t.Fatalf("want a downgrade refusal naming --allow-downgrade, got %v", err)
				}
				if n := atomic.LoadInt32(downloads); n != 0 {
					t.Fatalf("download hit %d times before the refusal", n)
				}
				return
			}
			if err != nil && strings.Contains(err.Error(), "downgrade") {
				t.Fatalf("unexpected downgrade refusal: %v", err)
			}
			if n := atomic.LoadInt32(downloads); n == 0 {
				t.Fatalf("never reached the download step (err=%v)", err)
			}
		})
	}
}
