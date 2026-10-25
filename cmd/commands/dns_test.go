package commands

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/sysexec"
)

// TestRerunWithOsascript_BacktickBlocked verifies that workdir values
// containing shell metacharacters are rejected before reaching the osascript
// call. In non-interactive contexts (tests run without a TTY) the function
// returns early, so we test the metacharacter guard by checking that an
// injection payload causes the expected error — not arbitrary command execution.
func TestRerunWithOsascript_BacktickBlocked(t *testing.T) {
	injectionPayloads := []struct {
		name    string
		workdir string
	}{
		{"backtick", "/tmp/proj`id`"},
		{"dollar-paren", "/tmp/proj$(id)"},
		{"semicolon", "/tmp/proj;id"},
		{"pipe", "/tmp/proj|id"},
		{"ampersand", "/tmp/proj&id"},
		{"redirect-gt", "/tmp/proj>/etc/hosts"},
		{"redirect-lt", "/tmp/proj</etc/hosts"},
		{"newline", "/tmp/proj\nid"},
		{"carriage-return", "/tmp/proj\rid"},
	}

	for _, tc := range injectionPayloads {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			err := rerunWithOsascript(ctx, tc.workdir)
			if err == nil {
				t.Fatalf("expected error for injection payload %q, got nil", tc.workdir)
			}
			// Must report the metacharacter guard, not a silent pass-through.
			if !strings.Contains(err.Error(), "metacharacter") &&
				!strings.Contains(err.Error(), "permission denied") {
				t.Errorf("error for %q = %q; want metacharacter or permission-denied message", tc.name, err.Error())
			}
		})
	}
}

// TestRerunWithOsascript_SafePathAccepted verifies that a workdir with only
// safe characters does not trip the metacharacter guard. In CI (non-TTY) the
// function still returns an error (no admin dialog), but it must be a
// permission-denied error — not a metacharacter-guard error.
func TestRerunWithOsascript_SafePathAccepted(t *testing.T) {
	// Hermetic: a fake runner, so a TTY run of this test can never reach the
	// real osascript admin dialog.
	t.Setenv("NSELF_HEADLESS", "")
	t.Setenv("CI", "")
	defer sysexec.Set(&sysexec.Fake{})()
	ctx := context.Background()
	err := rerunWithOsascript(ctx, "/Users/admin/my-project_v1.0")
	if err == nil {
		// In a TTY with osascript available this might succeed. In CI it won't.
		return
	}
	if strings.Contains(err.Error(), "metacharacter") {
		t.Errorf("safe path should not trigger metacharacter guard, got: %v", err)
	}
}

// TestRerunWithOsascript_NonInteractiveReturnsEarly verifies that a
// non-interactive context (cancelled context or non-TTY) returns immediately
// without executing the injection payload.
func TestRerunWithOsascript_NonInteractiveReturnsEarly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled
	err := rerunWithOsascript(ctx, "/tmp/safe-project")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

// TestDNSSetupHeadlessSkipEnvelope: under headless the /etc/hosts write is
// skipped, the command still exits 0 (start_config.go continues), but the v1.5
// envelope must say ok:false instead of claiming success (D-0830).
func TestDNSSetupHeadlessSkipEnvelope(t *testing.T) {
	run := runPilot(t, cfgFixtureProject(t), true, []string{"NSELF_HEADLESS=1"}, "config", "trust", "dns", "--json")
	if run.code != 0 {
		t.Fatalf("headless skip must exit 0, got %d\n%s", run.code, run.stderr)
	}
	data := validateEnvelope(t, run.stdout, "config trust dns")
	if data["ok"] != false {
		t.Errorf("headless-skipped dns setup: data = %v, want ok:false", data)
	}
}
