package build

// Purpose: apply the selected proxy's compose requirements before startup.
// Inputs: validated build state and the compose generator.
// Outputs: provider-specific compose settings and ACME bind directory.
// Constraints: use the sink so plan mode never creates host directories.

import (
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/nself-org/cli/internal/compose"
)

func (st *buildState) applyProxyProvider(g *compose.Generator) error {
	pluginDir := st.pluginDir
	provider, err := ProxyProvider(st.cfg, st.workdir, pluginDir)
	if err != nil {
		return err
	}
	if err := proxyComposeDiscoverable(st.workdir, pluginDir, provider); err != nil {
		return err
	}
	if provider == ProviderNginx && st.planning() && proxyDeclaredTraefik(st.cfg, st.workdir, pluginDir) {
		provider = ProviderTraefik
	}
	if provider != ProviderNginx {
		if err := checkProviderRoutes(st.withSnippetRoutes(st.routes), provider, st.cfg, st.workdir); err != nil {
			return err
		}
		// Create the bind source as the operator before Docker can create it
		// root-owned, which would prevent HTTP-01 challenge token writes.
		if err := st.sink.MkdirAll(filepath.Join(st.workdir, "ssl", ".acme-webroot"), 0755); err != nil {
			return fmt.Errorf("creating ACME webroot: %w", err)
		}
		g.OmitNginx()
		if proxyAPIDocsEnabled(st.cfg) {
			slog.Warn("docs." + st.cfg.BaseDomain + " is not served by traefik (D-0310); api-docs is an accepted difference (API_DOCS_* parsing debt D-0455)")
		}
	} else {
		if err := proxyFrontingStack(st.cfg, st.workdir); err != nil {
			return err
		}
		if err := st.guardDockerSocket(g); err != nil {
			return err
		}
		return cleanupProviderOutput(st.sink, st.workdir, provider)
	}
	return st.guardDockerSocket(g)
}
