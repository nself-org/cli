package main

// The `gh api` and pull request half of the fake world (port of fixtures/bin/gh).

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var notFound = fail(1, "gh: Not Found (HTTP 404)")

// flagVal is the value after flag in args, or "".
func flagVal(a []string, flag string) string {
	for i := 0; i+1 < len(a); i++ {
		if a[i] == flag {
			return a[i+1]
		}
	}
	return ""
}

func (w *world) gh(a []string) (string, error) {
	if len(a) < 2 {
		return "", fail(1, "usage")
	}
	repo := strings.TrimPrefix(flagVal(a, "-R"), "nself-org/")
	if a[0] == "api" {
		return w.api(a[1:])
	}
	switch a[0] + " " + a[1] {
	case "auth status":
		return "", w.flagErr("auth_fail")
	case "repo clone":
		return "", w.clone(strings.TrimPrefix(a[2], "nself-org/"), a[3])
	case "pr create":
		p := &fpr{repo: repo, head: flagVal(a, "--head"), state: "OPEN", n: len(w.prs) + 1}
		w.prs = append(w.prs, p)
		return "https://github.com/nself-org/" + repo + "/pull/" + strconv.Itoa(p.n) + "\n", w.flagErr("prcreate_fail")
	case "pr list":
		return w.prList(a, repo)
	case "pr view":
		for _, p := range w.prs {
			if p.repo == repo && strconv.Itoa(p.n) == a[2] {
				return p.state + "\n", nil
			}
		}
		return "", fail(1, "no PR")
	case "pr checks":
		return w.prChecks(a)
	case "run list", "run view":
		return w.run(a, repo)
	case "run download", "release download":
		return "", w.download(a)
	case "run rerun":
		w.on["rerun"] = true
		return "", w.flagErr("rerun_fail")
	case "release view":
		if !w.published() {
			return "", fail(1, "release not found")
		}
		if strings.Contains(strings.Join(a, " "), "publishedAt") {
			return "2026-01-01T00:00:00Z\n", nil
		}
		return "checksums.txt\nnself-1.5.0-darwin-arm64.tar.gz\n", nil
	case "workflow run":
		w.dispatched[a[2]+"@"+repo] = true
		return "", w.flagErr("dispatch_fail")
	}
	return "", fail(1, "stub gh: unhandled: "+strings.Join(a, " "))
}

func (w *world) published() bool { return w.on["rerun"] || w.on["published"] }

func (w *world) clone(repo, dir string) error {
	if w.on["clone_fail"] {
		return fail(1, "clone failed")
	}
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	if repo == "cli" {
		os.MkdirAll(filepath.Join(dir, ".github"), 0o755)
		os.WriteFile(filepath.Join(dir, ".github", "VERSION"), []byte(w.cliVer+"\n"), 0o644)
		if w.facts {
			os.MkdirAll(filepath.Join(dir, "tools", "facts"), 0o755)
		}
	}
	return nil
}

func (w *world) download(a []string) error {
	dir := flagVal(a, "-D")
	if a[0] == "run" && w.on["artifact_expired"] {
		return fail(1, "no valid artifacts found")
	}
	if a[0] == "release" && !w.published() {
		return fail(1, "release not found")
	}
	sums := w.sums
	if a[0] == "release" && w.on["published_differs"] {
		alt, _ := os.ReadFile(filepath.Join("testdata", "distfix", "checksums-alt.txt"))
		sums = string(alt)
	}
	os.MkdirAll(dir, 0o755)
	return os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(sums), 0o644)
}

func (w *world) prList(a []string, repo string) (string, error) {
	if w.on["prlist_fail"] {
		return "", fail(1, "gh: HTTP 500")
	}
	head, merged := flagVal(a, "--head"), strings.Contains(strings.Join(a, " "), "mergeCommit")
	for _, p := range w.prs {
		if p.repo != repo || p.head != head {
			continue
		}
		if merged {
			if p.state == "MERGED" {
				return p.sha + "\n", nil
			}
			continue
		}
		return strconv.Itoa(p.n) + " " + p.state + "\n", nil
	}
	return "", nil
}

func (w *world) prChecks(a []string) (string, error) {
	if strings.Contains(strings.Join(a, " "), "--json") {
		if w.on["missing_ctx"] {
			return "", nil
		}
		return "ci\n", nil
	}
	switch {
	case w.on["pending_forever"]:
		return "ci  pending\n", fail(8, "")
	case w.count["pending_left"] > 0:
		w.count["pending_left"]--
		return "ci  pending\n", fail(8, "")
	case w.on["nochecks"]:
		return "no required checks reported on the 'x' branch\n", fail(1, "")
	case w.on["checks_red"]:
		return "1 failing check\n", fail(1, "")
	}
	return "all required checks passed\n", nil
}

