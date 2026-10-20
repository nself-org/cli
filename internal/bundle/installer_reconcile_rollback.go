package bundle

// Purpose: undo fresh members after a refused reconcile without dropping kept schemas.
// Inputs: pre-install membership and ledger, completed install result.
// Outputs: rollback result and restored ledger records. Failed removals stay recorded.
import (
	"context"
	"io"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

func rollbackReconcile(ctx context.Context, store *ledger.Store, dir string, before map[string]string, prior ledger.Ledger, opts InstallOpts, result *InstallResult, out io.Writer) error {
	var fresh []string
	for _, name := range result.Installed {
		if _, existed := before[strings.ToLower(name)]; !existed {
			fresh = append(fresh, name)
		}
	}
	result.RolledBack = rollbackInstalled(ctx, opts.Config, plugin.UndoInstall, dir, fresh, out)
	removed := make(map[string]bool)
	for _, name := range result.RolledBack {
		removed[strings.ToLower(name)] = true
	}
	return store.Update(func(l *ledger.Ledger) error {
		// Restore the pre-install ledger, retaining any fresh member whose cleanup
		// failed so that it remains discoverable for a later remove.
		current := l.Clone()
		*l = prior.Clone()
		// Existing members may have been upgraded; retain their current facts
		// while restoring the ownership references from before this install.
		for key, record := range l.Plugins {
			if now, ok := current.Plugins[key]; ok {
				record.Tier, record.Version, record.Checksum = now.Tier, now.Version, now.Checksum
				l.Plugins[key] = record
			}
		}
		for _, name := range fresh {
			key := strings.ToLower(name)
			if removed[key] {
				continue
			}
			if record, ok := current.Plugins[key]; ok {
				l.Plugins[key] = record
				for _, slug := range record.InstalledBy {
					l.Bundles[slug] = current.Bundles[slug]
				}
			}
		}
		return nil
	})
}
