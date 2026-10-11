package commands

// Purpose: the partial-removal error shared by reset and uninstall (and clean):
// an infra-class error whose message names the failure and what was already
// removed, because an error envelope carries no data.
// Inputs:  what failed, the cause, and the paths removed so far.
// Outputs: an exit-2 (infra) error wrapping the cause.
// Constraints: split out of reset.go to keep it under 300 lines.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// volumesGone is the entry errPartialRemoval lists, ahead of the paths, once
// compose down --volumes has run: the database volumes are already deleted.
const volumesGone = "docker volumes (compose down --volumes)"

// errPartialRemoval is what JSON mode returns when a destructive step failed:
// an infra-class error (exit 2) whose message names the failure and everything
// already removed (paths, plus volumesGone when compose down --volumes ran),
// because an error envelope carries no data and the caller must not read the
// failure as either success or "nothing changed".
func errPartialRemoval(what string, cause error, removed []string) error {
	return partialRemovalError(what, cause, removed, "no files were removed")
}

// errRemovalFailed is errPartialRemoval for a failure of the deletion itself
// (D-0375): a failed RemoveAll may already have deleted part of a tree, so
// with nothing listed it says "some files may have been removed" rather than
// claiming nothing changed. Use errPartialRemoval for failures that happen
// before any deletion starts (compose down).
func errRemovalFailed(what string, cause error, removed []string) error {
	return partialRemovalError(what, cause, removed, "some files may have been removed")
}

// partialRemovalError formats the shared message; none is the wording used
// when removed is empty.
func partialRemovalError(what string, cause error, removed []string, none string) error {
	done := none
	if len(removed) > 0 {
		done = "already removed: " + strings.Join(jsonPaths(removed), ", ")
	}
	return errs.ExitWith(errs.ExitInfraError, fmt.Errorf("%s (%s): %w", what, done, cause))
}
