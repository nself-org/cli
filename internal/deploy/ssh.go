// Package deploy provides deployment management for the nSelf CLI.
// This file implements the SSH deploy helper used by 'nself deploy --env staging|prod' (G-003).
//
// Required env vars:
//
//	NSELF_DEPLOY_HOST   - Remote host in user@host:/remote/path format
//	NSELF_DEPLOY_USER   - SSH user (overridden by the user@host prefix when set)
//	NSELF_DEPLOY_KEY_PATH - Path to SSH private key (default: ~/.ssh/id_ed25519)
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// SSHConfig holds parameters for an SSH-based remote deploy.
type SSHConfig struct {
	// Host is the remote target in "user@host:/remote/path" format.
	// NSELF_DEPLOY_HOST is the conventional env var name.
	Host string

	// KeyPath is the path to the SSH private key.
	// Falls back to NSELF_DEPLOY_KEY_PATH, then ~/.ssh/id_ed25519.
	KeyPath string

	// Follow streams container logs after a successful deploy until the
	// context is cancelled (e.g. Ctrl-C from the user).
	Follow bool
}

// SSHConfigFromEnv builds an SSHConfig from environment variables.
// target should be "staging" or "prod".
func SSHConfigFromEnv(target string) SSHConfig {
	upper := strings.ToUpper(target)
	host := os.Getenv("NSELF_DEPLOY_HOST_" + upper)
	if host == "" {
		// Fallback: STAGING_DEPLOY_HOST / PROD_DEPLOY_HOST (legacy convention).
		host = os.Getenv(upper + "_DEPLOY_HOST")
	}
	return SSHConfig{
		Host:    host,
		KeyPath: remote.DefaultKeyPath(),
	}
}

// DeployViaSsh copies the compose file to the remote host via rsync, pulls
// updated images, and runs 'docker compose up -d' on the remote.
// If cfg.Follow is true, it streams 'docker compose logs --follow' until the
// context is cancelled.
//
// Steps:
//  1. rsync composePath to remote:/tmp/nself-compose.yml
//  2. ssh exec "docker compose -f /tmp/nself-compose.yml pull"
//  3. ssh exec "docker compose -f /tmp/nself-compose.yml up -d"
//  4. if follow: stream "docker compose -f /tmp/nself-compose.yml logs --follow"
func DeployViaSsh(ctx context.Context, cfg SSHConfig, composePath string) error {
	if cfg.Host == "" {
		return fmt.Errorf("SSHConfig.Host is empty; set NSELF_DEPLOY_HOST_<TARGET>")
	}

	sshTarget, remotePath, err := splitHost(cfg.Host)
	if err != nil {
		return err
	}

	// T31 defense-in-depth: remotePath is about to be joined into a path and
	// interpolated into shell command strings executed on the remote host
	// (see the fmt.Sprintf calls below and runSSH). The CLI-flag and
	// inventory-file layers (cmd/commands/env_target_crud.go,
	// internal/controlplane's inventory Load/synthesize) already validate
	// this before it reaches SSHConfig, but this check holds even if a
	// future caller (a test, a new command) constructs an SSHConfig
	// directly, bypassing those layers.
	if err := ValidateRemotePath(remotePath); err != nil {
		return fmt.Errorf("SSHConfig.Host: %w", err)
	}

	// The destination becomes an ssh/rsync operand: refuse anything ssh would
	// read as an option or split (leading '-', whitespace, control bytes).
	if err := remote.ValidateLegacyDest(sshTarget); err != nil {
		return fmt.Errorf("SSHConfig.Host: %w", err)
	}

	remoteCompose := filepath.Join(remotePath, "nself-compose.yml")
	if remotePath == "" || remotePath == "/" {
		remoteCompose = "/tmp/nself-compose.yml"
	}

	sshArgs := remote.BaseOptions(cfg.KeyPath)

	// 1. rsync compose file to remote.
	rsyncArgs := []string{
		// Agent forwarding is disabled via ForwardAgent=no in remote.BaseOptions (the -e
		// command below) — it is an ssh option and must never appear in rsync argv.
		"-az",
		"-e", "ssh " + strings.Join(sshArgs, " "),
		composePath,
		fmt.Sprintf("%s:%s", sshTarget, remoteCompose),
	}
	rc, err := remote.Command(ctx, "rsync", rsyncArgs...)
	if err != nil {
		return fmt.Errorf("rsync to %s: %w", sshTarget, err)
	}
	rc.Env = os.Environ()
	if out, rerr := rc.CombinedOutput(); rerr != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", sshTarget, rerr, strings.TrimSpace(string(out)))
	}

	// 2. docker compose pull on remote.
	if err := runSSH(ctx, sshTarget, cfg.KeyPath,
		fmt.Sprintf("docker compose -f %s pull", remoteCompose)); err != nil {
		return fmt.Errorf("remote pull: %w", err)
	}

	// 3. docker compose up -d on remote.
	if err := runSSH(ctx, sshTarget, cfg.KeyPath,
		fmt.Sprintf("docker compose -f %s up -d", remoteCompose)); err != nil {
		return fmt.Errorf("remote up: %w", err)
	}

	// 4. Optional log stream.
	if cfg.Follow {
		sc, serr := remote.Command(ctx, "ssh",
			append(remote.BaseOptions(cfg.KeyPath), sshTarget,
				fmt.Sprintf("docker compose -f %s logs --follow", remoteCompose))...)
		if serr == nil {
			sc.Env = os.Environ()
			sc.Stdout = os.Stdout
			sc.Stderr = os.Stderr
			// Ctrl-C or context cancellation; non-zero exit is not an error from the
			// user's perspective.
			_ = sc.Run()
		}
	}

	return nil
}

// runSSH executes a command on the remote host via SSH and returns any error.
func runSSH(ctx context.Context, sshTarget, keyPath, command string) error {
	args := append(remote.BaseOptions(keyPath), sshTarget, command)
	sc, err := remote.Command(ctx, "ssh", args...)
	if err != nil {
		return fmt.Errorf("ssh %s %q: %w", sshTarget, command, err)
	}
	sc.Env = os.Environ()
	if out, err := sc.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh %s %q: %w\n%s", sshTarget, command, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// splitHost splits "user@host:/remote/path" into ("user@host", "/remote/path").
// The path component may be empty when no colon is present, in which case /tmp
// is used by the caller.
func splitHost(host string) (sshTarget, remotePath string, err error) {
	idx := strings.LastIndex(host, ":")
	if idx < 0 {
		return host, "", nil
	}
	return host[:idx], host[idx+1:], nil
}
