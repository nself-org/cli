package commands

// env_explain.go — `nself env explain [VAR]` (CLI-R18).
//
// Purpose: Show the .env cascade actually in effect for the current project
//          and, given a variable name, which file wins and what every file
//          in the cascade sets it to. Works identically under the canonical
//          order and the NSELF_LEGACY_ENV_ORDER escape hatch, since both
//          read the exact same config.EnvCascadeOrder the loader uses.
// Inputs:  optional VAR positional arg; --reveal flag.
// Outputs: stdout tables/lines. No files are written.
// Constraints: Never prints secret VALUES unless --reveal is passed — default
//              output redacts to "(set, use --reveal to show)" and shows only
//              which file won.
// SPORT:   cli/cmd/commands — CLI-R18 env cascade canon.

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var envExplainCmd = &cobra.Command{
	Use:   "explain [VAR]",
	Short: "Show the effective .env cascade and which file wins",
	Long: `Show the .env cascade in effect for this project.

With no argument, prints every file in load order (lowest precedence first),
whether it exists, and which mode is active (canonical or the
NSELF_LEGACY_ENV_ORDER escape hatch).

With a VAR argument, prints every existing cascade file that sets VAR, the
value each one sets, and which file wins. Values are redacted by default —
pass --reveal to show them.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runEnvExplain,
}

func init() {
	envExplainCmd.Flags().Bool("reveal", false, "Show actual values instead of redacting them")
	envCmd.AddCommand(envExplainCmd)
}

func runEnvExplain(cmd *cobra.Command, args []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	// ResolveEnv matches config.Load()'s own resolution exactly (process
	// ENV wins; otherwise .env's own ENV= key; otherwise "dev") — this
	// command exists specifically to show the truth of what a build would
	// do, so it must never diverge from Load()'s actual decision.
	activeEnv, envSource := config.ResolveEnv(dir)
	legacy := config.LegacyOrderActive()
	cascade := config.EnvCascade(dir, activeEnv, legacy)

	if len(args) == 0 {
		return printCascadeOverview(cascade, activeEnv, envSource, legacy)
	}

	reveal, _ := cmd.Flags().GetBool("reveal")
	return printVarExplain(cascade, args[0], reveal)
}

// printCascadeOverview prints every file in load order with existence and
// precedence, per `nself env explain` (no VAR argument). envSource names
// where activeEnv came from (config.ResolveEnv: "process environment",
// ".env", or "default") so an operator can see at a glance whether a
// mismatch between the box's actual .env and what they expected is a
// process-env override or a stale .env file.
func printCascadeOverview(cascade []config.CascadeFile, activeEnv, envSource string, legacy bool) error {
	mode := "canonical (CLI-R18)"
	if legacy {
		mode = fmt.Sprintf("LEGACY — %s=1 is set", config.LegacyEnvOrderVar)
	}

	fmt.Println()
	fmt.Printf("Environment: %s (from %s)   Cascade mode: %s\n\n", ui.C(ui.Bold, activeEnv), envSource, mode)

	tbl := ui.NewTable("Precedence", "File", "Exists", "Note")
	last := len(cascade) - 1
	for i, f := range cascade {
		// Only the first and last rows carry a qualifier; labelling every
		// middle row "(lowest)" said the opposite of what the table means.
		precedence := fmt.Sprintf("%d of %d", i+1, len(cascade))
		switch i {
		case 0:
			precedence += " (lowest)"
		case last:
			precedence += " (highest)"
		}
		exists := "no"
		if f.Exists {
			exists = "yes"
		}
		note := ""
		if i == last {
			note = "wins whenever it sets a variable"
		}
		tbl.AddRow(precedence, f.Name, exists, note)
	}
	tbl.Render()

	fmt.Println()
	if legacy {
		ui.Warn(fmt.Sprintf("%s is set — this is a temporary escape hatch. Run 'nself migrate' to move this project to the canonical order.", config.LegacyEnvOrderVar))
	}
	ui.Info("Use 'nself env explain VAR' to see which file wins for a specific variable.")
	return nil
}

// printVarExplain prints every existing cascade file that sets VAR, the
// value each sets, and which one wins.
func printVarExplain(cascade []config.CascadeFile, key string, reveal bool) error {
	type setter struct {
		file  string
		value string
	}
	var setters []setter
	var winner string
	var winnerValue string

	for _, f := range cascade {
		if !f.Exists {
			continue
		}
		vars, err := godotenv.Read(f.Path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", f.Path, err)
		}
		v, ok := vars[key]
		if !ok {
			continue
		}
		setters = append(setters, setter{file: f.Name, value: v})
		// Cascade is lowest-precedence first, so the last match wins.
		winner = f.Name
		winnerValue = v
	}

	fmt.Println()
	if len(setters) == 0 {
		fmt.Printf("%s is not set by any file in the cascade.\n", ui.C(ui.Bold, key))
		return nil
	}

	fmt.Printf("%s\n\n", ui.C(ui.Bold, key))
	tbl := ui.NewTable("File", "Value", "Winner")
	for _, s := range setters {
		display := "(set, use --reveal to show)"
		if reveal {
			display = s.value
		}
		mark := ""
		if s.file == winner {
			mark = ui.C(ui.Green, "yes")
		}
		tbl.AddRow(s.file, display, mark)
	}
	tbl.Render()

	fmt.Println()
	value := winnerValue
	if !reveal {
		value = "(redacted — use --reveal to show)"
	}
	ui.Success(fmt.Sprintf("%s wins: %s=%s", winner, key, value))
	return nil
}
