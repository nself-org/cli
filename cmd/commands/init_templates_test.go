package commands

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/errs"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
)

// TestPrintCloneNextSteps pins the clone-template hint to commands that exist:
// metadata apply lives under `nself db hasura` and takes no --file flag, and
// the legacy Flutter starter is never offered as a command to run (D-0127).
func TestPrintCloneNextSteps(t *testing.T) {
	tests := []struct {
		name    string
		plugins []string
		want    []string
		notWant []string
	}{
		{
			name:    "with plugins",
			plugins: []string{"chat", "realtime"},
			want: []string{
				"    cd /tmp/app\n",
				"    nself start\n",
				"    nself db migrate\n",
				"    nself db hasura metadata apply\n",
				"    nself plugin install chat realtime\n",
				"legacy starter app; see README.md",
			},
			notWant: []string{"--file", "flutter run", "nself hasura metadata"},
		},
		{
			name:    "no plugins",
			plugins: nil,
			want:    []string{"    nself db hasura metadata apply\n"},
			notWant: []string{"plugin install", "--file", "flutter run"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			printCloneNextSteps(&buf, "/tmp/app", tt.plugins)
			got := buf.String()
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, nw := range tt.notWant {
				if strings.Contains(got, nw) {
					t.Errorf("output contains stale %q:\n%s", nw, got)
				}
			}
		})
	}
}

// useTemplateSource swaps the entitlement source for the test. The listed
// plugins are licensed and uncovered; the returned counter is the number of
// licence questions asked.
func useTemplateSource(t *testing.T, licensed ...string) *int {
	t.Helper()
	calls := new(int)
	old := templateSource
	templateSource = func(context.Context) (clonetemplate.Lookup, clonetemplate.Entitled, error) {
		find := func(slug string) (clonetemplate.PluginInfo, bool) {
			for _, l := range licensed {
				if l == slug {
					return clonetemplate.PluginInfo{Slug: slug, Tier: "licensed", Bundles: []string{"synthetic"}}, true
				}
			}
			return clonetemplate.PluginInfo{Slug: slug, Tier: "free"}, true
		}
		return find, func(context.Context, string) (bool, error) { *calls++; return false, nil }, nil
	}
	t.Cleanup(func() { templateSource = old })
	return calls
}

// TestInitTemplateV14Unchanged: with the licence missing, v1.4 prints the
// report and scaffolds exactly the files the base binary wrote; v1.5 refuses
// with E110 and creates neither file nor directory.
func TestInitTemplateV14Unchanged(t *testing.T) {
	want, err := clonetemplate.Scaffold("discord-clone", clonetemplate.ScaffoldOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	compattest.Both(t, func(t *testing.T) {
		useTemplateSource(t, "chat", "realtime", "moderation", "auth")
		dest := filepath.Join(t.TempDir(), "app")
		err := runInitCloneTemplate("discord-clone", dest, false, false, false, true)
		if compat.V15() {
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E110" || errs.ExitCodeFor(err) != 3 {
				t.Fatalf("v1.5 err = %v, want E110 / exit 3", err)
			}
			if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
				t.Errorf("v1.5 refusal created %s", dest)
			}
			return
		}
		if err != nil {
			t.Fatalf("v1.4 must scaffold: %v", err)
		}
		got := 0
		_ = filepath.WalkDir(dest, func(_ string, d os.DirEntry, _ error) error {
			if d != nil && !d.IsDir() {
				got++
			}
			return nil
		})
		if got != len(want) {
			t.Errorf("v1.4 wrote %d files, want %d", got, len(want))
		}
	})
}

// TestInitTemplateFreeScaffolds: a free-only template scaffolds in v1.5 and
// asks no licence question.
func TestInitTemplateFreeScaffolds(t *testing.T) {
	compattest.Set(t, true)
	calls := useTemplateSource(t) // every plugin free
	dest := filepath.Join(t.TempDir(), "app")
	if err := runInitCloneTemplate("airbnb-clone", dest, false, false, false, true); err != nil {
		t.Fatalf("free template: %v", err)
	}
	if *calls != 0 {
		t.Errorf("free-only template made %d licence calls", *calls)
	}
	if _, err := os.Stat(filepath.Join(dest, "manifest.json")); err != nil {
		t.Errorf("free template not scaffolded: %v", err)
	}
}

const liveCatalog = `{"schema_version":1,"generated_from":{"plugins_ref":"a","bundles_ref":"b","generated_at":"2026-01-01T00:00:00Z"},"plugins":[
{"slug":"chat","tier":"licensed","version":"1.2.1","description":"synthetic","category":"misc","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":["chat"],"tier_pair":false},
{"slug":"notify","tier":"free","version":"1.2.1","description":"synthetic","category":"misc","maturity":"implemented","installable":true,"service_kind":"compose","commands":null,"bundles":[],"tier_pair":false}]}`

// TestTemplateSourceNoKey runs the production Source against a local catalog with no
// licence key: the licensed plugin is reported and the licence server (an
// address that refuses connections) is never needed.
func TestTemplateSourceNoKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(liveCatalog)) }))
	defer srv.Close()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NSELF_PLUGIN_CACHE", t.TempDir())
	t.Setenv("NSELF_LICENSE_KEY", "")
	t.Setenv("NSELF_PING_URL", "http://127.0.0.1:1")
	oldURL := catalogURL
	catalogURL = srv.URL
	t.Cleanup(func() { catalogURL = oldURL })
	find, entitled, err := templateSource(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rep := clonetemplate.Check(context.Background(), "t", []string{"chat", "notify"}, find, entitled)
	if len(rep.MissingLicence) != 1 || rep.MissingLicence[0].Plugin != "chat" || rep.Reason != "" {
		t.Errorf("report = %+v, want chat uncovered with no licence error", rep)
	}
}

func TestTemplateSourceNoCatalogIsE115(t *testing.T) {
	t.Setenv("NSELF_PLUGIN_CACHE", filepath.Join(t.TempDir(), "none"))
	oldURL := catalogURL
	catalogURL = "http://127.0.0.1:1/catalog.json"
	t.Cleanup(func() { catalogURL = oldURL })
	_, _, err := templateSource(context.Background())
	var ce *errs.CLIError
	if !errors.As(err, &ce) || ce.Code != "E115" {
		t.Fatalf("templateSource with no catalog and no cache = %v, want E115", err)
	}
}
