package commands

// Purpose: Shared remote-targeting support for `nself db migrate {up,status}`
//          and `nself db hasura metadata apply`. These commands previously
//          only ever operated on the local docker daemon; there was no way
//          to point them at a deployed staging/prod box short of manual
//          `ssh` + `docker exec` (gap #9 in
//          ~/Sites/nself/.claude/planning/nself-cli-gaps-from-ntask-dogfood.md).
// Inputs:  --env {dev,staging,prod} flag (mirrors `nself deploy`'s --env
//          convention) and the same control-plane inventory / legacy
//          NSELF_DEPLOY_HOST_<TARGET> env vars `nself deploy` already reads.
// Outputs: A resolved dbRemoteTarget describing whether to run locally or via
//          SSH against a specific server, plus a helper that re-invokes this
//          same `nself` binary over SSH on the remote host.
// Constraints: Local/default behavior is byte-identical when --env is
//              omitted or resolves to "local"/"dev" — this is purely additive.
//              Remote dispatch always delegates to the *remote* nself binary
//              (via `nself db migrate ...` / `nself db hasura ...` over SSH)
//              rather than trying to speak docker-exec/psql/Hasura HTTP to a
//              remote daemon from the local process — this reuses the exact
//              "SSH in and run the same subcommand" pattern already proven
//              by `nself deploy health --server` (runDeployHealth).
// SPORT: cli/cmd/commands — see gap #9.

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"

	"github.com/spf13/cobra"
)

// dbRemoteEnvFlag is the flag name shared by every db subcommand that
// supports remote targeting. Mirrors deployCmd's "--env" flag (deploy.go).
const dbRemoteEnvFlag = "env"

// addDBRemoteFlags registers --env and --server on cmd, matching the flag
// names and help text style already used by deployCmd/deployHealthCmd.
func addDBRemoteFlags(cmd *cobra.Command) {
	cmd.Flags().String(dbRemoteEnvFlag, "", "Target environment: local|staging|prod (default: local). Requires NSELF_DEPLOY_HOST_<ENV> or .nself/control-plane.yaml")
	cmd.Flags().String("server", "", "Run against a specific server name from control-plane inventory (overrides --env's default server selection)")
	cmd.Flags().Bool("allow-version-drift", false, "skip the local/remote nself version match check for remote --env targets")
}

// dbRemoteTarget is the resolved destination for a db/hasura command.
type dbRemoteTarget struct {
	// Local is true when the command should run against the local docker
	// daemon exactly as before this change (default, back-compat path).
	Local bool

	// EnvName is the resolved environment name ("local", "staging", "prod",
	// or a custom control-plane environment name).
	EnvName string

	// SSHTarget is "user@host" for a remote target. Empty when Local.
	SSHTarget string

	// KeyPath is the SSH private key path. Empty when Local.
	KeyPath string

	// RemotePath is the absolute path to the nSelf project on the remote
	// host, used to `cd` before invoking the remote nself binary.
	RemotePath string

	// AllowVersionDrift, when true, skips checkRemoteVersionDrift's
	// pre-flight local/remote nself version match check (--allow-version-drift).
	AllowVersionDrift bool
}

// resolveDBRemoteTarget reads --env/--server from cmd and resolves them to a
// dbRemoteTarget using the same control-plane inventory (.nself/control-plane.yaml
// or synthesized NSELF_DEPLOY_HOST_<ENV> env vars) that `nself deploy` already
// uses. When --env is empty or resolves to "local", Local=true is returned
// and every existing caller keeps today's exact local behavior.
func resolveDBRemoteTarget(cmd *cobra.Command) (dbRemoteTarget, error) {
	envFlag, _ := cmd.Flags().GetString(dbRemoteEnvFlag)
	serverFlag, _ := cmd.Flags().GetString("server")

	if envFlag == "" && serverFlag == "" {
		return dbRemoteTarget{Local: true, EnvName: "local"}, nil
	}

	envName := envFlag
	if envName == "" {
		envName = "local"
	}
	if canonical, ok := deployTargets[strings.ToLower(strings.TrimSpace(envName))]; ok {
		envName = canonical
	}

	if envName == "local" && serverFlag == "" {
		return dbRemoteTarget{Local: true, EnvName: "local"}, nil
	}

	workdir, err := projectRoot()
	if err != nil {
		return dbRemoteTarget{}, err
	}

	inv, err := controlplane.Load(workdir)
	if err != nil {
		return dbRemoteTarget{}, fmt.Errorf("resolve remote target: load inventory: %w", err)
	}

	env, ok := inv.Environments[envName]
	if !ok {
		return dbRemoteTarget{}, fmt.Errorf("resolve remote target: environment %q not found in control-plane inventory", envName)
	}

	srv, ok := findDBTargetServer(env, serverFlag)
	if !ok {
		if serverFlag != "" {
			return dbRemoteTarget{}, fmt.Errorf("resolve remote target: server %q not found in environment %q", serverFlag, envName)
		}
		return dbRemoteTarget{}, fmt.Errorf("resolve remote target: no servers configured for environment %q", envName)
	}

	if srv.Host == "" {
		// A server entry with no Host is local-only by convention (see
		// controlplane.Server doc comment) — run locally.
		return dbRemoteTarget{Local: true, EnvName: envName}, nil
	}

	remotePath := srv.RemotePath
	if remotePath == "" {
		remotePath = "/opt/nself"
	}

	return dbRemoteTarget{
		Local:      false,
		EnvName:    envName,
		SSHTarget:  srv.Host,
		KeyPath:    os.Getenv(srv.SSHKeyRef),
		RemotePath: remotePath,
	}, nil
}

