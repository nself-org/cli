package commands

// Purpose: RunE implementations for "nself db migrate audit" and "nself db
// migrate idempotent". Inputs are the cobra command/args; outputs are
// printed migration audit results or an error.
// Constraints: split out of db_audit.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/spf13/cobra"
)

func runDBMigrateAudit(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	results, err := database.AuditMigrations(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("audit migrations: %w", err)
	}

	audit := dbMigrateAuditData{Migrations: []dbMigrationAuditRow{}}
	for _, r := range results {
		issues := r.Issues
		if issues == nil {
			issues = []string{}
		}
		audit.Migrations = append(audit.Migrations, dbMigrationAuditRow{Name: r.Name, Applied: r.Applied, Idempotent: r.Idempotent, HasRollback: r.HasRollback, ChecksumMatch: r.ChecksumMatch, Issues: issues})
	}
	setData(cmd, audit)
	if len(results) == 0 {
		fmt.Println("No migration files found.")
		return nil
	}

	fmt.Printf("%-45s  %-7s  %-10s  %-8s  %-6s\n",
		"MIGRATION", "APPLIED", "IDEMPOTENT", "ROLLBACK", "DRIFT")
	fmt.Println(strings.Repeat("-", 85))

	hasIssues := false
	for _, r := range results {
		drift := "OK"
		if !r.ChecksumMatch {
			drift = "DRIFT"
		}

		fmt.Printf("%-45s  %-7s  %-10s  %-8s  %-6s\n",
			truncateStr(r.Name, 45),
			auditBoolMark(r.Applied),
			auditBoolMark(r.Idempotent),
			auditBoolMark(r.HasRollback),
			drift,
		)

		for _, issue := range r.Issues {
			fmt.Printf("  - %s\n", issue)
			hasIssues = true
		}
	}

	if hasIssues {
		fmt.Println()
		fmt.Println("Run 'nself db migrate idempotent <file>' to generate idempotent versions.")
	}
	return nil
}

func runDBMigrateIdempotent(_ *cobra.Command, args []string) error {
	filePath := args[0]

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read migration file: %w", err)
	}

	sqlContent := string(data)

	idempotent, issues := database.CheckMigrationIdempotency(sqlContent)

	if idempotent {
		fmt.Printf("Migration %q is idempotent. No changes needed.\n", filePath)
		return nil
	}

	fmt.Printf("Migration %q has non-idempotent patterns:\n", filePath)
	for _, issue := range issues {
		fmt.Printf("  - %s\n", issue)
	}

	converted, changes := database.GenerateIdempotentVersion(sqlContent)
	if len(changes) == 0 {
		fmt.Println("\nCould not automatically convert all patterns. Manual review required.")
		return nil
	}

	fmt.Printf("\nProposed idempotent version (%d change(s)):\n", len(changes))
	for _, c := range changes {
		fmt.Printf("  + %s\n", c)
	}
	fmt.Println()
	fmt.Println("--- Converted SQL ---")
	fmt.Println(converted)
	return nil
}
