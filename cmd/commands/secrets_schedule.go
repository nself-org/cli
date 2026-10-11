package commands

// Purpose: The `nself secrets schedule`, `list-schedules`, and `verify`
// subcommands — creating/showing named rotation schedules and checking
// that a named secret is present in the store. Split out of secrets.go
// (CLI-R12); see secrets_core.go for the file-splitting rationale shared
// by every secrets_*.go file in this split.
// Inputs: cobra.Command args/flags per subcommand (secret name, rotation
// interval).
// Outputs: an updated schedule record, or a printed schedule/verify report.
// Constraints: pure move — no behavior changes.

import (
	"fmt"
	"os"
	"regexp"

	"github.com/nself-org/cli/internal/secrets"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

// everyPattern matches a well-formed rotation cadence: one or more digits
// (no sign, no leading zero beyond a bare "0") followed by a literal "d",
// and NOTHING else. fmt.Sscanf("%dd", ...) previously accepted this flag
// as a prefix match, so "90days" silently parsed as 90 (ignoring the
// trailing "ays") and "-90d" silently parsed as a negative cadence that
// SaveRotationState would then persist as a schedule that is either
// permanently overdue or never due — a rotation schedule nobody actually
// enforces is a silent security regression, not a cosmetic one.
var everyPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)d$`)

var secretsScheduleCmd = &cobra.Command{
	Use:   "schedule",
	Short: "Add a rotation schedule or show all schedule statuses",
	Long: `Add a named rotation schedule, or show the status of all tracked schedules.

If --secret and --every are provided, the schedule is created or updated.
Otherwise, the current schedule table is printed.

Examples:
  nself secrets schedule --secret JWT_SIGNING_KEY --every 90d
  nself secrets schedule`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		secretName, _ := cmd.Flags().GetString("secret")
		everyFlag, _ := cmd.Flags().GetString("every")

		// If both --secret and --every are supplied, create/update the schedule.
		if secretName != "" && everyFlag != "" {
			if !everyPattern.MatchString(everyFlag) {
				return fmt.Errorf("--every must be in format <N>d with a non-negative integer N (e.g. 90d), got %q", everyFlag)
			}
			var cadenceDays int
			if _, err := fmt.Sscanf(everyFlag, "%dd", &cadenceDays); err != nil {
				return fmt.Errorf("--every must be in format <N>d (e.g. 90d): %w", err)
			}
			if err := secrets.AddSchedule(cwd, secretName, cadenceDays, 7); err != nil {
				return err
			}
			if jsonEnvelopeOn(cmd) {
				setData(cmd, secretsScheduleData{Action: "set", Secret: secretName, CadenceDays: cadenceDays, Schedules: []secretsScheduleRow{}})
			}
			fmt.Printf("Rotation schedule set: %s every %dd.\n", secretName, cadenceDays)
			return nil
		}

		// Default: show schedule table.
		checks, err := secrets.CheckSchedule(cwd)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, secretsScheduleData{Action: "show", Schedules: newScheduleRows(checks)})
		}
		if len(checks) == 0 {
			fmt.Println("No rotation schedules configured.")
			return nil
		}
		tbl := ui.NewTable("Secret", "Cadence", "Window", "Next Due", "Due In", "Status")
		for _, c := range checks {
			dueIn := "-"
			if c.DueInDays >= 0 {
				dueIn = fmt.Sprintf("%dd", c.DueInDays)
			}
			nextDue := c.NextDue
			if len(nextDue) > 10 {
				nextDue = nextDue[:10]
			}
			if nextDue == "" {
				nextDue = "-"
			}
			tbl.AddRow(c.SecretName, fmt.Sprintf("%dd", c.CadenceDays),
				fmt.Sprintf("%dd", c.WindowDays), nextDue, dueIn, c.Status)
		}
		tbl.Render()
		return nil
	},
}

// secretsListSchedulesCmd is an alias for schedule (show only) per the F08 spec.
var secretsListSchedulesCmd = &cobra.Command{
	Use:   "list-schedules",
	Short: "List all rotation schedule statuses (alias for: secrets schedule)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		checks, err := secrets.CheckSchedule(cwd)
		if err != nil {
			return err
		}
		if jsonEnvelopeOn(cmd) {
			setData(cmd, secretsSchedulesData{Schedules: newScheduleRows(checks)})
		}
		if len(checks) == 0 {
			fmt.Println("No rotation schedules configured.")
			return nil
		}
		tbl := ui.NewTable("Secret", "Cadence", "Window", "Next Due", "Due In", "Status")
		for _, c := range checks {
			dueIn := "-"
			if c.DueInDays >= 0 {
				dueIn = fmt.Sprintf("%dd", c.DueInDays)
			}
			nextDue := c.NextDue
			if len(nextDue) > 10 {
				nextDue = nextDue[:10]
			}
			if nextDue == "" {
				nextDue = "-"
			}
			tbl.AddRow(c.SecretName, fmt.Sprintf("%dd", c.CadenceDays),
				fmt.Sprintf("%dd", c.WindowDays), nextDue, dueIn, c.Status)
		}
		tbl.Render()
		return nil
	},
}

// secretsVerifyCmd checks that a named secret is present in the store.
var secretsVerifyCmd = &cobra.Command{
	Use:   "verify <KEY>",
	Short: "Verify that a named secret exists in the store",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if err := secrets.VerifySecretExists(cwd, secretsEnvFlag, args[0]); err != nil {
			return err
		}
		fmt.Printf("Secret %s: PRESENT in %s environment.\n", args[0], secretsEnvFlag)
		return nil
	},
}

// secretsRotationLogCmd prints the rotation event log.
