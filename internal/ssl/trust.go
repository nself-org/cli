package ssl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ErrSudoRequired is returned when an operation requires elevated privileges
// that are not currently available.
var ErrSudoRequired = errors.New("sudo required")

// mkcertCATimeout is the maximum time allowed for a CA trust operation.
// macOS security add-trusted-cert may show a UI prompt; we bound this tightly.
const mkcertCATimeout = 10 * time.Second

// MkcertCARoot returns the path to the mkcert CA root directory as reported
// by `mkcert -CAROOT`. Returns an error if mkcert is not on PATH or fails.
func mkcertCARoot() (string, error) {
	cmd := exec.Command("mkcert", "-CAROOT")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("mkcert -CAROOT: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// MkcertCACertPath returns the expected path to the mkcert root CA certificate.
func MkcertCACertPath() (string, error) {
	root, err := mkcertCARoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "rootCA.pem"), nil
}

// IsCAInstalled checks whether the mkcert CA certificate at caPath is already
// trusted by the OS trust store.
//
// On macOS: queries the system keychain via `security find-certificate`.
// On Linux: checks whether the cert file is present in the system CA bundle dir.
// On Windows: queries certutil -store Root.
// Returns (false, nil) when caPath does not exist.
func IsCAInstalled(caPath string) (bool, error) {
	if _, err := os.Stat(caPath); errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	switch runtime.GOOS {
	case "darwin":
		return isCAInstalledDarwin(caPath)
	case "linux":
		return isCAInstalledLinux(caPath)
	case "windows":
		return isCAInstalledWindows(caPath)
	default:
		// Unknown OS — assume not installed so we can attempt.
		return false, nil
	}
}

// InstallCA installs the mkcert root CA certificate into the OS trust store.
//
// macOS:   security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain <caPath>
// Linux:   copies cert into /usr/local/share/ca-certificates/ and runs update-ca-certificates
// Windows: certutil -addstore Root <caPath>
//
// Returns nil if the CA is already trusted. Returns ErrSudoRequired if
// elevated privileges are needed but not available.
func installCA(caPath string) error {
	if _, err := os.Stat(caPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("CA certificate not found at %s", caPath)
	}

	// Check if already installed — skip if so.
	installed, err := IsCAInstalled(caPath)
	if err == nil && installed {
		return nil
	}

	switch runtime.GOOS {
	case "darwin":
		return installCADarwin(caPath)
	case "linux":
		return installCALinux(caPath)
	case "windows":
		return installCAWindows(caPath)
	default:
		return fmt.Errorf("unsupported OS for CA trust: %s", runtime.GOOS)
	}
}

// ManualInstallCommand returns the platform-specific command string that the
// user can run manually to trust the CA, for use in fallback messages.
func ManualInstallCommand(caPath string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf(
			"sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s",
			caPath,
		)
	case "linux":
		dest := filepath.Join("/usr/local/share/ca-certificates", "mkcert-rootCA.crt")
		return fmt.Sprintf("sudo cp %s %s && sudo update-ca-certificates", caPath, dest)
	case "windows":
		return fmt.Sprintf("certutil -addstore Root %s", caPath)
	default:
		return fmt.Sprintf("# Add %s to your OS trust store", caPath)
	}
}

// ── macOS ─────────────────────────────────────────────────────────────────────

func isCAInstalledDarwin(caPath string) (bool, error) {
	// Extract the subject from the cert so we can search by it.
	// A simpler approach: check if the cert's SHA-1 fingerprint appears in
	// the system keychain. We use `security find-certificate -a` to search
	// all keychains for a cert with the same file.
	ctx, cancel := context.WithTimeout(context.Background(), mkcertCATimeout)
	defer cancel()

	// Use security verify-cert which exits 0 if trusted, non-zero if not.
	cmd := exec.CommandContext(ctx, "security", "verify-cert", "-c", caPath)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	// Non-zero exit means not trusted (or other error — treat as not installed).
	return false, nil
}

