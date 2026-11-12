package commands

// Purpose: remote deploy helpers, loading the per-target env cascade, resolving
// the SSH key path, and pushing a build to a remote host over rsync/ssh. Inputs
// are the workdir, target name, and host; outputs are loaded env vars or an error.
// Constraints: split out of deploy.go (CLI-R12) as a pure move, no behavior change.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// loadDeployEnvCascade loads target's env cascade for build and deploy.
func loadDeployEnvCascade(workdir, target string) {
	for _, f := range deployEnvCascadeFiles(workdir, target) {
		if _, err := os.Stat(f); err == nil {
			_ = config.OverloadEnvFile(f) // skips protected security keys, as config.Load does
		}
	}
	_ = os.Setenv("NSELF_DEPLOY_ENV", target)
	_ = os.Setenv("ENV", deployCascadeEnv(workdir, target))
}

// deployCascadeEnv names the environment whose cascade a deploy of target
// uses: a prod-class environment always uses "prod" (whatever its spelling:
// production, PROD, a tier-prod name), anything else its own lowercased name.
func deployCascadeEnv(workdir, target string) string {
	inv, _ := controlplane.Load(workdir)
	if prodClassFn(inv, target) {
		return "prod"
	}
	return strings.ToLower(strings.TrimSpace(target))
}

