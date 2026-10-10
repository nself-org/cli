package commands

// Purpose: RunE implementations for "nself db seed" and its run/list/verify/
// graph subcommands. Inputs are the cobra command/args; outputs are seed
// results printed to the user or an error.
// Constraints: split out of db.go (CLI-R12) as a pure move, no behavior change.

import (
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/seed"

	"github.com/spf13/cobra"
)

func runDBSeed(cmd *cobra.Command, args []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	var file string
	if len(args) > 0 {
		file = args[0]
	}
	if err := database.Seed(cmd.Context(), cfg, file); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	fmt.Println("Seed data applied.")
	return nil
}

func runDBSeedRun(cmd *cobra.Command, _ []string) error {
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	env, _ := cmd.Flags().GetString("env")
	if env == "" {
		env = cfg.Env
	}
	if env == "" {
		env = "dev"
	}

	fixture, _ := cmd.Flags().GetString("fixture")

	seeds, err := seed.CollectForRun(dir, env, fixture)
	if err != nil {
		return fmt.Errorf("collect seeds: %w", err)
	}

	if len(seeds) == 0 {
		fmt.Println("No seeds found.")
		return nil
	}

	// Guard against destructive seeds in production.
	if cfg.IsProduction() {
		for _, s := range seeds {
			if s.Destructive {
				return fmt.Errorf("destructive seed %s cannot run in production", s.Name)
			}
		}
	}

	for _, s := range seeds {
		if err := database.Seed(cmd.Context(), cfg, s.Path); err != nil {
			return fmt.Errorf("seed %s: %w", s.Name, err)
		}
		fmt.Printf("  Applied: %s\n", s.Name)
	}
	fmt.Printf("Applied %d seed(s).\n", len(seeds))
	return nil
}

func runDBSeedList(cmd *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	seeds, err := seed.ListSeeds(dir)
	if err != nil {
		return fmt.Errorf("list seeds: %w", err)
	}

	listed := dbSeedListData{Seeds: seedRows(seeds), Fixtures: []string{}}
	if fx, ferr := seed.ListFixtures(dir); ferr == nil && fx != nil {
		listed.Fixtures = fx
	}
	setData(cmd, listed)
	if len(seeds) == 0 {
		fmt.Println("No seeds found. Create db/seeds/ directory to get started.")
		return nil
	}

	fmt.Printf("%-30s %-20s %-12s %s\n", "NAME", "ENV", "TYPE", "DEPENDS ON")
	for _, s := range seeds {
		seedType := "standard"
		if s.Idempotent {
			seedType = "idempotent"
		}
		if s.Destructive {
			seedType = "destructive"
		}
		deps := strings.Join(s.DependsOn, ", ")
		fmt.Printf("%-30s %-20s %-12s %s\n", s.Name, s.Env, seedType, deps)
	}

	// Also list fixtures.
	fixtures, _ := seed.ListFixtures(dir)
	if len(fixtures) > 0 {
		fmt.Printf("\nFixtures: %s\n", strings.Join(fixtures, ", "))
	}
	return nil
}

func runDBSeedVerify(cmd *cobra.Command, _ []string) error {
	fixture, _ := cmd.Flags().GetString("fixture")
	if fixture == "" {
		return fmt.Errorf("--fixture is required")
	}

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	seeds, err := seed.CollectForRun(dir, "", fixture)
	if err != nil {
		return fmt.Errorf("collect fixture seeds: %w", err)
	}

	if len(seeds) == 0 {
		return fmt.Errorf("fixture %q has no seeds", fixture)
	}

	setData(cmd, dbSeedVerifyData{Fixture: fixture, Seeds: seedRows(seeds)})
	fmt.Printf("Fixture %q: %d seed file(s)\n", fixture, len(seeds))
	for _, s := range seeds {
		marker := "  "
		if s.Idempotent {
			marker = "I "
		}
		if s.Destructive {
			marker = "D "
		}
		fmt.Printf("  %s%s\n", marker, s.Name)
	}
	fmt.Println("Verification: seed files parsed successfully.")
	return nil
}

func runDBSeedGraph(cmd *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	nodes, err := seed.DependencyGraph(dir)
	if err != nil {
		return fmt.Errorf("dependency graph: %w", err)
	}

	graph := dbSeedGraphData{Nodes: []dbSeedGraphNode{}}
	for _, n := range nodes {
		graph.Nodes = append(graph.Nodes, dbSeedGraphNode{Name: n.Name, Env: n.Env, DependsOn: nonNilStrings(n.DependsOn)})
	}
	setData(cmd, graph)
	if len(nodes) == 0 {
		fmt.Println("No seeds found.")
		return nil
	}

	for _, n := range nodes {
		if len(n.DependsOn) == 0 {
			fmt.Printf("%s (%s)\n", n.Name, n.Env)
		} else {
			fmt.Printf("%s (%s) -> %s\n", n.Name, n.Env, strings.Join(n.DependsOn, ", "))
		}
	}
	return nil
}
