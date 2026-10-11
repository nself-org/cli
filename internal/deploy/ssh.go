// Package deploy provides SSH deployment helpers for nSelf.
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// rsyncFile copies a local file using only parsed host argv and validated paths.
func rsyncFile(ctx context.Context, t sshTransport, src, dest string) error {
	args, err := t.rsyncArgv(src, dest)
	if err != nil {
		return err
	}
	rc, err := remote.Command(ctx, "rsync", args...)
	if err != nil {
		return err
	}
	rc.Env = os.Environ()
	if out, err := rc.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", t.spec.Dest(), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// envNameRe is the shape of an env name allowed in a remote .env.<name> file name.
var envNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

// SSHConfig holds parameters for an SSH-based remote deploy.
type SSHConfig struct {
	// Host is the remote target in "user@host:/remote/path" format.
	// NSELF_DEPLOY_HOST is the conventional env var name.
	Host string
	// RemotePath is the separately configured install directory for canonical hosts.
	RemotePath string

	// KeyPath is the path to the SSH private key.
	// Falls back to NSELF_DEPLOY_KEY_PATH, then ~/.ssh/id_ed25519.
	KeyPath string

	// Follow streams container logs after a successful deploy until the
	// context is cancelled (e.g. Ctrl-C from the user).
	Follow bool

	// EnvFile, when set, is a local env file rsynced to <remote path>/.env.<EnvName>
	// right after the compose file, so the host gets the env the compose was
	// built from. EnvName must be a plain name (a-z, 0-9, _ and -).
	EnvFile string
	EnvName string
	// InventoryEnv is the inventory key used by the host-key enrolment command.
	InventoryEnv string
	// ServerName is the inventory key used by the host-key enrolment command.
	ServerName string
}

// SSHConfigFromEnv builds an SSHConfig from environment variables.
func SSHConfigFromEnv(target string) SSHConfig {
	upper := strings.ToUpper(target)
	host := os.Getenv("NSELF_DEPLOY_HOST_" + upper)
	if host == "" {
		// Fallback: STAGING_DEPLOY_HOST / PROD_DEPLOY_HOST (legacy convention).
		host = os.Getenv(upper + "_DEPLOY_HOST")
	}
	return SSHConfig{
		Host:       host,
		KeyPath:    remote.DefaultKeyPath(),
		EnvName:    strings.ToLower(target),
		ServerName: strings.ToLower(target) + "-app",
	}
}

// DeployViaSsh copies env then compose, promotes env, pulls images and restarts.
func DeployViaSsh(ctx context.Context, cfg SSHConfig, composePath string) error {
	if cfg.Host == "" {
		return fmt.Errorf("SSHConfig.Host is empty; set NSELF_DEPLOY_HOST_<TARGET>")
	}

	spec, err := remote.ParseHostSpec(cfg.Host)
	if err != nil {
		return err
	}
	remotePath := cfg.RemotePath
	if spec.LegacyPath != "" {
		remotePath = spec.LegacyPath
	}

	// Validate direct callers as well as inventory callers before any process starts.
	// ValidateCopyPath also refuses ".." segments.
	if err := remote.ValidateCopyPath(remotePath); err != nil {
		return fmt.Errorf("SSHConfig.Host: %w", err)
	}

	// rsync -e re-splits the key path, so it needs the safe alphabet.
	if err := checkDeployKeyPath(cfg.KeyPath); err != nil {
		return err
	}

	// The env name is checked before the host-key scan: a bad name needs no network.
	var envTmp, envFinal string
	if cfg.EnvFile != "" {
		if !envNameRe.MatchString(cfg.EnvName) || remotePath == "" || remotePath == "/" {
			return fmt.Errorf("env file %q needs a plain env name and a non-root remote path (got %q, %q)", cfg.EnvFile, cfg.EnvName, remotePath)
		}
		envFinal = filepath.Join(remotePath, ".env."+cfg.EnvName)
		envTmp = envFinal + ".nself-new"
	}

	remoteCompose := filepath.Join(remotePath, "nself-compose.yml")
	if remotePath == "" || remotePath == "/" {
		remoteCompose = "/tmp/nself-compose.yml"
	}

	serverName := cfg.ServerName
	if serverName == "" {
		serverName = cfg.EnvName + "-app"
	}
	identityEnv := cfg.InventoryEnv
	if identityEnv == "" {
		identityEnv = cfg.EnvName
	}
	// Shipping an env file means shipping secrets: strict in v1.4 as well.
	policy, err := hostKeyPolicy(ctx, spec.String(), identityEnv, serverName, true, cfg.EnvFile != "")
	if err != nil {
		return err
	}
	t, err := newSSHTransport(spec, cfg.KeyPath, policy)
	if err != nil {
		return err
	}

	// 1. Env file first, to a temporary name, so a failed env copy never leaves
	// a new compose beside an old env. It is renamed into place only after the
	// compose copy has succeeded.
	if envTmp != "" {
		if err := rsyncFile(ctx, t, cfg.EnvFile, envTmp); err != nil {
			return err
		}
	}

	// 2. rsync compose file to remote.
	if err := rsyncFile(ctx, t, composePath, remoteCompose); err != nil {
		return err
	}

	// 3. Promote the env file.
	if envTmp != "" {
		if err := runSSH(ctx, t, fmt.Sprintf("mv -f %s %s", remote.ShellQuote(envTmp), remote.ShellQuote(envFinal))); err != nil {
			return fmt.Errorf("promoting env file: %w", err)
		}
	}

	// 4. docker compose pull on remote.
	if err := runSSH(ctx, t, fmt.Sprintf("docker compose -f %s pull", remote.ShellQuote(remoteCompose))); err != nil {
		return fmt.Errorf("remote pull: %w", err)
	}

	// 5. docker compose up -d on remote.
	if err := runSSH(ctx, t, fmt.Sprintf("docker compose -f %s up -d", remote.ShellQuote(remoteCompose))); err != nil {
		return fmt.Errorf("remote up: %w", err)
	}

	// 6. Optional log stream.
	if cfg.Follow {
		sc, serr := remote.Command(ctx, "ssh",
			t.sshArgv(fmt.Sprintf("docker compose -f %s logs --follow", remote.ShellQuote(remoteCompose)))...)
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
func runSSH(ctx context.Context, t sshTransport, command string) error {
	sc, err := remote.Command(ctx, "ssh", t.sshArgv(command)...)
	if err != nil {
		return err
	}
	sc.Env = os.Environ()
	if out, err := sc.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh %s %q: %w\n%s", t.spec.Dest(), command, err, strings.TrimSpace(string(out)))
	}
	return nil
}
