package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
)

// RenderPluginConfigs walks installed plugins' configs/ directories
// and renders templates into the project workdir. For each installed
// plugin that has a configs/ directory, every file inside it is read,
// ${VAR} substitutions are replaced with project values, and the result
// is written to {workdir}/{relative_path} (stripping the configs/ prefix).
//
// Returns the number of files rendered, or an error if any file
// operation fails.
func RenderPluginConfigs(workdir, pluginDir string, cfg *config.Config) (int, error) {
	vars := buildTemplateVars(workdir, cfg)

	// List installed plugins by reading directories under pluginDir.
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading plugin directory %s: %w", pluginDir, err)
	}

	rendered := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		configsDir := filepath.Join(pluginDir, entry.Name(), "configs")
		info, err := os.Stat(configsDir)
		if err != nil || !info.IsDir() {
			// Plugin has no configs/ directory; skip.
			continue
		}

		count, err := renderPluginConfigDir(workdir, configsDir, vars)
		if err != nil {
			return rendered, fmt.Errorf("rendering configs for plugin %s: %w", entry.Name(), err)
		}
		rendered += count
	}

	return rendered, nil
}

// renderPluginConfigDir walks a single plugin's configs/ directory and
// renders each file into workdir, preserving the relative path structure
// beneath configs/.
func renderPluginConfigDir(workdir, configsDir string, vars map[string]string) (int, error) {
	rendered := 0

	err := filepath.Walk(configsDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		// Compute relative path from the configs/ directory.
		relPath, err := filepath.Rel(configsDir, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}

		// Read template content.
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading template %s: %w", path, err)
		}

		// Render ${VAR} substitutions.
		output := renderTemplate(string(content), vars)

		// Write rendered file to workdir, preserving directory structure.
		destPath := filepath.Join(workdir, relPath)
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", destDir, err)
		}
		if err := os.WriteFile(destPath, []byte(output), 0644); err != nil {
			return fmt.Errorf("writing rendered config %s: %w", destPath, err)
		}

		rendered++
		return nil
	})

	return rendered, err
}

// renderTemplate performs simple ${KEY} variable substitution on a
// template string. It does NOT use Go's text/template package because
// plugin config files (e.g., prometheus.yml, alertmanager.yml) may
// contain {{ }} syntax for their own templating engines.
func renderTemplate(content string, vars map[string]string) string {
	result := content
	for key, value := range vars {
		result = strings.ReplaceAll(result, "${"+key+"}", value)
	}
	return result
}

// buildTemplateVars constructs the variable map used to template plugin
// config and nginx files. Provides the standard set of variables that
// plugins can reference via ${VAR_NAME} substitution syntax.
//
// This function is shared by RenderPluginConfigs and
// InjectPluginNginxRoutes (in plugin_nginx.go).
func buildTemplateVars(workdir string, cfg *config.Config) map[string]string {
	network := cfg.DockerNetwork
	if network == "" {
		network = cfg.ProjectName + "_network"
	}

	pluginDir := expandPluginDir(cfg.PluginSystem.Dir)
	if pluginDir == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			pluginDir = filepath.Join(home, ".nself", "plugins")
		}
	}

	return map[string]string{
		"SSL_DIR":              pluginSSLDir(cfg.BaseDomain),
		"PROJECT_NAME":         cfg.ProjectName,
		"BASE_DOMAIN":          cfg.BaseDomain,
		"POSTGRES_USER":        cfg.Postgres.User,
		"POSTGRES_PASSWORD":    cfg.Postgres.Password,
		"POSTGRES_DB":          cfg.Postgres.DB,
		"DOCKER_NETWORK":       network,
		"COMPOSE_PROJECT_NAME": cfg.ProjectName,
		"HASURA_PORT":          fmt.Sprintf("%d", cfg.Hasura.Port),
		"AUTH_PORT":            fmt.Sprintf("%d", cfg.Auth.Port),
		"NSELF_PLUGIN_DIR":     pluginDir,
	}
}

// pluginSSLDir is the ${SSL_DIR} of a plugin snippet: the base domain's
// certificate dir relative to the nginx ssl mount, the one the CLI's own
// templates read (service.conf.tmpl: <mount>/certificates/<dir>/fullchain.pem)
// and internal/ssl.Generator writes. Snippets say
// /etc/nginx/ssl/${SSL_DIR}/fullchain.pem, so the value carries the
// certificates/ level. An empty domain falls back to localhost, as the
// generator does.
func pluginSSLDir(baseDomain string) string {
	dir := ssl.DomainToDirName(baseDomain)
	if dir == "" {
		dir = "localhost"
	}
	return "certificates/" + dir
}

// expandPluginDir is expandHomeDir under v1.5 only. v1.4.13 used PLUGIN_DIR
// as written, so the default "~/.nself/plugins" never resolved and a project
// on default config got no plugin nginx snippets and ${NSELF_PLUGIN_DIR}
// rendered as "~/.nself/plugins". Expanding it in a patch would publish every installed
// plugin at <plugin>.<base> on the first rebuild after an upgrade and fail
// the build on a snippet the user had copied into nginx/sites by hand
// (D-0634), so v1.4 keeps the old value.
func expandPluginDir(dir string) string {
	// compat.V15(D-0478): PLUGIN_DIR "~/..." used as written, so default-config projects get no plugin nginx snippets -> "~" expands to the home dir and installed plugins' snippets are injected
	if compat.V15() {
		return expandHomeDir(dir)
	}
	return dir
}

// expandHomeDir turns a leading "~" or "~/" into the user's home dir.
// config.ApplyDefaults sets PLUGIN_DIR to the literal "~/.nself/plugins",
// which os.Stat would otherwise read relative to the working dir, so no
// plugin snippet was ever found. Anything else is returned unchanged.
func expandHomeDir(dir string) string {
	if dir != "~" && !strings.HasPrefix(dir, "~/") {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return dir
	}
	return filepath.Join(home, strings.TrimPrefix(dir, "~"))
}
