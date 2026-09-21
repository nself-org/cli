package ssl

// Purpose: mkcert CA-trust/hosts-file application, small path/name helpers, certificate expiry checking, and mkcert output copying, backing GenerateWithResult in generator.go.
// Inputs: generated domain lists, certificate/key file paths, and a GenerateResult being populated.
// Outputs: applied trust store + /etc/hosts entries (with manual-fallback notes), an expiry day count, or copied certificate files.
// Constraints: split out of generator.go as a pure move (CLI-R12); no behavior change.

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// applyTrustAndHosts performs the mkcert CA trust and /etc/hosts steps.
// Results are written into result. Errors are never returned — only
// manual instructions are set.
func (g *Generator) applyTrustAndHosts(domains []string, result *GenerateResult) {
	// ── CA trust ────────────────────────────────────────────────────
	if mkcertAvailable() {
		caPath, err := MkcertCACertPath()
		if err == nil {
			installed, err := IsCAInstalled(caPath)
			if err == nil && installed {
				result.CAInstalled = true
			} else {
				// Not installed — attempt installation.
				installErr := installCA(caPath)
				if installErr == nil {
					result.CAInstalled = true
				} else if errors.Is(installErr, ErrSudoRequired) {
					result.CAManualCmd = ManualInstallCommand(caPath)
				}
				// Other errors: set manual cmd as well.
				if installErr != nil && result.CAManualCmd == "" {
					result.CAManualCmd = ManualInstallCommand(caPath)
				}
			}
		}
	}

	// ── /etc/hosts ──────────────────────────────────────────────────
	// Gate first, before anything else touches the filesystem: a
	// production box was found with 14 *.local.nself.org entries appended
	// to /etc/hosts by a bare `nself build` run. shouldManageHosts vetoes
	// ENV=prod unconditionally and otherwise only allows a recognized
	// local-dev domain shape (or an explicit --hosts opt-in) — see
	// hosts_gate.go.
	if !shouldManageHosts(g.cfg.Env, g.cfg.BaseDomain, g.explicitHosts) {
		slog.Debug("skipping /etc/hosts management — not a local-dev build",
			"env", g.cfg.Env, "base_domain", g.cfg.BaseDomain)
		return
	}

	filtered := filterHostsEntries(domains)
	if len(filtered) == 0 {
		return
	}

	// Permission pre-flight: skip with a warning rather than attempting the
	// read/write dance and only discovering it can't be written partway
	// through.
	if !canWriteHostsFile(hostsFile) {
		slog.Warn("/etc/hosts is not writable — skipping automatic update",
			"file", hostsFile, "fix", "sudo nself dns-setup")
		result.HostsManualNote = hostsManualNote(filtered)
		return
	}

	// Count how many are not yet in the file before adding.
	existing, err := readHostsFile(hostsFile)
	if err != nil {
		// Can't read — record note and skip.
		result.HostsManualNote = hostsManualNote(filtered)
		return
	}
	present := buildPresentSet(existing)
	var newEntries []string
	for _, h := range filtered {
		if !present[h] {
			newEntries = append(newEntries, h)
		}
	}

	if len(newEntries) == 0 {
		result.HostsAdded = 0
		return
	}

	// Print what will be written before writing it.
	slog.Info("adding /etc/hosts entries", "file", hostsFile, "count", len(newEntries), "hosts", newEntries)

	addErr := addHosts(newEntries)
	if addErr == nil {
		result.HostsAdded = len(newEntries)
	} else {
		result.HostsManualNote = hostsManualNote(newEntries)
	}
}

// hostsManualNote builds a short note about what hosts entries need to be added manually.
func hostsManualNote(hostnames []string) string {
	var sb strings.Builder
	sb.WriteString("Run once to set up DNS:\n")
	sb.WriteString("  sudo nself dns-setup\n\n")
	sb.WriteString("Or add manually to /etc/hosts:\n")
	for _, h := range hostnames {
		fmt.Fprintf(&sb, "  127.0.0.1\t%s\n", h)
	}
	return sb.String()
}

// DomainToDirName converts a domain to a directory-safe name by replacing dots with dashes.
// Exported so callers outside this package (cmd/commands' ssl_add.go) can assert
// their own domain-to-directory conversion agrees with this one instead of
// duplicating the literal replacement rule, which could silently diverge.
func DomainToDirName(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}

// fileExists returns true if the path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// CheckCertExpiry reads the PEM-encoded x509 certificate at certPath, computes the
// number of days until it expires, and returns that count.
// If the certificate is already expired, it returns 0 and an error.
// If fewer than 30 days remain, a WARN is written to stderr and the remaining days
// are returned with a nil error.
func CheckCertExpiry(certPath string) (int, error) {
	data, err := os.ReadFile(certPath)
	if err != nil {
		return 0, fmt.Errorf("reading certificate %s: %w", certPath, err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return 0, fmt.Errorf("no PEM block found in %s", certPath)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0, fmt.Errorf("parsing certificate %s: %w", certPath, err)
	}

	now := time.Now()
	remaining := cert.NotAfter.Sub(now)

	// Convert to whole days using integer arithmetic (truncate toward zero).
	daysRemaining := int(remaining.Hours()) / 24

	if now.After(cert.NotAfter) {
		expired := int(-remaining.Hours()) / 24
		return 0, fmt.Errorf("certificate expired %d days ago", expired)
	}

	if daysRemaining < 30 {
		fmt.Fprintf(os.Stderr, "WARN: certificate at %s expires in %d days\n", certPath, daysRemaining)
	}

	return daysRemaining, nil
}

// copyMkcertCerts copies a fullchain.pem and privkey.pem pair into destDir.
// destDir is created if it does not exist. Both files are written with 0640
// permissions so nginx can read them without world-readable exposure.
func copyMkcertCerts(srcFullchain, srcPrivkey, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating certificate directory %s: %w", destDir, err)
	}

	chainData, err := os.ReadFile(srcFullchain)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcFullchain, err)
	}
	// 0644: fullchain.pem is the public certificate chain, not a secret. It is
	// served to every client that connects, so restricting reads buys nothing
	// and breaks any consumer running under a different uid (nginx in its
	// container being the one that matters here). privkey.pem below stays
	// restricted, which is the file that actually needs it.
	if err := os.WriteFile(filepath.Join(destDir, "fullchain.pem"), chainData, 0644); err != nil {
		return fmt.Errorf("writing fullchain.pem to %s: %w", destDir, err)
	}

	keyData, err := os.ReadFile(srcPrivkey)
	if err != nil {
		return fmt.Errorf("reading %s: %w", srcPrivkey, err)
	}
	if err := os.WriteFile(filepath.Join(destDir, "privkey.pem"), keyData, 0640); err != nil {
		return fmt.Errorf("writing privkey.pem to %s: %w", destDir, err)
	}

	return nil
}
