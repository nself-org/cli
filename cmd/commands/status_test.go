package commands

// Purpose: D-0180. The `status` help must not claim exit 1 is only "Error
// running checks": v1.4 mode also exits 1 for services still starting, and
// v1.5 moves that state to 11.

import (
	"strings"
	"testing"
)

func TestStatusHelp_ExitCodeOneDocumented(t *testing.T) {
	long := statusCmd.Long
	for _, want := range []string{
		"1  Error running checks (v1.4 mode only: also services still starting)",
		"11  Services still starting",
		"1 then means only an error running checks",
	} {
		if !strings.Contains(long, want) {
			t.Errorf("status help missing %q", want)
		}
	}
}
