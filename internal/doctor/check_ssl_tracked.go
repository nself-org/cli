package doctor

// check_ssl_tracked.go — SEC-GITIGNORE-01: certificate keys and env secrets stay out of git.
//
// Purpose: D-0459. `nself init` writes `/ssl/` and `.env.secrets` into the
// .gitignore of a new project only (D-0163), so older projects can still
// commit ssl/certificates/*/privkey.pem, and keys committed before that fix
// stay tracked. This check asks real git, not a .gitignore parser, so nested
// .gitignore files, negations, core.excludesFile and a .gitignore git refuses
// to read are all judged the way git judges them.
// Inputs: a context and the project directory; git on PATH.
// Outputs: one CheckResult in the security section: warn with the remediation
// when a probe path is not ignored or when ssl/ or .env.secrets is tracked.
// Constraints: read-only (git check-ignore, git ls-files). Passes as skipped
// only when git is missing or git says the project is not a repository; any
// other git failure (dubious ownership, a corrupt repo) warns, because a
// skip there would hide tracked keys.

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// sslGitBin finds the git binary; tests swap it for a stub.
var sslGitBin = func() (string, error) { return exec.LookPath("git") }

const sslTrackedCheckName = "SEC-GITIGNORE-01"

// sslIgnoreProbes are paths that must be ignored. They need not exist:
// check-ignore --no-index tests the rules, not the index.
var sslIgnoreProbes = []string{
	"ssl/privkey.pem",
	"ssl/certificates/x/privkey.pem",
	".env.secrets",
}

// sslTrackedSpecs are the pathspecs that must have no tracked files.
var sslTrackedSpecs = []string{"ssl/", ".env.secrets"}

// CheckSSLTracked reports whether certificate keys under ssl/ and
// .env.secrets are ignored by git and not already tracked (D-0459).
func CheckSSLTracked(ctx context.Context, projectDir string) CheckResult {
	res := CheckResult{Section: "security", Name: sslTrackedCheckName}
	gitBin, err := sslGitBin()
	if err != nil {
		res.Status, res.Message = "pass", "skipped: git not installed"
		return res
	}
	var stderr string
	git := func(args ...string) (string, int, error) {
		cmd := exec.CommandContext(ctx, gitBin, args...)
		cmd.Dir = projectDir
		out, err := cmd.Output()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			stderr = strings.TrimSpace(string(ee.Stderr))
			return string(out), ee.ExitCode(), nil
		}
		return string(out), 0, err
	}

	out, code, err := git("rev-parse", "--is-inside-work-tree")
	switch {
	case err == nil && code == 0 && strings.TrimSpace(out) == "true":
	case err == nil && code != 0 && strings.Contains(stderr, "not a git repository"):
		res.Status, res.Message = "pass", "skipped: project is not in a git work tree"
		return res
	case err == nil && code == 0:
		res.Status, res.Message = "pass", "skipped: project is not in a git work tree (inside .git or a bare repo)"
		return res
	default:
		res.Status = "warn"
		res.Message = fmt.Sprintf("cannot tell whether ssl/ keys are tracked: git rev-parse failed (exit %d): %s", code, firstLine(stderr, err))
		return res
	}

	var problems []string
	var notIgnored []string
	for _, p := range sslIgnoreProbes {
		_, code, err := git("check-ignore", "-q", "--no-index", "--", p)
		switch {
		case err != nil || code > 1:
			problems = append(problems, fmt.Sprintf("git check-ignore %s failed (exit %d): %s", p, code, firstLine(stderr, err)))
		case code == 1:
			notIgnored = append(notIgnored, p)
		}
	}
	if len(notIgnored) > 0 {
		problems = append(problems, "not ignored by git: "+strings.Join(notIgnored, ", "))
	}

	out, code, err = git(append([]string{"ls-files", "-z", "--"}, sslTrackedSpecs...)...)
	var tracked []string
	if err != nil || code != 0 {
		problems = append(problems, fmt.Sprintf("git ls-files failed (exit %d): %s", code, firstLine(stderr, err)))
	} else {
		for _, f := range strings.Split(out, "\x00") {
			if f != "" {
				tracked = append(tracked, f)
			}
		}
	}
	if len(tracked) > 0 {
		shown := tracked
		if len(shown) > 3 {
			shown = shown[:3]
		}
		problems = append(problems, fmt.Sprintf("%d tracked file(s) under ssl/ or .env.secrets: %s",
			len(tracked), strings.Join(shown, ", ")))
	}

	if len(problems) == 0 {
		res.Status, res.Message = "pass", "ssl/ and .env.secrets are ignored by git and not tracked"
		return res
	}
	res.Status = "warn"
	res.Message = strings.Join(problems, "; ") +
		". Fix: add /ssl/ and .env.secrets to .gitignore"
	if len(tracked) > 0 {
		res.Message += ", untrack them (git rm --cached -r ssl/ .env.secrets), and rotate every key or secret that was committed: it stays in git history"
		res.FixCmd = "git rm --cached -r --ignore-unmatch -- ssl/ .env.secrets"
	}
	return res
}

// firstLine returns the first line of git's stderr, or err's text when git
// did not run.
func firstLine(stderr string, err error) string {
	if err != nil {
		return err.Error()
	}
	line, _, _ := strings.Cut(stderr, "\n")
	return line
}
