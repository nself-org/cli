package commands

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

// ── db pgbouncer ─────────────────────────────────────────────────────

var dbPgBouncerCmd = &cobra.Command{
	Use:   "pgbouncer",
	Short: "PgBouncer connection pooler operations",
	Long: `PgBouncer connection pooler operations.

Subcommands:
  status         Show PgBouncer runtime status
  generate       Write config files to .nself/pgbouncer/
  connection-url Print the PgBouncer connection URL`,
	RunE: dataEnv(func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	}),
}

// ── db pgbouncer status ──────────────────────────────────────────────

var dbPgBouncerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show PgBouncer runtime status (SHOW STATS via psql)",
	RunE:  dataEnv(runDBPgBouncerStatus),
}

// ── db pgbouncer generate ────────────────────────────────────────────

var dbPgBouncerGenerateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Write PgBouncer config files to .nself/pgbouncer/",
	RunE:  dataEnv(runDBPgBouncerGenerate),
}

// ── db pgbouncer connection-url ──────────────────────────────────────

var dbPgBouncerConnURLCmd = &cobra.Command{
	Use:   "connection-url",
	Short: "Print the PgBouncer connection URL",
	RunE:  dataEnv(runDBPgBouncerConnURL),
}

func init() {
	dbPgBouncerCmd.AddCommand(dbPgBouncerStatusCmd, dbPgBouncerGenerateCmd, dbPgBouncerConnURLCmd)
	dbCmd.AddCommand(dbPgBouncerCmd)
}

// ── runners ──────────────────────────────────────────────────────────

func runDBPgBouncerStatus(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	setData(cmd, dbPgBouncerStatusData{Enabled: cfg.PgBouncer.Enabled})
	if !cfg.PgBouncer.Enabled {
		ui.Info("PgBouncer is not enabled. Set PGBOUNCER_ENABLED=true in your .env file.")
		return nil
	}

	status, err := database.GetPgBouncerStatus(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("pgbouncer status: %w", err)
	}

	setData(cmd, dbPgBouncerStatusData{Enabled: true, Running: status.Running, PoolMode: status.PoolMode, ActivePools: status.ActivePools, ActiveClients: status.ActiveClients, IdleServers: status.IdleServers})
	if !status.Running {
		ui.Warn("PgBouncer does not appear to be running.")
		return nil
	}

	ui.Info("PgBouncer status:")
	fmt.Printf("  Pool mode:      %s\n", status.PoolMode)
	fmt.Printf("  Active pools:   %d\n", status.ActivePools)
	fmt.Printf("  Active clients: %d\n", status.ActiveClients)
	fmt.Printf("  Idle servers:   %d\n", status.IdleServers)
	return nil
}

func runDBPgBouncerGenerate(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if err := database.WritePgBouncerConfig(cmd.Context(), cfg, dir); err != nil {
		return fmt.Errorf("pgbouncer generate: %w", err)
	}

	ui.Success("PgBouncer config files written to .nself/pgbouncer/")
	ui.Info("Files: pgbouncer.ini, pg_hba.conf, userlist.txt")
	return nil
}

func runDBPgBouncerConnURL(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	if !cfg.PgBouncer.Enabled {
		ui.Info("PgBouncer is not enabled. Set PGBOUNCER_ENABLED=true in your .env file.")
	}

	url := database.PgBouncerConnectionURL(cfg)
	fmt.Println(url)
	return nil
}
