package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DeepChecks runs all 12 subsystem categories for --deep mode.
func DeepChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	var results []CheckResult

	results = append(results, HostChecks(ctx, verbose)...)
	results = append(results, DockerDeepChecks(ctx, verbose)...)
	results = append(results, PostgresChecks(ctx, projectDir, verbose)...)
	results = append(results, HasuraChecks(ctx, verbose)...)
	results = append(results, NginxChecks(ctx, projectDir, verbose)...)
	results = append(results, SSLChecks(ctx, verbose)...)
	results = append(results, PingChecks(ctx, verbose)...)
	results = append(results, PluginHealthChecks(ctx, projectDir, verbose)...)

	// P7-PLUG-59: boot-migration readiness (E118/E119) and boot apply (E120).
	if home, herr := os.UserHomeDir(); herr == nil {
		results = append(results, PluginMigrationChecks(ctx, filepath.Join(home, ".nself", "plugins"))...)
	}

	// P2-E7-W2-S6-T21: PayPal multi-account CSV parity validation.
	results = append(results, CheckPayPalCSVParity(ctx))

	results = append(results, LicenseChecks(ctx, projectDir, verbose)...)
	results = append(results, MonitoringChecks(ctx)...)
	results = append(results, BackupChecks(ctx, projectDir)...)
	results = append(results, SecurityChecks(ctx, projectDir)...)

	// S69-T05: ai+moderation wiring gap check.
	results = append(results, CheckModerationWired(ctx))

	// S70-T06: Hasura introspection disabled in production.
	results = append(results, CheckHasuraIntrospection(ctx))

	// S77-T08: orphaned Hasura remote schemas after plugin uninstall.
	results = append(results, CheckOrphanRemoteSchemas(ctx))

	// S12.T01: ɳSentry Prometheus scrape config (OBS-SCRAPE-01).
	// pluginDir defaults to ~/.nself/plugins; projectDir is the caller's --project-dir.
	if home, herr := os.UserHomeDir(); herr == nil {
		pluginDir := filepath.Join(home, ".nself", "plugins")
		results = append(results, CheckOBSScrape(ctx, projectDir, pluginDir))
	}

	// S12.T09: telemetry redaction coverage audit (OBS-REDACT-01).
	results = append(results, CheckRedactionCoverage(ctx))

	// S74-T02 + S74-T-PERM-01: RLS enforcement for np_* tables (PERM-RLS-01).
	results = append(results, CheckRLSEnforcement(ctx, false)...)

	// ɳSentry-specific RLS checks (NSENTRY-RLS-01..07).
	// Runs only when one or more of the 7 ɳSentry baseline plugins is installed.
	// Severity is CRITICAL (fail) — ɳSentry tables hold cross-tenant observability data.
	if home, herr := os.UserHomeDir(); herr == nil {
		nsentryPluginDir := filepath.Join(home, ".nself", "plugins")
		results = append(results, CheckNSentryRLS(ctx, nsentryPluginDir)...)
	}

	// S1.T10: Hasura metadata YAML row-filter check for np_* tables (PERM-HASURA-01).
	results = append(results, CheckHasuraMetadataYAML(ctx, projectDir, false)...)

	// S98-02-T11: JWT key rotation check (JWT-ROT-01).
	results = append(results, CheckJWTRotation(projectDir))

	// S98-02-T12: SSRF guard verification (SSRF-01).
	results = append(results, CheckSSRF(projectDir))

	// CI token rotation check (CI-TOKEN-01).
	results = append(results, CheckCIToken(projectDir))

	// CI vault sync check (CI-VAULT-SYNC-01).
	results = append(results, CheckCIVaultSync(projectDir))

	// S03-T06: SDK version coherence check (SDK-VERSION-01).
	results = append(results, CheckSDKVersions(ctx)...)

	// S10.T06: SEC-HARDENING-01..08 — Security-Always-Free hardening checks.
	results = append(results, HardeningChecks(ctx, projectDir)...)

	// NSCAN-001..010 baseline scan via the nself-audit plugin's POST /scan/run
	// endpoint. Skipped gracefully if the plugin is not installed/running.
	// Security-Always-Free: no license required.
	results = append(results, CheckNSelfAuditScan(ctx, projectDir)...)

	// S9.T03 + S9.T15: OPS-DRILL-01 — verify a successful `nself backup drill`
	// ran within the last 7 days. Read-only check against .nself/drill-log.json.
	results = append(results, CheckOPSDrill(ctx, projectDir))

	// S12.T06: LEGAL-COPPA-01 — verify COPPA parental-consent flow wiring
	// (migration on disk, FAMILY_CONSENT_HMAC_SECRET set, TTL within policy).
	results = append(results, CheckLegalCOPPA(projectDir))

	// S12.T07: LEGAL-GDPR-A9-01 — verify GDPR Article 9 special-category
	// consent flow (migration on disk, privacy disclosure section, DPO contact).
	results = append(results, CheckLegalGDPRA9(projectDir))

	// nself.org-specific checks. Gated on NSELF_DOGFOOD=1 so end-user
	// `nself doctor --deep` runs are not slowed by HTTP probes against nself.org
	// subdomains. CI workflow at web/.github/workflows/dogfood-check.yml exports
	// NSELF_DOGFOOD=1.
	if os.Getenv("NSELF_DOGFOOD") == "1" {
		results = append(results, DogfoodChecks(ctx, projectDir, verbose)...)
	}

	// MINIO-CRED-01: flag minioadmin default credentials in prod/staging.
	results = append(results, CheckMinioCredentials())

	return results
}

