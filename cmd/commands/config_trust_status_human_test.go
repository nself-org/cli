package commands

import (
	"strings"
	"testing"
)

// TestConfigTrustStatusHumanWarnings pins D-0746's human warning verdict.
// Inputs: a fixture project without certificates, in both compatibility modes.
// Outputs: the real CLI exit status and human diagnostic text.
// Constraints: runPilot isolates HOME and clears inherited NSELF_* settings.
func TestConfigTrustStatusHumanWarnings(t *testing.T) {
	for _, tc := range []struct {
		name string
		v15  bool
		code int
	}{
		{name: "v1.5", v15: true, code: 2},
		{name: "v1.4", v15: false, code: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"trust", "status"}
			if tc.v15 {
				args = append([]string{"config"}, args...)
			}
			r := runPilot(t, cfgFixtureProject(t), tc.v15, nil, args...)
			if r.code != tc.code {
				t.Fatalf("trust status: exit %d, want %d\nstdout: %s\nstderr: %s", r.code, tc.code, r.stdout, r.stderr)
			}
			if !strings.Contains(r.stdout, "project cert — not found") || !strings.Contains(r.stderr, "Some trust components need attention.") {
				t.Fatalf("missing human warning: stdout %q, stderr %q", r.stdout, r.stderr)
			}
			const legacyError = "Error: trust warnings present (exit code 2)"
			if !tc.v15 && !strings.Contains(r.stderr, legacyError) {
				t.Fatalf("v1.4 stderr %q, want %q", r.stderr, legacyError)
			}
			if tc.v15 && strings.Contains(r.stdout+r.stderr, "trust warnings present (exit code 2)") {
				t.Fatalf("v1.5 printed the legacy error: stdout %q, stderr %q", r.stdout, r.stderr)
			}
		})
	}
}
