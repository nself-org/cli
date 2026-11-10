package main

// An in-memory stand-in for GitHub, git and Docker Hub that answers the Runner
// calls of the release steps. It never runs a real command: it records every
// call and answers from state. Port of scripts/release/fixtures/bin/*.

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	armHash   = "554a93c77f0a4d50fabbdc3ed7bd01638f092ce082f2d54d866d659d662bfd77"
	intelHash = "6f823864cb70bb06e0d87f751db0fafcf96375da658da2b281b0fb1d9a4deeba"
	altArm    = "639ecc2daad4e41e102743725f3369010658cbdd72a1343a675af421ee99b664"
)

// fpr is a pull request in the fake.
type fpr struct {
	repo, head, state, sha string
	n                      int
}

// world is the fake GitHub plus the clones.
type world struct {
	t     *testing.T
	calls []string
	work  string
	root  string

	latest   string                       // latest cli release tag, "" = none (404)
	tags     map[string]map[string]string // remote tags: repo -> tag -> sha
	local    map[string]map[string]string // local tags in the clones
	mainSHA  map[string]string            // repo -> origin/main
	branches map[string]bool              // "repo/branch" on the remote
	prs      []*fpr
	on       map[string]bool // scenario flags (the files under $ST)
	count    map[string]int  // scenario counters

	adminVer, cliVer string   // versions on main
	adminCommits     []string // admin commits that touched package.json, newest first
	pkgAt            map[string]string
	formulaVer       string
	formulaArm       string
	formulaIntel     string
	prior            []string // other cli tags, newest first
	merges           []string // admin-merge.sh calls: "repo pr tty ans"
	typed            string   // what the owner types at admin-merge
	facts            bool     // the cli clone has tools/facts
	notAncestor      map[string]bool
	hub              map[string]bool // "repo:version" images that are up
	dispatched       map[string]bool
	sums             string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "distfix", "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	return &world{
		t: t, work: t.TempDir(), root: t.TempDir(), latest: "v1.4.13", sums: string(b),
		tags: map[string]map[string]string{"cli": {}, "admin": {}}, local: map[string]map[string]string{"cli": {}, "admin": {}},
		mainSHA:  map[string]string{"cli": "c0ffee1", "admin": "ad0001", "homebrew-nself": "b0001", "web": "e0001"},
		branches: map[string]bool{}, on: map[string]bool{}, count: map[string]int{}, notAncestor: map[string]bool{},
		hub: map[string]bool{}, dispatched: map[string]bool{}, pkgAt: map[string]string{},
		adminVer: "1.4.13", cliVer: "1.4.13", formulaVer: "1.4.13", formulaArm: "73432f349295c10e6f28621d92fdc7736284ecb8287b5df697f98d5289acd138", formulaIntel: "eda10ee8bd97ae6707c92bef4a77624fa26883e5bea39efb2c413c18cf2aa809",
		typed: "y", prior: []string{"v1.4.13"},
	}
}

func (w *world) record(line string) { w.calls = append(w.calls, line) }

func (w *world) called(prefix string) int {
	n := 0
	for _, c := range w.calls {
		if strings.HasPrefix(c, prefix) {
			n++
		}
	}
	return n
}

