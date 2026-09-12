package commands

// Purpose: `nself server resize` handler — changes a server's type
// (CPU/RAM/disk). Refuses a disk-shrinking resize with a plain-English
// explanation instead of Hetzner's raw "invalid_input" error (server.Resize
// does the actual disk-size comparison; this file only reports it).
// Inputs: --id, --type, --upgrade-disk, --token, --token-env, --json.
// Outputs: printed confirmation of the started action; a non-nil error
// (including server.ErrDiskShrink) on validation or provider failure.

import (
	"errors"
	"fmt"

	"github.com/nself-org/cli/internal/server"
	"github.com/nself-org/cli/internal/ui"

	"github.com/spf13/cobra"
)

var serverResizeCmd = &cobra.Command{
	Use:   "resize",
	Short: "Change a server's type (CPU/RAM/disk)",
	Long: `Change a server's type (e.g. cx22 -> cx41).

Hetzner Cloud has no API to shrink a server's disk. If --type names a type
with a smaller disk than the server currently has, this command refuses and
explains the only supported path: snapshot the current server, provision a
new server of the smaller type, restore from the snapshot, then destroy the
original with 'nself server destroy'.`,
	Example: `  nself server resize --id 12345 --type cx41
  nself server resize --id 12345 --type cx41 --upgrade-disk`,
	RunE: runServerResize,
}

func init() {
	f := serverResizeCmd.Flags()
	f.Int64("id", 0, "server ID (required)")
	f.String("type", "", "target server type, e.g. cx41 (required)")
	f.Bool("upgrade-disk", false, "also grow the disk to match the new type (irreversible)")
	f.Bool("json", false, "output as JSON")
}

func runServerResize(cmd *cobra.Command, args []string) error {
	id, _ := cmd.Flags().GetInt64("id")
	targetType, _ := cmd.Flags().GetString("type")
	upgradeDisk, _ := cmd.Flags().GetBool("upgrade-disk")
	jsonOut, _ := cmd.Flags().GetBool("json")

	client, err := newServerClient(cmd)
	if err != nil {
		return err
	}

	if !jsonOut {
		ui.CommandHeader("nself server resize", fmt.Sprintf("server %d -> %s", id, targetType))
	}

	action, err := server.Resize(cmd.Context(), client, server.ResizeRequest{
		ServerID: id, TargetType: targetType, UpgradeDisk: upgradeDisk,
	})
	if err != nil {
		if errors.Is(err, server.ErrDiskShrink) {
			return err // already carries the full explanation, don't wrap again
		}
		return fmt.Errorf("resize server %d: %w", id, err)
	}

	if jsonOut {
		return ui.PrintJSON(action)
	}

	ui.Success(fmt.Sprintf("Started resize of server %d to %s (action %d, status %s)", id, targetType, action.ID, action.Status))
	return nil
}
