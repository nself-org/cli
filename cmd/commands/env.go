package commands

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/env"

	"github.com/spf13/cobra"
)

// ── Parent command ──────────────────────────────────────────────────

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Multi-environment management: switch, list, diff, copy",
	Long: `Manage multiple environments (dev, staging, prod) with isolated
Docker project names, networks, volumes, and port ranges.

Subcommands:
  use     Switch active environment
  list    List available environments
  show    Show current environment config (secrets redacted)
  diff    Compare two environment configs
  copy    Scaffold a new environment from an existing one`,
	// Without NoArgs, an unknown subcommand falls through to RunE, prints the
	// help text and exits 0. `nself env get FOO` therefore looked like it
	// succeeded and emitted this Long description on stdout — the golden path
	// captured that prose as a base URL and curled it. Reject unknown
	// subcommands instead; bare `nself env` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// ── use ─────────────────────────────────────────────────────────────

var envUseCmd = &cobra.Command{
	Use:   "use <env>",
	Short: "Switch active environment",
	Args:  cobra.ExactArgs(1),
	RunE:  runEnvUse,
}

// ── list ────────────────────────────────────────────────────────────

var envListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available environments",
	RunE:  runEnvList,
}

// ── show ────────────────────────────────────────────────────────────

var envShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current environment config (secrets redacted)",
	RunE:  runEnvShow,
}

// ── diff ────────────────────────────────────────────────────────────

var envDiffCmd = &cobra.Command{
	Use:   "diff <env-a> <env-b>",
	Short: "Compare two environment configs side-by-side",
	Args:  cobra.ExactArgs(2),
	RunE:  runEnvDiff,
}

// ── copy ────────────────────────────────────────────────────────────

var envCopyCmd = &cobra.Command{
	Use:   "copy <from> <to>",
	Short: "Scaffold a new environment from an existing one",
	Args:  cobra.ExactArgs(2),
	RunE:  runEnvCopy,
}

// ── init ────────────────────────────────────────────────────────────

func init() {
	envCmd.AddCommand(envUseCmd)
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envShowCmd)
	envCmd.AddCommand(envDiffCmd)
	envCmd.AddCommand(envCopyCmd)

	RootCmd.AddCommand(envCmd)
}

// ── run functions ───────────────────────────────────────────────────

func runEnvUse(_ *cobra.Command, args []string) error {
	targetEnv := args[0]

	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	previousEnv := env.ActiveEnv(dir)
	if err := env.SetActiveEnv(dir, targetEnv); err != nil {
		return fmt.Errorf("set active env: %w", err)
	}

	if previousEnv == targetEnv {
		fmt.Printf("Already on %s.\n", targetEnv)
	} else {
		fmt.Printf("Switched from %s to %s.\n", previousEnv, targetEnv)
		fmt.Println("Run 'nself build' to regenerate docker-compose for the new environment.")
	}
	return nil
}

func runEnvList(_ *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	envs := env.List(dir)
	if len(envs) == 0 {
		fmt.Println("No environments found. Create .env.dev to get started.")
		return nil
	}

	for _, e := range envs {
		marker := "  "
		if e.Active {
			marker = "* "
		}
		fmt.Printf("%s%s\n", marker, e.Name)
	}
	return nil
}

func runEnvShow(_ *cobra.Command, _ []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	current := env.ActiveEnv(dir)
	pairs, err := env.Show(dir, current)
	if err != nil {
		return fmt.Errorf("show env: %w", err)
	}

	fmt.Printf("Environment: %s\n\n", current)
	for _, p := range pairs {
		fmt.Printf("  %-40s %s\n", p.Key, p.Value)
	}
	return nil
}

func runEnvDiff(_ *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	diffs, err := env.Diff(dir, args[0], args[1])
	if err != nil {
		return fmt.Errorf("env diff: %w", err)
	}

	if len(diffs) == 0 {
		fmt.Printf("No differences between %s and %s.\n", args[0], args[1])
		return nil
	}

	fmt.Printf("%-40s %-30s %s\n", "KEY", args[0], args[1])
	for _, d := range diffs {
		valA := d.ValueA
		valB := d.ValueB
		if valA == "" {
			valA = "(not set)"
		}
		if valB == "" {
			valB = "(not set)"
		}
		fmt.Printf("%-40s %-30s %s\n", d.Key, valA, valB)
	}
	return nil
}

func runEnvCopy(_ *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	if err := env.Copy(dir, args[0], args[1]); err != nil {
		return fmt.Errorf("env copy: %w", err)
	}

	fmt.Printf("Created .env.%s from .env.%s.\n", args[1], args[0])
	fmt.Printf("Edit .env.%s to customize, then run 'nself env use %s'.\n", args[1], args[1])
	return nil
}
