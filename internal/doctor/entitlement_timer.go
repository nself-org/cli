package doctor

import (
	"context"
	"runtime"

	"github.com/nself-org/cli/internal/license/entitlement"
	"github.com/nself-org/cli/internal/maintenance"
)

// Seams for tests.
var (
	timerWanted = entitlement.TimerWanted
	timerStatus = maintenance.Status
)

// Purpose: surface E132 when a licensed v1.5 stack has no unattended entitlement refresh timer.
// Inputs: project directory. Outputs: no finding when the timer is active, not wanted (v1.4,
// free stack, autorefresh disabled, custom plugin directory) or Windows; otherwise one warning.
// Constraints: read-only, never installs the timer, never prints a path, key or token.
// SPORT: cap:cli.entitlement-autorefresh (P7-PLUG-64).
func EntitlementTimerChecks(_ context.Context, projectDir string) []CheckResult {
	if runtime.GOOS == "windows" || !timerWanted() {
		return nil
	}
	st := timerStatus(projectDir)
	if st.Active {
		return nil
	}
	return []CheckResult{{
		Section: "plugins", Name: "ENTITLEMENT-REFRESH-TIMER", Status: "warn",
		Message: "E132: no active entitlement refresh timer (reason: " + st.Reason + "); licensed plugins stop serving 30 days after the last refresh",
		FixCmd:  "nself build",
	}}
}
