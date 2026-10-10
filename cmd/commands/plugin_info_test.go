package commands

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
)

// infoFixture serves notify as a free/licensed tier pair, points the plugin
// directory at a temp dir and, when installedTier is not empty, installs
// notify there as that tier. It returns nothing; the env carries the setup.
func infoFixture(t *testing.T, installedTier string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[` +
			`{"name":"notify","tier":"free","tier_pair":true,"version":"1.0.0","description":"free notify"},` +
			`{"name":"notify","tier":"pro","tier_pair":true,"version":"1.1.0","description":"licensed notify","bundles":["claw"]}]}`))
	}))
	t.Cleanup(srv.Close)
	pluginDir := t.TempDir()
	if installedTier != "" {
		dir := filepath.Join(pluginDir, "notify")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body := `{"name":"notify","version":"1.0.0","description":"d","tier":"` + installedTier + `"}`
		if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	t.Setenv("NSELF_PLUGIN_DIR", pluginDir)
	t.Setenv("NSELF_PLUGIN_LICENSE_KEY", "")
	t.Setenv("HOME", t.TempDir())
}

func runInfo(t *testing.T, asJSON bool) string {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("open", false, "")
	cmd.Flags().Bool("json", asJSON, "")
	return captureVocabularyOutput(t, func() {
		if err := runPluginInfo(cmd, []string{"notify"}); err != nil {
			t.Error(err)
		}
	})
}

// TestPluginInfoJSONTier: `plugin info --json` reports tier and tier_reason;
// an installed plugin reports the tier it is installed as.
func TestPluginInfoJSONTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "free")
		var got struct {
			Name       string `json:"name"`
			Version    string `json:"version"`
			Tier       string `json:"tier"`
			TierReason string `json:"tier_reason"`
		}
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatalf("not JSON: %v", err)
		}
		if got.Name != "notify" || got.Tier != "free" || got.TierReason != "installed" {
			t.Fatalf("installed free: %+v", got)
		}
		if compat.V15() && got.Version != "1.0.0" {
			t.Fatalf("v1.5 describes the installed tier's entry, got version %s", got.Version)
		}

		infoFixture(t, "pro")
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatal(err)
		}
		if got.Tier != "licensed" || got.TierReason != "installed" {
			t.Fatalf("installed pro: %+v", got)
		}
	})
}

// TestPluginInfoNotInstalled: with nothing installed, v1.5 reports what an
// install would pick (free here: no licence) and why; v1.4 keeps the first
// registry match and reports no reason.
func TestPluginInfoNotInstalled(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "")
		var got struct {
			Tier       string `json:"tier"`
			TierReason string `json:"tier_reason"`
		}
		if err := json.Unmarshal([]byte(runInfo(t, true)), &got); err != nil {
			t.Fatal(err)
		}
		wantReason := ""
		if compat.V15() {
			wantReason = "default-free"
		}
		if got.Tier != "free" || got.TierReason != wantReason {
			t.Fatalf("not installed: %+v, want free/%q", got, wantReason)
		}
	})
}

// TestPluginInfoTableTier: the table shows the licence word and the reason in
// v1.5 and the registry word, unchanged, in v1.4.
func TestPluginInfoTableTier(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		infoFixture(t, "pro")
		out := runInfo(t, false)
		if compat.V15() {
			if !strings.Contains(out, "Tier reason") || !strings.Contains(out, "installed") || !strings.Contains(out, "licensed") {
				t.Fatalf("v1.5 table: %s", out)
			}
		} else if strings.Contains(out, "Tier reason") {
			t.Fatalf("v1.4 table must not change: %s", out)
		}
	})
}

// TestPluginUpdateTierFlag: `plugin update` takes --tier, rejects a bad value
// and a missing plugin name before touching anything.
func TestPluginUpdateTierFlag(t *testing.T) {
	if pluginUpdateCmd.Flags().Lookup("tier") == nil {
		t.Fatal("plugin update must have a --tier flag")
	}
	run := func(tier string, args ...string) error {
		cmd := &cobra.Command{}
		cmd.Flags().String("tier", tier, "")
		return runPluginUpdateTier(cmd, args)
	}
	if err := run("gold", "notify"); err == nil || !strings.Contains(err.Error(), "invalid --tier") {
		t.Fatalf("bad tier: %v", err)
	}
	if err := run("licensed"); err == nil || !strings.Contains(err.Error(), "needs a plugin name") {
		t.Fatalf("no name: %v", err)
	}
}
