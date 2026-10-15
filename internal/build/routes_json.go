package build

import (
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// writeRoutesJSON writes the deterministic proxy contract through the build sink.
func (st *buildState) writeRoutesJSON(m *routemodel.Model) error {
	out := st.withSnippetRoutes(m)
	// api-docs.conf is written by the build finish step, not by Build, so its
	// route is added here (withSnippetRoutes already returned a fresh slice).
	docs, err := routemodel.APIDocs(st.cfg)
	if err != nil {
		return fmt.Errorf("modelling api-docs route: %w", err)
	}
	if docs != nil {
		docs.File = filepath.ToSlash(apiDocsSitePath("nginx/sites", st.cfg.Nginx.FrontedBy, st.cfg.ProjectName))
		out.Routes = append(out.Routes, *docs)
	}
	b, err := routemodel.Marshal(out)
	if err != nil {
		return fmt.Errorf("encoding routes.json: %w", err)
	}
	dir := filepath.Join(st.workdir, ".nself", "generated")
	if err := st.sink.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating routes.json directory: %w", err)
	}
	if err := st.sink.WriteFile(filepath.Join(dir, "routes.json"), b, 0644); err != nil {
		return fmt.Errorf("writing routes.json: %w", err)
	}
	st.filesGenerated++
	return nil
}
