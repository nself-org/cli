package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ver = "1.5.0"

// go1 runs a real (non-plan) release with --yes on an interactive terminal.
func (f *rfix) go1(extra ...string) int {
	return f.run(append([]string{"--version", ver, "--yes"}, extra...)...)
}

// from runs a real release resumed at the given step.
func (f *rfix) from(s string) int { return f.go1("--from", s) }

func newTTY(t *testing.T) *rfix { return newFixture(t, "", true) }

// workDir is where the run keeps its clones and state.
func (f *rfix) workDir() string { return f.w.work }

// bumpMains is bump_mains: the 1.5.0 bump is on main (as if S2/S3 had merged),
// with no PR record, so S4 and S9 take their no-PR path.
func (w *world) bumpMains(repos ...string) {
	if len(repos) == 0 {
		repos = []string{"admin", "cli"}
	}
	for _, r := range repos {
		if r == "admin" {
			w.adminVer, w.mainSHA["admin"] = ver, "adbump"
			w.adminCommits = []string{"adbump", "ad0001"}
			w.pkgAt["adbump"], w.pkgAt["ad0001"] = ver, "1.4.13"
		} else {
			w.cliVer = ver
		}
	}
}

// fakeMergedPR is fake_merged_pr: a merged PR record as admin-merge leaves it.
func (w *world) fakeMergedPR(repo, head, sha string) {
	w.prs = append(w.prs, &fpr{repo: repo, head: head, state: "MERGED", sha: sha, n: len(w.prs) + 1})
}

// bumpBrew is bump_brew: the formula already carries 1.5.0; with good also the
// checksums' hashes, otherwise the old 1.4.13 hashes.
func (w *world) bumpBrew(good bool) {
	w.formulaVer = ver
	if good {
		w.formulaArm, w.formulaIntel = armHash, intelHash
	}
}

// stderrHas fails unless stderr holds every fragment.
func stderrHas(t *testing.T, f *rfix, frags ...string) {
	t.Helper()
	for _, s := range frags {
		if !strings.Contains(f.err.String(), s) {
			t.Errorf("stderr lacks %q:\n%s", s, f.err.String())
		}
	}
}

func stdoutHas(t *testing.T, f *rfix, frags ...string) {
	t.Helper()
	for _, s := range frags {
		if !strings.Contains(f.out.String(), s) {
			t.Errorf("stdout lacks %q:\n%s", s, f.out.String())
		}
	}
}

func wantCode(t *testing.T, got, want int, f *rfix) {
	t.Helper()
	if got != want {
		t.Fatalf("exit %d, want %d\nstdout:\n%s\nstderr:\n%s", got, want, f.out.String(), f.err.String())
	}
}

// seedWork pre-creates files under the run's work dir.
func (f *rfix) seedWork(rel, content string) {
	p := filepath.Join(f.workDir(), rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(content), 0o644)
}

// merged is the repos admin-merge ran for, in order.
func (f *rfix) merged() string { return f.w.mergedOrder() }

// wantEq fails when got differs from want, showing both quoted.
func wantEq(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s mismatch\n got: %q\nwant: %q", what, got, want)
	}
}
