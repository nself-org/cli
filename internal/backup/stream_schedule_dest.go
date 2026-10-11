package backup

// stream_schedule_dest.go — destination checks and unit-file writes for backup schedule.
//
// Purpose: make `backup schedule` accept exactly the destinations `backup stream` accepts, and refuse inline credentials, which would land in a unit file (D-0467); write the unit files with safe modes.
// Inputs: the --to and --heartbeat-to values, the project directory (for host:// inventory lookup) and the rendered unit texts.
// Outputs: nil, or an error that never echoes the value; the unit files in the unit directory.
// Constraints: validation runs before the dry-run branch and before anything is written. The service unit is 0600 (it can hold --to and --heartbeat-to), the timer 0644, and both modes are set with Chmod so a pre-existing file is tightened too.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/controlplane"
)

// checkScheduleDestination validates one destination flag value (flag names it
// in the error). Inline credentials are refused first with advice; then the
// value must parse as a destination, so a host:// server missing from the
// inventory is refused now, not at 02:00. Nothing is started.
func checkScheduleDestination(flag, v, projectDir string) error {
	if destinations.HasInlineCredentials(v) {
		return fmt.Errorf("%s carries inline options or credentials; the systemd unit file would hold them. Use a named remote from rclone.conf (rclone config), or define it in the environment (RCLONE_CONFIG_<NAME>_TYPE plus its keys) and pass it with --env-file, then give %s as <name>:<path>", flag, flag)
	}
	var inv *destinations.Inventory
	if destinations.KindOf(v) == destinations.KindHost {
		var err error
		if inv, err = controlplane.Load(projectDir); err != nil {
			return fmt.Errorf("%s: %v", flag, err)
		}
	}
	if _, err := destinations.Parse(v, inv); err != nil {
		return fmt.Errorf("%s: %v", flag, err)
	}
	return nil
}

// writeUnitFiles writes the service (0600) and timer (0644) units into
// unitDir, creating it. Both get an explicit Chmod because WriteFile keeps the
// mode of a file that already exists (and the umask trims a new one).
func writeUnitFiles(unitDir, service, timer string) error {
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", unitDir, err)
	}
	servicePath := filepath.Join(unitDir, "nself-backup-stream.service")
	timerPath := filepath.Join(unitDir, "nself-backup-stream.timer")
	if err := os.WriteFile(servicePath, []byte(service), 0600); err != nil {
		return fmt.Errorf("write service unit: %w", err)
	}
	if err := os.Chmod(servicePath, 0600); err != nil {
		return fmt.Errorf("restrict service unit: %w", err)
	}
	if err := os.WriteFile(timerPath, []byte(timer), 0644); err != nil {
		return fmt.Errorf("write timer unit: %w", err)
	}
	if err := os.Chmod(timerPath, 0644); err != nil {
		return fmt.Errorf("set timer unit mode: %w", err)
	}
	return nil
}
