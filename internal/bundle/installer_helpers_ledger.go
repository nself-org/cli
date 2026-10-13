package bundle

// installer_helpers_ledger.go — the bootstrap source of every ledger store this
// package writes through (newLedgerStore), D-0374.
//
// Purpose: bootstrap runs once, so its scan must be complete. Membership comes
// from the full loaded catalogue (Names), not the display order (All), so a
// catalogue bundle outside DisplayOrder is still recorded; and a plugin
// directory whose plugin.json exists but cannot be read or parsed is reported
// in Sources.Unreadable, which the store refuses to persist. A plugin.json that
// parses but fails manifest validation (a missing description, say) is NOT
// unreadable: its plugin is recorded from the raw fields and the validation
// error goes to Sources.Invalid, a note that never blocks a persist (D-0475).
// Inputs: the plugin directory.
// Outputs: ledger.Sources, or an error when the catalogue or the directory
// cannot be read.
// Constraints: read only. A directory without plugin.json is not a plugin
// (.nself-state, stray folders) and is skipped, as plugin.LoadManifestsFromDir
// skips it. LedgerSources (ledger_sources.go) is this same source.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/bundle/ledger"
	"github.com/nself-org/cli/internal/plugin"
)

// bootstrapSources is the strict, full-catalogue bootstrap source of pluginDir.
func bootstrapSources(pluginDir string) func() (ledger.Sources, error) {
	return func() (ledger.Sources, error) {
		if err := EnsureLoaded(context.Background()); err != nil {
			return ledger.Sources{}, fmt.Errorf("loading the bundle catalog: %w", err)
		}
		var src ledger.Sources
		for _, slug := range Names() {
			if b, ok := Get(slug); ok {
				src.Bundles = append(src.Bundles, ledger.BundleDef{Slug: b.Slug, Installable: b.IsInstallable(), Plugins: b.Plugins})
			}
		}
		plugins, unreadable, invalid, err := scanPluginDir(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		src.Plugins, src.Unreadable, src.Invalid = plugins, unreadable, invalid
		return src, nil
	}
}

// rawManifest is the part of plugin.json the fallback reads when
// plugin.LoadManifest rejects the file for validation reasons.
type rawManifest struct {
	Name            string `json:"name"`
	Version         string `json:"version"`
	Tier            string `json:"tier"`
	RequiresLicense bool   `json:"requires_license"`
	LicenseType     string `json:"licenseType"`
	Checksum        string `json:"checksum"`
}

// scanPluginDir scans pluginDir once. For each subdirectory with a plugin.json:
// a manifest plugin.LoadManifest accepts is a fact; one it rejects is read raw.
// A file that cannot be read or parsed, or whose name is empty or not a valid
// plugin name, is unreadable ("<dir>: <error>"); any other is a fact built from
// the raw fields plus an invalid note ("<dir>: <error>; recorded by name").
func scanPluginDir(pluginDir string) (plugins []ledger.InstalledPlugin, unreadable, invalid []string, err error) {
	entries, err := os.ReadDir(pluginDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, fmt.Errorf("reading plugin directory: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(pluginDir, e.Name(), "plugin.json")
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		m, loadErr := plugin.LoadManifest(path)
		if loadErr == nil {
			plugins = append(plugins, ledgerFact(m))
			continue
		}
		raw, rawErr := readRawManifest(path)
		if rawErr != nil {
			unreadable = append(unreadable, fmt.Sprintf("%s: %v", e.Name(), rawErr))
			continue
		}
		plugins = append(plugins, ledgerFact(&plugin.PluginManifest{
			Name: raw.Name, Version: raw.Version, Tier: raw.Tier, Checksum: raw.Checksum,
			RequiresLicense: raw.RequiresLicense, LicenseType: raw.LicenseType,
		}))
		invalid = append(invalid, fmt.Sprintf("%s: %v; recorded by name", e.Name(), loadErr))
	}
	return plugins, unreadable, invalid, nil
}

// readRawManifest reads path as plain JSON; it errors unless the file parses
// and carries a valid plugin name.
func readRawManifest(path string) (rawManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return rawManifest{}, err
	}
	var raw rawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return rawManifest{}, err
	}
	if raw.Name == "" {
		return rawManifest{}, errors.New("plugin.json has no name")
	}
	if err := plugin.ValidateName(raw.Name); err != nil {
		return rawManifest{}, err
	}
	return raw, nil
}
