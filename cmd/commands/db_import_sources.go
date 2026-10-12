package commands

// Purpose: attach registered source adapters to the migrate tree and canon import hub.
// Inputs: env-name flags, started project. Outputs: importer Report via EmitData.
// Constraints: existing supabase/firebase command behavior is preserved.
import (
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/output"
	"github.com/spf13/cobra"
	"os"
)

var importRun = importer.Run

func init() { registerImportCommands() }
func registerImportCommands() {
	for _, source := range importer.Sources() {
		exists := false
		for _, child := range migrateCmd.Commands() {
			if child.Name() == source.Name() {
				exists = true
			}
		}
		if exists || source.Name() == "firebase" || source.Name() == "supabase" {
			continue
		}
		migrateCmd.AddCommand(importSourceCommand(source))
	}
}
func importSourceCommand(source importer.Source) *cobra.Command {
	cmd := &cobra.Command{Use: source.Name(), Short: "Import and verify data from " + source.Name(), Args: cobra.NoArgs}
	for _, flag := range source.Flags() {
		cmd.Flags().String(flag.Name, "", flag.Usage)
	}
	cmd.Flags().Bool("plan", false, "Check source and target and print the import plan without writing")
	cmd.Flags().Bool("keep-stage", false, "Keep the private portable bundle after a successful import")
	cmd.Flags().Bool("skip-storage", false, "Exempt each storage object from this import")
	cmd.Flags().Bool("json", false, "Print the structured import report")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := config.Load(dir)
		if err != nil {
			return err
		}
		values := map[string]string{}
		for _, flag := range source.Flags() {
			values[flag.Name], _ = cmd.Flags().GetString(flag.Name)
		}
		plan, _ := cmd.Flags().GetBool("plan")
		keep, _ := cmd.Flags().GetBool("keep-stage")
		skip, _ := cmd.Flags().GetBool("skip-storage")
		report, err := importRun(cmd.Context(), cfg, source, importer.Input{Values: values, Env: os.Getenv}, importer.RunOptions{ProjectDir: dir, Plan: plan, KeepStage: keep, SkipStorage: skip})
		if err != nil {
			return err
		}
		if emitErr := output.EmitData(output.Default(), "db import "+source.Name(), report); emitErr != nil {
			return emitErr
		}
		return err
	}
	return cmd
}
