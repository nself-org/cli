package health

// Purpose: the pass/fail verdict of a start's health step (p7-wire-20): a
// report below the required percentage, or with a core service down, is not a
// running stack.
// Inputs: a *HealthReport and the required percentage.
// Outputs: nil, or an error wrapping errs.ErrHealthTimeout / ErrServiceUnhealthy.
// Constraints: pure; no I/O.

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// CoreServices must be healthy for a start to count as up, whatever the
// percentage says.
var CoreServices = map[string]bool{"postgres": true, "hasura": true, "auth": true, "nginx": true}

// Verdict returns why report is not a running stack, or nil.
func Verdict(report *HealthReport, requiredPct int) error {
	if report == nil {
		return fmt.Errorf("no health report before the timeout: %w", errs.ErrHealthTimeout)
	}
	var down []string
	for _, r := range report.Results {
		if CoreServices[r.Service] && !r.OK() {
			down = append(down, r.Service+" ("+r.Status+")")
		}
	}
	if len(down) > 0 {
		return fmt.Errorf("core service not healthy: %s: %w", strings.Join(down, ", "), errs.ErrServiceUnhealthy)
	}
	pct := 0
	if report.Total > 0 {
		pct = report.Healthy * 100 / report.Total
	}
	if pct < requiredPct {
		return fmt.Errorf("%d/%d services healthy (%d%%, required %d%%): %w", report.Healthy, report.Total, pct, requiredPct, errs.ErrHealthTimeout)
	}
	return nil
}

// RequiredPct is the healthy percentage a start needs: 80, 60 with --quick, or
// the project's HEALTH_CHECK_REQUIRED (cfgPct, when set and not --quick).
func RequiredPct(quick bool, cfgPct int) int {
	switch {
	case quick:
		return 60
	case cfgPct > 0:
		return cfgPct
	}
	return 80
}
