package commands

// Purpose: `nself runner verify [--host ...]` — the important half of
//   G-012. Checks one or more hosts against the manifest and prints a
//   parity matrix, so "same GitHub Actions labels, different tools
//   installed" (gh/zip/unzip present on one host, absent on another,
//   2026-09-11) is caught by a single command instead of a mystery job
//   failure days later.
// Inputs:  --host (repeatable; local when omitted), --ssh-key, --json.
// Outputs: a text parity matrix (default) or a JSON []runner.HostReport
//   (--json); exit 1 if any reachable host has a failing check or any
//   check drifts across hosts.
// Constraints: never contacts a host that isn't named by the caller via
//   --host or implied by "local" — no hardcoded host list.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/runner"
	"github.com/spf13/cobra"
)

var runnerVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Check host(s) against the dependency manifest and report drift",
	Long: `Check one or more hosts against internal/runner/manifest.yaml's declarative
dependency set: every required package/binary, whether the shared work
directory is a real directory (not a symlink), and whether any cached
Chromium resolves its shared libraries (the exact failure that hid for
hours as a misleading "Target page, context or browser has been closed"
on 2026-09-11).

Pass multiple --host flags to check several hosts in one run and print a
parity matrix — this is how two hosts advertising identical GitHub Actions
labels but different installed tools gets caught before it causes a job
to pass or fail depending on luck.

Examples:
  nself runner verify
  nself runner verify --host ci@167.235.x.x --host ci@167.233.x.x
  nself runner verify --host ci@167.235.x.x --json`,
	RunE: runRunnerVerify,
}

func init() {
	runnerVerifyCmd.Flags().StringSlice("host", nil,
		"SSH target(s) user@host to verify (repeatable). Default: this machine.")
	runnerVerifyCmd.Flags().String("ssh-key", "", "SSH private key path (default: NSELF_DEPLOY_KEY_PATH or ~/.ssh/id_ed25519)")
	runnerVerifyCmd.Flags().Bool("json", false, "Print raw JSON ([]runner.HostReport) instead of the text parity matrix")
}

func runRunnerVerify(cmd *cobra.Command, args []string) error {
	hosts, _ := cmd.Flags().GetStringSlice("host")
	sshKey, _ := cmd.Flags().GetString("ssh-key")
	asJSON, _ := cmd.Flags().GetBool("json")

	m, err := runner.LoadEmbeddedManifest()
	if err != nil {
		return err
	}

	executors := runnerExecutorsFromFlags(hosts, sshKey)
	reports := runner.VerifyHosts(context.Background(), executors, m)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(reports); err != nil {
			return err
		}
	} else {
		fmt.Print(runner.RenderMatrix(reports))
	}

	if runnerVerifyFoundProblems(reports) {
		// Output already written above — errs.Exit is silent by design
		// (internal/errs.ExitError.Silent), so main() prints nothing further.
		// Never call os.Exit directly here: main() is the only os.Exit
		// caller (internal/repoqa/os_exit_test.go enforces this).
		return errs.Exit(1)
	}
	return nil
}

// runnerVerifyFoundProblems reports true when any reachable host has a
// failing check, any host is entirely unreachable, or any check drifts
// across hosts — the three conditions that should make `nself runner
// verify` a usable CI gate, not just an FYI.
func runnerVerifyFoundProblems(reports []runner.HostReport) bool {
	for _, r := range reports {
		if r.Err != "" {
			return true
		}
		for _, c := range r.Checks {
			if c.Status == runner.StatusFail {
				return true
			}
		}
	}
	return len(runner.DetectDrift(reports)) > 0
}
