package build

// Purpose: derive the serving proxy and refuse incomplete swaps before writes.
// Inputs: resolved config, project path, installed plugins and route snapshot.
// Outputs: provider name or a deterministic refusal.
// Constraints: no Docker calls or provider rendering; ADR 0027 and EPIC D3.
import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

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
	// Compose discovery uses filesystem paths relative to the process cwd and
	// does not expand ~/. Treat a literal tilde directory exactly as it does.
	absDir, err := filepath.Abs(pluginDir)
	if err != nil {
		return "", err
	}
	dir, err := proxyRealPath(absDir, project)
	if os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return ProviderNginx, nil
	}
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(dir, "traefik", "plugin.json")); os.IsNotExist(err) || errors.Is(err, syscall.ENOTDIR) {
		return ProviderNginx, nil
	} else if err != nil {
		return "", err
	}
	if plugin.IsDisabled("traefik", dir) {
		return ProviderNginx, nil
	}
	if err := proxyLocalDir(project, dir); err != nil {
		return "", err
	}
	if err := proxyComposeDiscoverable(workdir, pluginDir, ProviderTraefik); err != nil {
		return "", err
	}
	return ProviderTraefik, nil
}

// proxyLocalDir enforces the same isolation for installed and declared plugins.
func proxyLocalDir(project, dir string) error {
	rel, err := filepath.Rel(project, dir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errs.New("E480", "traefik installed in shared plugin directory "+dir).
			WithWhy("One install there would flip every project using that directory at its next build (EPIC D3).").
			WithFix("Export NSELF_PLUGIN_DIR=" + filepath.Join(project, ".nself", "plugins") + " in the shell for nself add, and set NSELF_PLUGIN_DIR in this project's .env for builds; reinstall there, or run nself remove traefik from the shared directory.")
	}
	return nil
}

// proxyComposeDiscoverable uses actual discovery with an isolated memory sink.
// Normalisation records plan effects and writes only memory, never plugin files.
func proxyComposeDiscoverable(workdir, pluginDir, provider string) error {
	files, err := discoverPluginComposeFilesFx(&planEffects{}, newMemSink(workdir, ""), workdir, pluginDir)
	expected, pathErr := filepath.Abs(filepath.Join(pluginDir, "traefik", pluginComposeFilename))
	if pathErr != nil {
		return pathErr
	}
	// Missing traefik must remain E480 even if v1.5 discovery reports E128.
	if provider == ProviderTraefik {
		if _, statErr := os.Stat(expected); os.IsNotExist(statErr) || errors.Is(statErr, syscall.ENOTDIR) {
			return proxyMissingFragment(expected)
		}
	}
	if errors.Is(err, syscall.ENOTDIR) && provider == ProviderNginx {
		return nil
	}
	if err != nil {
		return err
	}
	for _, file := range files {
		if provider == ProviderTraefik && file == expected {
			return nil
		}
		if provider == ProviderNginx && proxyTraefikFragment(file) {
			return errs.New("E480", "a traefik fragment would load while nginx serves 80/443: "+file+"; remove it or reinstall traefik").
				WithFix("Export NSELF_PLUGIN_DIR in the shell for nself add and set it in .env for builds, using this project's plugin directory.")
		}
	}
	if provider == ProviderNginx {
		return nil
	}
	if info, err := os.Lstat(filepath.Dir(expected)); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return errs.New("E480", "traefik entry "+filepath.Dir(expected)+" is not loaded by compose discovery").
			WithWhy("Symlinked plugin entries are not loaded.").
			WithFix("Install the plugin as a real directory inside this project; export NSELF_PLUGIN_DIR in the shell for nself add and set it in .env for builds.")
	}
	return proxyMissingFragment(expected)
}

func proxyMissingFragment(expected string) error {
	return errs.New("E480", "traefik compose fragment is not loaded at "+expected).
		WithWhy("Provider lookup and compose discovery must find the same project-local plugin.").
		WithFix("Install traefik with its compose fragment in a real directory inside this project; export an absolute NSELF_PLUGIN_DIR in the shell for nself add and set it in .env for builds.")
}

func proxyTraefikFragment(file string) bool {
	name := filepath.Base(filepath.Dir(file))
	return name == "traefik" || strings.HasPrefix(name, "traefik.")
}

// proxyRealPath follows existing ancestors even when the final directory is absent.
func proxyRealPath(path, workdir string) (string, error) {
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
	if err := proxyFrontingStack(cfg, workdir); err != nil {
		return err
	}
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
			raw := r.RawUpstream
			if !strings.Contains(raw, "://") {
				raw = "http://" + raw
			}
			u, err := url.Parse(raw)
			if err != nil {
				items = append(items, fmt.Sprintf("route %s: upstream %s cannot be parsed", r.ID, r.RawUpstream))
			} else if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery {
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
