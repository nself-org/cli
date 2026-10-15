package commands

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker/dockertest"
	"github.com/spf13/cobra"
)

func TestRemainingComposeCallsCarryEnvFiles(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"stream", "services", "summary", "volume", "status", "plugin", "deploy", "logs"} {
		t.Run(name, func(t *testing.T) {
			root, check := dockertest.ProjectEnv(t, `{"Name":"plugin-notify"}`)
			t.Chdir(root)
			cmd := &cobra.Command{}
			addLogsFlags(cmd)
			cmd.SetContext(ctx)
			var err error
			switch name {
			case "stream":
				err = streamFilteredLogs(ctx, root, []string{"compose", "logs"}, LogsOptions{}, io.Discard)
			case "services":
				_, err = getRunningServices(ctx, root)
			case "summary":
				_, err = CollectLogSummary(ctx, root, "notify", 1)
			case "volume":
				_, err = SampleLogVolume(ctx, root, "notify", time.Second)
			case "status":
				err = runLogsStatus(cmd)
			case "plugin":
				_, err = resolveViaCompose("notify")
			case "deploy":
				err = runDeployLogs(cmd, nil)
			case "logs":
				err = runLogs(cmd, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			check()
		})
	}
}

func TestUpdateRestartCarriesEnvFiles(t *testing.T) {
	root, check := dockertest.ProjectEnv(t, "")
	t.Chdir(root)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"tag_name":"v99.0.0","html_url":"https://example.invalid/release"}`)
	}))
	defer srv.Close()
	old := githubLatestReleaseURL
	githubLatestReleaseURL = srv.URL
	t.Cleanup(func() { githubLatestReleaseURL = old })
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.Flags().Bool("admin", true, "")
	cmd.Flags().Bool("force", true, "")
	cmd.Flags().Bool("restart", true, "")
	if err := updateCmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	check()
}
