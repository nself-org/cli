package trust

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/sysexec"
)

const linuxDNSTimeout = 30 * time.Second

// linuxDNSResolver identifies the active DNS resolver on Linux.
type linuxDNSResolver int

const (
	resolverUnknown        linuxDNSResolver = iota
	resolverSystemd                         // systemd-resolved
	resolverNetworkManager                  // NetworkManager with dnsmasq
	resolverDnsmasqRaw                      // raw dnsmasq
)

// detectLinuxResolver determines which DNS resolver system is in use.
func detectLinuxResolver() linuxDNSResolver {
	// Check for systemd-resolved.
	if _, err := os.Stat(sysPath("/run/systemd/resolve/stub-resolv.conf")); err == nil {
		return resolverSystemd
	}

	// Check for NetworkManager.
	if _, err := sysexec.LookPath("NetworkManager"); err == nil {
		return resolverNetworkManager
	}

	// Check for raw dnsmasq.
	if _, err := sysexec.LookPath("dnsmasq"); err == nil {
		return resolverDnsmasqRaw
	}

	return resolverUnknown
}

// CheckDNSLinux returns true if DNS is already configured for .local wildcard
// resolution on this Linux system.
func CheckDNSLinux() bool {
	switch detectLinuxResolver() {
	case resolverSystemd:
		_, err := os.Stat(sysPath("/etc/systemd/resolved.conf.d/nself.conf"))
		return err == nil

	case resolverNetworkManager, resolverDnsmasqRaw:
		// Check for the line in /etc/dnsmasq.conf or /etc/dnsmasq.d/nself.conf.
		for _, confPath := range []string{sysPath("/etc/dnsmasq.conf"), sysPath("/etc/dnsmasq.d/nself.conf")} {
			data, err := os.ReadFile(confPath)
			if err == nil && strings.Contains(string(data), dnsmasqConfLine) {
				return true
			}
		}
		return false

	default:
		return false
	}
}

// SetupDNSLinux configures DNS resolution for .local wildcard domains on Linux.
// Returns alreadyDone=true when DNS was already configured.
func SetupDNSLinux(cfg TrustConfig) (alreadyDone bool, err error) {
	if CheckDNSLinux() {
		return true, nil
	}

	// Headless: state was checked above (read-only). Writing resolver config
	// and restarting services needs root, so make no change.
	if isHeadless() {
		return false, fmt.Errorf("dns setup: %w", ErrSkippedHeadless)
	}

	// All DNS configuration on Linux requires root.
	if os.Geteuid() != 0 {
		return false, fmt.Errorf(
			"root privileges required for DNS configuration — run with sudo:\n" +
				"  sudo nself trust --skip-ssl --skip-ports",
		)
	}

	resolver := detectLinuxResolver()

	switch resolver {
	case resolverSystemd:
		return false, setupSystemdResolved()

	case resolverNetworkManager:
		// NetworkManager: configure via dnsmasq if available, else systemd-resolved fallback.
		if _, lookErr := sysexec.LookPath("dnsmasq"); lookErr == nil {
			return false, setupRawDnsmasqLinux()
		}
		return false, setupSystemdResolved()

	case resolverDnsmasqRaw:
		return false, setupRawDnsmasqLinux()

	default:
		return false, fmt.Errorf(
			"no supported DNS resolver detected (checked: systemd-resolved, NetworkManager, dnsmasq)\n" +
				"Install dnsmasq: sudo apt install dnsmasq  OR  sudo dnf install dnsmasq",
		)
	}
}

// setupSystemdResolved creates a drop-in config for systemd-resolved to resolve
// all .local queries via 127.0.0.1 (dnsmasq).
func setupSystemdResolved() error {
	confDir := sysPath("/etc/systemd/resolved.conf.d")
	confPath := confDir + "/nself.conf"

	if err := os.MkdirAll(confDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", confDir, err)
	}

	content := "[Resolve]\nDNS=127.0.0.1\nDomains=~local\n"
	if err := os.WriteFile(confPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", confPath, err)
	}

	// Restart systemd-resolved.
	ctx, cancel := context.WithTimeout(context.Background(), linuxDNSTimeout)
	defer cancel()

	if out, err := sysexec.Output(ctx, "systemctl", "restart", "systemd-resolved"); err != nil {
		return fmt.Errorf("restarting systemd-resolved: %w — %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// setupRawDnsmasqLinux appends the .local wildcard address line to the
// dnsmasq configuration and restarts the service.
func setupRawDnsmasqLinux() error {
	// Determine which conf file to use.
	confPath := ""
	for _, candidate := range []string{sysPath("/etc/dnsmasq.conf"), sysPath("/etc/dnsmasq.d/nself.conf")} {
		if _, err := os.Stat(candidate); err == nil {
			confPath = candidate
			break
		}
	}
	if confPath == "" {
		// Default to /etc/dnsmasq.conf.
		confPath = sysPath("/etc/dnsmasq.conf")
	}

	// Check if already configured.
	existing, err := os.ReadFile(confPath)
	if err == nil && strings.Contains(string(existing), dnsmasqConfLine) {
		return nil
	}

	// Append the line.
	f, err := os.OpenFile(confPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", confPath, err)
	}
	defer func() { _ = f.Close() }()

	if _, err := fmt.Fprintf(f, "\n# nself: wildcard .local DNS resolution\n%s\n", dnsmasqConfLine); err != nil {
		return fmt.Errorf("writing to %s: %w", confPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("closing %s: %w", confPath, err)
	}

	// Restart dnsmasq — try systemctl first, then sysvinit.
	if err := restartServiceLinux("dnsmasq"); err != nil {
		return fmt.Errorf("restarting dnsmasq: %w", err)
	}

	return nil
}

// restartServiceLinux restarts a service via systemctl or the sysvinit service command.
func restartServiceLinux(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), linuxDNSTimeout)
	defer cancel()

	// Try systemctl first.
	if _, err := sysexec.LookPath("systemctl"); err == nil {
		if _, err := sysexec.Output(ctx, "systemctl", "restart", name); err == nil {
			return nil
		}
	}

	// Fallback to service command.
	ctx2, cancel2 := context.WithTimeout(context.Background(), linuxDNSTimeout)
	defer cancel2()

	if out, err := sysexec.Output(ctx2, "service", name, "restart"); err != nil {
		return fmt.Errorf("service %s restart: %w — %s", name, err, strings.TrimSpace(string(out)))
	}

	return nil
}
