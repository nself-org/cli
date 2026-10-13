//go:build darwin

package trust

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/sysexec"
)

// pfTimeout is the timeout for pfctl operations.
//
// It lived in the untagged ports_common.go alongside pfAnchorPath and friends,
// whose comments explain they sit there "so that tests on all platforms can
// reference the constant". That rationale does not hold for pfTimeout: nothing
// outside this darwin-only file ever referenced it, so on linux it was simply
// dead and the unused linter said so. The three constants that ARE referenced
// cross-platform stay where they are.
const pfTimeout = 30 * time.Second

// pfAnchorPath, launchDaemonPlistPath, pfTimeout, and launchDaemonPlistContent
// are defined in ports_common.go (no build tag) so tests on all platforms can
// reference them.

// pfAnchorContent returns the pf anchor rules for the given config.
func pfAnchorContent(cfg TrustConfig) string {
	return fmt.Sprintf(
		"rdr pass on lo0 inet proto tcp from any to any port 80 -> 127.0.0.1 port %d\n"+
			"rdr pass on lo0 inet proto tcp from any to any port 443 -> 127.0.0.1 port %d\n",
		cfg.NginxHTTPPort,
		cfg.NginxSSLPort,
	)
}

// alreadyConfiguredPfctl checks whether the nself.local pf anchor is active by
// running a read-only pfctl query. Returns (true, nil) when the anchor rules
// match the expected ports. Returns (false, nil) when the rules are absent.
// Returns (false, err) only on unexpected errors so callers can decide whether
// to fall back to the prompt path.
func alreadyConfiguredPfctl(ctx context.Context, cfg TrustConfig) (bool, error) {
	out, err := sysexec.Output(ctx, "pfctl", "-a", "nself.local", "-sr")
	if err != nil {
		// pfctl exits non-zero when the anchor doesn't exist OR when it lacks
		// read permission. Distinguish the two: if the output contains a
		// permission-related message we treat it as an unknown state (not "not
		// installed") so the caller can consult the file-based fallback.
		combined := string(out)
		if strings.Contains(combined, "permission") ||
			strings.Contains(combined, "Permission") ||
			strings.Contains(combined, "Operation not permitted") {
			return false, fmt.Errorf("pfctl permission denied: %s", strings.TrimSpace(combined))
		}
		// Anchor missing or pfctl not found — definitely not configured.
		return false, nil
	}

	output := string(out)
	httpRule := fmt.Sprintf("port 80 -> 127.0.0.1 port %d", cfg.NginxHTTPPort)
	sslRule := fmt.Sprintf("port 443 -> 127.0.0.1 port %d", cfg.NginxSSLPort)

	if strings.Contains(output, httpRule) && strings.Contains(output, sslRule) {
		return true, nil
	}
	return false, nil
}

// launchDaemonLoaded returns true if the com.nself.portforward LaunchDaemon plist
// is present on disk AND the service appears to be loaded. It checks the plist
// file without admin access, then queries launchctl as a best-effort confirmation.
// When launchctl fails or is unavailable it falls back to the plist-presence check
// alone — better to skip the dialog when the plist is present than to re-prompt
// every time pfctl happens to be restricted.
func launchDaemonLoaded() bool {
	// Quick file presence check — no admin needed.
	if _, err := os.Stat(sysPath(launchDaemonPlistPath)); err != nil {
		return false
	}
	// Best-effort: ask launchctl whether the service is known to the system.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := sysexec.Run(ctx, "launchctl", "print", "system/com.nself.portforward")
	if err != nil {
		// launchctl failed (not loaded or permission issue). The plist is present
		// though — treat as loaded to avoid unnecessary re-prompting.
		return true
	}
	return true
}

