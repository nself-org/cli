// remover_recover.go — resume an interrupted `bundle remove` (D-0368).
//
// Purpose: when the ledger no longer records the bundle but the remove intent
// does, the ledger lost the bundle after the intent was written (the file was
// deleted and re-bootstrapped, or the final write landed and only the intent
// cleanup failed). Restore the record so the rerun finishes the job instead of
// failing with E134.
// Inputs: the held ledger Tx, the loaded ledger, the bundle and the plugin dir.
// Outputs: the ledger to plan from; the intent is cleared when it is stale.
// Constraints: runs inside the Locked callback of Remove. Explicit flags are
// untouched. The bundle record comes back only when at least one remaining
// member is still on disk; otherwise the stale intent is just cleared.
package bundle

import (
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

// recoverIntent reads the intent for b and, if the ledger lost the bundle,
// restores it. It returns the ledger to plan from and the intent's start time
// (zero when there is no intent).
func recoverIntent(tx *ledger.Tx, l ledger.Ledger, b Bundle, pluginDir string, out io.Writer) (ledger.Ledger, ledger.RemoveIntent, bool, error) {
	in, found, err := tx.RemoveIntent(b.Slug)
	if err != nil {
		return l, in, false, fmt.Errorf("reading the remove intent: %w", err)
	}
	if !found {
		return l, in, false, nil
	}
	_, _ = fmt.Fprintf(out, "resuming an interrupted remove of bundle %s (started %s)\n", b.Slug, in.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	if _, recorded := l.Bundles[strings.ToLower(b.Slug)]; recorded {
		return l, in, true, nil
	}
	var members []ledger.InstalledPlugin
	for _, name := range in.Remaining {
		if isAlreadyInstalled(pluginDir, name) {
			members = append(members, ledger.InstalledPlugin{Name: name})
		}
	}
	if len(members) == 0 {
		if err := tx.ClearRemoveIntent(b.Slug); err != nil {
			return l, in, false, fmt.Errorf("clearing a stale remove intent: %w", err)
		}
		return l, in, false, nil
	}
	l = l.Clone()
	l.AddBundle(b.Slug, members, in.StartedAt)
	return l, in, true, nil
}