// api answers `gh api ...` by path; a is the argument list after "api".
func (w *world) api(a []string) (string, error) {
	path := ""
	for i := 0; i < len(a); i++ {
		if a[i] == "-H" || a[i] == "--jq" {
			i++
		} else if !strings.HasPrefix(a[i], "-") {
			path = a[i]
			break
		}
	}
	p := strings.TrimPrefix(path, "repos/nself-org/")
	repo, rest, _ := strings.Cut(p, "/")
	switch {
	case p == "cli/releases/latest":
		if w.on["latest_fail"] {
			return "", fail(1, "gh: HTTP 500")
		}
		if w.latest == "" {
			return "", notFound
		}
		return w.latest + "\n", nil
	case strings.HasPrefix(rest, "git/ref/tags/"):
		if w.on["tagcheck_fail"] {
			return "", fail(1, "gh: HTTP 502")
		}
		if _, ok := w.tags[repo][strings.TrimPrefix(rest, "git/ref/tags/")]; ok {
			return "{}\n", nil
		}
		return "", notFound
	case strings.HasSuffix(rest, "protection/required_status_checks"):
		if w.on["required_unreadable"] {
			return "", fail(1, "gh: HTTP 500")
		}
		if w.on["required@"+repo] {
			return "ci\n", nil
		}
		return "", notFound
	case strings.HasPrefix(rest, "commits?"):
		return strings.Join(w.adminCommits, "\n") + "\n", nil
	case strings.HasPrefix(rest, "commits/"):
		return w.commit(repo, strings.TrimPrefix(rest, "commits/"))
	case strings.HasPrefix(rest, "milestones"):
		return w.milestones(), nil
	case strings.HasPrefix(rest, "actions/organization-secrets"):
		if w.on["no_secret"] {
			return "OTHER\n", nil
		}
		return "DOCKERHUB_TOKEN\n", nil
	case strings.HasPrefix(rest, "contents/"):
		return w.contents(repo, strings.TrimPrefix(rest, "contents/"))
	}
	return "", fail(1, "stub gh: unhandled api path: "+path)
}

func (w *world) commit(repo, ref string) (string, error) {
	if w.on["commit_api_fail"] {
		return "", fail(1, "gh: HTTP 500")
	}
	if sha, ok := w.tags[repo][ref]; ok {
		return sha + "\n", nil
	}
	return "", fail(1, "gh: No commit found for SHA: "+ref+" (HTTP 422)")
}

func (w *world) milestones() string {
	switch {
	case w.on["dup_open_milestone"]:
		return "closed\nopen\n"
	case w.on["dup_closed_milestone"]:
		return "closed\nclosed\n"
	case w.on["no_milestone"], w.on["only_bare_milestone"]:
		return ""
	}
	return "closed\n"
}

func (w *world) contents(repo, p string) (string, error) {
	p, ref, _ := strings.Cut(p, "?ref=")
	switch {
	case repo == "admin" && p == "package.json":
		v := w.adminVer
		if pv, ok := w.pkgAt[ref]; ok {
			v = pv
		}
		return `{"name":"admin","version":"` + v + `"}` + "\n", nil
	case repo == "cli" && p == ".github/VERSION":
		if ref != "" && w.on["tag_version_wrong"] {
			return "1.4.0\n", nil
		}
		return w.cliVer + "\n", nil
	case p == "Formula/nself.rb":
		return w.formula(), nil
	case p == ".github/workflows":
		if w.on["workflows_fail"] {
			return "", fail(1, "gh: HTTP 502")
		}
		if w.on["api_order"] { // name order, as the contents API and the shell fixture's gh stub answer
			return "release.yml\nsdk-manual-publish.yml\nsdk-publish-go.yml\nsdk-py-publish.yml\nsdk-ts-publish.yml\n", nil
		}
		return "sdk-ts-publish.yml\nsdk-py-publish.yml\nsdk-publish-go.yml\nsdk-manual-publish.yml\nrelease.yml\n", nil
	case strings.HasPrefix(p, ".github/workflows/"):
		return workflowText(strings.TrimPrefix(p, ".github/workflows/")), nil
	}
	return "", fail(1, "stub gh: no such file: "+p)
}

func workflowText(name string) string {
	switch name {
	case "sdk-manual-publish.yml":
		return "on:\n  push:\n    tags:\n      - \"v*\"\n"
	case "sdk-publish-go.yml":
		return "on:\n  push:\n  workflow_dispatch:\n    inputs:\n      version:\n        required: false\n"
	}
	return "on:\n  push:\n  workflow_dispatch:\n"
}

// formula renders Formula/nself.rb with the world's version and hashes.
func (w *world) formula() string {
	b, _ := os.ReadFile(filepath.Join("testdata", "distfix", "nself.rb"))
	s := strings.Replace(string(b), `version "1.4.13"`, `version "`+w.formulaVer+`"`, 1)
	s = strings.Replace(s, "73432f349295c10e6f28621d92fdc7736284ecb8287b5df697f98d5289acd138", w.formulaArm, 1)
	return strings.Replace(s, "eda10ee8bd97ae6707c92bef4a77624fa26883e5bea39efb2c413c18cf2aa809", w.formulaIntel, 1)
}
