package commands

import (
	"regexp"

	"github.com/spf13/cobra"
)

// migrationNameAllowed matches only lowercase alphanumeric characters, underscores, and hyphens.
var migrationNameAllowed = regexp.MustCompile(`^[a-z0-9_-]+$`)

// ── Parent command ──────────────────────────────────────────────────

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database operations: migrations, backups, restore, seed, shell",
	Long: `Database operations: migrations, backups, restore, seed, shell.

Subcommands:
  migrate   Manage database migrations (up/down/status/create)
  seed      Run seed data
  backup    Create pg_dump backup
  restore   Restore from backup
  shell     Open psql interactive shell
  reset     Drop and recreate database (DESTRUCTIVE)
  hasura    Hasura metadata operations`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── migrate ─────────────────────────────────────────────────────────

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db migrate` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbMigrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long: `Apply pending migrations to the project's Postgres database.

Pass --env staging|prod (with NSELF_DEPLOY_HOST_<ENV> set, or an entry in
.nself/control-plane.yaml) to run this against a deployed target instead of
the local docker daemon: the command re-invokes 'nself db migrate up' on the
remote host over SSH, cd'ing into NSELF_REMOTE_PATH_<ENV> (default
/opt/nself) first.

Before running the remote command, the CLI proactively verifies the remote
host's nself version matches the local one (running 'nself --version' over
the same SSH connection). An older or newer remote CLI errors clearly,
naming both versions and the host, instead of silently trusting output from
a version-mismatched binary or failing with a raw SSH error. Pass
--allow-version-drift to skip this check and run anyway.`,
	RunE: runDBMigrateUp,
}

var dbMigrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Revert last migration",
	RunE:  runDBMigrateDown,
}

var dbMigrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long: `Show which migrations have been applied to the project's Postgres database.

Pass --env staging|prod to check a deployed target instead of the local
docker daemon (see 'nself db migrate up --help' for remote-targeting and
version-drift details).`,
	RunE: runDBMigrateStatus,
}

var dbMigrateCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create new migration file",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBMigrateCreate,
}

var dbMigrateApplyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Apply a specific migration file (G-008)",
	Long: `Apply a single SQL migration file by path.

The file is validated to exist before execution. If the migration is
already recorded in schema_versions (matched by filename + SHA-256
checksum), the command warns and exits cleanly without re-applying.

This closes G-008: plugin-claw external RLS migrations can be applied
via CLI without requiring 'nself db shell' as a workaround.`,
	RunE: runDBMigrateApply,
}

// ── seed ────────────────────────────────────────────────────────────

var dbSeedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Database seeding: run, list, verify, graph",
	Long: `Database seeding with environment-aware fixtures.

Subcommands:
  run      Execute seeds for current environment
  list     List available seeds and fixtures
  verify   Verify a fixture is deterministic
  graph    Show seed dependency graph

Legacy usage (backward compat):
  nself db seed [file]   — runs a single seed file directly`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDBSeed,
}

// ── backup ──────────────────────────────────────────────────────────

var dbBackupCmd = &cobra.Command{
	Use:   "backup [file]",
	Short: "Create pg_dump backup",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runDBBackup,
}

// ── restore ─────────────────────────────────────────────────────────

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <file>",
	Short: "Restore from backup",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBRestore,
}

// ── shell ───────────────────────────────────────────────────────────

var dbShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open psql interactive shell",
	Long: `Open an interactive psql shell connected to the project database.

The shell is launched inside the running Postgres Docker container via
'docker exec'. psql must be available inside the container (standard
with the official postgres image).

Windows note: on Windows the shell is proxied through 'docker exec'
so psql itself does not need to be in the host PATH. However if you
are connecting to an external Postgres instance (not managed by nself)
you must ensure psql.exe is in your PATH before running this command.`,
	RunE: runDBShell,
}

// ── list ────────────────────────────────────────────────────────────

var dbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List databases in the project Postgres instance",
	Long: `List all databases in the project's Postgres container.

Connects to the running Postgres container and prints all database names
(equivalent to \l in psql). Useful for verifying that db create/drop/reset
operated on the correct database.`,
	RunE: runDBList,
}

// ── drop ────────────────────────────────────────────────────────────

var dbDropCmd = &cobra.Command{
	Use:   "drop",
	Short: "Drop the project database (DESTRUCTIVE)",
	RunE:  runDBDrop,
}

// ── reset ───────────────────────────────────────────────────────────

var dbResetForce bool
