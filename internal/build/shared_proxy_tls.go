package build

import "github.com/nself-org/cli/internal/nginx"

// Purpose: retain TLS for pairs the shared-proxy pipeline copies to the owner.
// Inputs: the served TLS decision and the detected shared role.
// Outputs: whether this build can serve TLS after sharedFinish succeeds.
// Constraints: only registered shared frontends may rely on project-local
// pairs; sharedBegin checks their targets and sharedFinish copies them before
// a successful build. Ordinary fronted projects require the served pair.
func (st *buildState) sharedTLS(served bool) bool {
	if served || !st.shared.active() {
		return served
	}
	cfg := *st.cfg
	cfg.Nginx.FrontedBy = ""
	return nginx.NewGenerator(&cfg, st.workdir).HasSSL()
}
