package commands

// Purpose: `nself env <unknown>` must fail, not print help and exit 0.
//
// Inputs:  argv for the env command.
// Outputs: assertions on the returned error.
// Constraints: regression guard. envCmd's RunE returns cmd.Help(), so before
// Args: cobra.NoArgs an unknown subcommand fell through to the parent, printed
// the Long description on stdout and exited 0. The golden path ran
// `nself env get NSELF_API_URL`, captured that prose as a base URL, and curled
// it — a silent wrong answer rather than a loud failure.

import (
	"strings"
	"testing"
)

func TestEnvCmd_RejectsUnknownSubcommand(t *testing.T) {
	for _, argv := range [][]string{
		{"get", "NSELF_API_URL"},
		{"bogus-subcommand"},
		{"set", "FOO", "bar"},
	} {
		t.Run(strings.Join(argv, "_"), func(t *testing.T) {
			envCmd.SetArgs(argv)
			t.Cleanup(func() { envCmd.SetArgs(nil) })

			if err := envCmd.Args(envCmd, argv); err == nil {
				t.Errorf("nself env %v must be rejected, got nil error", argv)
			}
		})
	}
}

func TestEnvCmd_BareInvocationStillAllowed(t *testing.T) {
	// `nself env` with no args must keep printing help — NoArgs permits zero.
	if err := envCmd.Args(envCmd, []string{}); err != nil {
		t.Errorf("bare `nself env` must be allowed, got %v", err)
	}
}

func TestEnvCmd_RealSubcommandsStillRegistered(t *testing.T) {
	// NoArgs on the parent must not shadow the actual subcommands.
	want := map[string]bool{"use": false, "list": false, "show": false, "diff": false}
	for _, c := range envCmd.Commands() {
		name := strings.Fields(c.Use)[0]
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q is no longer registered under env", name)
		}
	}
}
