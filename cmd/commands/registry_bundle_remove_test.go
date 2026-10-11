package commands

// Purpose: P7-PLUG-20 (ruling R1): the registry must show that removing a
// bundle with --delete-data is destructive in both compat modes, and the
// helpers the bundle remove command tests share.
// Inputs: the canon view per mode (v1.4) and the real command tree built into
// the v1.5 registry.
// Constraints: nothing here runs a command body. The registry cannot express
// "write, but destructive with --delete-data" without understating the v1.4
// default (a paid bundle still drops data), so the class itself stays
// destructive (internal/canon/domains/extend.yaml, unchanged) and the flag is
// asserted to be in effect destructive too.

import (
	"os"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/spf13/cobra"
)

func TestRegistryBundleRemoveDestructive(t *testing.T) {
	// v1.4: the canon view classes the command itself as destructive (its
	// default drops the data of a paid bundle), so --delete-data cannot lower it.
	compattest.Set(t, false)
	v14, err := canon.Effective(false)
	if err != nil {
		t.Fatal(err)
	}
	if e, ok := v14.Commands["bundle remove"]; !ok || e.SideEffect != canon.SideEffectDestructive {
		t.Errorf("v1.4 canon: bundle remove = %+v (present %v), want destructive", e, ok)
	}

	// v1.5: bundle remove is a shim to remove, which is destructive; the
	// registry lists --delete-data on both and its effective class is destructive.
	compattest.Set(t, true)
	reattachRealTree()
	defer PrepareTreeForGeneration()()
	reg, err := BuildRegistry(true)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	// Both cobra commands carry the flag (bundle remove directly, remove for the v1.5 shim target).
	for _, c := range []*cobra.Command{bundleRemoveCmd, removeCmd} {
		if f := c.Flags().Lookup("delete-data"); f == nil || f.Value.Type() != "bool" {
			t.Errorf("%s has no bool --delete-data flag: %v", c.CommandPath(), f)
		}
	}
	for _, path := range []string{"nself remove", "nself bundle remove"} {
		c, ok := reg.Lookup(path)
		if !ok {
			t.Fatalf("%s is not in the v1.5 registry", path)
		}
		// The loaded entry itself must be destructive, with and without the flag set;
		// setting --delete-data may only raise the class, never lower it.
		without := cmdregistry.EffectiveSideEffect(c, map[string]bool{})
		with := cmdregistry.EffectiveSideEffect(c, map[string]bool{"delete-data": true})
		if c.SideEffect != canon.SideEffectDestructive || without != c.SideEffect {
			t.Errorf("%s class = %q, effective without flag = %q, want destructive", path, c.SideEffect, without)
		}
		if canon.Rank(with) < canon.Rank(without) || with != canon.SideEffectDestructive {
			t.Errorf("%s --delete-data effective = %q (without %q), want destructive", path, with, without)
		}
		// The shim lists no flags of its own; the target carries --delete-data.
		if path == "nself remove" {
			var flag *cmdregistry.Flag
			for i := range c.Flags {
				if c.Flags[i].Name == "delete-data" {
					flag = &c.Flags[i]
				}
			}
			if flag == nil || flag.Type != "bool" || flag.Hidden {
				t.Errorf("%s registry entry lacks a visible bool --delete-data flag: %+v", path, flag)
			}
		}
	}
	if s, _ := reg.Lookup("nself bundle remove"); s == nil || s.Target == nil || *s.Target != "nself remove" {
		t.Errorf("v1.5 bundle remove should be a shim to nself remove, got %+v", s)
	}
}

// seedBundleLedger records bundle slug with the given members in the ledger of
// pluginDir. It writes an empty ledger first when none exists, so bootstrap
// does not mark the members explicit (the fixture bundles are not in the
// display order the bootstrap knows).
func seedBundleLedger(t *testing.T, pluginDir, slug string, members ...string) {
	t.Helper()
	store := bundle.LedgerStore(pluginDir)
	if _, err := os.Stat(store.Path()); err != nil {
		data, merr := ledger.New().Marshal()
		if merr != nil {
			t.Fatal(merr)
		}
		if err := os.MkdirAll(store.Dir(), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(store.Path(), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var in []ledger.InstalledPlugin
	for _, m := range members {
		in = append(in, ledger.InstalledPlugin{Name: m, Version: "1.0.0", Tier: ledger.TierFree})
	}
	if err := store.Update(func(l *ledger.Ledger) error {
		l.AddBundle(slug, in, time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
