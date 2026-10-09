package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/plugin/count"
	"github.com/spf13/cobra"
)

func captureVocabularyOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()
	data := make(chan []byte, 1)
	errors := make(chan error, 1)
	go func() {
		b, readErr := io.ReadAll(r)
		data <- b
		errors <- readErr
	}()
	fn()
	_ = w.Close()
	b := <-data
	if readErr := <-errors; readErr != nil {
		t.Fatal(readErr)
	}
	_ = r.Close()
	return string(b)
}

func TestTierListColumns(t *testing.T) {
	registryFixture(t)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("json", false, "")
	got := captureVocabularyOutput(t, func() {
		if err := runPluginListAvailable(cmd, ""); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(got, "Licensed") || !strings.Contains(got, "Free") || strings.Contains(got, "pro tier") {
		t.Fatalf("columns: %s", got)
	}
}

func TestTierListJSONLicense(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		registryFixture(t)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.Flags().Bool("json", true, "")
		var b bytes.Buffer
		cmd.SetOut(&b)
		if err := runPluginListAvailable(cmd, ""); err != nil {
			t.Fatal(err)
		}
		got := b.String()
		if compat.V15() && (!strings.Contains(got, `"license":"licensed"`) || strings.Contains(got, `"tier"`)) {
			t.Fatalf("v1.5 JSON: %s", got)
		}
		if !compat.V15() && (!strings.Contains(got, `"tier":"pro"`) || strings.Contains(got, `"license"`) || strings.Contains(got, `"Tier"`)) {
			t.Fatalf("v1.4 JSON: %s", got)
		}
		var rows []map[string]any
		if err := json.Unmarshal([]byte(got), &rows); err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			for _, key := range []string{"name", "version", "category", "tier_pair", "is_default"} {
				if _, ok := row[key]; !ok {
					t.Fatalf("missing snake_case key %q: %s", key, got)
				}
			}
		}
	})
}

func registryFixture(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[{"name":"example","tier":"pro","version":"1.0.0"},{"name":"example","tier":"free","version":"1.0.0"}]}`))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
}

func TestPluginSearchLicensedEqualsPro(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"plugins":[{"name":"example","tier":"pro","description":"example"},{"name":"free-only","tier":"free","description":"example"}]}`))
		}))
		defer srv.Close()
		t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
		t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
		run := func(flag string) string {
			cmd := &cobra.Command{}
			cmd.Flags().Bool("free", false, "")
			cmd.Flags().Bool("pro", flag == "pro", "")
			cmd.Flags().Bool("licensed", flag == "licensed", "")
			cmd.Flags().Bool("json", true, "")
			return captureVocabularyOutput(t, func() {
				if err := runPluginSearch(cmd, []string{"example"}); err != nil {
					t.Error(err)
				}
			})
		}
		if a, b := run("licensed"), run("pro"); a != b || !strings.Contains(a, `"example"`) || strings.Contains(a, `"free-only"`) {
			t.Fatalf("licensed and pro result differ: %q / %q", a, b)
		} else if compat.V15() && (!strings.Contains(a, `"license":"licensed"`) || strings.Contains(a, `"tier"`)) {
			t.Fatalf("v1.5 search JSON: %s", a)
		} else if !compat.V15() && (!strings.Contains(a, `"tier":"pro"`) || strings.Contains(a, `"license"`)) {
			t.Fatalf("v1.4 search JSON: %s", a)
		}
	})
}

func TestPluginSearchProDeprecated(t *testing.T) {
	if !pluginSearchCmd.Flags().Lookup("pro").Hidden {
		t.Fatal("deprecated alias is visible")
	}
	proSearchWarning = sync.Once{}
	var b bytes.Buffer
	cmd := &cobra.Command{}
	cmd.Flags().Bool("free", false, "")
	cmd.Flags().Bool("pro", true, "")
	cmd.Flags().Bool("licensed", false, "")
	cmd.Flags().Bool("json", true, "")
	cmd.SetErr(&b)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"plugins":[{"name":"example","tier":"pro"}]}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	_ = runPluginSearch(cmd, []string{"example"})
	_ = runPluginSearch(cmd, []string{"example"})
	if strings.Count(b.String(), "--pro is deprecated") != 1 || !strings.Contains(b.String(), "v1.6.0") {
		t.Fatal(b.String())
	}
}

