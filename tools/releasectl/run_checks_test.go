package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseRunUnreportedRequiredChecksNeverMerge is case 8: a required check that
// never reports stops the run; nothing merges.
func TestReleaseRunUnreportedRequiredChecksNeverMerge(t *testing.T) {
	f := newTTY(t)
	f.w.on["nochecks"], f.w.on["required@admin"] = true, true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: admin#1: required checks not reported as passing after 6 tries (ci); nothing was merged\n")
	if len(f.w.merges) != 0 || f.w.adminVer != "1.4.13" {
		t.Errorf("merged %v admin=%s", f.w.merges, f.w.adminVer)
	}
}

// TestReleaseRunNoRequiredChecksProceeds: protection requires nothing, so the run goes on.
func TestReleaseRunNoRequiredChecksProceeds(t *testing.T) {
	f := newTTY(t)
	f.w.on["nochecks"] = true
	wantCode(t, f.from("S2"), 0, f)
	stdoutHas(t, f, "S2 admin-lockstep: admin requires no status checks on main; continuing to the owner prompt\n")
	if f.merged() != "admin cli homebrew-nself" {
		t.Errorf("merged %q", f.merged())
	}
}

// TestReleaseRunGreenButContextMissing: green output but a required context absent.
func TestReleaseRunGreenButContextMissing(t *testing.T) {
	f := newTTY(t)
	f.w.on["required@admin"], f.w.on["missing_ctx"] = true, true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "required checks not reported as passing after 6 tries (ci )")
	if len(f.w.merges) != 0 {
		t.Error("merged")
	}
}

// TestReleaseRunRequiredContextPresentProceeds and a skipped context counts as satisfied.
func TestReleaseRunRequiredContextPresentProceeds(t *testing.T) {
	for _, flag := range []string{"", "skip_ctx"} {
		f := newTTY(t)
		f.w.on["required@admin"] = true
		if flag != "" {
			f.w.on[flag] = true
		}
		wantCode(t, f.from("S2"), 0, f)
		if len(f.w.merges) != 3 {
			t.Errorf("%q: merges %v", flag, f.w.merges)
		}
	}
}

// TestReleaseRunPendingChecksAreWaitedFor and the budget (case 15).
func TestReleaseRunPendingChecksAreWaitedFor(t *testing.T) {
	f := newTTY(t)
	f.w.count["pending_left"] = 3
	wantCode(t, f.from("S2"), 0, f)
	if len(f.w.merges) != 3 || f.sleeps < 3 {
		t.Errorf("merges %d sleeps %d", len(f.w.merges), f.sleeps)
	}
	stdoutHas(t, f, "  admin#1: ci  pending\n")
}

func TestReleaseRunPendingPastBudgetStopsRun(t *testing.T) {
	f := newTTY(t)
	f.w.on["pending_forever"] = true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "admin#1: required checks still pending after 45 minutes; nothing was merged. Look at the PR, then --from S2\n")
	if len(f.w.merges) != 0 {
		t.Error("merged")
	}
	if f.sleeps != checksBudget-1 {
		t.Errorf("sleeps %d want %d", f.sleeps, checksBudget-1)
	}
}

// TestReleaseRunStaleRemoteBranchGivesDeleteCommand is case 11: leftovers stop the run
// with a recovery line, not a raw git error.
func TestReleaseRunStaleRemoteBranchGivesDeleteCommand(t *testing.T) {
	f := newTTY(t)
	f.w.branches["admin/release/v1.5.0-lockstep"], f.w.on["branch_differs"] = true, true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: branch release/v1.5.0-lockstep already exists on admin with different content; delete it: gh api -X DELETE repos/nself-org/admin/git/refs/heads/release/v1.5.0-lockstep\n")
	if len(f.w.merges) != 0 || f.w.called("gh pr create") != 0 {
		t.Error("went on after the stale branch")
	}
}

func TestReleaseRunIdenticalRemoteBranchIsReused(t *testing.T) {
	f := newTTY(t)
	f.w.branches["admin/release/v1.5.0-lockstep"] = true
	wantCode(t, f.from("S2"), 0, f)
	if f.w.called("git -C "+f.workDir()+"/admin push -q origin release/v1.5.0-lockstep") != 0 {
		t.Error("pushed a branch that already matched")
	}
}

// TestReleaseRunNonCloneWorkDirSaysWhatToRemove is case 11b.
func TestReleaseRunNonCloneWorkDirSaysWhatToRemove(t *testing.T) {
	f := newTTY(t)
	f.seedWork("cli/junk", "x")
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: "+filepath.Join(f.workDir(), "cli")+" exists but is not a git clone; remove it ("+filepath.Join(f.workDir(), "cli")+"), then re-run\n")
	if _, err := os.Stat(filepath.Join(f.workDir(), "cli", "junk")); err != nil {
		t.Error("the junk dir was touched")
	}
}

// TestReleaseRunRevertedBumpGetsItsOwnMessage is case 17.
func TestReleaseRunRevertedBumpGetsItsOwnMessage(t *testing.T) {
	f := newTTY(t)
	f.w.fakeMergedPR("admin", "release/v1.5.0-lockstep", "ad0001")
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: PR 1 on admin is merged but main does not carry v1.5.0 (the bump was reverted or main was reset); delete branch release/v1.5.0-lockstep (gh api -X DELETE repos/nself-org/admin/git/refs/heads/release/v1.5.0-lockstep) and re-run to open a fresh PR\n")
	if len(f.w.merges) != 0 {
		t.Error("merged")
	}
}

func TestReleaseRunClosedPRIsNotReused(t *testing.T) {
	f := newTTY(t)
	f.w.prs = append(f.w.prs, &fpr{repo: "admin", head: "release/v1.5.0-lockstep", state: "CLOSED", n: 1})
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "PR 1 on admin was closed unmerged; delete branch release/v1.5.0-lockstep")
}

// TestReleaseRunOpenPRIsReused: an open PR from an earlier attempt is merged, not duplicated.
func TestReleaseRunOpenPRIsReused(t *testing.T) {
	f := newTTY(t)
	f.w.prs = append(f.w.prs, &fpr{repo: "admin", head: "release/v1.5.0-lockstep", state: "OPEN", n: 1})
	f.w.branches["admin/release/v1.5.0-lockstep"] = true
	wantCode(t, f.from("S2"), 0, f)
	stdoutHas(t, f, "S2 admin-lockstep: reusing admin#1 (OPEN)\n")
	if f.merged() != "admin cli homebrew-nself" {
		t.Errorf("merged %q", f.merged())
	}
}
