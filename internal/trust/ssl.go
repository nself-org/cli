package trust

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/sysexec"
)

const mkcertInstallTimeout = 5 * time.Minute
const mkcertCertTimeout = 60 * time.Second

// CheckMkcert checks the current state of mkcert, its CA, and project certs.
// Returns (installed, caInstalled, certsValid).
func CheckMkcert(cfg TrustConfig) (installed bool, caInstalled bool, certsValid bool) {
	_, err := sysexec.LookPath("mkcert")
	installed = err == nil

	if installed {
		caPath, pathErr := ssl.MkcertCACertPath()
		if pathErr == nil {
			trusted, _ := ssl.IsCAInstalled(caPath)
			caInstalled = trusted
		}
	}

	if cfg.WorkDir != "" {
		fullchain := filepath.Join(cfg.WorkDir, "ssl", "fullchain.pem")
		privkey := filepath.Join(cfg.WorkDir, "ssl", "privkey.pem")
		if fileExists(fullchain) && fileExists(privkey) {
			days, expErr := ssl.CheckCertExpiry(fullchain)
			certsValid = expErr == nil && days >= 30
		}
	}

	return installed, caInstalled, certsValid
}

// MkcertCAPath returns the path to the mkcert root CA certificate.
// Returns an error if mkcert is not installed or CAROOT is unavailable.
func MkcertCAPath() (string, error) {
	return ssl.MkcertCACertPath()
}

// SetupMkcert ensures mkcert is installed, the CA is trusted, and wildcard
// certificates are generated for the project's base domain. Returns
// alreadyDone=true when both the CA is trusted AND the certs exist and are
// valid (>30 days remaining) — i.e. nothing was done.
//
// Idempotency: each sub-step (CA trust, cert generation) is checked
// independently before any admin-privilege operation is attempted.
func SetupMkcert(cfg TrustConfig) (alreadyDone bool, err error) {
	mkcertInstalled, caInstalled, certsValid := CheckMkcert(cfg)

	// If both CA is trusted and certs are valid, nothing to do.
	if caInstalled && certsValid {
		return true, nil
	}

	// Headless: state was checked above (read-only). Installing mkcert and
	// trusting its CA are system changes behind an admin prompt, so skip.
	// Generating certs for an already-trusted CA writes only into the project.
	if isHeadless() && (!mkcertInstalled || !caInstalled) {
		return false, fmt.Errorf("ssl setup: %w", ErrSkippedHeadless)
	}

	// Ensure mkcert is installed.
	if _, lookErr := sysexec.LookPath("mkcert"); lookErr != nil {
		if err = installMkcert(); err != nil {
			return false, fmt.Errorf("installing mkcert: %w", err)
		}
	}

	// Ensure CA is trusted — only if it isn't already.
	caPath, pathErr := ssl.MkcertCACertPath()
	if pathErr != nil {
		return false, fmt.Errorf("locating mkcert CA: %w", pathErr)
	}

	if !caInstalled {
		if err = installMkcertCA(caPath); err != nil {
			return false, fmt.Errorf("trusting mkcert CA: %w", err)
		}
	}

	// Generate certificates only if they are missing or expiring.
	if !certsValid {
		if err = generateCerts(cfg); err != nil {
			return false, fmt.Errorf("generating certificates: %w", err)
		}
	}

	return false, nil
}

// installMkcert installs mkcert via Homebrew on macOS or returns an error
// with manual installation instructions for Linux.
func installMkcert() error {
	switch runtime.GOOS {
	case "darwin":
		ctx, cancel := context.WithTimeout(context.Background(), mkcertInstallTimeout)
		defer cancel()

		if err := sysexec.Stream(ctx, "brew", "install", "mkcert"); err != nil {
			return fmt.Errorf("brew install mkcert: %w", err)
		}
		return nil

	case "linux":
		return fmt.Errorf(
			"mkcert not found — install it manually:\n" +
				"  Debian/Ubuntu: sudo apt install libnss3-tools && " +
				"curl -sL https://github.com/FiloSottile/mkcert/releases/latest/download/mkcert-$(uname -m)-linux-amd64 " +
				"-o /usr/local/bin/mkcert && chmod +x /usr/local/bin/mkcert\n" +
				"  Arch: sudo pacman -S mkcert\n" +
				"  Fedora: sudo dnf install mkcert",
		)

	default:
		return fmt.Errorf("mkcert not found — install it from https://github.com/FiloSottile/mkcert")
	}
}

