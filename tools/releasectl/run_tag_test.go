package main

import (
	"strings"
	"testing"
)

func tagWorld(t *testing.T) *rfix {
	f := newTTY(t)
	f.w.bumpMains()
	return f
}

func (f *rfix) cliTag() string { return f.w.tags["cli"]["v1.5.0"] }

// TestReleaseRunBumpedMainsMakeS2S3Done is case 9: with the bumps already on main, S2 and
// S3 say done, only the formula merges, and the tag is created.
func TestReleaseRunBumpedMainsMakeS2S3Done(t *testing.T) {
	f := tagWorld(t)
	wantCode(t, f.from("S2"), 0, f)
	stdoutHas(t, f, "S2 admin-lockstep done\n", "S3 cli-version done\n")
	if f.merged() != "homebrew-nself" {
		t.Errorf("merged %q", f.merged())
	}
	if f.cliTag() != "c0ffee1" {
		t.Errorf("tag %q", f.cliTag())
	}
	stdoutHas(t, f, "S4 tag: v1.5.0 -> c0ffee1 (release.yml creates the release)\n")
}

// TestReleaseRunDenylistRulesStopTheTag covers DENY-003/004/005 at tag time: each stops
// S4 before any tag exists and names its rule.
func TestReleaseRunDenylistRulesStopTheTag(t *testing.T) {
	cases := []struct {
		name string
		flag string
		want string
	}{
		{"DENY-003", "zero_commits", "release: S4 tag failed: DENY-003: no commits since v1.4.13; merge the version bump (S3) first\n"},
		{"DENY-004 failed run", "license_fail", "DENY-004: the newest license-audit.yml run on main is 'failure'. Fix the flagged licenses, run: gh workflow run license-audit.yml -R nself-org/cli --ref main, wait for it to pass, then --from S4\n"},
		{"DENY-004 none", "license_none", "DENY-004: no license-audit.yml run on main. Start one: gh workflow run license-audit.yml -R nself-org/cli --ref main, wait for it to pass, then --from S4\n"},
		{"DENY-005 none", "no_milestone", "DENY-005: no closed milestone titled exactly v1.5.0 (a milestone named 1.5.0 does not count); create or close it, then --from S4\n"},
		{"DENY-005 bare name", "only_bare_milestone", "DENY-005: no closed milestone titled exactly v1.5.0"},
		{"DENY-005 open duplicate", "dup_open_milestone", "DENY-005: a milestone titled v1.5.0 is not closed (an open duplicate?); close or delete the duplicate so exactly one closed milestone has that title, then --from S4\n"},
		{"DENY-005 two closed", "dup_closed_milestone", "DENY-005: more than one milestone is titled v1.5.0; pre-release-gate.yml needs exactly one (closed). Delete the duplicates, then --from S4\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := tagWorld(t)
			f.w.on[c.flag] = true
			wantCode(t, f.from("S4"), 1, f)
			stderrHas(t, f, c.want)
			if f.cliTag() != "" || f.w.called("git -C "+f.workDir()+"/cli push") != 0 {
				t.Errorf("a tag was created or pushed: %v", f.w.calls)
			}
		})
	}
}

// TestReleaseRunPRBranchLicenseRunNoLongerStopsTheTag is release/test.sh's
// "DENY-004 PR-branch run" case with its new answer: release.sh stopped S4 on a
// newer failed PR-branch run because pre-release-gate.yml read any branch. The
// gate now runs `releasectl gate`, which reads main, so S4 reads main only and
// the tag is made.
func TestReleaseRunPRBranchLicenseRunNoLongerStopsTheTag(t *testing.T) {
	f := tagWorld(t)
	f.w.on["license_pr_fail"] = true
	wantCode(t, f.from("S4"), 0, f)
	if f.cliTag() == "" {
		t.Error("no tag")
	}
	for _, c := range f.w.calls {
		if strings.Contains(c, "--workflow license-audit.yml") && !strings.Contains(c, "--branch main") {
			t.Errorf("license-audit read off main: %q", c)
		}
	}
}

