package commands

// Purpose: the v1.5 supabase import on the existing `migrate supabase` command (canon:
// `db import supabase`). Inputs: the importer flags registered here. Outputs: the importer
// Report through the shared source command. Constraints: v1.4 keeps the artifact generator;
// the new flags are additive and ignored there.
import (
	"fmt"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/importer"
	_ "github.com/nself-org/cli/internal/importer/supabase"
	"github.com/spf13/cobra"
)

func supabaseSource() importer.Source {
	for _, s := range importer.Sources() {
		if s.Name() == "supabase" {
			return s
		}
	}
	return nil
}

// registerSupabaseImportFlags adds the importer flags to the existing command (project-ref is
// shared with the artifact generator).
func registerSupabaseImportFlags(cmd *cobra.Command) {
	for _, f := range supabaseSource().Flags() {
		if cmd.Flags().Lookup(f.Name) == nil {
			cmd.Flags().String(f.Name, "", f.Usage+" (v1.5 import)")
		}
	}
	cmd.Flags().Bool("plan", false, "Check source and target and print the import plan without writing (v1.5 import)")
	cmd.Flags().Bool("keep-stage", false, "Keep the private portable bundle after a successful import (v1.5 import)")
	cmd.Flags().Bool("skip-storage", false, "Exempt each storage object from this import (v1.5 import)")
}

// supabaseRequiredFlags keeps --project-ref and --supabase-key required for the artifact
// generator only: the import needs neither (the database URL and API URL come from flags).
func supabaseRequiredFlags(cmd *cobra.Command, _ []string) error {
	required := "true"
	// compat.V15(P7-ADOPT-13): --project-ref and --supabase-key required -> optional
	if compat.V15() {
		required = "false"
	}
	for _, name := range []string{"project-ref", "supabase-key"} {
		if err := cmd.Flags().SetAnnotation(name, cobra.BashCompOneRequiredFlag, []string{required}); err != nil {
			return err
		}
	}
	return nil
}

// runSupabaseImport runs the shared import command body with this command's flags.
func runSupabaseImport(cmd *cobra.Command) error {
	source := supabaseSource()
	if source == nil {
		return fmt.Errorf("supabase importer is not registered")
	}
	return importSourceCommand(source).RunE(cmd, nil)
}
