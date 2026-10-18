package commands

// Purpose: License key mutation commands split out of license.go (CLI-R12
// Batch B mechanical file-size split). Holds `nself license set/add/remove/
// clear` — every subcommand that adds, replaces, or deletes stored keys.
// Inputs: cobra command args (the key value(s) or a key-prefix/product query).
// Outputs: stdout confirmation messages; errors wrap the underlying license
// package failures.
// Constraints: entitlement refresh is v1.5-only. defaultPingURL/pricingURL
// consts and the licenseCmd/init() registration remain in license.go.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var licenseSetCmd = &cobra.Command{
	Use:   "set <key>",
	Short: "Replace all keys with a single key",
	Long: `Save a license key, replacing any existing keys.

If multiple keys are currently configured, they will all be replaced with the
new key. A warning is printed when replacing multiple keys. Use 'nself license add'
to add a key without replacing existing ones.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		replaced, err := license.SetKeyReplaceAll(key)
		if err != nil {
			return fmt.Errorf("setting license key: %w", err)
		}
		if replaced > 1 {
			fmt.Fprintf(os.Stderr, "Warning: Replacing %d existing keys with single key.\n", replaced)
		}
		if cwd, err := os.Getwd(); err == nil {
			var validationErr error
			if projectDir, err := config.FindNSelfRoot(cwd); err == nil {
				validationErr = entitlement.RefreshInstalledForKey(cmd.Context(), projectDir, key)
			} else {
				validationErr = entitlement.ValidateKeyOutsideProject(cmd.Context(), key)
			}
			if errors.Is(validationErr, entitlement.ErrValidationUnavailable) {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: key saved; could not validate online, the next build will retry.")
			} else if validationErr != nil {
				return validationErr
			}
		}
		pp := license.DetectProduct(key)
		setData(cmd, licenseSetData{Key: license.MaskKey(key), Product: licenseProductName(pp, ""), Replaced: replaced})
		if pp != nil {
			fmt.Printf("%s license key saved.\n", pp.DisplayName)
		} else {
			fmt.Println("License key saved.")
		}
		return nil
	},
}

var licenseAddCmd = &cobra.Command{
	Use:   "add <key> [key...]",
	Short: "Add one or more license keys",
	Long: `Add license keys without replacing existing ones.

Each key is validated for format and verified against ping.nself.org before
being saved. Supports up to 10 keys total.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pingURL := os.Getenv("NSELF_PING_API_URL")
		if pingURL == "" {
			pingURL = defaultPingURL
		}

		added := []licenseKeyRef{}
		for _, key := range args {
			masked := license.MaskKey(key)

			if err := license.ValidateKeyFormat(key); err != nil {
				return fmt.Errorf("invalid key %q: %w", masked, err)
			}

			// Validate against server with spinner feedback.
			sp := ui.NewSpinner(fmt.Sprintf("Validating key %s ...", masked))
			sp.Start()
			ctx, cancel := context.WithTimeout(cmd.Context(), 30*time.Second)
			valid, err := plugin.ValidateLicenseRemote(ctx, key, pingURL)
			cancel()
			if err != nil {
				sp.Fail(fmt.Sprintf("Validation error for %s: %v", masked, err))
				return fmt.Errorf("validating key: %w", err)
			}
			if !valid {
				sp.Fail(fmt.Sprintf("Key %s is invalid or expired", masked))
				return fmt.Errorf("key %s is invalid or expired", masked)
			}
			sp.Success(fmt.Sprintf("Key %s validated", masked))

			if err := license.AddKey(key); err != nil {
				return fmt.Errorf("adding key: %w", err)
			}

			pp := license.DetectProduct(key)
			added = append(added, licenseKeyRef{Key: masked, Product: licenseProductName(pp, "Unknown")})
			if pp != nil {
				ui.Success(fmt.Sprintf("%s license activated.", pp.DisplayName))
				fmt.Printf("  %s Install plugins: %s\n",
					ui.C(ui.Blue, ui.IconArrow),
					ui.C(ui.Cyan, "nself plugin list --pro"),
				)
			} else {
				ui.Success(fmt.Sprintf("License key %s added.", masked))
			}
		}
		setData(cmd, licenseAddData{Added: added})
		return nil
	},
}

var licenseRemoveCmd = &cobra.Command{
	Use:   "remove <key-or-product>",
	Short: "Remove a license key by value or product name",
	Long: `Remove a license key by key prefix or product name.

Examples:
  nself license remove nself_claw_xxx   Remove by key prefix
  nself license remove claw             Remove all claw keys
  nself license remove chat             Remove all chat keys`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := args[0]
		removed, err := license.RemoveKey(query)
		if err != nil {
			return err
		}
		setData(cmd, licenseRemoveData{Removed: removed})
		if removed == 1 {
			fmt.Println("License key removed.")
		} else {
			fmt.Printf("%d license keys removed.\n", removed)
		}
		return nil
	},
}

var licenseClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove all saved license keys",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := license.ClearKey(); err != nil {
			return fmt.Errorf("clearing license key: %w", err)
		}
		fmt.Println("All license keys removed.")
		return nil
	},
}

// uniqueStrings returns a deduplicated copy of the input slice preserving order.
func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
