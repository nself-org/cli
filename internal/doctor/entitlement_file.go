package doctor

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
	"github.com/nself-org/cli/internal/license/entitlement"
)

// Purpose: surface E126 before the server-signed entitlement grace runs out.
// Inputs: project directory and the current time, with a bounded JWT read.
// Outputs: no finding for free stacks; warning for missing, invalid or stale files.
// Constraints: read-only, v1.5 only, never trust an unsigned exp or local mtime.
// SPORT: contract:plugin.entitlement-token v1 (P7-PLUG-24).
func EntitlementFileChecks(_ context.Context, projectDir string) []CheckResult {
	return entitlementFileChecks(projectDir, time.Now())
}

// Purpose: check the entitlement against an injectable clock for boundary tests.
// Inputs: project directory and now. Outputs: zero or one doctor findings.
// Constraints: token bytes and claims are never printed; no network access.
// SPORT: contract:plugin.entitlement-token v1.
func entitlementFileChecks(projectDir string, now time.Time) []CheckResult {
	// compat.V15(P7-PLUG-24): no runtime entitlement diagnostic -> warn on stale signed entitlement
	if !compat.V15() {
		return nil
	}
	installed, err := entitlement.HasLicensedInstalled()
	if err != nil {
		return entitlementWarning("Cannot classify installed plugin manifests; run nself build to check them.")
	}
	if !installed {
		return nil
	}
	f, err := os.Open(filepath.Join(projectDir, ".nself", "state", "entitlement.jwt"))
	if err != nil {
		return entitlementWarning("Cannot read the entitlement file; run nself build to restore it.")
	}
	defer func() { _ = f.Close() }()
	const readCap = 64 << 10
	data, err := io.ReadAll(io.LimitReader(f, readCap+1))
	if err != nil || len(data) > readCap {
		return entitlementWarning("Cannot read the entitlement file; run nself build to restore it.")
	}
	claims, err := license.ParseLicenseJWT(string(data))
	if err != nil || claims.Exp <= 0 {
		return entitlementWarning("The entitlement file is invalid; run nself build to refresh it.")
	}
	if now.Sub(time.Unix(claims.Exp, 0)) <= 7*24*time.Hour {
		return nil
	}
	e := errs.New("E126", "the entitlement JWT expired more than seven days ago")
	return entitlementWarning("E126: " + e.What + "; refresh with nself license refresh followed by nself build before the thirty-day grace expires.")
}

// Purpose: format a safe entitlement diagnostic without exposing the token.
// Inputs: static diagnostic message. Outputs: one warning.
// Constraints: no automatic mutations. SPORT: P7-PLUG-24.
func entitlementWarning(message string) []CheckResult {
	return []CheckResult{{Section: "plugins", Name: "PLUGIN-ENTITLEMENT", Status: "warn", Message: message}}
}
