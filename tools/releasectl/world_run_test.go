package main

// The `gh run`, workflow and git halves of the fake world.

import (
	"path/filepath"
	"strconv"
	"strings"
)

// frun is one workflow run of the fake.
type frun struct {
	id, status, concl, created string
	ok                         bool
}

// dockerRun is the newest docker-publish.yml run of a repo (stub gh logic).
func (w *world) dockerRun(repo, event string) frun {
	wf := "docker-publish.yml@" + repo
	switch {
	case event == "" && w.on["inflight@"+repo]:
		return frun{"7000", "in_progress", "", "2026-01-02T00:00:00Z", true}
	case repo == "admin" && !w.dispatched[wf] && len(w.tags["admin"]) > 0:
		if w.on["admin_tagrun_fail"] {
			return frun{"7002", "completed", "failure", "2026-01-03T00:00:00Z", true}
		}
		c := w.count["adminrun"]
		w.count["adminrun"]++
		if c < 2 {
			return frun{"7002", "in_progress", "", "2026-01-03T00:00:00Z", true}
		}
		return frun{"7002", "completed", "success", "2026-01-03T00:00:00Z", true}
	case w.dispatched[wf]:
		return frun{"7001", "completed", "success", "2026-01-02T00:00:00Z", true}
	}
	return frun{}
}

func (w *world) run(a []string, repo string) (string, error) {
	line := strings.Join(a, " ")
	if a[1] == "view" {
		return w.runView(a[2], line)
	}
	wf, br, ev := flagVal(a, "--workflow"), flagVal(a, "--branch"), flagVal(a, "--event")
	switch wf {
	case "release.yml":
		if sha := w.tags["cli"][br]; sha != "" && strings.Contains(line, `"`+sha+`"`) {
			return "5551\n", w.flagErr("runlist_fail")
		}
		return "", w.flagErr("runlist_fail")
	case "license-audit.yml":
		return w.license(br), nil
	case "docker-publish.yml":
		return w.dockerList(repo, ev, line), nil
	}
	if w.count["sdkpending@"+wf] > 0 {
		w.count["sdkpending@"+wf]--
		return "pending\n", nil
	}
	if w.dispatched[wf+"@"+repo] {
		return "success\n", nil
	}
	return "none\n", nil
}

func (w *world) license(branch string) string {
	switch {
	case w.on["license_fail"]:
		return "failure\n"
	case branch == "" && w.on["license_pr_fail"]:
		return "failure\n"
	case w.count["license_inflight"] > 0:
		w.count["license_inflight"]--
		return "pending\n"
	case w.on["license_none"]:
		return "none\n"
	}
	return "success\n"
}

func (w *world) dockerList(repo, ev, line string) string {
	rec := w.dockerRun(repo, ev)
	switch {
	case ev == "workflow_dispatch" && strings.Contains(line, "databaseId,status,conclusion"):
		switch {
		case !rec.ok, strings.Contains(line, `== "`+rec.id+`"`):
			return "none\n"
		case rec.status != "completed":
			return "pending\n"
		}
		return rec.concl + "\n"
	case ev == "workflow_dispatch":
		if rec.ok {
			return rec.id + "\n"
		}
		return "0\n"
	case strings.Contains(line, "--json createdAt --jq"):
		return rec.created + "\n"
	case strings.Contains(line, "--json status --jq"):
		if rec.ok && rec.status != "completed" {
			return "1\n"
		}
		return "0\n"
	}
	if rec.ok && rec.concl == "success" {
		return "1\n"
	}
	return "0\n"
}

