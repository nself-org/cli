package errs

import (
	"strings"
	"testing"
)

// Purpose: keep default stack recovery guidance complete and ordered.
// Inputs: E139 registry fix. Outputs: recovery sequence assertions.
// Constraints: archive only after restoring and restarting. SPORT: D-0715.
func TestStackUpdateDefaultFixArchivesJournal(t *testing.T) {
	fix := Registry["E139"].DefaultFix
	previous := -1
	for _, step := range []string{"backup restore", "restart the previous images", "archive the journal", "before starting a new upgrade"} {
		index := strings.Index(fix, step)
		if index <= previous {
			t.Fatalf("E139 fix lacks ordered recovery step %q: %s", step, fix)
		}
		previous = index
	}
	if !strings.Contains(fix, "update-stack.<run_id>.json") {
		t.Fatalf("E139 fix lacks journal archive destination: %s", fix)
	}
}
