// Package hostpin provides SSH host-key pin alias resolution for callers that
// do their own pin lookup (admin connect, access). It lives at L1 so L1
// packages can resolve a pin alias without importing internal/deploy (L2).
package hostpin

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// KnownHostsPath is the dedicated deploy pin file. The operator's SSH file is
// read only; this package never writes it.
func KnownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nself", "deploy_known_hosts")
}

// HostPort returns the effective SSH port for spec (22 when unset).
func HostPort(spec remote.HostSpec) int {
	if spec.Port == 0 {
		return 22
	}
	return spec.Port
}

// KeyAlias is the pin alias for spec. A port or an IP literal needs a
// synthetic alias: the sdk refuses an IP literal as a pin alias. '+' is
// allowed in pin aliases but forbidden in host names, keeping the namespaces
// disjoint.
func KeyAlias(spec remote.HostSpec) string {
	if spec.Port != 0 || net.ParseIP(spec.Host) != nil {
		return "nself+" + spec.Host + "+" + strconv.Itoa(HostPort(spec))
	}
	return spec.Host
}

// LegacyKeyAlias is the v1.4.13 spelling of the IP/port pin alias. It can
// equal a real host name, so it is only ever read, and only when no pin exists
// under the new alias (D-0696).
func LegacyKeyAlias(spec remote.HostSpec) string {
	return "nself-" + strings.ReplaceAll(spec.Host, ":", "-") + "-" + strconv.Itoa(HostPort(spec))
}

// TrustedFingerprints returns the fingerprints ssh-keygen reports for alias in
// path. Errors are swallowed: the caller treats an empty set as "no pin".
func TrustedFingerprints(ctx context.Context, alias, path string) map[string]bool {
	seen := map[string]bool{}
	out, err := exec.CommandContext(ctx, "ssh-keygen", "-F", alias, "-f", path).Output()
	if err != nil {
		return seen
	}
	for _, line := range strings.Split(string(out), "\n") {
		if fp, err := remote.Fingerprint(line); err == nil {
			seen[fp] = true
		}
	}
	return seen
}

// PinAlias returns the HostKeyAlias to give ssh for a host with an explicit
// port or an IP literal. It is the new alias unless only a v1.4.13 pin exists.
func PinAlias(ctx context.Context, spec remote.HostSpec) string {
	alias := KeyAlias(spec)
	if alias == spec.Host || len(TrustedFingerprints(ctx, alias, KnownHostsPath())) != 0 {
		return alias
	}
	if legacy := LegacyKeyAlias(spec); len(TrustedFingerprints(ctx, legacy, KnownHostsPath())) != 0 {
		return legacy
	}
	return alias
}
