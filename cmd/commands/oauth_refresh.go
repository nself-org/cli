package commands

import (
	"os"

	"github.com/nself-org/cli/internal/oauth"

	"github.com/spf13/cobra"
)

var oauthCmd = &cobra.Command{
	Use:   "oauth",
	Short: "Manage OAuth provider tokens",
	Long: `Manage OAuth provider tokens for nSelf plugins.

Subcommands:
  refresh   Trigger immediate token refresh outside the cron schedule`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself oauth` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var oauthRefreshCmd = &cobra.Command{
	Use:   "refresh <provider>",
	Short: "Trigger immediate OAuth token refresh for a provider",
	Long: `Trigger an immediate OAuth token refresh outside the cron schedule.

The refresh posts to the provider plugin's /oauth/refresh-now endpoint and
reports success/failure per account. Use after a provider outage (e.g. B-21-style
token expiry) to restore service without waiting for the next scheduled refresh.

Flags:
  --account-id   Scope refresh to a single account
  --all          Refresh every account on every configured provider (runs in parallel)

Success/failure is visible in the OAuth refresh Prometheus metrics within 5s
(requires the oauth-exporter from S41-T03 to be running).

OAuth tokens are never exposed in command output.

Examples:
  nself oauth refresh google
  nself oauth refresh google --account-id user@example.com
  nself oauth refresh --all`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOAuthRefresh,
}

func init() {
	f := oauthRefreshCmd.Flags()
	f.String("account-id", "", "Scope refresh to a specific account ID")
	f.Bool("all", false, "Refresh all providers (runs in parallel)")
	f.String("base-url", "", "nSelf API base URL (overrides NSELF_API_URL)")

	oauthCmd.AddCommand(oauthRefreshCmd)
	RootCmd.AddCommand(oauthCmd)
}

func runOAuthRefresh(cmd *cobra.Command, args []string) error {
	all, _ := cmd.Flags().GetBool("all")
	accountID, _ := cmd.Flags().GetString("account-id")
	baseURL, _ := cmd.Flags().GetString("base-url")

	provider := ""
	if len(args) == 1 {
		provider = args[0]
	}

	_, err := oauth.Refresh(cmd.Context(), oauth.RefreshOptions{
		Provider:  provider,
		AccountID: accountID,
		All:       all,
		BaseURL:   baseURL,
		Stdout:    os.Stdout,
		Stderr:    os.Stderr,
	})
	return err
}
