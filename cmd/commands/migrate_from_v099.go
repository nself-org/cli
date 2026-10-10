// migrate_from_v099.go
//
// `nself migrate from-v099` is the operator-facing entry point for the
// home-level v0.9.9 → v1.x migration shim (see internal/migrate/v099_shim.go).
//
// It is intentionally a separate top-level command (not a subcommand of
// `migrate`) because:
//   - It is OPT-IN. We do not want it ever auto-running on first install.
//   - It targets the operator's HOME directory, not any project directory,
//     and that distinction is easier to communicate with a separate command.
//   - The existing `migrate from-bash` already covers the per-project Bash-era
//     artifact path; this one is complementary, not a replacement.

package commands

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	migrate "github.com/nself-org/cli/internal/migrate"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var migrateFromV099Cmd = &cobra.Command{
	Use:   "from-v099",
	Short: "Migrate v0.9.9 home-level state (license key, channel, ssh keys) to v1.x layout",
	Long: `Detect and migrate v0.9.9 (Bash-era) home-level state to the current
v1.x layout.

The v0.9.9 CLI persisted state under ~/.nself/v099-state/. The Go CLI moved
to ~/.config/nself + ~/.nself/. This command:

  1. Scans ~/.nself/v099-state/ for recognised legacy entries
  2. Backs everything up to ~/.nself/v099-state-backup/<timestamp>/
  3. Migrates license.key → ~/.config/nself/license.json (verified=false)
  4. Migrates channel preference → ~/.config/nself/channel.json
  5. Migrates ssh keypair → ~/.nself/ssh/ (perms 0600)
  6. Renames the legacy directory to ~/.nself/v099-state.migrated.<ts>/

The command is OPT-IN. It never auto-runs. After it completes, re-validate
your license with:

    nself license verify

Examples:
  nself migrate from-v099           # Interactive: prompts before acting
  nself migrate from-v099 --yes     # Non-interactive (CI / scripted use)
  nself migrate from-v099 --check   # Detect only, do not migrate

See also: nself migrate from-bash    (per-project Bash-era artifact migration)
Wiki: https://github.com/nself-org/cli/wiki/migration/v099-to-v1x`,
	RunE: dataEnv(runMigrateFromV099),
}

func init() {
	migrateFromV099Cmd.Flags().Bool("yes", false, "Skip the confirmation prompt (non-interactive)")
	migrateFromV099Cmd.Flags().Bool("check", false, "Detect legacy state without migrating")
	migrateCmd.AddCommand(migrateFromV099Cmd)
}

func runMigrateFromV099(cmd *cobra.Command, args []string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	check, _ := cmd.Flags().GetBool("check")

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	det := migrate.DetectV099Home(home)
	if !det.Present {
		ui.Info(fmt.Sprintf("No v0.9.9 home state found at %s. Nothing to do.", det.LegacyDir))
		return nil
	}

	ui.Info(fmt.Sprintf("Found v0.9.9 home state at %s", det.LegacyDir))
	for _, e := range det.Found {
		dest := e.NewPath
		if dest == "" {
			dest = "(backup only — no v1.x destination)"
		}
		ui.Info(fmt.Sprintf("  - %s  →  %s", e.LegacyPath, dest))
		ui.Dimmed("    " + e.Action)
	}

	if check {
		ui.Info("--check supplied; not migrating.")
		return nil
	}

	if !yes {
		fmt.Print("\nProceed with migration? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.ToLower(strings.TrimSpace(line))
		if line != "y" && line != "yes" {
			ui.Info("Aborted.")
			return nil
		}
	}

	res, err := migrate.RunV099Migration(home)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	ui.Success(fmt.Sprintf("Backup written to %s", res.BackupDir))
	for _, m := range res.Migrated {
		ui.Success(fmt.Sprintf("  migrated: %s", m))
	}
	for _, b := range res.Backed {
		ui.Info(fmt.Sprintf("  backup-only: %s", b))
	}
	for _, w := range res.Warnings {
		ui.Warn("  " + w)
	}

	ui.Info("Next: re-validate your license with `nself license verify`.")
	return nil
}
