package main

import (
	"fmt"
	"strings"
	"testing"
)

const (
	pubDown = "gh release download v1.5.0 -R nself-org/cli -p checksums.txt -D "
	jobsAPI = "gh run view 99 -R nself-org/cli --json jobs --jq"
)

func publishedRules(sums, formula string) map[string][]reply {
	return map[string][]reply{pubDown: one(reply{hook: writeSums(sums)}), formApi: one(reply{out: formula})}
}

func goodFormula(t *testing.T) string {
	f := fixture(t, "nself.rb")
	f = strings.Replace(f, `version "1.4.13"`, `version "1.5.0"`, 1)
	f = strings.Replace(f, "73432f349295c10e6f28621d92fdc7736284ecb8287b5df697f98d5289acd138", "554a93c77f0a4d50fabbdc3ed7bd01638f092ce082f2d54d866d659d662bfd77", 1)
	return strings.Replace(f, "eda10ee8bd97ae6707c92bef4a77624fa26883e5bea39efb2c413c18cf2aa809", "6f823864cb70bb06e0d87f751db0fafcf96375da658da2b281b0fb1d9a4deeba", 1)
}

func TestReleaseLibVerifyPublishedMatches(t *testing.T) {
	e := newRel(t, publishedRules(fixture(t, "checksums.txt"), goodFormula(t)))
	if err := e.r.VerifyPublished(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseLibVerifyPublishedMismatchExactMessage(t *testing.T) {
	e := newRel(t, publishedRules(fixture(t, "checksums-alt.txt"), goodFormula(t)))
	err := e.r.VerifyPublished()
	want := "release: S7 release-wait failed: Formula/nself.rb on homebrew-nself main does not match the published checksums.txt " +
		"(arm formula 554a93c77f0a vs published 639ecc2daad4; intel formula 6f823864cb70 vs published f565bd646fe9). " +
		"brew install would fail its checksum. Re-write the formula from the published file\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S6"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v\nwant %s", err, want)
	}
}

func TestReleaseLibVerifyPublishedOldFormulaVersionFails(t *testing.T) {
	e := newRel(t, publishedRules(fixture(t, "checksums.txt"), fixture(t, "nself.rb")))
	err := e.r.VerifyPublished()
	if err == nil || !strings.Contains(err.Error(), "(arm formula 73432f349295 vs published 554a93c77f0a; intel formula eda10ee8bd97 vs published 6f823864cb70)") {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseLibVerifyPublishedMissingDarwinLines(t *testing.T) {
	e := newRel(t, publishedRules("aaa  nself-1.5.0-linux-amd64.tar.gz\n", goodFormula(t)))
	err := e.r.VerifyPublished()
	want := "release: S7 release-wait failed: the published checksums.txt has no darwin arm64/amd64 lines\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S7"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
	if e.run.count("gh api -H") != 0 {
		t.Fatal("formula must not be read after a bad checksums file")
	}
}

func gateRules(state ...reply) map[string][]reply {
	return map[string][]reply{tagAPI: one(reply{out: "sha1\n"}), runList: one(reply{out: "99\n"}), jobsAPI: state}
}

func TestReleaseLibReleaseGateOK(t *testing.T) {
	e := newRel(t, gateRules(reply{out: "ok\n"}))
	if err := e.r.ReleaseGate(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseLibReleaseGateWaitsWhilePending(t *testing.T) {
	e := newRel(t, gateRules(reply{out: "pending\n"}, reply{out: "ok\n"}))
	if err := e.r.ReleaseGate(); err != nil {
		t.Fatal(err)
	}
	if want := "  waiting for release.yml jobs (build, sbom, provenance, Pre-Release Gate) (1/120)\n"; e.out.String() != want {
		t.Fatalf("stdout %q, want %q", e.out.String(), want)
	}
}

func TestReleaseLibReleaseGateFailedJobExactMessage(t *testing.T) {
	e := newRel(t, gateRules(reply{out: "failed:sbom\n"}))
	err := e.r.ReleaseGate()
	want := "release: S6 homebrew-formula failed: release.yml job 'sbom' failed in run 99; the formula is not merged for a release that cannot publish. " +
		"Fix the cause and re-run the failed jobs: gh run rerun 99 -R nself-org/cli --failed\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S6"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseLibReleaseGateStillPendingAfterBudget(t *testing.T) {
	e := newRel(t, gateRules(reply{out: "pending\n"}))
	err := e.r.ReleaseGate()
	if err == nil || !strings.Contains(err.Error(), "release.yml jobs are still running after the wait budget (run 99)") {
		t.Fatalf("got %v", err)
	}
	if e.sleeps != 120 {
		t.Fatalf("sleeps %d, want 120", e.sleeps)
	}
}

func TestReleaseLibReleaseGateNoRunResumesAtS5(t *testing.T) {
	e := newRel(t, map[string][]reply{tagAPI: one(reply{code: 1, stderr: "404"})})
	err := e.r.ReleaseGate()
	want := "release: S6 homebrew-formula failed: no release.yml run found for v1.5.0 (run S5 first)\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S5"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
}

// pagedRunner answers the milestone call like `gh api --jq`: without
// --paginate only the first 30 (newest) milestones are seen, and v1.4.12 is
// the 35th, so only a paginated call finds it.
type pagedRunner struct{}

func (pagedRunner) Run(name string, args ...string) (string, error) {
	if name != "gh" || len(args) == 0 || args[0] != "api" {
		return "", nil
	}
	seen := 30
	for _, a := range args {
		if a == "--paginate" {
			seen = 35
		}
	}
	for i := 1; i <= seen; i++ {
		if fmt.Sprintf("v1.4.%d", 47-i) == "v1.4.12" {
			return "closed\n", nil
		}
	}
	return "", nil
}

func TestReleaseCtlCreateRetractMilestone31PlusClosedUsesPagination(t *testing.T) {
	e := newEnv(t, "")
	e.d.Runner = pagedRunner{}
	if !e.d.ruleMilestone("v1.4.12") {
		t.Fatal("DENY-005 missed the milestone")
	}
}

func TestReleaseCtlCreateRetractMilestoneCallPaginatesAllStates(t *testing.T) {
	e := newEnv(t, "")
	e.d.ruleMilestone("v1.4.12")
	call := ""
	for _, c := range e.r.calls {
		if strings.HasPrefix(c, "gh api") {
			call = c
		}
	}
	if !strings.Contains(call, "--paginate") || !strings.Contains(call, "state=all") {
		t.Fatalf("not paginated: %s", call)
	}
}

func TestReleaseCtlCreateRetractGateVerbPassAndRefuse(t *testing.T) {
	e := newEnv(t, "")
	e.r.out["git -C "+e.d.Root+" tag"] = "v1.0.0\n"
	e.r.out["git -C "+e.d.Root+" rev-list"] = "3\n"
	if code := RunWith(e.d, []string{"gate", "v1.0.1"}); code != 0 {
		t.Fatalf("code %d: %s", code, e.err.String())
	}
	e.r.out["gh api"] = ""
	if code := RunWith(e.d, []string{"gate", "v1.0.1"}); code != 1 || !strings.Contains(e.err.String(), "DENY-005") {
		t.Fatalf("code %d: %s", code, e.err.String())
	}
}