func TestUpsellVocabulary(t *testing.T) {
	got := upsellUnlockLine()
	if !strings.Contains(got, "plugins with nSelf+") || retiredPluginTerm.MatchString(got) {
		t.Fatal(got)
	}
}

var retiredPluginTerm = regexp.MustCompile(`(?i)\b(plugins pro|pro tiers?|pro plugins?)\b`)

func TestBundleListVocabulary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got := captureVocabularyOutput(t, func() {
		if err := runBundleList(nil, nil); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(got, "Plugin Bundles") || retiredPluginTerm.MatchString(got) {
		t.Fatal(got)
	}
}

func TestHelpTopicCountsFromEmbedded(t *testing.T) {
	a, err := count.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := captureVocabularyOutput(t, func() {
		if err := printHelpTopic("plugins"); err != nil {
			t.Error(err)
		}
	})
	if !strings.Contains(got, fmt.Sprintf("Free plugins (%d)", a.Free.Installable)) ||
		!strings.Contains(got, fmt.Sprintf("Licensed plugins (%d)", a.Pro.Installable)) ||
		strings.Contains(helpTopics["plugins"].Body, "%d") {
		t.Fatalf("help counts: %s", got)
	}
	_, path, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test source")
	}
	source, err := os.ReadFile(strings.Replace(path, "plugin_search_test.go", "help_topics.go", 1))
	if err != nil {
		t.Fatal(err)
	}
	if regexp.MustCompile(`(?:Free|Licensed) plugins \([0-9]+\)`).Match(source) {
		t.Fatal("help topic contains a hardcoded plugin count")
	}
}

func TestPluginInstallTierFlag(t *testing.T) {
	cmd := pluginInstallCmd
	if err := cmd.Flags().Set("preview", "true"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Flags().Set("preview", "false") })
	for _, tc := range []struct{ flag, wire string }{{"licensed", plugin.WireTierPro}, {"pro", plugin.WireTierPro}} {
		t.Run(tc.flag, func(t *testing.T) {
			t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
			if err := cmd.Flags().Set("tier", tc.flag); err != nil {
				t.Fatal(err)
			}
			err := runPluginInstall(cmd, []string{"https://example.com/plugin.tar.gz"})
			if err == nil || !strings.Contains(err.Error(), "not supported for third-party URL installs") {
				t.Fatalf("flag rejected before preview gate: %v", err)
			}
			if got := os.Getenv("NSELF_PLUGIN_INSTALL_TIER"); got != tc.wire {
				t.Fatalf("install tier = %q, want %q", got, tc.wire)
			}
		})
	}
	t.Run("bogus", func(t *testing.T) {
		t.Setenv("NSELF_PLUGIN_INSTALL_TIER", "")
		if err := cmd.Flags().Set("tier", "bogus"); err != nil {
			t.Fatal(err)
		}
		err := runPluginInstall(cmd, []string{"https://example.com/plugin.tar.gz"})
		if err == nil || !strings.Contains(err.Error(), `must be "free" or "licensed"`) {
			t.Fatalf("bogus tier error: %v", err)
		}
	})
	_ = cmd.Flags().Set("tier", "")
}

func TestMarketplaceJSONLicenseProjection(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		rows := []marketplacePlugin{{Name: "example", Tier: plugin.WireTierPro, Version: "1.0"}}
		for _, single := range []bool{false, true} {
			got := captureVocabularyOutput(t, func() {
				if err := printMarketplaceJSON(rows, single); err != nil {
					t.Error(err)
				}
			})
			var data any
			if err := json.Unmarshal([]byte(got), &data); err != nil {
				t.Fatal(err)
			}
			item := data
			if !single {
				item = data.([]any)[0]
			}
			m := item.(map[string]any)
			if m["name"] != "example" || m["version"] != "1.0" {
				t.Fatalf("lost API fields: %s", got)
			}
			if compat.V15() && (m["license"] != "licensed" || m["tier"] != nil) {
				t.Fatalf("v1.5 JSON: %s", got)
			}
			if !compat.V15() && (m["tier"] != "pro" || m["license"] != nil) {
				t.Fatalf("v1.4 JSON: %s", got)
			}
		}
	})
}
