package commands

import (
	"fmt"
	"os"
	"path/filepath"

	spmigrate "github.com/nself-org/cli/internal/migrate/supabase"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// migrateSupabaseCmd implements `nself migrate supabase`.
var migrateSupabaseCmd = &cobra.Command{
	Use:   "supabase",
	Short: "Migrate a Supabase project to nSelf",
	Long: `Pull schema, data, auth users, and storage buckets from a Supabase project
and generate nSelf-compatible migration artifacts.

Steps performed:
  1. Connect to Supabase via PostgREST + service_role key
  2. Pull public schema (tables, columns, RLS policies where accessible)
  3. Pull auth users via Supabase Admin API
  4. Pull storage buckets via Supabase Storage API
  5. Generate Drizzle-compatible SQL migration file
  6. Generate Hasura metadata YAML
  7. Generate auth user import script
  8. Print summary and next steps

Flags:
  --project-ref   Supabase project reference (the ID segment of your project URL)
  --supabase-key  service_role key from Supabase > Project Settings > API

Examples:
  nself migrate supabase --project-ref=abcdefghijkl --supabase-key=eyJ...
  nself migrate supabase --project-ref=abcdefghijkl --supabase-key=eyJ... --out=./migration

Note: CI/offline test environments should set NSELF_SUPABASE_SKIP=1 to skip
live Supabase connectivity.`,
	RunE: dataEnv(runMigrateSupabase),
}

func init() {
	migrateSupabaseCmd.Flags().String("project-ref", "", "Supabase project reference ID (required)")
	migrateSupabaseCmd.Flags().String("supabase-key", "", "Supabase service_role key (required)")
	migrateSupabaseCmd.Flags().String("out", ".", "Output directory for generated files (default: current dir)")
	_ = migrateSupabaseCmd.MarkFlagRequired("project-ref")
	_ = migrateSupabaseCmd.MarkFlagRequired("supabase-key")

	migrateCmd.AddCommand(migrateSupabaseCmd)
}

func runMigrateSupabase(cmd *cobra.Command, _ []string) error {
	projectRef, _ := cmd.Flags().GetString("project-ref")
	supabaseKey, _ := cmd.Flags().GetString("supabase-key")
	outDir, _ := cmd.Flags().GetString("out")

	if outDir == "" {
		outDir = "."
	}

	ui.CommandHeader("nself migrate supabase", fmt.Sprintf("Migrating Supabase project %q", projectRef))

	// CI / offline skip guard.
	if os.Getenv("NSELF_SUPABASE_SKIP") == "1" {
		ui.Info("NSELF_SUPABASE_SKIP=1 — skipping live Supabase connectivity (CI mode)")
		return nil
	}

	cfg := spmigrate.Config{
		ProjectRef: projectRef,
		ServiceKey: supabaseKey,
	}

	steps := ui.NewInitSteps(false,
		"Connect and pull schema",
		"Pull auth users",
		"Pull storage buckets",
		"Generate migration SQL",
		"Generate Hasura metadata",
		"Generate auth import script",
		"Write files",
	)

	// Step 1: pull everything (schema + RLS + users + buckets in one call).
	steps.Next()
	result, err := spmigrate.Pull(cmd.Context(), cfg)
	if err != nil {
		return fmt.Errorf("pulling from Supabase: %w", err)
	}

	// Steps 2-3: already done inside Pull — advance progress display.
	steps.Next()
	steps.Next()

	// Step 4-6: generate artifacts.
	steps.Next()
	steps.Next()
	steps.Next()
	arts := spmigrate.Generate(projectRef, result)

	// Step 7: write files.
	steps.Next()

	absOut, err := filepath.Abs(outDir)
	if err != nil {
		return fmt.Errorf("resolving output directory: %w", err)
	}
	if mkErr := os.MkdirAll(absOut, 0755); mkErr != nil {
		return fmt.Errorf("creating output directory: %w", mkErr)
	}

	artifacts := []struct {
		name    string
		content string
		perm    os.FileMode
	}{
		{"supabase-migration.sql", arts.MigrationSQL, 0644},
		{"hasura-metadata.yaml", arts.HasuraMetadata, 0644},
		{"import-auth-users.sh", arts.AuthImportScript, 0755},
	}

	for _, a := range artifacts {
		path := filepath.Join(absOut, a.name)
		if writeErr := os.WriteFile(path, []byte(a.content), a.perm); writeErr != nil {
			return fmt.Errorf("writing %s: %w", a.name, writeErr)
		}
		ui.Success(fmt.Sprintf("Written: %s", path))
	}

	steps.Done()

	// Summary.
	fmt.Println()
	ui.Section("Summary")
	fmt.Println(arts.Summary)

	return nil
}
