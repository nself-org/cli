package build

import (
	"errors"
	"fmt"
	"io"

	"github.com/nself-org/cli/internal/errs"
)

// EntitlementRefreshError identifies the completed entitlement publication step.
// Purpose: isolate runtime licence failures from compose validation failures.
// Inputs: refresh error. Outputs: wrapped error. Constraints: only E125 is soft
// during start; build completes all later steps first. SPORT: plugin isolation.
type EntitlementRefreshError struct{ Err error }

func (e *EntitlementRefreshError) Error() string { return e.Err.Error() }
func (e *EntitlementRefreshError) Unwrap() error { return e.Err }

func wrapEntitlementRefresh(err error) error {
	if err == nil {
		return nil
	}
	return &EntitlementRefreshError{Err: err}
}

// StartMayContinue permits only an E125 from the completed entitlement step.
// Inputs: build error/warnings. Outputs: continuation decision.
// Constraints: fragment and later build errors keep their original exit codes.
// SPORT: free-service startup after entitlement failure.
func StartMayContinue(err error, warnings io.Writer) bool {
	if err == nil {
		return true
	}
	var refresh *EntitlementRefreshError
	if errors.As(err, &refresh) && errs.Describe(refresh.Err) != nil && errs.Describe(refresh.Err).Code == "E125" {
		_, _ = fmt.Fprintln(warnings, err)
		return true
	}
	return false
}
