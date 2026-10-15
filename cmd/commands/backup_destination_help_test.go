package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBackupDestinationHelp(t *testing.T) {
	for _, cmd := range []*cobra.Command{backupStreamCmd, backupScheduleCmd} {
		t.Run(cmd.Name(), func(t *testing.T) {
			flag := cmd.Flags().Lookup("to")
			if flag == nil {
				t.Fatal("missing --to flag")
			}
			for _, term := range []string{"rclone", "path://", "host://"} {
				if !strings.Contains(flag.Usage, term) {
					t.Errorf("--to help %q must describe %q destinations", flag.Usage, term)
				}
			}
		})
	}
}
