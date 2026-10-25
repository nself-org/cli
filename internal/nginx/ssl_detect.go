package nginx

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// ssl_detect.go — decides whether generated nginx site confs should
// terminate TLS, split out of generator.go (CLI-R12 file-size discipline).
//
// Purpose: cli#385 — SSL_MODE only ever controls whether *this build*
// GENERATES new local certificates (internal/ssl/generator.go skips local
// generation entirely for "letsencrypt"/"custom"/"none", since certbot or
// the operator supplies those certs out of band). Before this fix, the
// nginx generator's hasSSL flag was hard-wired to `SSL_MODE == "local"`, so
// a "letsencrypt"/"custom" project whose certs were already issued and
// sitting on disk from an earlier `certbot`/`ssl add` run still generated
// HTTP-only site confs with no `listen 443 ssl` block. Reconciling that
// generated tree onto a serving nginx that already terminates TLS per
// service would have dropped HTTPS stack-wide.
// Inputs: cfg (BaseDomain, SSLMode) and workdir (project root — certs live
// under workdir/ssl/certificates/<dir>/, see nginxtopo.CertDirName).
// Outputs: sslShouldEmit reports whether nginx should emit the 443/
// ssl_certificate block; sslCertsPresent reports raw cert-file presence.
// Constraints: see sslShouldEmit's own doc comment for the per-mode rules.

// sslShouldEmit decides whether generated nginx site confs should include a
// `listen 443 ssl` server block with ssl_certificate directives.
//
// Inputs: cfg, workdir (see package doc above), and the resolved SSL_MODE
// (never empty; NewGenerator defaults it to "local" before calling this).
// Outputs: true when nginx should emit the 443/ssl_certificate block.
// Constraints:
//   - "none" is an explicit opt-out: always false, regardless of any stray
//     cert files that might exist on disk.
//   - "local" is always true: `nself build` regenerates/refreshes local
//     certs on every run (internal/ssl/generator.go), so they are
//     guaranteed present by the time this runs in the real Build() flow.
//   - any other mode ("letsencrypt", "custom", ...) checks the filesystem:
//     these modes never generate certs themselves, so presence on disk is
//     the only trustworthy signal that nginx can safely reference them.
//   - a fronted project (NGINX_FRONTED_BY) is served by the fronting stack's
//     nginx, which mounts its own ssl/ (D-0572): for every mode but "none" the
//     pair must be present there, so neither site confs nor plugin snippets
//     name a certificate the serving nginx cannot load. The project's own
//     ssl/ (written by this build) and assumed plan-mode pairs do not count.
//     Exception: under a shared proxy (NGINX_MODE=shared owner) the build
//     copies the project's pair into the owner's ssl/ itself
//     (internal/build/shared_proxy_certs.go), so the rules below apply.
//   - assumed lists domains whose certificate pair a plan-mode build would
//     create but has not (WithAssumedCerts): a pair for cfg.BaseDomain among
//     them counts as present, so a plan keeps the TLS blocks apply produces.
func sslShouldEmit(cfg *config.Config, workdir, mode string, assumed []string) bool {
	switch mode {
	case "none":
		return false
	}
	if cfg.Nginx.FrontedBy != "" && !sharedProxyFronted(cfg, workdir) {
		return servedCertsPresent(cfg, workdir)
	}
	switch mode {
	case "local":
		return true
	default:
		return sslAssumed(cfg, assumed) || sslCertsPresent(cfg, workdir)
	}
}

// sslAssumed reports whether assumed covers cfg.BaseDomain's certificate.
func sslAssumed(cfg *config.Config, assumed []string) bool {
	for _, d := range assumed {
		if d == cfg.BaseDomain {
			return true
		}
	}
	return false
}

// sslCertsPresent reports whether a usable certificate pair (fullchain.pem +
// privkey.pem) already exists on disk for cfg.BaseDomain, at the same path
// internal/ssl.Generator writes to and RenderServiceRoute reads from.
// sslDirName converts a domain to the directory-safe name nginx's
// ssl_certificate directives are rooted at. Delegates to
// nginxtopo.CertDirNameOrLocalhost — CertDirName is the function
// internal/ssl.Generator uses to decide where on disk it writes certs — so the
// generated nginx path and the actual cert location can never independently
// drift. BaseDomain is never empty by the time a real Build() runs
// (ApplyDefaults fills it), but generator tests and ad-hoc callers may still
// pass "", which falls back to "localhost".
func sslDirName(domain string) string {
	return nginxtopo.CertDirNameOrLocalhost(domain)
}

func sslCertsPresent(cfg *config.Config, workdir string) bool {
	return certPairPresent(filepath.Join(workdir, "ssl"), cfg.BaseDomain)
}

// servedCertsPresent is sslCertsPresent against the ssl dir the fronting
// stack's nginx mounts (nginxtopo.ServedSSLDir). An unconfirmed fronting
// layout reads as absent: the build refuses it anyway (resolveNginxSitesDir).
func servedCertsPresent(cfg *config.Config, workdir string) bool {
	dir, err := nginxtopo.ServedSSLDir(workdir, cfg.Nginx.FrontedBy)
	return err == nil && certPairPresent(dir, cfg.BaseDomain)
}

// sharedProxyFronted reports whether the fronting stack is a shared-proxy
// owner, whose registration makes the build copy this project's pair into the
// owner's ssl/.
func sharedProxyFronted(cfg *config.Config, workdir string) bool {
	root, err := nginxtopo.ServedRoot(workdir, cfg.Nginx.FrontedBy)
	if err != nil {
		return false
	}
	return nginxtopo.SharedOwnerRegistered(root)
}

// certPairPresent reports whether fullchain.pem and privkey.pem exist under
// <sslDir>/certificates/<dir of domain>.
func certPairPresent(sslDir, domain string) bool {
	certDir := filepath.Join(sslDir, "certificates", sslDirName(domain))
	if _, err := os.Stat(filepath.Join(certDir, "fullchain.pem")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(certDir, "privkey.pem")); err != nil {
		return false
	}
	return true
}
