package build

// orchestrator_nginx.go — the build's route snapshot, and the plugin and
// hand-managed routes added to routes.json (P7-DEPL-22).
//
// Purpose: modelRoutes gives the preflight and the renderer one route
// snapshot. withSnippetRoutes extends a copy of it with the routes parsed from
// the plugin snippets this build copies and from the hand-managed
// nginx/conf.d*/ files, for routes.json only.
// Inputs: the plugin dir, the served nginx dir, the model.
// Outputs: a model with source plugin / hand_managed routes and the
// top-level `unmodelled_global` list.
// Constraints: the extended copy is used only to write routes.json. The
// duplicate preflight, SetSSL and the nginx renderer treat every route as a
// generated service (one server_name per route, TLS reset to the project's,
// one nginx/sites file each), which would break a snippet's http and https
// server blocks for one name. Nothing here writes under nginx/, so nginx
// output does not change.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// modelRoutes is the build preflight and renderer's single route snapshot.
func (st *buildState) modelRoutes(hasSSL bool, trusted func(string) bool) (*routemodel.Model, error) {
	return routemodel.Build(st.cfg, st.workdir, hasSSL, trusted)
}

// withSnippetRoutes returns a copy of m extended with the plugin and
// hand-managed routes. m itself is not changed.
func (st *buildState) withSnippetRoutes(m *routemodel.Model) *routemodel.Model {
	set := &snippetSet{}
	set.plugins(st.workdir, st.cfg)
	set.handManaged(st.workdir, st.cfg, m.Env)
	out := *m
	out.Routes = append(append([]routemodel.Route{}, m.Routes...), set.routes...)
	out.UnmodelledGlobal = append(append([]string{}, m.UnmodelledGlobal...), set.global...)
	return &out
}

// snippetSet collects what the snippets of one build parse to.
type snippetSet struct {
	routes []routemodel.Route
	global []string
}

// add parses one nginx file. A file that does not parse is recorded in global
// (a provider other than nginx then refuses), never dropped.
func (s *snippetSet) add(source, owner, file, text string) {
	sn, err := routemodel.ParseSnippet(source, owner, file, text)
	if err != nil {
		s.global = append(s.global, fmt.Sprintf("unparsed %s %s/%s: %v", source, owner, file, err))
		return
	}
	s.routes = append(s.routes, sn.Routes...)
	s.global = append(s.global, sn.Global...)
}

// plugins parses the snippets injectPluginNginxRoutesVia copies, rendered the
// same way. Read errors are left to the injection step, which reports them.
func (s *snippetSet) plugins(workdir string, cfg *config.Config) {
	pluginDir, err := resolvePluginNginxDir("", cfg)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(pluginDir)
	if err != nil {
		return
	}
	vars := buildTemplateVars(workdir, cfg)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		matches, _ := pluginConfPaths(pluginDir, e.Name())
		for _, match := range matches {
			if data, err := os.ReadFile(match); err == nil {
				s.add("plugin", e.Name(), filepath.Base(match), renderPluginSnippet(string(data), vars))
			}
		}
	}
}

// handManaged parses the hand-managed conf.d and conf.d-<env> files of the
// served nginx dir (default.conf and files this CLI generated are skipped).
func (s *snippetSet) handManaged(workdir string, cfg *config.Config, env string) {
	dir, err := nginxtopo.ServedNginxDir(workdir, cfg.Nginx.FrontedBy)
	if err != nil {
		dir, _ = nginxtopo.ServedNginxDir(workdir, "")
	}
	for _, sub := range []string{"conf.d", "conf.d-" + env} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".conf") || e.Name() == "default.conf" {
				continue
			}
			file := e.Name()
			if sub != "conf.d" {
				file = sub + "/" + file
			}
			data, err := os.ReadFile(filepath.Join(dir, sub, e.Name()))
			switch {
			case err != nil:
				s.global = append(s.global, fmt.Sprintf("unread hand_managed %s: %v", file, err))
			case !strings.Contains(string(data[:min(len(data), 128)]), nginxGeneratedMarker):
				s.add("hand_managed", "", file, string(data))
			}
		}
	}
}
