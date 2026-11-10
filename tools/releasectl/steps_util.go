package main

// Port of scripts/release/lib/common.sh (the step helpers): plan rows, the
// "Run S<n>?" prompt, clones, branches, pull requests and the owner-answered
// merge. Every external command goes through the Runner.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// pollSeconds is POLL_SECONDS: the sleep between polls.
const pollSeconds = 20 * time.Second

// imageWaitPolls is IMAGE_WAIT_POLLS: 180 polls x 20 s = 60 minutes.
const imageWaitPolls = 180

// stepState holds the globals the shell steps leave behind for each other.
type stepState struct {
	PrInfo   string // "<n> <STATE>" of the last pr_info, or ""
	BuildSt  string // build_state result
	RelSt    string // release_state result
	Lic      string // lic_state result
	ImgS     string // image_step globals
	ImgID    string
	ImgRepo  string
	ImgHub   string
	ImgPre   string
	Pub      string
	AdminSHA string
}

// planLine is plan_line: one plan row `S<n> <id> <repo> <command>`.
func (r *Release) planLine(s, id, repo, cmd string) {
	r.say("%s %s %s %s", s, id, repo, cmd)
}

// stepDone is step_done.
func (r *Release) stepDone(s, id string) { r.say("%s %s done", s, id) }

// stepConfirm is step_confirm: release.sh's own prompt. --yes skips only it.
func (r *Release) stepConfirm(s, id string) error {
	if r.Yes {
		return nil
	}
	r.Out.Write([]byte("Run " + s + " " + id + "? [y/N] "))
	ans := ""
	if r.In != nil {
		ans, _ = r.In.ReadString('\n')
	}
	if a := strings.TrimSpace(ans); a == "y" || a == "Y" {
		return nil
	}
	return &FatalError{"stopped at " + s + " by the operator; resume with --from " + s}
}

func (r *Release) warn(format string, a ...any) {
	if r.Err != nil {
		r.Err.Write([]byte("release: " + fmt.Sprintf(format, a...) + "\n"))
	}
}

func (r *Release) git(repo string, args ...string) (string, error) {
	return r.Run.Run("git", append([]string{"-C", filepath.Join(r.Work, repo)}, args...)...)
}

// ensureClone is ensure_clone: a sibling clone under Work.
func (r *Release) ensureClone(repo string) error {
	d := filepath.Join(r.Work, repo)
	if fi, err := os.Stat(filepath.Join(d, ".git")); err == nil && fi.IsDir() {
		return nil
	}
	if _, err := os.Lstat(d); err == nil {
		return r.die(r.Step, r.ID, d+" exists but is not a git clone; remove it ("+d+"), then re-run")
	}
	if err := os.MkdirAll(r.Work, 0o755); err != nil {
		return err
	}
	if _, err := r.Run.Run("gh", "repo", "clone", "nself-org/"+repo, d, "--", "--quiet"); err != nil {
		return r.die(r.Step, r.ID, "clone of "+repo+" failed")
	}
	return nil
}

// resetClone is reset_clone: back to a clean origin/main.
func (r *Release) resetClone(repo string) error {
	steps := [][]string{{"fetch", "-q", "--tags", "origin", "main"}, {"checkout", "-q", "-f", "--detach", "origin/main"}, {"clean", "-fdq"}}
	for _, a := range steps {
		if _, err := r.git(repo, a...); err != nil {
			return r.die(r.Step, r.ID, "could not refresh the clone of "+repo+"; remove "+filepath.Join(r.Work, repo)+" and re-run")
		}
	}
	return nil
}

// checkoutBranch is checkout_branch: the clone on the remote copy of a branch.
func (r *Release) checkoutBranch(repo, br string) error {
	if err := r.ensureClone(repo); err != nil {
		return err
	}
	if _, err := r.git(repo, "ls-remote", "--exit-code", "--heads", "origin", br); err != nil {
		return r.die(r.Step, r.ID, "a PR exists on "+repo+" but branch "+br+" is gone; close the PR and re-run")
	}
	for _, a := range [][]string{{"fetch", "-q", "origin", br}, {"checkout", "-q", "-f", "-B", br, "FETCH_HEAD"}, {"clean", "-fdq"}} {
		if _, err := r.git(repo, a...); err != nil {
			return r.die(r.Step, r.ID, "could not check out "+br+" in "+filepath.Join(r.Work, repo))
		}
	}
	return nil
}

// commitAll is commit_all: (false, nil) when there is nothing to commit, and a
// non-nil error when git add or git commit itself failed.
func (r *Release) commitAll(repo, msg string) (bool, error) {
	if _, err := r.git(repo, "add", "-A"); err != nil {
		return false, fmt.Errorf("git add failed in %s: %w", repo, err)
	}
	if _, err := r.git(repo, "diff", "--cached", "--quiet"); err == nil {
		return false, nil
	}
	if _, err := r.git(repo, "commit", "-q", "-m", msg); err != nil {
		return false, fmt.Errorf("git commit failed in %s: %w", repo, err)
	}
	return true, nil
}

// bumpAll is bump_all: cli's own bump-version.sh edits cli and ../admin.
func (r *Release) bumpAll(extra ...string) error {
	args := append([]string{"scripts/bump-version.sh", r.Version}, extra...)
	_, err := r.runIn(filepath.Join(r.Work, "cli"), false, "bash", args...)
	return err
}

// prInfo is pr_info: PrInfo = "<n> <STATE>" for the branch, or "".
func (r *Release) prInfo(repo, br string) error {
	out, err := r.ghNeed("the PRs of "+repo, "pr", "list", "-R", "nself-org/"+repo, "--head", br, "--state", "all",
		"--json", "number,state", "--jq", `.[0] | select(.) | "\(.number) \(.state)"`)
	r.PrInfo = out
	return err
}

// prMerged is pr_merged.
func (r *Release) prMerged(repo, n string) (bool, error) {
	out, err := r.ghNeed("PR "+repo+"#"+n, "pr", "view", n, "-R", "nself-org/"+repo, "--json", "state", "--jq", ".state")
	return out == "MERGED", err
}

func (r *Release) prNumber() string { return strings.SplitN(r.PrInfo, " ", 2)[0] }

func (r *Release) prState() string {
	if i := strings.LastIndex(r.PrInfo, " "); i >= 0 {
		return r.PrInfo[i+1:]
	}
	return r.PrInfo
}
