package commands

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestMigrateFirebaseCmdRegistered verifies the firebase subcommand is wired
// under the migrate parent command. Not parallel: see hasSubcommand.
func TestMigrateFirebaseCmdRegistered(t *testing.T) {
	if !hasSubcommand(migrateCmd, "firebase") {
		t.Fatal("expected 'firebase' subcommand under migrateCmd; not found")
	}
}

// TestMigrateFirebaseFlagsPresent verifies all documented flags are registered
// on the firebase subcommand.
func TestMigrateFirebaseFlagsPresent(t *testing.T) {
	t.Parallel()
	required := []string{"export-dir", "auth-export", "output-dir", "project-name", "dry-run"}
	for _, flag := range required {
		if migrateFirebaseCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on migrateFirebaseCmd; not found", flag)
		}
	}
}

// TestMigrateSupabaseCmdRegistered verifies the supabase subcommand is wired
// under the migrate parent command. Not parallel: see hasSubcommand.
func TestMigrateSupabaseCmdRegistered(t *testing.T) {
	if !hasSubcommand(migrateCmd, "supabase") {
		t.Fatal("expected 'supabase' subcommand under migrateCmd; not found")
	}
}

// TestMigrateSupabaseFlagsPresent verifies all documented flags are registered
// on the supabase subcommand.
func TestMigrateSupabaseFlagsPresent(t *testing.T) {
	t.Parallel()
	required := []string{"project-ref", "supabase-key", "out"}
	for _, flag := range required {
		if migrateSupabaseCmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag --%s on migrateSupabaseCmd; not found", flag)
		}
	}
}

// hasSubcommand reports whether parent has a direct child with Use == use.
//
// Callers must not run in parallel (D-0309): cobra's Commands() sorts the
// shared child slice in place on its first call after an AddCommand, so two
// parallel readers race and can transiently lose an entry, and canon tests
// (applyCanon, BuildRegistry) relocate migrate children of the shared RootCmd
// tree while they run. A sequential test never overlaps the parallel ones.
func hasSubcommand(parent *cobra.Command, use string) bool {
	for _, sub := range parent.Commands() {
		if sub.Use == use {
			return true
		}
	}
	return false
}
