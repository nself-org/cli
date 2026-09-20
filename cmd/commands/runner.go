package commands

// Purpose: `nself runner` — build and audit self-hosted GitHub Actions CI
//   runner hosts (G-012). Closes the gap where runner hosts were hand-built
//   so required system dependencies were discovered only when a job failed,
//   and two hosts advertising identical GitHub Actions labels could
//   silently drift apart.
// Inputs:  none directly — see runner_provision.go and runner_verify.go for
//   each subcommand's flags.
// Outputs: registers `runner` (with `provision` and `verify` subcommands)
//   on RootCmd.
// Constraints: all host-affecting logic lives in internal/runner; this file
//   is wiring only, matching every other command group in cmd/commands.
// SPORT: CLI-CMD-RUNNER-001

import (
	"github.com/nself-org/cli/internal/runner"
	"github.com/spf13/cobra"
)

var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Provision and audit self-hosted GitHub Actions CI runner hosts",
	Long: `Provision and audit self-hosted GitHub Actions CI runner hosts.

Runner hosts were previously hand-built: required system dependencies (gh,
zip, unzip, Playwright/Chromium's shared libraries, ...) were discovered
only when a job failed mid-run, and two hosts advertising the identical
GitHub Actions labels (self-hosted,Linux,X64) could silently drift apart —
the same commit would pass or fail depending on which host claimed the job.

The dependency set is declarative (internal/runner/manifest.yaml, compiled
into this binary) so provision and verify always check the same list.

Subcommands:
  provision   Install dependencies, create the runner user, register N
              runner instances as systemd services
  verify      Check one or more hosts against the manifest and print a
              parity matrix — the important half: this is how "same
              labels, different tools" gets caught before it causes a
              mystery failure.`,
	// Without NoArgs an unknown subcommand falls through to this RunE,
	// prints the help text and exits 0 — a silent success a caller can
	// capture as real output (see env.go: the golden path curled the help
	// prose as a base URL). Bare `nself runner` still prints help.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	runnerCmd.AddCommand(runnerProvisionCmd)
	runnerCmd.AddCommand(runnerVerifyCmd)
	RootCmd.AddCommand(runnerCmd)
}

// runnerExecutorsFromFlags builds one runner.Executor per --host flag
// value, or a single runner.LocalExecutor when no --host is given. Shared
// by both subcommands so "no --host means check/act on this machine" and
// "user@host means SSH" behave identically for provision and verify.
func runnerExecutorsFromFlags(hosts []string, sshKey string) []runner.Executor {
	if len(hosts) == 0 {
		return []runner.Executor{runner.LocalExecutor{}}
	}
	executors := make([]runner.Executor, len(hosts))
	for i, h := range hosts {
		if h == "local" {
			executors[i] = runner.LocalExecutor{}
			continue
		}
		executors[i] = runner.NewSSHExecutor(h, sshKey)
	}
	return executors
}
