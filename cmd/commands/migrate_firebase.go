package commands

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/migrate/firebase"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// migrateFirebaseCmd implements `nself migrate firebase`.
// It reads a Firestore JSON export (produced by `firebase firestore:export`)
// and generates SQL migration, Hasura metadata, and optional auth import artifacts.
var migrateFirebaseCmd = &cobra.Command{
	Use:   "firebase",
	Short: "Generate nSelf migration artifacts from a Firebase export",
	Long: `Generate nSelf-compatible migration artifacts from a Firebase Firestore
and Auth JSON export.

Workflow:
  1. Export your Firestore data:
       firebase firestore:export --format json ./firebase-export
  2. (Optional) Export your Auth users:
       firebase auth:export --format json ./firebase-export/auth-users.json
  3. Run this command:
       nself migrate firebase --export-dir ./firebase-export

Artifacts written to <output-dir> (default: <export-dir>/nself-migration):
  - migration.sql        — CREATE TABLE statements with RLS scaffolding
  - hasura-metadata.yaml — Hasura table tracking + permissions scaffold
  - auth-import.sql      — INSERT statements for Auth users (when --auth-export given)
  - MIGRATION_SUMMARY.md — Step-by-step next-actions guide

No live Firebase connection is required. The command operates entirely on
the exported JSON files.

See: https://github.com/nself-org/cli/wiki/cmd-migrate-firebase`,
	RunE: dataEnv(runMigrateFirebase),
}

func init() {
	migrateFirebaseCmd.Flags().String("export-dir", "", "Directory produced by `firebase firestore:export` (required)")
	migrateFirebaseCmd.Flags().String("auth-export", "", "Path to Firebase Auth JSON export (optional)")
	migrateFirebaseCmd.Flags().String("output-dir", "", "Directory for generated artifacts (default: <export-dir>/nself-migration)")
	migrateFirebaseCmd.Flags().String("project-name", "firebase_import", "Project name used in generated SQL and file names")
	migrateFirebaseCmd.Flags().Bool("dry-run", false, "Generate migration artifacts without applying them to a running nSelf instance")
	_ = migrateFirebaseCmd.MarkFlagRequired("export-dir")
	migrateCmd.AddCommand(migrateFirebaseCmd)
}

func runMigrateFirebase(cmd *cobra.Command, _ []string) error {
	exportDir, _ := cmd.Flags().GetString("export-dir")
	authExport, _ := cmd.Flags().GetString("auth-export")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	projectName, _ := cmd.Flags().GetString("project-name")

	ui.CommandHeader("nself migrate firebase", "Firebase → nSelf migration scaffold")
	fmt.Println()

	opts := firebase.Options{
		ExportDir:      exportDir,
		AuthExportFile: authExport,
		OutputDir:      outputDir,
		ProjectName:    projectName,
	}

	result, err := firebase.Run(cmd.Context(), opts)
	if err != nil {
		return fmt.Errorf("firebase migration: %w", err)
	}

	ui.Success(fmt.Sprintf("Inferred %d table(s): %s", len(result.Tables), strings.Join(result.Tables, ", ")))
	fmt.Println()
	if result.SchemaSQL != "" {
		fmt.Printf("  Schema SQL:       %s\n", result.SchemaSQL)
	}
	if result.HasuraYAML != "" {
		fmt.Printf("  Hasura metadata:  %s\n", result.HasuraYAML)
	}
	if result.AuthImportSQL != "" {
		fmt.Printf("  Auth import SQL:  %s\n", result.AuthImportSQL)
	}
	if result.Summary != "" {
		fmt.Printf("  Migration guide:  %s\n", result.Summary)
	}
	fmt.Println()
	ui.Info("Next steps: review MIGRATION_SUMMARY.md in the output directory.")
	return nil
}
