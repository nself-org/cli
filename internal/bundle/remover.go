// remover.go — `nself bundle remove <bundle>` orchestrator (P7-PLUG-20).
//
// Purpose: remove a bundle without deleting what something else still needs.
// A plugin goes only when the bundle install ledger says no other installed
// bundle references it AND the owner did not install it on its own (PLUG D6);
// a plugin another bundle or the owner needs stays, with the reason printed.
// Inputs: a bundle slug and RemoveOpts (dry-run, delete-data, confirmation).
// Outputs: a RemoveResult; the plugins removed from the plugin directory and
// the ledger at <pluginDir>/.nself-state/bundles.json updated to match.
// Constraints: the plan, the removals and the ledger writes all happen inside
// one ledger lock (Store.Locked), so a concurrent install cannot add a
// reference between the plan and the delete. Before the first removal the
// ledger is written (a bootstrap ledger included) and a remove intent is
// recorded in .nself-state/remove-intent.json (remover_intent.go); the intent
// shrinks after each member and is cleared on full success, so a kill or a
// failed final write is resumed by running the same command again. Removal
// stops at the first failure and the ledger then reflects exactly what was
// removed. A member that an installed plugin outside the plan depends on is
// kept and recorded explicit (remover_deps.go).
// Plugin data is kept unless --delete-data (see remover_plan.go for the
// v1.4/v1.5 rules and the confirmation gate). Members are removed in reverse
// bundle order (a dependency before its dependents is removed last).
package bundle

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// RemoveOpts captures the user-controllable knobs for bundle remove.
type RemoveOpts struct {
	DryRun bool

	// DeleteData drops plugin data (schema/tables). It needs confirmation:
	// Yes, or Confirm answering true; otherwise Remove refuses with E403.
	DeleteData bool
	// Yes is the --yes flag: confirms DeleteData without a prompt.
	Yes bool
	// Confirm asks the owner on a terminal; nil means not interactive.
	Confirm func(prompt string) bool
	// KeepData is the deprecated --keep-data alias. From v1.5 it is a no-op
	// (data is kept by default); in v1.4 mode it keeps data of a paid bundle.
	KeepData bool

	// Out is the stream for human-facing progress; nil → os.Stderr.
	Out io.Writer

	// PluginDir overrides the default plugin location.
	PluginDir string

	// Config overrides the loaded project config.
	Config *config.Config

	// remover is an internal hook for tests. nil → plugin.Remove.
	remover func(ctx context.Context, cfg *config.Config, name, pluginDir string, keepData, force bool) error
	// reverseDeps is an internal hook for tests. nil → plugin.ReverseDependencies.
	reverseDeps func(pluginDir, name string) ([]string, error)
	// writeHook is an internal hook for tests: it can fail a ledger or intent write.
	writeHook func(path string) error
	// noIntent is an internal hook for tests: it skips the remove intent and the
	// checkpoint write (the pre-D-0368 behaviour). Production never sets it.
	noIntent bool
}

// RemoveResult captures the outcome of a bundle remove call.
type RemoveResult struct {
	Bundle       string
	Planned      []string     // plugins this call removes (on disk, no other reference)
	Kept         []KeptPlugin // members still on disk that stay, with the reason
	NotInstalled []string     // members not on disk
	Removed      []string
	Failed       []string
	DryRun       bool
	KeepData     bool // effective: plugin data was (or would be) kept
	Changed      bool // membership changed; the command layer must reconcile
}

