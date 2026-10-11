package bundle

// installer_helpers_ledger.go — the bootstrap source of every ledger store this
// package writes through (newLedgerStore), D-0374.
//
// Purpose: bootstrap runs once, so its scan must be complete. Membership comes
// from the full loaded catalogue (Names), not the display order (All), so a
// catalogue bundle outside DisplayOrder is still recorded; and a plugin
// directory whose plugin.json exists but cannot be read is reported in
// Sources.Unreadable, which the store refuses to persist.
// Inputs: the plugin directory.
// Outputs: ledger.Sources, or an error when the catalogue or the directory
// cannot be read.
// Constraints: read only. A directory without plugin.json is not a plugin
// (.nself-state, stray folders) and is skipped, as plugin.LoadManifestsFromDir
// skips it.

import (
	"context"
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
		unreadable, err := unreadableManifests(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		plugins, err := installedPlugins(pluginDir)
		if err != nil {
			return ledger.Sources{}, err
		}
		src.Plugins, src.Unreadable = plugins, unreadable
		return src, nil
	}
}

// unreadableManifests lists, as "<dir>: <error>", every plugin directory whose
// plugin.json exists (or cannot be checked) but does not parse.
func unreadableManifests(pluginDir string) ([]string, error) {
	entries, err := os.ReadDir(pluginDir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}
	var bad []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(pluginDir, e.Name(), "plugin.json")
		if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if _, err := plugin.LoadManifest(path); err != nil {
			bad = append(bad, fmt.Sprintf("%s: %v", e.Name(), err))
		}
	}
	return bad, nil
}
