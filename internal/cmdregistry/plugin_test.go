package cmdregistry

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/spf13/cobra"
)

func pluginFixture() *cobra.Command {
	r := fixtureRoot(false)
	p := &cobra.Command{Use: "demo", Short: "demo", RunE: noop, Annotations: map[string]string{annPluginKey: "demo", annSourceKey: sourceInstalled}}
	s := &cobra.Command{Use: "sub", Short: "write", RunE: noop, Annotations: map[string]string{annPluginKey: "demo", annSourceKey: sourceInstalled, annSideKey: "write", annArgsKey: `[{"name":"file","required":true}]`, annFlagsKey: `[{"name":"yes","type":"bool","side_effect":"write"}]`}}
	p.AddCommand(s)
	r.AddCommand(p)
	return r
}

func TestRegistryPluginNodes(t *testing.T) {
	r := mustBuild(t, pluginFixture(), true)
	p, ok := r.Lookup("nself demo")
	if !ok || p.Canon != "plugin" || p.Plugin == nil || *p.Plugin != "demo" || p.SideEffect != "destructive" || p.JSON != "none" || p.Output != "document" {
		t.Fatalf("plugin root: %+v, %v", p, ok)
	}
	s, ok := r.Lookup("nself demo sub")
	if !ok || s.SideEffect != "write" || len(s.Args) != 1 || s.Args[0].Name != "file" || len(s.Flags) != 1 || s.Flags[0].Name != "yes" {
		t.Fatalf("plugin sub: %+v, %v", s, ok)
	}
}

func TestRegistryCountsPlugin(t *testing.T) {
	base := mustBuild(t, fixtureRoot(false), true)
	r := mustBuild(t, pluginFixture(), true)
	if r.Counts.Plugin != 2 || r.Counts.TopLevel != base.Counts.TopLevel {
		t.Fatalf("counts %+v base %+v", r.Counts, base.Counts)
	}
}

func TestRegistryBuiltinMountValidation(t *testing.T) {
	for _, tc := range []struct {
		name, source, want string
		entry              bool
	}{
		{"missing entry", sourceBuiltin, "needs a canon entry", false},
		{"wrong canon", sourceBuiltin, "must say canon plugin", true},
		{"installed owns core path", sourceInstalled, "plugin-mounted command's canon entry", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := fixtureRoot(false)
			root.AddCommand(&cobra.Command{Use: "fixture", RunE: noop, Annotations: map[string]string{annPluginKey: "fixture", annSourceKey: tc.source}})
			f := fixtureCanon(t)
			if tc.entry {
				f.Commands["fixture"] = canon.Entry{Canon: canon.CanonCore, SideEffect: canon.SideEffectRead}
			}
			_, err := Build(root, f, fixtureTypes(), fixtureOpts(true))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
	root := fixtureRoot(false)
	root.AddCommand(&cobra.Command{Use: "fixture", RunE: noop, Annotations: map[string]string{annPluginKey: "fixture", annSourceKey: sourceBuiltin}})
	f := fixtureCanon(t)
	f.Commands["fixture"] = canon.Entry{Canon: canon.CanonPlugin, SideEffect: canon.SideEffectRead}
	r, err := Build(root, f, fixtureTypes(), fixtureOpts(true))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := r.Lookup("nself fixture")
	if !ok || c.Canon != canon.CanonPlugin || c.Plugin == nil || *c.Plugin != "fixture" {
		t.Fatalf("builtin registry entry: %+v, found=%t", c, ok)
	}
}

func TestRegistryMalformedPluginAnnotations(t *testing.T) {
	root := pluginFixture()
	cmd, _, err := root.Find([]string{"demo", "sub"})
	if err != nil {
		t.Fatal(err)
	}
	cmd.Annotations[annArgsKey] = `broken`
	cmd.Annotations[annFlagsKey] = `broken`
	r := mustBuild(t, root, true)
	c, ok := r.Lookup("nself demo sub")
	if !ok || len(c.Args) != 0 || len(c.Flags) != 0 {
		t.Fatalf("invalid annotation must not create registry fields: %+v", c)
	}
}
