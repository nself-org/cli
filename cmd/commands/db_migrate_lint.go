package commands

// Purpose: `nself db migrate lint [file...]` — statically refuse migrations
// that mix many bare CREATE TABLE statements with no IF NOT EXISTS guard in
// one file (the tasks_schema-style mistake behind cli#386: fine against an
// empty database, a hard failure against one where some tables already
// exist). See internal/database/migrate_lint.go for the check itself.
// Inputs: cobra command/args — explicit file paths, or none to lint the
// auto-detected (or --migration-dir) migrations directory.
// Outputs: printed per-file OK/FAIL lines; a non-nil error (refusal) if any
// file trips the lint.
// Constraints: read-only — never touches a live database or writes anything.

import (
	"fmt"

	"github.com/nself-org/cli/internal/database"

	"github.com/spf13/cobra"
)

var dbMigrateLintCmd = &cobra.Command{
	Use:   "lint [file...]",
	Short: "Refuse migrations with many unguarded CREATE TABLE statements",
	Long: `Statically scans migration SQL for a single file issuing many bare
CREATE TABLE statements with no IF NOT EXISTS guard anywhere in the file.

Such a file applies fine against an empty database but fails hard (and can
leave a mixed apply/fail state) against a database where some of the tables
already exist — the exact shape that blocked staging alignment in cli#386.

With no arguments, lints every migration in the auto-detected (or
--migration-dir) migrations directory. Pass one or more file paths to lint
specific files instead.

Exits non-zero (refuses) if any file trips the lint.`,
	RunE: dataEnv(runDBMigrateLint),
}

func init() {
	dbMigrateLintCmd.Flags().String("migration-dir", "", "Lint migrations in this directory instead of the auto-detected one")
	dbMigrateCmd.AddCommand(dbMigrateLintCmd)
}

func runDBMigrateLint(cmd *cobra.Command, args []string) error {
	files := args
	if len(files) == 0 {
		cfg, err := loadProjectConfig()
		if err != nil {
			return err
		}
		migrationDir, _ := cmd.Flags().GetString("migration-dir")
		files, err = database.ListMigrationFiles(cfg, migrationDir)
		if err != nil {
			return err
		}
	}

	failed := 0
	for _, f := range files {
		finding, err := database.LintMigrationFile(f)
		if err != nil {
			return err
		}
		if finding == nil {
			fmt.Printf("OK    %s\n", f)
			continue
		}
		failed++
		fmt.Printf("FAIL  %s: %s\n", f, finding.Message)
	}

	setData(cmd, dbMigrateLintData{Files: append([]string{}, files...)})
	if failed > 0 {
		return fmt.Errorf("db migrate lint: refused %d migration file(s) — see messages above", failed)
	}
	fmt.Println("All migrations passed lint.")
	return nil
}
