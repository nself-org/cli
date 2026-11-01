package progress

// Purpose: redact secrets from an event detail before it is written.
// Inputs: free text (an error message or a reason).
// Outputs: the text with URL credentials and secret assignments masked, then
// the shared telemetry redactor applied.
// Constraints: delegates entirely to output.RedactText so there is exactly one
// copy of the credential and key-value rules (D-0742). Do not reintroduce a
// local regexp or private rule here.

import (
	"github.com/nself-org/cli/internal/output"
)

// redactText masks URL credentials, then secret assignments, then runs the
// shared telemetry redactor via output.RedactText (D-0742).
func redactText(s string) string {
	return output.RedactText(s)
}
