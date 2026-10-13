package ssl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/sysexec"
)

// mkcertAvailable returns true if the mkcert binary is on PATH.
func mkcertAvailable() bool {
	_, err := sysexec.LookPath("mkcert")
	return err == nil
}

// generateWithMkcert generates trusted development certificates using mkcert.
// Writes fullchain.pem and privkey.pem into certDir.
// Returns the number of certificate sets generated (1 on success).
func generateWithMkcert(certDir string, domains []string) (int, error) {
	fullchain := filepath.Join(certDir, "fullchain.pem")
	privkey := filepath.Join(certDir, "privkey.pem")

	args := []string{
		"-cert-file", fullchain,
		"-key-file", privkey,
	}
	args = append(args, domains...)

	output, err := sysexec.Output(context.Background(), "mkcert", args...)
	if err != nil {
		return 0, fmt.Errorf("mkcert failed: %w\n%s", err, string(output))
	}

	return 1, nil
}

// generateWithOpenSSL generates self-signed certificates with SAN support using OpenSSL.
// Writes fullchain.pem and privkey.pem into certDir.
// Returns the number of certificate sets generated (1 on success).
func generateWithOpenSSL(certDir string, domains []string) (int, error) {
	// Build OpenSSL config with [alt_names] SAN section.
	confContent := buildOpenSSLConfig(domains)

	// Write temporary config file.
	confPath := filepath.Join(certDir, "openssl.cnf")
	if err := os.WriteFile(confPath, []byte(confContent), 0644); err != nil {
		return 0, fmt.Errorf("writing openssl config: %w", err)
	}
	defer func() { _ = os.Remove(confPath) }()

	fullchain := filepath.Join(certDir, "fullchain.pem")
	privkey := filepath.Join(certDir, "privkey.pem")

	cmd := exec.Command("openssl", "req",
		"-x509",
		"-nodes",
		"-days", "365",
		"-newkey", "rsa:2048",
		"-keyout", privkey,
		"-out", fullchain,
		"-config", confPath,
		"-extensions", "v3_req",
		"-subj", "/CN="+domains[0],
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("openssl failed: %w\n%s", err, string(output))
	}

	return 1, nil
}

// buildOpenSSLConfig creates an OpenSSL configuration string with SAN entries
// for all provided domains.
func buildOpenSSLConfig(domains []string) string {
	var b strings.Builder

	b.WriteString("[req]\n")
	b.WriteString("distinguished_name = req_distinguished_name\n")
	b.WriteString("req_extensions = v3_req\n")
	b.WriteString("prompt = no\n")
	b.WriteString("\n")
	b.WriteString("[req_distinguished_name]\n")
	b.WriteString("CN = " + domains[0] + "\n")
	b.WriteString("\n")
	b.WriteString("[v3_req]\n")
	b.WriteString("keyUsage = digitalSignature, keyEncipherment\n")
	b.WriteString("extendedKeyUsage = serverAuth\n")
	b.WriteString("subjectAltName = @alt_names\n")
	b.WriteString("\n")
	b.WriteString("[alt_names]\n")

	dnsIdx := 1
	ipIdx := 1
	for _, d := range domains {
		if isIPAddress(d) {
			b.WriteString("IP." + strconv.Itoa(ipIdx) + " = " + d + "\n")
			ipIdx++
		} else {
			b.WriteString("DNS." + strconv.Itoa(dnsIdx) + " = " + d + "\n")
			dnsIdx++
		}
	}

	return b.String()
}

// isIPAddress returns true if the string looks like an IPv4 or IPv6 address.
func isIPAddress(s string) bool {
	// IPv6 contains colons.
	if strings.Contains(s, ":") {
		return true
	}
	// IPv4: all parts are numeric.
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}
