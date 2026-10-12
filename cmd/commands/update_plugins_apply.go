package commands

// Purpose: the plugin step of `nself update --yes` (P7-LIVE-10): install the
// versions the plan showed, and prove it.
// Inputs: the plan's plugin pins and the registry the plan was computed from.
// Outputs: installed plugins at the planned versions, or a coded error.
// Constraints: plugin.Update reads the shared registry cache (TTL), so --yes
// first writes the plan's registry into that cache; a plan (--plan) writes
// nothing. After each plugin the installed version must equal the planned one,
// else the caller's transactional restore of the lock pins runs.

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/lock"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/upgrade/plan"
)

// installedPluginVersion is the version in the plugin's installed manifest,
// "" when the plugin is not on disk.
func installedPluginVersion(pluginDir, slug string) (string, error) {
	facts, err := bundle.InstalledFacts(pluginDir, []string{slug})
	if err != nil {
		return "", fmt.Errorf("reading the installed %s manifest: %w", slug, err)
	}
	if len(facts) == 0 {
		return "", nil
	}
	return facts[0].Version, nil
}

// applyPluginPins primes the shared registry cache with reg (the registry the
// plan used), then updates each planned plugin and checks the installed version
// against the planned pin. A mismatch is a coded error naming both versions.
func applyPluginPins(ctx context.Context, w io.Writer, d updateDeps, dir string, reg *plugin.Registry, plugs []plan.Pin) error {
	if len(plugs) > 0 && reg != nil {
		if err := d.primeRegistry(reg); err != nil {
			return fmt.Errorf("preparing the plugin registry cache for the planned versions: %w", err)
		}
	}
	for _, pin := range plugs {
		progress(w, "Updating plugin %s: %s -> %s", pin.Name, pin.From, pin.To)
		if err := d.updatePlugin(ctx, dir, pin.Name); err != nil {
			return fmt.Errorf("updating plugin %q (plugins updated before it stay installed): %w", pin.Name, err)
		}
		got, err := d.installedVersion(dir, pin.Name)
		if err != nil {
			return fmt.Errorf("verifying plugin %q after the update: %w", pin.Name, err)
		}
		if strings.TrimPrefix(got, "v") != strings.TrimPrefix(pin.To, "v") {
			return errs.New("E400", fmt.Sprintf(
				"update: plugin %q: the plan said %s but %s is installed (plugins updated before it stay installed)", pin.Name, pin.To, orNone(got))).
				WithFix("The pins in " + lock.FileName + " were restored. Run nself update --plan and retry.")
		}
	}
	return nil
}

func orNone(v string) string {
	if v == "" {
		return "no version"
	}
	return v
}
