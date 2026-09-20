package plugin

// Purpose: Plugin discovery and listing — scans pluginDir for installed plugins
//          and merges with registry data to produce PluginInfo/InstalledPluginInfo lists.
// Inputs:  pluginDir string (absolute path to plugin installation directory).
// Outputs: []PluginInfo or []InstalledPluginInfo; error on directory read failure.
// Constraints: Registry fetch uses a 30s timeout; pluginDir non-existence returns nil, nil.
//              Directories without a valid plugin.json are skipped, with a warning on
//              stderr naming the directory and the parse error — never silently.
// SPORT: list/inventory operations; callers: cmd/plugin/list.go, cmd/plugin/inventory.go

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/nginx"
)

// ListInstalled scans pluginDir and returns detailed information for every
// installed plugin. Each entry's Status is "running", "installed", or
// "unknown" depending on the plugin's current runtime state.
func ListInstalled(pluginDir string) ([]InstalledPluginInfo, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP. No plugin directory means no plugins installed — the
			// normal state of a fresh install, and identical to an empty
			// directory from every caller's point of view. Only ENOENT; an
			// unreadable plugin dir still errors, so "I could not list your
			// plugins" is never rendered as "you have none".
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	var plugins []InstalledPluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginDir, entry.Name(), "plugin.json")
		m, err := parseManifest(manifestPath)
		if err != nil {
			continue // skip directories without valid manifests
		}

		status := "installed"
		if st, stErr := Status(m.Name); stErr == nil {
			status = st.State
		}
		// Overlay lifecycle state: dormant/expired beats runtime state.
		if store, lcErr := LoadLifecycleStore(); lcErr == nil {
			if rec, ok := store.Records[m.Name]; ok {
				switch rec.State {
				case StateDormant:
					status = "dormant"
				case StateExpired:
					status = "expired"
				}
			}
		}

		tier := m.Tier
		if tier == "" {
			if m.RequiresLicense || m.LicenseType == "pro" {
				tier = "pro"
			} else {
				tier = "free"
			}
		}

		plugins = append(plugins, InstalledPluginInfo{
			Name:        m.Name,
			Version:     m.Version,
			Tier:        tier,
			Status:      status,
			Description: m.Description,
		})
	}

	return plugins, nil
}

// LoadManifestsFromDir scans pluginDir and returns the full PluginManifest for
// every installed plugin. It is the full-manifest counterpart to ListInstalled,
// used by features that need manifest fields beyond name/version/tier/status
// (e.g., federation's GraphQL block). Directories without a valid plugin.json
// are silently skipped. If pluginDir does not exist, nil is returned.
func LoadManifestsFromDir(pluginDir string) ([]*PluginManifest, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP — same rationale as ListInstalled above: absent dir ==
			// nothing installed, and only ENOENT takes this branch.
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}
	var manifests []*PluginManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginDir, entry.Name(), "plugin.json")
		m, err := parseManifest(manifestPath)
		if err != nil {
			continue // skip directories without a valid manifest
		}
		manifests = append(manifests, m)
	}
	return manifests, nil
}

// List returns plugin information. When installed is true it scans pluginDir
// for locally installed plugins. When false it returns all plugins known to
// the registry.
func List(pluginDir string, installed bool) ([]PluginInfo, error) {
	if installed {
		return listInstalled(pluginDir)
	}
	return listFromRegistry(pluginDir)
}

