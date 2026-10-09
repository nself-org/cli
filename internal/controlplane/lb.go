package controlplane

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"os"
	"os/exec"
	"strings"
)

// lbHelperPath is the relative path (from the server's RemotePath) to the
// nself-lb helper binary on the remote LB server.
const lbHelperPath = "bin/nself-lb"

// DrainResult indicates the outcome of a Drain or Enable call.
type DrainResult int

const (
	// DrainOK means the helper ran and the command succeeded.
	DrainOK DrainResult = iota

	// DrainHelperAbsent means the helper binary was not found on the LB server
	// (SSH exit 127). Callers should log a WARN and degrade gracefully.
	DrainHelperAbsent

	// DrainFailed means the helper was found but returned a non-zero exit code.
	DrainFailed
)

// Drain removes appName from the active upstream pool on the LB server by
// running the remote nself-lb helper:
//
//	ssh ... <remote_path>/bin/nself-lb drain <appName>
//
// The ctx controls the lifetime of the SSH subprocess; cancelling ctx or
// reaching its deadline aborts the in-flight connection.
//
// If the helper binary is absent (SSH exit 127), Drain returns DrainHelperAbsent
// and a nil error so the caller can degrade gracefully (log WARN + continue).
// Other SSH failures return DrainFailed with a descriptive error.
func Drain(ctx context.Context, srv Server, appName string) (DrainResult, error) {
	return runLBHelper(ctx, srv, "drain", appName)
}

// Enable re-adds appName to the active upstream pool on the LB server by
// running the remote nself-lb helper:
//
//	ssh ... <remote_path>/bin/nself-lb enable <appName>
//
// The ctx controls the lifetime of the SSH subprocess.
// Same error semantics as Drain.
func Enable(ctx context.Context, srv Server, appName string) (DrainResult, error) {
	return runLBHelper(ctx, srv, "enable", appName)
}

// runLBHelper invokes the nself-lb helper on the remote LB server via SSH.
// It uses the same SSH hygiene rules as the probe layer:
//   - BatchMode=yes (no passphrase prompt)
//   - v1.5 host-key preflight before running the remote drain helper
//   - ConnectTimeout=5 (fail fast; LB changes are on the hot path)
//   - ForwardAgent=no (no agent forwarding)
//
// SECURITY: helperBin, action, and appName are passed as SEPARATE argv elements
// to the local ssh process (not concatenated into a shell string). The remote
// sshd therefore execs the binary directly without shell tokenisation, which
// prevents injection via shell metacharacters in appName or remotePath.
// ValidateServerName at inventory-load time provides a second defence in depth
// against hostile appName values originating from control-plane.yaml.
//
// The function NEVER calls osascript, sudo, pfctl, launchctl, or any elevated
// command. All privilege is delegated to the server-side nself-lb helper.
func runLBHelper(ctx context.Context, srv Server, action, appName string) (DrainResult, error) {
	keyPath := os.Getenv(srv.SSHKeyRef)
	if keyPath == "" {
		return DrainFailed, fmt.Errorf("lb: env var %s is not set for server %s", srv.SSHKeyRef, srv.Name)
	}

	remotePath := strings.TrimRight(srv.RemotePath, "/")
	// helperBin is the absolute path on the remote host. Pass it plus action and
	// appName as distinct SSH argv tokens so that remote sshd execs the binary
	// directly — no shell expansion, no injection risk.
	helperBin := remotePath + "/" + lbHelperPath

	spec, err := remote.ParseHostSpec(srv.Host)
	if err != nil {
		return DrainFailed, err
	}
	args := []string{
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", fmt.Sprintf("ConnectTimeout=%d", probeConnectTimeout),
		"-o", "ForwardAgent=no",
	}
	keyOpts, err := HostKeyOptions(ctx, "", srv.Name, TierProd, srv.Host, false)
	if err != nil {
		return DrainFailed, err
	}
	args = append(args, keyOpts...)
	args = append(args, spec.SSHArgs()...)
	args = append(args,
		// Three separate tokens: binary path, action, app name.
		// SSH with a multi-token remote command joins them with spaces and
		// passes the result to the remote sshd as the command to exec.
		// Because each token is a distinct element in our local argv, the
		// local shell never touches these values.
		helperBin, action, appName,
	)

	cmd := exec.CommandContext(ctx, "ssh", args...) //nolint:gosec // argv tokens are validated; no shell interpolation
	out, err := cmd.CombinedOutput()
	if err == nil {
		return DrainOK, nil
	}

	// Distinguish "command not found" (exit 127) from other failures.
	// SSH itself exits 127 when the remote shell cannot find the command.
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() == 127 {
			return DrainHelperAbsent, nil
		}
	}

	// Any other SSH or remote failure.
	outStr := strings.TrimSpace(string(out))
	if outStr != "" {
		return DrainFailed, fmt.Errorf("lb: %s %s on %s: %w: %s", action, appName, srv.Name, err, outStr)
	}
	return DrainFailed, fmt.Errorf("lb: %s %s on %s: %w", action, appName, srv.Name, err)
}
