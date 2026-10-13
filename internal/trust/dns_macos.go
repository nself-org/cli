//go:build darwin

package trust

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/sysexec"
)

// dnsmasqConfLine, resolverContent, and resolverPath are defined in
// dns_common.go (shared across platforms so tests compile everywhere).
const dnsmasqTimeout = 60 * time.Second

// dnsmasqConfPaths returns candidate paths for dnsmasq.conf in preference order.
func dnsmasqConfPaths() []string {
	return []string{
		sysPath("/opt/homebrew/etc/dnsmasq.conf"), // Apple Silicon
		sysPath("/usr/local/etc/dnsmasq.conf"),    // Intel Mac
	}
}

// findDnsmasqConfFunc is the function used to locate dnsmasq.conf. Production
// always uses the disk-scanning helper below; tests may override this seam to
// point configureDnsmasqConf at a controllable temp path.
var findDnsmasqConfFunc = findDnsmasqConfReal

// findDnsmasqConf returns the first dnsmasq.conf path that exists.
// Falls back to the Apple Silicon default if none exist.
func findDnsmasqConf() string {
	return findDnsmasqConfFunc()
}

// findDnsmasqConfReal is the production implementation of findDnsmasqConf.
func findDnsmasqConfReal() string {
	for _, p := range dnsmasqConfPaths() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return sysPath("/opt/homebrew/etc/dnsmasq.conf")
}

// CheckDNSDarwin checks whether dnsmasq is configured for .local resolution
// and whether /etc/resolver/local is present with the correct nameserver.
func CheckDNSDarwin() (dnsConfigured bool, resolverConfigured bool) {
	// Check dnsmasq.conf
	confPath := findDnsmasqConf()
	data, err := os.ReadFile(confPath)
	if err == nil {
		dnsConfigured = strings.Contains(string(data), dnsmasqConfLine)
	}

	// Check /etc/resolver/local
	rData, err := os.ReadFile(sysPath(resolverPath))
	if err == nil {
		resolverConfigured = strings.Contains(string(rData), "nameserver 127.0.0.1")
	}

	return dnsConfigured, resolverConfigured
}

// SetupDNSDarwin configures dnsmasq and /etc/resolver/local for .local wildcard
// DNS resolution on macOS. Returns dnsAlreadyDone=true and resolverAlreadyDone=true
// when each respective component was already configured.
func SetupDNSDarwin(cfg TrustConfig) (dnsAlreadyDone bool, resolverAlreadyDone bool, err error) {
	dnsAlreadyDone, resolverAlreadyDone = CheckDNSDarwin()

	// Headless: state was checked above (read-only). Anything missing needs
	// brew, a service restart and an admin-privileged write, so make no
	// change and report the skip.
	if isHeadless() && !(dnsAlreadyDone && resolverAlreadyDone) {
		return dnsAlreadyDone, resolverAlreadyDone, fmt.Errorf("dns setup: %w", ErrSkippedHeadless)
	}

	// --- dnsmasq setup ---
	if !dnsAlreadyDone {
		if err = ensureDnsmasqInstalled(); err != nil {
			return false, resolverAlreadyDone, fmt.Errorf("installing dnsmasq: %w", err)
		}
		if err = configureDnsmasqConf(); err != nil {
			return false, resolverAlreadyDone, fmt.Errorf("configuring dnsmasq.conf: %w", err)
		}
		if err = restartDnsmasq(); err != nil {
			return false, resolverAlreadyDone, fmt.Errorf("restarting dnsmasq: %w", err)
		}
	}

	// --- /etc/resolver/local setup ---
	if !resolverAlreadyDone {
		if err = setupResolver(); err != nil {
			return dnsAlreadyDone || !dnsAlreadyDone, false, fmt.Errorf("configuring /etc/resolver/local: %w", err)
		}
		// Restart dnsmasq after writing the resolver so it is in sync regardless of
		// whether the conf was already configured. Errors are non-fatal.
		_ = restartDnsmasq()
		// Non-fatal: a stale DNS cache resolves itself; the caller returns nil
		// either way, so the error is deliberately dropped rather than stored.
		_ = flushDNSCache()
	}

	return dnsAlreadyDone, resolverAlreadyDone, nil
}

// ensureDnsmasqInstalled checks if dnsmasq is installed via Homebrew and
// installs it if not.
func ensureDnsmasqInstalled() error {
	ctx, cancel := context.WithTimeout(context.Background(), dnsmasqTimeout)
	defer cancel()

	if err := sysexec.Run(ctx, "brew", "list", "dnsmasq"); err == nil {
		// Already installed.
		return nil
	}

	// Install dnsmasq.
	installCtx, installCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer installCancel()

	if err := sysexec.Stream(installCtx, "brew", "install", "dnsmasq"); err != nil {
		return fmt.Errorf("brew install dnsmasq: %w", err)
	}
	return nil
}

// configureDnsmasqConf ensures the dnsmasq.conf contains the wildcard .local
// address line. Appends it if not present.
func configureDnsmasqConf() error {
	confPath := findDnsmasqConf()

	// Ensure the directory exists (e.g. first-time Homebrew install).
	if err := os.MkdirAll(strings.TrimSuffix(confPath, "dnsmasq.conf"), 0755); err != nil {
		return fmt.Errorf("creating dnsmasq conf directory: %w", err)
	}

	// Read existing content.
	var existing string
	data, err := os.ReadFile(confPath)
	if err == nil {
		existing = string(data)
	}

	if strings.Contains(existing, dnsmasqConfLine) {
		// Already configured.
		return nil
	}

	// Append the line.
	f, err := os.OpenFile(confPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening dnsmasq.conf: %w", err)
	}
	defer func() { _ = f.Close() }()

	_, err = fmt.Fprintf(f, "\n# nself: wildcard .local DNS resolution\n%s\n", dnsmasqConfLine)
	if err != nil {
		return fmt.Errorf("writing to dnsmasq.conf: %w", err)
	}
	return nil
}

// restartDnsmasq restarts the dnsmasq Homebrew service.
func restartDnsmasq() error {
	ctx, cancel := context.WithTimeout(context.Background(), dnsmasqTimeout)
	defer cancel()

	if out, err := sysexec.Output(ctx, "brew", "services", "restart", "dnsmasq"); err != nil {
		return fmt.Errorf("brew services restart dnsmasq: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// setupResolver writes /etc/resolver/local via osascript (requires admin).
func setupResolver() error {
	script := `do shell script "mkdir -p /etc/resolver && printf 'nameserver 127.0.0.1\n' > /etc/resolver/local" with administrator privileges`
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := sysexec.Output(ctx, "osascript", "-e", script)
	if err != nil {
		return fmt.Errorf("creating /etc/resolver/local: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// flushDNSCache flushes the macOS DNS cache.
func flushDNSCache() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := sysexec.Run(ctx, "dscacheutil", "-flushcache"); err != nil {
		return fmt.Errorf("dscacheutil -flushcache: %w", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel2()

	if err := sysexec.Run(ctx2, "killall", "-HUP", "mDNSResponder"); err != nil {
		return fmt.Errorf("killall -HUP mDNSResponder: %w", err)
	}
	return nil
}
