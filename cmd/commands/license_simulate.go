package commands

import (
	"fmt"
	"strconv"

	"github.com/nself-org/cli/internal/license"

	"github.com/spf13/cobra"
)

var licenseSimulateCmd = &cobra.Command{
	Use:   "simulate-offline <days>",
	Short: "Simulate being offline for N days (testing only)",
	Long: `Simulate offline mode for testing grace period behavior.

Requires LICENSE_ALLOW_SIMULATION=true (disabled by default in production).
Backdates the license cache to appear as if the system has been offline
for the specified number of days.

The grace ladder is keyed on GraceSoftThreshold (72h) and GraceHardThreshold
(7 days), so the days below are chosen to land inside each band.

Examples:
  nself license simulate-offline 0    # Just went offline — silent
  nself license simulate-offline 5    # Past 72h — warning, still writable
  nself license simulate-offline 10   # Past 7 days — read-only
  nself license simulate-offline --clear  # Reset to current time`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clear, _ := cmd.Flags().GetBool("clear")
		if clear {
			if err := license.ClearSimulation(); err != nil {
				return err
			}
			fmt.Println("Simulation cleared. License cache restored to current time.")
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("specify number of days or use --clear")
		}

		days, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid days value: %w", err)
		}

		result, err := license.SimulateOffline(days)
		if err != nil {
			return err
		}

		fmt.Printf("Simulated %d days offline.\n", days)
		fmt.Printf("Grace state: %s\n", result.State)
		fmt.Printf("Can proceed: %v\n", result.CanProceed)
		fmt.Printf("Write allowed: %v\n", result.WriteAllowed)
		if result.Message != "" {
			fmt.Printf("Message: %s\n", result.Message)
		}

		banner := license.BannerAtWarning(*result)
		if banner != "" {
			fmt.Printf("\nBanner: %s\n", banner)
		}

		return nil
	},
}

func init() {
	licenseSimulateCmd.Flags().Bool("clear", false, "Clear simulation and restore cache to current time")
	licenseCmd.AddCommand(licenseSimulateCmd)
}