func (w *world) hasCall(sub string) bool {
	for _, c := range w.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func fail(code int, stderr string) error { return &RunError{Code: code, Stderr: stderr} }

// Run answers one external command.
func (w *world) Run(name string, args ...string) (string, error) {
	w.record(name + " " + strings.Join(args, " "))
	switch name {
	case "gh":
		return w.gh(args)
	case "git":
		return w.git(args)
	case "curl":
		return w.curl(args)
	}
	return "", fail(127, "unknown command "+name)
}

// RunIn answers one command run in a directory (admin-merge, bump, node, go).
func (w *world) RunIn(dir string, tty bool, name string, args ...string) (string, error) {
	rel := strings.TrimPrefix(dir, w.work+"/")
	mark := ""
	if tty {
		mark = " tty"
	}
	w.record("in " + rel + mark + ": " + name + " " + strings.Join(args, " "))
	switch {
	case len(args) > 0 && strings.HasSuffix(args[0], "admin-merge.sh"):
		return "", w.adminMerge(tty, args)
	case len(args) > 0 && strings.HasSuffix(args[0], "bump-version.sh"):
		return "", w.flagErr("bump_fail")
	case len(args) > 0 && strings.HasSuffix(args[0], "update-formula.sh"):
		b, _ := os.ReadFile(flagVal(args, "--checksums-file"))
		w.formulaVer, w.formulaArm, w.formulaIntel = "1.5.0", sumsHashOf(string(b), "arm64"), sumsHashOf(string(b), "amd64")
		return "", w.flagErr("formula_fail")
	case name == "node":
		return "", w.flagErr("node_fail")
	case name == "go":
		return "", w.flagErr("facts_fail")
	}
	return "", fail(127, "unknown command "+name)
}

func sumsHashOf(sums, arch string) string {
	for _, l := range strings.Split(sums, "\n") {
		if f := strings.Fields(l); len(f) == 2 && f[1] == "nself-1.5.0-darwin-"+arch+".tar.gz" {
			return f[0]
		}
	}
	return ""
}

func (w *world) flagErr(flag string) error {
	if w.on[flag] {
		return fail(1, flag)
	}
	return nil
}

// adminMerge is the stub admin-merge.sh: it needs a terminal and a typed y.
func (w *world) adminMerge(tty bool, args []string) error {
	repo, n := strings.TrimPrefix(args[2], "nself-org/"), args[4]
	t := "no"
	if tty {
		t = "yes"
	}
	w.merges = append(w.merges, "repo="+repo+" pr="+n+" tty="+t+" ans="+w.typed)
	if !tty || w.typed != "y" {
		return fail(1, "declined")
	}
	for _, p := range w.prs {
		if p.repo == repo && strconv.Itoa(p.n) == n {
			p.state, p.sha = "MERGED", "m"+n+"sha"
			w.mainSHA[repo] = p.sha
			switch repo {
			case "admin":
				w.adminVer = "1.5.0"
			case "cli":
				w.cliVer = "1.5.0"
			}
		}
	}
	return nil
}

// mergedOrder is the repos admin-merge was called for, in order.
func (w *world) mergedOrder() string {
	var out []string
	for _, m := range w.merges {
		out = append(out, strings.TrimPrefix(strings.Fields(m)[0], "repo="))
	}
	return strings.Join(out, " ")
}

// curl answers the Docker Hub tag lookups (port of fixtures/bin/curl): the
// image shows once the matching publish workflow was dispatched, on the third
// lookup of an in-flight run, or on the fourth after admin's tag push.
func (w *world) curl(args []string) (string, error) {
	url := args[len(args)-1]
	const up = `{"images":[{"os":"linux","architecture":"amd64"},{"os":"linux","architecture":"arm64"}]}`
	repo := ""
	switch {
	case strings.Contains(url, "/nself/nself/tags/"):
		repo = "cli"
	case strings.Contains(url, "/nself/nself-admin/tags/"):
		repo = "admin"
	default:
		return "", fail(22, "404")
	}
	if w.on["inflight@"+repo] {
		c := w.count["hubcalls@"+repo]
		w.count["hubcalls@"+repo]++
		if c >= 2 {
			return up, nil
		}
	}
	if w.dispatched["docker-publish.yml@"+repo] {
		return up, nil
	}
	if repo == "admin" && !w.on["admin_tagrun_fail"] && len(w.tags["admin"]) > 0 {
		c := w.count["hubadmin"]
		w.count["hubadmin"]++
		if c >= 3 {
			return up, nil
		}
	}
	return "", fail(22, "404")
}

// rfix holds one Release over the world with captured output.
type rfix struct {
	w        *world
	d        Deps
	out, err *bytes.Buffer
	sleeps   int
}

// newFixture wires Deps over the world. stdin feeds the "Run S<n>?" prompts.
func newFixture(t *testing.T, stdin string, interactive bool) *rfix {
	w := newWorld(t)
	f := &rfix{w: w, out: &bytes.Buffer{}, err: &bytes.Buffer{}}
	os.MkdirAll(filepath.Join(w.root, "scripts"), 0o755)
	os.WriteFile(filepath.Join(w.root, "scripts", "admin-merge.sh"), []byte("#"), 0o644)
	f.d = Deps{
		Runner: w, Stdout: f.out, Stderr: f.err, Stdin: bufio.NewReader(strings.NewReader(stdin)),
		Interactive: interactive, Root: w.root,
		Env:    func(k string) string { return map[string]string{"TMPDIR": filepath.Dir(w.work) + "/"}[k] },
		HaveGH: func() bool { return true }, Have: func(string) bool { return true },
		Now: time.Now, Sleep: func(time.Duration) { f.sleeps++ },
	}
	// The work dir is TMPDIR/nself-release-<v>; point it at the world's temp dir.
	w.work = filepath.Join(filepath.Dir(w.work), "nself-release-1.5.0")
	t.Cleanup(func() { os.RemoveAll(w.work); os.Remove(w.work + ".state") })
	return f
}

// run executes `releasectl run` and returns the exit code.
func (f *rfix) run(args ...string) int {
	return RunWith(f.d, append([]string{"run"}, args...))
}
