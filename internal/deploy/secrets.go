// Package deploy provides deployment orchestration for nSelf projects.
// This file implements secrets push over SSH using scp (G-003 / ops-profile).
//
// Purpose:   Copy a local .env file to a remote deploy host without echoing values.
// Inputs:    SSHConfig (host + key path), PushSecretsOptions (target, paths, dry-run).
// Outputs:   Remote ~/app/.env written; nothing secret reaches stdout.
// Constraints: scp must be in PATH; SSH key must be pre-installed on remote.
// SPORT:     deploy subsystem — F08-SERVICE-INVENTORY.md
package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// PushSecretsOptions configures the secrets push operation.
type PushSecretsOptions struct {
	// Target is the deploy target name (e.g. "ops", "staging", "prod").
	Target string

	// EnvFile is the local path to the .env file to push.
	// Defaults to {ProjectRoot}/.env.{Target}, falling back to {ProjectRoot}/.env.
	EnvFile string

	// RemotePath is the remote destination path. Defaults to ~/app/.env.
	RemotePath string

	// DryRun logs what would be done without executing scp.
	DryRun bool

	// ProjectRoot is the local project root used to resolve the default EnvFile.
	// Defaults to the current working directory when empty.
	ProjectRoot string
}

// PushSecrets copies a local .env file to the remote deploy host over SSH using scp.
//
// Secret values are never printed to stdout; only the target path and host are logged.
// The operation is idempotent: re-running safely overwrites the remote file.
//
// Resolution order for EnvFile (when not explicitly set):
//  1. {ProjectRoot}/.env.{Target}
//  2. {ProjectRoot}/.env
//
// The remote host is extracted from SSHConfig.Host which may be:
//   - "user@host" or "host"          (RemotePath used as-is)
//   - "user@host:/remote/path"       (path component ignored; RemotePath wins)
func PushSecrets(ctx context.Context, cfg SSHConfig, opts PushSecretsOptions) error {
	if cfg.Host == "" {
		return fmt.Errorf("SSHConfig.Host is empty; set NSELF_DEPLOY_HOST_%s", strings.ToUpper(opts.Target))
	}

	// Resolve project root.
	root := opts.ProjectRoot
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolving project root: %w", err)
		}
	}

	// Resolve env file path.
	envFile, err := resolveEnvFile(opts.EnvFile, root, opts.Target)
	if err != nil {
		return err
	}

	// Resolve remote path.
	remotePath := opts.RemotePath
	if remotePath == "" {
		remotePath = "~/app/.env"
	}

	// Extract sshTarget (strip any path component from Host).
	spec, err := remote.ParseHostSpec(cfg.Host)
	if err != nil {
		return fmt.Errorf("parsing deploy host: %w", err)
	}
	sshTarget := spec.Dest()
	if strings.Contains(spec.Host, ":") {
		sshTarget = "[" + spec.Host + "]"
		if spec.User != "" {
			sshTarget = spec.User + "@" + sshTarget
		}
	}

	remoteDestination := fmt.Sprintf("%s:%s", sshTarget, remotePath)

	if opts.DryRun {
		fmt.Printf("DRY-RUN: would push secrets to %s (source: %s)\n",
			remoteDestination, envFile)
		return nil
	}
	serverName := cfg.ServerName
	if serverName == "" {
		serverName = opts.Target + "-app"
	}
	policy, err := HostKeyOptions(ctx, spec.String(), opts.Target, serverName, true)
	if err != nil {
		return err
	}

	// Build scp command. Stdout is not connected — prevents secret values from
	// appearing in terminal output. Stderr is forwarded so scp errors are visible.
	scpArgs := append([]string{"-i", cfg.KeyPath}, policy...)
	scpArgs = append(scpArgs, "-o", "ForwardAgent=no")
	scpArgs = append(scpArgs, spec.SSHOptions()...)
	if spec.Port != 0 {
		scpArgs[len(scpArgs)-2] = "-P"
	}
	scpArgs = append(scpArgs, "--", envFile, remoteDestination)

	cmd, err := remote.Command(ctx, "scp", scpArgs...)
	if err != nil {
		return fmt.Errorf("scp secrets to %s: %w", remoteDestination, err)
	}
	cmd.Env = os.Environ()
	cmd.Stdout = nil // intentionally suppressed — file contents must not reach stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("scp secrets to %s: %w", remoteDestination, err)
	}

	fmt.Printf("Secrets pushed to %s\n", remoteDestination)
	return nil
}

// resolveEnvFile returns the env file path to use for the push.
// If explicit is non-empty it is used directly (existence check still applies).
// Otherwise it tries {root}/.env.{target} then {root}/.env.
func resolveEnvFile(explicit, root, target string) (string, error) {
	if explicit != "" {
		if err := checkReadable(explicit); err != nil {
			return "", fmt.Errorf("env file %s: %w", explicit, err)
		}
		return explicit, nil
	}

	// Try target-specific file first.
	if target != "" {
		candidate := filepath.Join(root, ".env."+target)
		if err := checkReadable(candidate); err == nil {
			return candidate, nil
		}
	}

	// Fall back to .env.
	fallback := filepath.Join(root, ".env")
	if err := checkReadable(fallback); err != nil {
		return "", fmt.Errorf("no env file found: tried .env.%s and .env in %s", target, root)
	}
	return fallback, nil
}

// checkReadable returns an error if the file does not exist or is not readable.
func checkReadable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	_ = f.Close()
	return nil
}
