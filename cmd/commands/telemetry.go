package commands

// telemetry.go — `nself telemetry` command group (Q07 / S65)
//
// Manages CLI telemetry preference and displays anonymous install-ID.
//
// Commands:
//   nself telemetry status   — show enabled/disabled state + install-ID
//   nself telemetry on       — enable telemetry (writes to ~/.nself/config.toml)
//   nself telemetry off      — disable telemetry (writes to ~/.nself/config.toml)
//
// Environment override:
//   NSELF_TELEMETRY=0          disables telemetry regardless of config.
//   NSELF_TELEMETRY_OPT_OUT=1  legacy disable override.
//
// Privacy policy: https://nself.org/legal/privacy

import (
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/telemetry"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage CLI telemetry preferences",
	Long: `Manage CLI telemetry opt-in/out preferences.

Anonymous usage events (commands run, errors, durations) are sent to
ping.nself.org/telemetry. No PII, no file paths, no command arguments.

Disable at any time:
  nself telemetry off
  NSELF_TELEMETRY=0 nself <cmd>

Privacy policy: https://nself.org/legal/privacy`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself telemetry` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

var telemetryStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current telemetry state and anonymous install-ID",
	Long: `Print whether telemetry is currently enabled or disabled, and display
the anonymous install-ID used to de-duplicate events server-side.

Sources checked in priority order:
  1. NSELF_TELEMETRY=0 environment variable (highest priority)
  2. NSELF_TELEMETRY_OPT_OUT=1 environment variable (legacy)
  3. ~/.config/nself/telemetry file
  4. ~/.nself/config.toml [telemetry] enabled = <bool>
  5. Default: enabled

The install-ID is a random UUID stored at ~/.config/nself/install-id.
It contains no personal data and is never linked to your account.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		enabled := telemetry.IsEnabled()
		installID := telemetry.LoadOrCreateInstallID()
		pref := config.GetTelemetryPreference()

		state := "enabled"
		if !enabled {
			state = "disabled"
		}

		fmt.Printf("Telemetry:  %s\n", state)
		fmt.Printf("Install-ID: %s\n", installID)
		fmt.Printf("Source:     %s\n", pref.Source)

		if os.Getenv("NSELF_TELEMETRY") == "0" {
			fmt.Println()
			fmt.Println("Note: NSELF_TELEMETRY=0 is set. Unset it to re-enable.")
		} else if os.Getenv("NSELF_TELEMETRY_OPT_OUT") == "1" {
			fmt.Println()
			fmt.Println("Note: NSELF_TELEMETRY_OPT_OUT=1 is set. This overrides any stored preference.")
		}

		fmt.Println()
		fmt.Println("Privacy policy: https://nself.org/legal/privacy")
		return nil
	},
}

var telemetryOffCmd = &cobra.Command{
	Use:   "off",
	Short: "Disable telemetry (writes to ~/.nself/config.toml)",
	Long: `Disable telemetry by writing enabled = false to ~/.nself/config.toml.

You can also set NSELF_TELEMETRY=0 in the environment to disable without
modifying the config file.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NSELF_TELEMETRY") == "0" || os.Getenv("NSELF_TELEMETRY_OPT_OUT") == "1" {
			fmt.Println("Telemetry env override is already active. Writing preference to config too.")
		}
		if err := config.SetTelemetryEnabled(false); err != nil {
			return fmt.Errorf("saving telemetry preference: %w", err)
		}
		ui.Success("Telemetry disabled. Preference saved to ~/.nself/config.toml")
		return nil
	},
}

var telemetryOnCmd = &cobra.Command{
	Use:   "on",
	Short: "Enable telemetry (writes to ~/.nself/config.toml)",
	Long: `Enable telemetry by writing enabled = true to ~/.nself/config.toml.

If NSELF_TELEMETRY=0 or NSELF_TELEMETRY_OPT_OUT=1 is set in the environment,
that env var takes precedence. Unset it to fully re-enable.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("NSELF_TELEMETRY") == "0" {
			fmt.Fprintln(os.Stderr, "Warning: NSELF_TELEMETRY=0 is set in the environment.")
			fmt.Fprintln(os.Stderr, "Telemetry will remain disabled until that variable is unset.")
			fmt.Fprintln(os.Stderr, "Writing the preference to config anyway.")
		} else if os.Getenv("NSELF_TELEMETRY_OPT_OUT") == "1" {
			fmt.Fprintln(os.Stderr, "Warning: NSELF_TELEMETRY_OPT_OUT=1 is set in the environment.")
			fmt.Fprintln(os.Stderr, "Telemetry will remain disabled until that variable is unset.")
			fmt.Fprintln(os.Stderr, "Writing the preference to config anyway.")
		}
		if err := config.SetTelemetryEnabled(true); err != nil {
			return fmt.Errorf("saving telemetry preference: %w", err)
		}
		ui.Success("Telemetry enabled. Preference saved to ~/.nself/config.toml")
		return nil
	},
}

func init() {
	telemetryCmd.AddCommand(telemetryStatusCmd)
	telemetryCmd.AddCommand(telemetryOffCmd)
	telemetryCmd.AddCommand(telemetryOnCmd)
	RootCmd.AddCommand(telemetryCmd)
}
