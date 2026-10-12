// remover_recover.go — read the remove intent of an interrupted `bundle remove`
// (D-0368).
//
// Purpose: tell the user a remove is being resumed, and drop an intent whose
// bundle the ledger no longer records.
// Inputs: the held ledger Tx, the loaded ledger and the bundle.
// Outputs: whether the intent belongs to a recorded bundle; a stale intent is
// cleared.
// Constraints: runs inside the Locked callback of Remove. The checkpoint write
// makes the ledger durable before any delete, so a recorded bundle needs no
// repair. If the ledger does not record the bundle (the file was deleted and
// re-bootstrapped, which marks leftovers explicit), the intent is only cleared
// and the normal path follows (E134 on v1.5, a no-op warning on v1.4): nothing
// is rebuilt from the intent, because a plugin that came back unrecorded after
// a stale intent must not be deleted on its say-so.
package bundle

import (
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
)

// recoverIntent reads the intent for b. It returns the intent and true when
// the ledger records the bundle (a resume line is printed); a stale intent
// (bundle not recorded) is cleared and reported as absent.
func recoverIntent(tx *ledger.Tx, l ledger.Ledger, b Bundle, out io.Writer) (ledger.RemoveIntent, bool, error) {
	in, found, err := tx.RemoveIntent(b.Slug)
	if err != nil {
		return in, false, fmt.Errorf("reading the remove intent: %w", err)
	}
	if !found {
		return in, false, nil
	}
	if _, recorded := l.Bundles[strings.ToLower(b.Slug)]; !recorded {
		if err := tx.ClearRemoveIntent(b.Slug); err != nil {
			return in, false, fmt.Errorf("clearing a stale remove intent: %w", err)
		}
		return in, false, nil
	}
	_, _ = fmt.Fprintf(out, "resuming an interrupted remove of bundle %s (started %s)\n", b.Slug, in.StartedAt.Format("2006-01-02T15:04:05Z07:00"))
	return in, true, nil
}
