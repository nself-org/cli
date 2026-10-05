package ssl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// Generator creates SSL certificates for all domains in the project configuration.
type Generator struct {
	cfg *config.Config
	// explicitHosts records an operator opt-in (--hosts) to /etc/hosts
	// management for a domain shape shouldManageHosts's default heuristic
	// does not recognize as local-dev. Never overrides the cfg.Env=="prod"
	// veto — see hosts_gate.go. Set via WithExplicitHosts; false by default
	// (unchanged behavior for every existing caller).
	explicitHosts bool
}

// NewGenerator creates an SSL Generator from the given config.
func NewGenerator(cfg *config.Config) *Generator {
	return &Generator{cfg: cfg}
}

// WithExplicitHosts marks that the operator explicitly requested /etc/hosts
// management for this build (the --hosts flag), overriding
// shouldManageHosts's local-dev-domain heuristic for an unrecognized domain
// shape. It never overrides the cfg.Env=="prod" veto. Returns g for
// chaining, matching the WithWorkDir convention used elsewhere in this CLI
// (e.g. compose.NewGeneratorWithProfile(...).WithWorkDir(...)).
func (g *Generator) WithExplicitHosts(v bool) *Generator {
	g.explicitHosts = v
	return g
}

// GenerateResult holds the output of a Generate call including trust/hosts status.
type GenerateResult struct {
	// Count is the number of certificate sets generated.
	Count int
	// CAInstalled is true when the mkcert CA was already trusted or was
	// successfully installed during this call.
	CAInstalled bool
	// CAManualCmd is non-empty when the CA could not be installed automatically.
	// It holds the command the user should run manually.
	CAManualCmd string
	// HostsAdded is the number of new /etc/hosts entries written.
	HostsAdded int
	// HostsManualNote is non-empty when the hosts file could not be written.
	HostsManualNote string
}

// Generate creates SSL certificates for all collected domains.
// Certificates are written to outputDir/certificates/{dir}/fullchain.pem and privkey.pem.
// Returns the count of certificate sets generated.
func (g *Generator) Generate(outputDir string) (int, error) {
	result, err := g.GenerateWithResult(outputDir)
	if err != nil {
		return 0, err
	}
	return result.Count, nil
}

// NeedsCerts reports whether sslMode makes nself generate local certificates.
// letsencrypt (certbot after first start), custom (user-supplied files) and
// none (HTTP only) skip generation. ssl.Generator and the build plan mode
// (internal/build planSSL) both call it so the skip rule lives once.
func NeedsCerts(sslMode string) bool {
	switch sslMode {
	case "letsencrypt", "custom", "none":
		return false
	}
	return true
}

// minCertDays is the remaining validity at or above which an existing
// certificate pair is reused instead of regenerated.
const minCertDays = 30

// CertPairValid reports whether dir holds fullchain.pem and privkey.pem as
// regular files and the chain has at least minCertDays days left. Generation is
// skipped for such a pair; the build plan mode uses the same test to decide
// whether it would generate one.
func CertPairValid(dir string) bool {
	full := filepath.Join(dir, "fullchain.pem")
	if !fileExists(full) || !fileExists(filepath.Join(dir, "privkey.pem")) {
		return false
	}
	days, err := CheckCertExpiry(full)
	return err == nil && days >= minCertDays
}

// GenerateWithResult creates SSL certificates and returns detailed results
// including the CA trust and /etc/hosts steps.
//
// When SSL_MODE is "letsencrypt", "custom", or "none", local certificate
// generation is skipped. In letsencrypt mode, certbot provisions the real
// certs after the stack starts. In custom mode, the user supplies their own
// certs. In none mode, SSL is disabled entirely. Nginx must be configured to
// start without SSL cert references in these modes — see nginx/generator.go.
func (g *Generator) GenerateWithResult(outputDir string) (*GenerateResult, error) {
	// Skip local certificate generation for non-local SSL modes.
	// letsencrypt: certbot provisions certs after first start via ACME.
	// custom:      user provides their own certificate files.
	// none:        SSL is disabled; nginx runs HTTP-only.
	if !NeedsCerts(g.cfg.SSLMode) {
		return &GenerateResult{Count: 0}, nil
	}

	domains := g.collectDomains()
	if len(domains) == 0 {
		return nil, fmt.Errorf("no domains collected for SSL generation")
	}

	// Build the certificate directory name from BASE_DOMAIN.
	// Dots become dashes: nself.org -> nself-org, localhost stays localhost.
	certDirName := DomainToDirName(g.cfg.BaseDomain)
	if certDirName == "" {
		certDirName = "localhost"
	}

	certDir := filepath.Join(outputDir, "certificates", certDirName)
	// 0755, not 0750: this directory is bind-mounted into the nginx container,
	// which cannot traverse it otherwise. It holds a public certificate chain,
	// so the traversal bit is not protecting anything. privkey.pem inside keeps
	// the restrictive mode mkcert/openssl give it.
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return nil, fmt.Errorf("creating certificate directory %s: %w", certDir, err)
	}

	result := &GenerateResult{}

	// Check for pre-existing mkcert certs from `nself trust`.
	// If ssl/fullchain.pem + ssl/privkey.pem exist at the top level of outputDir
	// (put there by `nself trust`), use them instead of generating new certs.
	topFullchain := filepath.Join(outputDir, "fullchain.pem")
	topPrivkey := filepath.Join(outputDir, "privkey.pem")
	if CertPairValid(outputDir) {
		if copyErr := copyMkcertCerts(topFullchain, topPrivkey, certDir); copyErr == nil {
			result.Count = 1
			result.CAInstalled = true // mkcert certs are inherently trusted
			g.applyTrustAndHosts(domains, result)
			return result, nil
		}
		// Copy failed — fall through to normal generation.
	}

	// Skip generation if certs already exist, are valid, and have >30 days remaining.
	// Expired, near-expiry, or unreadable pairs are regenerated.
	if CertPairValid(certDir) {
		result.Count = 1
		// Still attempt CA trust and hosts even when certs are fresh.
		g.applyTrustAndHosts(domains, result)
		return result, nil
	}

	// Try mkcert first for trusted local development certificates.
	var certCount int
	var mkcertUnavailable bool
	if mkcertAvailable() {
		count, err := generateWithMkcert(certDir, domains)
		if err == nil {
			certCount = count
		}
		// mkcert failed — fall through to OpenSSL.
	} else {
		mkcertUnavailable = true
	}

	if certCount == 0 {
		// Fallback to OpenSSL self-signed certificates.
		// Record the mkcert absence so callers can inspect via errors.Is.
		count, err := generateWithOpenSSL(certDir, domains)
		if err != nil {
			if mkcertUnavailable {
				return nil, fmt.Errorf("%w; OpenSSL fallback also failed: %v", errs.ErrMkcertNotFound, err)
			}
			return nil, fmt.Errorf("%w: %v", errs.ErrSSLGenerationFailed, err)
		}
		if mkcertUnavailable {
			// Succeeded with OpenSSL fallback — record the mkcert absence
			// as informational context without failing.
			result.CAManualCmd = errs.ErrMkcertNotFound.Error()
		}
		certCount = count
	}

	result.Count = certCount

	// Attempt CA trust installation and /etc/hosts management.
	// These are best-effort: failures produce instructions, not errors.
	g.applyTrustAndHosts(domains, result)

	return result, nil
}
