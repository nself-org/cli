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
// Constraints: Reuses the exact SSH flag set + key resolution already used by
//              `nself deploy` (remote.BaseOptions/remote.DefaultKeyPath/splitHost) so remote
//              auth behavior is identical across every remote-targeting
//              command in the CLI — no second SSH convention.
// SPORT: cli/internal/deploy — see gap #9.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

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
	sshTarget, remotePath, _ := splitHost(cfg.Host)
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
	if err := remote.ValidateLegacyDest(rt.SSHTarget); err != nil {
		return "", err
	}
	// Historical argv and inherited environment, byte-identical to the
	// pre-sdk implementation: options, destination, one command element, no
	// "--". sdk/go/remote owns the exec funnel; the legacy shape lives here
	// so the sdk's public API has no relaxed mode.
	args := append(remote.BaseOptions(rt.KeyPath), rt.SSHTarget, command)
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
