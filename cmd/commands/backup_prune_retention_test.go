package commands

// Purpose: `nself backup prune` must honour BACKUP_RETENTION_* config when no
// --keep-* flag is passed (D-0717). Inputs: a throwaway cobra command with the
// real flag defaults and a config carrying retention values. Outputs: none.

import (
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/spf13/cobra"
)

func newPruneRetentionCmd() *cobra.Command {
	c := &cobra.Command{Use: "prune"}
	c.Flags().Int("keep-daily", 7, "")
	c.Flags().Int("keep-weekly", 4, "")
	c.Flags().Int("keep-monthly", 12, "")
	return c
}

func TestPruneRetention(t *testing.T) {
	cfg := &config.Config{}
	cfg.Backup.RetentionDaily, cfg.Backup.RetentionWeekly, cfg.Backup.RetentionMonthly = 14, 8, 24

	t.Run("config applies without flags", func(t *testing.T) {
		d, w, m := pruneRetention(newPruneRetentionCmd(), cfg)
		if d != 14 || w != 8 || m != 24 {
			t.Fatalf("got %d/%d/%d, want config 14/8/24", d, w, m)
		}
	})
	t.Run("flag beats config", func(t *testing.T) {
		c := newPruneRetentionCmd()
		if err := c.Flags().Parse([]string{"--keep-daily", "2"}); err != nil {
			t.Fatal(err)
		}
		d, w, m := pruneRetention(c, cfg)
		if d != 2 || w != 8 || m != 24 {
			t.Fatalf("got %d/%d/%d, want 2/8/24", d, w, m)
		}
	})
	t.Run("flag defaults when config empty", func(t *testing.T) {
		d, w, m := pruneRetention(newPruneRetentionCmd(), &config.Config{})
		if d != 7 || w != 4 || m != 12 {
			t.Fatalf("got %d/%d/%d, want 7/4/12", d, w, m)
		}
	})
}
