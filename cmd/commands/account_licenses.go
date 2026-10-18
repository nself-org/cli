package commands

// Purpose: the "nself account licenses" subcommand and its RunE. Inputs are
// the cobra command/args; outputs are printed license info or an error.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var accountLicensesCmd = &cobra.Command{
	Use:   "licenses",
	Short: "List and manage account licenses",
	Long: `List and manage licenses linked to your account.

With no flags, displays all licenses in a table.

Flags:
  --activate <id>   Activate a license key on this device
  --json            Output raw JSON`,
	SilenceUsage: true,
	RunE:         runAccountLicenses,
}

func init() {
	accountLicensesCmd.Flags().String("activate", "", "License ID to activate on this device")
	accountLicensesCmd.Flags().Bool("json", false, "Output raw JSON")
}

func runAccountLicenses(cmd *cobra.Command, _ []string) error {
	activate, _ := cmd.Flags().GetString("activate")
	jsonOut, _ := cmd.Flags().GetBool("json")
	if err := accountJSONGuard(cmd, "activate"); err != nil {
		return err
	}

	af, err := requireLogin()
	if err != nil {
		return err
	}

	// --activate
	if activate != "" {
		if !promptConfirm(fmt.Sprintf("Activate license %s on this device? [y/N]: ", activate)) {
			fmt.Println("Canceled.")
			return nil
		}
		if err := auth.ActivateLicense(cmdCtx(cmd), af.AccessToken, activate); err != nil {
			return handleAuthError(err)
		}
		ui.Success(fmt.Sprintf("License %s activated.", activate))
		return nil
	}

	// Default: list licenses.
	licenses, err := auth.GetLicenses(cmdCtx(cmd), af.AccessToken)
	if err != nil {
		return handleAuthError(err)
	}

	if jsonEnvelopeOn(cmd) {
		return emitDoc(cmd, accountLicenseRows(licenses), true)
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(licenses)
	}

	if len(licenses) == 0 {
		fmt.Println("No licenses linked to your account.")
		fmt.Printf("Activate a license at https://nself.org/account/licenses or via 'nself license set <key>'.\n")
		return nil
	}

	tbl := ui.NewTable("KEY PREFIX", "TIER", "STATUS", "EXPIRES")
	for _, lic := range licenses {
		prefix := lic.ID
		if len(prefix) > 12 {
			prefix = prefix[:12] + "..."
		}
		status := lic.Tier
		if !lic.IsActive {
			status = lic.Tier + " (inactive)"
		}
		expires := lic.ExpiresAt
		if expires == "" {
			expires = "never"
		}
		tbl.AddRow(prefix, lic.Tier, status, expires)
	}
	tbl.Render()
	return nil
}

// ── account devices ───────────────────────────────────────────────────────────
