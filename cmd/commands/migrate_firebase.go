package commands

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/compat"
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

v1.5 (NSELF_V15=1): the command runs the import pipeline against the live project
instead (also reachable as ` + "`nself db import firebase`" + `):
  nself db import firebase --project <id> --credentials-file <key.json> \
      --auth-export ./auth-users.json [--bucket <name>] [--plan]
With FIRESTORE_EMULATOR_HOST set, no credentials file is needed.

See: https://github.com/nself-org/cli/wiki/cmd-migrate-firebase`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// compat.V15(P7-ADOPT-14): artifact generator -> import pipeline
		if compat.V15() {
			return runFirebaseImport(cmd)
		}
		// The v1.5 flags exist so help and docs describe the import, but v1.4 never had them:
		// refuse them with cobra's old text instead of ignoring them and writing artifacts.
		for _, name := range firebaseV15OnlyFlags {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("unknown flag: --%s", name)
			}
		}
		return dataEnv(runMigrateFirebase)(cmd, args)
	},
}

func init() {
	migrateFirebaseCmd.Flags().String("export-dir", "", "Directory produced by `firebase firestore:export` (required)")
	migrateFirebaseCmd.Flags().String("auth-export", "", "Path to Firebase Auth JSON export (optional)")
	migrateFirebaseCmd.Flags().String("output-dir", "", "Directory for generated artifacts (default: <export-dir>/nself-migration)")
	migrateFirebaseCmd.Flags().String("project-name", "firebase_import", "Project name used in generated SQL and file names")
	migrateFirebaseCmd.Flags().Bool("dry-run", false, "Generate migration artifacts without applying them to a running nSelf instance")
	// v1.5 import pipeline flags (ignored in v1.4 mode); --export-dir is checked in runMigrateFirebase.
	migrateFirebaseCmd.Flags().String("project", "", "Firebase project id (v1.5 import)")
	migrateFirebaseCmd.Flags().String("credentials-file", "", "Service-account key file (v1.5 import; not needed with FIRESTORE_EMULATOR_HOST)")
	migrateFirebaseCmd.Flags().String("bucket", "", "Cloud Storage bucket to copy (v1.5 import)")
	migrateFirebaseCmd.Flags().Bool("plan", false, "Check source and target and print the import plan without writing (v1.5 import)")
	migrateFirebaseCmd.Flags().Bool("keep-stage", false, "Keep the private portable bundle after a successful import (v1.5 import)")
	migrateFirebaseCmd.Flags().Bool("skip-storage", false, "Exempt each storage object from this import (v1.5 import)")
	migrateCmd.AddCommand(migrateFirebaseCmd)
}

func runMigrateFirebase(cmd *cobra.Command, _ []string) error {
	exportDir, _ := cmd.Flags().GetString("export-dir")
	authExport, _ := cmd.Flags().GetString("auth-export")
	outputDir, _ := cmd.Flags().GetString("output-dir")
	projectName, _ := cmd.Flags().GetString("project-name")
	if exportDir == "" {
		return fmt.Errorf("required flag(s) \"export-dir\" not set")
	}

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
