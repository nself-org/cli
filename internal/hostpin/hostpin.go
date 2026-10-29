// Package hostpin holds the deploy pin file location and the ssh-keygen pin
// lookup shared by deploy callers. It lives at L0 (stdlib only, the sdk
// fingerprint parser is injected). The pin alias itself is remote.PinAlias, the
// one scheme deploy, admin connect and access share (D-0463).
package hostpin

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// KnownHostsPath is the dedicated deploy pin file. The operator's SSH file is
// read only; this package never writes it.
func KnownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nself", "deploy_known_hosts")
}

// HostPort returns the effective SSH port for spec (22 when unset).
func HostPort(port int) int {
	if port == 0 {
		return 22
	}
	return port
}

// TrustedFingerprints returns the fingerprints ssh-keygen reports for alias in
// path, each line parsed by fp (the sdk remote.Fingerprint; injected so this
// package stays at L0). Errors are swallowed: the caller treats an empty set as
// "no pin".
func TrustedFingerprints(ctx context.Context, alias, path string, fp func(string) (string, error)) map[string]bool {
	seen := map[string]bool{}
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-F", alias, "-f", path).Output()
	if err != nil {
		return seen
	}
	for _, line := range strings.Split(string(out), "\n") {
		if f, err := fp(line); err == nil {
			seen[f] = true
		}
	}
	return seen
}