// Remove removes the bundle's plugins that no other bundle and no explicit
// install needs, and forgets the bundle in the ledger. See the file comment.
func Remove(ctx context.Context, bundleSlug string, opts RemoveOpts) (*RemoveResult, error) {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}

	b, ok := Get(bundleSlug)
	if !ok {
		return nil, UnknownBundleError(bundleSlug)
	}
	if !b.IsInstallable() {
		return nil, fmt.Errorf("bundle %q is not removable as a unit (meta bundle or no plugins)", b.Slug)
	}
	if opts.DeleteData && opts.KeepData {
		return nil, errors.New("--delete-data and --keep-data cannot be used together")
	}

	deleteData, warn := dataPolicy(b, opts)
	if err := confirmDeleteData(b, opts); err != nil {
		return nil, err
	}
	if warn {
		_, _ = fmt.Fprintf(out, "warning: removing bundle %s drops plugin data by default in v1.4 mode; from v1.5 data is kept unless you pass --delete-data. Pass --keep-data to keep it now.\n", b.Slug)
	}

	result := &RemoveResult{Bundle: b.Slug, DryRun: opts.DryRun, KeepData: !deleteData}
	pluginDir := opts.PluginDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir()
	}
	store := newLedgerStore(pluginDir)
	if opts.writeHook != nil {
		store.SetWriteHookForTest(opts.writeHook)
	}

	if opts.DryRun {
		// Load never writes: a dry run leaves no ledger file behind.
		l, _, err := store.Load()
		if err != nil {
			return result, err
		}
		plan, perr := planWithKeeps(b, l, pluginDir, opts)
		if perr != nil {
			return result, unrecordedBundle(out, b, store.Path(), perr)
		}
		result.adopt(plan)
		printRemovePlan(out, b, result, opts)
		_, _ = fmt.Fprintln(out, "(dry-run) no changes made.")
		return result, nil
	}

	run := &removeRun{b: b, opts: opts, out: out, pluginDir: pluginDir, deleteData: deleteData, result: result}
	err := store.Locked(func(tx *ledger.Tx, l ledger.Ledger) error { return run.locked(ctx, tx, l) })
	removeErr := run.removeErr
	if errors.Is(err, errBundleNotRecorded) {
		return result, unrecordedBundle(out, b, store.Path(), err)
	}
	if err != nil {
		if len(result.Removed) > 0 {
			err = fmt.Errorf("removed %s but updating the bundle ledger %s failed: %w (run nself doctor)",
				strings.Join(result.Removed, ", "), store.Path(), err)
		}
		return result, errors.Join(removeErr, err)
	}

	_, _ = fmt.Fprintf(out, "\nBundle %q (%s) remove summary: %d removed, %d kept, %d failed, %d already absent.\n",
		b.Name, b.Slug, len(result.Removed), len(result.Kept), len(result.Failed), len(result.NotInstalled))
	if removeErr != nil {
		return result, removeErr
	}
	if len(result.Planned) == 0 {
		_, _ = fmt.Fprintln(out, "Nothing to remove from disk; the bundle is no longer recorded.")
	}
	return result, nil
}

// removeMembers removes result.Planned one at a time and stops at the first
// failure. force=true to plugin.Remove skips its own reverse-dependency check:
// planned members may depend on each other, and a plugin outside the plan that
// needs a member already made keepDependedOn keep that member. afterEach (may be
// nil) runs after each member is removed.
func removeMembers(ctx context.Context, out io.Writer, result *RemoveResult, cfg *config.Config, pluginDir string,
	removeFn func(context.Context, *config.Config, string, string, bool, bool) error, deleteData bool, afterEach func(name string)) error {
	if removeFn == nil {
		removeFn = plugin.Remove
	}
	for _, name := range result.Planned {
		_, _ = fmt.Fprintf(out, "  → removing %s...\n", name)
		if err := removeFn(ctx, cfg, name, pluginDir, !deleteData, true); err != nil {
			_, _ = fmt.Fprintf(out, "  ✗ %s remove failed: %v\n", name, err)
			result.Failed = append(result.Failed, name)
			return fmt.Errorf("bundle %q remove stopped at %s: %w (removed so far: %s; fix the cause and run 'nself bundle remove %s' again to continue)",
				result.Bundle, name, err, listOrNone(result.Removed), result.Bundle)
		}
		result.Removed = append(result.Removed, name)
		result.Changed = true
		_, _ = fmt.Fprintf(out, "  ✓ %s removed\n", name)
		if afterEach != nil {
			afterEach(name)
		}
	}
	return nil
}

func listOrNone(l []string) string {
	if len(l) == 0 {
		return "none"
	}
	return strings.Join(l, ", ")
}
