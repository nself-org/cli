package commands

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/plugin"
)

// TestRemoveDeleteDataHelp describes both compatibility modes so operators do
// not mistake v1.4's implicit data deletion for the v1.5 removal policy.
func TestRemoveDeleteDataHelp(t *testing.T) {
	flag := removeCmd.Flags().Lookup("delete-data")
	if flag == nil {
		t.Fatal("remove is missing --delete-data")
	}
	for _, policy := range []string{
		"v1.5 keeps data unless --delete-data",
		"asks for confirmation (or pass --yes)",
		"v1.4 plugin remove never asks and drops data unless --keep-data",
	} {
		if !strings.Contains(flag.Usage, policy) {
			t.Errorf("--delete-data help missing %q: %s", policy, flag.Usage)
		}
	}
}

// TestInstallResolvesBundlesBeforePlugins pins the resolution order CLI-R19
// depends on. A bundle and a plugin can share a name; `nself install nchat`
// must install the bundle, not a single plugin that happens to match.
func TestInstallResolvesBundlesBeforePlugins(t *testing.T) {
	names := bundle.Names()
	if len(names) == 0 {
		t.Fatal("no bundles registered — the bundle-first rule cannot be verified")
	}

	var installable int
	for _, name := range names {
		if resolvesToBundle(name) {
			installable++
		}
	}
	if installable == 0 {
		t.Fatalf("none of the %d registered bundles resolve as installable", len(names))
	}

	// A name that is definitely not a bundle must fall through to the plugin path.
	if resolvesToBundle("definitely-not-a-bundle-xyz") {
		t.Error("an unknown name resolved as a bundle")
	}
}

// TestInstallAndRemoveAreRegistered guards the sugar itself: the product claim
// is "everything else is one `nself install X` away", which is only true if
// those two commands exist at the top level.
func TestInstallAndRemoveAreRegistered(t *testing.T) {
	want := map[string]bool{"install": false, "remove": false}
	for _, c := range RootCmd.Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
			if c.Hidden {
				t.Errorf("%q is hidden; it is the documented short form and must appear in help", c.Name())
			}
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%q is not registered as a top-level command", name)
		}
	}
}

// TestUninstallIsHiddenButStillWorks is the safety property behind CLI-R19's
// deviation from the ticket. `uninstall` was NOT argv-rewritten onto
// `reset --purge`, because bare `uninstall` keeps database volumes while bare
// `reset` removes them — a blanket redirect would turn a safe habit into data
// loss. It stays registered and hidden so old invocations behave identically.
func TestUninstallIsHiddenButStillWorks(t *testing.T) {
	var found bool
	for _, c := range RootCmd.Commands() {
		if c.Name() != "uninstall" {
			continue
		}
		found = true
		if !c.Hidden {
			t.Error("uninstall should be hidden from help now that reset covers it")
		}
		if c.RunE == nil {
			t.Error("uninstall must keep a working RunE — hiding it must not disable it")
		}
		for _, flag := range []string{"keep-data", "purge", "yes"} {
			if c.Flags().Lookup(flag) == nil {
				t.Errorf("uninstall lost its --%s flag; old invocations would break", flag)
			}
		}
	}
	if !found {
		t.Fatal("uninstall is no longer registered — every existing script using it breaks")
	}
}

// TestResetAbsorbedUninstallModes checks that the replacement named in the
// deprecation warning actually exists. A warning pointing at a flag that is not
// there is worse than no warning.
func TestResetAbsorbedUninstallModes(t *testing.T) {
	for _, c := range RootCmd.Commands() {
		if c.Name() != "reset" {
			continue
		}
		for _, flag := range []string{"keep-data", "purge", "yes"} {
			if c.Flags().Lookup(flag) == nil {
				t.Errorf("reset is missing --%s, which the uninstall deprecation warning points at", flag)
			}
		}
		return
	}
	t.Fatal("reset is not registered")
}

// TestUnknownCommandSuggestsInstall verifies the hint a user actually sees when
// they type a command the core does not have. Before CLI-R19 the error was
// "plugin binary not found: nself-waf", which tells the user nothing about how
// to proceed.
func TestUnknownCommandSuggestsInstall(t *testing.T) {
	err := plugin.ProxyCommand("definitely-not-installed-xyz", nil)
	if err == nil {
		t.Fatal("expected an error for an uninstalled command")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nself install definitely-not-installed-xyz") {
		t.Errorf("error does not tell the user how to install it: %q", msg)
	}
}