// TestReleaseRunLicenseAuditInFlight is case 12: an in-flight run is waited for; one that
// never ends stops S4 with no tag.
func TestReleaseRunLicenseAuditInFlight(t *testing.T) {
	f := tagWorld(t)
	f.w.count["license_inflight"] = 3
	wantCode(t, f.from("S4"), 0, f)
	stdoutHas(t, f, "S4 tag: license-audit.yml (main) is still running; waiting up to 30 minutes\n")
	if f.cliTag() == "" {
		t.Error("no tag after the wait")
	}
	g := tagWorld(t)
	g.w.count["license_inflight"] = 999
	wantCode(t, g.from("S4"), 1, g)
	stderrHas(t, g, "DENY-004: license-audit.yml on main is still running after 30 minutes; let it finish, then --from S4\n")
	if g.cliTag() != "" {
		t.Error("tagged with the audit still running")
	}
}

// TestReleaseRunTagOnAnotherCommitIsRefused: a tag is never moved.
func TestReleaseRunTagOnAnotherCommitIsRefused(t *testing.T) {
	f := tagWorld(t)
	f.w.tags["cli"]["v1.5.0"] = "old"
	f.w.on["tag_version_wrong"] = true
	wantCode(t, f.from("S4"), 1, f)
	stderrHas(t, f, "release: S4 tag failed: tag v1.5.0 exists but its .github/VERSION is not 1.5.0; tags are never moved, inspect by hand\n")
	if f.cliTag() != "old" {
		t.Errorf("tag moved to %q", f.cliTag())
	}
	g := tagWorld(t)
	g.w.fakeMergedPR("cli", "release/v1.5.0", "merged1")
	g.w.tags["cli"]["v1.5.0"] = "old"
	wantCode(t, g.from("S4"), 1, g)
	stderrHas(t, g, "tag v1.5.0 is at old but the merged release commit is merged1; tags are never moved, inspect by hand\n")
}

// TestReleaseRunMovedMainTagsTheReviewedCommit is case 13.
func TestReleaseRunMovedMainTagsTheReviewedCommit(t *testing.T) {
	f := tagWorld(t)
	f.w.fakeMergedPR("cli", "release/v1.5.0", "rev1")
	f.w.mainSHA["cli"] = "moved2"
	wantCode(t, f.from("S4"), 0, f)
	if f.cliTag() != "rev1" || f.cliTag() == f.w.mainSHA["cli"] {
		t.Errorf("tag %q main %q", f.cliTag(), f.w.mainSHA["cli"])
	}
	stdoutHas(t, f, "S4 tag: origin/main (moved2) moved on; tagging the reviewed release commit rev1\n")
}

func TestReleaseRunMergeCommitOutsideHistoryIsRefused(t *testing.T) {
	f := tagWorld(t)
	f.w.fakeMergedPR("cli", "release/v1.5.0", "side1")
	f.w.mainSHA["cli"] = "moved2"
	f.w.notAncestor["side1"] = true
	wantCode(t, f.from("S4"), 1, f)
	stderrHas(t, f, "the merged release commit side1 is not in origin/main's history (rewritten or reverted); inspect before tagging\n")
	if f.cliTag() != "" {
		t.Error("tagged")
	}
}

// TestReleaseRunTagPushChecks: refused push, wrong remote answer, stale version.
func TestReleaseRunTagPushChecks(t *testing.T) {
	cases := []struct{ flag, want string }{
		{"push_refused", "release: S4 tag failed: push of v1.5.0 was refused\n"},
		{"tag_create_fail", "could not create the local tag (a stale v1.5.0 in "},
		{"lsremote_empty", "tag v1.5.0 on the remote is '', expected c0ffee1\n"},
		{"show_fail", "cannot read .github/VERSION at c0ffee1\n"},
		{"revlist_fail", "cannot count commits since v1.4.13\n"},
	}
	for _, c := range cases {
		f := tagWorld(t)
		f.w.on[c.flag] = true
		wantCode(t, f.from("S4"), 1, f)
		if !strings.Contains(f.err.String(), c.want) {
			t.Errorf("%s: stderr %q lacks %q", c.flag, f.err.String(), c.want)
		}
	}
	f := tagWorld(t)
	f.w.cliVer = "1.4.99"
	wantCode(t, f.from("S4"), 1, f)
	stderrHas(t, f, "c0ffee1 carries version 1.4.99, not 1.5.0 (S3 not merged?)\n")
}
