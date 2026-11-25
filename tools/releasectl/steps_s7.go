package main

// Port of scripts/release/lib/publish.sh: S7 release wait, the published-hash
// check and the web lock PR. The web PR is opened but never merged: the owner
// does that. GET-only except the web branch push and PR; no deploy.

import (
	"os"
	"path/filepath"
	"strings"
)

// readFile is os.ReadFile returning text.
func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}

// webPR is web_pr: pin the cli source in the web repo and open (not merge) a PR.
func (r *Release) webPR() error {
	const s, id = "S7", "release-wait"
	br := "release/cli-v" + r.Version
	if err := r.prInfo("web", br); err != nil {
		return err
	}
	if r.PrInfo != "" {
		r.say("S7 release-wait web done (PR #%s, %s)", r.prNumber(), r.prState())
		return nil
	}
	if !r.Have("node") {
		return r.die(s, id, "node is not installed (needed for update-lock.mjs)")
	}
	if err := r.stepConfirm(s, id); err != nil {
		return err
	}
	if err := r.ensureClone("web"); err != nil {
		return err
	}
	if err := r.resetClone("web"); err != nil {
		return err
	}
	if _, err := r.git("web", "checkout", "-q", "-B", br); err != nil {
		return r.die(s, id, "could not create branch "+br)
	}
	if _, err := r.runIn(filepath.Join(r.Work, "web"), false, "node", "org/scripts/sources/update-lock.mjs", "cli", r.Tag); err != nil {
		return r.die(s, id, "update-lock.mjs failed in the web clone")
	}
	title := "chore(sources): pin cli " + r.Tag
	changed, cerr := r.commitAll("web", title)
	if cerr != nil {
		return r.die(s, id, cerr.Error())
	}
	if !changed {
		r.say("S7 release-wait web done (lock already pins %s)", r.Tag)
		return nil
	}
	if _, err := r.git("web", "push", "-q", "origin", br); err != nil {
		return r.die(s, id, "push of "+br+" to web was refused")
	}
	url, err := r.Run.Run("gh", "pr", "create", "-R", "nself-org/web", "--base", "main", "--head", br, "--title", title,
		"--body", "Pins the public cli source to "+r.Tag+" (update-lock.mjs). The owner merges; no deploy.")
	if err != nil {
		return r.die(s, id, "gh pr create failed on web")
	}
	r.say("S7 release-wait web: opened %s (owner merges; no deploy)", strings.TrimRight(url, "\n"))
	return nil
}

// failedJobs is failed_jobs: the names of the failed jobs of the run, or "?".
func (r *Release) failedJobs() string {
	g := r.ghTry("run", "view", r.RunID, "-R", "nself-org/cli", "--json", "jobs",
		"--jq", `[.jobs[] | select(.conclusion == "failure") | .name] | join(", ")`)
	if g.Fail != nil || g.Out == "" {
		return "?"
	}
	return g.Out
}

func (r *Release) planReleaseWait() {
	w, t := r.Work, r.Tag
	_ = w
	r.planLine("S7", "release-wait", "cli", "gh run view RUN_ID -R nself-org/cli --json status,conclusion (wait for success); gh release view "+t+" -R nself-org/cli --json assets")
	r.planLine("S7", "release-wait", "cli", "gh release download "+t+" -R nself-org/cli -p checksums.txt; compare its darwin lines with on_arm/on_intel of Formula/nself.rb on homebrew-nself main")
	r.planLine("S7", "release-wait", "web", "node org/scripts/sources/update-lock.mjs cli "+t+" (fresh web clone); gh pr create -R nself-org/web --head release/cli-v"+r.Version+" (owner merges; no deploy)")
}

// StepReleaseWait is step_release_wait (S7).
func (r *Release) StepReleaseWait() error {
	const s, id = "S7", "release-wait"
	if r.Plan {
		r.planReleaseWait()
		return nil
	}
	if err := r.FindRun(); err != nil {
		return err
	}
	if r.RunID == "" {
		return r.die(s, id, "no release.yml run found for "+r.Tag)
	}
	if err := r.releaseState(r.RunID); err != nil {
		return err
	}
	early := r.RelSt == "success"
	if r.RelSt == "pending" {
		ok, err := r.poll("release run "+r.RunID, 120, r.releaseFinished)
		if err != nil {
			return err
		}
		if !ok {
			return r.die(s, id, "release run "+r.RunID+" still running after the wait budget")
		}
		if err := r.releaseState(r.RunID); err != nil {
			return err
		}
	}
	if r.RelSt != "success" {
		return r.die(s, id, "release run "+r.RunID+" concluded '"+r.RelSt+"'; failed jobs: "+r.failedJobs()+
			". 'Verify Homebrew formula lockstep': --from S6. 'Pre-Release Gate': satisfy the DENY rule in its log (the tag never moves), then gh run rerun "+
			r.RunID+" -R nself-org/cli --failed. Any other job: fix, then gh run rerun "+r.RunID+" -R nself-org/cli --failed. Then --from S7")
	}
	assets, err := r.ghNeed("the release "+r.Tag, "release", "view", r.Tag, "-R", "nself-org/cli", "--json", "assets", "--jq", ".assets[].name")
	if err != nil {
		return err
	}
	names := strings.Split(assets, "\n")
	has := false
	for _, n := range names {
		has = has || n == "checksums.txt"
	}
	if !has {
		return r.die(s, id, "release "+r.Tag+" has no checksums.txt asset")
	}
	if err := r.VerifyPublished(); err != nil {
		return err
	}
	if early {
		r.stepDone(s, id)
	} else {
		r.say("S7 release-wait: published with %d assets; formula hashes match the published checksums", len(names))
	}
	return r.webPR()
}
