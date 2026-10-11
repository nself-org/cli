package commands

// Purpose: The audit/compliance-facing `nself secrets` subcommands —
// rotation-log (print rotation event history), decrypt-on-deploy (deploy
// hook helper), audit (compliance report), lint (config sanity checks),
// and rekey (rotate/remove recipient keys). Split out of secrets.go
// (CLI-R12); see secrets_core.go for the file-splitting rationale shared
// by every secrets_*.go file in this split.
// Inputs: cobra.Command args/flags per subcommand.
// Outputs: printed reports/logs, or an updated recipient key set (rekey).
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/secrets"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var secretsRotationLogCmd = &cobra.Command{
	Use:   "rotation-log",
	Short: "Show rotation event log",
	Long: `Display the history of rotation events recorded for this project.

Optionally filter by secret name with --secret.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		filterSecret, _ := cmd.Flags().GetString("secret")
		log, err := secrets.LoadRotationLog(cwd)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, newSecretsRotationLogData(log.Events, filterSecret))
		}
		if len(log.Events) == 0 {
			fmt.Println("No rotation events recorded.")
			return nil
		}
		tbl := ui.NewTable("Secret", "Rotated At", "Status", "Note")
		count := 0
		for _, e := range log.Events {
			if filterSecret != "" && e.SecretName != filterSecret {
				continue
			}
			rotatedAt := e.RotatedAt
			if len(rotatedAt) > 19 {
				rotatedAt = rotatedAt[:19]
			}
			tbl.AddRow(e.SecretName, rotatedAt, e.Status, e.Note)
			count++
		}
		if count == 0 {
			fmt.Printf("No events for secret %q.\n", filterSecret)
			return nil
		}
		tbl.Render()
		fmt.Printf("\n%d event(s).\n", count)
		return nil
	},
}

var secretsDecryptOnDeployCmd = &cobra.Command{
	Use:   "decrypt-on-deploy",
	Short: "Output decrypted secrets as KEY=VALUE for CI/CD",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		output, err := secrets.DecryptForDeploy(cwd, secretsEnvFlag)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) { // CLI-only row: the decrypted values never cross a machine surface
			setData(cmd, newSecretsDecryptData(secretsEnvFlag, output))
		}
		fmt.Print(output)
		return nil
	},
}

var secretsAuditCmd = &cobra.Command{
	Use:   "audit",
	Short: "Report secrets that haven't been rotated in >90 days",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		findings, err := secrets.Audit(cwd, secretsEnvFlag)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, newSecretsAuditData(secretsEnvFlag, findings))
		}
		if len(findings) == 0 {
			fmt.Println("All secrets are within rotation policy.")
			return nil
		}
		tbl := ui.NewTable("Key", "Issue", "Severity")
		for _, f := range findings {
			tbl.AddRow(f.Key, f.Issue, f.Severity)
		}
		tbl.Render()
		fmt.Printf("\n%d finding(s) in %s environment.\n", len(findings), secretsEnvFlag)
		return nil
	},
}

var secretsLintCmd = &cobra.Command{
	Use:   "lint",
	Short: "Check for plaintext secrets in git-tracked files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		findings, err := secrets.LintSecrets(cwd)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, newSecretsLintData(findings))
		}
		if len(findings) == 0 {
			fmt.Println("No plaintext secrets detected.")
			return nil
		}
		tbl := ui.NewTable("File", "Line", "Rule", "Description")
		for _, f := range findings {
			tbl.AddRow(f.File, fmt.Sprintf("%d", f.Line), f.Rule, f.Message)
		}
		tbl.Render()
		fmt.Printf("\n%d finding(s).\n", len(findings))
		return nil
	},
}

var secretsRekeyCmd = &cobra.Command{
	Use:   "rekey --remove <pubkey>",
	Short: "Re-encrypt all secrets, removing a team member's key",
	RunE: func(cmd *cobra.Command, args []string) error {
		removePubKey, _ := cmd.Flags().GetString("remove")
		if removePubKey == "" {
			return fmt.Errorf("--remove flag is required (specify the public key to remove)")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		return secrets.Rekey(cwd, removePubKey)
	},
}
