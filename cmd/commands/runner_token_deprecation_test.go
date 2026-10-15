package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestRunnerTokenDeprecation checks the public flag parser without provisioning
// a host. Deprecated argv input remains compatible and never echoes its value.
func TestRunnerTokenDeprecation(t *testing.T) {
	flag := runnerProvisionCmd.Flags().Lookup("token")
	if !strings.Contains(flag.Deprecated, "GITHUB_RUNNER_TOKEN") {
		t.Fatal("--token must be deprecated in favor of GITHUB_RUNNER_TOKEN")
	}
	original, changed := flag.Value.String(), flag.Changed
	t.Cleanup(func() {
		_ = flag.Value.Set(original)
		flag.Changed = changed
	})
	t.Setenv("GITHUB_RUNNER_TOKEN", "env-registration-secret")
	for _, tc := range []struct {
		name string
		args []string
		warn bool
	}{
		{"separate", []string{"--token", "argv-registration-secret"}, true},
		{"equals", []string{"--token=argv-registration-secret"}, true},
		{"empty", []string{"--token="}, true},
		{"environment", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "provision"}
			cmd.Flags().AddFlagSet(runnerProvisionCmd.Flags())
			var output bytes.Buffer
			cmd.SetErr(&output)
			cmd.SetOut(&output)
			if err := cmd.ParseFlags(tc.args); err != nil {
				t.Fatal(err)
			}
			warning := output.String()
			if got := strings.Contains(warning, "deprecated"); got != tc.warn {
				t.Fatalf("deprecation warning = %t, want %t: %q", got, tc.warn, warning)
			}
			if tc.warn && (!strings.Contains(warning, "GITHUB_RUNNER_TOKEN") || !strings.Contains(warning, "process")) {
				t.Fatalf("warning must explain the safer token source and argv exposure: %q", warning)
			}
			if strings.Contains(warning, "registration-secret") {
				t.Fatalf("warning exposed a token: %q", warning)
			}
		})
	}
}
