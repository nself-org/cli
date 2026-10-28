package commands

import (
	"strings"
	"testing"
)

func TestBackupDestinationHelp(t *testing.T) {
	for _, cmd := range []string{"stream", "schedule"} {
		c, _, err := backupCmd.Find([]string{cmd})
		if err != nil {
			t.Fatal(err)
		}
		f := c.Flags().Lookup("to")
		if f == nil {
			t.Fatalf("%s has no --to", cmd)
		}
		for _, scheme := range []string{"rclone", "path://", "host://"} {
			if !strings.Contains(f.Usage, scheme) {
				t.Errorf("%s --to help lacks %s: %s", cmd, scheme, f.Usage)
			}
		}
	}
}
