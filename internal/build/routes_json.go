package build

import (
	"fmt"
	"path/filepath"

	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// writeRoutesJSON writes the deterministic proxy contract through the build sink.
func (st *buildState) writeRoutesJSON(m *routemodel.Model) error {
	b, err := routemodel.Marshal(m)
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
