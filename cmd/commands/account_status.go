package commands

// Purpose: the "nself account status" subcommand plus the cached-account
// printers (printCachedAccount, printCachedAccountJSON). Inputs are the cobra
// command/args and the loaded *auth.AuthFile; outputs are printed text/JSON.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// accountReloginMsg is the one message every account command shows when the
// auth server rejects the stored device token (401).
const accountReloginMsg = "the auth server rejected your saved login (revoked, or its signing key rotated) — run: nself login --force"

// errAccountRelogin returns the re-login error for a rejected device token.
func errAccountRelogin() error { return errors.New(accountReloginMsg) }

var accountStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current account summary",
	Long: `Show current account summary.

Displays email, tier, license count, and registered devices.
Use --json for raw JSON output.`,
	SilenceUsage: true,
	RunE:         runAccountStatus,
}

func init() {
	accountStatusCmd.Flags().Bool("json", false, "Output raw JSON")
}

func runAccountStatus(cmd *cobra.Command, _ []string) error {
	jsonOut, _ := cmd.Flags().GetBool("json")

	af, err := auth.ReadAuthFile()
	if err != nil {
		if err == auth.ErrNotLoggedIn {
			fmt.Println("not logged in — run: nself account login")
			return nil
		}
		return fmt.Errorf("account status: %w", err)
	}

	info, infoErr := auth.GetSession(cmdCtx(cmd), af.AccessToken)
	if errors.Is(infoErr, auth.ErrNotLoggedIn) {
		// 401: the server answered and rejected the token. Cached info would
		// hide that, so fail with the one re-login message (D-0480).
		return errAccountRelogin()
	}
	if infoErr != nil {
		// Fall back to cached data.
		if jsonOut {
			return printCachedAccountJSON(af)
		}
		ui.Warn("Could not reach auth server — showing cached account info.")
		printCachedAccount(af)
		return nil
	}

	if !info.Authenticated {
		fmt.Println("session expired — run: nself account login")
		return nil
	}

	acc := info.Account
	tier := acc.Tier
	if tier == "" {
		tier = "free"
	}

	if jsonOut {
		out := map[string]interface{}{
			"email":          acc.Email,
			"display_name":   acc.DisplayName,
			"tier":           tier,
			"email_verified": acc.EmailVerified,
			"mfa_enabled":    acc.MFAEnabled,
			"bundles":        af.Bundles,
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("Account:   %s\n", acc.Email)
	if acc.DisplayName != "" {
		fmt.Printf("Name:      %s\n", acc.DisplayName)
	}
	fmt.Printf("Tier:      %s\n", tier)

	verified := "no"
	if acc.EmailVerified {
		verified = "yes"
	}
	fmt.Printf("Verified:  %s\n", verified)

	mfa := "disabled"
	if acc.MFAEnabled {
		mfa = "enabled"
	}
	fmt.Printf("MFA:       %s\n", mfa)

	if len(af.Bundles) > 0 {
		fmt.Printf("Bundles:   %s\n", strings.Join(af.Bundles, ", "))
	}
	if af.ExpiresAt != "" {
		fmt.Printf("Expires:   %s\n", af.ExpiresAt)
	}

	return nil
}

// printCachedAccount prints account info from the local auth file (no network).
func printCachedAccount(af *auth.AuthFile) {
	tier := af.Tier
	if tier == "" {
		tier = "free"
	}
	fmt.Printf("Account:   %s (cached)\n", af.Email)
	if af.DisplayName != "" {
		fmt.Printf("Name:      %s\n", af.DisplayName)
	}
	fmt.Printf("Tier:      %s\n", tier)
	if len(af.Bundles) > 0 {
		fmt.Printf("Bundles:   %s\n", strings.Join(af.Bundles, ", "))
	}
}

func printCachedAccountJSON(af *auth.AuthFile) error {
	tier := af.Tier
	if tier == "" {
		tier = "free"
	}
	out := map[string]interface{}{
		"email":        af.Email,
		"display_name": af.DisplayName,
		"tier":         tier,
		"bundles":      af.Bundles,
		"cached":       true,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// ── account show (legacy alias — same as status) ─────────────────────────────
