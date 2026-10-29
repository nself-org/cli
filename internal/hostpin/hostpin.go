// Package hostpin provides SSH host-key pin alias resolution for callers that
// do their own pin lookup (admin connect, access). It lives at L0 (stdlib
// only, the sdk fingerprint parser is injected) so L1 packages can resolve a pin
// alias without importing internal/deploy (L2).
package hostpin

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

// KeyAlias is the pin alias for host and port. A port or an IP literal needs a
// synthetic alias: the sdk refuses an IP literal as a pin alias. '+' is
// allowed in pin aliases but forbidden in host names, keeping the namespaces
// disjoint.
func KeyAlias(host string, port int) string {
	if port != 0 || net.ParseIP(host) != nil {
		return "nself+" + host + "+" + strconv.Itoa(HostPort(port))
	}
	return host
}

// LegacyKeyAlias is the v1.4.13 spelling of the IP/port pin alias. It can
// equal a real host name, so it is only ever read, and only when no pin exists
// under the new alias (D-0696).
func LegacyKeyAlias(host string, port int) string {
	return "nself-" + strings.ReplaceAll(host, ":", "-") + "-" + strconv.Itoa(HostPort(port))
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

// PinAlias returns the HostKeyAlias to give ssh for a host with an explicit
// port or an IP literal. It is the new alias unless only a v1.4.13 pin exists.
func PinAlias(ctx context.Context, host string, port int, fp func(string) (string, error)) string {
	alias := KeyAlias(host, port)
	if alias == host || len(TrustedFingerprints(ctx, alias, KnownHostsPath(), fp)) != 0 {
		return alias
	}
	if legacy := LegacyKeyAlias(host, port); len(TrustedFingerprints(ctx, legacy, KnownHostsPath(), fp)) != 0 {
		return legacy
	}
	return alias
}
