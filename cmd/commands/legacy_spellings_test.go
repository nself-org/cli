package commands

// legacy_spellings_test.go — Regression guard for P6-E2-W1-S1-T14.
//
// Purpose: BUILD-LEDGER.md Finding #11 claimed `nself release-status` was a
//   dead-ended regression (R09's argv rewrite feeding a command R11 later
//   extracted to a plugin). Live reproduction on 2026-08-31 disproved that:
//   both `nself release-status` and `nself release status` fail identically
//   because the `release` plugin simply is not installed — the same state
//   every other CLI-R11-extracted command is in until the user installs it.
//   These tests lock that verified-correct behaviour so a future change to
//   either the legacy-spelling table or the plugin proxy cannot silently
//   reintroduce a real dead-end (the rewrite producing a DIFFERENT failure
//   than the direct spelling, rather than the same one).
// Inputs:  os.Args (direct splice test), a scratch project dir with no
//   `release` plugin installed (end-to-end equivalence test).
// Outputs: assertions on the rewritten argv and on the two invocations'
//   observable failure.
// Constraints: does not assert byte-identical combined output. The legacy
//   spelling legitimately prints one extra "[DEPRECATED] 'nself
//   release-status'" line before the shared "[DEPRECATED] 'nself release'"
//   line the direct spelling also prints — two individually-accurate stacked
//   notices, not a bug. Reducing that to a single notice is an explicit
//   out-of-scope item for this ticket (a product decision about consolidating
//   multi-hop deprecation messages, not a correctness fix). What this test
//   demands instead is that both paths reach the SAME final failure: the
//   proxy's "Plugin error: ... no plugin named ... is installed" line and the
//   same exit code. A real dead-end (Finding #11's claim) would show up here
//   as the legacy spelling failing differently — or not failing at all —
//   compared to the direct spelling, which this test would catch.
// SPORT: CLI-CMD-RELEASE-STATUS-001

import (
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestRewriteLegacyInvocation_ReleaseStatusSplicesArgs locks the argv-splice
// behaviour itself, independent of plugin installation state: this is pure
// argument rewriting and must not regress even if the release plugin's
// install status changes.
func TestRewriteLegacyInvocation_ReleaseStatusSplicesArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "release-status with a flag",
			args: []string{"nself", "release-status", "--flag"},
			want: []string{"nself", "release", "status", "--flag"},
		},
		{
			name: "release-status with no extra args",
			args: []string{"nself", "release-status"},
			want: []string{"nself", "release", "status"},
		},
	}

	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			os.Args = append([]string(nil), tc.args...)

			got := rewriteLegacyInvocation()

			if got != "release-status" {
				t.Fatalf("rewriteLegacyInvocation() returned %q, want %q", got, "release-status")
			}
			if len(os.Args) != len(tc.want) {
				t.Fatalf("os.Args = %v, want %v", os.Args, tc.want)
			}
			for i := range tc.want {
				if os.Args[i] != tc.want[i] {
					t.Fatalf("os.Args = %v, want %v", os.Args, tc.want)
				}
			}
		})
	}
}

// TestReleaseStatusLegacyAndDirectSpellingsFailIdentically is the test that
// would have caught Finding #11's premise being wrong in the first place (or
// would catch a REAL future divergence between the two paths): it runs the
// built CLI's actual dispatch logic for both `release-status` and `release
// status` against a project directory with no `release` plugin installed,
// and asserts both reach the same final failure.
func TestReleaseStatusLegacyAndDirectSpellingsFailIdentically(t *testing.T) {
	compattest.Set(t, false) // D-0275: Execute() under v1.5 reshapes the shared root command and breaks every registration test after it
	runFromScratchDir(t)
	t.Setenv("HOME", t.TempDir()) // ensure no release plugin is "installed" via a real ~/.nself

	const wantFailure = `unknown command "release", and no plugin named "release" is installed`

	run := func(args []string) (out string, err error) {
		origArgs := os.Args
		t.Cleanup(func() { os.Args = origArgs })
		os.Args = append([]string{"nself"}, args...)
		out = captureStderr(t, func() { err = Execute() })
		return out, err
	}

	legacyOut, legacyErr := run([]string{"release-status"})
	directOut, directErr := run([]string{"release", "status"})

	if legacyErr == nil || directErr == nil {
		t.Fatalf("expected both invocations to fail: legacy=%v direct=%v", legacyErr, directErr)
	}
	if !strings.Contains(legacyOut, wantFailure) {
		t.Fatalf("legacy spelling did not reach the plugin-not-installed failure:\n%s", legacyOut)
	}
	if !strings.Contains(directOut, wantFailure) {
		t.Fatalf("direct spelling did not reach the plugin-not-installed failure:\n%s", directOut)
	}

	// The legacy path legitimately prints one additional deprecation line
	// (the rename notice, on top of the extraction notice both paths share) —
	// see the file-level comment. What must NOT differ is the failure itself.
	lastLegacy := lastNonEmptyLine(legacyOut)
	lastDirect := lastNonEmptyLine(directOut)
	if lastLegacy != lastDirect {
		t.Fatalf("legacy and direct spellings ended in different failures:\n  legacy: %q\n  direct: %q", lastLegacy, lastDirect)
	}
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return lines[i]
		}
	}
	return ""
}
