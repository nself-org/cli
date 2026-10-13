package ledger

// bootstrap_invalid_test.go — D-0475: Sources.Invalid notes are reported by
// BootstrapReport and never block a persist.

import (
	"strings"
	"testing"
)

func TestInvalidNotesAreWarningsNotRefusals(t *testing.T) {
	s := NewStore(t.TempDir())
	s.Sources = func() (Sources, error) {
		src := fixtureSources()
		src.Invalid = []string{"nodesc: missing description; recorded by name"}
		return src, nil
	}
	var warned []string
	s.Warn = func(m string) { warned = append(warned, m) }
	if _, err := s.EnsureBootstrapped(); err != nil {
		t.Fatalf("EnsureBootstrapped = %v, want success despite Invalid", err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "fails validation and is recorded from its raw fields: nodesc:") {
		t.Errorf("warnings = %q", warned)
	}
	_, report := BootstrapReport(Sources{Invalid: []string{"a: b"}})
	if len(report) != 1 {
		t.Errorf("report = %q, want the one note", report)
	}
}