// findDBTargetServer picks the server to target within env: the named
// server if serverFlag is set, otherwise the Primary server, otherwise the
// first server in the list.
func findDBTargetServer(env controlplane.Environment, serverFlag string) (controlplane.Server, bool) {
	if serverFlag != "" {
		for _, s := range env.Servers {
			if s.Name == serverFlag {
				return s, true
			}
		}
		return controlplane.Server{}, false
	}
	for _, s := range env.Servers {
		if s.Primary {
			return s, true
		}
	}
	if len(env.Servers) > 0 {
		return env.Servers[0], true
	}
	return controlplane.Server{}, false
}

// runRemoteNselfCommand re-invokes the given nself subcommand (e.g.
// []string{"db", "migrate", "up"}) on the remote target's own nself binary
// over SSH, cd'ing into rt.RemotePath first. Output streams directly to the
// caller's stdout/stderr.
//
// Delegating to the remote CLI binary (rather than trying to reach the
// remote docker daemon or Hasura HTTP endpoint directly from the local
// process) means the remote box always uses its own local resolution for
// project name / container name / Hasura port — the same values `nself
// build`/`nself start` used to stand up that box. It also means the
// remote-targeting logic here has no opinion about the remote CLI's
// internals, so it keeps working across CLI versions on either side.
//
// gap #16: if the remote nself binary is older and lacks a subcommand used
// here, ssh/bash reports "command not found" or cobra reports "unknown
// command" — wrapRemoteVersionDriftError turns that into an explicit,
// actionable message instead of a raw stack trace or bare exit status.
//
// Before running the real remote command, this also runs a proactive
// checkRemoteVersionDrift probe (unless rt.AllowVersionDrift is set) — a
// mismatched remote CLI can run a subcommand "successfully" while printing
// untrustworthy output (see checkRemoteVersionDrift's doc comment), which
// wrapRemoteVersionDriftError's after-the-fact "command not found" sniffing
// cannot catch.
func runRemoteNselfCommand(ctx context.Context, rt dbRemoteTarget, args ...string) error {
	keyPath := rt.KeyPath
	if keyPath == "" {
		keyPath = defaultSSHKeyPath()
	}

	sshArgs := []string{
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", "ForwardAgent=no",
		"-o", "StrictHostKeyChecking=accept-new",
	}

	if !rt.AllowVersionDrift {
		if driftErr := checkRemoteVersionDrift(ctx, rt, sshArgs, joinRemoteArgs(args)); driftErr != nil {
			return driftErr
		}
	}

	remoteCmd := fmt.Sprintf("cd %s && nself %s", shellQuoteArg(rt.RemotePath), strings.Join(shellQuoteArgs(args), " "))
	sshArgs = append(sshArgs, rt.SSHTarget, remoteCmd)

	out, err := runSSHCaptured(ctx, sshArgs)
	if out != "" {
		fmt.Println(out)
	}
	if err != nil {
		return wrapRemoteVersionDriftError(rt, args, err, out)
	}
	return nil
}

// shellQuoteArg single-quotes s for safe inclusion in a remote shell command,
// escaping embedded single quotes POSIX-style.
func shellQuoteArg(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellQuoteArgs applies shellQuoteArg to every element of args.
func shellQuoteArgs(args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = shellQuoteArg(a)
	}
	return out
}

// defaultSSHKeyPath mirrors sshKeyPath() in deploy.go for callers that don't
// have a per-server SSHKeyRef configured.
func defaultSSHKeyPath() string {
	return sshKeyPath()
}

// dispatchRemoteIfNeeded resolves --env/--server on cmd and, when they name a
// remote target, re-invokes remoteArgs (e.g. "db", "migrate", "up") on that
// target's own nself binary over SSH instead of running locally.
//
// Returns handled=true when the remote path was taken (whether it succeeded
// or failed) — callers should return immediately with the returned error in
// that case. Returns handled=false when the target resolved to local, in
// which case the caller proceeds with its original local implementation
// exactly as before this feature existed (default, back-compat path).
func dispatchRemoteIfNeeded(cmd *cobra.Command, remoteArgs ...string) (handled bool, err error) {
	// Commands invoked without the remote flags registered (e.g. directly in
	// unit tests via a minimal cobra.Command) have no --env/--server flags at
	// all; GetString returns an error we can safely ignore as "not remote".
	if cmd.Flags().Lookup(dbRemoteEnvFlag) == nil {
		return false, nil
	}

	target, resolveErr := resolveDBRemoteTarget(cmd)
	if resolveErr != nil {
		return true, resolveErr
	}
	if target.Local {
		return false, nil
	}

	if allowDrift, _ := cmd.Flags().GetBool("allow-version-drift"); allowDrift {
		// A remote CLI older than this one can ignore --dry-run and apply for
		// real; the drift probe is what catches it, so the two never combine.
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return true, fmt.Errorf("--allow-version-drift cannot be combined with --dry-run on a remote target: an older remote nself may ignore --dry-run and apply. Drop --allow-version-drift (the remote must run the same nself version)")
		}
		target.AllowVersionDrift = true
	}

	if !cmd.Flags().Changed("json") {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Running 'nself %s' on %s:%s (env=%s)...\n", strings.Join(remoteArgs, " "), target.SSHTarget, target.RemotePath, target.EnvName)
	}

	return true, runRemoteNselfCommand(cmd.Context(), target, remoteArgs...)
}
