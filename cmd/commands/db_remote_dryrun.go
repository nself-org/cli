package commands

// Purpose: strict remote support check for a forwarded --dry-run (P7-PROD-84).
// Inputs: the resolved dbRemoteTarget, the ssh flag argv and the remote argv.
// Outputs: nil only when the remote's own `nself version --json` advertises
// the db-dry-run-safe capability; otherwise an error before any command that
// could apply is sent.
// Constraints: a remote older than the dry-run fixes applies on --dry-run (the
// released v1.4.12 does), and a version number cannot prove otherwise: source
// builds report the same 1.4.12. So the proof is a capability the remote
// states itself. A failed or timed-out probe, output that is not JSON, or a
// missing capability is a refusal, never a pass. --allow-version-drift never
// relaxes this. The probe runs `nself version --json`, which a pre-capability
// remote answers without the field (refused) or not at all (refused).
// SPORT: cli/cmd/commands — see P7-PROD-84, db_remote.go, internal/version.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/version"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// remoteProbeTimeout bounds the capability probe; a var so tests can shrink it.
var remoteProbeTimeout = 20 * time.Second

// hasDryRunArg reports whether the remote argv carries --dry-run.
func hasDryRunArg(args []string) bool {
	return slices.Contains(args, "--dry-run")
}

// parseRemoteCapabilities extracts the capabilities array from the probe's
// combined stdout+stderr. Noise around the JSON object (ssh banners, the
// dev-build warning) is tolerated; anything else is an error.
func parseRemoteCapabilities(out string) ([]string, error) {
	i, j := strings.Index(out, "{"), strings.LastIndex(out, "}")
	if i < 0 || j < i {
		return nil, fmt.Errorf("probe output is not JSON")
	}
	var v struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(out[i:j+1]), &v); err != nil {
		return nil, fmt.Errorf("probe output is not valid version JSON: %w", err)
	}
	return v.Capabilities, nil
}

// checkRemoteDryRunSupport refuses unless the remote advertises
// version.CapDBDryRunSafe.
func checkRemoteDryRunSupport(ctx context.Context, rt dbRemoteTarget, sshFlagArgs []string) error {
	spec, err := remote.ParseHostSpec(rt.SSHTarget)
	if err != nil {
		return err
	}
	pctx, cancel := context.WithTimeout(ctx, remoteProbeTimeout)
	defer cancel()
	probeArgs := append(append([]string{}, sshFlagArgs...), spec.SSHArgs()...)
	probeArgs = append(probeArgs, "nself version --json")
	out, err := runSSHCaptured(pctx, probeArgs)
	if err != nil {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): could not read the remote capabilities: %w\nprobe output: %s", rt.SSHTarget, rt.EnvName, err, out)
	}
	caps, err := parseRemoteCapabilities(out)
	if err != nil {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): %w\nprobe output: %s", rt.SSHTarget, rt.EnvName, err, out)
	}
	if !slices.Contains(caps, version.CapDBDryRunSafe) {
		return fmt.Errorf("refusing remote --dry-run on %s (env=%s): the remote nself does not advertise %q, so it may ignore --dry-run and apply; upgrade the remote to a release that includes it (--allow-version-drift does not apply to --dry-run)", rt.SSHTarget, rt.EnvName, version.CapDBDryRunSafe)
	}
	return nil
}
