package commands

// Purpose: Pre-flight local/remote nself CLI version-drift check for
//          runRemoteNselfCommand (db_remote.go). Split out from
//          db_remote_exec.go to keep each file under the 300-line cap.
// Inputs:  A dbRemoteTarget (host/env/remote path), the ssh flag argv
//          (everything before the target host, e.g. "-i <key> -o ...") used
//          to reach it, and the subcommand about to be run remotely.
// Outputs: nil when local and remote nself versions match (or the local
//          build has no parseable semver, e.g. "dev"); otherwise a clear,
//          actionable error naming the host, env, and both versions.
// Constraints: Never blocks the real command silently — a probe failure or
//              unparseable remote version always surfaces as an explicit
//              error rather than being swallowed. --allow-version-drift
//              (handled by the caller) is the only way to skip this check.
// SPORT: cli/cmd/commands — see gap #16 (version drift).

import (
	"context"
	"fmt"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/version"
)

// semverPattern extracts the first dotted-triple version token from noisy
// output (ANSI-colored banners, "nself v0.9.9" prefixes, build suffixes,
// etc.) on both the local and remote sides.
var semverPattern = regexp.MustCompile(`\d+\.\d+\.\d+`)

// checkRemoteVersionDrift probes the remote host's nself CLI version and
// compares it against the local build's version before runRemoteNselfCommand
// trusts that host's output. Real incident: an older remote CLI (v1.0.9) ran
// a subcommand fine but printed garbage output (migrations misreported as
// "up.sql" with unstable pending counts) — that passed through silently
// because no proactive check existed. This closes that gap ahead of time
// instead of relying solely on wrapRemoteVersionDriftError's after-the-fact
// "command not found" sniffing.
//
// sshFlagArgs is the ssh flag argv only (e.g. "-i <key> -o BatchMode=yes
// ..."), matching the prefix runRemoteNselfCommand builds before it appends
// the target host and remote command — the probe reaches the same host the
// same way the real command will.
func checkRemoteVersionDrift(ctx context.Context, rt dbRemoteTarget, sshFlagArgs []string, subCmd string) error {
	localVersion := semverPattern.FindString(version.GetVersion())
	if localVersion == "" {
		// Dev builds (Version == "dev") carry no parseable semver — nothing
		// meaningful to compare against, so skip rather than block.
		return nil
	}

	spec, err := remote.ParseHostSpec(rt.SSHTarget)
	if err != nil {
		return err
	}
	probeArgs := append(append([]string{}, sshFlagArgs...), spec.SSHArgs()...)
	probeArgs = append(probeArgs, "nself --version")
	out, err := runSSHCaptured(ctx, probeArgs)
	if err != nil {
		return fmt.Errorf(
			"could not determine remote nself version on %s (env=%s): %w\nprobe output: %s\nrun 'ssh %s nself --version' manually to diagnose",
			rt.SSHTarget, rt.EnvName, err, out, rt.SSHTarget,
		)
	}

	remoteVersion := semverPattern.FindString(out)
	if remoteVersion == "" {
		return fmt.Errorf(
			"could not determine remote nself version on %s (env=%s): probe output had no parseable version\nprobe output: %s\nrun 'ssh %s nself --version' manually to diagnose",
			rt.SSHTarget, rt.EnvName, out, rt.SSHTarget,
		)
	}

	if remoteVersion == localVersion {
		return nil
	}

	return fmt.Errorf(
		"version drift: local nself is v%s but %s (env=%s) runs v%s — remote 'nself %s' output cannot be trusted across versions; upgrade the remote CLI to v%s (e.g. ssh %s 'nself update'), or pass --allow-version-drift to run anyway",
		localVersion, rt.SSHTarget, rt.EnvName, remoteVersion, subCmd, localVersion, rt.SSHTarget,
	)
}

// joinRemoteArgs mirrors runRemoteNselfCommand's "nself <sub cmd>" string
// used in drift error messages, without the shell-quoting needed to
// actually execute it remotely.
func joinRemoteArgs(args []string) string {
	return strings.Join(args, " ")
}
