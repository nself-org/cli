package commands

// build_plugin_lifecycle.go — plugin dormant/expiry banner + auto-removal
// for `nself build`. Split out of build.go (kept build.go under the
// 300-line cap when G-014 orphan-container detection was added) as a pure
// move: same checks/output/errors/order, no behavior change.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
)

// runPluginLifecycleCheck loads the lifecycle store, transitions expired plugins,
// prints dormant banners, and auto-removes fully-expired plugins.
// Auto-removal is intentionally build-only (not start) — start is read-only on lifecycle.
//
// A removal the plan announced (plugin-remove effect) that fails is fatal
// (EPIC ruling D): the error is returned and the build writes nothing. The
// records of plugins that were removed are still saved first. An unreadable
// store stays advisory (nothing was announced from it).
func runPluginLifecycleCheck(quiet bool) error {
	store, err := plugin.LoadLifecycleStore()
	if err != nil {
		// Non-fatal: lifecycle store is advisory only.
		if !quiet {
			ui.Warn("Could not load plugin lifecycle store: " + err.Error())
		}
		return nil
	}

	now := time.Now()
	dormant, autoRemove := store.CheckExpiry(now)

	// Print dormant banners.
	for _, name := range dormant {
		if rec, ok := store.Records[name]; ok && !quiet {
			ui.Warn(plugin.DormantBanner(rec, now))
		}
	}

	// Print banners for already-dormant plugins (transitioned in a prior run).
	for name, rec := range store.Records {
		if rec.State == plugin.StateDormant {
			alreadyPrinted := false
			for _, d := range dormant {
				if d == name {
					alreadyPrinted = true
					break
				}
			}
			if !alreadyPrinted && !quiet {
				ui.Warn(plugin.DormantBanner(rec, now))
			}
		}
	}

	// Auto-remove expired plugins.
	var failed []string
	for _, name := range autoRemove {
		if !quiet {
			ui.Warn(fmt.Sprintf("Removing expired plugin %q (grace period exhausted)", name))
		}
		restoreEnv := build.SnapshotEnv() // config.Load exports the cascade; the write must not see it
		cfg, cfgErr := config.Load(".")
		restoreEnv()
		if cfgErr != nil {
			// Fall back to default plugin dir.
			cfg = &config.Config{}
		}
		pluginDir := resolvePluginDir()
		if removeErr := plugin.Remove(context.Background(), cfg, name, pluginDir, false, true); removeErr != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", name, removeErr))
		} else {
			// Clear the record after successful removal.
			delete(store.Records, name)
		}
	}

	// Persist transitions (dormant → expired state changes).
	if len(dormant) > 0 || len(autoRemove) > 0 {
		if saveErr := store.Save(); saveErr != nil {
			failed = append(failed, "saving the lifecycle store: "+saveErr.Error())
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("expired plugin removal failed, nothing was built (plugins removed before the failure stay removed): %s", strings.Join(failed, "; "))
	}
	return nil
}
