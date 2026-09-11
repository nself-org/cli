package commands

// Purpose: `nself plugin count` — the ONE command that answers "how many
// plugins are there", replacing scripts/plugin-counts.sh (retired to a
// thin pointer, see plugin-counts.sh) and the independent counting logic
// that used to live in the web build and various docs. Works fully
// offline: the count artifact is embedded in the binary via
// internal/plugin/count, no registry checkout or network call required.
// Inputs: cobra command/flags (--json); no positional args.
// Outputs: a human-readable table with the advertised (installable,
// de-duplicated) count as the headline, or with --json the embedded
// plugins/counts.json artifact verbatim.
// Constraints: never computes a count itself — only renders the vendored
// artifact from internal/plugin/count. See that package's TODO(sync) for
// how the artifact is refreshed from nself-org/plugins.

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/plugin/count"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var pluginCountCmd = &cobra.Command{
	Use:   "count",
	Short: "Show the authoritative plugin counts (free, pro, advertised)",
	Long: `Show the authoritative plugin counts.

Reads the embedded plugins/counts.json artifact — the single generated
source of truth for "how many plugins are there" across the website, CLI,
and docs. Works fully offline: no registry checkout, no network call.

The "advertised" number is the installable, de-duplicated total: raw
registry entries minus any plugin marked non-installable, minus slugs that
appear in both the free and pro registries as the same plugin (not two).
This is the only number that should ever appear in user-facing copy.`,
	SilenceUsage: true,
	RunE:         runPluginCount,
}

func init() {
	pluginCountCmd.Flags().Bool("json", false, "Print the embedded counts.json artifact verbatim")
	pluginCmd.AddCommand(pluginCountCmd)
}

func runPluginCount(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	if jsonOut {
		_, err := os.Stdout.Write(count.RawJSON())
		return err
	}

	a, err := count.Load()
	if err != nil {
		return fmt.Errorf("plugin count: %w", err)
	}

	tbl := ui.NewTable("Registry", "Entries", "Installable")
	tbl.AddRow("free", strconv.Itoa(a.Free.Entries), strconv.Itoa(a.Free.Installable))
	tbl.AddRow("pro", strconv.Itoa(a.Pro.Entries), strconv.Itoa(a.Pro.Installable))
	tbl.AddRow("total", strconv.Itoa(a.Totals.Entries), strconv.Itoa(a.Totals.Installable))
	tbl.Render()

	if len(a.Overlap.Duplicates) > 0 {
		fmt.Printf("\n%d shared slug(s) counted once, not twice: %s\n",
			len(a.Overlap.Duplicates), strings.Join(a.Overlap.Duplicates, ", "))
	}

	fmt.Printf("\nAdvertised: %d\n", a.Advertised)
	fmt.Println("(installable, de-duplicated across free + pro: the number for any user-facing surface)")

	return nil
}
