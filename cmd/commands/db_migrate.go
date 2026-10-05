package commands

// Purpose: RunE implementations for "nself db migrate up/down/status/create/
// apply". Inputs are the cobra command/args; outputs are migration results
// printed to the user or an error.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/database"

	"github.com/spf13/cobra"
)

func runDBMigrateUp(cmd *cobra.Command, _ []string) error {
	// Read --migration-dir and --dry-run before the remote dispatch:
	// with --env/--server the command re-runs on the remote host, and dropping
	// --dry-run there would apply for real (P7-PROD-77 for --migration-dir,
	// P7-PROD-84 for the default directory). --dry-run is always forwarded;
	// runRemoteNselfCommand then refuses unless the remote proves it supports
	// it (db_remote_dryrun.go), also under --allow-version-drift.
	migrationDir, _ := cmd.Flags().GetString("migration-dir")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	remoteArgs := []string{"db", "migrate", "up"}
	if migrationDir != "" {
		remoteArgs = append(remoteArgs, "--migration-dir", migrationDir)
	}
	if dryRun {
		remoteArgs = append(remoteArgs, "--dry-run")
	}
	if handled, err := dispatchRemoteIfNeeded(cmd, remoteArgs...); handled {
		return err
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	// Verify PostgreSQL is running before attempting any migration.
	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	checkCmd := exec.CommandContext(cmd.Context(), "docker", "exec", container,
		"pg_isready", "-U", user)
	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("PostgreSQL is not running — start with 'nself start' first")
	}

	plugin, _ := cmd.Flags().GetString("plugin")

	// --migration-dir: apply all .sql files in the given directory (G-008).
	// With --dry-run only list the pending files: that path issues SELECT
	// statements only and never reaches MigrateUpDir (P7-PROD-77).
	if migrationDir != "" && dryRun {
		pending, err := database.PendingDirMigrations(cmd.Context(), cfg, migrationDir)
		if err != nil {
			return fmt.Errorf("pending migrations in dir: %w", err)
		}
		if len(pending) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No pending migrations in directory.")
			return nil
		}
		for _, name := range pending {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), name)
		}
		return nil
	}
	if migrationDir != "" {
		count, err := database.MigrateUpDir(cmd.Context(), cfg, migrationDir)
		if err != nil {
			return fmt.Errorf("migrate up dir: %w", err)
		}
		if count > 0 {
			fmt.Printf("Applied %d migration(s) from %s.\n", count, migrationDir)
		} else {
			fmt.Println("No pending migrations in directory.")
		}
		return nil
	}

	// Default directory dry-run: PendingMigrations issues SELECTs only and
	// creates nothing (P7-PROD-84); it never reaches MigrateUp.
	if dryRun {
		pending, err := database.PendingMigrations(cmd.Context(), cfg, plugin)
		if err != nil {
			return fmt.Errorf("pending migrations: %w", err)
		}
		if len(pending) == 0 {
			fmt.Println("No pending migrations.")
			return nil
		}
		for _, name := range pending {
			fmt.Println(name)
		}
		return nil
	}

	count, err := database.MigrateUp(cmd.Context(), cfg, plugin)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	if count > 0 {
		fmt.Printf("Applied %d migration(s).\n", count)
	} else {
		fmt.Println("No pending migrations.")
	}
	return nil
}

