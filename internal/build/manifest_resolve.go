// manifest_resolve.go — ResolveDeclaredPlugins and its helpers: reconciling
// a project manifest's declared plugins against what's actually installed
// and wired into the generated stack. Split out of manifest.go to stay
// under the 300-line/file cap (internal/repoqa).
package build

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/plugin"
)

// PluginInstall installs one plugin. A variable so tests can stand in for the
// registry download and the database schema step.
var PluginInstall = plugin.Install

// InstallDeclaredPlugins installs the plugins nself.yaml declares that are not
// yet installed, exactly as a write-mode build would at that step, without
// rendering anything. reconcile.Apply runs it as a confirmed effect before its
// final render, so the render that is checked and held already contains the
// plugins' nginx and compose artifacts. Install failures are warnings, as in a
// build; the render then reports the plugin as missing. The environment is left
// as found.
func InstallDeclaredPlugins(ctx context.Context, workdir string) error {
	defer SnapshotEnv()()
	repin := pinCompatEnv()
	cfg, err := config.Load(workdir)
	repin()
	if err != nil {
		return fmt.Errorf("loading config to install declared plugins: %w", err)
	}
	_ = resolveDeclaredPluginsFx(ctx, writeEffects{}, cfg, workdir, DefaultPluginDir(), expectedCoreServices(cfg))
	return nil
}

// ResolveDeclaredPlugins guarantees every plugin declared in nself.yaml is
// wired into the build, in three steps per declared plugin:
//
//  1. Already installed in pluginDir → satisfied (its compose fragment, if
//     any, is picked up by DiscoverPluginComposeFiles).
//  2. Provided by a core compose service (auth, storage) → satisfied.
//  3. Otherwise attempt a best-effort auto-install from the registry, then
//     re-check. Still absent → reported in missing.
//
// Returns the list of declared plugins that could not be wired. Callers MUST
// surface missing plugins to the user (build warning + BuildResult) — a
// declared plugin is never silently dropped.
func ResolveDeclaredPlugins(ctx context.Context, cfg *config.Config, workdir, pluginDir string, composeServices []string) (missing []string) {
	return resolveDeclaredPluginsFx(ctx, writeEffects{}, cfg, workdir, pluginDir, composeServices)
}

// resolveDeclaredPluginsFx is ResolveDeclaredPlugins with the auto-install
// routed through fx. Plan mode records a plugin-install effect, makes no
// network call, and treats the plugin as wired (apply installs it).
func resolveDeclaredPluginsFx(ctx context.Context, fx Effects, cfg *config.Config, workdir, pluginDir string, composeServices []string) (missing []string) {
	manifest, err := LoadProjectManifest(workdir)
	if err != nil {
		slog.Warn("could not parse project manifest — declared plugins not resolved", "err", err)
		return nil
	}
	declared := manifest.DeclaredPlugins()
	if len(declared) == 0 {
		return nil
	}

	serviceSet := make(map[string]bool, len(composeServices))
	for _, s := range composeServices {
		serviceSet[s] = true
	}

	for _, name := range declared {
		if pluginInstalled(pluginDir, name) {
			continue
		}
		if satisfiedByCoreService(name, serviceSet) {
			continue
		}
		if autoInstallEnabled() {
			var err error
			_ = fx.Do(EffectPluginInstall, name, "auto-install declared plugin from the registry (nself.yaml)", func() error {
				installCtx, cancel := context.WithTimeout(ctx, autoInstallTimeout)
				defer cancel()
				err = PluginInstall(installCtx, cfg, name, pluginDir)
				return nil
			})
			if fx.Planning() {
				continue
			}
			if err == nil && pluginInstalled(pluginDir, name) {
				slog.Info("auto-installed declared plugin", "plugin", name, "source", "nself.yaml")
				continue
			}
			slog.Warn("declared plugin auto-install failed", "plugin", name, "err", err)
		}
		missing = append(missing, name)
	}

	return missing
}

// pluginInstalled reports whether the named plugin has an install directory
// (with at least a plugin.json or compose fragment) under pluginDir.
func pluginInstalled(pluginDir, name string) bool {
	dir := filepath.Join(pluginDir, name)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false
	}
	// A directory with a manifest or compose fragment counts as installed.
	for _, f := range []string{"plugin.json", pluginComposeFilename} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// satisfiedByCoreService reports whether a declared plugin name is provided
// by a generated core compose service instead of a plugin container.
func satisfiedByCoreService(name string, serviceSet map[string]bool) bool {
	if serviceSet[name] {
		return true
	}
	for _, alias := range coreServiceAliases[name] {
		if serviceSet[alias] {
			return true
		}
	}
	return false
}

// expectedCoreServices returns the compose service names the generator will
// emit for core/optional services, derived from config. Used by
// ResolveDeclaredPlugins to recognise declared names (auth, storage) that a
// core service satisfies — the compose YAML is generated after plugin
// resolution, so the names are derived rather than parsed.
func expectedCoreServices(cfg *config.Config) []string {
	services := []string{"postgres", "hasura", "auth"}
	if cfg.Nginx.FrontedBy == "" {
		services = append(services, "nginx")
	}
	if cfg.Minio.Enabled {
		services = append(services, "minio")
	}
	if cfg.Redis.Enabled {
		services = append(services, "redis")
	}
	if cfg.Functions.Enabled {
		services = append(services, "functions")
	}
	if cfg.Mailpit.Enabled {
		services = append(services, "mailpit")
	}
	return services
}
