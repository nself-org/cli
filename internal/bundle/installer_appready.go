package bundle

// installer_appready.go — what happens after the member loop succeeded: the
// ledger step, the app-ready steps and the verdict.
//
// Purpose: record the bundle in the ledger (<pluginDir>/.nself-state/bundles.json),
// then, when InstallOpts.Hooks is set, run reconcile, start, readiness, tracking
// and health and report app-ready or the first failing step as E135.
// Inputs: the Runner, the ledger store, the plugin directory, the planned plugins and
// the plugins that were on disk before the install (name -> version).
// Outputs: nil, or E135 (v1.5) / a printed warning (v1.4) for a failed step.
// Constraints: a ledger write failure does NOT roll the install back (the plugins
// are on disk); the error says so. Hooks nil: only the ledger step runs.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/bundle/appready"
	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
)

// newLedgerStore is the ledger of pluginDir with its bootstrap source wired:
// the strict full-catalogue scan (installer_helpers_ledger.go, D-0374).
func newLedgerStore(pluginDir string) *ledger.Store {
	s := ledger.NewStore(pluginDir)
	s.Sources = bootstrapSources(pluginDir)
	return s
}

// addCatalogHolders adds to each member's installed_by every other recorded
// bundle whose catalogue lists it (D-0338), the rule the remove plan uses to
// restore lost references (keepCatalogMembers). A forced plugin remove and a
// reinstall through bundle A would otherwise leave installed_by naming only A
// although recorded bundle C also contains the plugin. Existing references are
// kept: the ledger errs toward keeping a plugin.
func addCatalogHolders(l *ledger.Ledger, installed string, members []ledger.InstalledPlugin, at time.Time) {
	recorded := make([]string, 0, len(l.Bundles))
	for slug := range l.Bundles {
		if slug != installed {
			recorded = append(recorded, slug)
		}
	}
	sort.Strings(recorded)
	for _, slug := range recorded {
		other, ok := Get(slug)
		if !ok {
			continue
		}
		var held []ledger.InstalledPlugin
		for _, m := range members {
			if containsFold(other.Plugins, m.Name) {
				// Name only: AddBundle keeps the recorded facts and installed_at.
				held = append(held, ledger.InstalledPlugin{Name: m.Name})
			}
		}
		if len(held) > 0 {
			l.AddBundle(slug, held, at)
		}
	}
}

func finishInstall(ctx context.Context, rn *appready.Runner, store *ledger.Store, pluginDir string, planned []string, before map[string]string, opts InstallOpts, result *InstallResult) error {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	prior, _, err := store.Load()
	if err != nil {
		return err
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
			// A member that was on disk before this install and has no ledger
			// record was installed on its own: record it explicit so a later
			// bundle remove keeps it. (Errs toward keeping the plugin.)
			for _, m := range members {
				key := strings.ToLower(m.Name)
				if _, wasThere := before[key]; !wasThere {
					continue
				}
				if _, known := l.Plugins[key]; !known {
					l.MarkExplicit(m)
				}
			}
			l.AddBundle(result.Bundle, members, time.Now())
			addCatalogHolders(l, result.Bundle, members, time.Now())
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
		if f.Step == appready.StepReconcile {
			return errors.Join(appReadyVerdict(out, result.Bundle, f), rollbackReconcile(ctx, store, pluginDir, before, prior, opts, result, out))
		}
		return appReadyVerdict(out, result.Bundle, f)
	}
	return nil
}

// stepCause keeps the failing step's error reachable for errors.Is and
// errors.As while hiding any exit coder inside it (an *exec.ExitError from
// docker compose): errs.ExitCodeFor finds ExitCode() first on the chain, so
// without this E135 would exit with docker's status instead of its registered 2.
type stepCause struct{ err error }

func (c *stepCause) Error() string { return c.err.Error() }
func (c *stepCause) Unwrap() error { return c.err }

// ExitCode is E135's registered exit status (2, codes_bundle_install.go); never 0.
func (c *stepCause) ExitCode() int { return 2 }

// appReadyVerdict turns the first failed step into E135. A failed reconcile is
// not an app-ready problem: it returns its error in both modes, as bundle install
// did before the reconcile moved into Install (P7-LIVE-06).
func appReadyVerdict(out io.Writer, bundleSlug string, f *appready.Failure) error {
	if f.Step == appready.StepReconcile {
		return fmt.Errorf("bundle %s: reconcile failed after install: %w", bundleSlug, f.Err)
	}
	e := errs.Newf("E135", "bundle %s is not app-ready: step %s failed for plugin %s", bundleSlug, f.Step, f.Plugin).WithWhy(f.Err.Error())
	e.Wrapped = &stepCause{err: f.Err}
	// compat.V15(P7-PLUG-19): bundle install not app-ready exits 0 with a warning -> E135 exit 2
	if compat.V15() {
		return e
	}
	_, _ = fmt.Fprintf(out, "warning: %s: %s\n", e.What, f.Err)
	return nil
}
