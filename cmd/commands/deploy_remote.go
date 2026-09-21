package commands

// Purpose: remote deploy helpers, loading the per-target env cascade, resolving
// the SSH key path, and pushing a build to a remote host over rsync/ssh. Inputs
// are the workdir, target name, and host; outputs are loaded env vars or an error.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// loadDeployEnvCascade loads the env file cascade for the given deploy target
// into the current process environment. Later files override earlier ones.
//
// Cascade per target:
//
//	local      → .env.dev + .env.local
//	staging    → .env.dev + .env.staging + .env.secrets
//	prod       → .env.dev + .env.prod   + .env.secrets
//
// Missing files are silently skipped. The NSELF_DEPLOY_ENV env var is set
// to the canonical target name so downstream helpers can introspect it.
//
// Gap #13 fix: this also sets ENV to the same canonical target name. ENV is
// the variable config.Load (internal/config/loader.go) actually keys its own
// file cascade on — before this fix, ENV was never set here, so the
// subsequent 'nself build' subprocess (spawned by runDeploy via runCLISelf)
// resolved config.Load's cascade against the default "dev" tier regardless
// of which target this process just loaded, silently baking dev-tier values
// (wrong POSTGRES_DB, wrong ports, etc.) into docker-compose.yml even for a
// staging/prod deploy.
func loadDeployEnvCascade(workdir, target string) {
	files := deployEnvCascadeFiles(workdir, target)
	for _, f := range files {
		if _, err := os.Stat(f); err == nil {
			// Overload merges into os.Environ — missing files already skipped above.
			_ = godotenv.Overload(f)
		}
	}
	// Expose the resolved target so subprocesses and plugins can read it.
	_ = os.Setenv("NSELF_DEPLOY_ENV", target)
	// Gap #13: make config.Load's own cascade selection agree with the
	// cascade we just loaded into this process's environment.
	_ = os.Setenv("ENV", target)
}

// deployEnvCascadeFiles returns the ordered list of .env files that make up
// target's cascade, matching config.Load's own cascade order (internal/config/loader.go)
// so the set of files loaded here and the set config.Load merges in the
// 'nself build' subprocess are identical.
func deployEnvCascadeFiles(workdir, target string) []string {
	switch target {
	case "local":
		return []string{
			filepath.Join(workdir, ".env.dev"),
			filepath.Join(workdir, ".env.local"),
		}
	case "staging":
		return []string{
			filepath.Join(workdir, ".env.dev"),
			filepath.Join(workdir, ".env.staging"),
			filepath.Join(workdir, ".env.secrets"),
		}
	default: // "prod"
		return []string{
			filepath.Join(workdir, ".env.dev"),
			filepath.Join(workdir, ".env.prod"),
			filepath.Join(workdir, ".env.secrets"),
		}
	}
}

