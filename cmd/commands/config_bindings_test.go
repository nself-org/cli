package commands

// config_bindings_test.go — the config bindings gate over the real tree
// (P7-SURF-06, EPIC D6).
//
// Purpose: every flag whose upper-snake name is a known configuration key is
//          bound (flags.yaml) or exempt (exempt.yaml) with a reason; every
//          binding and exemption names a flag that exists; and the registry
//          reports the bound key as flag.env in both compat modes.
// Constraints: reads the live command tree through buildRegistry; no I/O.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config/bindings"
)

// registryFlagRefs lists every declared flag of the registry as canonical refs.
func registryFlagRefs(t *testing.T, v15 bool) []bindings.FlagRef {
	t.Helper()
	reg, err := buildRegistry(v15)
	if err != nil {
		t.Fatalf("buildRegistry(%v): %v", v15, err)
	}
	var refs []bindings.FlagRef
	for _, f := range reg.Root.Flags {
		refs = append(refs, bindings.FlagRef{Command: bindings.Root, Flag: f.Name})
	}
	for _, c := range reg.Commands {
		for _, f := range c.Flags {
			refs = append(refs, bindings.FlagRef{Command: strings.TrimPrefix(c.Path, "nself "), Flag: f.Name})
		}
	}
	return refs
}

func TestConfigBindingsGate(t *testing.T) {
	b, err := bindings.Load()
	if err != nil {
		t.Fatalf("bindings.Load: %v", err)
	}
	refs := registryFlagRefs(t, true)
	if len(refs) < 100 {
		t.Fatalf("only %d flags read from the registry: the gate would be vacuous", len(refs))
	}
	if err := b.Check(refs); err != nil {
		t.Fatalf("%v", err)
	}

	// Negative: the same tree plus an unbound config-named flag must fail.
	err = b.Check(append(append([]bindings.FlagRef{}, refs...), bindings.FlagRef{Command: "init", Flag: "base-domain"}))
	if err == nil || !strings.Contains(err.Error(), "BASE_DOMAIN") {
		t.Fatalf("an unbound base-domain flag must fail the gate, got %v", err)
	}

	// Registry: Flag.env carries the key on every bound flag, in both modes.
	for _, v15 := range []bool{true, false} {
		reg, err := buildRegistry(v15)
		if err != nil {
			t.Fatal(err)
		}
		cmd, ok := reg.Lookup("init")
		if !ok {
			t.Fatalf("v15=%v: no init command", v15)
		}
		var got *string
		for _, f := range cmd.Flags {
			if f.Name == "domain" {
				got = f.Env
			}
		}
		if got == nil || *got != "BASE_DOMAIN" {
			t.Errorf("v15=%v: init --domain env = %v, want BASE_DOMAIN", v15, got)
		}
	}
}