// deployEnvCascadeFiles returns target's cascade as paths under workdir, from
// config.EnvCascadeOrder (the single owner of the order, minus .env.local for a
// remote target, honouring
// NSELF_LEGACY_ENV_ORDER exactly as config.Load does).
func deployEnvCascadeFiles(workdir, target string) []string {
	names := config.EnvCascadeOrder(deployCascadeEnv(workdir, target), config.LegacyOrderActive())
	if target != "local" { // a remote deploy never ships .env.local
		names = config.WithoutLocalOverride(names)
	}
	files := make([]string, 0, len(names))
	for _, n := range names {
		files = append(files, filepath.Join(workdir, n))
	}
	return files
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

// remoteDeployPushFn is the remote push. A variable so tests can record the
// host and environment a deploy reaches without running ssh or rsync.
var remoteDeployPushFn = remoteDeployPushDispatch

// remoteDeployPush checks image architecture before rsyncing the compose and
// environment to the host, then pulls and restarts services over SSH.
func remoteDeployPush(ctx context.Context, workdir, host, target string, jsonOut bool) error {
	sshKey := sshKeyPath()
	if abs, absErr := filepath.Abs(workdir); absErr == nil {
		workdir = abs // rsync sources are absolute: the snapshot path is workdir-relative
	}

	// sshKey goes into the rsync ssh command string, which rsync passes
	// through a shell. Reject any key path containing shell metacharacters to
	// prevent command injection via NSELF_DEPLOY_SSH_KEY.
	if err := deploy.CheckKeyPath(sshKey); err != nil {
		return err
	}

	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return fmt.Errorf("NSELF_DEPLOY_HOST_%s: %w", strings.ToUpper(target), err)
	}
	inv, loadErr := controlplane.Load(workdir)
	if loadErr != nil {
		return loadErr
	}
	loadedInv := inv
	inv = deployPreflightInventory(loadedInv, target, spec.String())
	files, err := controlplane.ComposeFilesForDeploy(filepath.Join(workdir, "docker-compose.yml"))
	if err != nil {
		return err
	}
	// The merged .env is shipped below: the host key is checked with the shipping policy
	// (strict in v1.4 too, E487) before the preflight architecture read and before any rsync.
	policy, err := legacyHostKeyPolicy(ctx, loadedInv, target, spec)
	if err != nil {
		return err
	}
	if err := controlplane.PreflightShipping(ctx, inv, target, files); err != nil {
		return err
	}
	sshTarget := spec.Dest()
	if strings.Contains(spec.Host, ":") {
		sshTarget = spec.String()
		if spec.Port != 0 {
			sshTarget = strings.TrimSuffix(sshTarget, ":"+fmt.Sprint(spec.Port))
		}
	}
	remotePath, err := legacyRemotePath(spec.LegacyPath, target)
	if err != nil {
		return err
	}
	tr, err := newPushRunner(workdir, spec, sshKey, policy)
	if err != nil {
		return err
	}

	// Gap #13: push one merged snapshot of every value config.Load resolved
	// (CLI-R18 order .env -> .env.<target> -> .env.secrets -> .env.local) as
	// .env.<target>, so the host env matches the pushed compose file's provenance.
	resolvedEnvPath, cleanupResolvedEnv, err := writeResolvedDeployEnv(workdir, target)
	if err != nil {
		return fmt.Errorf("preparing resolved .env for %s: %w", target, err)
	}
	defer cleanupResolvedEnv()

	// rsync compose + env files to the remote. Every ssh option is built by the deploy
	// transport (ForwardAgent=no is an ssh option, never an rsync one).
	if !jsonOut {
		fmt.Printf("  [running] rsync compose + resolved env to %s:%s\n", sshTarget, remotePath)
	}
	if out, err := tr.rsync(ctx, nil, remotePath+"/", "docker-compose.yml", resolvedEnvPath); err != nil {
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
		if out, err := tr.rsync(ctx, []string{"--delete"}, remotePath+"/hasura/", "hasura/"); err != nil {
			return fmt.Errorf("rsync hasura/ to %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
		}
	}
	// Rename the pushed snapshot to the expected .env.<target> name on the
	// remote (rsync above pushes it under its resolvedEnvPath basename).
	renameCmd := fmt.Sprintf("cd %s && mv %s .env.%s", remote.ShellQuote(remotePath), remote.ShellQuote(filepath.Base(resolvedEnvPath)), target)
	if out, err := tr.ssh(ctx, renameCmd); err != nil {
		return fmt.Errorf("finalizing resolved .env on %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}

	// Pull new images on the remote.
	sshPull := fmt.Sprintf("cd %s && docker compose pull", remote.ShellQuote(remotePath))
	if !jsonOut {
		fmt.Printf("  [running] docker compose pull on %s\n", sshTarget)
	}
	if out, err := tr.ssh(ctx, sshPull); err != nil {
		return fmt.Errorf("remote pull on %s failed: %w\n%s", sshTarget, err, strings.TrimSpace(string(out)))
	}

	// Discover which services exist in the pushed compose so the rolling order
	// only restarts real services (e.g. generated composes name object storage
	// "minio", not "storage" — restarting a nonexistent service aborts deploys).
	// This must succeed: falling back to a fixed guess list when it fails is
	// exactly the production incident (deploy_service_order.go) this guards
	// against, just on the remote path instead of the local one.
	lsCmd := fmt.Sprintf("cd %s && docker compose config --services", remote.ShellQuote(remotePath))
	out, err := tr.ssh(ctx, lsCmd)
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
		restartCmd := fmt.Sprintf("cd %s && docker compose up -d --no-deps %s", remote.ShellQuote(remotePath), remote.ShellQuote(svc))
		if !jsonOut {
			fmt.Printf("  [running] Rolling restart: %s on %s\n", svc, sshTarget)
		}
		if out, err := tr.ssh(ctx, restartCmd); err != nil {
			return fmt.Errorf("remote rolling restart of %s failed: %w\n%s\nRun 'nself logs %s' on the remote host for details", svc, err, strings.TrimSpace(string(out)), svc)
		}
	}

	// FIX-CLI-3: apply Hasura metadata through the host's own nself (a no-op
	// without hasura/), retried briefly: this path has no per-service health gate.
	if !jsonOut {
		fmt.Printf("  [running] hasura metadata apply on %s\n", sshTarget)
	}
	if out, err := tr.ssh(ctx, deploy.HasuraMetadataApplyCommand(remotePath)); err != nil {
		// Mirrors hasura.IsStrict's default (strict in staging/prod, warn in
		// dev/local) — this path has no *config.Config to read cfg.Env from,
		// so it keys off the deploy target string directly instead.
		strict := remoteHasuraStrict(workdir, target)
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

// legacyHostKeyPolicy: the single-host path's strict (E487) host-key policy for the env's tier.
func legacyHostKeyPolicy(ctx context.Context, inv *controlplane.Inventory, target string, spec remote.HostSpec) ([]string, error) {
	tier := controlplane.DeriveTier(target, "remote")
	if e, ok := inv.Environments[target]; ok {
		tier = e.Tier
	}
	return controlplane.HostKeyOptions(ctx, target, target+"-app", tier, spec.String(), true)
}

// legacyRemotePath: the host spec's path, else NSELF_REMOTE_PATH_<ENV>, else /opt/nself.
func legacyRemotePath(rp, target string) (string, error) {
	if rp == "" {
		rp = os.Getenv("NSELF_REMOTE_PATH_" + strings.ToUpper(target))
	}
	if rp == "" {
		rp = "/opt/nself"
	}
	if !remotePathRe.MatchString(rp) {
		return "", fmt.Errorf("NSELF_DEPLOY_HOST_%s remote path contains unsafe characters (got %q): only [a-zA-Z0-9/_.-] allowed", strings.ToUpper(target), rp)
	}
	return rp, nil
}

// runRemoteDeployHealthCheck gates a single-host deploy on the host, never this machine:
// the v1.5 apply already gated each service (E250/E251); v1.4 runs nself doctor over the transport.
func runRemoteDeployHealthCheck(ctx context.Context, workdir, host, target string, jsonOut bool) (deployStep, error) {
	// compat.V15(P7-WIRE-21): single-host health ran nself doctor over the transport -> gated per service on the host by the apply
	if compat.V15() {
		announce(ctx, jsonOut, "Health checks", "  [done] Health checks (gated per service on the host by the apply)")
		return deployStep{Name: "Health checks", Status: "done"}, nil
	}
	announce(ctx, jsonOut, "Health checks", fmt.Sprintf("  [running] Health checks on %s (nself doctor)", host))
	failed := deployStep{Name: "Health checks", Status: "failed"}
	key := sshKeyPath()
	if err := deploy.CheckKeyPath(key); err != nil {
		return failed, err
	}
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return failed, fmt.Errorf("NSELF_DEPLOY_HOST_%s: %w", strings.ToUpper(target), err)
	}
	inv, err := controlplane.Load(workdir)
	if err != nil {
		return failed, err
	}
	rp, err := legacyRemotePath(spec.LegacyPath, target)
	if err != nil {
		return failed, err
	}
	policy, err := legacyHostKeyPolicy(ctx, inv, target, spec)
	if err != nil {
		return failed, err
	}
	tr, err := newPushRunner(workdir, spec, key, policy)
	if err != nil {
		return failed, err
	}
	if out, err := tr.ssh(ctx, fmt.Sprintf("cd %s && nself doctor", remote.ShellQuote(rp))); err != nil {
		return failed, fmt.Errorf("health check on %s failed (service: %s): %w. Run 'nself doctor --verbose' on the host for details", spec.Dest(), health.DoctorFailedService(string(out)), err)
	}
	if !jsonOut {
		fmt.Printf("  [done] Health checks passed on %s\n", spec.Dest())
	}
	return deployStep{Name: "Health checks", Status: "done"}, nil
}

// ── subcommands ──────────────────────────────────────────────────────────────
