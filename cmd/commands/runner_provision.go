package commands

// Purpose: `nself runner provision [--host user@host]` — install the
//   manifest's dependency set on a runner host, create the runner user
//   with passwordless sudo, and register N runner instances as systemd
//   services. See internal/runner/provision.go for the actual steps.
// Inputs:  --host (repeatable; local when omitted), --instances,
//   --install-root, --github-url, --labels, --token (or GITHUB_RUNNER_TOKEN
//   env), --ssh-key.
// Outputs: each provisioning step's name + captured output printed to
//   stdout; non-zero exit on the first failing step.
// Constraints: the registration token is read from an env var by default
//   and never logged or echoed back — see runProvision below.

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/cli/internal/runner"
	"github.com/spf13/cobra"
)

var runnerProvisionCmd = &cobra.Command{
	Use:   "provision",
	Short: "Install runner dependencies, user, sudoers, and N runner instances",
	Long: `Install the declarative dependency set (internal/runner/manifest.yaml),
create the runner service user with passwordless sudo, ensure the shared
work directory is a real directory (never a symlink — a symlinked _work
silently breaks actions/checkout's credential injection), and register
--instances runner instances as systemd services.

Every step is idempotent: safe to re-run against a host that's already
partially provisioned.

Examples:
  nself runner provision --github-url https://github.com/nself-org/cli
  nself runner provision --host ci@167.235.x.x --instances 2 --labels nself-ci
  GITHUB_RUNNER_TOKEN=... nself runner provision --github-url https://github.com/nself-org/web`,
	RunE: runRunnerProvision,
}

func init() {
	runnerProvisionCmd.Flags().StringSlice("host", nil,
		"SSH target(s) user@host to provision (repeatable). Default: this machine.")
	runnerProvisionCmd.Flags().String("ssh-key", "", "SSH private key path (default: NSELF_DEPLOY_KEY_PATH or ~/.ssh/id_ed25519)")
	runnerProvisionCmd.Flags().Int("instances", 1, "Number of runner instances to install on this host")
	runnerProvisionCmd.Flags().String("install-root", "", "Base install directory (default: /opt/actions-runner)")
	runnerProvisionCmd.Flags().String("github-url", "", "Repo or org URL runners register against, e.g. https://github.com/nself-org/cli")
	runnerProvisionCmd.Flags().StringSlice("labels", nil, "Extra labels appended after self-hosted,Linux,X64")
	runnerProvisionCmd.Flags().String("token", "", "GitHub Actions runner registration token (overrides GITHUB_RUNNER_TOKEN env). Never logged.")
}

func runRunnerProvision(cmd *cobra.Command, args []string) error {
	hosts, _ := cmd.Flags().GetStringSlice("host")
	sshKey, _ := cmd.Flags().GetString("ssh-key")
	instances, _ := cmd.Flags().GetInt("instances")
	installRoot, _ := cmd.Flags().GetString("install-root")
	githubURL, _ := cmd.Flags().GetString("github-url")
	labels, _ := cmd.Flags().GetStringSlice("labels")
	token, _ := cmd.Flags().GetString("token")

	if token == "" {
		token = os.Getenv("GITHUB_RUNNER_TOKEN")
	}
	if githubURL == "" {
		return fmt.Errorf("--github-url is required (the repo or org runners register against)")
	}
	if token == "" {
		return fmt.Errorf("a runner registration token is required: pass --token or set GITHUB_RUNNER_TOKEN")
	}
	if len(hosts) > 1 {
		return fmt.Errorf("provision takes at most one --host per invocation; run it once per host")
	}

	m, err := runner.LoadEmbeddedManifest()
	if err != nil {
		return err
	}

	executors := runnerExecutorsFromFlags(hosts, sshKey)
	ex := executors[0]

	opts := runner.ProvisionOptions{
		Instances:   instances,
		InstallRoot: installRoot,
		GithubURL:   githubURL,
		RegToken:    token,
		Labels:      labels,
	}

	fmt.Printf("Provisioning %s (%d instance(s))...\n", ex.Label(), opts.Instances)
	result, err := runner.Provision(context.Background(), ex, m, opts)
	for _, step := range result.Steps {
		fmt.Printf("  [%s] %s\n", step.Name, step.Output)
	}
	if err != nil {
		return err
	}
	fmt.Println("Provision complete.")
	return nil
}
