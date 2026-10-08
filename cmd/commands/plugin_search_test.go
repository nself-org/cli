package commands

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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
	fn()
	_ = w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
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
		if !compat.V15() && (!strings.Contains(got, `"Tier":"pro"`) || strings.Contains(got, `"license"`)) {
			t.Fatalf("v1.4 JSON: %s", got)
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
}

func TestPluginSearchLicensedEqualsPro(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"plugins":[{"name":"example","tier":"pro","description":"example"},{"name":"example","tier":"free","description":"example"}]}`))
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
		if a, b := run("licensed"), run("pro"); a != b || !strings.Contains(a, `"example"`) || strings.Contains(a, `"free"`) {
			t.Fatalf("licensed and pro result differ: %q / %q", a, b)
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
	if !strings.Contains(upsellUnlockLine(), "plugins") {
		t.Fatal(upsellUnlockLine())
	}
}

func TestBundleListVocabulary(t *testing.T) {
	if plugin.BundleLabel("Chat") != "Licensed (in the Chat Bundle)" {
		t.Fatal("bundle label")
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
	if !strings.Contains(got, "Licensed plugins") || !strings.Contains(got, "Free plugins") || a.Free.Installable == 0 || a.Pro.Installable == 0 {
		t.Fatalf("help counts: %s", got)
	}
}
