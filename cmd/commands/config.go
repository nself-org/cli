package commands

import (
	"github.com/spf13/cobra"
)

// configCmd is the parent command for all configuration management.
var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage project configuration",
	Long: `Manage configuration with interactive wizard, environment switching,
secret rotation, and validation.

Subcommands:
  config show      Display all key=value pairs (masked by default)
  config get       Get a single configuration value
  config set       Update a configuration value
  config list      List all known config keys with current values
  config validate  Validate configuration against all registered rules
  config export    Export config to a file
  config import    Import config from a file`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself config` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// --- Subcommand declarations ---

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show all config key=value pairs (masked by default)",
	Long: `Load the .env file (or .env.{env} when --env is set) and display all
key=value pairs sorted alphabetically. Secret values are masked as **** by
default; use --reveal to print plaintext values.

Secret keys are any key whose name contains: SECRET, PASSWORD, KEY, TOKEN.`,
	RunE: runConfigShow,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a single configuration value",
	Long: `Return the raw value of KEY from .env (or .env.{env} with --env).
Output is the plain value only, making it safe for scripting.

Exits non-zero if the key is not found.`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Update a configuration value (writes to .env)",
	Long: `Update KEY=VALUE in .env (or .env.{env} with --env) in-place.
If the key does not exist, it is appended to the end of the file.
Existing comments and formatting are preserved.

NOTE: config set always writes to .env, not .env.dev or other cascade files.`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known config keys with current values",
	Long: `Display a table of KEY | VALUE | SOURCE for every known configuration
variable. Unknown keys are omitted. Keys with no value show the default or
(unset). Use --env to select the environment file.`,
	RunE: runConfigList,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate configuration against all registered rules",
	Long: `Load config and run all registered validators. Failures are printed
to stderr.

Exits 0 when all validators pass; exits 1 if any fail.`,
	RunE: runConfigValidate,
}

var configExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export current config to a file or stdout",
	Long: `Write all key=value pairs from .env (or .env.{env}) to --output file
or stdout. Use --format to select env (default), json, or yaml output.
Secret values are included in the export. Use standard file permissions
to protect the output file.`,
	Args: cobra.NoArgs,
	RunE: runConfigExport,
}

var configImportCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import config from a file into .env",
	Long: `Read key=value pairs from FILE and merge them into .env (or .env.{env}).
Keys that already exist are overwritten; new keys are appended.
Prompts for confirmation before overwriting differing keys.
Use --force to skip the prompt.`,
	Args: cobra.ExactArgs(1),
	RunE: runConfigImport,
}

// --- init: register commands and flags ---

func init() {
	// Persistent flags inherited by all subcommands.
	configCmd.PersistentFlags().String("env", "", "Target environment (reads .env.{env})")
	configCmd.PersistentFlags().Bool("reveal", false, "Show secret values in plaintext")
	configCmd.PersistentFlags().Bool("json", false, "JSON output")

	// Show-specific flags.
	configShowCmd.Flags().String("format", "table", "Output format: table|yaml|json")

	// Import-specific flags.
	configImportCmd.Flags().Bool("force", false, "Skip confirmation prompt when overwriting keys")
	configImportCmd.Flags().Bool("dry-run", false, "Show what would change without writing")

	// Export-specific flags.
	configExportCmd.Flags().String("format", "env", "Output format: env|json|yaml")
	configExportCmd.Flags().String("output", "", "Output file path (default: stdout)")

	// Wire all subcommands to parent.
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configValidateCmd)
	configCmd.AddCommand(configExportCmd)
	configCmd.AddCommand(configImportCmd)

	RootCmd.AddCommand(configCmd)
}

// --- Helpers ---
