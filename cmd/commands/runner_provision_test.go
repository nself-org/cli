package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// Stop at argument validation so these tests never provision a host.
func TestRunnerProvisionTokenDeprecation(t *testing.T) {
	for _, tc := range []struct {
		name        string
		args        []string
		env         string
		wantWarning bool
		wantError   string
	}{
		{"flag", []string{"--token", "registration-secret"}, "", true, "--github-url is required"},
		{"empty flag", []string{"--token="}, "", true, "--github-url is required"},
		{"environment", nil, "environment-secret", false, "--github-url is required"},
		{"missing", []string{"--github-url", "https://github.com/example/repo"}, "", false, "a runner registration token is required: set GITHUB_RUNNER_TOKEN"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_RUNNER_TOKEN", tc.env)
			cmd := &cobra.Command{}
			cmd.Flags().StringSlice("host", nil, "")
			cmd.Flags().String("ssh-key", "", "")
			cmd.Flags().Int("instances", 1, "")
			cmd.Flags().String("install-root", "", "")
			cmd.Flags().String("github-url", "", "")
			cmd.Flags().StringSlice("labels", nil, "")
			cmd.Flags().String("token", "", "")
			var stderr bytes.Buffer
			cmd.SetErr(&stderr)
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			err := runRunnerProvision(cmd, nil)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			warning := stderr.String()
			if tc.wantWarning {
				for _, text := range []string{"--token is deprecated", "process listings", "shell history", "GITHUB_RUNNER_TOKEN"} {
					if !strings.Contains(warning, text) {
						t.Errorf("warning %q missing %q", warning, text)
					}
				}
			} else if warning != "" {
				t.Errorf("unexpected warning: %q", warning)
			}
			if strings.Contains(warning, "registration-secret") || strings.Contains(warning, "environment-secret") {
				t.Fatal("warning exposed token")
			}
		})
	}
}

func TestRunnerProvisionTokenHelpDeprecation(t *testing.T) {
	usage := runnerProvisionCmd.Flags().Lookup("token").Usage
	for _, text := range []string{"DEPRECATED", "GITHUB_RUNNER_TOKEN", "process listings", "shell history"} {
		if !strings.Contains(usage, text) {
			t.Errorf("token help %q missing %q", usage, text)
		}
	}
}
