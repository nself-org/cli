package commands

// Purpose: register the nself bundle source and give it a positional <dir> argument.
// Inputs: the bundle directory. Outputs: the shared import command with the directory in the "dir" value.
// Constraints: works whichever of this init and registerImportCommands runs first.
import (
	"github.com/nself-org/cli/internal/importer"
	_ "github.com/nself-org/cli/internal/importer/nself"
	"github.com/spf13/cobra"
)

func init() { attachBundleArg(migrateCmd) }

// attachBundleArg finds (or builds) the nself source command and turns its hidden "dir" flag
// into one required positional argument.
func attachBundleArg(parent *cobra.Command) {
	var cmd *cobra.Command
	for _, child := range parent.Commands() {
		if child.Name() == "nself" {
			cmd = child
		}
	}
	if cmd == nil {
		for _, source := range importer.Sources() {
			if source.Name() == "nself" {
				cmd = importSourceCommand(source)
				parent.AddCommand(cmd)
			}
		}
	}
	if cmd == nil || cmd.Flags().Lookup("dir") == nil {
		return
	}
	_ = cmd.Flags().MarkHidden("dir")
	cmd.Use = "nself <dir>"
	cmd.Short = "Import and verify a portable bundle written by `nself db export`"
	cmd.Args = cobra.ExactArgs(1)
	run := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		if err := c.Flags().Set("dir", args[0]); err != nil {
			return err
		}
		return run(c, args)
	}
}
