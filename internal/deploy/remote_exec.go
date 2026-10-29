package deploy

// Purpose: Exported remote-command primitives reused by non-deploy commands
//          (nself db migrate/hasura) that need to run a command against a
//          remote target's docker daemon over SSH, instead of assuming the
//          project is always on the local docker host (gap #9 in
//          nself-cli-gaps-from-ntask-dogfood.md).
// Inputs:  Target env name ("staging"/"prod") or an explicit Server-shaped
//          host string ("user@host:/remote/path"), plus the remote command
//          argv to run.
// Outputs: Combined stdout+stderr and an error wrapping any SSH/exec failure.
// Constraints: Reuses the SSH host-key policy + key resolution already used by
//              `nself deploy` (acceptNewPolicy/remote.DefaultKeyPath) so remote
//              auth behavior is identical across every remote-targeting
//              command in the CLI — no second SSH convention.
// SPORT: cli/internal/deploy — see gap #9.

import (
"bytes"
"context"
"fmt"
"os"
"strconv"
"strings"

	"github.com/nself-org/cli/internal/hostpin"
"github.com/nself-org/cli/sdk/go/v2/remote"
)

// deployKeyScanTypes is the keyscan order: ed25519 first, then ecdsa, then rsa.
var deployKeyScanTypes = []string{"ed25519", "ecdsa", "rsa"}

// ScanDeployKeys reads the live host keys. It makes one ssh-keyscan call per
// algorithm (several algorithms per call trip connection throttling on small
// sshd hosts), ed25519 first, and tries the next algorithm only when the
// previous call returned no key. A connect error (no server banner) stops at once: a
// second attempt cannot succeed. HostKey.Type comes from the scanned line.
func ScanDeployKeys(ctx context.Context, spec remote.HostSpec) ([]remote.HostKey, error) {
	for _, typ := range deployKeyScanTypes {
		cmd, err := remote.Command(ctx, "ssh-keyscan", "-T", "5", "-t", typ, "-p", strconv.Itoa(deployHostPort(spec)), "--", spec.Host)
		if err != nil {
			return nil, err
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err = cmd.Run()
		if keys := parseScannedKeys(stdout.String()); len(keys) > 0 {
			return keys, nil
		}
		// OpenSSH exits 1 when the host offers no key of the asked type, but it
		// has printed the server banner ("# host:port SSH-2.0-...") by then, on
		// stdout or stderr depending on the release. No banner means no
		// connection (refused, timeout): stop, another type cannot work.
		if err != nil && !scanSawBanner(stdout.String()+"\n"+stderr.String()) {
			return nil, fmt.Errorf("ssh-keyscan %s: %w", spec.Host, err)
		}
	}
	return nil, fmt.Errorf("ssh-keyscan %s returned no ed25519, ecdsa or rsa key", spec.Host)
}

// scanSawBanner reports whether ssh-keyscan connected: it prints a
// "# host:port SSH-2.0-..." comment line once it has read the server banner.
func scanSawBanner(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# ") && strings.Contains(line, "SSH-") {
			return true
		}
	}
	return false
}

// parseScannedKeys reads "host type base64" lines; comment and blank lines
// and lines remote.Fingerprint refuses are dropped.
func parseScannedKeys(out string) []remote.HostKey {
	var keys []remote.HostKey
	for _, line := range strings.Split(out, "\n") {
		fp, err := remote.Fingerprint(line)
		if err != nil {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		keys = append(keys, remote.HostKey{Line: line, Type: fields[1], Fingerprint: fp})
	}
	return keys
}

func deployTrustedFingerprints(ctx context.Context, alias, path string) map[string]bool {
	return hostpin.TrustedFingerprints(ctx, alias, path)
}

// RemoteTarget describes a resolved remote host for a non-deploy command.
type RemoteTarget struct {
	// SSHTarget is "user@host" (no path component).
	SSHTarget string
	// RemotePath is the absolute path on the remote host where the nSelf
	// project lives (used only for cd-prefixed commands; empty is fine for
	// a plain `docker exec`, which needs no working directory).
	RemotePath string
	// KeyPath is the SSH private key path.
	KeyPath string
}

// ResolveRemoteTargetFromEnv builds a RemoteTarget for "staging"/"prod" from
// the same NSELF_DEPLOY_HOST_<TARGET> / <TARGET>_DEPLOY_HOST env-var
// convention used by `nself deploy`. Returns ok=false when no host is
// configured for target (e.g. "local", or a target with no env var set) —
// callers should fall back to local execution in that case.
func ResolveRemoteTargetFromEnv(target string) (rt RemoteTarget, ok bool) {
	cfg := SSHConfigFromEnv(target)
	if cfg.Host == "" {
		return RemoteTarget{}, false
	}
	spec, err := remote.ParseHostSpec(cfg.Host)
	if err != nil {
		// Preserve the remote route so RunRemoteCommand rejects the bad host.
		return RemoteTarget{SSHTarget: cfg.Host, KeyPath: cfg.KeyPath}, true
	}
	sshTarget, remotePath := spec.String(), spec.LegacyPath
	return RemoteTarget{
		SSHTarget:  sshTarget,
		RemotePath: remotePath,
		KeyPath:    cfg.KeyPath,
	}, true
}

// RunRemoteCommand runs command on the remote target via SSH and returns
// combined stdout+stderr. Errors are wrapped with the remote target and
// command for easy diagnosis; callers that need to detect "command not
// found on an older CLI" (gap #16, version drift) should inspect the
// returned error text for the exec/ssh "command not found" markers.
func RunRemoteCommand(ctx context.Context, rt RemoteTarget, command string) (string, error) {
	if rt.SSHTarget == "" {
		return "", fmt.Errorf("remote target has no SSH host configured")
	}
	spec, err := remote.ParseHostSpec(rt.SSHTarget)
	if err != nil {
		return "", err
	}
	// Use deploy's isolated TOFU policy in v1.5; v1.4 retains its historical
	// accept-new argv. sdk/go/remote still owns the exec funnel.
	args := append([]string{"-i", rt.KeyPath}, acceptNewPolicy()...)
	args = append(args, "-o", "ForwardAgent=no")
	args = append(args, spec.SSHArgs()...)
	args = append(args, command)
	sc, err := remote.Command(ctx, "ssh", args...)
	if err != nil {
		return "", err
	}
	sc.Env = os.Environ()
	out, err := sc.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		return trimmed, fmt.Errorf("remote command on %s failed: %w\n%s", rt.SSHTarget, err, trimmed)
	}
	return trimmed, nil
}

// RemoteDockerExecCommand builds the shell command string for
// `docker exec <container> <args...>` run on the remote host. Arguments are
// shell-quoted individually so container names/args containing spaces are
// passed through safely.
func RemoteDockerExecCommand(container string, args ...string) string {
	parts := make([]string, 0, len(args)+3)
	parts = append(parts, "docker", "exec", shellQuote(container))
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

// shellQuote wraps s in single quotes for safe inclusion in a remote shell
// command string, escaping any embedded single quotes POSIX-style.
func shellQuote(s string) string {
	return remote.ShellQuote(s)
}
