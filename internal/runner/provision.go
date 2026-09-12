package runner

// Purpose: build a self-hosted GitHub Actions CI runner host from the same
//   declarative Manifest verify.go audits against, so a freshly provisioned
//   host and a "should be identical" existing host are checked with
//   literally the same dependency list — no separate shell script that can
//   drift from what verify expects.
// Inputs:  a Manifest, an Executor bound to the target host, and
//   ProvisionOptions (instance count, GitHub registration details).
// Outputs: a ProvisionResult recording each step's output, for the caller
//   to print or log; a non-nil error stops at the first failing step.
// Constraints: every step is idempotent — safe to re-run provision against
//   a host that's already partially set up — and every side-effecting
//   command runs through Executor so tests assert exact command strings
//   against a fake, never a real host.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ProvisionOptions parameterizes one provisioning run.
type ProvisionOptions struct {
	// Instances is how many runner instances to register on this host.
	// Defaults to 1.
	Instances int
	// InstallRoot is the base directory each instance installs into, as
	// InstallRoot/runner-<N>. Defaults to /opt/actions-runner.
	InstallRoot string
	// GithubURL is the repo or org URL runners register against, e.g.
	// https://github.com/nself-org/cli.
	GithubURL string
	// RegToken is a GitHub Actions runner registration token. Sourced from
	// an env var by the caller (cmd/commands/runner_provision.go) — never
	// hardcoded, never included in ProvisionResult.
	RegToken string
	// Labels are extra labels appended after the standard
	// self-hosted,Linux,X64 set.
	Labels []string
}

// ProvisionStep records one step's name and captured output.
type ProvisionStep struct {
	Name   string
	Output string
}

// ProvisionResult is the full record of a Provision run.
type ProvisionResult struct {
	Steps []ProvisionStep
}

// Provision installs the manifest's dependency set, creates the runner
// user with passwordless sudo, ensures the work directory is a real
// directory (never a symlink — see verify.go's checkWorkDirNotSymlink for
// why that matters), and installs opts.Instances runner instances as
// systemd services.
func Provision(ctx context.Context, ex Executor, m *Manifest, opts ProvisionOptions) (*ProvisionResult, error) {
	if opts.Instances < 1 {
		opts.Instances = 1
	}
	if opts.InstallRoot == "" {
		opts.InstallRoot = "/opt/actions-runner"
	}

	result := &ProvisionResult{}
	baseSteps := []struct {
		name string
		run  func() (string, error)
	}{
		{"ensure-gh-apt-repo", func() (string, error) { return ensureGHAptRepo(ctx, ex) }},
		{"install-packages", func() (string, error) { return ensurePackages(ctx, ex, m) }},
		{"create-runner-user", func() (string, error) { return ensureRunnerUser(ctx, ex, m.RunnerUser) }},
		{"grant-passwordless-sudo", func() (string, error) { return ensurePasswordlessSudo(ctx, ex, m.RunnerUser) }},
		{"ensure-work-dir", func() (string, error) { return ensureRealWorkDir(ctx, ex, m.WorkDir) }},
	}
	for _, s := range baseSteps {
		out, err := s.run()
		result.Steps = append(result.Steps, ProvisionStep{Name: s.name, Output: out})
		if err != nil {
			return result, fmt.Errorf("runner provision: step %q: %w", s.name, err)
		}
	}

	for i := 1; i <= opts.Instances; i++ {
		out, err := installRunnerInstance(ctx, ex, m, opts, i)
		stepName := "install-runner-" + strconv.Itoa(i)
		result.Steps = append(result.Steps, ProvisionStep{Name: stepName, Output: out})
		if err != nil {
			return result, fmt.Errorf("runner provision: instance %d: %w", i, err)
		}
	}
	return result, nil
}

// ensureGHAptRepo adds GitHub CLI's official apt repository only when `gh`
// isn't already resolvable — Ubuntu's own archive doesn't reliably carry a
// current `gh` package, which is exactly how one runner host had it and
// another didn't while advertising identical labels (2026-09-11 incident).
func ensureGHAptRepo(ctx context.Context, ex Executor) (string, error) {
	script := `if command -v gh >/dev/null 2>&1; then echo "gh already present"; else ` +
		`sudo mkdir -p -m 755 /etc/apt/keyrings && ` +
		`curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo tee /etc/apt/keyrings/githubcli-archive-keyring.gpg >/dev/null && ` +
		`sudo chmod go+r /etc/apt/keyrings/githubcli-archive-keyring.gpg && ` +
		`echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list >/dev/null && ` +
		`echo "gh apt repo installed"; fi`
	return ex.Run(ctx, script)
}

