package main

import (
	"strings"
	"testing"
)

// TestReleaseRunPreflightFailures: each failing precondition names S1 and the cause.
func TestReleaseRunPreflightFailures(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*rfix)
		want  string
	}{
		{"missing tool", func(f *rfix) { f.d.Have = func(t string) bool { return t != "node" } }, "release: S1 preflight failed: missing tool: node\n"},
		{"not logged in", func(f *rfix) { f.w.on["auth_fail"] = true }, "release: S1 preflight failed: gh is not logged in (run: gh auth login)\n"},
		{"clone fails", func(f *rfix) { f.w.on["clone_fail"] = true }, "release: S1 preflight failed: clone of cli failed\n"},
		{"facts need go", func(f *rfix) {
			f.w.facts = true
			f.d.Have = func(t string) bool { return t != "go" }
		}, "release: S1 preflight failed: missing tool: go (cli has tools/facts; S3 regenerates facts)\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTTY(t)
			c.setup(f)
			wantCode(t, f.go1(), 1, f)
			stderrHas(t, f, c.want)
		})
	}
}

// TestReleaseRunPreflightWarnsAboutDockerHubToken: a hidden token is a warning, not a stop.
func TestReleaseRunPreflightWarnsAboutDockerHubToken(t *testing.T) {
	f := newTTY(t)
	f.w.on["no_secret"] = true
	f.run("--version", ver, "--yes", "--from", "S1", "--dry-run") // plan only: no calls
	if len(f.w.calls) != 0 {
		t.Fatalf("dry-run called %v", f.w.calls)
	}
	g := newTTY(t)
	g.w.on["no_secret"], g.w.on["bump_fail"] = true, true
	wantCode(t, g.go1(), 1, g) // stops at S2, after S1 passed with the warning
	stderrHas(t, g, "release: S1 preflight: warning: DOCKERHUB_TOKEN not visible to nself-org/cli (P7-PROD-61); S8/S9 will fail without it\n")
	stdoutHas(t, g, "S1 preflight: cli main is at 1.4.13, releasing 1.5.0; work dir "+g.workDir()+"\n")
}

// TestReleaseRunFactsRegenerationFailure and the go requirement of S3.
func TestReleaseRunFactsRegenerationFailure(t *testing.T) {
	f := newTTY(t)
	f.w.facts = true
	f.w.bumpMains("admin")
	f.w.on["facts_fail"] = true
	wantCode(t, f.from("S3"), 1, f)
	stderrHas(t, f, "release: S3 cli-version failed: facts regeneration failed\n")
	if f.w.called("gh pr create") != 0 {
		t.Error("opened a PR after the facts failure")
	}
}

// TestReleaseRunBumpChangedNothing: a bump that edits nothing stops the step.
func TestReleaseRunBumpChangedNothing(t *testing.T) {
	f := newTTY(t)
	f.w.on["nochange@admin"] = true
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: bump changed nothing in admin\n")
}

// TestReleaseRunPRFailures: gh pr create, the push and the merge each end the step.
func TestReleaseRunPRFailures(t *testing.T) {
	cases := []struct {
		name, flag, want string
	}{
		{"push refused", "push_refused", "push of release/v1.5.0-lockstep to admin was refused"},
		{"pr create fails", "prcreate_fail", "gh pr create failed on admin"},
		{"branch checkout fails", "git_checkout_fail", "could not refresh the clone of cli; remove "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newTTY(t)
			f.w.on[c.flag] = true
			wantCode(t, f.from("S2"), 1, f)
			stderrHas(t, f, "release: S2 admin-lockstep failed: "+c.want)
		})
	}
}

// TestReleaseRunDeclinedAdminMergeFails: the owner types n at admin-merge's prompt.
func TestReleaseRunDeclinedAdminMergeFails(t *testing.T) {
	f := newTTY(t)
	f.w.typed = "n"
	wantCode(t, f.from("S2"), 1, f)
	stderrHas(t, f, "release: S2 admin-lockstep failed: admin-merge.sh failed on admin#1 (check branch protection: it restores itself; see its audit log)\n")
	if got := strings.Join(f.w.merges, "|"); got != "repo=admin pr=1 tty=yes ans=n" {
		t.Errorf("merges %q", got)
	}
	if f.w.adminVer != "1.4.13" {
		t.Error("admin was bumped without the owner's yes")
	}
}

// TestReleaseRunHomebrewPRStates: a closed or merged-but-missing formula PR is explained.
func TestReleaseRunHomebrewPRStates(t *testing.T) {
	br := "release/nself-v1.5.0-554a93c7"
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.prs = append(f.w.prs, &fpr{repo: "homebrew-nself", head: br, state: "CLOSED", n: 1})
	wantCode(t, f.from("S6"), 1, f)
	stderrHas(t, f, "release: S6 homebrew-formula failed: PR 1 was closed unmerged; delete branch "+br+" (gh api -X DELETE repos/nself-org/homebrew-nself/git/refs/heads/"+br+") and retry\n")
	g := tagged(t)
	g.w.bumpMains("admin")
	g.w.fakeMergedPR("homebrew-nself", br, "x")
	wantCode(t, g.from("S6"), 1, g)
	stderrHas(t, g, "release: S6 homebrew-formula failed: PR 1 for these hashes is merged but Formula/nself.rb on main does not carry them (reverted?); restore the formula by hand, then --from S6\n")
}

func TestReleaseRunFormulaScriptFailures(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.on["formula_fail"] = true
	wantCode(t, f.from("S6"), 1, f)
	stderrHas(t, f, "release: S6 homebrew-formula failed: update-formula.sh refused the checksums\n")
	g := tagged(t)
	g.w.bumpMains("admin")
	g.w.on["nochange@homebrew-nself"] = true
	wantCode(t, g.from("S6"), 1, g)
	stderrHas(t, g, "release: S6 homebrew-formula failed: formula unchanged\n")
}

// TestReleaseRunChecksumsGoneButFormulaAtVersion: S6 is done and says why it cannot re-check.
func TestReleaseRunChecksumsGoneButFormulaAtVersion(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.bumpBrew(true)
	f.w.on["artifact_expired"], f.w.on["published"] = true, false
	wantCode(t, f.from("S6"), 0, f) // the lockstep re-run publishes the release, so S7 passes
	stdoutHas(t, f, "S6 homebrew-formula: formula is at 1.5.0; checksums are gone so the hashes cannot be re-checked here (S7 checks them against the published file)\n", "S6 homebrew-formula done\n")
}

func TestReleaseRunNodeMissingForWebLock(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.on["rerun"] = true
	f.w.formulaVer, f.w.formulaArm, f.w.formulaIntel = ver, armHash, intelHash
	f.d.Have = func(t string) bool { return t != "node" }
	wantCode(t, f.from("S7"), 1, f)
	stderrHas(t, f, "release: S7 release-wait failed: node is not installed (needed for update-lock.mjs)\n")
}
