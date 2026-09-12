package commands

// build_plugin_lifecycle.go — plugin dormant/expiry banner + auto-removal
// for `nself build`. Split out of build.go (kept build.go under the
// 300-line cap when G-014 orphan-container detection was added) as a pure
// move: same checks/output/errors/order, no behavior change.

import (
	"context"
	"fmt"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"
)

// runPluginLifecycleCheck loads the lifecycle store, transitions expired plugins,
// prints dormant banners, and auto-removes fully-expired plugins.
// Auto-removal is intentionally build-only (not start) — start is read-only on lifecycle.
func runPluginLifecycleCheck(quiet bool) {
	store, err := plugin.LoadLifecycleStore()
	if err != nil {
		// Non-fatal: lifecycle store is advisory only.
		if !quiet {
			ui.Warn("Could not load plugin lifecycle store: " + err.Error())
		}
		return
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
	for _, name := range autoRemove {
		if !quiet {
			ui.Warn(fmt.Sprintf("Removing expired plugin %q (grace period exhausted)", name))
		}
		cfg, cfgErr := config.Load(".")
		if cfgErr != nil {
			// Fall back to default plugin dir.
			cfg = &config.Config{}
		}
		pluginDir := resolvePluginDir()
		if removeErr := plugin.Remove(context.Background(), cfg, name, pluginDir, false, true); removeErr != nil {
			if !quiet {
				ui.Warn(fmt.Sprintf("Auto-remove of %q failed: %v", name, removeErr))
			}
		} else {
			// Clear the record after successful removal.
			delete(store.Records, name)
		}
	}

	// Persist transitions (dormant → expired state changes).
	if len(dormant) > 0 || len(autoRemove) > 0 {
		if saveErr := store.Save(); saveErr != nil && !quiet {
			ui.Warn("Could not save plugin lifecycle store: " + saveErr.Error())
		}
	}
}
