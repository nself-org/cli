package commands

// Purpose: `nself deploy <env> --follow`: tail the container logs of the target
// after the deploy finished, until Ctrl-C (staging/prod only). Moved out of
// runDeployBody (deploy_run.go) for the 300-line cap (P7-SURF-15).
// Inputs: the run's cobra command, project and target, the finished run's
// summary values and the production-gate inventory.
// Outputs: the log tail on stdout (stderr in JSON mode); an error when the tail
// cannot start. Ctrl-C and a non-zero exit of the tail are not errors.
// Constraints: in v1.5 JSON mode the run document is already on stdout when the
// tail starts, so a failure to start it is reported on stderr and returned as a
// silent error: the exit status stays non-zero and stdout keeps exactly one
// document (no error envelope after a success document).

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/ui"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

func followDeployLogs(cmd *cobra.Command, workdir, target, strategy string, start time.Time, steps []deployStep, jsonOut bool, gateInv *controlplane.Inventory) error {
	if !jsonOut {
		ui.Info("Following container logs (Ctrl-C to stop)...")
	}
	host := legacyDeployHost(target)
	if host == "" {
		// Local host: stream logs directly.
		// Ctrl-C and a non-zero exit of the tail are not errors.
		_ = docker.Stream(cmd.Context(), workdir, []string{"compose", "logs", "-f", "--tail=50"}, os.Stdout, os.Stderr)
		return nil
	}
	// Remote follow: tail logs on the remote host via SSH.
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return afterDocument(jsonOut, finalizeFor(cmd.Context(), jsonOut, target, strategy, start, steps, err))
	}
	remotePath := spec.LegacyPath
	logsCmd := "docker compose logs -f --tail=50"
	if remotePath != "" {
		logsCmd = fmt.Sprintf("cd %s && docker compose logs -f --tail=50", remote.ShellQuote(remotePath))
	}
	sshArgs := []string{"-i", sshKeyPath(), "-o", "ForwardAgent=no"}
	tier := controlplane.TierLocalServers
	if prodClassFn(gateInv, target) {
		tier = controlplane.TierProd
	}
	keyOpts, err := controlplane.HostKeyOptions(cmd.Context(), target, "primary", tier, host, false)
	if err != nil {
		return afterDocument(jsonOut, err)
	}
	sshArgs = append(sshArgs, keyOpts...)
	sshArgs = append(sshArgs, spec.SSHArgs()...)
	sc := exec.CommandContext(cmd.Context(), "ssh", append(sshArgs, logsCmd)...)
	sc.Stdout = os.Stdout
	sc.Stderr = os.Stderr
	_ = sc.Run() // Ctrl-C exits; non-zero exit is not an error from user perspective.
	return nil
}

// afterDocument makes a failure that happens after the run document was
// printed silent in v1.5 JSON mode (see the file header); anywhere else the
// error is returned unchanged.
func afterDocument(jsonOut bool, err error) error {
	if err == nil || !jsonOut || !deployDocEnvelope() {
		return err
	}
	reportStreamError(err)
	return &streamedError{err: err}
}
