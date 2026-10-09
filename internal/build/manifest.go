package build

// Purpose: Read the project's nself.yaml manifest and guarantee that every
//          declared plugin is wired into the generated stack (auto-installing
//          missing plugins when possible) — or loudly reported, never silently
//          dropped (PCI plugin-injection-dropped, 2026-07-03).
// Inputs:  workdir (project root containing nself.yaml), pluginDir (global
//          plugin install dir), cfg (resolved env-cascade config), generated
//          compose service names.
// Outputs: Declared plugin list, missing-plugin report, best-effort installs.
// Constraints: Backward compatible — projects without nself.yaml keep the
//              install-then-discover flow unchanged. Auto-install is
//              best-effort and can be disabled with
//              NSELF_AUTO_INSTALL_PLUGINS=false (offline/CI).
// SPORT: cli/internal/build — manifest injection (gap #8, ntask dogfood).

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/bundle"
)

// manifestFilenames are the accepted names for the project manifest, checked
// in order. The canonical name is nself.yaml.
var manifestFilenames = []string{"nself.yaml", "nself.yml"}

// ProjectManifest is the parsed subset of nself.yaml that the build pipeline
// consumes. Unknown keys are ignored so app repos can keep richer metadata
// (display_name, tier, auth_mode, ...) in the same file.
type ProjectManifest struct {
	App           string          `yaml:"app"`
	CLIMinVersion string          `yaml:"cli_min_version"`
	Bundle        string          `yaml:"bundle"`
	Bundles       []string        `yaml:"bundles"`
	Plugins       ManifestPlugins `yaml:"plugins"`
}

// ManifestPlugins accepts both manifest shapes:
//
//	plugins:            # flat list
//	  - cron
//	  - notify
//
//	plugins:            # tiered map
//	  free: [cron, notify]
//	  pro:  [ai-gateway]
type ManifestPlugins struct {
	Free []string
	Pro  []string
	Flat []string
}

// UnmarshalYAML implements custom decoding for the two supported shapes.
func (mp *ManifestPlugins) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&mp.Flat)
	case yaml.MappingNode:
		var aux struct {
			Free []string `yaml:"free"`
			Pro  []string `yaml:"pro"`
		}
		if err := node.Decode(&aux); err != nil {
			return fmt.Errorf("parsing plugins block: %w", err)
		}
		mp.Free = aux.Free
		mp.Pro = aux.Pro
		return nil
	}
	// Null/absent — leave zero value.
	return nil
}

// LoadProjectManifest reads nself.yaml (or nself.yml) from workdir.
// Returns (nil, nil) when no manifest exists — callers treat that as
// "no declarations, legacy discover-only flow".
func LoadProjectManifest(workdir string) (*ProjectManifest, error) {
	for _, name := range manifestFilenames {
		path := filepath.Join(workdir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", name, err)
		}
		var m ProjectManifest
		if err := yaml.Unmarshal(data, &m); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", name, err)
		}
		return &m, nil
	}
	return nil, nil
}

// DeclaredPlugins returns the deduplicated, sorted set of plugin slugs the
// manifest declares: plugins.free + plugins.pro + flat list + the expansion
// of bundle/bundles via the canonical bundle catalog. Placeholder entries
// (anything that is not a bare slug, e.g. "(free plugins only ...)") are
// dropped.
func (m *ProjectManifest) DeclaredPlugins() []string {
	if m == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(name string) {
		slug := strings.ToLower(strings.TrimSpace(name))
		if !isPluginSlug(slug) || seen[slug] {
			return
		}
		seen[slug] = true
		out = append(out, slug)
	}

	for _, n := range m.Plugins.Flat {
		add(n)
	}
	for _, n := range m.Plugins.Free {
		add(n)
	}
	for _, n := range m.Plugins.Pro {
		add(n)
	}

	bundles := m.Bundles
	if m.Bundle != "" {
		bundles = append(bundles, m.Bundle)
	}
	if len(bundles) > 0 {
		// P6-E4-W3-S3-T10 CI regression: bundle.Load() is only wired into
		// `nself bundle`'s PersistentPreRunE. Every OTHER command that
		// reaches this expansion (build/start/restart/stop/ops/status, via
		// internal/build's manifest loader) needs the resolver populated
		// too, or a manifest's `bundles:` entries silently expand to
		// nothing — exactly the silent-drop this file's own PCI charter
		// forbids. EnsureLoaded is idempotent: a no-op once any caller in
		// this process has already loaded the registry.
		if err := bundle.EnsureLoaded(context.Background()); err != nil {
			// Never silently drop: loudly report and continue with
			// whatever plugins are already flat-declared. A manifest that
			// ONLY uses bundles: with no cache and no network will end up
			// with zero plugins here — surfaced, not hidden.
			slog.Warn("could not resolve bundle catalog — declared bundle(s) will NOT be expanded",
				"bundles", bundles, "err", err)
		}
	}
	for _, slug := range bundles {
		b, ok := bundle.Get(slug)
		if !ok {
			continue
		}
		for _, p := range b.Plugins {
			add(p)
		}
	}

	sort.Strings(out)
	return out
}

// isPluginSlug reports whether s looks like a valid plugin slug:
// lowercase letters, digits, and hyphens only.
func isPluginSlug(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// coreServiceAliases maps declared plugin names that are satisfied by core
// compose services rather than plugin containers. "auth" and "storage" are
// wired by the compose generator directly (hasura-auth container + MinIO),
// so declaring them must not produce a false "missing plugin" report.
var coreServiceAliases = map[string][]string{
	"auth":    {"auth"},
	"storage": {"minio", "storage"},
}

// autoInstallEnabled reports whether missing declared plugins may be
// auto-installed from the registry. Disabled with
// NSELF_AUTO_INSTALL_PLUGINS=false (offline builds, hermetic CI).
func autoInstallEnabled() bool {
	return !strings.EqualFold(os.Getenv("NSELF_AUTO_INSTALL_PLUGINS"), "false")
}

// autoInstallTimeout bounds a single best-effort plugin auto-install so an
// unreachable registry can never hang `nself build`.
const autoInstallTimeout = 60 * time.Second