// sshKeyPath returns the SSH key path from NSELF_DEPLOY_SSH_KEY env or the
// default ~/.ssh/id_ed25519.
func sshKeyPath() string {
	if k := os.Getenv("NSELF_DEPLOY_SSH_KEY"); k != "" {
		return k
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ssh", "id_ed25519")
}

// remoteDeployPush rsyncs the compose file and env to the remote host, then
// pulls new images and runs a rolling restart via SSH.
// host format: "user@host:/remote/path"
func remoteDeployPush(ctx context.Context, workdir, host, target string, jsonOut bool) error {
	sshKey := sshKeyPath()

	// sshKey is interpolated into the rsync "-e" command, which rsync passes
	// through a shell. Reject any key path containing shell metacharacters to
	// prevent command injection via NSELF_DEPLOY_SSH_KEY.
	if sshKey != "" && !sshKeyRe.MatchString(sshKey) {
		return fmt.Errorf("NSELF_DEPLOY_SSH_KEY contains unsafe characters (got %q): only [a-zA-Z0-9/_.~-] allowed", sshKey)
	}

	// Split user@host:/path into ssh-target and remote-path.
	colonIdx := strings.LastIndex(host, ":")
	if colonIdx < 0 {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s format must be user@host:/remote/path (got %q)", strings.ToUpper(target), host)
	}
	sshTarget := host[:colonIdx]
	remotePath := host[colonIdx+1:]
	if remotePath == "" {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s remote path is empty (got %q)", strings.ToUpper(target), host)
	}
	if !remotePathRe.MatchString(remotePath) {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s remote path contains unsafe characters (got %q): only [a-zA-Z0-9/_.-] allowed", strings.ToUpper(target), remotePath)
	}

	// Gap #13 fix: the file that used to be rsynced here (.env.<target> alone,
	// e.g. .env.staging) is only ONE layer of the cascade that config.Load
	// actually merged to produce the docker-compose.yml being pushed
	// alongside it (CLI-R18 canonical order: .env -> .env.<target> ->
	// .env.secrets -> .env.local). Pushing just one layer left the remote's
	// env file mismatched with values baked into the compose file (wrong
	// POSTGRES_DB, wrong ports, etc.) whenever an earlier/later layer set them.
	//
	// Write a merged snapshot containing every value config.Load resolved
	// for this deploy, and push that as .env.<target> instead of the raw
	// single-layer file. This keeps the remote filename convention the
	// on-box CLI already expects, while guaranteeing its contents match
	// docker-compose.yml byte-for-byte in provenance.
	resolvedEnvPath, cleanupResolvedEnv, err := writeResolvedDeployEnv(workdir, target)
	if err != nil {
		return fmt.Errorf("preparing resolved .env for %s: %w", target, err)
	}
	defer cleanupResolvedEnv()

	// rsync compose + env files to the remote.
	// Agent forwarding is disabled via ForwardAgent=no in the -e ssh command —
	// it is an ssh option and must never appear in rsync argv (breaks rsync 3.x).
	rsyncArgs := []string{
		"-az",
		"-e", fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new -o ForwardAgent=no", sshKey),
		"docker-compose.yml",
		resolvedEnvPath,
		fmt.Sprintf("%s:%s/", sshTarget, remotePath),
	}
	if !jsonOut {
		fmt.Printf("  [running] rsync compose + resolved env to %s:%s\n", sshTarget, remotePath)
	}
	rc := exec.CommandContext(ctx, "rsync", rsyncArgs...)
	rc.Dir = workdir
	rc.Env = os.Environ()
	if out, err := rc.CombinedOutput(); err != nil {
		return fmt.Errorf("rsync to %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}

	// FIX-CLI-3: sync hasura/ (metadata YAML tree) if the project has one, so
	// the remote 'nself db hasura metadata apply' invoked below has something
	// to apply. Best-effort: a project with no hasura/ dir simply skips this —
	// matches ApplyIfPresent's clean-skip behavior in the local start path.
	if info, statErr := os.Stat(filepath.Join(workdir, "hasura")); statErr == nil && info.IsDir() {
		if !jsonOut {
			fmt.Printf("  [running] rsync hasura/ metadata to %s:%s\n", sshTarget, remotePath)
		}
		hasuraRsync := exec.CommandContext(ctx, "rsync",
			"-az", "--delete",
			"-e", fmt.Sprintf("ssh -i %s -o StrictHostKeyChecking=accept-new -o ForwardAgent=no", sshKey),
			"hasura/",
			fmt.Sprintf("%s:%s/hasura/", sshTarget, remotePath),
		)
		hasuraRsync.Dir = workdir
		hasuraRsync.Env = os.Environ()
		if out, err := hasuraRsync.CombinedOutput(); err != nil {
			return fmt.Errorf("rsync hasura/ to %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
		}
	}
	// Rename the pushed snapshot to the expected .env.<target> name on the
	// remote (rsync above pushes it under its resolvedEnvPath basename).
	renameCmd := fmt.Sprintf("cd %s && mv %s .env.%s", remotePath, filepath.Base(resolvedEnvPath), target)
	rn := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no",
		sshTarget, renameCmd)
	rn.Env = os.Environ()
	if out, err := rn.CombinedOutput(); err != nil {
		return fmt.Errorf("finalizing resolved .env on %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}

	// Pull new images on the remote.
	sshPull := fmt.Sprintf("cd %s && docker compose pull", remotePath)
	if !jsonOut {
		fmt.Printf("  [running] docker compose pull on %s\n", sshTarget)
	}
	pc := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no",
		sshTarget, sshPull)
	pc.Env = os.Environ()
	if out, err := pc.CombinedOutput(); err != nil {
		return fmt.Errorf("remote pull on %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}

	// Discover which services exist in the pushed compose so the rolling order
	// only restarts real services (e.g. generated composes name object storage
	// "minio", not "storage" — restarting a nonexistent service aborts deploys).
	// This must succeed: falling back to a fixed guess list when it fails is
	// exactly the production incident (deploy_service_order.go) this guards
	// against, just on the remote path instead of the local one.
	lsCmd := fmt.Sprintf("cd %s && docker compose config --services", remotePath)
	lc := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no",
		sshTarget, lsCmd)
	lc.Env = os.Environ()
	out, err := lc.CombinedOutput()
	if err != nil {
		return fmt.Errorf("remote rolling restart: listing services on %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}
	var remotePresent []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			remotePresent = append(remotePresent, s)
		}
	}

	// Order: core services first (only those present), then every other
	// present service in the order docker reported them. Plugin discovery
	// is local-only (deploy_service_order.go), so it does not apply here —
	// the remote host's plugin fragments aren't visible to this process.
	order := resolveServiceOrder(remotePresent, nil)

	// Rolling restart on the remote: sequence the services via SSH.
	for _, svc := range order {
		restartCmd := fmt.Sprintf("cd %s && docker compose up -d --no-deps %s", remotePath, svc)
		if !jsonOut {
			fmt.Printf("  [running] Rolling restart: %s on %s\n", svc, sshTarget)
		}
		sc := exec.CommandContext(ctx, "ssh",
			"-i", sshKey,
			"-o", "StrictHostKeyChecking=accept-new",
			"-o", "ForwardAgent=no",
			sshTarget, restartCmd)
		sc.Env = os.Environ()
		if out, err := sc.CombinedOutput(); err != nil {
			return fmt.Errorf("remote rolling restart of %s failed: %w\n%s\nRun 'nself logs %s' on the remote host for details", svc, err, strings.TrimSpace(string(out)), svc)
		}
	}

	// FIX-CLI-3: apply Hasura metadata on the remote once hasura is back up.
	// Re-invokes the remote box's own 'nself db hasura metadata apply' (which
	// cleanly no-ops when hasura/ isn't present) rather than speaking the
	// Hasura HTTP API over the tunnel — same "SSH in, run the same
	// subcommand" pattern as runRemoteNselfCommand (db_remote.go). A short
	// retry loop absorbs the gap between "container up" (no health gate on
	// this remote path, unlike the local rolling restart) and "API ready".
	applyCmd := fmt.Sprintf("cd %s && for i in 1 2 3 4 5; do nself db hasura metadata apply && exit 0; sleep 3; done; exit 1", remotePath)
	if !jsonOut {
		fmt.Printf("  [running] hasura metadata apply on %s\n", sshTarget)
	}
	ac := exec.CommandContext(ctx, "ssh",
		"-i", sshKey,
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no",
		sshTarget, applyCmd)
	ac.Env = os.Environ()
	if out, err := ac.CombinedOutput(); err != nil {
		// Mirrors hasura.IsStrict's default (strict in staging/prod, warn in
		// dev/local) — this path has no *config.Config to read cfg.Env from,
		// so it keys off the deploy target string directly instead.
		strict := target == "staging" || target == "prod"
		if raw := os.Getenv("NSELF_HASURA_METADATA_STRICT"); raw != "" {
			strict = raw == "true"
		}
		msg := fmt.Sprintf("remote hasura metadata apply failed: %v\n%s", err, strings.TrimSpace(string(out)))
		if strict {
			return fmt.Errorf("%s", msg)
		}
		fmt.Printf("  [warn] %s (non-fatal; set NSELF_HASURA_METADATA_STRICT=true to fail deploys on this)\n", msg)
	}

	return nil
}

// ── subcommands ──────────────────────────────────────────────────────────────
