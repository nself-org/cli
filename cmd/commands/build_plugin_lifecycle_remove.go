package commands

// build_plugin_lifecycle_remove.go — removal of one expired plugin for
// `nself build` (D-0324).
//
// Purpose: remove an expired plugin the same way `nself remove` does: check it
// against the bundle ledger first, and drop its ledger record afterwards.
// Inputs: a warning writer, the config, the plugin dir and the plugin name.
// Outputs: nil when the plugin is gone; the guard's or the remover's error.
// Constraints: in v1.5 a plugin an installed bundle references is refused with
// E133 (the caller counts that as a failed removal, which is fatal to the
// build, so the plan and the apply agree). In v1.4 the guard only warns. A
// ledger update failure after the plugin is gone is a warning, not a failure.

import (
	"context"
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// lifecycleRemovePlugin removes a plugin from disk; a seam so tests can fake it.
var lifecycleRemovePlugin = plugin.Remove

// removeExpiredPlugin guards, removes and forgets one expired plugin.
func removeExpiredPlugin(w io.Writer, cfg *config.Config, pluginDir, name string) error {
	if err := bundle.GuardPluginRemove(w, pluginDir, name, false); err != nil {
		return err
	}
	if err := lifecycleRemovePlugin(context.Background(), cfg, name, pluginDir, false, true); err != nil {
		return err
	}
	if err := bundle.ForgetPlugin(pluginDir, name); err != nil {
		_, _ = fmt.Fprintf(w, "warning: plugin %q is removed but the bundle ledger could not be updated: %v\n", name, err)
	}
	return nil
}
