package main

// Port of scripts/release/lib/common.sh (pull requests): open, merge through
// admin-merge.sh with the owner's terminal, and the reuse-or-open flow.

import (
	"path/filepath"
	"strings"
)

// openPR is open_pr: push the branch (or reuse an identical remote one) and
// open the PR. It returns the PR number.
func (r *Release) openPR(s, id, repo, br, title, body string) (string, error) {
	if _, err := r.git(repo, "ls-remote", "--exit-code", "--heads", "origin", br); err == nil {
		r.git(repo, "fetch", "-q", "origin", br)
		if _, err := r.git(repo, "diff", "--quiet", "FETCH_HEAD", "HEAD"); err != nil {
			return "", r.die(s, id, "branch "+br+" already exists on "+repo+" with different content; delete it: gh api -X DELETE repos/nself-org/"+repo+"/git/refs/heads/"+br)
		}
	} else if _, err := r.git(repo, "push", "-q", "origin", br); err != nil {
		return "", r.die(s, id, "push of "+br+" to "+repo+" was refused")
	}
	url, err := r.Run.Run("gh", "pr", "create", "-R", "nself-org/"+repo, "--base", "main", "--head", br, "--title", title, "--body", body)
	if err != nil {
		return "", r.die(s, id, "gh pr create failed on "+repo)
	}
	url = strings.TrimRight(url, "\n")
	return url[strings.LastIndex(url, "/")+1:], nil
}

// mergePR is merge_pr: green checks, then admin-merge.sh on the owner's
// terminal. No pre-answer; --yes never applies here.
func (r *Release) mergePR(s, id, repo, n string) error {
	if ok, err := r.prMerged(repo, n); err != nil || ok {
		return err
	}
	if err := r.WaitChecks(s, id, repo, n); err != nil {
		return err
	}
	r.say("%s %s: handing the terminal to admin-merge.sh for %s#%s (answer its prompt)", s, id, repo, n)
	if _, err := r.runIn(filepath.Join(r.Work, repo), true, "bash", r.AdminMerge, "--repo", "nself-org/"+repo, "--pr", n); err != nil {
		return r.die(s, id, "admin-merge.sh failed on "+repo+"#"+n+" (check branch protection: it restores itself; see its audit log)")
	}
	ok, err := r.prMerged(repo, n)
	if err != nil {
		return err
	}
	if !ok {
		return r.die(s, id, repo+"#"+n+" is not merged after admin-merge.sh (declined at its prompt?)")
	}
	return nil
}

// prFlow is pr_flow: reuse an open PR or open one from the commit already
// made; then merge it.
func (r *Release) prFlow(s, id, repo, br, key, title, body string) error {
	if err := r.prInfo(repo, br); err != nil {
		return err
	}
	var n string
	if r.PrInfo != "" {
		n = r.prNumber()
		r.say("%s %s: reusing %s#%s (%s)", s, id, repo, n, r.prState())
	} else {
		var err error
		if n, err = r.openPR(s, id, repo, br, title, body); err != nil {
			return err
		}
		r.say("%s %s: opened %s#%s", s, id, repo, n)
	}
	if err := r.stateSet(key, n); err != nil {
		return err
	}
	return r.mergePR(s, id, repo, n)
}

// stalePR is stale_pr: a PR on the branch that cannot be reused.
func (r *Release) stalePR(s, id, repo, br string) error {
	del := "delete branch " + br + " (gh api -X DELETE repos/nself-org/" + repo + "/git/refs/heads/" + br + ")"
	switch {
	case strings.HasSuffix(r.PrInfo, "CLOSED"):
		return r.die(s, id, "PR "+r.prNumber()+" on "+repo+" was closed unmerged; "+del+" and retry")
	case strings.HasSuffix(r.PrInfo, "MERGED"):
		return r.die(s, id, "PR "+r.prNumber()+" on "+repo+" is merged but main does not carry v"+r.Version+" (the bump was reverted or main was reset); "+del+" and re-run to open a fresh PR")
	}
	return nil
}
