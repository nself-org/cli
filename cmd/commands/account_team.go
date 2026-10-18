package commands

// Purpose: the "nself account show" and "nself account team" subcommands and
// their RunE. Inputs are the cobra command/args; outputs are printed team
// info or an error.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// accountShowCmd remains for backward compat. It delegates to runAccountStatus.
var accountShowCmd = &cobra.Command{
	Use:          "show",
	Short:        "Show your account details (alias for status)",
	SilenceUsage: true,
	Hidden:       true,
	RunE:         runAccountStatus,
}

// ── account team ─────────────────────────────────────────────────────────────

var accountTeamCmd = &cobra.Command{
	Use:   "team",
	Short: "Manage team members",
	Long: `Manage team members on your account.

With no flags, lists all current members.

Flags:
  --invite <email>          Send a team invite
  --remove <email>          Remove a member (prompts for confirmation)
  --role <email> <role>     Set a member role (owner|admin|member)
  --json                    Output raw JSON`,
	SilenceUsage: true,
	RunE:         runAccountTeam,
}

func init() {
	accountTeamCmd.Flags().String("invite", "", "Email address to invite")
	accountTeamCmd.Flags().String("remove", "", "Email address to remove")
	accountTeamCmd.Flags().StringSlice("role", nil, "Set role: <email> <role>")
	accountTeamCmd.Flags().Bool("json", false, "Output raw JSON")
}

func runAccountTeam(cmd *cobra.Command, _ []string) error {
	invite, _ := cmd.Flags().GetString("invite")
	remove, _ := cmd.Flags().GetString("remove")
	roleArgs, _ := cmd.Flags().GetStringSlice("role")
	jsonOut, _ := cmd.Flags().GetBool("json")
	if err := accountJSONGuard(cmd, "invite", "remove", "role"); err != nil {
		return err
	}

	af, err := requireLogin()
	if err != nil {
		return err
	}

	// --invite
	if invite != "" {
		if err := auth.InviteTeamMember(cmdCtx(cmd), af.AccessToken, invite); err != nil {
			return handleAuthError(err)
		}
		ui.Success(fmt.Sprintf("Invite sent to %s.", invite))
		return nil
	}

	// --remove
	if remove != "" {
		if !promptConfirm(fmt.Sprintf("Remove %s from your team? [y/N]: ", remove)) {
			fmt.Println("Canceled.")
			return nil
		}
		if err := auth.RemoveTeamMember(cmdCtx(cmd), af.AccessToken, remove); err != nil {
			return handleAuthError(err)
		}
		ui.Success(fmt.Sprintf("Removed %s from team.", remove))
		return nil
	}

	// --role <email> <role>
	if len(roleArgs) >= 2 {
		email := roleArgs[0]
		role := roleArgs[1]
		if err := auth.SetTeamMemberRole(cmdCtx(cmd), af.AccessToken, email, role); err != nil {
			return handleAuthError(err)
		}
		ui.Success(fmt.Sprintf("Role for %s set to %s.", email, role))
		return nil
	}

	// Default: list members.
	members, err := auth.GetTeamMembers(cmdCtx(cmd), af.AccessToken)
	if err != nil {
		return handleAuthError(err)
	}

	if jsonEnvelopeOn(cmd) {
		return emitDoc(cmd, orEmpty(members), true)
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(members)
	}

	if len(members) == 0 {
		fmt.Println("No team members. Invite someone with --invite <email>.")
		return nil
	}

	tbl := ui.NewTable("NAME", "EMAIL", "ROLE", "JOINED")
	for _, m := range members {
		joined := m.JoinedAt
		if len(joined) >= 10 {
			joined = joined[:10]
		}
		tbl.AddRow(m.Name, m.Email, m.Role, joined)
	}
	tbl.Render()
	return nil
}

// ── account licenses ─────────────────────────────────────────────────────────