// CheckPortsDarwin returns true if the pf anchor rules are already loaded
// with the expected port forwarding configuration.
//
// It tries pfctl first. If pfctl is unavailable or returns a permission error,
// it falls back to checking whether the LaunchDaemon plist is present and the
// service is loaded — a reliable proxy for "we already ran the osascript block".
func CheckPortsDarwin(cfg TrustConfig) bool {
	ctx, cancel := context.WithTimeout(context.Background(), pfTimeout)
	defer cancel()

	configured, err := alreadyConfiguredPfctl(ctx, cfg)
	if err == nil {
		// pfctl succeeded (zero exit). Trust its answer directly.
		return configured
	}

	// pfctl returned a permission error or was unavailable. Fall back to the
	// LaunchDaemon file check. If the plist is present and the service is loaded,
	// we almost certainly already ran the setup and should skip the osascript
	// dialog. If the check is inconclusive, fall through to the prompt path so
	// we never silently skip a genuinely missing configuration.
	return launchDaemonLoaded()
}

// SetupPortsDarwin configures pfctl port forwarding on macOS so that
// ports 80 and 443 are redirected to the Nginx HTTP and SSL ports.
// Returns alreadyDone=true when the rules were already active.
//
// Idempotency: CheckPortsDarwin runs first via pfctl (no admin needed for
// read-only queries) and falls back to LaunchDaemon presence when pfctl is
// restricted. The osascript admin prompt is only issued when the state check
// determines configuration is genuinely absent.
func SetupPortsDarwin(cfg TrustConfig) (alreadyDone bool, err error) {
	if CheckPortsDarwin(cfg) {
		slog.Info("ports already configured, skipping admin prompt")
		return true, nil
	}

	// Headless: state was checked above (read-only). The anchor, pfctl load
	// and LaunchDaemon install all need an admin prompt, so make no change.
	if isHeadless() {
		return false, fmt.Errorf("port forwarding setup: %w", ErrSkippedHeadless)
	}

	anchor := pfAnchorContent(cfg)

	// Build the osascript shell commands to write anchor file, load it, and install LaunchDaemon.
	// All three steps run under administrator privileges via a single osascript call.
	shellCmd := fmt.Sprintf(
		"mkdir -p /etc/pf.anchors && "+
			"printf '%%s' '%s' > %s && "+
			"{ pfctl -ef %s; _pfrc=$?; [ \"$_pfrc\" -eq 0 ] || [ \"$_pfrc\" -eq 1 ]; } && "+
			"mkdir -p /Library/LaunchDaemons && "+
			"printf '%%s' '%s' > %s && "+
			"launchctl load -w %s",
		escapeForOsascript(anchor),
		pfAnchorPath,
		pfAnchorPath,
		escapeForOsascript(launchDaemonPlistContent),
		launchDaemonPlistPath,
		launchDaemonPlistPath,
	)

	script := fmt.Sprintf(`do shell script "%s" with administrator privileges`, escapeForOsascript(shellCmd))

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	out, execErr := sysexec.Output(ctx, "osascript", "-e", script)
	if execErr != nil {
		return false, fmt.Errorf("configuring pfctl port forwarding: %w — %s", execErr, strings.TrimSpace(string(out)))
	}

	return false, nil
}

// escapeForOsascript escapes a string for safe embedding inside an osascript
// double-quoted shell string. Single quotes inside the shell command are the
// main concern; we replace them with '\\” to break out of single-quoting
// used by printf.
func escapeForOsascript(s string) string {
	// Escape backslashes first, then double-quotes (osascript outer quotes),
	// then percent signs (printf format specifiers handled by %% in callers).
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// writePfAnchorDirect writes the pf anchor file directly (for use when
// already running as root). Exposed for testing/Linux paths.
func writePfAnchorDirect(cfg TrustConfig) error {
	if err := os.MkdirAll(sysPath("/etc/pf.anchors"), 0755); err != nil {
		return fmt.Errorf("creating /etc/pf.anchors: %w", err)
	}
	content := pfAnchorContent(cfg)
	if err := os.WriteFile(sysPath(pfAnchorPath), []byte(content), 0644); err != nil {
		return fmt.Errorf("writing pf anchor file: %w", err)
	}
	return nil
}
