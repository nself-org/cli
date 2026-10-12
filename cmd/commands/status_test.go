package commands

// Purpose: D-0180. The `status` help must match the exit codes the code
// returns: 1 is only an error running checks (a starting service counts as
// unhealthy in v1.4 mode, exit 2), and v1.5 moves unhealthy/starting to 10/11.

import (
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/health"
)

func TestStatusHelp_ExitCodesDocumented(t *testing.T) {
	long := statusCmd.Long
	for _, want := range []string{
		"1  Error running checks",
		"2  One or more services unhealthy",
		"1 still means an error running\nchecks",
		"11  Services still starting",
		"without NSELF_V15 such a service\n      counts as unhealthy and the exit code is 2",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("status help missing %q", want)
		}
	}
}

// TestStatusExit_StartingMatchesHelp pins what the help says: a starting
// service exits 2 in v1.4 mode and 11 in v1.5 mode; exit 1 is never requested.
func TestStatusExit_StartingMatchesHelp(t *testing.T) {
	results := []health.HealthResult{{Service: "postgres", Status: "healthy"}, {Service: "hasura", Status: "starting"}}
	t.Run("v1.4", func(t *testing.T) {
		t.Setenv("NSELF_V15", "0")
		if got := statusExit(results, true, false); got != 2 {
			t.Errorf("v1.4 starting = %d, want 2", got)
		}
	})
	t.Run("v1.5", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		if got := statusExit(results, true, false); got != 11 {
			t.Errorf("v1.5 starting = %d, want 11", got)
		}
		failed := []health.HealthResult{{Service: "hasura", Status: "unhealthy"}}
		if got := statusExit(failed, true, false); got != 10 {
			t.Errorf("v1.5 unhealthy = %d, want 10", got)
		}
	})
}