// listInstalled scans pluginDir for subdirectories that contain a valid
// plugin.json manifest and returns PluginInfo for each.
func listInstalled(pluginDir string) ([]PluginInfo, error) {
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KEEP — same rationale as ListInstalled above: absent dir ==
			// nothing installed, and only ENOENT takes this branch.
			return nil, nil
		}
		return nil, fmt.Errorf("reading plugin directory: %w", err)
	}

	var plugins []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginDir, entry.Name(), "plugin.json")
		m, err := parseManifest(manifestPath)
		if err != nil {
			// A directory here is an INSTALLED plugin, so an unreadable
			// manifest is not the same as "not a plugin" — the plugin is on
			// disk and the user was told it installed. Swallowing the error
			// made it vanish from `nself plugin list --installed` with no
			// output at all, which is indistinguishable from never having
			// installed it.
			//
			// Found 2026-09-13: the free Task Bundle's `notifications` plugin
			// declares status "deprecated" but carries the flat
			// deprecated/deprecatedSince/replacedBy fields instead of the
			// `deprecation` block validateManifest requires, so its manifest
			// is invalid. `nself plugin install notifications` printed
			// "installed successfully", the directory and manifest were
			// written, and the plugin was then invisible to every listing.
			//
			// Still a `continue`, deliberately: one bad manifest must not hide
			// the other seven. But it is now a visible diagnostic.
			fmt.Fprintf(os.Stderr,
				"warning: installed plugin %q has an unreadable manifest and is being skipped: %v\n",
				entry.Name(), err)
			continue
		}

		running := false
		if st, err := Status(m.Name); err == nil && st.State == "running" {
			running = true
		}

		plugins = append(plugins, PluginInfo{
			Name:      m.Name,
			Version:   m.Version,
			Category:  m.Category,
			Installed: true,
			Running:   running,
			UpdatedAt: m.UpdatedAt,
		})
	}

	return plugins, nil
}

// listFromRegistry fetches the registry and returns PluginInfo for every
// known plugin, marking those already installed locally.
func listFromRegistry(pluginDir string) ([]PluginInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cacheDir := defaultCacheDir()
	reg, err := FetchRegistry(ctx, "", cacheDir)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}

	// Build a set of installed plugin names for quick lookup.
	installedSet := make(map[string]bool)
	if entries, dirErr := os.ReadDir(pluginDir); dirErr == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				installedSet[entry.Name()] = true
			}
		}
	}

	plugins := make([]PluginInfo, 0, len(reg.Plugins))
	for _, m := range reg.Plugins {
		installed := installedSet[m.Name]
		running := false
		if installed {
			if st, err := Status(m.Name); err == nil && st.State == "running" {
				running = true
			}
		}
		plugins = append(plugins, PluginInfo{
			Name:          m.Name,
			Version:       m.Version,
			Category:      m.Category,
			Installed:     installed,
			Running:       running,
			PublishStatus: m.PublishStatus,
			UpdatedAt:     m.UpdatedAt,
		})
	}

	return plugins, nil
}

// baseServiceRoutes lists the always-on nSelf service routes that are present
// on any clean init. Plugins must not claim these paths — they are owned by
// core infrastructure and seeded into the conflict-detection set before any
// plugin routes are evaluated.
var baseServiceRoutes = []nginx.NginxRoute{
	{ServerName: "api", Location: "/", PluginName: "hasura"},
	{ServerName: "auth", Location: "/", PluginName: "auth"},
	{ServerName: "storage", Location: "/", PluginName: "storage"},
}

// collectInstalledPluginRoutes scans pluginDir and returns the nginx routes
// declared by all installed plugins except the one named skipPlugin (the plugin
// being installed, so reinstalls are permitted). Base service routes (Hasura,
// Auth, Storage) are seeded first so plugins that claim those paths are
// rejected with a clear conflict message naming the base service as owner.
func collectInstalledPluginRoutes(pluginDir, skipPlugin string) []nginx.NginxRoute {
	// Seed with always-on base service routes. This prevents false-positive
	// "clean install succeeded" for plugins that try to claim /api, /auth, or
	// /storage — and produces a clear "claimed by hasura/auth/storage" message.
	routes := make([]nginx.NginxRoute, len(baseServiceRoutes))
	copy(routes, baseServiceRoutes)

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return routes
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if strings.EqualFold(entry.Name(), skipPlugin) {
			continue
		}
		manifestPath := filepath.Join(pluginDir, entry.Name(), "plugin.json")
		m, err := parseManifest(manifestPath)
		if err != nil {
			continue
		}
		for _, endpoint := range m.APIEndpoints {
			ep := strings.TrimPrefix(endpoint, "https://")
			ep = strings.TrimPrefix(ep, "http://")
			parts := strings.SplitN(ep, "/", 2)
			serverName := parts[0]
			location := "/"
			if len(parts) == 2 && parts[1] != "" {
				location = "/" + parts[1]
			}
			routes = append(routes, nginx.NginxRoute{
				ServerName: serverName,
				Location:   location,
				PluginName: m.Name,
			})
		}
	}
	return routes
}
