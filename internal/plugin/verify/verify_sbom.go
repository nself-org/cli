package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/nself-org/cli/internal/httptimeout"
)

// sbomProbeTimeout bounds the best-effort GitHub Release lookup below.
//
// A real 404 from github.com/nself-org/bundles answers in well under a
// second (measured directly, both for an existing and a nonexistent tag).
// 30s only mattered for the case where the request never gets a response at
// all; that case is exactly what we want to fail open on quickly instead of
// sitting on, since this check is advisory (see the fail-open comment below).
const sbomProbeTimeout = 5 * time.Second

// sbomBaseURL is the GitHub Release base VerifySBOM probes. A package-level
// var (not a const) purely so tests can point it at an httptest server
// instead of the real github.com — production code never changes it.
var sbomBaseURL = "https://github.com/nself-org/bundles"

// SBOMCheckOptions controls SBOM verification behavior.
type SBOMCheckOptions struct {
	SkipCheck bool   // true when this plugin's SBOM cannot live at the lookup source
	Version   string // plugin version to verify

	// SkipReason overrides the logged reason for SkipCheck. Defaults to the
	// air-gapped-install message when empty, so existing callers (only the
	// --skip-sbom-check flag) are unaffected.
	SkipReason string
}

// VerifySBOM downloads and validates the SBOM for a plugin release.
// Returns nil if SBOM is valid, absent, or check is skipped/unreachable.
// Returns error only when a response WAS obtained and it fails schema
// validation — i.e. something claiming to be an SBOM is actually malformed,
// which is the one case that indicates real tampering or corruption rather
// than mere absence.
func VerifySBOM(ctx context.Context, pluginName, version string, opts SBOMCheckOptions) error {
	if opts.SkipCheck {
		reason := opts.SkipReason
		if reason == "" {
			reason = "--skip-sbom-check"
		}
		slog.Warn("SBOM check skipped", "plugin", pluginName, "version", version, "reason", reason)
		return nil
	}

	// Artifact name matches S2.T11 format: sbom-{version}.cdx.json
	artifactName := fmt.Sprintf("sbom-%s.cdx.json", version)

	// Download from GitHub Release assets.
	// URL: https://github.com/nself-org/bundles/releases/download/{version}/{artifactName}
	url := fmt.Sprintf("%s/releases/download/%s/%s", sbomBaseURL, version, artifactName)

	ctx, cancel := context.WithTimeout(ctx, sbomProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("sbom: create request: %w", err)
	}
	req.Header.Set("User-Agent", "nself-cli")

	client := httptimeout.WithTimeout(sbomProbeTimeout)
	resp, err := client.Do(req)
	if err != nil {
		// Transport-level failure (timeout, DNS, connection refused, ...).
		// This probe is advisory — checksum (Step 5) and signature (Step 5b)
		// already gated integrity before this ever runs — so a network that
		// cannot answer is treated the same as a definitive 404 below rather
		// than blocking or failing the install.
		slog.Warn("SBOM check unreachable, proceeding without it", "plugin", pluginName, "version", version, "error", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// 404 (no SBOM published, e.g. older or non-GitHub-hosted releases)
		// and any other non-200 (403/5xx/etc.) are equally "could not obtain
		// a valid SBOM from this source" — advisory, not a hard failure.
		slog.Warn("no SBOM found (pre-SBOM release)", "plugin", pluginName, "version", version, "status", resp.StatusCode)
		return nil
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("sbom: read response: %w", err)
	}

	// Validate CycloneDX JSON schema (minimal check).
	if err := validateCycloneDX(data); err != nil {
		return fmt.Errorf("sbom: invalid CycloneDX JSON for %s@%s: %w", pluginName, version, err)
	}

	slog.Info("sbom verified", "plugin", pluginName, "version", version)
	return nil
}

// validateCycloneDX checks the minimal required CycloneDX fields.
func validateCycloneDX(data []byte) error {
	var doc struct {
		BOMFormat   string `json:"bomFormat"`
		SpecVersion string `json:"specVersion"`
		Version     int    `json:"version"`
		Components  []any  `json:"components"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("not valid JSON: %w", err)
	}
	if doc.BOMFormat != "CycloneDX" {
		return fmt.Errorf("bomFormat must be CycloneDX, got %q", doc.BOMFormat)
	}
	if doc.SpecVersion == "" {
		return fmt.Errorf("specVersion is required")
	}
	return nil
}
