package main

// Port of scripts/release/lib/tag.sh: S4, the tag-time denylist rules
// (DENY-003/004/005) and the annotated tag. DENY-001/002 need the build output
// and are re-checked by release.yml's Pre-Release Gate.

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const licJQ = `.[0] | if . == null then "none" elif .status != "completed" then "pending" else .conclusion end`

// licState is lic_state: Lic = pending | success | <conclusion> | none for the
// newest license-audit.yml run on main. release.sh also read the newest run on
// any branch, because pre-release-gate.yml did; the gate now runs
// `releasectl gate`, which reads main (rules.go), so that second pass is gone.
func (r *Release) licState() error {
	args := []string{"run", "list", "-R", "nself-org/cli", "--workflow", "license-audit.yml",
		"--branch", "main", "--limit", "1", "--json", "status,conclusion", "--jq", licJQ}
	out, err := r.ghNeed("the license-audit.yml runs", args...)
	r.Lic = out
	return err
}

// licWait is lic_wait: Lic after waiting (30 min at most) for an in-flight run.
func (r *Release) licWait() error {
	saved := r.Poll
	r.Poll = 30 * time.Second
	defer func() { r.Poll = saved }()
	if err := r.licState(); err != nil {
		return err
	}
	if r.Lic != "pending" {
		return nil
	}
	r.say("S4 tag: license-audit.yml (main) is still running; waiting up to 30 minutes")
	if _, err := r.poll("license-audit.yml run (main)", 60, func() (bool, error) {
		if err := r.licState(); err != nil {
			return false, err
		}
		return r.Lic != "pending", nil
	}); err != nil {
		return err
	}
	return r.licState()
}

func (r *Release) dieTag(msg string) error { return r.die("S4", "tag", msg) }

// denyChecks is deny_checks: DENY-003/004/005 for the commit about to be tagged.
// A tag list that cannot be read stops the step instead of passing DENY-003.
func (r *Release) denyChecks(sha string) error {
	out, err := r.git("cli", "tag", "--sort=-version:refname")
	if err != nil {
		return r.dieTag("cannot list the cli tags for DENY-003: " + err.Error())
	}
	prior := ""
	for _, t := range strings.Fields(out) {
		if t != r.Tag {
			prior = t
			break
		}
	}
	if prior != "" {
		cnt, err := r.git("cli", "rev-list", "--count", prior+".."+sha)
		if err != nil {
			return r.dieTag("cannot count commits since " + prior)
		}
		if n, _ := strconv.Atoi(strings.TrimSpace(cnt)); n <= 0 {
			return r.dieTag("DENY-003: no commits since " + prior + "; merge the version bump (S3) first")
		}
	}
	const rerun = "gh workflow run license-audit.yml -R nself-org/cli --ref main"
	if err := r.licWait(); err != nil {
		return err
	}
	switch r.Lic {
	case "success":
	case "pending":
		return r.dieTag("DENY-004: license-audit.yml on main is still running after 30 minutes; let it finish, then --from S4")
	case "none":
		return r.dieTag("DENY-004: no license-audit.yml run on main. Start one: " + rerun + ", wait for it to pass, then --from S4")
	default:
		return r.dieTag("DENY-004: the newest license-audit.yml run on main is '" + r.Lic + "'. Fix the flagged licenses, run: " + rerun + ", wait for it to pass, then --from S4")
	}
	return r.denyMilestone()
}

// denyMilestone is DENY-005: exactly one milestone titled TAG, and it is closed.
func (r *Release) denyMilestone() error {
	out, err := r.ghNeed("the cli milestones", "api", "repos/nself-org/cli/milestones?state=all&per_page=100", "--paginate",
		"--jq", `.[] | select(.title == "`+r.Tag+`") | .state`)
	if err != nil {
		return err
	}
	if out == "" {
		return r.dieTag("DENY-005: no closed milestone titled exactly " + r.Tag + " (a milestone named " + r.Version + " does not count); create or close it, then --from S4")
	}
	lines := strings.Split(out, "\n")
	for _, l := range lines {
		if l != "closed" {
			return r.dieTag("DENY-005: a milestone titled " + r.Tag + " is not closed (an open duplicate?); close or delete the duplicate so exactly one closed milestone has that title, then --from S4")
		}
	}
	if len(lines) != 1 {
		return r.dieTag("DENY-005: more than one milestone is titled " + r.Tag + "; pre-release-gate.yml needs exactly one (closed). Delete the duplicates, then --from S4")
	}
	return nil
}

