// Package deploy provides SSH deployment helpers for nSelf.
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// SSHKeyRe is the safe alphabet for an SSH key path. The one definition: the
// legacy push in cmd/commands aliases it.
var SSHKeyRe = regexp.MustCompile(`^[a-zA-Z0-9/_.~-]+$`)

// rsyncFile copies a local file using only parsed host argv and validated paths.
func rsyncFile(ctx context.Context, opts []string, spec remote.HostSpec, src, dest string) error {
	if err := remote.ValidateCopyPath(dest); err != nil {
		return err
	}
	sshOpts := append(append([]string{}, opts...), spec.SSHOptions()...)
	for _, opt := range sshOpts {
		if strings.ContainsAny(opt, " \t\n\r'\"\\") {
			return fmt.Errorf("ssh option %q is unsafe in rsync -e", opt)
		}
	}
	sshTarget := spec.String()
	if spec.Port != 0 {
		sshTarget = strings.TrimSuffix(sshTarget, ":"+strconv.Itoa(spec.Port))
	}
	args := []string{"-az", "-e", "ssh " + strings.Join(sshOpts, " "), "--", src, sshTarget + ":" + dest}
	rc, err := remote.Command(ctx, "rsync", args...)
	if err != nil {
		return err
	}
	rc.Env = os.Environ()
	if out, err := rc.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
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
}

// DeployKnownHostsPath is the dedicated pin file. The operator's SSH file is read only.
func DeployKnownHostsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "nself", "deploy_known_hosts")
}

func deployHostPort(spec remote.HostSpec) int {
	if spec.Port == 0 {
		return 22
	}
	return spec.Port
}

func deployKeyAlias(spec remote.HostSpec) string {
	if spec.Port != 0 {
		return "nself-" + strings.ReplaceAll(spec.Host, ":", "-") + "-" + strconv.Itoa(spec.Port)
	}
	return spec.Host
}

// TrustHostKey scans live keys and pins only the key with the offered fingerprint.
func TrustHostKey(ctx context.Context, host, offered string) error {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return errs.New("E484", err.Error())
	}
	if !strings.HasPrefix(offered, "SHA256:") {
		return errs.New("E487", "--trust-host-key needs a SHA256 fingerprint")
	}
	keys, err := ScanDeployKeys(ctx, spec)
	if err != nil {
		return errs.New("E487", fmt.Sprintf("host key scan for %s failed: %v", spec.Host, err))
	}
	for _, k := range keys {
		if k.Fingerprint == offered {
			return (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add(deployKeyAlias(spec), k.Line)
		}
	}
	return errs.New("E487", fmt.Sprintf("host %s presented no key matching %s", spec.Host, offered))
}

