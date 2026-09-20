package commands

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nself-org/cli/internal/backup/pitr"
	"github.com/spf13/cobra"
)

// ── nself pitr ────────────────────────────────────────────────────────────────

var pitrCmd = &cobra.Command{
	Use:   "pitr",
	Short: "Point-in-time recovery: enable, disable, status, base-backup, restore",
	Long: `Point-in-time recovery (PITR) via WAL archiving.

PITR archives PostgreSQL WAL segments continuously so that any second within
the retention window can be used as a restore target. A daily base backup
(pg_basebackup) is taken automatically when configured.

Subcommands:
  enable       Configure WAL archiving and start PITR
  disable      Stop WAL archiving
  status       Show retention window and latest WAL timestamp
  base-backup  Take a manual base backup immediately
  restore      Restore the database to a specific point in time`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself backup pitr` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── nself pitr enable ─────────────────────────────────────────────────────────

var pitrEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Configure WAL archiving and enable PITR",
	Long: `Configure PostgreSQL for continuous WAL archiving.

Writes a postgresql.conf snippet to .nself/pitr/postgresql.conf.d/pitr.conf
that sets wal_level=logical, archive_mode=on, and archive_command pointing to
the destination. Mount this file into your PostgreSQL container to activate.

Example:
  nself pitr enable --to s3://my-bucket/pitr`,
	RunE: runPITREnable,
}

// ── nself pitr disable ────────────────────────────────────────────────────────

var pitrDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Stop WAL archiving (removes PITR config snippet)",
	RunE:  runPITRDisable,
}

// ── nself pitr status ─────────────────────────────────────────────────────────

var pitrStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show PITR retention window, segment count, and latest WAL timestamp",
	RunE:  runPITRStatus,
}

// ── nself pitr base-backup ───────────────────────────────────────────────────

var pitrBaseBackupCmd = &cobra.Command{
	Use:   "base-backup",
	Short: "Take a manual pg_basebackup snapshot now",
	RunE:  runPITRBaseBackup,
}

// ── nself pitr restore ────────────────────────────────────────────────────────

var pitrRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore database to a specific point in time",
	Long: `Restore the PostgreSQL database to the state it held at a given time.

The --to flag accepts an RFC3339 timestamp or a relative duration:
  nself pitr restore --to 2026-04-22T15:30:00Z
  nself pitr restore --to "30 minutes ago"
  nself pitr restore --to "2 hours ago"

The command locates the latest base backup before the target time, downloads
it, writes recovery configuration, restarts Postgres in recovery mode, and
waits for replay to complete.`,
	RunE: runPITRRestore,
}

// ── init ──────────────────────────────────────────────────────────────────────

func init() {
	// pitr enable flags
	pitrEnableCmd.Flags().String("to", "", "Destination URL for WAL archives (e.g. s3://bucket/pitr)")
	pitrEnableCmd.Flags().String("encrypt-recipient", "", "age public key to encrypt WAL segments")
	pitrEnableCmd.Flags().Int("retention-days", 7, "Number of days to retain WAL segments")
	pitrEnableCmd.Flags().Int("wal-timeout", 60, "archive_timeout in seconds (flush WAL segment every N seconds)")

	// pitr base-backup flags
	pitrBaseBackupCmd.Flags().String("encrypt-recipient", "", "age public key to encrypt the base backup")

	// pitr restore flags
	pitrRestoreCmd.Flags().String("to", "", "Target time in RFC3339 or relative form (required)")
	pitrRestoreCmd.Flags().String("identity", "", "Path to age identity file for decrypting encrypted archives")
	pitrRestoreCmd.Flags().Bool("auto-promote", true, "Automatically promote Postgres after recovery")
	if err := pitrRestoreCmd.MarkFlagRequired("to"); err != nil {
		// Programming error: MarkFlagRequired only returns an error when the named
		// flag does not exist. Since "--to" is registered on the line above,
		// this fires only if this code is misedited. Bug-in-our-code guard.
		log.Fatalf("pitr restore: mark --to required: %v — this is a code bug, not a config error", err)
	}

	// Register subcommands.
	pitrCmd.AddCommand(pitrEnableCmd)
	pitrCmd.AddCommand(pitrDisableCmd)
	pitrCmd.AddCommand(pitrStatusCmd)
	pitrCmd.AddCommand(pitrBaseBackupCmd)
	pitrCmd.AddCommand(pitrRestoreCmd)

	// Register top-level command.
	backupCmd.AddCommand(pitrCmd)
}

// ── run functions ─────────────────────────────────────────────────────────────

