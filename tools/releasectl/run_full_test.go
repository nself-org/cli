package main

import (
	"os"
	"strings"
	"testing"
)

func fullWorld(t *testing.T) *rfix {
	f := newTTY(t)
	f.w.facts = true
	return f
}

// TestReleaseRunFullRunS1toS10 is case 3: a full run, --yes, with the owner answering
// every merge on the terminal.
func TestReleaseRunFullRunS1toS10(t *testing.T) {
	f := fullWorld(t)
	w := f.w
	wantCode(t, f.go1(), 0, f)
	stdoutHas(t, f, "release v1.5.0: S1..S10 complete\n", "S1 preflight: cli main is at 1.4.13, releasing 1.5.0; work dir "+f.workDir()+"\n")
	if got, want := strings.Join(w.merges, "|"), "repo=admin pr=1 tty=yes ans=y|repo=cli pr=2 tty=yes ans=y|repo=homebrew-nself pr=3 tty=yes ans=y"; got != want {
		t.Errorf("merges %q want %q", got, want)
	}
	if w.adminVer != ver || w.cliVer != ver {
		t.Errorf("versions admin=%s cli=%s", w.adminVer, w.cliVer)
	}
	if w.formulaVer != ver || w.formulaArm != armHash || w.formulaIntel != intelHash || w.formulaArm == w.formulaIntel {
		t.Errorf("formula %s %s %s", w.formulaVer, w.formulaArm, w.formulaIntel)
	}
	if w.tags["cli"]["v1.5.0"] != w.mainSHA["cli"] || w.tags["admin"]["v1.5.0"] != w.mainSHA["admin"] {
		t.Errorf("tags cli=%s admin=%s main=%v", w.tags["cli"]["v1.5.0"], w.tags["admin"]["v1.5.0"], w.mainSHA)
	}
	for _, c := range []string{
		"in cli: bash scripts/bump-version.sh 1.5.0 --no-goldens",
		"in cli: bash scripts/bump-version.sh 1.5.0\n",
		"gh run rerun 5551 -R nself-org/cli --failed",
		"gh workflow run docker-publish.yml -R nself-org/cli -f version=1.5.0",
		"in cli: go run -mod=vendor ./tools/facts",
		"in cli: go run -mod=vendor ./tools/facts -readme README.md",
		"in homebrew-nself: bash .github/scripts/update-formula.sh --version v1.5.0 --checksums-file " + f.workDir() + "/checksums/checksums.txt",
		"in web: node org/scripts/sources/update-lock.mjs cli v1.5.0",
	} {
		if !w.hasCall(strings.TrimSuffix(c, "\n")) {
			t.Errorf("missing call %q in %v", c, w.calls)
		}
	}
	// S3's bump has no flag: the exact call ends the line.
	if w.called("in cli: bash scripts/bump-version.sh 1.5.0 --no-goldens") != 1 || w.called("in cli: bash scripts/bump-version.sh 1.5.0") != 2 {
		t.Errorf("bump calls: %v", w.calls)
	}
	if w.called("gh release create") != 0 || w.called("gh workflow run docker-publish.yml -R nself-org/admin") != 0 {
		t.Errorf("unexpected release/dispatch: %v", w.calls)
	}
	if w.called("gh workflow run sdk-ts-publish.yml -R nself-org/cli --ref v1.5.0") != 1 ||
		w.called("gh workflow run sdk-py-publish.yml -R nself-org/cli --ref v1.5.0") != 1 {
		t.Errorf("sdk ts/py dispatch: %v", w.calls)
	}
	if !w.hasCall("gh workflow run sdk-publish-go.yml -R nself-org/cli --ref v1.5.0 -f version=1.5.0") {
		t.Error("sdk go not dispatched with the cli version")
	}
	if w.called("gh workflow run sdk-manual") != 0 {
		t.Error("manual sdk workflow was dispatched")
	}
	stdoutHas(t, f, "a docker-publish.yml run on cli (9/9)\n", "sdk-manual-publish.yml has no workflow_dispatch; publish by hand if its tag run failed\n",
		"S5 build-wait: build ok, checksums from the build artifact of run 5551\n")
}

// TestReleaseRunFullRunSideEffects: the web PR branch, admin image and cleanup.
func TestReleaseRunFullRunSideEffects(t *testing.T) {
	f := fullWorld(t)
	wantCode(t, f.go1(), 0, f)
	if !f.w.branches["web/release/cli-v1.5.0"] {
		t.Error("web lock PR branch was not pushed")
	}
	stdoutHas(t, f, "S7 release-wait web: opened https://github.com/nself-org/web/pull/4 (owner merges; no deploy)\n",
		"S9 admin-image: tagged admin v1.5.0 -> m1sha (this starts admin's docker-publish.yml)\n",
		"S9 admin-image: nself/nself-admin:1.5.0 is up\n")
	if _, err := os.Stat(f.workDir()); err == nil {
		t.Error("work dir left behind")
	}
	if _, err := os.Stat(f.workDir() + ".state"); err == nil {
		t.Error("state file left behind")
	}
	if f.sleeps == 0 {
		t.Error("no poll sleeps recorded")
	}
}

