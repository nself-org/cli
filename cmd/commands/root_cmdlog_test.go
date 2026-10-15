package commands

import (
	"context"
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

// TestCommandLogLifecycle exercises Execute in isolated processes, including
// failures Cobra does not send through PersistentPostRunE.
func TestCommandLogLifecycle(t *testing.T) {
	for _, mode := range []string{"success", "run-error", "pre-error", "silent", "context", "post-error"} {
		t.Run(mode, func(t *testing.T) {
			home := t.TempDir()
			child := exec.Command(os.Args[0], "-test.run=^TestCommandLogLifecycleChild$")
			child.Dir = home
			child.Env = append(os.Environ(), "HOME="+home, "NSELF_TEST_LOG_MODE="+mode,
				"NSELF_V15=0", "NSELF_CMD_LOG_ENABLED=true", "NSELF_ALLOW_SOURCE_DIR=1")
			if out, err := child.CombinedOutput(); err != nil {
				t.Fatalf("child: %v\n%s", err, out)
			}
			raw, err := os.ReadFile(filepath.Join(home, ".nself", "logs", "nself.log"))
			if err != nil {
				t.Fatal(err)
			}
			var entry cmdlog.Entry
			if err := json.Unmarshal(raw, &entry); err != nil {
				t.Fatalf("want exactly one entry: %v: %s", err, raw)
			}
			want := 3
			if mode == "success" {
				want = 0
			} else if mode == "silent" || mode == "context" {
				want = 7
			}
			if entry.ExitCode != want || (want != 0 && entry.Error == "") {
				t.Errorf("entry = %+v, want exit %d and failure detail", entry, want)
			}
			if strings.Contains(string(raw), "FAKE-d0652-secret") {
				t.Fatalf("secret leaked: %s", raw)
			}
		})
	}
}

func TestCommandLogLifecycleChild(t *testing.T) {
	mode := os.Getenv("NSELF_TEST_LOG_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	os.Args = []string{"nself", "secrets", "set", "KEY", "FAKE-d0652-secret"}
	failure := errs.New("E426", "refused FAKE-d0652-secret")
	secretsSetCmd.RunE = func(cmd *cobra.Command, args []string) error {
		path := filepath.Join(os.Getenv("HOME"), ".nself", "logs", "nself.log")
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			return fmt.Errorf("log written before command finished: %v", err)
		}
		switch mode {
		case "run-error":
			return failure
		case "silent":
			return errs.Exit(7)
		case "context":
			RootCmd.SetContext(context.WithValue(cmd.Context(), exitCodeKey, 7))
		}
		return nil
	}
	if mode == "pre-error" {
		secretsSetCmd.PreRunE = func(*cobra.Command, []string) error { return failure }
	}
	if mode == "post-error" {
		secretsSetCmd.PostRunE = func(*cobra.Command, []string) error { return failure }
	}
	err := Execute()
	if mode == "success" && err != nil {
		t.Fatal(err)
	}
}
