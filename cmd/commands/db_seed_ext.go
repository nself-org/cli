package commands

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/seed"
	"github.com/spf13/cobra"
)

// ── db seed fixtures ─────────────────────────────────────────────────

var dbSeedFixturesCmd = &cobra.Command{
	Use:   "fixtures",
	Short: "Manage seed fixtures",
	Long: `Manage seed fixtures: list, run, verify checksums, and show/save manifests.

Subcommands:
  list     List available fixtures
  run      Run a specific fixture
  verify   Verify fixture checksums against a stored manifest
  manifest Show or save the fixture manifest`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself db seed fixtures` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var dbSeedFixturesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available fixtures",
	RunE:  runDBSeedFixturesList,
}

var dbSeedFixturesRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Run a specific fixture",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBSeedFixturesRun,
}

var dbSeedFixturesVerifyCmd = &cobra.Command{
	Use:   "verify <name>",
	Short: "Verify fixture checksums against a stored manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBSeedFixturesVerify,
}

var dbSeedFixturesManifestCmd = &cobra.Command{
	Use:   "manifest <name>",
	Short: "Show or store a fixture manifest",
	Args:  cobra.ExactArgs(1),
	RunE:  runDBSeedFixturesManifest,
}

// ── db seed matrix ───────────────────────────────────────────────────

var dbSeedMatrixCmd = &cobra.Command{
	Use:   "matrix",
	Short: "Show the seed environment/file matrix",
	RunE:  runDBSeedMatrix,
}

// ── init ─────────────────────────────────────────────────────────────

func init() {
	// --save flag for manifest subcommand.
	dbSeedFixturesManifestCmd.Flags().Bool("save", false, "Store the manifest to .nself/seed-manifests/")

	// Wire fixtures subcommand tree into existing dbSeedCmd.
	dbSeedCmd.AddCommand(dbSeedFixturesCmd)
	dbSeedFixturesCmd.AddCommand(dbSeedFixturesListCmd)
	dbSeedFixturesCmd.AddCommand(dbSeedFixturesRunCmd)
	dbSeedFixturesCmd.AddCommand(dbSeedFixturesVerifyCmd)
	dbSeedFixturesCmd.AddCommand(dbSeedFixturesManifestCmd)

	// Wire matrix subcommand into existing dbSeedCmd.
	dbSeedCmd.AddCommand(dbSeedMatrixCmd)
}

// ── run functions ─────────────────────────────────────────────────────

func runDBSeedFixturesList(_ *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	fixtures, err := seed.ListFixtures(dir)
	if err != nil {
		return fmt.Errorf("listing fixtures: %w", err)
	}

	if len(fixtures) == 0 {
		fmt.Println("No fixtures found. Create db/seeds/fixtures/<name>/ to get started.")
		return nil
	}

	fmt.Printf("%-25s %s\n", "FIXTURE", "FILES")
	for _, name := range fixtures {
		m, err := seed.LoadFixtureManifest(dir, name)
		if err != nil {
			fmt.Printf("%-25s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("%-25s %d file(s)\n", name, len(m.Files))
	}
	return nil
}

func runDBSeedFixturesRun(cmd *cobra.Command, args []string) error {
	fixtureName := args[0]

	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	runner := func(filePath string) error {
		return database.Seed(cmd.Context(), cfg, filePath)
	}

	result, err := seed.RunFixture(dir, fixtureName, runner)
	if err != nil {
		return fmt.Errorf("running fixture %q: %w", fixtureName, err)
	}

	fmt.Printf("Fixture %q: applied %d file(s) in %s.\n", fixtureName, result.FilesRun, result.Duration.Round(1000000))
	return nil
}

func runDBSeedFixturesVerify(_ *cobra.Command, args []string) error {
	fixtureName := args[0]

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	stored, err := seed.LoadStoredManifest(dir, fixtureName)
	if err != nil {
		return fmt.Errorf("loading stored manifest for %q: %w", fixtureName, err)
	}

	if stored == nil {
		fmt.Printf("No stored manifest for fixture %q. Run 'nself db seed fixtures manifest --save %s' first.\n", fixtureName, fixtureName)
		return nil
	}

	changed, err := seed.VerifyFixture(dir, fixtureName, stored)
	if err != nil {
		return fmt.Errorf("verifying fixture %q: %w", fixtureName, err)
	}

	if len(changed) == 0 {
		fmt.Printf("Fixture %q: all %d file(s) match stored checksums.\n", fixtureName, len(stored.Files))
		return nil
	}

	fmt.Printf("Fixture %q: %d file(s) have changed:\n", fixtureName, len(changed))
	for _, name := range changed {
		fmt.Printf("  changed: %s\n", name)
	}
	return fmt.Errorf("fixture %q has %d changed file(s)", fixtureName, len(changed))
}

func runDBSeedFixturesManifest(cmd *cobra.Command, args []string) error {
	fixtureName := args[0]
	save, _ := cmd.Flags().GetBool("save")

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	manifest, err := seed.LoadFixtureManifest(dir, fixtureName)
	if err != nil {
		return fmt.Errorf("loading fixture manifest for %q: %w", fixtureName, err)
	}

	// Display manifest.
	fmt.Printf("Fixture:    %s\n", manifest.Name)
	fmt.Printf("TotalHash:  %s\n", manifest.TotalHash)
	fmt.Printf("Files (%d):\n", len(manifest.Files))

	for _, f := range manifest.Files {
		fmt.Printf("  %-35s %s  %d bytes\n", f.Name, f.Checksum, f.Size)
	}

	if save {
		if err := seed.WriteFixtureManifest(dir, manifest); err != nil {
			return fmt.Errorf("saving fixture manifest for %q: %w", fixtureName, err)
		}
		fmt.Printf("\nManifest saved to .nself/seed-manifests/%s.json\n", fixtureName)
	} else {
		fmt.Printf("\nRun with --save to persist this manifest for future verify operations.\n")
	}

	return nil
}

func runDBSeedMatrix(_ *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	matrix, err := seed.BuildEnvMatrix(dir)
	if err != nil {
		return fmt.Errorf("building env matrix: %w", err)
	}

	fmt.Print(seed.PrintMatrix(matrix))
	return nil
}
