package build

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/build/lockdrift"
	"github.com/nself-org/cli/internal/errs"
)

// CheckPluginLedgerStart rejects plugin drift before a frozen start; plain starts allow drift.
func CheckPluginLedgerStart(pluginDir string, frozen bool) error {
	if !frozen {
		return nil
	}
	drift, derr := lockdrift.PluginLedgerDrift(pluginDir)
	if derr != nil {
		return errs.New("E455", "cannot compare the plugin ledger with the plugin dir: "+derr.Error()).WithFix("run nself build, then start again")
	}
	if len(drift) > 0 {
		return errs.New("E455", fmt.Sprintf("the plugin dir differs from the bundle ledger in %d place(s):\n  %s", len(drift), strings.Join(drift, "\n  "))).
			WithWhy("--frozen refuses to start when the plugin dir was updated without re-running nself build").
			WithFix("run nself build to update the bundle ledger, then start again with --frozen")
	}
	return nil
}
