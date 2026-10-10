package commands

// migrate generate — AI-assisted SQL migration generation.
//
// Usage:
//
//	nself migrate generate "add user soft delete"
//
// The command calls the nself-ai plugin to produce a candidate SQL migration file
// from a natural-language description, then presents a dry-run preview and asks
// the developer to confirm before the file is staged in migrations/.
//
// The additive-only guard (enforced in internal/migrationai) ensures that no
// DROP, TRUNCATE, or DELETE statement is ever written to disk without an explicit
// warning comment. NSELF_MIGRATION_AUTO_APPLY is always false in production.

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/migrationai"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var migrateGenerateCmd = &cobra.Command{
	Use:   "generate <description>",
	Short: "Generate a SQL migration from a natural-language description",
	Long: `Use the nSelf AI plugin to propose a SQL migration from a plain-English prompt.

The AI reads your description, optionally scans your model files for schema context,
and writes a candidate migration file to migrations/. You review the proposed SQL
and confirm or reject it before it is staged.

The additive-only guard prevents any DROP, TRUNCATE, or DELETE statement from being
written without an explicit warning comment. NSELF_MIGRATION_AUTO_APPLY is always
false — you must run 'nself db migrate up' to apply.

Examples:
  nself migrate generate "add soft-delete column to users"
  nself migrate generate "add index on orders.created_at"
  nself migrate generate "add tenant_id to all np_ tables"`,
	Args: cobra.ExactArgs(1),
	RunE: dataEnv(runMigrateGenerate),
}

var (
	migrateGenerateYes           bool
	migrateGenerateConfidenceMin float64
	migrateGenerateMigrationsDir string
	migrateGenerateAIEndpoint    string
	migrateGenerateDryRun        bool
)

func init() {
	migrateGenerateCmd.Flags().BoolVarP(&migrateGenerateYes, "yes", "y", false,
		"Auto-accept the proposal without interactive confirmation")
	migrateGenerateCmd.Flags().Float64Var(&migrateGenerateConfidenceMin, "confidence", 0.75,
		"Minimum AI confidence to accept a proposal (0.0-1.0)")
	migrateGenerateCmd.Flags().StringVar(&migrateGenerateMigrationsDir, "migrations-dir", "migrations",
		"Directory to write migration files into")
	migrateGenerateCmd.Flags().StringVar(&migrateGenerateAIEndpoint, "ai-endpoint", "",
		"nSelf AI plugin endpoint (default: NSELF_AI_ENDPOINT env var or http://localhost:3001)")
	migrateGenerateCmd.Flags().BoolVar(&migrateGenerateDryRun, "dry-run", false,
		"Preview the generated SQL without writing any files")

	migrateCmd.AddCommand(migrateGenerateCmd)
}

func runMigrateGenerate(cmd *cobra.Command, args []string) error {
	description := strings.TrimSpace(args[0])
	if description == "" {
		return fmt.Errorf("migration description is required")
	}

	// Build generator config.
	cfg := migrationai.DefaultConfig()
	cfg.ConfidenceMin = migrateGenerateConfidenceMin
	cfg.MigrationsDir = migrateGenerateMigrationsDir

	if migrateGenerateAIEndpoint != "" {
		cfg.AIEndpoint = migrateGenerateAIEndpoint
	} else if envEndpoint := os.Getenv("NSELF_AI_ENDPOINT"); envEndpoint != "" {
		cfg.AIEndpoint = envEndpoint
	}

	// Hard rule: NSELF_MIGRATION_AUTO_APPLY must never be true in production profiles.
	if os.Getenv("NSELF_MIGRATION_AUTO_APPLY") == "true" {
		profile := os.Getenv("NSELF_PROFILE")
		if profile == "prod" || profile == "production" {
			ui.Warn("NSELF_MIGRATION_AUTO_APPLY=true is forbidden in production profiles. Ignoring.")
		}
	}
	cfg.AutoApply = false // always — no auto-apply regardless of env

	ui.Section("AI Migration Generator")
	fmt.Printf("  Prompt:   %s\n", description)
	fmt.Printf("  Endpoint: %s\n", cfg.AIEndpoint)
	fmt.Printf("  Min conf: %.2f\n\n", cfg.ConfidenceMin)

	gen := migrationai.NewGenerator(cfg)

	// Use a 60-second timeout for the AI call to prevent hanging.
	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	spin := ui.NewSpinner("Asking AI for a migration proposal...")
	spin.Start()

	proposal, err := gen.Generate(ctx, description, nil)
	spin.Stop()

	if err != nil {
		if lcErr, ok := err.(*migrationai.ErrLowConfidence); ok {
			ui.Warn(fmt.Sprintf("Proposal rejected: AI confidence %.2f < minimum %.2f",
				lcErr.Got, lcErr.Required))
			ui.Info("Try rephrasing your description for a more specific prompt.")
			return nil
		}
		return fmt.Errorf("generate migration: %w", err)
	}

	// Print the dry-run preview.
	fmt.Printf("Proposed SQL (confidence: %.0f%%, model: %s):\n\n", proposal.Confidence*100, proposal.AIModel)
	fmt.Println(strings.Repeat("-", 60))
	printMigrationSQL(proposal.SQL)
	if proposal.WarningSQL != "" {
		fmt.Println()
		ui.Warn("The following statements were blocked by the additive-only guard:")
		fmt.Println(proposal.WarningSQL)
	}
	fmt.Println(strings.Repeat("-", 60))

	if migrateGenerateDryRun {
		ui.Info("Dry run complete. No files written.")
		return nil
	}

	// Prompt for confirmation unless --yes was passed.
	if !migrateGenerateYes {
		if !promptMigrateYesNo("Accept this proposal and stage the migration file?") {
			if err := migrationai.DeleteMigrationFile(proposal.FilePath); err != nil {
				ui.Warn(fmt.Sprintf("Could not remove staged file %s: %v", proposal.FilePath, err))
			}
			ui.Info("Proposal rejected. No changes made.")
			return nil
		}
	}

	ui.Success(fmt.Sprintf("Migration staged: %s", proposal.FilePath))
	ui.Info("Review the file, then run: nself db migrate up")
	return nil
}

// printMigrationSQL prints SQL with two-space indentation per line.
func printMigrationSQL(sql string) {
	for _, line := range strings.Split(sql, "\n") {
		if strings.TrimSpace(line) == "" {
			fmt.Println()
		} else {
			fmt.Printf("  %s\n", line)
		}
	}
}

// promptMigrateYesNo asks the user a yes/no question on stdin. Returns true on "y" / "yes".
func promptMigrateYesNo(question string) bool {
	fmt.Printf("\n%s [y/N] ", question)
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))
	return answer == "y" || answer == "yes"
}
