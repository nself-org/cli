package main

import (
	"strings"
	"testing"
)

func published(t *testing.T) *rfix {
	f := tagged(t)
	f.w.on["published"] = true
	return f
}

// TestReleaseRunSdkApiErrorIsNotNothingToDispatch is case 16: a failed workflow list
// stops S10 instead of reading as "nothing to dispatch".
func TestReleaseRunSdkApiErrorIsNotNothingToDispatch(t *testing.T) {
	f := published(t)
	f.w.on["workflows_fail"] = true
	wantCode(t, f.from("S10"), 1, f)
	stderrHas(t, f, "release: S10 sdk-publish failed: cannot read the cli workflow list at v1.5.0: gh: HTTP 502\n")
	if strings.Contains(f.out.String(), "nothing to dispatch") {
		t.Error("treated an API error as nothing to dispatch")
	}
}

func TestReleaseRunSdkDispatchesOnTheTag(t *testing.T) {
	f := published(t)
	wantCode(t, f.from("S10"), 0, f)
	want := []string{
		"gh workflow run sdk-ts-publish.yml -R nself-org/cli --ref v1.5.0",
		"gh workflow run sdk-py-publish.yml -R nself-org/cli --ref v1.5.0",
		"gh workflow run sdk-publish-go.yml -R nself-org/cli --ref v1.5.0 -f version=1.5.0",
	}
	var got []string
	for _, c := range f.w.calls {
		if strings.HasPrefix(c, "gh workflow run") {
			got = append(got, c)
		}
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("dispatches:\n%s", strings.Join(got, "\n"))
	}
	stdoutHas(t, f, "S10 sdk-publish: sdk-ts-publish.yml succeeded\n", "S10 sdk-publish: sdk-publish-go.yml succeeded\n")
}

// TestReleaseRunSdkPendingRunIsWaitedFor: a run on the tag still going is waited for, not
// dispatched again.
func TestReleaseRunSdkPendingRunIsWaitedFor(t *testing.T) {
	f := published(t)
	f.w.count["sdkpending@sdk-ts-publish.yml"] = 2
	f.w.dispatched["sdk-ts-publish.yml@cli"] = true
	wantCode(t, f.from("S10"), 0, f)
	stdoutHas(t, f, "  waiting for sdk-ts-publish.yml run on v1.5.0 (1/90)\n", "S10 sdk-publish sdk-ts-publish.yml done\n")
	if f.w.called("gh workflow run sdk-ts-publish.yml") != 0 {
		t.Error("dispatched a workflow whose run succeeded")
	}
}

func TestReleaseRunSdkDispatchFailure(t *testing.T) {
	f := published(t)
	f.w.on["dispatch_fail"] = true
	wantCode(t, f.from("S10"), 1, f)
	stderrHas(t, f, "release: S10 sdk-publish failed: could not dispatch sdk-ts-publish.yml\n")
}

// TestReleaseRunAdminTagOnAnotherCommitIsRefused is case 16b.
func TestReleaseRunAdminTagOnAnotherCommitIsRefused(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.tags["admin"]["v1.5.0"] = "old"
	wantCode(t, f.from("S9"), 1, f)
	stderrHas(t, f, "release: S9 admin-image failed: admin tag v1.5.0 is at old but the lockstep release commit is adbump; tags are never moved, inspect by hand\n")
	if f.w.tags["admin"]["v1.5.0"] != "old" {
		t.Error("admin tag moved")
	}
}

// TestReleaseRunFailedTagPushPublishFallsBackToOneDispatch is case 16c.
func TestReleaseRunFailedTagPushPublishFallsBackToOneDispatch(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.on["admin_tagrun_fail"] = true
	wantCode(t, f.from("S9"), 0, f)
	if f.w.called("gh workflow run docker-publish.yml -R nself-org/admin -f version=1.5.0") != 1 {
		t.Errorf("want exactly one admin dispatch: %v", f.w.calls)
	}
	stdoutHas(t, f, "S9 admin-image: dispatched docker-publish.yml on admin\n", "S9 admin-image: nself/nself-admin:1.5.0 is up (linux/amd64, linux/arm64)\n")
}

