package apidocs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginRoute is one REST route contributed by a plugin via its plugin.yaml
// rest_routes key.
type PluginRoute struct {
	// PluginName is the plugin identifier (e.g. "ai", "notify").
	PluginName string
	// Method is the HTTP method (GET, POST, PUT, PATCH, DELETE).
	Method string
	// Path is the URL path (e.g. "/ai/v1/complete").
	Path string
	// Summary is a short human-readable description.
	Summary string
}

// pluginManifest mirrors the subset of plugin.yaml we need for REST routes.
type pluginManifest struct {
	Name       string          `json:"name"`
	RestRoutes []manifestRoute `json:"rest_routes"`
}

type manifestRoute struct {
	Method  string `json:"method"`
	Path    string `json:"path"`
	Summary string `json:"summary"`
}

// CollectPluginRoutes walks pluginDir and reads rest_routes from each plugin's
// manifest (plugin.yaml or plugin.json). Missing manifests are silently skipped.
func CollectPluginRoutes(pluginDir string) ([]PluginRoute, error) {
	if pluginDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolving home directory: %w", err)
		}
		pluginDir = filepath.Join(home, ".nself", "plugins")
	}

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return nil, fmt.Errorf("reading plugin dir: %w", err)
	}

	var routes []PluginRoute
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		manifest, err := loadPluginManifest(filepath.Join(pluginDir, e.Name()))
		if err != nil || manifest == nil {
			continue
		}
		name := manifest.Name
		if name == "" {
			name = e.Name()
		}
		for _, r := range manifest.RestRoutes {
			if r.Method == "" || r.Path == "" {
				continue
			}
			summary := r.Summary
			if summary == "" {
				summary = name + " " + r.Method + " " + r.Path
			}
			routes = append(routes, PluginRoute{
				PluginName: name,
				Method:     r.Method,
				Path:       r.Path,
				Summary:    summary,
			})
		}
	}
	return routes, nil
}

// loadPluginManifest attempts to read plugin.json from pluginPath.
// Returns nil (no error) when no manifest is found.
func loadPluginManifest(pluginPath string) (*pluginManifest, error) {
	candidates := []string{
		filepath.Join(pluginPath, "plugin.json"),
	}
	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var m pluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", c, err)
		}
		return &m, nil
	}
	return nil, nil
}
