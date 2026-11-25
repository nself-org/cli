package main

// Port of scripts/release/lib/steps3.sh (part 1): S8, the cli image check.
// Images are checked on Docker Hub through the Runner (curl, no login); a
// dispatch happens only when no publish run is on the way. cli's
// docker-publish.yml starts from the Release run and takes input `version`.

import (
	"encoding/json"
	"strconv"
	"strings"
)

// hubAPI is HUB_API.
const hubAPI = "https://hub.docker.com/v2/repositories"

// hubOK is hub_ok: the X.Y.Z tag exists with linux/amd64 and linux/arm64.
func (r *Release) hubOK(hub string) bool {
	out, err := r.Run.Run("curl", "-fsS", hubAPI+"/"+hub+"/tags/"+r.Version)
	if err != nil {
		return false
	}
	var t struct {
		Images []struct {
			OS   string `json:"os"`
			Arch string `json:"architecture"`
		} `json:"images"`
	}
	if json.Unmarshal([]byte(out), &t) != nil {
		return false
	}
	seen := map[string]bool{}
	for _, i := range t.Images {
		seen[i.OS+"/"+i.Arch] = true
	}
	return seen["linux/amd64"] && seen["linux/arm64"]
}

func (r *Release) hubOKImg() (bool, error) { return r.hubOK(r.ImgHub), nil }

// imageReady is image_ready: the image is up, or the dispatch we made failed.
func (r *Release) imageReady() (bool, error) {
	if r.hubOK(r.ImgHub) {
		return true, nil
	}
	before := r.ImgPre
	if before == "" {
		before = "0"
	}
	st, err := r.ghNeed("the docker-publish.yml dispatches", "run", "list", "-R", "nself-org/"+r.ImgRepo, "--workflow", "docker-publish.yml",
		"--event", "workflow_dispatch", "--limit", "1", "--json", "databaseId,status,conclusion",
		"--jq", `.[0] | if . == null or (.databaseId|tostring) == "`+before+`" then "none" elif .status != "completed" then "pending" else .conclusion end`)
	if err != nil {
		return false, err
	}
	switch st {
	case "failure", "cancelled", "timed_out":
		return false, r.die(r.ImgS, r.ImgID, "docker-publish.yml run for "+r.ImgRepo+" concluded '"+st+"'; fix it, then re-run from "+r.ImgS)
	}
	return false, nil
}

// publishRunPresent is publish_run_present: a docker-publish.yml run on the
// image repo created after the cli release.
func (r *Release) publishRunPresent() (bool, error) {
	g := r.ghTry("run", "list", "-R", "nself-org/"+r.ImgRepo, "--workflow", "docker-publish.yml", "--limit", "1",
		"--json", "createdAt", "--jq", `.[0].createdAt // ""`)
	if g.Fail != nil {
		return false, nil
	}
	return g.Out != "" && !(g.Out < r.Pub), nil
}

func (r *Release) countNeed(what string, args ...string) (int, error) {
	out, err := r.ghNeed(what, args...)
	n, _ := strconv.Atoi(strings.TrimSpace(out))
	return n, err
}

// imageStep is image_step: done if the image is up; otherwise wait (about 3
// min) for the publish run the release or the tag push starts, wait on it
// while it runs, and dispatch only when there is none.
func (r *Release) imageStep(s, id, repo, hub string) error {
	r.ImgS, r.ImgID, r.ImgRepo, r.ImgHub = s, id, repo, hub
	if r.hubOK(hub) {
		r.stepDone(s, id)
		return nil
	}
	pub, err := r.ghNeed("the cli release "+r.Tag, "release", "view", r.Tag, "-R", "nself-org/cli", "--json", "publishedAt", "--jq", ".publishedAt")
	if err != nil {
		return err
	}
	r.Pub = pub
	if _, err := r.poll("a docker-publish.yml run on "+repo, 9, r.publishRunPresent); err != nil {
		return err
	}
	runs := "the docker-publish.yml runs of " + repo
	pending, err := r.countNeed(runs, "run", "list", "-R", "nself-org/"+repo, "--workflow", "docker-publish.yml",
		"--limit", "5", "--json", "status", "--jq", `[.[] | select(.status != "completed")] | length`)
	if err != nil {
		return err
	}
	tag := hub + ":" + r.Version
	if pending > 0 {
		r.say("%s %s: a docker-publish.yml run is in flight on %s; waiting for %s", s, id, repo, tag)
		ok, err := r.poll(tag, imageWaitPolls, r.hubOKImg)
		if err != nil {
			return err
		}
		if !ok {
			return r.die(s, id, "the in-flight run on "+repo+" did not publish "+tag+"; check it, then --from "+s)
		}
		r.say("%s %s: %s is up", s, id, tag)
		return nil
	}
	recent, err := r.countNeed(runs, "run", "list", "-R", "nself-org/"+repo, "--workflow", "docker-publish.yml",
		"--limit", "5", "--json", "createdAt,conclusion", "--jq", `[.[] | select(.conclusion == "success" and .createdAt >= "`+pub+`")] | length`)
	if err != nil {
		return err
	}
	if recent > 0 {
		if ok, _ := r.poll(tag+" after a recent run", 6, r.hubOKImg); ok {
			r.say("%s %s: %s is up", s, id, tag)
			return nil
		}
	}
	return r.dispatchImage(s, id, repo, tag)
}

// dispatchImage confirms, dispatches docker-publish.yml once and waits for the image.
func (r *Release) dispatchImage(s, id, repo, tag string) error {
	if err := r.stepConfirm(s, id); err != nil {
		return err
	}
	before, err := r.ghNeed("the docker-publish.yml dispatches", "run", "list", "-R", "nself-org/"+repo, "--workflow", "docker-publish.yml",
		"--event", "workflow_dispatch", "--limit", "1", "--json", "databaseId", "--jq", ".[0].databaseId // 0")
	if err != nil {
		return err
	}
	r.ImgPre = before
	if _, err := r.Run.Run("gh", "workflow", "run", "docker-publish.yml", "-R", "nself-org/"+repo, "-f", "version="+r.Version); err != nil {
		return r.die(s, id, "could not dispatch docker-publish.yml on "+repo)
	}
	r.say("%s %s: dispatched docker-publish.yml on %s", s, id, repo)
	r.Sleep(r.Poll)
	ok, err := r.poll(tag, imageWaitPolls, r.imageReady)
	if err != nil {
		return err
	}
	if !ok {
		return r.die(s, id, tag+" (amd64 + arm64) not on Docker Hub after the wait budget")
	}
	r.say("%s %s: %s is up (linux/amd64, linux/arm64)", s, id, tag)
	return nil
}

// StepDockerVerify is step_docker_verify (S8).
func (r *Release) StepDockerVerify() error {
	if r.Plan {
		v := r.Version
		r.planLine("S8", "docker-verify", "cli", "curl -fsS "+hubAPI+"/nself/nself/tags/"+v+" (need linux/amd64 and linux/arm64)")
		r.planLine("S8", "docker-verify", "cli", "gh run list -R nself-org/cli --workflow docker-publish.yml (wait about 3 min for the run the Release starts, then for it to finish)")
		r.planLine("S8", "docker-verify", "cli", "gh workflow run docker-publish.yml -R nself-org/cli -f version="+v+" (once, only if no run publishes the tag), then wait and re-check")
		return nil
	}
	return r.imageStep("S8", "docker-verify", "cli", "nself/nself")
}
