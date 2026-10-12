package commands

// Purpose: register the firebase importer source and run it for `nself db import firebase`.
// Inputs: the migrate firebase flags (project, credentials-file, auth-export, bucket, plan,
// keep-stage, skip-storage). Outputs: the importer Report through the shared source command.
// Constraints: v1.5 only (the caller is compat-gated); flag errors name the flag, never its value.
import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/firebase"
	"github.com/spf13/cobra"
)

// firebaseImportFlags are copied from the migrate firebase command to the source command.
var firebaseImportFlags = []string{"project", "credentials-file", "auth-export", "bucket", "plan", "keep-stage", "skip-storage", "json"}

// runFirebaseImport runs the registered firebase source through the shared importer command.
func runFirebaseImport(cmd *cobra.Command) error {
	var source importer.Source
	for _, s := range importer.Sources() {
		if s.Name() == "firebase" {
			source = s
		}
	}
	if source == nil {
		return fmt.Errorf("firebase importer source is not registered")
	}
	child := importSourceCommand(source)
	child.SetContext(cmd.Context())
	values := map[string]string{}
	for _, name := range firebaseImportFlags {
		if f := cmd.Flags().Lookup(name); f != nil && f.Changed {
			if err := child.Flags().Set(name, f.Value.String()); err != nil {
				return err
			}
			values[name] = f.Value.String()
		}
	}
	// The v1.4 --dry-run promised "without applying"; in the pipeline that is --plan.
	if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
		if err := child.Flags().Set("plan", "true"); err != nil {
			return err
		}
		values["plan"] = "true"
	}
	if err := firebase.CheckInput(importer.Input{Values: values, Env: os.Getenv}); err != nil {
		return err
	}
	return child.RunE(child, nil)
}
