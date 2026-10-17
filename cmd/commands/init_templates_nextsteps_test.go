package commands

// Purpose: pins what `nself init --template` prints as next steps: the six app
// clones print what they printed at the merge-base (golden), multi-app prints
// exactly its manifest's next_steps and never mentions the flutter starter.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
	"github.com/spf13/cobra"
)

func TestInitTemplateNextSteps(t *testing.T) {
	// Golden: byte-for-byte the output at the merge-base for the six clones.
	for _, name := range clonetemplate.All() {
		if name == "multi-app" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			m, err := clonetemplate.GetManifest(name)
			if err != nil {
				t.Fatal(err)
			}
			want := "\n  Next steps:\n    cd /tmp/app\n    nself start\n    nself db migrate\n    nself db hasura metadata apply\n"
			if len(m.RequiredPlugins) > 0 {
				want += "    nself plugin install " + strings.Join(m.RequiredPlugins, " ") + "\n"
			}
			want += "\n  The flutter/ folder holds a legacy starter app; see README.md.\n\n"
			var buf bytes.Buffer
			printTemplateNextSteps(&buf, "/tmp/app", m)
			if buf.String() != want {
				t.Errorf("output changed:\n%q\nwant:\n%q", buf.String(), want)
			}
		})
	}

	t.Run("multi-app", func(t *testing.T) {
		m, err := clonetemplate.GetManifest("multi-app")
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		printTemplateNextSteps(&buf, "/tmp/app", m)
		want := "\n  Next steps:\n    cd /tmp/app\n    nself build\n    nself start\n" +
			"    nself db migrate up --migration-dir migrations\n    nself db reconcile --apply\n\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
		for _, bad := range []string{"flutter", "hasura metadata apply", "plugin install"} {
			if strings.Contains(buf.String(), bad) {
				t.Errorf("multi-app next steps mention %q", bad)
			}
		}
	})

	// A generic template without a flutter/ folder prints the generic steps and no flutter note.
	t.Run("generic without flutter", func(t *testing.T) {
		var buf bytes.Buffer
		printTemplateNextSteps(&buf, "/tmp/app", clonetemplate.Manifest{Name: "multi-app"})
		want := "\n  Next steps:\n    cd /tmp/app\n    nself start\n    nself db migrate\n    nself db hasura metadata apply\n\n"
		if buf.String() != want {
			t.Errorf("output = %q, want %q", buf.String(), want)
		}
	})
}

// TestTemplateInfoBundledOffline: `nself template info multi-app` answers from
// the embedded manifest without touching the registry; other names still go there.
func TestTemplateInfoBundledOffline(t *testing.T) {
	compattest.Set(t, false)
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.NotFound(w, nil)
	}))
	defer srv.Close()
	t.Setenv("NSELF_TEMPLATE_REGISTRY_URL", srv.URL)
	run := func(asJSON bool, slug string) (string, error) {
		_ = templateInfoCmd.Flags().Set("json", map[bool]string{true: "true", false: "false"}[asJSON])
		t.Cleanup(func() { _ = templateInfoCmd.Flags().Set("json", "false") })
		return captureStdout(t, func() error { return templateInfoCmd.RunE(templateInfoCmd, []string{slug}) })
	}

	out, err := run(false, "multi-app")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"multi-app v1.0.0", "nself build", "nself db migrate up --migration-dir migrations", "nself db reconcile --apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("info output lacks %q:\n%s", want, out)
		}
	}
	for _, bad := range []string{"hasura metadata apply", "flutter", "plugin install"} {
		if strings.Contains(out, bad) {
			t.Errorf("info output mentions %q:\n%s", bad, out)
		}
	}

	out, err = run(true, "multi-app")
	if err != nil {
		t.Fatal(err)
	}
	var e templateEntry
	if err := json.Unmarshal([]byte(out), &e); err != nil || e.Slug != "multi-app" || e.TemplateVersion != "1.0.0" || e.RequiredPlugins == nil {
		t.Errorf("json = %q (%v), want the bundled multi-app entry with an empty plugin list", out, err)
	}
	if n := atomic.LoadInt32(&hits); n != 0 {
		t.Fatalf("bundled info made %d registry requests, want 0", n)
	}

	if _, err := run(false, "saas-starter"); err == nil {
		t.Error("an unbundled name must still fail through the registry (server returns 404)")
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Error("an unbundled name never reached the registry")
	}
}

// TestInitMultiAppJSONNext: `init --template multi-app --json` reports the
// manifest's next_steps and never the generic `hasura metadata apply`
// (it would replace the whole metadata document). The six clones keep the
// generic list.
func TestInitMultiAppJSONNext(t *testing.T) {
	useTemplateSource(t)
	run := func(template string) []string {
		out, _ := pilotBuffers(t)
		c := &cobra.Command{Use: "init"}
		f := c.Flags()
		f.Bool("json", false, "")
		for _, b := range []string{"fast", "interactive", "non-interactive", "no-seed", "dry-run", "skip-validation", "wizard", "demo", "full", "force", "quiet", "no-pgvector", "list-presets"} {
			f.Bool(b, false, "")
		}
		for _, n := range []string{"template", "name", "domain", "profile", "preset", "cs-template"} {
			f.String(n, "", "")
		}
		c.RunE = initCmd.RunE
		c.SetContext(context.Background())
		dest := filepath.Join(t.TempDir(), "app")
		if err := c.ParseFlags([]string{"--json", "--quiet", "--template", template, dest}); err != nil {
			t.Fatal(err)
		}
		var err error
		inDir(t, t.TempDir(), func() { err = c.RunE(c, c.Flags().Args()) })
		if err != nil {
			t.Fatal(err)
		}
		return stringList(t, validateEnvelope(t, out.String(), "init")["next"])
	}
	m, _ := clonetemplate.GetManifest("multi-app")
	if got := run("multi-app"); strings.Join(got, "|") != strings.Join(m.NextSteps, "|") || strings.Contains(strings.Join(got, " "), "metadata apply") {
		t.Errorf("multi-app next = %v, want %v", got, m.NextSteps)
	}
	if got := run("discord-clone"); len(got) < 3 || got[0] != "nself start" {
		t.Errorf("discord-clone next = %v, want the generic clone steps", got)
	}
}
