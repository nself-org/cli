package commands

// Purpose: nself ci serve — self-hosted CI webhook listener daemon.
//   Receives GitHub push/pull_request webhooks, verifies HMAC-SHA256 signatures,
//   and runs the nself-ci gate in an ephemeral Docker container per job.
//   Replaces GitHub Actions compute while keeping GitHub as the SCM and status
//   target. Webhooks are free on GitHub; only Actions minutes are billed.
// Inputs:  flags (--addr, --secret, --concurrency, --workdir, --timeout)
// Outputs: HTTP server on :3845; /healthz; /; GitHub commit status posted per job
// Constraints: Requires gh CLI (OAuth), Docker daemon, nself-ci binary on PATH
//   or adjacent plugin source. SPORT: CLI-CMD-CI-SERVE-001
// SPORT: CLI-CMD-CI-SERVE-001

import (
	"os"
	"strings"

	nscicmd "github.com/nself-org/cli/internal/cmd/ci"
	"github.com/spf13/cobra"
)

var ciServeCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the nself-ci webhook listener daemon (port 3845)",
	Long: `Start an HTTP server that receives GitHub push and pull_request webhooks,
verifies the HMAC-SHA256 signature, and runs the nself-ci gate suite for each
event in an ephemeral Docker container.

GitHub webhooks are free — only Actions compute minutes are billed. This daemon
moves the compute to your own ops box while keeping GitHub as the SCM and
commit-status target.

The server posts a "nself-ci" GitHub commit status (pending → success/failure)
via gh OAuth, identical to the nself ci one-shot command.

Fail-closed defaults (T32): this daemon clones and executes attacker-
influenced repo content on inbound webhooks, so three things are required,
not optional, unless explicitly overridden:
  - a webhook secret must be configured (--secret / GITHUB_WEBHOOK_SECRET),
    or the server refuses to start. --insecure overrides this (NOT
    recommended: disables signature verification entirely).
  - the triggering repo must be in --allowed-repos / NSELF_CI_ALLOWED_REPOS,
    or the job is rejected with 403 before any clone happens. An empty
    allowlist rejects every job — there is no "allow all" override.
  - Docker must be available to sandbox the job, or the run refuses, unless
    --allow-unsandboxed is passed (NOT recommended: the cloned repo's gate
    commands then execute directly on this host with the daemon's own
    privileges).

Environment variables:
  GITHUB_WEBHOOK_SECRET     HMAC secret (same value configured in GitHub webhook settings)
  NSELF_CI_ALLOWED_REPOS    Comma-separated "owner/repo" allowlist (merged with --allowed-repos)
  NSELF_CI_ALLOW_UNSANDBOXED  "true" to allow running the gate outside Docker (same as --allow-unsandboxed)
  NSELF_CI_EVENT_SINK       Optional URL to POST completion events (JSON) to — seam for
                            the nself-cron-monitor plugin on port 3839.

Examples:
  nself ci serve --secret "$GITHUB_WEBHOOK_SECRET" --allowed-repos nself-org/cli
  nself ci serve --addr :3845 --concurrency 4 --allowed-repos nself-org/cli,nself-org/web
  nself ci serve --secret "$GITHUB_WEBHOOK_SECRET" --workdir /var/ci/workdirs --allowed-repos nself-org/cli`,
	RunE: func(cmd *cobra.Command, args []string) error {
		addr, _ := cmd.Flags().GetString("addr")
		secret, _ := cmd.Flags().GetString("secret")
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		workdir, _ := cmd.Flags().GetString("workdir")
		jobTimeout, _ := cmd.Flags().GetInt("timeout")
		verbose, _ := cmd.Flags().GetBool("verbose")
		insecure, _ := cmd.Flags().GetBool("insecure")
		allowUnsandboxed, _ := cmd.Flags().GetBool("allow-unsandboxed")
		allowedRepos, _ := cmd.Flags().GetStringSlice("allowed-repos")

		if !allowUnsandboxed && strings.EqualFold(os.Getenv("NSELF_CI_ALLOW_UNSANDBOXED"), "true") {
			allowUnsandboxed = true
		}
		allowedRepos = append(allowedRepos, splitAllowedReposEnv(os.Getenv("NSELF_CI_ALLOWED_REPOS"))...)

		return nscicmd.RunServe(nscicmd.ServeConfig{
			Addr:             addr,
			Secret:           secret,
			Concurrency:      concurrency,
			WorkDir:          workdir,
			JobTimeout:       jobTimeout,
			Verbose:          verbose,
			ResolveGate:      ensureCIBinary,
			Insecure:         insecure,
			AllowUnsandboxed: allowUnsandboxed,
			AllowedRepos:     allowedRepos,
		})
	},
}

// splitAllowedReposEnv parses a comma-separated NSELF_CI_ALLOWED_REPOS value
// into a slice, trimming whitespace and dropping empty entries.
func splitAllowedReposEnv(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func init() {
	ciServeCmd.Flags().String("addr", ":3845", "Listen address (host:port). Port 3845 = nself-ci-serve per F10 port registry")
	ciServeCmd.Flags().String("secret", "", "HMAC-SHA256 webhook secret (overrides GITHUB_WEBHOOK_SECRET env)")
	ciServeCmd.Flags().Int("concurrency", 2, "Max concurrent CI jobs")
	ciServeCmd.Flags().String("workdir", "/tmp/nself-ci-workdirs", "Base directory for ephemeral checkout workdirs")
	ciServeCmd.Flags().Int("timeout", 600, "Per-job timeout in seconds")
	ciServeCmd.Flags().BoolP("verbose", "v", false, "Verbose gate output")
	ciServeCmd.Flags().Bool("insecure", false, "DANGEROUS: start without a webhook secret (disables signature verification). Default: refuse to start when no secret is configured.")
	ciServeCmd.Flags().Bool("allow-unsandboxed", false, "DANGEROUS: run the gate directly on this host when Docker is unavailable, instead of refusing the job. Default: refuse.")
	ciServeCmd.Flags().StringSlice("allowed-repos", nil, "Comma-separated or repeatable \"owner/repo\" allowlist. Default: empty, which rejects every job (fail-closed, no \"allow all\").")

	ciCmd.AddCommand(ciServeCmd)
}
