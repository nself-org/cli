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
	"github.com/nself-org/cli/internal/output"
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
		catalogFixture(t)
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
		// v1.5 reads the catalog (P7-PLUG-09): its "example" rows are the
		// licensed twin and the free-only plugin is not matched by --licensed.
		if a, b := run("licensed"), run("pro"); a != b || !strings.Contains(a, `"example"`) || strings.Contains(a, `"free-only"`) {
			t.Fatalf("licensed and pro result differ: %q / %q", a, b)
		} else if compat.V15() && (!strings.Contains(a, `"tier": "licensed"`) || strings.Contains(a, `"tier": "free"`)) {
			t.Fatalf("v1.5 search JSON: %s", a)
		} else if !compat.V15() && (!strings.Contains(a, `"tier":"pro"`) || strings.Contains(a, `"license"`)) {
			t.Fatalf("v1.4 search JSON: %s", a)
		}
	})
}

// catalogFixtureBody is a catalog with a tier pair (example), a free plugin in
// a bundle (chat) and a licensed-only plugin.
const catalogFixtureBody = `{"schema_version":1,"generated_from":{"plugins_ref":"a","bundles_ref":"b","generated_at":"2026-10-13T00:00:00Z"},
"plugins":[
{"slug":"chat","tier":"free","version":"1.2.1","description":"Team chat service","category":"messaging","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":["chat"],"tier_pair":false},
{"slug":"example","tier":"free","version":"1.2.1","description":"example","category":"misc","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":[],"tier_pair":true},
{"slug":"example","tier":"licensed","version":"1.2.1","description":"example","category":"misc","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":[],"tier_pair":true},
{"slug":"free-only","tier":"free","version":"1.2.1","description":"no match for e-x-a-m-p-l-e","category":"misc","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":[],"tier_pair":false}]}`

// catalogFixture serves catalogFixtureBody, points the search command at it,
// isolates the cache dir and returns the server (so a test can close it).
func catalogFixture(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"fixture"`)
		_, _ = w.Write([]byte(catalogFixtureBody))
	}))
	t.Cleanup(srv.Close)
	old := catalogURL
	catalogURL = srv.URL
	t.Cleanup(func() { catalogURL = old })
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	return srv
}

func searchCmd(json bool, stderr *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("free", false, "")
	cmd.Flags().Bool("pro", false, "")
	cmd.Flags().Bool("licensed", false, "")
	cmd.Flags().Bool("json", json, "")
	if stderr != nil {
		cmd.SetErr(stderr)
	}
	return cmd
}

func TestPluginSearchEnvelopeV15(t *testing.T) {
	compattest.Set(t, true)
	srv := catalogFixture(t)
	var doc bytes.Buffer
	old := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: &doc, Err: &bytes.Buffer{}} }
	t.Cleanup(func() { pilotWriter = old; output.ResetState() })

	if err := runPluginSearch(searchCmd(true, nil), []string{"chat"}); err != nil {
		t.Fatal(err)
	}
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Data          struct {
			Count   int `json:"count"`
			Plugins []struct {
				Slug, Tier, Description string
				Bundles                 []string
			} `json:"plugins"`
		} `json:"data"`
	}
	if err := json.Unmarshal(doc.Bytes(), &env); err != nil {
		t.Fatalf("one envelope expected: %v\n%s", err, doc.String())
	}
	if env.SchemaVersion != "1" || env.Command != "config plugins search" || env.Data.Count != 1 || len(env.Data.Plugins) != 1 {
		t.Fatalf("envelope: %s", doc.String())
	}
	if p := env.Data.Plugins[0]; p.Slug != "chat" || p.Tier != "free" || p.Description != "Team chat service" || len(p.Bundles) != 1 || p.Bundles[0] != "chat" {
		t.Fatalf("row: %+v", p)
	}
	if strings.Contains(doc.String(), `"bundles": null`) {
		t.Fatalf("bundles must be a list, never null: %s", doc.String())
	}

	// A tier pair lists both tiers, --licensed narrows, no match is an empty list.
	doc.Reset()
	if err := runPluginSearch(searchCmd(true, nil), []string{"example"}); err != nil || strings.Count(doc.String(), `"slug": "example"`) != 2 {
		t.Fatalf("tier pair: %v\n%s", err, doc.String())
	}
	doc.Reset()
	if err := runPluginSearch(searchCmd(true, nil), []string{"zzz-nothing"}); err != nil || !strings.Contains(doc.String(), `"plugins": []`) {
		t.Fatalf("empty result: %v\n%s", err, doc.String())
	}

	// Offline: the cache answers, with the stderr note; the exit stays clean.
	srv.Close()
	doc.Reset()
	var stderr bytes.Buffer
	if err := runPluginSearch(searchCmd(true, &stderr), []string{"chat"}); err != nil {
		t.Fatalf("offline search with a cache: %v", err)
	}
	if !strings.Contains(stderr.String(), "using cached catalog from ") || !strings.Contains(doc.String(), `"slug": "chat"`) {
		t.Fatalf("offline: stderr %q doc %s", stderr.String(), doc.String())
	}
}

func TestPluginSearchV14Unchanged(t *testing.T) {
	compattest.Set(t, false)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "catalog") {
			t.Errorf("v1.4 search requested the catalog: %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"plugins":[{"name":"example","tier":"pro","category":"misc","description":"an example"}]}`))
	}))
	defer srv.Close()
	t.Setenv("NSELF_PLUGIN_REGISTRY", srv.URL)
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	old := catalogURL
	catalogURL = "http://127.0.0.1:1/catalog.json" // unreachable: any use fails the test
	t.Cleanup(func() { catalogURL = old })

	got := captureVocabularyOutput(t, func() {
		if err := runPluginSearch(searchCmd(true, nil), []string{"example"}); err != nil {
			t.Error(err)
		}
	})
	want := `{"name":"example","tier":"pro","category":"misc","description":"an example"}` + "\n"
	if got != want {
		t.Fatalf("v1.4 JSON changed:\n got %q\nwant %q", got, want)
	}
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
