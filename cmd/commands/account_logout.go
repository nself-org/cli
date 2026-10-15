package commands

// Purpose: the "nself account logout" subcommand and its RunE. Inputs are the
// cobra command/args; outputs are a cleared auth file or an error.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"errors"
	"fmt"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var accountLogoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Log out of your nSelf account",
	Long: `Log out of your nSelf account.

Revokes the current session on the auth server and deletes the local
credentials file at ~/.nself/auth.json.

Use --all to revoke every active session (useful when a device is lost).`,
	SilenceUsage: true,
	RunE:         runAccountLogout,
}

func init() {
	accountLogoutCmd.Flags().Bool("all", false, "Revoke all active sessions, not just the current one")
}

func runAccountLogout(cmd *cobra.Command, _ []string) error {
	all, _ := cmd.Flags().GetBool("all")

	af, err := auth.ReadAuthFile()
	if err != nil {
		if err == auth.ErrNotLoggedIn {
			fmt.Println("Not logged in.")
			return nil
		}
		return fmt.Errorf("logout: %w", err)
	}

	// Revoke on server (best-effort — still delete local file on failure).
	if revokeErr := auth.RevokeSession(cmdCtx(cmd), af.AccessToken, all); revokeErr != nil {
		if errors.Is(revokeErr, auth.ErrNotLoggedIn) {
			ui.Warn("session already invalid on the server")
		} else {
			ui.Warn(fmt.Sprintf("Could not revoke session on server: %v", revokeErr))
		}
		ui.Warn("Deleting local credentials anyway.")
	}

	if err := auth.DeleteAuthFile(); err != nil {
		return fmt.Errorf("deleting credentials: %w", err)
	}

	if all {
		ui.Success("Logged out. All sessions revoked.")
	} else {
		ui.Success("Logged out. Session revoked.")
	}
	return nil
}

// ── account status ───────────────────────────────────────────────────────────
