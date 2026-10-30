package commands

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker/dockertest"
	"github.com/spf13/cobra"
)

func TestD0747CommandComposeCallsCarryEnvFiles(t *testing.T) {
	root, calls := dockertest.ComposeProject(t, "plugin-notify\n")
	t.Chdir(root)
	ctx := context.Background()
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	steps := []struct {
		name string
		run  func() error
	}{
		{"logs stream", func() error {
			return streamFilteredLogs(ctx, root, []string{"compose", "logs"}, LogsOptions{}, io.Discard)
		}},
		{"logs services", func() error { _, err := getRunningServices(ctx, root); return err }},
		{"logs summary", func() error { _, err := CollectLogSummary(ctx, root, "nginx", 10); return err }},
		{"logs volume", func() error { _, err := SampleLogVolume(ctx, root, "nginx", time.Second); return err }},
		{"logs status", func() error { return runLogsStatus(cmd) }},
		{"deploy logs", func() error { return runDeployLogs(cmd, nil) }},
		{"reset", func() error { return resetComposeDown(ctx, root) }},
		{"update restart", func() error { return restartUpdatedProject(ctx, root) }},
		{"remote logs command", func() error { return exec.Command("sh", "-c", remoteProjectLogsCommand(root)).Run() }},
		{"plugin lookup", func() error { _, _ = resolveViaCompose("notify"); return nil }},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
	}
	if got := calls(); len(got) != len(steps) {
		t.Fatalf("got %d calls, want %d", len(got), len(steps))
	}
}
