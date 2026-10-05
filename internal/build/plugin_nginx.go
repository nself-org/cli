package build

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// claimsOf returns the set of "<port>|<server_name>" claims a rendered
// nginx config makes, using the shared block parser in
// nginx_server_blocks.go. Keying on port as well as name matches nginx's
// own conflict rule: the same name served on :80 by one file and :443 by
// another is a legitimate split, not a conflict.
func claimsOf(content string) map[string]bool {
	claims := make(map[string]bool)
	for _, block := range parseServerBlocks(content) {
		ports := block.Ports
		if len(ports) == 0 {
			ports = []string{defaultListenPort}
		}
		for _, name := range block.ServerNames {
			if name == "_" {
				continue // the catch-all default server, intentionally shared
			}
			for _, port := range ports {
				claims[port+"|"+name] = true
			}
		}
	}
	return claims
}

// checkServerNameConflict reports whether any server_name/port pair declared
// in content is already claimed by a DIFFERENT .conf file already sitting in
// sitesDir. destName is excluded from the scan so a plugin re-running
// `nself build` and rewriting its own previously-injected file is not a
// conflict with itself.
//
// Returns an error naming both files and the shared name so the build fails
// loudly, instead of nginx silently picking one of two server blocks at
// runtime (2026-09-03 staging: two generated site files both claimed
// "api.staging.nself.org"; nginx logged "conflicting server name ...
// ignored" and kept serving whichever loaded last).
//
// This is the early, precise check on the plugin-injection path.
// checkServerNameUniqueness in postvalidate_nginx.go is the backstop that
// sweeps the whole directory after every writer has run.
//
// sitesDir is read through sink, so plan mode sees the site confs this build
// already planned.
func checkServerNameConflict(sink Sink, sitesDir, destName, content string) error {
	newClaims := claimsOf(content)
	if len(newClaims) == 0 {
		return nil
	}

	entries, err := sink.ReadDir(sitesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading %s: %w", sitesDir, err)
	}

	// Sorted so the file named in the error is the same on every run;
	// os.ReadDir order is not guaranteed across platforms.
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && entry.Name() != destName {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		existing, readErr := sink.ReadFile(filepath.Join(sitesDir, name))
		if readErr != nil {
			continue
		}
		existingClaims := claimsOf(string(existing))
		for claim := range newClaims {
			if !existingClaims[claim] {
				continue
			}
			parts := strings.SplitN(claim, "|", 2)
			return fmt.Errorf("nginx server_name conflict: %q on port %s is claimed by both %s and %s — nginx would silently ignore one of them (\"conflicting server name\"); rename one route or remove the duplicate", parts[1], parts[0], name, destName)
		}
	}
	return nil
}

// InjectPluginNginxRoutes scans pluginDir for installed plugins that ship
// nginx route configs (in a nginx/ subdirectory) and copies them into the
// project's nginx/sites/ directory after templating config variables.
//
// Returns the number of config files injected.
// Plugins without a nginx/ directory are silently skipped.
func InjectPluginNginxRoutes(workdir, pluginDir string, cfg *config.Config) (int, error) {
	return injectPluginNginxRoutesVia(newDiskSink(workdir), workdir, pluginDir, cfg)
}

// injectPluginNginxRoutesVia is InjectPluginNginxRoutes writing through sink.
func injectPluginNginxRoutesVia(sink Sink, workdir, pluginDir string, cfg *config.Config) (int, error) {
	if pluginDir == "" {
		pluginDir = cfg.PluginSystem.Dir
	}
	if pluginDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return 0, fmt.Errorf("resolving home directory: %w", err)
		}
		pluginDir = filepath.Join(home, ".nself", "plugins")
	}

	// If the plugin directory does not exist, there is nothing to inject.
	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		return 0, nil
	}

	// Reuse the shared template variable map.
	vars := buildTemplateVars(workdir, cfg)

	// Scan each plugin directory for nginx/*.conf files.
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return 0, fmt.Errorf("reading plugin directory %s: %w", pluginDir, err)
	}

	count := 0
	// Served sites dir (D-0045): a fronted project's plugin confs must land in
	// the fronting stack's nginx/sites, the tree its nginx actually reads.
	sitesDir, err := resolveNginxSitesDir(workdir, cfg.Nginx.FrontedBy)
	if err != nil {
		return 0, err
	}
	if err := sink.MkdirAll(sitesDir, 0755); err != nil {
		return 0, fmt.Errorf("creating %s: %w", sitesDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pluginName := entry.Name()
		nginxDir := filepath.Join(pluginDir, pluginName, "nginx")

		// Skip plugins without an nginx/ directory.
		if _, err := os.Stat(nginxDir); os.IsNotExist(err) {
			continue
		}

		// Glob for .conf files inside the plugin's nginx/ directory.
		pattern := filepath.Join(nginxDir, "*.conf")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return count, fmt.Errorf("globbing %s: %w", pattern, err)
		}

		for _, match := range matches {
			content, err := os.ReadFile(match)
			if err != nil {
				return count, fmt.Errorf("reading %s: %w", match, err)
			}

			// Template the config content with project values.
			// Reuses renderTemplate from plugin_configs.go.
			rendered := renderTemplate(string(content), vars)

			// Write to nginx/sites/{pluginname}-{filename} to avoid conflicts.
			filename := filepath.Base(match)
			destName := pluginName + "-" + filename
			destPath := filepath.Join(sitesDir, destName)

			// A duplicate server_name across two site files is not a warning
			// — nginx silently serves only one of them ("conflicting server
			// name ... ignored"). Fail the build so the conflict is fixed
			// before it ever reaches nginx, naming both sources.
			if err := checkServerNameConflict(sink, sitesDir, destName, rendered); err != nil {
				return count, err
			}

			if err := sink.WriteFile(destPath, []byte(rendered), 0644); err != nil {
				return count, fmt.Errorf("writing %s: %w", destPath, err)
			}
			count++
		}
	}

	return count, nil
}
