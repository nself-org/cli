package main

// Port of scripts/release/lib/steps2.sh: S5 build wait + checksums, S6 the
// Homebrew formula PR. release.yml is the engine; this only waits on it.

import (
	"path/filepath"
	"strings"
)

const buildJQ = `
    [.jobs[] | select(.name == "Build release artifacts" or .name == "build")][0]
    | if . == null or .status != "completed" then "pending" else .conclusion end`

const relJQ = `if .status != "completed" then "pending" else .conclusion end`

// buildState is build_state: BuildSt = success | failure | pending.
func (r *Release) buildState(id string) error {
	out, err := r.ghNeed("the jobs of run "+id, "run", "view", id, "-R", "nself-org/cli", "--json", "jobs", "--jq", buildJQ)
	r.BuildSt = out
	return err
}

// releaseState is release_state: RelSt = success | failure | pending for the run.
func (r *Release) releaseState(id string) error {
	out, err := r.ghNeed("run "+id, "run", "view", id, "-R", "nself-org/cli", "--json", "status,conclusion", "--jq", relJQ)
	r.RelSt = out
	return err
}

func (r *Release) buildFinished() (bool, error) {
	if err := r.buildState(r.RunID); err != nil {
		return false, err
	}
	return r.BuildSt != "pending", nil
}

func (r *Release) releaseFinished() (bool, error) {
	if err := r.releaseState(r.RunID); err != nil {
		return false, err
	}
	return r.RelSt != "pending", nil
}

// noChecksums is no_checksums: the artifact expired and nothing is published.
func (r *Release) noChecksums() error {
	id := r.RunID
	if id == "" {
		id = "?"
	}
	rid := r.RunID
	if rid == "" {
		rid = "ID"
	}
	return r.die(r.Step, r.ID, "no checksums.txt: the release-checksums artifact of run "+id+" has expired (retention is 1 day) and the release is not published. "+
		"Re-run the WHOLE run (gh run rerun "+rid+" -R nself-org/cli, not --failed), wait for its build to finish (checksums are fetched fresh each time, nothing cached to clear)", "S5")
}

// StepBuildWait is step_build_wait (S5).
func (r *Release) StepBuildWait() error {
	if r.Plan {
		w := r.Work
		r.planLine("S5", "build-wait", "cli", "gh run list -R nself-org/cli --workflow release.yml --branch "+r.Tag+" (the run whose headSha is the tag commit)")
		r.planLine("S5", "build-wait", "cli", "gh run view RUN_ID -R nself-org/cli --json jobs (wait for the build job)")
		r.planLine("S5", "build-wait", "cli", "gh run download RUN_ID -R nself-org/cli -n release-checksums -D "+w+"/checksums")
		return nil
	}
	ok, err := r.poll("release.yml run for "+r.Tag, 30, r.RunPresent)
	if err != nil {
		return err
	}
	if !ok {
		return r.die("S5", "build-wait", "no release.yml run appeared for "+r.Tag+" (check the Actions tab of nself-org/cli)")
	}
	if err := r.buildState(r.RunID); err != nil {
		return err
	}
	early := r.BuildSt == "success"
	if r.BuildSt == "pending" {
		if ok, err := r.poll("build job of run "+r.RunID, 120, r.buildFinished); err != nil {
			return err
		} else if !ok {
			return r.die("S5", "build-wait", "build job still running after the wait budget")
		}
	}
	if err := r.buildState(r.RunID); err != nil {
		return err
	}
	if r.BuildSt != "success" {
		return r.die("S5", "build-wait", "build job concluded '"+r.BuildSt+"' in run "+r.RunID+"; fix it, then re-run the failed jobs")
	}
	have, err := r.EnsureChecksums()
	if err != nil {
		return err
	}
	if !have {
		return r.noChecksums()
	}
	if early {
		r.stepDone("S5", "build-wait")
	} else {
		r.say("S5 build-wait: build ok, checksums from %s", r.ChecksumsSrc)
	}
	return nil
}

// rerunLockstep is rerun_lockstep: with the formula on main, re-run the
// lockstep job of the release run if it failed and the run is not successful.
// With wait a run still in progress is waited for; without it, left alone.
func (r *Release) rerunLockstep(wait bool) error {
	if err := r.FindRun(); err != nil {
		return err
	}
	if r.RunID == "" {
		return nil
	}
	if wait {
		if ok, err := r.poll("run "+r.RunID+" to settle", 60, r.releaseFinished); err != nil || !ok {
			return err
		}
	}
	if err := r.releaseState(r.RunID); err != nil {
		return err
	}
	c := r.RelSt
	if c == "success" || c == "pending" {
		return nil
	}
	out, err := r.ghNeed("the jobs of run "+r.RunID, "run", "view", r.RunID, "-R", "nself-org/cli", "--json", "jobs",
		"--jq", `[.jobs[] | select(.name | test("Homebrew"; "i")) | .conclusion] | first // empty`)
	if err != nil {
		return err
	}
	if out != "failure" {
		r.say("S6 homebrew-formula: run %s concluded '%s' but its lockstep job did not fail; not re-running", r.RunID, c)
		return nil
	}
	if _, err := r.Run.Run("gh", "run", "rerun", r.RunID, "-R", "nself-org/cli", "--failed"); err != nil {
		return r.die("S6", "homebrew-formula", "could not re-run the failed jobs of run "+r.RunID)
	}
	r.say("S6 homebrew-formula: re-ran failed jobs of run %s", r.RunID)
	r.Sleep(r.Poll)
	return nil
}

