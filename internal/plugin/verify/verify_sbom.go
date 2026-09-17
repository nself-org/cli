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

// SBOMCheckOptions controls SBOM verification behavior.
type SBOMCheckOptions struct {
	SkipCheck bool   // --skip-sbom-check flag (air-gapped installs only)
	Version   string // plugin version to verify
}

// VerifySBOM downloads and validates the SBOM for a plugin release.
// Returns nil if SBOM is valid or check is skipped.
// Returns error if SBOM is missing (non-404), malformed, or fails schema validation.
func VerifySBOM(ctx context.Context, pluginName, version string, opts SBOMCheckOptions) error {
	if opts.SkipCheck {
		slog.Warn("SBOM check skipped (air-gapped installs only)", "plugin", pluginName, "version", version, "flag", "--skip-sbom-check")
		return nil
	}

	// Artifact name matches S2.T11 format: sbom-{version}.cdx.json
	artifactName := fmt.Sprintf("sbom-%s.cdx.json", version)

	// Download from GitHub Release assets.
	// URL: https://github.com/nself-org/bundles/releases/download/{version}/{artifactName}
	url := fmt.Sprintf("https://github.com/nself-org/bundles/releases/download/%s/%s", version, artifactName)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("sbom: create request: %w", err)
	}
	req.Header.Set("User-Agent", "nself-cli")

	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return fmt.Errorf("sbom: download from %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		// SBOM not present on release — warn but don't fail (older plugin releases pre-S2.T11).
		slog.Warn("no SBOM found (pre-SBOM release)", "plugin", pluginName, "version", version)
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sbom: download failed with HTTP %d", resp.StatusCode)
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
