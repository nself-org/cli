package entitlement

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// IsLicensedPlugin classifies the native manifest's licence axis.
// Purpose: share build, start and doctor policy without importing L2 plugin code.
// Inputs: installed plugin directory. Outputs: licensed status or named E125.
// Constraints: missing manifests are free; malformed manifests fail closed.
// SPORT: ADR 0005; contract:plugin.entitlement-token.
func IsLicensedPlugin(dir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.json"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("E125", "cannot read plugin manifest in "+dir, err)
	}
	var m struct {
		Version         json.RawMessage `json:"manifest_version"`
		License         string          `json:"license"`
		Tier            string          `json:"tier"`
		RequiresLicense bool            `json:"requires_license"`
		IsCommercial    bool            `json:"isCommercial"`
		Category        json.RawMessage `json:"category"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return false, errs.Wrap("E125", "invalid plugin manifest in "+dir, err)
	}
	licensed := m.License == "licensed"
	if isV1ManifestVersion(m.Version) {
		licensed = licensed || m.RequiresLicense || m.IsCommercial || (m.Tier != "" && m.Tier != "free")
	} else if strings.TrimSpace(string(m.Version)) != "2" {
		return false, errs.New("E125", "unsupported plugin manifest version in "+dir)
	}
	var category string
	_ = json.Unmarshal(m.Category, &category)
	if licensed && category == "security" {
		return false, errs.New("E125", "security plugin in "+dir+" cannot require a licence (ADR 0005)")
	}
	return licensed, nil
}

// isV1ManifestVersion mirrors manifestv2.IsV1Version without an L1 -> L2 import.
// Purpose: preserve the real loader's v1 version spellings in licence policy.
// Inputs: raw version. Outputs: v1 status. Constraints: quoted numbers other than "1" are not v1.
// SPORT: contract:plugin.entitlement-token.
func isV1ManifestVersion(v json.RawMessage) bool {
	t := strings.TrimSpace(string(v))
	switch t {
	case "", "null", `"1"`:
		return true
	}
	if strings.HasPrefix(t, `"`) {
		return false
	}
	f, err := strconv.ParseFloat(t, 64)
	return err == nil && (f == 0 || f == 1)
}

// HasLicensedInstalled reports whether an enabled installed plugin is licensed.
// Purpose: share the installation gate for refresh and diagnostics.
// Inputs: NSELF_PLUGIN_DIR or user's default installation. Outputs: status/error.
// Constraints: disabled plugins are ignored; malformed manifests fail closed.
// SPORT: contract:plugin.entitlement-token.
func HasLicensedInstalled() (bool, error) {
	dir := strings.TrimSpace(os.Getenv("NSELF_PLUGIN_DIR"))
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, errs.Wrap("E125", "cannot locate installed plugins", err)
		}
		dir = filepath.Join(home, ".nself", "plugins")
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, errs.Wrap("E125", "cannot read installed plugins", err)
	}
	found := false
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginDir := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(pluginDir, ".disabled")); err == nil {
			continue
		}
		licensed, err := IsLicensedPlugin(pluginDir)
		if err != nil {
			return false, err
		}
		found = found || licensed
	}
	return found, nil
}