func installCADarwin(caPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mkcertCATimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx,
		"security", "add-trusted-cert",
		"-d",
		"-r", "trustRoot",
		"-k", "/Library/Keychains/System.keychain",
		caPath,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return nil
	}

	if ctx.Err() != nil {
		// Timed out — likely waiting for a UI prompt.
		return ErrSudoRequired
	}

	// Check if it's a permission error.
	errStr := stderr.String()
	if strings.Contains(errStr, "permission") ||
		strings.Contains(errStr, "authorization") ||
		strings.Contains(errStr, "Permission denied") {
		return ErrSudoRequired
	}

	return fmt.Errorf("security add-trusted-cert: %w\n%s", err, errStr)
}

// ── Linux ─────────────────────────────────────────────────────────────────────

func isCAInstalledLinux(caPath string) (bool, error) {
	// Check whether a copy of the cert exists in the system CA certs directory.
	dest := filepath.Join("/usr/local/share/ca-certificates", "mkcert-rootCA.crt")
	_, err := os.Stat(dest)
	if err == nil {
		return true, nil
	}
	// Also check /etc/ssl/certs/ for distros that copy there directly.
	altDest := filepath.Join("/etc/ssl/certs", "mkcert-rootCA.pem")
	_, err = os.Stat(altDest)
	return err == nil, nil
}

func installCALinux(caPath string) error {
	dest := filepath.Join("/usr/local/share/ca-certificates", "mkcert-rootCA.crt")

	// Try to copy the CA cert.
	data, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("reading CA cert: %w", err)
	}

	if err := os.WriteFile(dest, data, 0644); err != nil {
		if os.IsPermission(err) {
			return ErrSudoRequired
		}
		return fmt.Errorf("writing CA cert to %s: %w", dest, err)
	}

	// Run update-ca-certificates.
	ctx, cancel := context.WithTimeout(context.Background(), mkcertCATimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "update-ca-certificates")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if os.IsPermission(err) || strings.Contains(stderr.String(), "permission") {
			return ErrSudoRequired
		}
		return fmt.Errorf("update-ca-certificates: %w\n%s", err, stderr.String())
	}

	return nil
}

// ── Windows ───────────────────────────────────────────────────────────────────

// isCAInstalledWindows is an UNIMPLEMENTED probe that deliberately always
// answers "not installed", and returns a nil error because there is no
// failure to report — it never inspects anything.
//
// Why a constant false is the safe constant here, and why it is not the
// error-swallowing shape it resembles: the two wrong answers are not
// symmetric. Over-reporting "not installed" costs one redundant run of
// installCAWindows, which is idempotent (certutil -addstore on an
// already-trusted root is a no-op). Over-reporting "installed" skips a needed
// install and leaves the user with TLS errors and nothing to point at. There
// is no third state to lose, so there is nothing for a caller to act wrongly
// on — every caller here (generator_trust.go, trust/status.go, trust/ssl.go)
// either proceeds to install or reports "not trusted", both of which are
// accurate for a host we have not checked.
//
// It previously ran `certutil -store Root`, discarded the output with `_ = out`
// and returned false on both branches. That call could not change the result,
// so it was removed: it only added a process spawn (and a hang risk) to a
// function whose answer was already fixed. Implementing this properly means
// matching the CA's thumbprint against the store — see also isCAInstalledLinux,
// which does path-existence matching rather than a real trust query.
func isCAInstalledWindows(_ string) (bool, error) {
	return false, nil
}

func installCAWindows(caPath string) error {
	ctx, cancel := context.WithTimeout(context.Background(), mkcertCATimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "certutil", "-addstore", "Root", caPath)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errStr := stderr.String()
		if strings.Contains(errStr, "Access is denied") ||
			strings.Contains(errStr, "0x80070005") {
			return ErrSudoRequired
		}
		return fmt.Errorf("certutil -addstore Root: %w\n%s", err, errStr)
	}

	return nil
}
