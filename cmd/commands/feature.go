package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/featureflags"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// featureCmd exposes the CLI-built-in feature flag registry. It is distinct
// from `nself flag`, which talks to the runtime feature-flags PLUGIN
// (port 3305). `feature` is a binary-local, plugin-free registry suited to
// build-time / install-time capability gates that ship with the CLI.
//
// State is persisted to <projectDir>/.nself/features.json. The CLI uses the
// current working directory as the project root, matching `nself build` /
// `nself start` behavior.
var featureCmd = &cobra.Command{
	Use:   "features",
	Short: "Manage CLI-built-in feature flags",
	Long: `Manage CLI-built-in feature flags.

The feature flag registry is compiled into the nSelf binary (registry.yaml in
internal/featureflags). Per-project overrides persist to .nself/features.json
in the current working directory.

This is distinct from 'nself flag', which manages runtime feature flags via
the feature-flags plugin (port 3305). 'nself feature' covers build-time and
install-time capability gates for v1.1.0+ release features (ɳSentry, ɳFamily
COPPA strict mode, multi-tenant strict mode, etc.).

Subcommands:
  list     List all registered feature flags with effective state
  enable   Override a flag to enabled=true
  disable  Override a flag to enabled=false
  status   Show the effective state for a single flag

Examples:
  nself feature list
  nself feature enable nsentry-enabled
  nself feature status nfamily-coppa-strict
  nself feature disable cloud-multi-tenant-strict`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself config features` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

// projectDir returns the project root used for state persistence.
// In v1, this is simply the current working directory: every nself state
// file in the codebase uses CWD as the implicit project root.
func projectDir() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

func loadRegAndState(cmd *cobra.Command) (*featureflags.Registry, *featureflags.State, string, error) {
	reg, err := featureflags.Load()
	if err != nil {
		return nil, nil, "", err
	}
	dir, err := projectDir()
	if err != nil {
		return nil, nil, "", err
	}
	state, err := featureflags.LoadState(dir)
	if err != nil {
		return nil, nil, "", err
	}
	return reg, state, dir, nil
}

var featureListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered feature flags with effective state",
	Long: `List every flag declared in the built-in registry, including the
effective enabled state (override or default) and the override source.

Use --json for machine-readable output.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")

		reg, state, _, err := loadRegAndState(cmd)
		if err != nil {
			return fmt.Errorf("feature list: %w", err)
		}

		statuses := reg.Resolve(state)

		if jsonOut {
			b, err := json.MarshalIndent(statuses, "", "  ")
			if err != nil {
				return fmt.Errorf("feature list: marshal: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}

		if len(statuses) == 0 {
			ui.Info("No feature flags registered.")
			return nil
		}

		ui.CommandHeader("Feature Flags", fmt.Sprintf("%d registered", len(statuses)))
		t := ui.NewTable("KEY", "TYPE", "SURFACE", "ENABLED", "SOURCE", "INTRODUCED")
		for _, s := range statuses {
			t.AddRow(
				s.Flag.Key,
				string(s.Flag.Type),
				s.Flag.Surface,
				boolStr(s.Enabled),
				s.Source,
				s.Flag.Introduced,
			)
		}
		t.Render()
		return nil
	},
}

var featureEnableCmd = &cobra.Command{
	Use:   "enable <flag>",
	Short: "Enable a feature flag (records an override)",
	Long: `Set an override that forces the flag to enabled=true. Persists to
.nself/features.json in the current project directory.

Example:
  nself feature enable nsentry-enabled`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setOverride(cmd, args[0], true)
	},
}

var featureDisableCmd = &cobra.Command{
	Use:   "disable <flag>",
	Short: "Disable a feature flag (records an override)",
	Long: `Set an override that forces the flag to enabled=false. Persists to
.nself/features.json in the current project directory.

Example:
  nself feature disable clawde-telemetry-opt-in`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return setOverride(cmd, args[0], false)
	},
}

func setOverride(cmd *cobra.Command, key string, enabled bool) error {
	reg, state, dir, err := loadRegAndState(cmd)
	if err != nil {
		return fmt.Errorf("feature %s: %w", verb(enabled), err)
	}
	if err := reg.SetOverride(state, key, enabled); err != nil {
		return fmt.Errorf("feature %s: %w", verb(enabled), err)
	}
	if err := featureflags.SaveState(dir, state); err != nil {
		return fmt.Errorf("feature %s: save state: %w", verb(enabled), err)
	}
	if enabled {
		ui.Success(fmt.Sprintf("Enabled feature flag: %s", key))
	} else {
		ui.Success(fmt.Sprintf("Disabled feature flag: %s", key))
	}
	return nil
}

func verb(enabled bool) string {
	if enabled {
		return "enable"
	}
	return "disable"
}

var featureStatusCmd = &cobra.Command{
	Use:   "status <flag>",
	Short: "Show the effective state for a single flag",
	Long: `Display the effective state for one flag — its registry entry plus
whether an override is in play.

Use --json for machine-readable output.

Example:
  nself feature status nfamily-coppa-strict`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		key := args[0]

		reg, state, _, err := loadRegAndState(cmd)
		if err != nil {
			return fmt.Errorf("feature status: %w", err)
		}
		st, ok := reg.ResolveOne(state, key)
		if !ok {
			return fmt.Errorf("feature status: unknown flag %q (run `nself feature list`)", key)
		}

		if jsonOut {
			b, err := json.MarshalIndent(st, "", "  ")
			if err != nil {
				return fmt.Errorf("feature status: marshal: %w", err)
			}
			fmt.Println(string(b))
			return nil
		}

		ui.CommandHeader("Feature Flag", st.Flag.Key)
		t := ui.NewTable("FIELD", "VALUE")
		t.AddRow("Key", st.Flag.Key)
		t.AddRow("Type", string(st.Flag.Type))
		t.AddRow("Surface", st.Flag.Surface)
		t.AddRow("Description", st.Flag.Description)
		t.AddRow("Default", boolStr(st.Flag.Default))
		t.AddRow("Enabled", boolStr(st.Enabled))
		t.AddRow("Source", st.Source)
		t.AddRow("Introduced", st.Flag.Introduced)
		if st.Flag.AutoKill != "" {
			t.AddRow("Auto-kill", st.Flag.AutoKill)
		}
		if st.Flag.Owner != "" {
			t.AddRow("Owner", st.Flag.Owner)
		}
		if st.Flag.DocsURL != "" {
			t.AddRow("Docs", st.Flag.DocsURL)
		}
		t.Render()
		return nil
	},
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func init() {
	// TRAP-09 mirror: guard against duplicate `feature` registration if a
	// future command file is ever added with the same Use string.
	for _, c := range RootCmd.Commands() {
		if c.Name() == "feature" {
			return
		}
	}

	featureListCmd.Flags().Bool("json", false, "Output as JSON")
	featureStatusCmd.Flags().Bool("json", false, "Output as JSON")

	featureCmd.AddCommand(
		featureListCmd,
		featureEnableCmd,
		featureDisableCmd,
		featureStatusCmd,
	)

	// CRITICAL: do not collide with the existing `flag` command (managed via
	// the runtime feature-flags plugin in internal/flags/). `feature` and
	// `flag` are intentionally distinct surfaces — see featureCmd Long help.
	for _, c := range RootCmd.Commands() {
		if strings.EqualFold(c.Name(), "feature") {
			return
		}
	}
	configCmd.AddCommand(featureCmd)
}

// Ensure strings import is referenced (used by duplicate-registration guard).
var _ = strings.EqualFold
