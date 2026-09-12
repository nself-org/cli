package runner

// Purpose: audit one runner host against the declarative Manifest and
//   report exactly what's missing or drifted, so "same GitHub Actions
//   labels, different tools installed" is caught by `nself runner verify`
//   before it causes a mystery job failure, not after.
// Inputs:  a Manifest and an Executor bound to the host under test.
// Outputs: []CheckResult, one per manifest dependency plus the work-dir
//   symlink check and the chromium ldd check (chromium.go).
// Constraints: every check is one Executor.Run call with a deterministic,
//   easily-mocked command string — see verify_test.go for the fake
//   Executor these are asserted against.

import (
	"context"
	"fmt"
	"strings"
)

// CheckStatus is the outcome of a single verify check.
type CheckStatus string

const (
	// StatusPass means the check found what it expected.
	StatusPass CheckStatus = "pass"
	// StatusFail means the check found the dependency/condition missing or
	// broken — this is what should block a job from being trusted on this
	// host.
	StatusFail CheckStatus = "fail"
	// StatusWarn is informational: the check couldn't run to completion in
	// a meaningful way (e.g. no cached Chromium yet) but that's not itself
	// evidence of drift.
	StatusWarn CheckStatus = "warn"
)

// CheckResult is one named check's outcome on one host.
type CheckResult struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

// VerifyHost runs every manifest-declared check against one host and
// returns the full result set. It never returns an error itself — a host
// that is entirely unreachable should be detected by the caller with a
// cheap reachability probe first (see VerifyHosts in report.go); if it
// isn't, every check below simply reports StatusFail with the underlying
// exec error as Detail, which is still an honest (if noisier) answer.
func VerifyHost(ctx context.Context, ex Executor, m *Manifest) []CheckResult {
	results := make([]CheckResult, 0, len(m.Dependencies)+2)
	for _, d := range m.Dependencies {
		results = append(results, checkDependency(ctx, ex, d))
	}
	results = append(results, checkWorkDirNotSymlink(ctx, ex, m.WorkDir))
	results = append(results, checkChromiumLdd(ctx, ex, m)...)
	return results
}

// checkDependency runs the binary-on-PATH check when the dependency
// declares one, otherwise falls back to a package-manager query — see
// Dependency.HasBinary's doc comment in manifest.go for why.
func checkDependency(ctx context.Context, ex Executor, d Dependency) CheckResult {
	name := "dep:" + d.Name
	if d.HasBinary() {
		out, err := ex.Run(ctx, "command -v "+shellQuote(d.Binary))
		if err != nil || out == "" {
			return CheckResult{Name: name, Status: StatusFail,
				Detail: fmt.Sprintf("binary %q not on PATH — %s", d.Binary, d.Reason)}
		}
		return CheckResult{Name: name, Status: StatusPass, Detail: out}
	}
	out, err := ex.Run(ctx, "dpkg -s "+shellQuote(d.AptPackage)+
		" >/dev/null 2>&1 && echo installed || echo missing")
	if err != nil || !strings.Contains(out, "installed") {
		return CheckResult{Name: name, Status: StatusFail,
			Detail: fmt.Sprintf("apt package %q not installed — %s", d.AptPackage, d.Reason)}
	}
	return CheckResult{Name: name, Status: StatusPass}
}

// checkWorkDirNotSymlink is the highest-severity check in this package.
//
// Git resolves symlinks before matching `includeIf.gitdir:`, so a runner
// whose `_work` directory is a symlink (e.g. onto a bind-mounted data
// volume set up by hand) silently defeats actions/checkout's credential
// injection: the job fails with "fatal: could not read Username for
// 'https://github.com'" and nothing in that error points at the real
// cause. `_work` must always be a real directory or an actual bind mount
// (which `stat`/`readlink` both report as a plain directory, never a
// symlink) — never `ln -s`.
func checkWorkDirNotSymlink(ctx context.Context, ex Executor, workDir string) CheckResult {
	const name = "workdir:not-symlink"
	if workDir == "" {
		return CheckResult{Name: name, Status: StatusWarn, Detail: "no work_dir configured in manifest"}
	}
	q := shellQuote(workDir)
	cmd := "if [ -L " + q + " ]; then echo SYMLINK; elif [ -d " + q + " ]; then echo DIR; else echo ABSENT; fi"
	out, err := ex.Run(ctx, cmd)
	if err != nil {
		return CheckResult{Name: name, Status: StatusFail, Detail: "could not stat " + workDir + ": " + err.Error()}
	}
	switch strings.TrimSpace(out) {
	case "SYMLINK":
		return CheckResult{Name: name, Status: StatusFail, Detail: workDir +
			" is a symlink — git resolves symlinks before matching includeIf.gitdir, " +
			"which silently breaks actions/checkout credential injection. Replace it " +
			"with a real directory or a bind mount."}
	case "DIR":
		return CheckResult{Name: name, Status: StatusPass, Detail: workDir + " is a real directory"}
	default:
		return CheckResult{Name: name, Status: StatusWarn, Detail: workDir + " does not exist yet"}
	}
}

// shellQuote wraps s in single quotes for safe inclusion in a shell
// command, escaping embedded single quotes POSIX-style. Every value quoted
// with this in the runner package comes from the compiled-in manifest or a
// CLI flag, never from webhook/repo-controlled input.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