func (w *world) runView(id, line string) (string, error) {
	if w.on["runview_fail"] {
		return "", fail(1, "gh: HTTP 500")
	}
	if strings.Contains(line, "headSha") {
		if id != "5551" {
			return "deadbeef\n", nil
		}
		return w.tags["cli"]["v1.5.0"] + "\n", nil
	}
	if strings.Contains(line, "status,conclusion") {
		if w.on["rerun"] {
			return "success\n", nil
		}
		return "failure\n", nil
	}
	c := w.count["buildviews"]
	w.count["buildviews"]++
	switch {
	case strings.Contains(line, "Build release artifacts"):
		if c < 2 {
			return "pending\n", nil
		}
		return "success\n", nil
	case strings.Contains(line, "Sign and publish"):
		switch {
		case w.on["gate_fail"]:
			return "failed:Pre-Release Gate\n", nil
		case c < 2:
			return "pending\n", nil
		}
		return "ok\n", nil
	case strings.Contains(line, "join"):
		var names []string
		if !w.on["rerun"] {
			names = append(names, "Verify Homebrew formula lockstep")
		}
		if w.on["gate_fail"] {
			names = append(names, "Pre-Release Gate")
		}
		return strings.Join(names, ", ") + "\n", nil
	}
	if w.on["rerun"] {
		return "success\n", nil
	}
	return "failure\n", nil
}

// git answers `git -C DIR ...` against the fake clones.
func (w *world) git(a []string) (string, error) {
	if len(a) < 3 || a[0] != "-C" {
		return "", fail(1, "unsupported git call")
	}
	repo, r := filepath.Base(a[1]), a[2:]
	if w.local[repo] == nil {
		w.local[repo] = map[string]string{}
	}
	if w.tags[repo] == nil {
		w.tags[repo] = map[string]string{}
	}
	switch r[0] {
	case "fetch", "checkout", "clean", "add":
		return "", w.flagErr("git_" + r[0] + "_fail")
	case "commit":
		if err := w.flagErr("git_commit_fail"); err != nil {
			return "", err
		}
		w.on["committed@"+repo] = true
		return "", nil
	case "ls-remote":
		if r[1] == "--exit-code" {
			if w.branches[repo+"/"+r[4]] {
				return "sha\trefs/heads/" + r[4] + "\n", nil
			}
			return "", fail(2, "")
		}
		tag := strings.TrimSuffix(strings.TrimPrefix(r[2], "refs/tags/"), "^{}")
		if sha, ok := w.tags[repo][tag]; ok && !w.on["lsremote_empty"] {
			return sha + "\trefs/tags/" + tag + "^{}\n", nil
		}
		return "", nil
	case "diff":
		if r[1] == "--cached" && !w.on["nochange@"+repo] {
			return "", fail(1, "")
		}
		if r[1] == "--quiet" && w.on["branch_differs"] {
			return "", fail(1, "")
		}
		return "", nil
	case "push":
		if w.on["push_refused"] {
			return "", fail(1, "rejected")
		}
		if t := strings.TrimPrefix(r[3], "refs/tags/"); t != r[3] {
			w.tags[repo][t] = w.local[repo][t]
		} else {
			w.branches[repo+"/"+r[3]] = true
		}
		return "", nil
	}
	return w.gitRead(repo, r)
}

// gitRead answers the read-only and tag git calls.
func (w *world) gitRead(repo string, r []string) (string, error) {
	switch r[0] {
	case "rev-parse":
		if r[1] == "origin/main" {
			return w.mainSHA[repo] + "\n", nil
		}
		t := strings.TrimSuffix(strings.TrimPrefix(r[3], "refs/tags/"), "^{commit}")
		if sha, ok := w.local[repo][t]; ok {
			return sha + "\n", nil
		}
		return "", fail(1, "")
	case "tag":
		if r[1] == "-a" {
			if w.on["tag_create_fail"] {
				return "", fail(128, "tag failed")
			}
			w.local[repo][r[2]] = r[5]
			return "", nil
		}
		return strings.Join(w.prior, "\n") + "\n", nil
	case "rev-list":
		if w.on["revlist_fail"] {
			return "", fail(128, "bad range")
		}
		if w.on["zero_commits"] {
			return "0\n", nil
		}
		return strconv.Itoa(3) + "\n", nil
	case "merge-base":
		if w.notAncestor[r[2]] {
			return "", fail(1, "")
		}
		return "", nil
	case "show":
		if w.on["show_fail"] {
			return "", fail(128, "bad object")
		}
		return w.cliVer + "\n", nil
	}
	return "", fail(1, "stub git: unhandled: "+strings.Join(r, " "))
}
