package trust

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/sysexec"
)

const iptablesTimeout = 30 * time.Second

// CheckPortsLinux returns true if the iptables OUTPUT redirect rules are
// already in place for the configured Nginx ports.
func CheckPortsLinux(cfg TrustConfig) bool {
	// -C checks for the rule; exits 0 if present, non-zero if not.
	ctx, cancel := context.WithTimeout(context.Background(), iptablesTimeout)
	defer cancel()

	if err := sysexec.Run(ctx, "iptables",
		"-t", "nat",
		"-C", "OUTPUT",
		"-p", "tcp",
		"--dport", "443",
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", cfg.NginxSSLPort),
	); err != nil {
		return false
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), iptablesTimeout)
	defer cancel2()

	return sysexec.Run(ctx2, "iptables",
		"-t", "nat",
		"-C", "OUTPUT",
		"-p", "tcp",
		"--dport", "80",
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", cfg.NginxHTTPPort),
	) == nil
}

// SetupPortsLinux adds iptables NAT OUTPUT rules to redirect ports 80 and 443
// to the configured Nginx ports. Returns alreadyDone=true if rules already exist.
func SetupPortsLinux(cfg TrustConfig) (alreadyDone bool, err error) {
	if CheckPortsLinux(cfg) {
		return true, nil
	}

	// Headless: state was checked above (read-only). iptables rules and the
	// persistence unit need root, so make no change.
	if isHeadless() {
		return false, fmt.Errorf("port forwarding setup: %w", ErrSkippedHeadless)
	}

	if os.Geteuid() != 0 {
		return false, fmt.Errorf(
			"root privileges required for iptables port forwarding — run with sudo:\n"+
				"  sudo nself trust --skip-dns --skip-ssl\n"+
				"Or add the rules manually:\n"+
				"  sudo iptables -t nat -A OUTPUT -p tcp --dport 443 -j REDIRECT --to-port %d\n"+
				"  sudo iptables -t nat -A OUTPUT -p tcp --dport 80 -j REDIRECT --to-port %d",
			cfg.NginxSSLPort,
			cfg.NginxHTTPPort,
		)
	}

	// Add SSL redirect rule.
	if err := addIPTablesRule(cfg.NginxSSLPort, 443); err != nil {
		return false, fmt.Errorf("adding iptables SSL rule: %w", err)
	}

	// Add HTTP redirect rule.
	if err := addIPTablesRule(cfg.NginxHTTPPort, 80); err != nil {
		return false, fmt.Errorf("adding iptables HTTP rule: %w", err)
	}

	// Persist rules.
	if err := persistIPTablesRules(); err != nil {
		// Non-fatal: warn but don't fail the entire setup.
		_ = err
	}

	return false, nil
}

// addIPTablesRule adds a single NAT OUTPUT REDIRECT rule for the given ports.
func addIPTablesRule(toPort, fromPort int) error {
	ctx, cancel := context.WithTimeout(context.Background(), iptablesTimeout)
	defer cancel()

	if out, err := sysexec.Output(ctx, "iptables",
		"-t", "nat",
		"-A", "OUTPUT",
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d", fromPort),
		"-j", "REDIRECT",
		"--to-port", fmt.Sprintf("%d", toPort),
	); err != nil {
		return fmt.Errorf("iptables -A OUTPUT --dport %d: %w — %s", fromPort, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// persistIPTablesRules saves current iptables rules using iptables-save and
// creates a systemd unit if iptables-save is not available. Returns an error
// only if both approaches fail — the caller treats this as non-fatal.
func persistIPTablesRules() error {
	// Try iptables-save to /etc/iptables/rules.v4.
	if _, err := sysexec.LookPath("iptables-save"); err == nil {
		if mkErr := os.MkdirAll(sysPath("/etc/iptables"), 0755); mkErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), iptablesTimeout)
			defer cancel()

			out, saveErr := sysexec.Stdout(ctx, "iptables-save")
			if saveErr == nil {
				writeErr := os.WriteFile(sysPath("/etc/iptables/rules.v4"), out, 0644)
				if writeErr == nil {
					return nil
				}
			}
		}
	}

	// Fallback: create a systemd unit to restore rules on boot.
	const unitContent = `[Unit]
Description=nself iptables port forwarding rules
After=network.target

[Service]
Type=oneshot
ExecStart=/sbin/iptables-restore /etc/iptables/nself.rules
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	// Save rules to a dedicated file.
	ctx, cancel := context.WithTimeout(context.Background(), iptablesTimeout)
	defer cancel()

	out, err := sysexec.Stdout(ctx, "iptables-save")
	if err != nil {
		return fmt.Errorf("iptables-save: %w", err)
	}

	if writeErr := os.WriteFile(sysPath("/etc/iptables/nself.rules"), out, 0644); writeErr != nil {
		return fmt.Errorf("writing /etc/iptables/nself.rules: %w", writeErr)
	}

	unitPath := sysPath("/etc/systemd/system/nself-portforward.service")
	if writeErr := os.WriteFile(unitPath, []byte(unitContent), 0644); writeErr != nil {
		return fmt.Errorf("writing systemd unit: %w", writeErr)
	}

	// Enable the unit.
	enableCtx, enableCancel := context.WithTimeout(context.Background(), iptablesTimeout)
	defer enableCancel()

	if out, err := sysexec.Output(enableCtx, "systemctl", "enable", "nself-portforward.service"); err != nil {
		return fmt.Errorf("enabling nself-portforward service: %w — %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}
