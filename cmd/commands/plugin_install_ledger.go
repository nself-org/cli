package commands

// Purpose: `nself plugin install <name>` records the plugin as an explicit
// install in the bundle ledger (P7-PLUG-19), so a later bundle remove keeps a
// plugin the owner installed on its own (contract:cli.bundle-ledger v1, D6).
// Inputs: the plugin directory and the names of the plugins installed just now.
// Outputs: the ledger at <pluginDir>/.nself-state/bundles.json updated with
// MarkExplicit (version and tier read from each plugin.json).
// Constraints: only when the ledger file already exists (a plugin install never
// bootstraps a missing ledger: the first bundle install does). Third-party URL
// installs have no registry name and are skipped. A ledger failure prints a
// warning to w and never fails the install.

import (
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

func recordExplicitInstalls(w io.Writer, pluginDir string, installed []string) {
	var names []string
	for _, n := range installed {
		if !plugin.IsThirdPartyInstallSource(n) {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return
	}
	store := bundle.LedgerStore(pluginDir)
	if _, err := os.Stat(store.Path()); err != nil {
		return // no ledger yet (or unreadable): nothing to keep in step
	}
	facts, err := bundle.InstalledFacts(pluginDir, names)
	if err != nil {
		_, _ = fmt.Fprintf(w, "warning: could not record the install in the bundle ledger: %v\n", err)
		return
	}
	if len(facts) == 0 {
		return
	}
	if err := store.Update(func(l *ledger.Ledger) error {
		for _, f := range facts {
			l.MarkExplicit(f)
		}
		return nil
	}); err != nil {
		_, _ = fmt.Fprintf(w, "warning: the plugin is installed but recording it in the bundle ledger %s failed: %v\n", store.Path(), err)
	}
}
