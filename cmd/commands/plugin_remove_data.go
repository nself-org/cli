package commands

// Purpose: the data half of a plugin remove (D-0330). Decides whether
// `nself remove <plugin>` keeps the plugin's schema/tables, and asks before it
// deletes them, the same way bundle remove does (E403 without --yes or a
// terminal answer). printPluginRemovePlan, the --dry-run of a plugin remove,
// lives here too.
// Inputs: the remove command's --keep-data, --delete-data and --yes flags, and
// compat.V15().
// Outputs: a pluginDataPlan (keep or drop, whether a confirmation is due, a
// warning line), or an error: contradictory flags, or E403 (exit 4) for an
// unconfirmed --delete-data.
// Constraints: v1.4 keeps its rule (drop unless --keep-data; --delete-data is
// a no-op) and only adds a warning, so a forced removal of a bundle member is
// never silent. v1.5 keeps data unless --delete-data. `nself plugin remove`
// has no --delete-data flag (it moves to `nself remove` in v1.5), so v1.5
// prints a note there naming the flag.

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/bundle"
	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/plugin"

	"github.com/spf13/cobra"
)

// pluginRemoveFn is plugin.Remove; tests swap it to see the keepData the command passes.
var pluginRemoveFn = plugin.Remove

// pluginDataPlan is what a plugin remove does with the plugin's data.
type pluginDataPlan struct {
	keep    bool   // the plugin's schema/tables stay
	confirm bool   // an explicit --delete-data that needs --yes or a terminal answer
	reason  string // why, for the dry-run data line
	notice  string // a warning line for stderr; empty for none
}

// planPluginRemoveData reads the data flags and returns the plan for the mode.
func planPluginRemoveData(cmd *cobra.Command) (pluginDataPlan, error) {
	keepData, _ := cmd.Flags().GetBool("keep-data")
	deleteData, _ := cmd.Flags().GetBool("delete-data")
	if deleteData && keepData {
		return pluginDataPlan{}, errors.New("--delete-data and --keep-data cannot be used together")
	}
	// compat.V15(D-0330): remove <plugin> drops plugin data unless --keep-data -> keeps it unless --delete-data, confirmed by --yes or a terminal
	if compat.V15() {
		switch {
		case deleteData:
			return pluginDataPlan{confirm: true, reason: "--delete-data; needs --yes or a terminal confirmation"}, nil
		case keepData:
			return pluginDataPlan{keep: true, reason: "the default from v1.5",
				notice: "warning: --keep-data is deprecated and has no effect from v1.5 (data is kept by default); it is removed in v1.6.0. Use --delete-data to drop data."}, nil
		}
		d := pluginDataPlan{keep: true, reason: "the default from v1.5; --delete-data drops it"}
		if cmd.Flags().Lookup("delete-data") == nil {
			// `nself plugin remove` has no --delete-data; say where it is.
			d.notice = "note: the plugin's data is kept; `nself remove <name> --delete-data` drops it."
		}
		return d, nil
	}
	switch {
	case keepData:
		return pluginDataPlan{keep: true, reason: "--keep-data"}, nil
	case deleteData:
		return pluginDataPlan{reason: "--delete-data"}, nil
	}
	return pluginDataPlan{reason: "the default for plugin remove",
		notice: "warning: plugin remove drops the plugin's schema/tables by default in v1.4 mode; from v1.5 data is kept unless you pass --delete-data. Pass --keep-data to keep it now."}, nil
}

// dataLine is the dry-run plan's data row.
func (d pluginDataPlan) dataLine() string {
	if d.keep {
		return fmt.Sprintf("  data: preserve (%s)", d.reason)
	}
	return fmt.Sprintf("  data: drop plugin schema/tables (%s)", d.reason)
}

// confirmPluginDeleteData is bundle remove's E403 gate for one plugin: an
// explicit v1.5 --delete-data needs --yes or a terminal answer before anything
// changes. A dry run never reaches it.
func confirmPluginDeleteData(cmd *cobra.Command, name string, d pluginDataPlan) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if !d.confirm || yes {
		return nil
	}
	prompt := fmt.Sprintf("Delete the data of plugin %s (schema/tables)? This cannot be undone.", name)
	if c := interactiveConfirm(cmd); c != nil && c(prompt) {
		return nil
	}
	return errs.New("E403", fmt.Sprintf("remove --delete-data deletes the data of plugin %q and was not confirmed", name)).
		WithFix("pass --yes to confirm, or run it on a terminal and answer y")
}

// printPluginRemovePlan is the --dry-run of a plugin remove: what would happen,
// including whether the bundle-reference guard would refuse. It changes nothing.
func printPluginRemovePlan(pluginDir, name string, data pluginDataPlan, force bool) error {
	if data.notice != "" {
		fmt.Fprintln(os.Stderr, data.notice)
	}
	fmt.Fprintf(os.Stderr, "Remove plan (dry-run) for plugin %s\n", name)
	fmt.Fprintln(os.Stderr, data.dataLine())
	refs, err := bundle.ReferencingBundles(pluginDir, name)
	switch {
	case err != nil:
		fmt.Fprintf(os.Stderr, "  bundles: unknown (ledger unreadable: %v)\n", err)
	case len(refs) == 0:
		fmt.Fprintln(os.Stderr, "  bundles: none reference this plugin")
	default:
		fmt.Fprintf(os.Stderr, "  bundles: referenced by %s\n", strings.Join(refs, ", "))
	}
	var warnings strings.Builder
	if gerr := bundle.GuardPluginRemove(&warnings, pluginDir, name, force); gerr != nil {
		fmt.Fprintf(os.Stderr, "  guard: would refuse: %v\n", gerr)
	} else {
		fmt.Fprintln(os.Stderr, "  guard: would proceed")
	}
	fmt.Fprintln(os.Stderr, "(dry-run) no changes made.")
	return nil
}
