package commands

// Purpose: the "nself account devices" subcommand and its RunE. Inputs are
// the cobra command/args; outputs are printed device info or an error.
// Constraints: split out of account.go (CLI-R12) as a pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/auth"
	"github.com/nself-org/cli/internal/ui"
	"github.com/spf13/cobra"
)

var accountDevicesCmd = &cobra.Command{
	Use:   "devices",
	Short: "List and revoke registered devices",
	Long: `List all devices registered to your account.

Flags:
  --revoke <device-id>   Revoke a device session
  --force                Required to revoke the current device
  --json                 Output raw JSON`,
	SilenceUsage: true,
	RunE:         runAccountDevices,
}

func init() {
	accountDevicesCmd.Flags().String("revoke", "", "Device ID to revoke")
	accountDevicesCmd.Flags().Bool("force", false, "Required to revoke the current device")
	accountDevicesCmd.Flags().Bool("json", false, "Output raw JSON")
}

func runAccountDevices(cmd *cobra.Command, _ []string) error {
	revoke, _ := cmd.Flags().GetString("revoke")
	force, _ := cmd.Flags().GetBool("force")
	jsonOut, _ := cmd.Flags().GetBool("json")
	if err := accountJSONGuard(cmd, "revoke"); err != nil {
		return err
	}

	af, err := requireLogin()
	if err != nil {
		return err
	}

	// --revoke
	if revoke != "" {
		// Check if this is the current device without --force.
		devices, listErr := auth.GetDevices(cmdCtx(cmd), af.AccessToken)
		if listErr == nil {
			for _, d := range devices {
				if d.ID == revoke && d.IsCurrent && !force {
					return fmt.Errorf("cannot revoke the current device without --force")
				}
			}
		}

		if !promptConfirm(fmt.Sprintf("Revoke device %s? [y/N]: ", revoke)) {
			fmt.Println("Canceled.")
			return nil
		}
		if err := auth.RevokeDevice(cmdCtx(cmd), af.AccessToken, revoke); err != nil {
			return handleAuthError(err)
		}
		ui.Success(fmt.Sprintf("Device %s revoked.", revoke))
		return nil
	}

	// Default: list devices.
	devices, err := auth.GetDevices(cmdCtx(cmd), af.AccessToken)
	if err != nil {
		return handleAuthError(err)
	}

	if jsonEnvelopeOn(cmd) {
		return emitEnv(cmd, orEmpty(devices), true)
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(devices)
	}

	if len(devices) == 0 {
		fmt.Println("No registered devices found.")
		return nil
	}

	tbl := ui.NewTable("DEVICE ID", "NAME", "OS", "LAST ACTIVE")
	for _, d := range devices {
		lastActive := d.LastActive
		if len(lastActive) >= 10 {
			lastActive = lastActive[:10]
		}
		name := d.Name
		if d.IsCurrent {
			name = name + " (current)"
		}
		idShort := d.ID
		if len(idShort) > 8 {
			idShort = idShort[:8]
		}
		tbl.AddRow(idShort, name, d.OS, lastActive)
	}
	tbl.Render()
	return nil
}

// ── account transfer ─────────────────────────────────────────────────────────
