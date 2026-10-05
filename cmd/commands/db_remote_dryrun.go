package commands

// Purpose: strict remote support check for a forwarded --dry-run (P7-PROD-84).
// Inputs: the resolved dbRemoteTarget, the ssh flag argv and the remote argv.
// Outputs: nil when the remote provably runs this build's version, else an
// error before any command that could apply is sent.
// Constraints: a remote CLI older than this fix can ignore --dry-run and apply
// for real, so a dry-run is only forwarded to a remote whose `nself --version`
// equals the local release. --allow-version-drift never relaxes this, and an
// unparseable local (dev build) or remote version is a refusal, not a pass.
// Equality is the proof because this binary is the one that has the fix.
// SPORT: cli/cmd/commands — see P7-PROD-84, db_remote.go, db_remote_version.go.

import (
	"context"
	"fmt"

	"github.com/nself-org/cli/internal/version"
)

// hasDryRunArg reports whether the remote argv carries --dry-run.
func hasDryRunArg(args []string) bool {
	for _, a := range args {
		if a == "--dry-run" {
			return true
		}
	}
	return false
}

// checkRemoteDryRunSupport probes `nself --version` on the remote and refuses
// unless it equals the local release version.
func checkRemoteDryRunSupport(ctx context.Context, rt dbRemoteTarget, sshFlagArgs []string) error {
	local := semverPattern.FindString(version.GetVersion())
	if local == "" {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): this build has no release version, so it cannot prove the remote supports --dry-run; run it from a release build", rt.SSHTarget, rt.EnvName)
	}
	probeArgs := append(append([]string{}, sshFlagArgs...), rt.SSHTarget, "nself --version")
	out, err := runSSHCaptured(ctx, probeArgs)
	if err != nil {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): could not read the remote nself version: %w\nprobe output: %s", rt.SSHTarget, rt.EnvName, err, out)
	}
	remote := semverPattern.FindString(out)
	if remote == "" {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): remote version probe had no parseable version\nprobe output: %s", rt.SSHTarget, rt.EnvName, out)
	}
	if remote != local {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): remote nself is v%s, local is v%s, and an older remote may ignore --dry-run and apply; upgrade the remote to v%s (--allow-version-drift does not apply to --dry-run)", rt.SSHTarget, rt.EnvName, remote, local, local)
	}
	return nil
}
