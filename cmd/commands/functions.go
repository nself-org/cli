package commands

// functions.go — `nself functions` command group (CLI-R12 Batch B
// mechanical file-size split). Each subcommand's flags, cobra.Command var,
// its own init() flag registration, and RunE handler now live in their own
// file: functions_deploy.go, functions_list.go, functions_invoke.go,
// functions_logs.go, functions_delete.go.

import (
	"regexp"

	"github.com/spf13/cobra"
)

// functionNamePattern validates function names: lowercase alphanumeric + hyphens.
var functionNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]*$`)

// functionsCmd is the root of the `nself functions` command group.
var functionsCmd = &cobra.Command{
	Use:   "functions",
	Short: "Manage serverless functions",
	Long: `Deploy, list, invoke, stream logs for, and delete serverless functions
running inside the nself functions service.

The functions service must be enabled and running:
  nself service enable functions
  nself build && nself start`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself functions` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	functionsCmd.AddCommand(functionsDeployCmd)
	functionsCmd.AddCommand(functionsListCmd)
	functionsCmd.AddCommand(functionsInvokeCmd)
	functionsCmd.AddCommand(functionsLogsCmd)
	functionsCmd.AddCommand(functionsDeleteCmd)

	RootCmd.AddCommand(functionsCmd)
}