// installMkcertCA trusts the mkcert CA in the OS keychain. On macOS this
// uses osascript for admin privileges. On Linux it returns an error with
// manual instructions.
func installMkcertCA(caPath string) error {
	switch runtime.GOOS {
	case "darwin":
		script := fmt.Sprintf(
			`do shell script "security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s" with administrator privileges`,
			appleScriptQuotedArg(caPath),
		)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		out, err := sysexec.Output(ctx, "osascript", "-e", script)
		if err != nil {
			return fmt.Errorf("trusting mkcert CA in keychain: %w — %s", err, strings.TrimSpace(string(out)))
		}
		return nil

	case "linux":
		return fmt.Errorf(
			"mkcert CA not trusted — run manually:\n"+
				"  mkcert -install\n"+
				"  (requires libnss3-tools on Debian/Ubuntu: sudo apt install libnss3-tools)\n"+
				"  CA path: %s",
			caPath,
		)

	default:
		return fmt.Errorf("mkcert CA not trusted — run: mkcert -install")
	}
}

// appleScriptQuotedArg single-quotes s for the shell and escapes the result
// for embedding inside an AppleScript double-quoted string, so a CA path with
// spaces (the default macOS CAROOT is ~/Library/Application Support/mkcert) or
// shell metacharacters stays one literal argument in the admin shell.
func appleScriptQuotedArg(s string) string {
	q := "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	q = strings.ReplaceAll(q, `\`, `\\`)
	return strings.ReplaceAll(q, `"`, `\"`)
}

// buildDomainList constructs the list of domains for certificate generation.
func buildDomainList(cfg TrustConfig) []string {
	domains := []string{}

	if cfg.BaseDomain != "" {
		domains = append(domains,
			cfg.BaseDomain,
			"*."+cfg.BaseDomain,
		)
		// mkcert rejects double-wildcard SANs (*.*.domain). Generate per-namespace
		// wildcards instead — one entry per namespace prefix extracted from ROUTES.
		for _, prefix := range cfg.NamespacePrefixes {
			domains = append(domains, "*."+prefix+"."+cfg.BaseDomain)
		}
	}

	domains = append(domains, "127.0.0.1")
	domains = append(domains, cfg.ExtraSSLDomains...)

	return domains
}

// generateCerts generates wildcard certificates using mkcert into {workdir}/ssl/.
func generateCerts(cfg TrustConfig) error {
	sslDir := filepath.Join(cfg.WorkDir, "ssl")
	if err := os.MkdirAll(sslDir, 0750); err != nil {
		return fmt.Errorf("creating ssl directory %s: %w", sslDir, err)
	}

	fullchain := filepath.Join(sslDir, "fullchain.pem")
	privkey := filepath.Join(sslDir, "privkey.pem")

	domains := buildDomainList(cfg)
	if len(domains) == 0 {
		return fmt.Errorf("no domains configured for certificate generation")
	}

	args := []string{
		"-cert-file", fullchain,
		"-key-file", privkey,
	}
	args = append(args, domains...)

	ctx, cancel := context.WithTimeout(context.Background(), mkcertCertTimeout)
	defer cancel()

	if err := sysexec.Stream(ctx, "mkcert", args...); err != nil {
		return fmt.Errorf("mkcert certificate generation failed: %w", err)
	}

	return nil
}

// fileExists returns true if path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
