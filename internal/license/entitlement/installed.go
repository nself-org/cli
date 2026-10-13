package entitlement

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/license"
)

// RefreshInstalled refreshes a project only when a licensed plugin is installed.
// Purpose: keep free-only stacks independent of licences. Inputs: context/project.
// Outputs: the refresh result. Constraints: native manifests, no runtime Docker calls.
// SPORT: start/license-set entitlement publication.
func RefreshInstalled(ctx context.Context, projectDir string) error {
	return RefreshInstalledForKey(ctx, projectDir, "")
}

// RefreshInstalledForKey validates a newly set key before publishing its token.
// Purpose: prevent old-key reuse after license set. Inputs: context/project/new key.
// Outputs: private runtime file or E125. Constraints: only licensed installations
// need online validation; a rejected key never retains the old token.
// SPORT: license-set token capture.
func RefreshInstalledForKey(ctx context.Context, projectDir, key string) error {
	// compat.V15(P7-PLUG-24): start and license set ignore entitlement -> refresh licensed installed plugins
	if !compat.V15() || DevKeyMode() {
		return nil
	}
	dir := strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = filepath.Join(home, ".nself", "plugins")
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), ".disabled")); err == nil {
			continue
		}
		if installedLicensed(filepath.Join(dir, entry.Name(), "plugin.json")) {
			if key != "" {
				result, err := license.Validate(ctx, key, nil)
				if err != nil || !result.CanProceed {
					if writeErr := writeAtomic(Path(projectDir), nil); writeErr != nil {
						return errs.Wrap("E125", "could not clear the plugin entitlement file", writeErr)
					}
					return errs.New("E125", "the new licence could not obtain a verified entitlement")
				}
			}
			return Refresh(ctx, projectDir)
		}
	}
	return nil
}

// installedLicensed reads the native manifest licence axis without runtime imports.
// Purpose: keep L1 publication independent of L2 plugin orchestration.
// Inputs: manifest path. Outputs: licensed status. Constraints: explicit free wins;
// legacy tier fields are consulted only when free is not explicit.
// SPORT: installed licence discovery.
func installedLicensed(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var m struct {
		License         string `json:"license"`
		Tier            string `json:"tier"`
		RequiresLicense bool   `json:"requiresLicense"`
	}
	if json.Unmarshal(data, &m) != nil {
		return false
	}
	if m.License == "free" {
		return false
	}
	return m.License == "licensed" || m.Tier == "pro" || m.Tier == "paid" || m.Tier == "licensed" || m.RequiresLicense
}
