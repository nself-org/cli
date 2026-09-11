package runner

// Purpose: a single command-execution seam (Executor) that provision.go and
//   verify.go run every host action through, so tests substitute a fake
//   recorder instead of shelling out, and so `verify` can be pointed at
//   several hosts (SSH) or the local machine with identical calling code.
// Inputs:  a shell command string.
// Outputs: combined stdout+stderr and an error (non-zero exit is returned
//   as an error, mirroring os/exec.CombinedOutput's contract).
// Constraints: SSHExecutor reuses internal/deploy's exported SSH primitives
//   (RunRemoteCommand/RemoteTarget) rather than opening a second, divergent
//   SSH code path — the same reuse rule the P7-E5 runner-fleet ticket
//   applies to job dispatch applies here to host verification.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
)

// Executor runs one shell command against a bound host and reports its
// label (used in verify's parity matrix and provision's log lines).
type Executor interface {
	Run(ctx context.Context, command string) (output string, err error)
	Label() string
}

// LocalExecutor runs commands on the current machine via `bash -c`. Bash
// (not POSIX sh) is required because the chromium-cache check uses
// `shopt -s nullglob`; every provisioning target (Ubuntu runner hosts) and
// every dev machine this ships on has bash available.
type LocalExecutor struct{}

// Run implements Executor.
func (LocalExecutor) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// Label implements Executor.
func (LocalExecutor) Label() string { return "local" }

// SSHExecutor runs commands on a remote host over SSH, reusing
// deploy.RunRemoteCommand so runner provisioning shares exactly one SSH
// convention (key resolution, StrictHostKeyChecking, ForwardAgent=no) with
// `nself deploy` and every other remote-targeting command in the CLI.
type SSHExecutor struct {
	Target deploy.RemoteTarget
}

// NewSSHExecutor builds an SSHExecutor from a "user@host" (or
// "user@host:/path", though runner provisioning ignores the path
// component) string and an SSH key path. An empty keyPath resolves the
// same NSELF_DEPLOY_KEY_PATH / NSELF_DEPLOY_SSH_KEY env-var convention
// `nself deploy` uses, defaulting to ~/.ssh/id_ed25519, so every
// remote-targeting command in the CLI shares one key-resolution rule.
func NewSSHExecutor(sshTarget, keyPath string) SSHExecutor {
	if keyPath == "" {
		keyPath = defaultSSHKeyPath()
	}
	return SSHExecutor{Target: deploy.RemoteTarget{
		SSHTarget: sshTarget,
		KeyPath:   keyPath,
	}}
}

// defaultSSHKeyPath mirrors deploy's unexported sshKeyPathEnv() default
// resolution (that function isn't exported; this is the same three-step
// fallback, not a divergent convention).
func defaultSSHKeyPath() string {
	if k := os.Getenv("NSELF_DEPLOY_KEY_PATH"); k != "" {
		return k
	}
	if k := os.Getenv("NSELF_DEPLOY_SSH_KEY"); k != "" {
		return k
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519")
}

// Run implements Executor.
func (e SSHExecutor) Run(ctx context.Context, command string) (string, error) {
	return deploy.RunRemoteCommand(ctx, e.Target, command)
}

// Label implements Executor.
func (e SSHExecutor) Label() string { return e.Target.SSHTarget }
