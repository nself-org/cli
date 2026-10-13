package trust

import (
	"errors"

	"github.com/nself-org/cli/internal/trust/sysexec"
)

// ErrSkippedHeadless is returned by a setup step that found its state missing
// but made no system change because the process is headless. Setup converts
// it into a TrustResult.Skipped entry; it is a clear "skipped (headless)"
// result, not a failure.
var ErrSkippedHeadless = sysexec.ErrSkippedHeadless

// isHeadless reports whether the CLI is running in a headless context where
// macOS admin prompts (osascript ... with administrator privileges) and every
// other system change must be suppressed. It is true when NSELF_HEADLESS is
// truthy ("1", "true", "yes", "on", case-insensitive), or when CI is truthy
// and NSELF_HEADLESS does not say otherwise. NSELF_HEADLESS=0 forces
// interactive behaviour even inside CI.
//
// When true:
//   - Every setup step checks its state first (read-only). A step whose state
//     is already configured reports "already done" as before.
//   - A step whose state is missing makes no system change (no osascript, no
//     sudo, no brew, no pfctl, no keychain or file writes outside the project)
//     and returns ErrSkippedHeadless.
//   - Setup records the step in TrustResult.Skipped so the operator is told
//     which admin-only steps to run by hand.
func isHeadless() bool { return sysexec.Headless() }

// isSkipped reports whether err carries ErrSkippedHeadless.
func isSkipped(err error) bool { return errors.Is(err, ErrSkippedHeadless) }
