package commands

// admin_start_scope_test.go — guards the blast radius of `nself admin start`.
//
// The defect: on first run `docker start <project>_admin` fails because the
// container does not exist yet, and the fallback shelled out to
// `nself start admin`. That reads like "start the admin service", but
// runStart takes `_ []string` and ignores its arguments entirely, so it
// booted the WHOLE stack — running the AI first-run wizard (which cannot
// install Ollama without systemd) and a whole-stack port preflight that
// flagged the project's OWN already-running containers as conflicts. That is
// what stopped the E2E golden path at step 11.

import (
	"os"
	"strings"
	"testing"
)

// TestAdminStart_DoesNotShellOutToNselfStart pins the fix: starting admin must
// not re-enter the stack boot sequence.
func TestAdminStart_DoesNotShellOutToNselfStart(t *testing.T) {
	src, err := os.ReadFile("admin.go")
	if err != nil {
		t.Fatalf("reading admin.go: %v", err)
	}
	body := string(src)

	for _, bad := range []string{
		`"nself", "start"`,
		`"nself","start"`,
	} {
		if strings.Contains(body, bad) {
			t.Errorf("admin.go shells out to %s; starting admin must bring up only "+
				"the admin service, not run the whole-stack boot sequence", bad)
		}
	}

	if !strings.Contains(body, "ComposeUpNoDeps") {
		t.Error("admin start no longer brings the admin service up directly; " +
			"expected a ComposeUpNoDeps call scoped to that one service")
	}
}

// TestStartCommand_RejectsPositionalArgs makes the silent-swallow impossible.
// runStart ignores positional args, so accepting them lets a caller believe a
// start was scoped to one service when it was not.
func TestStartCommand_RejectsPositionalArgs(t *testing.T) {
	if startCmd.Args == nil {
		t.Fatal("startCmd has no Args validator: positional arguments are silently ignored")
	}
	if err := startCmd.Args(startCmd, []string{"admin"}); err == nil {
		t.Error("`nself start admin` was accepted; runStart ignores the argument, " +
			"so it must be rejected rather than appearing to scope the start")
	}
	if err := startCmd.Args(startCmd, []string{}); err != nil {
		t.Errorf("`nself start` with no args must remain valid, got: %v", err)
	}
}