func runPITREnable(cmd *cobra.Command, _ []string) error {
	destination, _ := cmd.Flags().GetString("to")
	encRecipient, _ := cmd.Flags().GetString("encrypt-recipient")
	retentionDays, _ := cmd.Flags().GetInt("retention-days")
	walTimeout, _ := cmd.Flags().GetInt("wal-timeout")

	if destination == "" {
		return fmt.Errorf("--to is required: specify a destination URL (e.g. s3://bucket/pitr)")
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	pitrCfg := pitr.EnableConfig{
		Destination:       destination,
		EncryptRecipient:  encRecipient,
		RetentionDays:     retentionDays,
		WALArchiveTimeout: walTimeout,
	}

	if err := pitr.Enable(cfg, pitrCfg); err != nil {
		return fmt.Errorf("pitr enable: %w", err)
	}

	fmt.Println("PITR configuration written. Mount .nself/pitr/postgresql.conf.d/pitr.conf into your PostgreSQL container to activate WAL archiving.")
	fmt.Printf("Destination:     %s\n", destination)
	fmt.Printf("Retention:       %d days\n", retentionDays)
	fmt.Printf("WAL timeout:     %ds\n", walTimeout)
	if encRecipient != "" {
		fmt.Printf("Encryption:      enabled (age)\n")
	}
	return nil
}

func runPITRDisable(_ *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	if err := pitr.Disable(cfg); err != nil {
		return fmt.Errorf("pitr disable: %w", err)
	}
	fmt.Println("PITR disabled. Remove or rebuild the PostgreSQL container to deactivate WAL archiving.")
	return nil
}

func runPITRStatus(_ *cobra.Command, _ []string) error {
	catalogPath, err := pitr.CatalogPath()
	if err != nil {
		return err
	}

	catalog, err := pitr.LoadCatalog(catalogPath)
	if err != nil {
		return fmt.Errorf("pitr status: %w", err)
	}

	oldest, hasOldest := catalog.OldestRecoverableTime()
	newest, hasNewest := catalog.NewestWALTime()

	if !hasOldest && !hasNewest {
		fmt.Println("PITR: no catalog data found. Run 'nself pitr enable' to start WAL archiving.")
		return nil
	}

	fmt.Printf("Base backups:    %d\n", len(catalog.BaseBackups))
	fmt.Printf("WAL segments:    %d\n", len(catalog.WALSegments))
	if hasOldest {
		fmt.Printf("Oldest restore:  %s\n", oldest.Format(time.RFC3339))
	}
	if hasNewest {
		fmt.Printf("Latest WAL:      %s\n", newest.Format(time.RFC3339))
	}
	return nil
}

func runPITRBaseBackup(cmd *cobra.Command, _ []string) error {
	encRecipient, _ := cmd.Flags().GetString("encrypt-recipient")

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	catalogPath, err := pitr.CatalogPath()
	if err != nil {
		return err
	}

	catalog, err := pitr.LoadCatalog(catalogPath)
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	destDir := ".nself/pitr/base"
	archivePath, ts, err := pitr.TakeBaseBackup(cmd.Context(), cfg, destDir, encRecipient)
	if err != nil {
		return fmt.Errorf("base backup: %w", err)
	}

	bb := pitr.BaseBackup{
		Timestamp: ts,
		RemoteKey: archivePath,
	}
	if err := catalog.AddBaseBackup(catalogPath, bb); err != nil {
		return fmt.Errorf("update catalog: %w", err)
	}

	fmt.Printf("Base backup completed: %s\n", archivePath)
	fmt.Printf("Timestamp: %s\n", ts.Format(time.RFC3339))
	return nil
}

func runPITRRestore(cmd *cobra.Command, _ []string) error {
	// PITR restore is experimental — the remote backup fetch and data-directory
	// replacement are not yet fully implemented. Gate behind an explicit env flag
	// so users do not accidentally run a no-op restore in a disaster scenario.
	if os.Getenv("NSELF_PITR_EXPERIMENTAL") != "1" {
		return fmt.Errorf(
			"nself pitr restore is not yet production-ready: remote backup fetch " +
				"and data-directory replacement are still being implemented; " +
				"set NSELF_PITR_EXPERIMENTAL=1 to run anyway (at your own risk)",
		)
	}

	toStr, _ := cmd.Flags().GetString("to")
	identityPath, _ := cmd.Flags().GetString("identity")
	autoPromote, _ := cmd.Flags().GetBool("auto-promote")

	targetTime, err := pitr.ParseTargetTime(toStr)
	if err != nil {
		return fmt.Errorf("parse target time: %w", err)
	}

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	opts := pitr.RestoreOptions{
		TargetTime:   targetTime,
		IdentityPath: identityPath,
		AutoPromote:  autoPromote,
	}

	fmt.Printf("Starting PITR restore to %s...\n", targetTime.Format(time.RFC3339))
	if err := pitr.Restore(cmd.Context(), cfg, opts); err != nil {
		return fmt.Errorf("pitr restore: %w", err)
	}
	return nil
}