// TestReleaseRunRerunOfFinishedStepsChangesNothing is case 4: S4.. all say done, no PR,
// no dispatch, no re-run, no extra admin-merge.
func TestReleaseRunRerunOfFinishedStepsChangesNothing(t *testing.T) {
	f := fullWorld(t)
	wantCode(t, f.go1(), 0, f)
	w := f.w
	prs, merges := len(w.prs), len(w.merges)
	w.on["published"] = true
	before := w.called("gh workflow run") + w.called("gh run rerun") + w.called("gh pr create")
	f.out.Reset()
	f.err.Reset()
	wantCode(t, f.from("S4"), 0, f)
	for _, s := range []string{"S4 tag", "S5 build-wait", "S6 homebrew-formula", "S7 release-wait", "S8 docker-verify", "S9 admin-image", "S10 sdk-publish"} {
		stdoutHas(t, f, s)
	}
	if got := w.called("gh workflow run") + w.called("gh run rerun") + w.called("gh pr create"); got != before {
		t.Errorf("rerun made %d mutating calls", got-before)
	}
	if len(w.prs) != prs || len(w.merges) != merges {
		t.Errorf("rerun changed PRs or merges")
	}
	for _, s := range []string{"S4 tag done", "S5 build-wait done", "S8 docker-verify done", "S10 sdk-publish sdk-ts-publish.yml done"} {
		stdoutHas(t, f, s)
	}
}

// TestReleaseRunFromS6SkipsEarlierSteps is case 5.
func TestReleaseRunFromS6SkipsEarlierSteps(t *testing.T) {
	f := fullWorld(t)
	f.w.tags["cli"]["v1.5.0"] = "c0ffee1"
	f.w.bumpMains("admin")
	wantCode(t, f.from("S6"), 0, f)
	if f.merged() != "homebrew-nself" {
		t.Errorf("merged %q", f.merged())
	}
	for _, l := range strings.Split(f.out.String(), "\n") {
		for _, s := range []string{"S1 ", "S2 ", "S3 ", "S4 ", "S5 "} {
			if strings.HasPrefix(l, s) {
				t.Errorf("printed an early step: %q", l)
			}
		}
	}
	if f.w.hasCall("gh repo clone nself-org/cli ") || f.w.formulaArm != armHash || f.w.formulaIntel != intelHash {
		t.Errorf("clone/hashes wrong: %v", f.w.calls)
	}
}

// TestReleaseRunFactsAbsentSkipsRegeneration is case 6.
func TestReleaseRunFactsAbsentSkipsRegeneration(t *testing.T) {
	f := newTTY(t)
	f.w.bumpMains("admin")
	wantCode(t, f.from("S3"), 0, f)
	stdoutHas(t, f, "S3 cli-version: tools/facts absent at this commit; no facts regeneration\n")
	if f.w.called("in cli: go ") != 0 {
		t.Errorf("go ran without tools/facts: %v", f.w.calls)
	}
	if f.w.cliVer != ver {
		t.Error("cli VERSION not bumped")
	}
}

// TestReleaseRunRedChecksNeverMerge is case 7.
func TestReleaseRunRedChecksNeverMerge(t *testing.T) {
	f := newTTY(t)
	f.w.on["checks_red"] = true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: required checks on admin#1 are not green; nothing was merged\n",
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S2\n")
	if len(f.w.merges) != 0 || f.w.adminVer != "1.4.13" {
		t.Errorf("merged %v admin=%s", f.w.merges, f.w.adminVer)
	}
}

// TestReleaseRunFailingStepStopsRun: a failing step prints its name, exits 1 and no later
// step runs (rule 3).
func TestReleaseRunFailingStepStopsRun(t *testing.T) {
	f := newTTY(t)
	f.w.on["bump_fail"] = true
	wantCode(t, f.go1(), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: scripts/bump-version.sh failed in the cli clone\n")
	for _, c := range f.w.calls {
		if strings.Contains(c, "cli/contents/.github/VERSION") && !strings.Contains(c, "latest") {
			t.Errorf("S3 ran after the S2 failure: %q", c)
		}
	}
	if f.w.called("gh pr create") != 0 || strings.Contains(f.out.String(), "S3 ") {
		t.Errorf("later work happened: out=%q calls=%v", f.out.String(), f.w.calls)
	}
	if _, err := os.Stat(f.workDir()); err != nil {
		t.Error("work dir of a failed run must stay for the resume")
	}
}

// TestReleaseRunOperatorDeclinesPrompt: without --yes the "Run S<n>?" prompt is asked.
func TestReleaseRunOperatorDeclinesPrompt(t *testing.T) {
	f := newFixture(t, "n\n", true)
	wantCode(t, f.run("--version", ver, "--from", "S2"), 1, f)
	stdoutHas(t, f, "Run S2 admin-lockstep? [y/N] ")
	stderrHas(t, f, "release: stopped at S2 by the operator; resume with --from S2\n")
	if len(f.w.merges) != 0 || f.w.called("gh pr create") != 0 {
		t.Error("work happened after the decline")
	}
}