// CheckMinioCredentials reads NSELF_ENV, MINIO_ROOT_USER, and MINIO_ROOT_PASSWORD
// from the environment and returns a CRITICAL finding when either credential
// uses the insecure "minioadmin" default in a staging or production deployment.
// Dev environments are always unblocked.
//
// Check ID: MINIO-CRED-01
func CheckMinioCredentials() CheckResult {
	env := os.Getenv("NSELF_ENV")
	if env != "prod" && env != "staging" {
		return CheckResult{
			Section: "security",
			Name:    "MinIO credentials (MINIO-CRED-01)",
			Status:  "pass",
			Message: fmt.Sprintf("dev environment (%q) — minioadmin defaults accepted", env),
		}
	}

	rootUser := os.Getenv("MINIO_ROOT_USER")
	rootPassword := os.Getenv("MINIO_ROOT_PASSWORD")

	if rootUser == "minioadmin" || rootUser == "" {
		return CheckResult{
			Section: "security",
			Name:    "MinIO credentials (MINIO-CRED-01)",
			Status:  "fail",
			Message: "CRITICAL: MINIO_ROOT_USER is 'minioadmin' or unset — set a strong unique value in .env",
			FixCmd:  "Set MINIO_ROOT_USER=<strong-value> in .env and restart with: nself start",
		}
	}
	if rootPassword == "minioadmin" || len(rootPassword) < 16 {
		return CheckResult{
			Section: "security",
			Name:    "MinIO credentials (MINIO-CRED-01)",
			Status:  "fail",
			Message: "CRITICAL: MINIO_ROOT_PASSWORD is 'minioadmin' or too short (<16 chars) — set a strong value in .env",
			FixCmd:  "Set MINIO_ROOT_PASSWORD=<strong-value-16+chars> in .env and restart with: nself start",
		}
	}

	return CheckResult{
		Section: "security",
		Name:    "MinIO credentials (MINIO-CRED-01)",
		Status:  "pass",
		Message: "MINIO_ROOT_USER and MINIO_ROOT_PASSWORD are set to non-default values",
	}
}

// LicenseChecks verifies license is current and tier matches expected.
func LicenseChecks(ctx context.Context, projectDir string, verbose bool) []CheckResult {
	var results []CheckResult

	cachePath := filepath.Join(projectDir, ".nself", "cache", "entitlements.json")
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		results = append(results, CheckResult{Section: "license", Name: "License cache", Status: "pass",
			Message: "no cache (run nself license validate)"})
		return results
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		results = append(results, CheckResult{Section: "license", Name: "License cache", Status: "warn",
			Message: fmt.Sprintf("cannot read: %v", err)})
		return results
	}

	content := string(data)
	if strings.Contains(content, `"grace"`) {
		results = append(results, CheckResult{Section: "license", Name: "License grace", Status: "warn",
			Message: "license in grace period"})
	} else {
		results = append(results, CheckResult{Section: "license", Name: "License status", Status: "pass",
			Message: "active"})
	}

	return results
}

// FilterBySection returns only checks matching the given section name.
func FilterBySection(results []CheckResult, section string) []CheckResult {
	var filtered []CheckResult
	for _, r := range results {
		if strings.EqualFold(r.Section, section) {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