// TestReleaseRunAdminTagGoesOnTheBumpCommit is case 18c: without a PR record the admin tag
// sits on the bump commit, not the tip of main.
func TestReleaseRunAdminTagGoesOnTheBumpCommit(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.mainSHA["admin"] = "adtip"
	wantCode(t, f.from("S9"), 0, f)
	if got := f.w.tags["admin"]["v1.5.0"]; got != "adbump" || got == f.w.mainSHA["admin"] {
		t.Errorf("admin tag %q, main %q", got, f.w.mainSHA["admin"])
	}
}

func TestReleaseRunAdminBumpCommitMissing(t *testing.T) {
	f := published(t)
	wantCode(t, f.from("S9"), 1, f)
	stderrHas(t, f, "release: S9 admin-image failed: no admin commit sets package.json to 1.5.0 and no merged lockstep PR was found; run S2\n",
		"--from S2\n")
}

func TestReleaseRunAdminCommitNotInMainHistory(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.notAncestor["adbump"] = true
	wantCode(t, f.from("S9"), 1, f)
	stderrHas(t, f, "the admin release commit adbump is not in origin/main's history; inspect before tagging\n")
}

func TestReleaseRunDockerImageNeverAppears(t *testing.T) {
	f := published(t)
	f.w.on["dispatch_fail"] = true
	wantCode(t, f.from("S8"), 1, f)
	stderrHas(t, f, "release: S8 docker-verify failed: could not dispatch docker-publish.yml on cli\n")
}

func TestReleaseRunImageAlreadyUpIsDone(t *testing.T) {
	f := published(t)
	f.w.dispatched["docker-publish.yml@cli"] = true
	f.w.bumpMains("admin")
	wantCode(t, f.from("S8"), 0, f)
	stdoutHas(t, f, "S8 docker-verify done\n")
	if f.w.called("gh workflow run docker-publish.yml -R nself-org/cli") != 0 {
		t.Error("dispatched although the image was up")
	}
}

// TestReleaseRunWebPRDoneWhenOpen: an existing web PR is reported, not duplicated.
func TestReleaseRunWebPRDoneWhenOpen(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.on["rerun"] = true
	f.w.prs = append(f.w.prs, &fpr{repo: "web", head: "release/cli-v1.5.0", state: "OPEN", n: 1})
	f.w.formulaVer, f.w.formulaArm, f.w.formulaIntel = ver, armHash, intelHash
	wantCode(t, f.from("S7"), 0, f)
	stdoutHas(t, f, "S7 release-wait web done (PR #1, OPEN)\n")
}

func TestReleaseRunWebLockAlreadyPinned(t *testing.T) {
	f := published(t)
	f.w.bumpMains("admin")
	f.w.formulaVer, f.w.formulaArm, f.w.formulaIntel = ver, armHash, intelHash
	f.w.on["nochange@web"], f.w.on["rerun"] = true, true
	wantCode(t, f.from("S7"), 0, f)
	stdoutHas(t, f, "S7 release-wait web done (lock already pins v1.5.0)\n")
}

func TestReleaseRunWebCommitFailureIsReported(t *testing.T) {
	for _, flag := range []string{"git_add_fail", "git_commit_fail"} {
		f := published(t)
		f.w.bumpMains("admin")
		f.w.formulaVer, f.w.formulaArm, f.w.formulaIntel = ver, armHash, intelHash
		f.w.on["rerun"] = true
		f.w.on[flag] = true
		if code := f.from("S7"); code == 0 {
			t.Fatalf("%s: exit 0", flag)
		}
		if strings.Contains(f.out.String(), "lock already pins") {
			t.Fatalf("%s: reported as already pinned", flag)
		}
		if !strings.Contains(f.err.String(), "failed in web") {
			t.Fatalf("%s: stderr %q", flag, f.err.String())
		}
	}
}