func (r *Release) planFormula() {
	w, v, t := r.Work, r.Version, r.Tag
	br := "release/nself-v" + v + "-ARM8"
	p := func(repo, c string) { r.planLine("S6", "homebrew-formula", repo, c) }
	p("cli", "gh run view RUN_ID -R nself-org/cli --json jobs (wait until every job except Homebrew lockstep and Sign and publish succeeded)")
	p("homebrew-nself", "bash .github/scripts/update-formula.sh --version "+t+" --checksums-file "+w+"/checksums/checksums.txt (cwd "+w+"/homebrew-nself; writes on_arm and on_intel; the published checksums win once the release exists)")
	p("homebrew-nself", "git checkout -B "+br+"; git commit; git push origin "+br+"; gh pr create -R nself-org/homebrew-nself --base main --head "+br)
	p("homebrew-nself", "gh pr checks N -R nself-org/homebrew-nself --required (polled, 45 min budget)")
	p("homebrew-nself", "scripts/admin-merge.sh --repo nself-org/homebrew-nself --pr N")
	p("cli", "gh run rerun RUN_ID -R nself-org/cli --failed (the verify-homebrew-lockstep job)")
}

// StepHomebrewFormula is step_homebrew_formula (S6).
func (r *Release) StepHomebrewFormula() error {
	const s, id = "S6", "homebrew-formula"
	if r.Plan {
		r.planFormula()
		return nil
	}
	if err := r.LoadFormula(); err != nil {
		return err
	}
	have, err := r.EnsureChecksums()
	if err != nil {
		return err
	}
	var arm, intel string
	if have {
		b, _ := readFile(r.ChecksumsFile)
		arm, intel = r.SumsHash(b, "arm64"), r.SumsHash(b, "amd64")
		if arm == "" || intel == "" {
			return r.die(s, id, "checksums.txt has no darwin arm64/amd64 lines")
		}
		if r.FVer == r.Version && r.FArm == arm && r.FIntel == intel {
			r.stepDone(s, id)
			return r.rerunLockstep(false)
		}
		if r.FVer == r.Version {
			r.say("S6 homebrew-formula: formula is at %s but its hashes differ from %s; rewriting", r.Version, r.ChecksumsSrc)
		}
	} else {
		if r.FVer != r.Version {
			return r.noChecksums()
		}
		r.say("S6 homebrew-formula: formula is at %s; checksums are gone so the hashes cannot be re-checked here (S7 checks them against the published file)", r.Version)
		r.stepDone(s, id)
		return r.rerunLockstep(false)
	}
	return r.formulaPR(arm)
}

// formulaPR writes the formula on a branch and merges its PR (S6, second half).
func (r *Release) formulaPR(arm string) error {
	const s, id = "S6", "homebrew-formula"
	br := "release/nself-v" + r.Version + "-" + head(arm, 8)
	if err := r.prInfo("homebrew-nself", br); err != nil {
		return err
	}
	del := "(gh api -X DELETE repos/nself-org/homebrew-nself/git/refs/heads/" + br + ")"
	switch {
	case strings.HasSuffix(r.PrInfo, "CLOSED"):
		return r.die(s, id, "PR "+r.prNumber()+" was closed unmerged; delete branch "+br+" "+del+" and retry")
	case strings.HasSuffix(r.PrInfo, "MERGED"):
		return r.die(s, id, "PR "+r.prNumber()+" for these hashes is merged but Formula/nself.rb on main does not carry them (reverted?); restore the formula by hand, then --from S6")
	}
	if err := r.ReleaseGate(); err != nil {
		return err
	}
	if err := r.stepConfirm(s, id); err != nil {
		return err
	}
	title := "chore(formula): nself " + r.Tag
	if r.PrInfo == "" {
		if err := r.ensureClone("homebrew-nself"); err != nil {
			return err
		}
		if err := r.resetClone("homebrew-nself"); err != nil {
			return err
		}
		if _, err := r.git("homebrew-nself", "checkout", "-q", "-B", br); err != nil {
			return r.die(s, id, "could not create branch "+br)
		}
		if _, err := r.runIn(filepath.Join(r.Work, "homebrew-nself"), false, "bash", ".github/scripts/update-formula.sh",
			"--version", r.Tag, "--checksums-file", r.ChecksumsFile); err != nil {
			return r.die(s, id, "update-formula.sh refused the checksums")
		}
		changed, cerr := r.commitAll("homebrew-nself", title)
		if cerr != nil {
			return r.die(s, id, cerr.Error())
		}
		if !changed {
			return r.die(s, id, "formula unchanged")
		}
	} else if err := r.checkoutBranch("homebrew-nself", br); err != nil {
		return err
	}
	if err := r.prFlow(s, id, "homebrew-nself", br, "brew_pr", title,
		"Formula for cli "+r.Tag+", written by .github/scripts/update-formula.sh from "+r.ChecksumsSrc+" (both darwin hashes). Opened by scripts/release/release.sh in the cli repo."); err != nil {
		return err
	}
	return r.rerunLockstep(true)
}