// ensurePackages installs every manifest dependency in one apt-get call.
// apt-get install is itself idempotent for already-installed packages, so
// no separate pre-check is needed here (unlike the sudoers/user/work-dir
// steps, which write host state directly).
func ensurePackages(ctx context.Context, ex Executor, m *Manifest) (string, error) {
	quoted := make([]string, len(m.Dependencies))
	for i, d := range m.Dependencies {
		quoted[i] = shellQuote(d.AptPackage)
	}
	script := "sudo DEBIAN_FRONTEND=noninteractive apt-get update -qq && " +
		"sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends " +
		strings.Join(quoted, " ")
	return ex.Run(ctx, script)
}

// ensureRunnerUser creates the runner service account and adds it to the
// docker group, but only if it doesn't already exist.
func ensureRunnerUser(ctx context.Context, ex Executor, user string) (string, error) {
	q := shellQuote(user)
	script := "if id -u " + q + " >/dev/null 2>&1; then echo 'user already exists'; " +
		"else sudo useradd -m -s /bin/bash " + q + " && sudo usermod -aG docker " + q +
		" && echo 'runner user created'; fi"
	return ex.Run(ctx, script)
}

// ensurePasswordlessSudo grants the runner user NOPASSWD sudo via a
// dedicated /etc/sudoers.d drop-in, validated with visudo -cf before it
// takes effect. This is broad (ALL commands) because the evidence for this
// gap was `playwright install --with-deps` needing an interactive sudo
// prompt with no terminal available — CI steps in general assume
// passwordless sudo on a self-hosted runner. Idempotent: only writes when
// the file is missing or its content differs from the expected line.
func ensurePasswordlessSudo(ctx context.Context, ex Executor, user string) (string, error) {
	sudoersFile := "/etc/sudoers.d/" + user + "-nself-runner"
	line := user + " ALL=(ALL) NOPASSWD:ALL"
	script := fmt.Sprintf(
		"if [ -f %[1]s ] && sudo grep -qxF %[2]s %[1]s; then echo 'passwordless sudo already configured'; "+
			"else echo %[2]s | sudo tee %[1]s >/dev/null && sudo chmod 0440 %[1]s && sudo visudo -cf %[1]s "+
			"&& echo 'passwordless sudo granted'; fi",
		shellQuote(sudoersFile), shellQuote(line))
	return ex.Run(ctx, script)
}

// ensureRealWorkDir creates the shared work directory, refusing outright
// if the path already exists as a symlink rather than silently replacing
// it — a symlink there may be pointing at real data, and this is the exact
// misconfiguration G-012 exists to prevent (see verify.go's
// checkWorkDirNotSymlink for the full explanation).
func ensureRealWorkDir(ctx context.Context, ex Executor, workDir string) (string, error) {
	if workDir == "" {
		return "no work_dir configured; skipped", nil
	}
	q := shellQuote(workDir)
	script := "if [ -L " + q + " ]; then echo 'REFUSING: " + workDir +
		" is a symlink - replace it with a real directory or bind mount first (G-012)' 1>&2; exit 1; " +
		"else sudo mkdir -p " + q + " && echo '" + workDir + " ready (real directory)'; fi"
	return ex.Run(ctx, script)
}

// installRunnerInstance downloads (if not already present), configures,
// and starts one GitHub Actions runner instance as a systemd service via
// the runner's own svc.sh, which is the supported way to register a
// self-hosted runner as a systemd unit — reimplementing that unit file
// would just have to track GitHub's own runner releases anyway.
func installRunnerInstance(ctx context.Context, ex Executor, m *Manifest, opts ProvisionOptions, idx int) (string, error) {
	instanceDir := opts.InstallRoot + "/runner-" + strconv.Itoa(idx)
	name := ex.Label() + "-" + strconv.Itoa(idx)
	labels := strings.Join(append([]string{"self-hosted", "Linux", "X64"}, opts.Labels...), ",")

	script := fmt.Sprintf(`set -e
sudo mkdir -p %[1]s && sudo chown %[2]s:%[2]s %[1]s
cd %[1]s
if [ ! -f config.sh ]; then
  ARCH=$(uname -m); case "$ARCH" in x86_64) RARCH=x64 ;; aarch64) RARCH=arm64 ;; *) RARCH=x64 ;; esac
  VER=$(curl -fsSL https://api.github.com/repos/actions/runner/releases/latest | grep tag_name | cut -d '"' -f4 | tr -d v)
  curl -fsSL -o actions-runner.tar.gz "https://github.com/actions/runner/releases/download/v${VER}/actions-runner-linux-${RARCH}-${VER}.tar.gz"
  tar xzf actions-runner.tar.gz
fi
sudo -u %[2]s ./config.sh --url %[3]s --token %[4]s --name %[5]s --labels %[6]s --work _work --unattended --replace
sudo ./svc.sh install %[2]s
sudo ./svc.sh start
`, shellQuote(instanceDir), shellQuote(m.RunnerUser), shellQuote(opts.GithubURL),
		shellQuote(opts.RegToken), shellQuote(name), shellQuote(labels))
	return ex.Run(ctx, script)
}
