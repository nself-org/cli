package trust

import (
	"context"
	"runtime"
	"time"

	"github.com/nself-org/cli/internal/ssl"
	"github.com/nself-org/cli/internal/trust/sysexec"
)

// TrustStatus holds the current state of all trust components.
type TrustStatus struct {
	MkcertInstalled    bool // mkcert binary available on PATH
	CAInstalled        bool // mkcert CA in system keychain / trusted store
	CertsExist         bool // ssl/fullchain.pem + ssl/privkey.pem exist
	CertsValid         bool // certs not expired (>30 days remaining)
	DNSInstalled       bool // dnsmasq installed / systemd-resolved config present
	DNSRunning         bool // dnsmasq conf has .local wildcard line configured
	ResolverConfigured bool // /etc/resolver/local or systemd-resolved drop-in present
	PortsForwarding    bool // 443→NginxSSLPort, 80→NginxHTTPPort active
}

// CheckStatus checks the current state of all trust components for the
// given TrustConfig and returns a TrustStatus summary.
func CheckStatus(cfg TrustConfig) TrustStatus {
	s := TrustStatus{}

	// --- mkcert ---
	_, err := sysexec.LookPath("mkcert")
	s.MkcertInstalled = err == nil

	if s.MkcertInstalled {
		caPath, pathErr := ssl.MkcertCACertPath()
		if pathErr == nil {
			trusted, _ := ssl.IsCAInstalled(caPath)
			s.CAInstalled = trusted
		}
	}

	// --- certificates ---
	_, caInstalled, certsValid := CheckMkcert(cfg)
	if s.MkcertInstalled {
		s.CAInstalled = caInstalled
	}
	s.CertsValid = certsValid
	if cfg.WorkDir != "" {
		fullchain := cfg.WorkDir + "/ssl/fullchain.pem"
		privkey := cfg.WorkDir + "/ssl/privkey.pem"
		s.CertsExist = fileExists(fullchain) && fileExists(privkey)
	}

	// --- DNS + resolver ---
	switch runtime.GOOS {
	case "darwin":
		dns, resolver := CheckDNSDarwin()
		// On macOS, "installed" means dnsmasq is in the brew list.
		s.DNSInstalled = isDnsmasqInstalledDarwin()
		s.DNSRunning = dns
		s.ResolverConfigured = resolver

	case "linux":
		configured := CheckDNSLinux()
		s.DNSInstalled = configured
		s.DNSRunning = configured
		s.ResolverConfigured = configured

	default:
		// Unsupported OS — leave all false.
	}

	// --- port forwarding ---
	switch runtime.GOOS {
	case "darwin":
		s.PortsForwarding = CheckPortsDarwin(cfg)
	case "linux":
		s.PortsForwarding = CheckPortsLinux(cfg)
	}

	return s
}

// isDnsmasqInstalledDarwin checks if dnsmasq is listed in Homebrew's installed packages.
func isDnsmasqInstalledDarwin() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return sysexec.Run(ctx, "brew", "list", "dnsmasq") == nil
}
