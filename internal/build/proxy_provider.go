package build

// Purpose: derive the serving proxy and refuse incomplete swaps before writes.
// Inputs: resolved config, project path, installed plugins and route snapshot.
// Outputs: provider name or a deterministic refusal.
// Constraints: no Docker calls or provider rendering; ADR 0027 and EPIC D3.
import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/plugin"
)

const ProviderNginx = "nginx"
const ProviderTraefik = "traefik"

// ProxyProvider selects Traefik only for an enabled project-local installation.
func ProxyProvider(cfg *config.Config, workdir, pluginDir string) (string, error) {
	workdir, err := filepath.Abs(workdir)
	if err != nil {
		return "", err
	}
	project, err := proxyRealPath(workdir, workdir)
	if err != nil {
		return "", err
	}
	dir, err := proxyRealPath(pluginDir, project)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "traefik", "plugin.json")); os.IsNotExist(err) {
		return ProviderNginx, nil
	} else if err != nil {
		return "", err
	}
	if plugin.IsDisabled("traefik", dir) {
		return ProviderNginx, nil
	}
	rel, err := filepath.Rel(project, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errs.New("E480", "traefik installed in shared plugin directory "+dir).
			WithWhy("One install there would flip every project using that directory at its next build (EPIC D3).").
			WithFix("Set NSELF_PLUGIN_DIR (PLUGIN_DIR) in this project's .env to " + filepath.Join(project, ".nself", "plugins") + " and reinstall, or run nself remove traefik from the shared directory.")
	}
	return ProviderTraefik, nil
}

// proxyRealPath follows existing ancestors even when the final directory is absent.
func proxyRealPath(path, workdir string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(workdir, path)
	}
	path = filepath.Clean(path)
	real, err := filepath.EvalSymlinks(path)
	if err == nil {
		return real, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path, nil
	}
	real, err = proxyRealPath(parent, workdir)
	return filepath.Join(real, filepath.Base(path)), err
}

// proxyAPIDocsEnabled mirrors writeFinalArtifacts' empty-section default.
func proxyAPIDocsEnabled(cfg *config.Config) bool {
	return cfg.ApiDocs.Enabled || cfg.ApiDocs.Path == ""
}

// checkProviderRoutes collects every incompatibility before refusing a swap.
func checkProviderRoutes(m *routemodel.Model, provider string, cfg *config.Config, workdir string) error {
	if provider == ProviderNginx {
		return nil
	}
	if cfg.Nginx.FrontedBy != "" {
		return errs.New("E482", "traefik refused for a fronted project").
			WithWhy("A fronted project does not own the serving proxy.").
			WithFix("Keep nginx here, change the provider in the fronting project, or unset NGINX_FRONTED_BY.")
	}
	items, names := proxyRouteLimitations(m)
	if proxyAPIDocsEnabled(cfg) {
		names["api-docs.conf"] = true
	}
	if err := proxyPluginSiteNames(cfg, names); err != nil {
		return err
	}
	foreign, err := proxyForeignSites(workdir, names)
	if err != nil {
		return err
	}
	items = append(items, foreign...)
	if len(items) == 0 {
		return nil
	}
	sort.Strings(items)
	return errs.New("E481", "traefik cannot serve every route:\n"+strings.Join(items, "\n")).
		WithWhy("The swap either serves every route the same way or does not happen (EPIC D3).").
		WithFix("Remove or model the named items, or keep nginx with nself remove traefik. A refused add leaves traefik installed.")
}

// proxyRouteLimitations collects lossy routes and the files this model writes.
func proxyRouteLimitations(m *routemodel.Model) ([]string, map[string]bool) {
	var items []string
	names := map[string]bool{}
	for _, r := range m.Routes {
		for _, u := range r.Unmodelled {
			items = append(items, fmt.Sprintf("route %s: unmodelled %s", r.ID, u))
		}
		if r.ShadowedBy != nil {
			items = append(items, fmt.Sprintf("route %s: shadowed by %s", r.ID, *r.ShadowedBy))
		}
		if r.Source != "plugin" && r.Source != "hand_managed" {
			names[filepath.Base(r.File)] = true
			u, err := url.Parse(r.RawUpstream)
			if err == nil && ((u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery) {
				items = append(items, fmt.Sprintf("route %s: upstream %s has a path or query routes.json v1 cannot carry (D-0310)", r.ID, r.RawUpstream))
			}
		}
	}
	for _, u := range m.UnmodelledGlobal {
		items = append(items, "global: "+u)
	}
	return items, names
}

// proxyPluginSiteNames matches injection's directory resolution and filenames.
func proxyPluginSiteNames(cfg *config.Config, names map[string]bool) error {
	dir, err := resolvePluginNginxDir("", cfg)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		paths, err := pluginConfPaths(dir, e.Name())
		if err != nil {
			return err
		}
		for _, p := range paths {
			names[e.Name()+"-"+filepath.Base(p)] = true
		}
	}
	return nil
}

// proxyForeignSites reports every served conf this build does not own.
func proxyForeignSites(workdir string, names map[string]bool) ([]string, error) {
	var items []string
	files, err := filepath.Glob(filepath.Join(workdir, "nginx", "sites", "*.conf"))
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		marked := strings.Contains(string(data[:min(len(data), 128)]), nginxGeneratedMarker)
		name := filepath.Base(file)
		if !marked {
			items = append(items, "file nginx/sites/"+name+": not generated by nself (hand-written or another project's)")
		} else if !names[name] {
			items = append(items, "file nginx/sites/"+name+": generated by another build (stale, or a project fronted by this one)")
		}
	}
	return items, nil
}
