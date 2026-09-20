package commands

import (
	"github.com/spf13/cobra"
)

var secretsEnvFlag string

var secretsCmd = &cobra.Command{
	Use:   "secrets",
	Short: "Manage encrypted project secrets (age encryption)",
	Long: `Manage encrypted secrets for nSelf projects using age encryption.

Secrets are stored as age-encrypted JSON files per environment in .secrets/.
Each team member has their own age keypair; secrets are encrypted to all
team members' public keys.

Subcommands:
  init             Generate age key and set up .secrets/
  set <KEY>        Set a secret value
  get <KEY>        Get a secret value
  list             List all secrets
  edit             Open decrypted secrets in $EDITOR
  rotate <KEY>     Rotate a secret value
  decrypt-on-deploy Output secrets as KEY=VALUE for CI/CD
  audit            Report secrets needing rotation (>90 days)
  lint             Check for plaintext secrets in source code
  rekey            Re-encrypt removing a team member's key`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself secrets` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	// Global --env flag for all secrets subcommands.
	secretsCmd.PersistentFlags().StringVar(&secretsEnvFlag, "env", "dev", "Environment (dev, staging, prod)")

	secretsRekeyCmd.Flags().String("remove", "", "Public key to remove from recipients")
	secretsRotateCmd.Flags().Bool("dual-window", false, "Keep old key as _PREVIOUS during overlap window")

	// schedule: optional flags for creating a named schedule.
	secretsScheduleCmd.Flags().String("secret", "", "Secret name to schedule (requires --every)")
	secretsScheduleCmd.Flags().String("every", "", "Rotation interval in days, e.g. 90d (requires --secret)")

	// rotation-log: optional filter flag.
	secretsRotationLogCmd.Flags().String("secret", "", "Filter log to a specific secret name")

	secretsCmd.AddCommand(secretsInitCmd)
	secretsCmd.AddCommand(secretsSetCmd)
	secretsCmd.AddCommand(secretsGetCmd)
	secretsCmd.AddCommand(secretsListCmd)
	secretsCmd.AddCommand(secretsEditCmd)
	secretsCmd.AddCommand(secretsRotateCmd)
	secretsCmd.AddCommand(secretsRetireCmd)
	secretsCmd.AddCommand(secretsScheduleCmd)
	secretsCmd.AddCommand(secretsListSchedulesCmd)
	secretsCmd.AddCommand(secretsVerifyCmd)
	secretsCmd.AddCommand(secretsRotationLogCmd)
	secretsCmd.AddCommand(secretsDecryptOnDeployCmd)
	secretsCmd.AddCommand(secretsAuditCmd)
	secretsCmd.AddCommand(secretsLintCmd)
	secretsCmd.AddCommand(secretsRekeyCmd)

	RootCmd.AddCommand(secretsCmd)
}
