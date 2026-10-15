package stack

import "testing"

// Purpose: reject unsafe persisted run IDs before they reach recovery guidance.
// Inputs: journals with valid and corrupt identities. Outputs: load assertions.
// Constraints: exercise the durable load boundary. SPORT: D-0715.
func TestJournalRunIDCharset(t *testing.T) {
	for _, runID := range []string{"", "has space", "a/b", `a\b`, "a;echo", "$(echo x)", "a'b", "a\nb", "a\x00b", "a_b", "café", "Run-20261015-aZ09", "-"} {
		t.Run(runID, func(t *testing.T) {
			project := t.TempDir()
			j := newJournal(false)
			j.RunID = runID
			if err := saveJournal(project, j); err != nil {
				t.Fatal(err)
			}
			got, err := loadJournal(project)
			valid := runID == "Run-20261015-aZ09" || runID == "-"
			if !valid {
				if err == nil || got != nil {
					t.Fatalf("unsafe run ID %q accepted: %+v, %v", runID, got, err)
				}
				return
			}
			if err != nil || got == nil || got.RunID != runID {
				t.Fatalf("valid run ID %q rejected: %+v, %v", runID, got, err)
			}
		})
	}
}
