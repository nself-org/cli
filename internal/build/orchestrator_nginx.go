package build

import (
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

// modelRoutes is the build preflight and renderer's single route snapshot.
func (st *buildState) modelRoutes(hasSSL bool, trusted func(string) bool) (*routemodel.Model, error) {
	return routemodel.Build(st.cfg, st.workdir, hasSSL, trusted)
}
