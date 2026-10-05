package commands

// The `help` command: cobra's default behaviour plus `help --json`.
//
// Purpose: EPIC D3 publishes the command registry as `nself help --json
// [path...]` instead of a new top-level verb. Without --json the command
// replicates cobra's built-in help command exactly (same Use, Short, Long,
// completion and output), so human `nself help [cmd]` is byte-identical.
//
// Inputs: the positional command path, the global --json flag, the registry
// of the live RootCmd tree for the current compat mode.
//
// Outputs: JSON mode writes one v1 envelope (command "help") whose data is the
// whole registry, or the subtree rooted at the named command. An unknown path
// is E401. Non-JSON mode writes cobra's help text, or cobra's "Unknown help
// topic" message plus the root usage (exit 0, as cobra does).
//
// Constraints:
//   - The command is installed with RootCmd.SetHelpCommand in init(), so
//     cobra's InitDefaultHelpCmd (called by the decorator, the registry
//     accessor and cobra's Execute) adopts it instead of creating its own.
//   - The registry always lists every command including hidden ones (contract
//     cli.command-registry v1); it holds no values from the user's project.
//   - JSON refusal for unsupported commands is the decorator's job; `help`
//     is registered as an envelope command in registry_types.go.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/observability"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
)

func init() { RootCmd.SetHelpCommand(newHelpCommand()) }

// helpWriter is the output seam of `help --json`; tests substitute buffers.
var helpWriter = output.Default

// newHelpCommand builds the help command. Use, Short, Long and the completion
// function mirror cobra's InitDefaultHelpCmd for the vendored cobra version.
func newHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long: `Help provides help for any command in the application.
Simply type ` + RootCmd.DisplayName() + ` help [path to command] for full details.`,
		ValidArgsFunction: helpCompletion,
		RunE:              runHelp,
	}
}

// helpCompletion completes command names the way cobra's help command does.
func helpCompletion(c *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	var completions []cobra.Completion
	cmd, _, e := c.Root().Find(args)
	if e != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if cmd == nil {
		cmd = c.Root() // root help command
	}
	for _, sub := range cmd.Commands() {
		// cobra also lists the root's own help command, which
		// IsAvailableCommand hides.
		if sub.IsAvailableCommand() || (cmd == c.Root() && sub.Name() == c.Name()) {
			if strings.HasPrefix(sub.Name(), toComplete) {
				completions = append(completions, cobra.CompletionWithDesc(sub.Name(), sub.Short))
			}
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// runHelp dispatches on JSON mode.
func runHelp(c *cobra.Command, args []string) error {
	on, err := jsonModeOf(c)
	if err != nil {
		return err
	}
	if on {
		return runHelpJSON(c, args)
	}
	return runHelpText(c, args)
}

// runHelpText is cobra's default help behaviour, verbatim.
func runHelpText(c *cobra.Command, args []string) error {
	cmd, _, e := c.Root().Find(args)
	if cmd == nil || e != nil {
		c.Printf("Unknown help topic %#q\n", args)
		cobra.CheckErr(c.Root().Usage())
		return nil
	}
	// Flow the context down to be used in help text.
	if cmd.Context() == nil && c.Context() != nil {
		cmd.SetContext(c.Context())
	}
	cmd.InitDefaultHelpFlag()    // make possible 'help' flag to be shown
	cmd.InitDefaultVersionFlag() // make possible 'version' flag to be shown
	cobra.CheckErr(cmd.Help())
	return nil
}

// runHelpJSON emits the registry, or the subtree at the named command.
func runHelpJSON(c *cobra.Command, args []string) error {
	reg, err := commandRegistry()
	if err != nil {
		return err
	}
	data := reg
	if len(args) > 0 {
		target, rest, ferr := c.Root().Find(args)
		if ferr != nil || target == nil || len(rest) > 0 || target == c.Root() {
			return unknownHelpTopic(args)
		}
		sub := reg.Subtree(target.CommandPath())
		if sub == nil {
			return unknownHelpTopic(args)
		}
		data = sub
	}
	return output.EmitData(helpWriter(), "help", data)
}

// unknownHelpTopic is the E401 for a path that names no command.
func unknownHelpTopic(args []string) error {
	msg := fmt.Sprintf("unknown help topic %q", strings.Join(args, " "))
	return errs.New("E401", observability.Redact(msg)).
		WithFix("Run `nself help --json` to list every command path.")
}