func runDBMigrateDown(cmd *cobra.Command, _ []string) error {
	migrationDir, _ := cmd.Flags().GetString("migration-dir")
	if migrationDir == "" && cmd.Flags().Changed("steps") {
		// Never silently revert one migration when the user asked for N.
		return fmt.Errorf("--steps requires --migration-dir")
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	if migrationDir != "" {
		steps, _ := cmd.Flags().GetInt("steps")
		reverted, err := database.MigrateDownDir(cmd.Context(), cfg, migrationDir, steps)
		for _, name := range reverted {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Reverted %s\n", name)
		}
		if err != nil {
			return fmt.Errorf("migrate down dir: %w", err)
		}
		return nil
	}
	if err := database.MigrateDown(cmd.Context(), cfg); err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	fmt.Println("Last migration reverted.")
	return nil
}

func runDBMigrateStatus(cmd *cobra.Command, _ []string) error {
	migrationDir := ""
	if f := cmd.Flags().Lookup("migration-dir"); f != nil {
		migrationDir = f.Value.String()
	}
	detect, _ := cmd.Flags().GetBool("detect")

	// Forward --migration-dir/--detect to the remote CLI: the pre-flight
	// version-drift check (#162) guarantees the remote binary understands
	// the flags.
	remoteArgs := []string{"db", "migrate", "status"}
	if migrationDir != "" {
		remoteArgs = append(remoteArgs, "--migration-dir", migrationDir)
	}
	if detect {
		remoteArgs = append(remoteArgs, "--detect")
	}
	if handled, err := dispatchRemoteIfNeeded(cmd, remoteArgs...); handled {
		return err
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	// --detect classifies each pending migration against the live schema
	// (BASELINE/APPLY/CONFLICT) instead of only checking the ledger — see
	// db_migrate_detect.go. It never writes anything.
	if detect {
		return runDBMigrateStatusDetect(cmd, cfg, migrationDir)
	}

	statuses, err := database.MigrateStatus(cmd.Context(), cfg, migrationDir)
	if err != nil {
		return fmt.Errorf("migrate status: %w", err)
	}
	if len(statuses) == 0 {
		fmt.Println("No migrations found.")
		return nil
	}
	fmt.Printf("%-40s %-10s %s\n", "MIGRATION", "STATUS", "APPLIED AT")
	for _, s := range statuses {
		status := "pending"
		appliedAt := ""
		if s.Applied {
			status = "applied"
			if !s.Timestamp.IsZero() {
				appliedAt = s.Timestamp.Format(time.RFC3339)
			}
		}
		fmt.Printf("%-40s %-10s %s\n", s.Name, status, appliedAt)
	}
	return nil
}

func runDBMigrateCreate(_ *cobra.Command, args []string) error {
	name := args[0]
	if name == "" {
		return fmt.Errorf("migration name is required")
	}
	// Allow only lowercase alphanumeric characters, underscores, and hyphens.
	// This prevents path traversal and other filesystem attacks.
	if !migrationNameAllowed.MatchString(name) {
		return fmt.Errorf("migration name %q contains invalid characters: only lowercase letters, digits, underscores, and hyphens are allowed", name)
	}

	ts := time.Now().Format("20060102150405")

	dir := "migrations"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create migrations directory: %w", err)
	}

	upFile := filepath.Join(dir, fmt.Sprintf("%s_%s.sql", ts, name))
	downFile := filepath.Join(dir, fmt.Sprintf("%s_%s.down.sql", ts, name))

	if err := os.WriteFile(upFile, []byte("-- migrate up\n"), 0o644); err != nil {
		return fmt.Errorf("create up migration: %w", err)
	}
	if err := os.WriteFile(downFile, []byte("-- migrate down\n"), 0o644); err != nil {
		return fmt.Errorf("create down migration: %w", err)
	}

	fmt.Printf("Created migration files:\n  %s\n  %s\n", upFile, downFile)
	return nil
}

// runDBMigrateApply implements 'nself db migrate apply --file <path>' (G-008).
// It applies a single SQL migration file and records it in schema_versions by
// filename + SHA-256 checksum. If the migration is already recorded, it warns
// and exits cleanly without re-applying (double-apply protection).
func runDBMigrateApply(cmd *cobra.Command, _ []string) error {
	filePath, _ := cmd.Flags().GetString("file")
	if filePath == "" {
		return fmt.Errorf("--file is required")
	}

	// Validate the file exists before loading config or touching the database.
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("migration file not found: %s", filePath)
		}
		return fmt.Errorf("stat migration file: %w", err)
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	skipped, err := database.ApplyFile(cmd.Context(), cfg, filePath)
	if err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	if skipped {
		fmt.Printf("already applied: %s (skipped)\n", filepath.Base(filePath))
	} else {
		fmt.Printf("Applied: %s\n", filepath.Base(filePath))
	}
	return nil
}
