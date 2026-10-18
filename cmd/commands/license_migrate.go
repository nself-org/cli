package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// licenseMigrateCmd — SP-04.O11 T11: dual-mode license migration.
//
// Without --account-id: legacy check mode (explain what will change, show expiry).
// With --account-id <uuid>: link the legacy key to the given nSelf account.
//
// The server endpoint POST /license/migrate handles the actual DB work.
// This command is safe to run multiple times (server is idempotent).
//
// runMigrateCheck/runMigrateLink live in license_migrate_actions.go and
// their ping_api calls live in license_migrate_api.go — CLI-R12 Batch B
// mechanical file-size split.
var licenseMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate legacy license key to a nSelf account",
	Long: `Check or link a legacy nself_pro_ license key to a nSelf account.

The new model uses per-product keys (nself_claw_, nself_chat_, etc.) linked
to a nSelf account, instead of the old tier-based nself_pro_ keys.

Without --account-id: shows migration status for all configured keys.
With --account-id <uuid>: links the legacy key to that nSelf account.

Examples:
  nself license migrate                             # check status
  nself license migrate --account-id <uuid>         # link key to account
  nself license migrate --key <key> --account-id <uuid>  # link a specific key`,
	RunE: runLicenseMigrate,
}

var (
	migrateAccountID string
	migrateKeyFlag   string
)

type migrationInfo struct { //nolint:unused // kept: licence-migration path has no entry point; deleting hides the gap, see qa/bugs/declared-but-never-wired-symbols.md
	Valid              bool     `json:"valid"`
	Tier               string   `json:"tier"`
	Product            string   `json:"product"`
	KeyType            string   `json:"key_type"`
	LegacyExpiresAt    string   `json:"legacy_expires_at"`
	GrandfatheredUntil string   `json:"grandfathered_until"`
	PluginsAllowed     []string `json:"plugins_allowed"`
	MigrationStatus    string   `json:"migration_status"`
}

type migrateResponse struct {
	Migrated        bool   `json:"migrated"`
	AlreadyMigrated bool   `json:"already_migrated"`
	AccountID       string `json:"account_id"`
	Tier            string `json:"tier"`
	Error           string `json:"error"`
}

type migrationStatusResponse struct {
	Migrated            bool   `json:"migrated"`
	MigrationStatus     string `json:"migration_status"`
	MigratedToAccountID string `json:"migrated_to_account_id"`
	GraceExpiresAt      string `json:"grace_expires_at"`
	DaysRemaining       int    `json:"days_remaining"`
	CutoverEnforced     bool   `json:"cutover_enforced"`
	Error               string `json:"error"`
}

func runLicenseMigrate(cmd *cobra.Command, args []string) error {
	pingURL := os.Getenv("NSELF_PING_API_URL")
	if pingURL == "" {
		pingURL = defaultPingURL
	}

	// If --account-id provided, perform the actual account-link migration.
	doc := &licenseMigrateData{Mode: "status", Keys: []licenseMigrateKey{}}
	run := runMigrateCheck
	if migrateAccountID != "" {
		doc.Mode, run = "link", runMigrateLink
	}
	// Without --account-id, show migration status for all configured keys.
	if err := run(cmd.Context(), pingURL, doc); err != nil {
		return err
	}
	setData(cmd, *doc)
	return nil
}

func init() {
	licenseMigrateCmd.Flags().StringVar(&migrateAccountID, "account-id", "", "nSelf account UUID to link the legacy key to")
	licenseMigrateCmd.Flags().StringVar(&migrateKeyFlag, "key", "", "specific license key to migrate (defaults to auto-detected legacy key)")
	licenseMigrateCmd.Flags().Bool("dry-run", false, "Preview migration steps without applying changes")
	licenseCmd.AddCommand(licenseMigrateCmd)
}
