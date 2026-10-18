package commands

// Purpose: The two `nself license migrate` actions — status check and
// account-link — split out of license_migrate.go (CLI-R12 Batch B
// mechanical file-size split). runLicenseMigrate (license_migrate.go)
// dispatches to whichever of these matches the --account-id flag.
// Inputs: a context and the ping_api base URL; reads the migrateKeyFlag/
// migrateAccountID package vars set by cobra flag parsing.
// Outputs: a status table (check) or a migration confirmation (link);
// errors wrap ping_api request failures.
// Constraints: pure move, no behavior change. licenseMigrateCmd, its
// flags, and the shared response types remain in license_migrate.go.

import (
	"context"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/ui"
)

// runMigrateCheck — status report for all legacy keys (no mutation).
func runMigrateCheck(ctx context.Context, pingURL string, doc *licenseMigrateData) error {
	keys := license.CollectLicenseKeys()
	if migrateKeyFlag != "" {
		keys = []string{migrateKeyFlag}
	}

	if len(keys) == 0 {
		fmt.Println("No license keys configured.")
		fmt.Printf("\nGet a product license at %s\n", pricingURL)
		return nil
	}

	hasLegacy := false
	for _, key := range keys {
		pp := license.DetectProduct(key)
		if pp != nil && (pp.Product == "pro" || pp.Product == "enterprise" || pp.Product == "max") {
			hasLegacy = true
			break
		}
	}

	if !hasLegacy {
		fmt.Println("No legacy keys found. All your keys use the new product model.")
		return nil
	}

	fmt.Println("Legacy License Migration Status")
	fmt.Println(strings.Repeat("=", 40))
	fmt.Println()

	tbl := ui.NewTable("Key", "Tier", "Expiry", "Migration Status")

	for _, key := range keys {
		pp := license.DetectProduct(key)
		if pp == nil {
			continue
		}

		masked := license.MaskKey(key)

		if pp.Product != "pro" && pp.Product != "enterprise" && pp.Product != "max" {
			doc.Keys = append(doc.Keys, licenseMigrateKey{Key: masked, Product: pp.DisplayName, Remaining: "(new format)", Status: "N/A"})
			tbl.AddRow(masked, pp.DisplayName, "(new format)", "N/A")
			continue
		}

		// Check migration status from the server.
		status, err := fetchMigrationStatus(ctx, key, pingURL)
		if err != nil {
			msg := licenseScrub(fmt.Sprintf("Error: %v", err), key)
			doc.Keys = append(doc.Keys, licenseMigrateKey{Key: masked, Product: pp.DisplayName, Remaining: "?", Status: msg})
			tbl.AddRow(masked, pp.DisplayName, "?", msg)
			continue
		}

		if status.Error != "" {
			msg := licenseScrub(fmt.Sprintf("Error: %s", status.Error), key)
			doc.Keys = append(doc.Keys, licenseMigrateKey{Key: masked, Product: pp.DisplayName, Remaining: "?", Status: msg})
			tbl.AddRow(masked, pp.DisplayName, "?", msg)
			continue
		}

		migStatus := status.MigrationStatus
		if migStatus == "" {
			migStatus = "legacy"
		}

		daysStr := "-"
		if status.DaysRemaining > 0 {
			daysStr = fmt.Sprintf("%d days", status.DaysRemaining)
		}

		doc.Keys = append(doc.Keys, licenseMigrateKey{Key: masked, Product: pp.DisplayName, Remaining: daysStr, Status: licenseScrub(migStatus, key)})
		tbl.AddRow(masked, pp.DisplayName, daysStr, licenseScrub(migStatus, key))
	}

	tbl.Render()

	fmt.Println()
	fmt.Println("To link a legacy key to your nSelf account, run:")
	fmt.Println("  nself license migrate --account-id <your-account-uuid>")
	fmt.Println()
	fmt.Printf("  View pricing and subscribe: %s\n", pricingURL)
	fmt.Println()

	return nil
}

// runMigrateLink — link the legacy key to a nSelf account via POST /license/migrate.
func runMigrateLink(ctx context.Context, pingURL string, doc *licenseMigrateData) error {
	// Determine which key to migrate.
	var keyToMigrate string

	if migrateKeyFlag != "" {
		keyToMigrate = migrateKeyFlag
	} else {
		keys := license.CollectLicenseKeys()
		for _, k := range keys {
			pp := license.DetectProduct(k)
			if pp != nil && (pp.Product == "pro" || pp.Product == "enterprise" || pp.Product == "max") {
				keyToMigrate = k
				break
			}
		}
		if keyToMigrate == "" && len(keys) > 0 {
			// If no legacy key found, use the first key (might be a legacy format without prefix).
			keyToMigrate = keys[0]
		}
	}

	if keyToMigrate == "" {
		return fmt.Errorf("no license key found to migrate; configure one with: nself license set <key>")
	}

	masked := license.MaskKey(keyToMigrate)
	fmt.Printf("Linking key %s to account %s ...\n", masked, migrateAccountID)

	resp, err := sendMigrateRequest(ctx, keyToMigrate, migrateAccountID, pingURL)
	if err != nil {
		return fmt.Errorf("migration request failed: %s", licenseScrub(err.Error(), keyToMigrate))
	}

	if resp.Error != "" {
		return fmt.Errorf("server error: %s", licenseScrub(resp.Error, keyToMigrate))
	}

	doc.Link = &licenseMigrateLink{Key: masked, AccountID: licenseScrub(resp.AccountID, keyToMigrate), Tier: licenseScrub(resp.Tier, keyToMigrate), AlreadyLinked: resp.AlreadyMigrated}
	if resp.AlreadyMigrated {
		fmt.Printf("Key %s is already linked to account %s (tier: %s).\n",
			masked, resp.AccountID, resp.Tier)
		return nil
	}

	if resp.Migrated {
		fmt.Printf("Migration complete.\n")
		fmt.Printf("  Key:       %s\n", masked)
		fmt.Printf("  Account:   %s\n", resp.AccountID)
		fmt.Printf("  Tier:      %s\n", resp.Tier)
		fmt.Println()
		fmt.Println("Your key is now linked to your nSelf account.")
		fmt.Println("Run 'nself license status' to verify.")
		return nil
	}

	return fmt.Errorf("migration returned unexpected response (migrated=false, no error)")
}
