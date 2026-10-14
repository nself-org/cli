package mcpgen_test

// Purpose: fuzz target of the tool-name derivation (P7-SURF-21, nightly in
// .github/workflows/fuzz-nightly.yml). Seeds also live in
// testdata/fuzz/FuzzToolName/.
// Constraints: no registry, no child, no disk: only cmdregistry.ToolName and the
// name rules of mcpgen on arbitrary strings.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/mcpgen"
	"github.com/nself-org/cli/schemas"
)

// mcpNameRe is the MCP tool name pattern.
var mcpNameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// FuzzToolName: for any two command paths, the name derivation never panics;
// a name that mcpgen accepts matches the MCP pattern and the registry
// charset, and agrees with cmdregistry.ValidateToolNames; two different valid
// paths with one name are reported as a collision, never accepted silently.
func FuzzToolName(f *testing.F) {
	for _, s := range [][2]string{
		{"nself status", "nself status urls"}, {"nself config-get", "nself config get"}, {"nself x; rm -rf ~", "x"},
		{"", ""}, {"nself ", "nself"}, {"a-b", "a b"}, {"a_b", "a-b"}, {"a__b", "a_ _b"}, {"A", "a"},
		{"../../etc/passwd", "a/b"}, {"a\x00b", "a\nb"}, {"a‮b", "a​b"}, {"stаtus", "status"},
		{strings.Repeat("a", 200), strings.Repeat("a ", 100)}, {"nself  sentry   ok", "sentry ok"},
	} {
		f.Add(s[0], s[1])
	}
	f.Fuzz(func(t *testing.T, a, b string) {
		na, nb := cmdregistry.ToolName(a), cmdregistry.ToolName(b)
		for _, p := range []struct{ path, name string }{{a, na}, {b, nb}} {
			ok := mcpgen.ValidToolName(p.name)
			reg := &cmdregistry.Registry{Commands: []cmdregistry.Command{{Path: p.path}}}
			if got := cmdregistry.ValidateToolNames(reg) == nil; got != ok {
				t.Fatalf("path %q name %q: mcpgen says %v, cmdregistry.ValidateToolNames says %v", p.path, p.name, ok, got)
			}
			if !ok {
				continue
			}
			if !mcpNameRe.MatchString(p.name) || len(p.name) > 64 || p.name == "" {
				t.Fatalf("path %q gave the accepted name %q, outside the MCP pattern", p.path, p.name)
			}
			if mcpgen.PluginPathOK(p.path) && strings.ContainsAny(p.name, "-.") {
				t.Fatalf("a valid plugin path %q gave %q", p.path, p.name)
			}
		}
		fuzzTools(t, a, b)
		if mcpgen.ValidToolName(na) && na == nb && strings.Join(strings.Fields(a), " ") != strings.Join(strings.Fields(b), " ") {
			reg := &cmdregistry.Registry{Commands: []cmdregistry.Command{{Path: a}, {Path: b}}}
			if err := cmdregistry.ValidateToolNames(reg); err == nil {
				t.Fatalf("paths %q and %q share the name %q and the collision went unreported", a, b, na)
			}
		}
	})
}

// fuzzTools generates the tool list of a registry holding two plugin commands
// named from a and b next to the core fixture. Whatever the names, the list
// builds, every name matches the pattern, none repeats and the core
// nself_foo_bar tool stays.
func fuzzTools(t *testing.T, a, b string) {
	reg, err := fixtureBuild(t, false, false, advNode(a, "fuzz a", "read", ""), advNode(b, "fuzz b", "read", ""))
	if err != nil {
		return // the mount refused the name: nothing reaches the generator
	}
	list, err := mcpgen.Tools(mcpgen.Options{Registry: reg, Invoke: noInvoke, Schema: schemas.Lookup, V15: true, Warn: func(string) {}})
	if err != nil {
		t.Fatalf("paths %q and %q stopped the tool list: %v", a, b, err)
	}
	seen := map[string]bool{}
	for _, st := range list {
		n := st.Tool.Name
		if !toolNameRe.MatchString(n) || seen[n] {
			t.Fatalf("paths %q and %q gave the bad or repeated tool name %q", a, b, n)
		}
		seen[n] = true
	}
	if !seen["nself_foo_bar"] {
		t.Fatalf("paths %q and %q removed the core tool nself_foo_bar", a, b)
	}
}
