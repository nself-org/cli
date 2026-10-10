package commands

// Purpose: `nself db migrate baseline <name...>` — record one or more
// on-disk migrations as applied WITHOUT executing their SQL. This is the
// verb cli#386 was missing: a populated database (built by dump/console/hand
// SQL) has no way to adopt the CLI's migration ledger except by hand-editing
// schema_versions, which the nSelf-First doctrine forbids.
// Inputs: cobra command/args (migration names) plus --yes/--dry-run/
// --migration-dir flags.
// Outputs: printed plan + (unless --dry-run) ledger rows written via
// database.BaselineMigration; a non-nil error on any failure or refusal.
// Constraints: baseline is destructive-adjacent — it makes the CLI believe
// SQL ran that never ran. It must never run as a side effect of any other
// command, must always print exactly what it will record, and must refuse
// without --yes or an interactive "yes" (see baselineConfirmed).

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"

	"github.com/spf13/cobra"
)

var dbMigrateBaselineCmd = &cobra.Command{
	Use:   "baseline <name> [name...]",
	Short: "Record migration(s) as applied WITHOUT running their SQL",
	Long: `Record one or more migrations in the schema_versions / nself_ops.migrations
ledger as already applied, without executing the migration's SQL.

Use this only when the migration's objects already exist in the database by
some other means (a pg_dump restore, console-created tables, hand-written
SQL) — the opposite case is 'nself db migrate apply', which runs the SQL.

Run 'nself db migrate status --detect' first to see which pending migrations
are safe BASELINE candidates (every object they create already exists).

Always prints exactly what would be recorded. Requires --yes (or an
interactive "yes" confirmation) to actually write; --dry-run never writes
anything, with or without --yes.`,
	Args: cobra.MinimumNArgs(1),
	RunE: dataEnv(runDBMigrateBaseline),
}

func init() {
	dbMigrateBaselineCmd.Flags().Bool("yes", false, "Skip interactive confirmation (required for non-interactive/CI use)")
	dbMigrateBaselineCmd.Flags().Bool("dry-run", false, "Print what would be recorded without writing anything")
	dbMigrateBaselineCmd.Flags().String("migration-dir", "", "Resolve migration names against this directory instead of the auto-detected one")
	dbMigrateCmd.AddCommand(dbMigrateBaselineCmd)
}

func runDBMigrateBaseline(cmd *cobra.Command, args []string) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	yesFlag, _ := cmd.Flags().GetBool("yes")
	migrationDir, _ := cmd.Flags().GetString("migration-dir")

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	plans, err := database.PlanBaseline(cmd.Context(), cfg, migrationDir, args)
	if err != nil {
		return fmt.Errorf("plan baseline: %w", err)
	}

	pending := pendingBaselineCount(plans)
	if pending == 0 {
		fmt.Println("Nothing to baseline: all requested migrations are already recorded as applied.")
		return nil
	}

	if !dryRun {
		prompt := fmt.Sprintf(
			"WARNING: this records %d migration(s) as applied WITHOUT running their SQL. Type \"yes\" to confirm: ",
			pending)
		confirmed, err := baselineConfirmed(yesFlag, os.Stdin, os.Stderr, prompt)
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		if !confirmed {
			fmt.Fprintln(os.Stderr, "aborted")
			return fmt.Errorf("baseline aborted: confirmation required (pass --yes for non-interactive use)")
		}
	}

	applied, err := runBaselinePlans(cmd.OutOrStdout(), cmd.Context(), cfg, plans, dryRun, database.BaselineMigration)
	if err != nil {
		return err
	}

	if dryRun {
		fmt.Println("Dry run: nothing was recorded.")
		return nil
	}
	fmt.Printf("Baselined %d migration(s).\n", applied)
	return nil
}

// pendingBaselineCount returns how many plans are not already applied.
func pendingBaselineCount(plans []database.BaselinePlan) int {
	n := 0
	for _, p := range plans {
		if !p.AlreadyApplied {
			n++
		}
	}
	return n
}

// baselineConfirmed implements the same destructive-adjacent confirmation
// gate as runDBReset: --yes skips the prompt for CI/automation, otherwise the
// operator must type exactly "yes". Pure I/O over injected reader/writer so
// it is unit-testable without a terminal.
func baselineConfirmed(yesFlag bool, in io.Reader, out io.Writer, prompt string) (bool, error) {
	if yesFlag {
		return true, nil
	}
	_, _ = fmt.Fprint(out, prompt)
	scanner := bufio.NewScanner(in)
	scanner.Scan()
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return strings.TrimSpace(scanner.Text()) == "yes", nil
}

// baselineWriter matches database.BaselineMigration's signature so tests can
// inject a fake and assert exactly which files were (or were not) written.
type baselineWriter func(ctx context.Context, cfg *config.Config, filePath string) error

// runBaselinePlans prints every plan's "would record" line, then — unless
// dryRun — calls writer for each plan not already applied. dryRun guarantees
// no write: the call to writer sits inside the `if !dryRun` branch and is
// never reached otherwise, so a dry run is structurally incapable of
// recording anything, not just conventionally.
func runBaselinePlans(out io.Writer, ctx context.Context, cfg *config.Config, plans []database.BaselinePlan, dryRun bool, writer baselineWriter) (applied int, err error) {
	for _, p := range plans {
		if p.AlreadyApplied {
			_, _ = fmt.Fprintf(out, "already applied: %s (skipped)\n", p.Name)
			continue
		}
		_, _ = fmt.Fprintf(out, "would record: %s (id=%s, checksum=%s)\n", p.Name, p.MigrationID, p.Checksum)
		if dryRun {
			continue
		}
		if werr := writer(ctx, cfg, p.FilePath); werr != nil {
			return applied, fmt.Errorf("baseline %s: %w", p.Name, werr)
		}
		applied++
		_, _ = fmt.Fprintf(out, "baselined: %s\n", p.Name)
	}
	return applied, nil
}
