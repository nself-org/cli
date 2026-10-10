package bundle

// installer_appready.go — what happens after the member loop succeeded: the
// ledger step, the app-ready steps and the verdict.
//
// Purpose: record the bundle in the ledger (<pluginDir>/.nself-state/bundles.json),
// then, when InstallOpts.Hooks is set, run reconcile, start, readiness, tracking
// and health and report app-ready or the first failing step as E135.
// Inputs: the Runner, the ledger store, the plugin directory and the planned plugins.
// Outputs: nil, or E135 (v1.5) / a printed warning (v1.4) for a failed step.
// Constraints: a ledger write failure does NOT roll the install back (the plugins
// are on disk); the error says so. Hooks nil: only the ledger step runs.

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// newLedgerStore is the ledger of pluginDir with its bootstrap source wired.
func newLedgerStore(pluginDir string) *ledger.Store {
	s := ledger.NewStore(pluginDir)
	s.Sources = LedgerSources(pluginDir)
	return s
}

func finishInstall(ctx context.Context, rn *appready.Runner, store *ledger.Store, pluginDir string, planned []string, opts InstallOpts, result *InstallResult) error {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	var names []string
	_ = rn.Step(appready.StepLedger, func() (string, error) {
		members, err := installedMembers(pluginDir, planned)
		if err != nil {
			return "", fmt.Errorf("reading installed plugins: %w", err)
		}
		if len(members) == 0 {
			return "no installed members to record", nil
		}
		if err := store.Update(func(l *ledger.Ledger) error {
			l.AddBundle(result.Bundle, members, time.Now())
			return nil
		}); err != nil {
			return "", fmt.Errorf("the plugins are installed (not rolled back) but recording them in %s failed: %w", store.Path(), err)
		}
		for _, m := range members {
			names = append(names, m.Name)
		}
		return fmt.Sprintf("%d plugins recorded", len(members)), nil
	})
	if opts.Hooks == nil {
		rn.Unverified("no app-ready hooks: only install and ledger ran")
	} else {
		oc := appready.Run(ctx, rn, *opts.Hooks, pluginDir, names, result.Changed)
		result.Reconciled = oc.Reconciled
		rep := rn.Report()
		appready.Render(out, rep)
	}
	if f := rn.Failure(); f != nil {
		return appReadyVerdict(out, result.Bundle, f)
	}
	return nil
}

// appReadyVerdict turns the first failed step into E135. A failed reconcile is
// not an app-ready problem: it returns its error in both modes, as bundle install
// did before the reconcile moved into Install (P7-LIVE-06).
func appReadyVerdict(out io.Writer, bundleSlug string, f *appready.Failure) error {
	if f.Step == appready.StepReconcile {
		return fmt.Errorf("bundle %s: reconcile failed after install: %w", bundleSlug, f.Err)
	}
	e := errs.Newf("E135", "bundle %s is not app-ready: step %s failed for plugin %s", bundleSlug, f.Step, f.Plugin).WithWhy(f.Err.Error())
	e.Wrapped = f.Err
	// compat.V15(P7-PLUG-19): bundle install not app-ready exits 0 with a warning -> E135 exit 2
	if compat.V15() {
		return e
	}
	_, _ = fmt.Fprintf(out, "warning: %s: %s\n", e.What, f.Err)
	return nil
}
