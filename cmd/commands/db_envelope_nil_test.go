package commands

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestJSONEnvelopeOnNilCommand: runBundleList accepts a nil command, so the
// envelope check must answer "off" for one instead of dereferencing it in v1.5
// (D-0275: the bundle list tests panicked under NSELF_V15=1).
func TestJSONEnvelopeOnNilCommand(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		if jsonEnvelopeOn(nil) {
			t.Error("jsonEnvelopeOn(nil) = true, want false")
		}
	})
}
