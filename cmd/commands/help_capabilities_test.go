package commands

// Purpose: prove `help capabilities` ranks the recorded queries, finds the
// plugin in the cached catalogue, honours --limit and speaks the v1 envelope.
// Constraints: the registry tests run in a fresh process (the real tree is
// shared with tests that redeclare flags); no network; the catalogue is the
// fixture in testdata/capabilities.

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin/catalog"
	"github.com/spf13/pflag"
)

func fixtureCatalog(t *testing.T) []catalog.Entry {
	t.Helper()
	b, err := os.ReadFile("testdata/capabilities/catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := catalog.Decode(b)
	if err != nil {
		t.Fatalf("fixture catalogue: %v", err)
	}
	return c.Plugins
}

func TestHelpCapabilities(t *testing.T) {
	if !inFreshProcess(t, "TestHelpCapabilities") {
		return
	}
	compattest.Set(t, true)
	resetMachineRegistry()
	reg, err := commandRegistry()
	if err != nil {
		t.Fatal(err)
	}
	plugins := fixtureCatalog(t)
	var queries []struct{ Query, Kind, Expect string }
	b, err := os.ReadFile("testdata/capabilities/queries.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &queries); err != nil || len(queries) != 8 {
		t.Fatalf("queries.json: %d queries, %v", len(queries), err)
	}
	for _, q := range queries {
		got := rankCapabilities(reg, plugins, q.Query, 3)
		found := false
		for _, m := range got {
			id := m.Path
			if m.Kind == "plugin" {
				id = m.Slug
			}
			found = found || (m.Kind == q.Kind && id == q.Expect)
		}
		if !found {
			t.Errorf("%q: want %s %q in the top 3, got %+v", q.Query, q.Kind, q.Expect, got)
		}
	}

	// A plugin that is not installable is never offered.
	for _, m := range rankCapabilities(reg, plugins, "kubernetes", 50) {
		if m.Slug == "planned" {
			t.Errorf("a not-installable plugin was offered: %+v", m)
		}
	}
	// The run text is exactly what runs it.
	for _, m := range rankCapabilities(reg, plugins, "kubernetes", 5) {
		if m.Kind == "plugin" && m.Run != "nself add "+m.Slug {
			t.Errorf("plugin run text %q", m.Run)
		}
		if m.Kind == "command" && m.Run != "nself "+m.Path {
			t.Errorf("command run text %q", m.Run)
		}
	}
	// --limit bounds the list; a query of only stop words matches nothing.
	if n := len(rankCapabilities(reg, plugins, "config", 2)); n != 2 {
		t.Errorf("limit 2 returned %d", n)
	}
	if got := rankCapabilities(reg, plugins, "to the a of", 5); len(got) != 0 {
		t.Errorf("stop words matched %+v", got)
	}
	// Hidden, shim and builtin commands are never candidates.
	for _, m := range rankCapabilities(reg, nil, "completion", 50) {
		if m.Path == "completion" || strings.HasPrefix(m.Path, "completion ") {
			t.Errorf("builtin offered: %+v", m)
		}
	}
	t.Log("CHECKED-TestHelpCapabilities")
}

func TestHelpCapabilitiesEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no cached catalogue: command hits only
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var out, errBuf bytes.Buffer
	old := helpWriter
	helpWriter = func() output.Writer { return output.Writer{Out: &out, Err: &errBuf} }
	t.Cleanup(func() { helpWriter = old })
	resetRegistryCache()
	t.Cleanup(resetRegistryCache)
	reattachRealTree()
	RootCmd.InitDefaultHelpCmd()
	c := capabilitiesCmd
	if err := c.ParseFlags([]string{"--json"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if f.Name == "json" {
				_ = f.Value.Set("false")
				f.Changed = false
			}
		})
	})
	if err := runCapabilities(c, []string{"diagnostic", "doctor"}); err != nil {
		t.Fatal(err)
	}
	var env struct {
		SchemaVersion string `json:"schema_version"`
		Command       string `json:"command"`
		Data          CapabilitiesResult
	}
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatalf("not one JSON document: %v\n%s", err, out.String())
	}
	if env.SchemaVersion != "1" || env.Command != "help capabilities" || env.Data.Query != "diagnostic doctor" {
		t.Fatalf("envelope %+v", env)
	}
	if len(env.Data.Results) == 0 || env.Data.Results[0].Path != "doctor" || env.Data.Results[0].Run != "nself doctor" {
		t.Fatalf("results %+v", env.Data.Results)
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr: %q", errBuf.String())
	}
}

func TestHelpCapabilitiesLimitRefused(t *testing.T) {
	c := capabilitiesCmd
	if err := c.Flags().Set("limit", "0"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Flags().Set("limit", "5") }()
	err := runCapabilities(c, []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "--limit") {
		t.Fatalf("limit 0: %v", err)
	}
}
