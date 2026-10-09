// remover.go — S2.T04: `nself bundle remove <bundle>` orchestrator.
//
// Walks the bundle's plugin list and removes each one via plugin.Remove. By
// default plugin data volumes are dropped; --keep-data preserves them.
// --dry-run prints the planned actions without touching the filesystem.
//
// Removal proceeds in REVERSE plugin order so that a plugin earlier in the
// list (often a dependency of later plugins, e.g. ai before claw) is removed
// last. plugin.Remove's reverse-dependency check is bypassed via force=true
// because the operator has explicitly asked to tear down the whole bundle.
package bundle

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// RemoveOpts captures the user-controllable knobs for bundle remove.
type RemoveOpts struct {
	DryRun   bool
	KeepData bool

	// Out is the stream for human-facing progress; nil → os.Stderr.
	Out io.Writer

	// PluginDir overrides the default plugin location.
	PluginDir string

	// Config overrides the loaded project config.
	Config *config.Config

	// remover is an internal hook for tests. nil → plugin.Remove.
	remover func(ctx context.Context, cfg *config.Config, name, pluginDir string, keepData, force bool) error
}

// RemoveResult captures the outcome of a bundle remove call.
type RemoveResult struct {
	Bundle       string
	Planned      []string // plugins planned for removal (post not-installed filter)
	NotInstalled []string
	Removed      []string
	Failed       []string
	DryRun       bool
	KeepData     bool
	Changed      bool // membership changed; the command layer must reconcile
}

// Remove tears down every plugin in the bundle. Plugins not currently
// installed are reported under NotInstalled and skipped. Per-plugin removal
// failures are collected in Failed and surfaced as the returned error, but
// removal continues for the remaining plugins rather than aborting on first
// failure — partial cleanup is more useful than a hung-bundle state.
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
		return nil, fmt.Errorf("bundle %q is not installable/removable as a unit (meta or free)", b.Slug)
	}

	result := &RemoveResult{
		Bundle:   b.Slug,
		DryRun:   opts.DryRun,
		KeepData: opts.KeepData,
	}

	pluginDir := opts.PluginDir
	if pluginDir == "" {
		pluginDir = defaultPluginDir()
	}

	// Bucket plugins: installed (planned) vs not-installed (skipped).
	for _, name := range b.Plugins {
		if isAlreadyInstalled(pluginDir, name) {
			result.Planned = append(result.Planned, name)
		} else {
			result.NotInstalled = append(result.NotInstalled, name)
		}
	}

	// Print plan.
	printRemovePlan(out, b, result, opts)

	if opts.DryRun {
		_, _ = fmt.Fprintln(out, "(dry-run) no changes made.")
		return result, nil
	}

	if len(result.Planned) == 0 {
		_, _ = fmt.Fprintln(out, "Nothing to remove.")
		return result, nil
	}

	removeFn := opts.remover
	if removeFn == nil {
		removeFn = func(ctx context.Context, cfg *config.Config, name, pd string, kd, force bool) error {
			return plugin.Remove(ctx, cfg, name, pd, kd, force)
		}
	}

	cfg := opts.Config
	if cfg == nil {
		loaded, lerr := loadConfigOrDefault()
		if lerr != nil {
			return result, fmt.Errorf("loading project config: %w", lerr)
		}
		cfg = loaded
	}

	// Remove in reverse order so that dependencies are torn down after their
	// dependents. The bundle's Plugins slice is curated so earlier entries
	// are typically dependencies (ai before claw, realtime before chat).
	for i := len(result.Planned) - 1; i >= 0; i-- {
		name := result.Planned[i]
		_, _ = fmt.Fprintf(out, "  → removing %s...\n", name)
		// force=true: operator opted into full bundle teardown; skip the
		// reverse-dependency check that would otherwise block.
		if err := removeFn(ctx, cfg, name, pluginDir, opts.KeepData, true); err != nil {
			_, _ = fmt.Fprintf(out, "  ✗ %s remove failed: %v\n", name, err)
			result.Failed = append(result.Failed, name)
			continue
		}
		result.Removed = append(result.Removed, name)
		result.Changed = true
		_, _ = fmt.Fprintf(out, "  ✓ %s removed\n", name)
	}

	_, _ = fmt.Fprintf(out, "\nBundle %q (%s) remove summary: %d removed, %d failed, %d already absent.\n",
		b.Name, b.Slug, len(result.Removed), len(result.Failed), len(result.NotInstalled))

	if len(result.Failed) > 0 {
		return result, fmt.Errorf("bundle %q remove had failures: %s", b.Slug, strings.Join(result.Failed, ", "))
	}
	return result, nil
}

// printRemovePlan prints the planned tear-down summary.
func printRemovePlan(out io.Writer, b Bundle, r *RemoveResult, opts RemoveOpts) {
	header := "Remove plan"
	if opts.DryRun {
		header = "Remove plan (dry-run)"
	}
	_, _ = fmt.Fprintf(out, "%s for bundle %s (%s)\n", header, b.Name, b.Slug)
	if opts.KeepData {
		_, _ = fmt.Fprintln(out, "  data: preserve (--keep-data)")
	} else {
		_, _ = fmt.Fprintln(out, "  data: drop plugin schema/tables")
	}
	if len(r.Planned) == 0 {
		_, _ = fmt.Fprintln(out, "  (no installed plugins from this bundle)")
	}
	for _, name := range r.Planned {
		_, _ = fmt.Fprintf(out, "  • %s\n", name)
	}
	if len(r.NotInstalled) > 0 {
		_, _ = fmt.Fprintf(out, "  not-installed: %s\n", strings.Join(r.NotInstalled, ", "))
	}
}
