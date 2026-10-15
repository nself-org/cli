package build

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/nginx"
	"github.com/nself-org/cli/internal/nginxtopo"
	"github.com/nself-org/cli/internal/sharedproxy"
)

// Purpose: share the build's TLS decision across preflight, plan and rendering.
// Inputs: config, project root, and certificate domains planned for generation.
// Outputs: a generator accounting for certificates the shared build will copy.
// Constraints: only a confirmed shared owner copies backend-local pairs;
// sharedBegin validates every target before writes, and sharedFinish installs
// the pairs before a successful build returns. Legacy fronted projects must
// already hold their pair in the served tree.
func (st *buildState) nginxGenerator(assumed ...string) *nginx.Generator {
	gen := nginx.NewGenerator(st.cfg, st.workdir)
	root, err := nginxtopo.ServedRoot(st.workdir, st.cfg.Nginx.FrontedBy)
	if st.cfg.Nginx.FrontedBy != "" && err == nil {
		if _, err := os.Stat(filepath.Join(sharedproxy.DirOf(root), sharedproxy.OwnerFile)); err == nil {
			src := filepath.Join(st.workdir, "ssl", pluginSSLDir(st.cfg.BaseDomain))
			_, certErr := os.Stat(filepath.Join(src, "fullchain.pem"))
			_, keyErr := os.Stat(filepath.Join(src, "privkey.pem"))
			if certErr == nil && keyErr == nil {
				assumed = append(append([]string{}, assumed...), st.cfg.BaseDomain)
			}
		}
	}
	return gen.WithAssumedCerts(assumed)
}
