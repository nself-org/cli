package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/cmdlog"
	"github.com/nself-org/cli/internal/errs"
	"github.com/spf13/cobra"
)

func TestCommandLogLifecycleChild(t *testing.T) {
	if os.Getenv("NSELF_TEST_LOG_CHILD") != "1" {
		t.Skip("child fixture")
	}
	c := &cobra.Command{Use: "log-fixture", RunE: func(c *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		if _, err := os.Stat(filepath.Join(home, ".nself/logs/nself.log")); !os.IsNotExist(err) {
			return fmt.Errorf("log was written before command completion")
		}
		mode, _ := c.Flags().GetString("mode")
		switch mode {
		case "error":
			secret, _ := c.Flags().GetString("api-token")
			return errs.New("E426", "fixture failure "+secret)
		case "state":
			return errs.Exit(12)
		}
		return nil
	}}
	c.Flags().String("api-token", "", "fixture")
	c.Flags().String("mode", "", "fixture")
	RootCmd.AddCommand(c)
	os.Args = []string{"nself", "log-fixture", "--api-token", "FAKE-log-lifecycle-secret", "--mode", os.Getenv("NSELF_TEST_LOG_MODE")}
	err := Execute()
	os.Exit(errs.ExitCodeFor(err))
}

func TestCommandLogLifecycle(t *testing.T) {
	for _, tc := range []struct {
		mode string
		code int
	}{{"success", 0}, {"error", errs.ExitAuthError}, {"state", 12}} {
		t.Run(tc.mode, func(t *testing.T) {
			home := t.TempDir()
			child := exec.Command(os.Args[0], "-test.run=^TestCommandLogLifecycleChild$")
			child.Dir = t.TempDir()
			child.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "NSELF_V15=0", "NSELF_CMD_LOG_ENABLED=1", "NSELF_TEST_LOG_CHILD=1", "NSELF_TEST_LOG_MODE="+tc.mode)
			err := child.Run()
			code := 0
			if err != nil {
				if e, ok := err.(*exec.ExitError); ok {
					code = e.ExitCode()
				} else {
					t.Fatal(err)
				}
			}
			if code != tc.code {
				t.Fatalf("exit = %d, want %d", code, tc.code)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".nself/logs/nself.log"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(string(raw), "\n") != 1 {
				t.Fatalf("want one entry, got %s", raw)
			}
			var entry cmdlog.Entry
			if err := json.Unmarshal(raw, &entry); err != nil {
				t.Fatal(err)
			}
			if entry.ExitCode != tc.code {
				t.Fatalf("logged exit = %d, want %d", entry.ExitCode, tc.code)
			}
			if strings.Contains(string(raw), "FAKE-log-lifecycle-secret") {
				t.Fatal("fixture secret reached command log")
			}
			if tc.mode == "error" && (!strings.Contains(entry.Error, "fixture failure") || !strings.Contains(entry.Error, "[REDACTED]")) {
				t.Fatalf("error missing or not scrubbed: %s", raw)
			}
		})
	}
}
