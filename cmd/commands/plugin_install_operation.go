package commands

// Purpose: serialize command-side plugin mutations with bundle operations.
// Inputs: context, plugin directory, and a mutation function.
// Outputs: the mutation error or an operation-lock acquisition error.
// Constraints: the mutex also serializes goroutines because the bundle lock
// is reentrant within a process. Callbacks must not acquire this helper again.
// Cross-process exclusion follows the bundle lock's Unix/WSL2 support.

import (
	"context"
	"sync"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

var pluginInstallOperationMu sync.Mutex

func withPluginInstallOperation(ctx context.Context, pluginDir string, fn func() error) error {
	pluginInstallOperationMu.Lock()
	defer pluginInstallOperationMu.Unlock()
	return bundle.LedgerStore(pluginDir).Operation(ctx, "plugin install/update", fn)
}

func installPluginWithOperation(ctx context.Context, cfg *config.Config, name, pluginDir string) error {
	return withPluginInstallOperation(ctx, pluginDir, func() error {
		return plugin.Install(ctx, cfg, name, pluginDir)
	})
}

// pluginUpdateFn is plugin.Update; tests replace it.
var pluginUpdateFn = plugin.Update

// updatePluginWithOperation updates one plugin and then refreshes its bundle
// ledger record in the same operation. plugin.Update leaves the ledger, which
// supplies the build's plugin pins, on the old version and checksum, so every
// update path (plugin update, --all, --tier, nself update) goes through here.
func updatePluginWithOperation(ctx context.Context, cfg *config.Config, name, pluginDir string) error {
	return withPluginInstallOperation(ctx, pluginDir, func() error {
		if err := pluginUpdateFn(ctx, cfg, name, pluginDir); err != nil {
			return err
		}
		return refreshBundleLedger(pluginDir, name)
	})
}
