package nginx

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/ssl"
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
// under workdir/ssl/certificates/<dir>/, see internal/ssl.DomainToDirName).
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
func sslShouldEmit(cfg *config.Config, workdir, mode string) bool {
	switch mode {
	case "none":
		return false
	case "local":
		return true
	default:
		return sslCertsPresent(cfg, workdir)
	}
}

// sslCertsPresent reports whether a usable certificate pair (fullchain.pem +
// privkey.pem) already exists on disk for cfg.BaseDomain, at the same path
// internal/ssl.Generator writes to and RenderServiceRoute/hasTrustedChain
// read from.
// sslDirName converts a domain to the directory-safe name nginx's
// ssl_certificate directives are rooted at. Delegates to
// internal/ssl.DomainToDirName — the same function internal/ssl.Generator
// uses to decide where on disk it writes certs — so the generated nginx
// path and the actual cert location can never independently drift (they
// used to be two hand-typed copies of the same replace-dots-with-dashes
// rule, one per package, kept in sync only by a comment cross-reference).
// The empty-domain fallback stays local to this package: BaseDomain is
// never empty by the time a real Build() runs (ApplyDefaults fills it),
// but generator tests and ad-hoc callers may still pass "".
func sslDirName(domain string) string {
	if domain == "" {
		return "localhost"
	}
	return ssl.DomainToDirName(domain)
}

func sslCertsPresent(cfg *config.Config, workdir string) bool {
	certDir := filepath.Join(workdir, "ssl", "certificates", sslDirName(cfg.BaseDomain))
	if _, err := os.Stat(filepath.Join(certDir, "fullchain.pem")); err != nil {
		return false
	}
	if _, err := os.Stat(filepath.Join(certDir, "privkey.pem")); err != nil {
		return false
	}
	return true
}
