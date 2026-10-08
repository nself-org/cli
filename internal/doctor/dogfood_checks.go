package doctor

// dogfood_checks.go — dogfood-only checks for nSelf's own backend (web/backend).
//
// Extra deep checks that run against the nself.org production stack to catch
// drift between what we ship and what users get. Invoked by `nself doctor --deep`
// when run inside a repo that declares itself a dogfood target via NSELF_DOGFOOD=1
// OR by the dogfood-check CI workflow at web/.github/workflows/dogfood-check.yml.
//
// All checks here are read-only and never modify state. Per the
// Security-Always-Free Doctrine, these checks run without a license.
//
// This file holds the entry point, the shared subapp lookup tables, and the
// license-vault check. The security header probes live in dogfood_headers.go,
// the dependency-license and sport-sync checks in dogfood_deps_sport.go, and
// the hex-color scan in dogfood_hexcolors.go — split out (CLI-R12) as a pure
// move from this file.

import (
	"context"
	"net/http"
	"os"
	"time"
)

// DogfoodChecks runs the full dogfood-only check set. It is invoked from
// DeepChecks when NSELF_DOGFOOD=1 is set, AND can be invoked directly from
// CI to gate nself.org's own deploys.
//
// Check IDs (cross-referenced in operations/dogfood-checks.md):
//
//	SEC-CSP-01    — CSP headers present on all subapps
//	SEC-HSTS-01   — HSTS preload value
//	SEC-AUTH-01   — HttpOnly cookie convention enforced
//	VENDOR-DEP-01 — no AGPL/SSPL deps in production source
//	DOGFOOD-SUBAPPS-01 — every shipped subapp has synced sport.json
//	DOGFOOD-HEX-01     — no hex colors in subapp src/ trees
//	DOGFOOD-LICENSE-01 — owner license vault path readable
//
// PERM-RLS-01 is intentionally NOT duplicated here; rls_check.go owns it.
func DogfoodChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	var results []CheckResult

	// Security headers — only run when subapp URLs are reachable. The CI job
	// uses a localhost reverse proxy; production hits the live subdomains.
	results = append(results, checkCSPHeaders(ctx)...)
	results = append(results, checkHSTSHeader(ctx))
	results = append(results, checkHttpOnlyCookies(ctx))

	// Vendor dependency license scan — production source only.
	results = append(results, checkAGPLDeps(ctx, projectDir))

	// Dogfood-only structural checks.
	results = append(results, checkSubappSportSync(projectDir)...)
	results = append(results, checkNoHexColorsInSrc(projectDir)...)
	results = append(results, checkLicenseVaultPath())

	return results
}

// dogfoodSubapps lists every subapp that must be present + sport-synced for
// nself.org's web monorepo. Mirrors the F11-SUBDOMAIN-MAP table.
var dogfoodSubapps = []string{
	"org", "docs", "nchat", "nclaw", "ntask", "ntv", "nfamily",
	"clawde", "cloud", "install", "base", "backend",
	// 12 + base = 13 per PPI Three-Surface Model.
}

// dogfoodSubappURLs maps subapps to their production URLs for header probing.
var dogfoodSubappURLs = map[string]string{
	"org":     "https://nself.org",
	"docs":    "https://nself.org/docs",
	"nchat":   "https://chat.nself.org",
	"nclaw":   "https://claw.nself.org",
	"ntask":   "https://task.nself.org",
	"ntv":     "https://ntv.nself.org",
	"cloud":   "https://cloud.nself.org",
	"install": "https://install.nself.org",
}

// httpHeaderClient is shared across header checks with a tight timeout so a
// single slow subapp can't hang the whole doctor run.
var httpHeaderClient = &http.Client{
	Timeout: 8 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		// Don't follow redirects — we want the headers from the canonical URL.
		return http.ErrUseLastResponse
	},
}

// checkLicenseVaultPath verifies the owner license env var path is readable
// when NSELF_PLUGIN_LICENSE_KEY_OWNER is referenced. This catches missing
// vault sourcing in the CI environment.
func checkLicenseVaultPath() CheckResult {
	const checkName = "DOGFOOD-LICENSE-01: license vault path"
	if os.Getenv("NSELF_PLUGIN_LICENSE_KEY") != "" {
		return CheckResult{Section: "dogfood", Name: checkName, Status: "pass",
			Message: "NSELF_PLUGIN_LICENSE_KEY set"}
	}
	if os.Getenv("NSELF_PLUGIN_LICENSE_KEY_OWNER") != "" {
		return CheckResult{Section: "dogfood", Name: checkName, Status: "pass",
			Message: "NSELF_PLUGIN_LICENSE_KEY_OWNER set"}
	}
	return CheckResult{Section: "dogfood", Name: checkName, Status: "warn",
		Message: "no license env var set; some Licensed plugin checks may skip",
		FixCmd:  "source ~/.claude/vault.env  # or set NSELF_PLUGIN_LICENSE_KEY"}
}