func (r *Release) planTag() {
	w, v, t := r.Work, r.Version, r.Tag
	p := func(c string) { r.planLine("S4", "tag", "cli", c) }
	p("gh pr list -R nself-org/cli --head release/v" + v + " --state merged --json mergeCommit (the S3 merge commit)")
	p("git -C " + w + "/cli fetch --tags origin main; tag origin/main, or the merge commit when main has moved on but still contains it; its .github/VERSION must be " + v)
	p("git -C " + w + "/cli rev-list --count PRIOR_TAG..SHA (DENY-003: more than 0 commits since the prior tag)")
	p("gh run list -R nself-org/cli --workflow license-audit.yml --branch main --limit 1 (DENY-004: waits up to 30 min while running; success required)")
	p("gh api repos/nself-org/cli/milestones?state=all&per_page=100 --paginate (DENY-005: exactly one milestone titled " + t + ", and it is closed)")
	p("git -C " + w + "/cli tag -a " + t + " -m " + t + " SHA && git -C " + w + "/cli push origin refs/tags/" + t)
}

// StepTag is step_tag (S4).
func (r *Release) StepTag() error {
	if r.Plan {
		r.planTag()
		return nil
	}
	merged, err := r.ghNeed("the release PR", "pr", "list", "-R", "nself-org/cli", "--head", "release/v"+r.Version, "--state", "merged",
		"--json", "mergeCommit", "--jq", ".[0].mergeCommit.oid // empty")
	if err != nil {
		return err
	}
	found, err := r.repoTagSHA("cli", r.Tag)
	if err != nil {
		return err
	}
	if found {
		return r.checkExistingTag(merged)
	}
	if err := r.stepConfirm("S4", "tag"); err != nil {
		return err
	}
	if err := r.ensureClone("cli"); err != nil {
		return err
	}
	if err := r.resetClone("cli"); err != nil {
		return err
	}
	head, _ := r.git("cli", "rev-parse", "origin/main")
	sha := strings.TrimSpace(head)
	target := sha
	if merged != "" && merged != sha {
		if _, err := r.git("cli", "merge-base", "--is-ancestor", merged, "origin/main"); err != nil {
			return r.dieTag("the merged release commit " + merged + " is not in origin/main's history (rewritten or reverted); inspect before tagging")
		}
		r.say("S4 tag: origin/main (%s) moved on; tagging the reviewed release commit %s", sha, merged)
		target = merged
	}
	vf, err := r.git("cli", "show", target+":.github/VERSION")
	if err != nil {
		return r.dieTag("cannot read .github/VERSION at " + target)
	}
	if cur := squash(vf); cur != r.Version {
		return r.dieTag(target + " carries version " + cur + ", not " + r.Version + " (S3 not merged?)")
	}
	if err := r.denyChecks(target); err != nil {
		return err
	}
	return r.pushTag(target)
}

// checkExistingTag handles a tag that is already on the remote: it must sit on
// the release commit; a tag is never moved.
func (r *Release) checkExistingTag(merged string) error {
	sha, cur := r.TagSHA, merged
	if merged == "" {
		cur = sha
		vf, err := r.ghNeed("the VERSION file at "+r.Tag, "api", "-H", "Accept: application/vnd.github.raw",
			"repos/nself-org/cli/contents/.github/VERSION?ref="+r.Tag)
		if err != nil {
			return err
		}
		if squash(vf) != r.Version {
			return r.dieTag("tag " + r.Tag + " exists but its .github/VERSION is not " + r.Version + "; tags are never moved, inspect by hand")
		}
	}
	if sha != cur {
		return r.dieTag("tag " + r.Tag + " is at " + sha + " but the merged release commit is " + cur + "; tags are never moved, inspect by hand")
	}
	r.stepDone("S4", "tag")
	return nil
}

// pushTag creates the annotated tag on target, pushes it and checks the remote.
func (r *Release) pushTag(target string) error {
	local, _ := r.git("cli", "rev-parse", "-q", "--verify", "refs/tags/"+r.Tag+"^{commit}")
	if strings.TrimSpace(local) != target {
		if _, err := r.git("cli", "tag", "-a", r.Tag, "-m", r.Tag, target); err != nil {
			return r.dieTag("could not create the local tag (a stale " + r.Tag + " in " + filepath.Join(r.Work, "cli") + ", or git user.name / user.email unset?)")
		}
	}
	if _, err := r.git("cli", "push", "-q", "origin", "refs/tags/"+r.Tag); err != nil {
		return r.dieTag("push of " + r.Tag + " was refused")
	}
	out, _ := r.git("cli", "ls-remote", "origin", "refs/tags/"+r.Tag+"^{}")
	cur := ""
	if f := strings.Fields(out); len(f) > 0 {
		cur = f[0]
	}
	if cur != target {
		return r.dieTag("tag " + r.Tag + " on the remote is '" + cur + "', expected " + target)
	}
	if err := r.stateSet("tag_sha", target); err != nil {
		return err
	}
	r.say("S4 tag: %s -> %s (release.yml creates the release)", r.Tag, target)
	return nil
}