// HostKeyOptions returns accept-new or strict policy after validating the live key.
func HostKeyOptions(ctx context.Context, host, env, server string, strict bool) ([]string, error) {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return nil, errs.New("E484", err.Error())
	}
	// compat.V15(P7-DEPL-13): accept-new -> strict for secret shipping and prod targets.
	if !compat.V15() || !strict {
		return []string{"-o", "StrictHostKeyChecking=accept-new"}, nil
	}
	keys, err := ScanDeployKeys(ctx, spec)
	if err != nil {
		return nil, errs.New("E487", fmt.Sprintf("host key scan for %s failed: %v", spec.Host, err))
	}
	home, _ := os.UserHomeDir()
	operatorAlias := spec.Host
	if spec.Port != 0 {
		operatorAlias = "[" + spec.Host + "]:" + strconv.Itoa(spec.Port)
	}
	operator := deployTrustedFingerprints(ctx, operatorAlias, filepath.Join(home, ".ssh", "known_hosts"))
	pinned := deployTrustedFingerprints(ctx, deployKeyAlias(spec), DeployKnownHostsPath())
	for _, k := range keys {
		if operator[k.Fingerprint] || pinned[k.Fingerprint] {
			opts := []string{"-o", "StrictHostKeyChecking=yes", "-o", "GlobalKnownHostsFile=" + DeployKnownHostsPath()}
			if spec.Port != 0 && !operator[k.Fingerprint] {
				opts = append(opts, "-o", "HostKeyAlias="+deployKeyAlias(spec))
			}
			return opts, nil
		}
	}
	fp := keys[0].Fingerprint
	return nil, errs.New("E487", fmt.Sprintf("unknown host key %s for %s; verify it, then run nself env target add %s %s --trust-host-key %s", fp, host, env, server, fp))
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
		Host:    host,
		KeyPath: remote.DefaultKeyPath(),
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
	if err := ValidateRemotePath(remotePath); err != nil {
		return fmt.Errorf("SSHConfig.Host: %w", err)
	}

	// rsync -e re-splits the key path, so it needs the safe alphabet.
	if cfg.KeyPath != "" && !SSHKeyRe.MatchString(cfg.KeyPath) {
		return fmt.Errorf("ssh key path contains unsafe characters (got %q): only [a-zA-Z0-9/_.~-] allowed", cfg.KeyPath)
	}

	remoteCompose := filepath.Join(remotePath, "nself-compose.yml")
	if remotePath == "" || remotePath == "/" {
		remoteCompose = "/tmp/nself-compose.yml"
	}

	strict := cfg.EnvFile != "" || cfg.EnvName == "prod" || cfg.EnvName == "production"
	policy, err := HostKeyOptions(ctx, spec.String(), cfg.EnvName, spec.Host, strict)
	if err != nil {
		return err
	}
	opts := append([]string{"-i", cfg.KeyPath}, policy...)
	opts = append(opts, "-o", "ForwardAgent=no")

	// 1. Env file first, to a temporary name, so a failed env copy never leaves
	// a new compose beside an old env. It is renamed into place only after the
	// compose copy has succeeded.
	var envTmp, envFinal string
	if cfg.EnvFile != "" {
		if !envNameRe.MatchString(cfg.EnvName) || remotePath == "" || remotePath == "/" {
			return fmt.Errorf("env file %q needs a plain env name and a non-root remote path (got %q, %q)", cfg.EnvFile, cfg.EnvName, remotePath)
		}
		envFinal = filepath.Join(remotePath, ".env."+cfg.EnvName)
		envTmp = envFinal + ".nself-new"
		if err := rsyncFile(ctx, opts, spec, cfg.EnvFile, envTmp); err != nil {
			return err
		}
	}

	// 2. rsync compose file to remote.
	if err := rsyncFile(ctx, opts, spec, composePath, remoteCompose); err != nil {
		return err
	}

	// 3. Promote the env file.
	if envTmp != "" {
		if err := runSSH(ctx, opts, spec, fmt.Sprintf("mv -f %s %s", remote.ShellQuote(envTmp), remote.ShellQuote(envFinal))); err != nil {
			return fmt.Errorf("promoting env file: %w", err)
		}
	}

	// 4. docker compose pull on remote.
	if err := runSSH(ctx, opts, spec,
		fmt.Sprintf("docker compose -f %s pull", remote.ShellQuote(remoteCompose))); err != nil {
		return fmt.Errorf("remote pull: %w", err)
	}

	// 5. docker compose up -d on remote.
	if err := runSSH(ctx, opts, spec,
		fmt.Sprintf("docker compose -f %s up -d", remote.ShellQuote(remoteCompose))); err != nil {
		return fmt.Errorf("remote up: %w", err)
	}

	// 6. Optional log stream.
	if cfg.Follow {
		sc, serr := remote.Command(ctx, "ssh",
			append(append([]string{}, opts...), append(spec.SSHArgs(),
				fmt.Sprintf("docker compose -f %s logs --follow", remote.ShellQuote(remoteCompose)))...)...)
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
func runSSH(ctx context.Context, opts []string, spec remote.HostSpec, command string) error {
	args := append(append([]string{}, opts...), spec.SSHArgs()...)
	args = append(args, command)
	sc, err := remote.Command(ctx, "ssh", args...)
	if err != nil {
		return err
	}
	sc.Env = os.Environ()
	if out, err := sc.CombinedOutput(); err != nil {
		return fmt.Errorf("ssh %s %q: %w\n%s", spec.Dest(), command, err, strings.TrimSpace(string(out)))
	}
	return nil
}
