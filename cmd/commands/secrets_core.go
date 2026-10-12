package commands

// Purpose: The basic `nself secrets` CRUD subcommands — init (create a new
// encrypted store), set, get, and list. Split out of secrets.go (CLI-R12)
// to keep each concern's cobra.Command declarations (and their inline
// RunE bodies) in a file under the size cap; the top-level secretsCmd var
// and the init() that wires every subcommand onto it remain in secrets.go.
// Inputs: cobra.Command args/flags per subcommand (secret name/value, the
// shared --env flag).
// Outputs: reads/writes the internal/secrets encrypted store; prints
// confirmation or the requested value/listing.
// Constraints: each var here is a complete top-level cobra.Command
// declaration (RunE inline). `secrets set` takes VALUE on stdin or at a
// no-echo prompt (secrets_core_stdin.go) so a secret need not sit on argv;
// the two-argument form still works for machine surfaces, where the invoker
// keeps the value off the child argv (internal/invoke, D-0468).

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/secrets"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var secretsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate age key and set up .secrets/ directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return secrets.Init(cwd)
	},
}

var secretsSetCmd = &cobra.Command{
	Use:   "set <KEY> [VALUE]",
	Short: "Set a secret value (prompts if value not provided)",
	Long: `Set a secret value in the selected environment.

VALUE is optional. When it is omitted, or is "-", the value is read from stdin:
a terminal gets a no-echo prompt on stderr, a pipe is read to the end (up to
1 MiB, one trailing newline removed). Prefer that over typing VALUE in the
command line, where the shell history and the process list can show it.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		key := args[0]
		var value string
		if len(args) > 1 && args[1] != "-" {
			value = args[1]
		} else {
			v, err := readSecretValue(cmd, key)
			if err != nil {
				return err
			}
			value = v
		}
		if err := secrets.Set(cwd, secretsEnvFlag, key, value); err != nil {
			return err
		}
		fmt.Printf("Secret %s set in %s environment.\n", key, secretsEnvFlag)
		return nil
	},
}

var secretsGetCmd = &cobra.Command{
	Use:   "get <KEY>",
	Short: "Get a secret value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		value, err := secrets.Get(cwd, secretsEnvFlag, args[0])
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) { // CLI-only row: the decrypted value never crosses a machine surface
			setData(cmd, secretsGetData{Key: args[0], Environment: secretsEnvFlag, Value: value})
		}
		fmt.Println(value)
		return nil
	},
}

var secretsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all secrets for an environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		keys, entries, err := secrets.List(cwd, secretsEnvFlag)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, newSecretsListData(secretsEnvFlag, keys, entries))
		}
		if len(keys) == 0 {
			fmt.Printf("No secrets in %s environment.\n", secretsEnvFlag)
			return nil
		}

		tbl := ui.NewTable("Key", "Updated", "Rotated")
		for _, k := range keys {
			e := entries[k]
			rotated := "-"
			if e.RotatedAt != "" {
				rotated = e.RotatedAt[:10]
			}
			updated := "-"
			if e.UpdatedAt != "" {
				updated = e.UpdatedAt[:10]
			}
			tbl.AddRow(k, updated, rotated)
		}
		tbl.Render()
		fmt.Printf("\n%d secrets in %s environment.\n", len(keys), secretsEnvFlag)
		return nil
	},
}
