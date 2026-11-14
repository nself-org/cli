package main

// Port of scripts/release/lib/sdk.sh: S10, the SDK publish dispatches. A
// workflow is dispatched only when no run on the tag has succeeded, and on the
// tag's ref so each publishes the manifest version committed at the tag. Only
// sdk-publish-go.yml takes the CLI version as an input.

import (
	"regexp"
	"strings"
)

var (
	sdkNameRe    = regexp.MustCompile(`^sdk-.*publish.*\.yml$`)
	dispatchRe   = regexp.MustCompile(`(?m)^[ \t]*workflow_dispatch:`)
	versionInRe  = regexp.MustCompile(`(?m)^[ \t]*version:[ \t]*$`)
	sdkStateJQ   = `(map(select(.conclusion == "success")) | length) as $ok | (map(select(.status != "completed")) | length) as $p | if $ok > 0 then "success" elif $p > 0 then "pending" elif length > 0 then "failure" else "none" end`
	sdkWaitTries = 90
)

// sdkFiles is sdk_files: the names of the sdk publish workflows on a ref.
func (r *Release) sdkFiles(ref string) ([]string, error) {
	out, err := r.ghNeed("the cli workflow list at "+ref, "api", "repos/nself-org/cli/contents/.github/workflows?ref="+ref, "--jq", ".[].name")
	if err != nil {
		return nil, err
	}
	var list []string
	for _, n := range strings.Split(out, "\n") {
		if sdkNameRe.MatchString(n) {
			list = append(list, n)
		}
	}
	return list, nil
}

// sdkRunState is sdk_run_state: success | pending | failure | none for the runs on the tag.
func (r *Release) sdkRunState(f string) (string, error) {
	return r.ghNeed("the "+f+" runs", "run", "list", "-R", "nself-org/cli", "--workflow", f, "--branch", r.Tag,
		"--limit", "10", "--json", "status,conclusion", "--jq", sdkStateJQ)
}

func (r *Release) sdkCond(f string, want func(string) bool) func() (bool, error) {
	return func() (bool, error) {
		st, err := r.sdkRunState(f)
		return err == nil && want(st), err
	}
}

// StepSdkPublish is step_sdk_publish (S10).
func (r *Release) StepSdkPublish() error {
	const s, id = "S10", "sdk-publish"
	if r.DryRun {
		r.planLine(s, id, "cli", "gh api repos/nself-org/cli/contents/.github/workflows?ref=main (dry run: not read; each sdk-*publish*.yml with workflow_dispatch gets gh workflow run FILE -R nself-org/cli --ref "+r.Tag+", skipped when a run on "+r.Tag+" already succeeded)")
		return nil
	}
	ref := r.Tag
	if r.Plan {
		ref = "main"
	}
	files, err := r.sdkFiles(ref)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := r.sdkOne(f, ref); err != nil {
			return err
		}
	}
	if len(files) == 0 {
		if r.Plan {
			r.planLine(s, id, "cli", "no sdk-*publish*.yml workflow found on "+ref)
		} else {
			r.say("S10 sdk-publish: no sdk-*publish*.yml workflow on %s; nothing to dispatch", ref)
		}
	}
	return nil
}

// sdkOne handles one sdk publish workflow.
func (r *Release) sdkOne(f, ref string) error {
	const s, id = "S10", "sdk-publish"
	txt, err := r.ghNeed(f+" at "+ref, "api", "-H", "Accept: application/vnd.github.raw", "repos/nself-org/cli/contents/.github/workflows/"+f+"?ref="+ref)
	if err != nil {
		return err
	}
	if !dispatchRe.MatchString(txt) {
		if r.Plan {
			r.planLine(s, id, "cli", "manual: "+f+" has no workflow_dispatch; publish it by hand")
		} else {
			r.say("S10 sdk-publish: %s has no workflow_dispatch; publish by hand if its tag run failed", f)
		}
		return nil
	}
	args := []string{"--ref", r.Tag}
	if f == "sdk-publish-go.yml" && versionInRe.MatchString(txt) {
		args = append(args, "-f", "version="+r.Version)
	}
	if r.Plan {
		r.planLine(s, id, "cli", "gh workflow run "+f+" -R nself-org/cli "+strings.Join(args, " ")+" (skipped when a run on "+r.Tag+" already succeeded)")
		return nil
	}
	st, err := r.sdkRunState(f)
	if err != nil {
		return err
	}
	if st == "pending" {
		ok, err := r.poll(f+" run on "+r.Tag, sdkWaitTries, r.sdkCond(f, func(s string) bool { return s != "pending" }))
		if err != nil {
			return err
		}
		if !ok {
			return r.die(s, id, f+" still running after the wait budget")
		}
		if st, err = r.sdkRunState(f); err != nil {
			return err
		}
	}
	if st == "success" {
		r.stepDone(s, id+" "+f)
		return nil
	}
	if err := r.stepConfirm(s, id+" "+f); err != nil {
		return err
	}
	if _, err := r.Run.Run("gh", append([]string{"workflow", "run", f, "-R", "nself-org/cli"}, args...)...); err != nil {
		return r.die(s, id, "could not dispatch "+f)
	}
	r.Sleep(r.Poll)
	ok, err := r.poll(f+" success on "+r.Tag, sdkWaitTries, r.sdkCond(f, func(s string) bool { return s == "success" }))
	if err != nil {
		return err
	}
	if !ok {
		return r.die(s, id, f+" has no successful run on "+r.Tag+" (registry secrets set? P7-PROD-61)")
	}
	r.say("S10 sdk-publish: %s succeeded", f)
	return nil
}
