package main

// Golden tests against transcripts captured from the shell scripts
// (testdata/shell/*.golden): the Go port must print what the script printed
// and make the gh calls it made, except for the deviations named here.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// shellRun is one captured script run: its sections by name.
type shellRun map[string]string

// loadShell reads testdata/shell/<name>.golden: "# " comment lines, then
// "== <section>" headers each followed by the section's lines.
func loadShell(t *testing.T, name string) shellRun {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "shell", name+".golden"))
	if err != nil {
		t.Fatal(err)
	}
	run, cur := shellRun{}, ""
	for _, l := range strings.SplitAfter(string(b), "\n") {
		switch {
		case l == "":
		case strings.HasPrefix(l, "== "):
			cur = strings.TrimSpace(l[3:])
			run[cur] = ""
		case cur == "" && strings.HasPrefix(l, "# "):
		default:
			run[cur] += l
		}
	}
	return run
}

func (s shellRun) exit(t *testing.T) int {
	n, err := strconv.Atoi(strings.TrimSpace(s["exit"]))
	if err != nil {
		t.Fatalf("exit section: %v", err)
	}
	return n
}

// ghCalls keeps the gh lines of a call log, one per line.
func ghCalls(calls []string) string {
	var b strings.Builder
	for _, c := range calls {
		if strings.HasPrefix(c, "gh ") {
			b.WriteString(c + "\n")
		}
	}
	return b.String()
}

// createDeviation is the one gh call the port changed on purpose: DENY-005
// reads every milestone page of every state, as pre-release-gate.yml did since
// D-0013 (page 1 of 30 closed milestones lost the newest tag), and the same
// rule now backs `releasectl gate`.
func createDeviation(gh string) string {
	return strings.ReplaceAll(gh, "gh api repos/{owner}/{repo}/milestones?state=closed --jq",
		"gh api --paginate repos/{owner}/{repo}/milestones?state=all&per_page=100 --jq")
}

// createWorld answers like the capture's fixture: tag v1.0.0 one commit behind.
func createWorld(t *testing.T) *env {
	e := newEnv(t, "")
	e.r.out["git -C "+e.d.Root+" tag --sort=-version:refname"] = "v1.0.0\n"
	e.r.out["git -C "+e.d.Root+" rev-list v1.0.0..HEAD --count"] = "1\n"
	e.r.out["gh release create"] = "https://github.com/nself-org/cli/releases/tag/v9.9.9\n"
	return e
}

func TestReleaseCtlCreateRetractShellCreatePass(t *testing.T) {
	sh := loadShell(t, "create-pass")
	e := createWorld(t)
	code := RunWith(e.d, []string{"create", "v9.9.9", "--generate-notes", "--title", "v9.9.9"})
	if code != sh.exit(t) {
		t.Fatalf("exit %d want %d: %s", code, sh.exit(t), e.err)
	}
	wantEq(t, "stdout", e.out.String(), sh["stdout"])
	wantEq(t, "stderr", e.err.String(), sh["stderr"])
	wantEq(t, "gh calls", ghCalls(e.r.calls), createDeviation(sh["gh"]))
	if len(e.r.ttys) != 1 || e.r.ttys[0] != "gh release create v9.9.9 --generate-notes --title v9.9.9" {
		t.Errorf("gh release create must run on the terminal like exec gh: %v", e.r.ttys)
	}
}

func TestReleaseCtlCreateRetractShellCreateBlocked(t *testing.T) {
	sh := loadShell(t, "create-blocked")
	e := createWorld(t)
	os.Mkdir(filepath.Join(e.d.Root, "dist"), 0o755)
	code := RunWith(e.d, []string{"create", "v9.9.9", "--draft"})
	if code != sh.exit(t) {
		t.Fatalf("exit %d want %d", code, sh.exit(t))
	}
	wantEq(t, "stdout", e.out.String(), sh["stdout"])
	wantEq(t, "stderr", e.err.String(), sh["stderr"])
	wantEq(t, "gh calls", ghCalls(e.r.calls), createDeviation(sh["gh"]))
}

// --dry-run prints the gh command line the script printed before its exec,
// and calls nothing.
func TestReleaseCtlCreateRetractShellCreateDryRun(t *testing.T) {
	sh := loadShell(t, "create-pass")
	var want string
	for _, l := range strings.SplitAfter(sh["stdout"], "\n") {
		if strings.HasPrefix(l, "[release-create] invoking: gh ") {
			want += l
		}
	}
	if want == "" {
		t.Fatal("no invoking line in the capture")
	}
	e := createWorld(t)
	if code := RunWith(e.d, []string{"create", "v9.9.9", "--dry-run", "--generate-notes", "--title", "v9.9.9"}); code != 0 {
		t.Fatalf("code %d", code)
	}
	wantEq(t, "stdout", e.out.String(), want)
	if len(e.r.calls) != 0 {
		t.Errorf("dry-run called %v", e.r.calls)
	}
}

// planDeviation drops the S4 row the port removed on purpose: release.sh read
// the newest license-audit.yml run on any branch only because
// pre-release-gate.yml did; the gate now runs `releasectl gate`, which reads
// main, so that second DENY-004 pass is gone.
func planDeviation(stdout string) string {
	return strings.Replace(stdout, "S4 tag cli gh run list -R nself-org/cli --workflow license-audit.yml --limit 1 (DENY-004 as pre-release-gate.yml reads it: any branch)\n", "", 1)
}

func TestReleaseRunShellPlanTranscript(t *testing.T) {
	sh := loadShell(t, "release-plan")
	if planDeviation(sh["stdout"]) == sh["stdout"] {
		t.Fatal("the any-branch row is not in the capture; planDeviation is stale")
	}
	f := newFixture(t, "", false)
	f.w.on["api_order"] = true
	code := f.run("--plan", "--version", "9.9.9")
	if code != sh.exit(t) {
		t.Fatalf("exit %d want %d: %s", code, sh.exit(t), f.err)
	}
	tmp := filepath.Dir(f.w.work)
	norm := func(s string) string { return strings.ReplaceAll(s, tmp, "@TMP@") }
	wantEq(t, "stdout", norm(f.out.String()), planDeviation(sh["stdout"]))
	wantEq(t, "stderr", norm(f.err.String()), sh["stderr"])
	wantEq(t, "gh calls", ghCalls(f.w.calls), sh["gh"])
}
