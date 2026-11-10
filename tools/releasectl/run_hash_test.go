package main

import (
	"os"
	"testing"
)

func tagged(t *testing.T) *rfix {
	f := newTTY(t)
	f.w.tags["cli"]["v1.5.0"] = "c0ffee1"
	return f
}

// TestReleaseRunS6DoneStillRerunsFailedLockstep is case 10: the formula is already right,
// S6 says done, merges nothing and still re-runs the failed lockstep job.
func TestReleaseRunS6DoneStillRerunsFailedLockstep(t *testing.T) {
	f := tagged(t)
	f.w.bumpBrew(true)
	f.w.bumpMains("admin")
	wantCode(t, f.from("S6"), 0, f)
	stdoutHas(t, f, "S6 homebrew-formula done\n", "S6 homebrew-formula: re-ran failed jobs of run 5551\n")
	if !f.w.hasCall("gh run rerun 5551 -R nself-org/cli --failed") || len(f.w.merges) != 0 {
		t.Errorf("rerun/merges wrong: %v %v", f.w.calls, f.w.merges)
	}
}

// TestReleaseRunInFlightPublishIsWaitedFor is case 10b: S8 waits for the run the Release
// started instead of dispatching a second publish.
func TestReleaseRunInFlightPublishIsWaitedFor(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.on["published"], f.w.on["inflight@cli"] = true, true
	wantCode(t, f.from("S8"), 0, f)
	stdoutHas(t, f, "S8 docker-verify: a docker-publish.yml run is in flight on cli; waiting for nself/nself:1.5.0\n", "S8 docker-verify: nself/nself:1.5.0 is up\n")
	if f.w.called("gh workflow run docker-publish.yml -R nself-org/cli") != 0 {
		t.Error("dispatched a second cli publish")
	}
	if f.w.tags["admin"]["v1.5.0"] == "" {
		t.Error("admin was not tagged")
	}
}

// TestReleaseRunStaleCachedRunIsLookedUpAgain is case 14: a poisoned state file and
// checksums dir never leak into the formula.
func TestReleaseRunStaleCachedRunIsLookedUpAgain(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.seedWork("checksums/checksums.txt", "poison  nself-1.5.0-darwin-arm64.tar.gz\n")
	os.WriteFile(f.workDir()+".state", []byte("run=9999\n"), 0o644)
	wantCode(t, f.from("S6"), 0, f)
	stdoutHas(t, f, "  run 9999 from an earlier attempt is not the build of v1.5.0; looking it up again\n")
	if f.w.formulaArm != armHash {
		t.Errorf("formula arm %s", f.w.formulaArm)
	}
}

// TestReleaseRunOldHashesAtTheVersionAreRewritten: the formula at 1.5.0 with 1.4.13 hashes
// is not "done".
func TestReleaseRunOldHashesAtTheVersionAreRewritten(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.bumpBrew(false)
	wantCode(t, f.from("S6"), 0, f)
	stdoutHas(t, f, "S6 homebrew-formula: formula is at 1.5.0 but its hashes differ from the build artifact of run 5551; rewriting\n")
	if f.w.formulaArm != armHash || f.w.formulaIntel != intelHash {
		t.Errorf("formula %s %s", f.w.formulaArm, f.w.formulaIntel)
	}
}

// TestReleaseRunPublishedChecksumsMismatchThenRewrite: S7 refuses a formula that differs
// from the published checksums; --from S6 rewrites it from the published file.
func TestReleaseRunPublishedChecksumsMismatchThenRewrite(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.on["published_differs"] = true
	wantCode(t, f.from("S6"), 1, f)
	stderrHas(t, f, "release: S7 release-wait failed: Formula/nself.rb on homebrew-nself main does not match the published checksums.txt (arm formula 554a93c77f0a vs published 639ecc2daad4; intel formula 6f823864cb70 vs published f565bd646fe9)",
		"brew install would fail its checksum. Re-write the formula from the published file\n",
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S6\n")
	f.out.Reset()
	f.err.Reset()
	wantCode(t, f.from("S6"), 0, f)
	if f.w.formulaArm != altArm {
		t.Errorf("formula arm %s want %s", f.w.formulaArm, altArm)
	}
}

// TestReleaseRunExpiredArtifactGivesTheRealRecovery is case 15.
func TestReleaseRunExpiredArtifactGivesTheRealRecovery(t *testing.T) {
	f := tagged(t)
	f.w.on["artifact_expired"] = true
	wantCode(t, f.from("S5"), 1, f)
	stderrHas(t, f, "release: S5 build-wait failed: no checksums.txt: the release-checksums artifact of run 5551 has expired (retention is 1 day) and the release is not published. Re-run the WHOLE run (gh run rerun 5551 -R nself-org/cli, not --failed), wait for its build to finish (checksums are fetched fresh each time, nothing cached to clear)\n")
}

func TestReleaseRunFailedGateJobStopsS6BeforeAnyMerge(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.on["gate_fail"] = true
	wantCode(t, f.from("S6"), 1, f)
	stderrHas(t, f, "release: S6 homebrew-formula failed: release.yml job 'Pre-Release Gate' failed in run 5551; the formula is not merged for a release that cannot publish. Fix the cause and re-run the failed jobs: gh run rerun 5551 -R nself-org/cli --failed\n")
	if len(f.w.merges) != 0 || f.w.called("gh pr create") != 0 {
		t.Error("merged or opened a PR")
	}
}

// TestReleaseRunS5WaitsForTheBuild: the build job is pending at first; S5 polls it.
func TestReleaseRunS5WaitsForTheBuild(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	wantCode(t, f.from("S5"), 0, f)
	stdoutHas(t, f, "  waiting for build job of run 5551 (1/120)\n", "S5 build-wait: build ok, checksums from the build artifact of run 5551\n")
}

func TestReleaseRunS5NoRunAppears(t *testing.T) {
	f := newTTY(t)
	wantCode(t, f.from("S5"), 1, f)
	stderrHas(t, f, "release: S5 build-wait failed: no release.yml run appeared for v1.5.0 (check the Actions tab of nself-org/cli)\n")
	stdoutHas(t, f, "  waiting for release.yml run for v1.5.0 (30/30)\n")
}

// TestReleaseRunS7Failures: a failed run names its failed jobs and every way back.
func TestReleaseRunS7Failures(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	f.w.on["runview_fail"] = true
	wantCode(t, f.from("S7"), 1, f)
	stderrHas(t, f, "release: S7 release-wait failed: cannot read run 5551: gh: HTTP 500\n")
}

func TestReleaseRunS7ReleaseNotPublished(t *testing.T) {
	f := tagged(t)
	f.w.bumpMains("admin")
	wantCode(t, f.from("S7"), 1, f)
	stderrHas(t, f, "release: S7 release-wait failed: release run 5551 concluded 'failure'; failed jobs: Verify Homebrew formula lockstep. 'Verify Homebrew formula lockstep': --from S6. 'Pre-Release Gate': satisfy the DENY rule in its log (the tag never moves), then gh run rerun 5551 -R nself-org/cli --failed. Any other job: fix, then gh run rerun 5551 -R nself-org/cli --failed. Then --from S7\n")
}
