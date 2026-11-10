package main

import (
	"reflect"
	"strings"
	"testing"
)

const (
	protAPI = "gh api repos/nself-org/admin/branches/main/protection/required_status_checks"
	prCheck = "gh pr checks 7 -R nself-org/admin --required"
	prJSON  = "gh pr checks 7 -R nself-org/admin --required --json"
)

func TestReleaseLibRequiredContextsReadsNames(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{out: "build\ntest\n"})})
	if got := e.r.requiredContexts("admin"); !reflect.DeepEqual(got, []string{"build", "test"}) {
		t.Fatalf("got %q", got)
	}
}

func TestReleaseLibRequiredContextsNotProtectedIsEmpty(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{code: 1, stderr: "gh: Branch not protected (HTTP 404)"})})
	if got := e.r.requiredContexts("admin"); got != nil {
		t.Fatalf("got %q, want none", got)
	}
}

func TestReleaseLibRequiredContextsUnreadable(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{code: 1, stderr: "gh: HTTP 502"})})
	if got := e.r.requiredContexts("admin"); !reflect.DeepEqual(got, []string{"?"}) {
		t.Fatalf("got %q", got)
	}
}

func TestReleaseLibMissingRequiredAllPassingOrSkipped(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{out: "build\nlint\n"}), prJSON: one(reply{out: "build\nlint\nextra\n"})})
	if got := e.r.missingRequired("admin", "7"); got != nil {
		t.Fatalf("got %q, want none", got)
	}
}

func TestReleaseLibMissingRequiredNamesAbsentOnes(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{out: "build\nlint\ntest\n"}), prJSON: one(reply{out: "lint\n"})})
	if got := e.r.missingRequired("admin", "7"); !reflect.DeepEqual(got, []string{"build", "test"}) {
		t.Fatalf("got %q", got)
	}
}

func TestReleaseLibMissingRequiredUnreadablePRChecks(t *testing.T) {
	e := newRel(t, map[string][]reply{protAPI: one(reply{out: "build\n"}), prJSON: one(reply{code: 1, stderr: "boom"})})
	if got := e.r.missingRequired("admin", "7"); !reflect.DeepEqual(got, []string{"?"}) {
		t.Fatalf("got %q", got)
	}
}

func TestReleaseLibWaitChecksGreenEchoesFirstFourLines(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: one(reply{out: "a\tpass\nb\tpass\nc\tpass\nd\tpass\ne\tpass\n"}),
		protAPI: one(reply{out: "a\n"}), prJSON: one(reply{out: "a\n"}),
	})
	if err := e.r.WaitChecks("S4", "merge-admin", "admin", "7"); err != nil {
		t.Fatal(err)
	}
	want := "  admin#7: a\tpass\n  admin#7: b\tpass\n  admin#7: c\tpass\n  admin#7: d\tpass\n"
	if e.out.String() != want {
		t.Fatalf("stdout %q, want %q", e.out.String(), want)
	}
}

func TestReleaseLibWaitChecksRedStopsWithExactMessage(t *testing.T) {
	e := newRel(t, map[string][]reply{prCheck: one(reply{out: "build\tfail\n", code: 1})})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	want := "release: S4 merge-admin failed: required checks on admin#7 are not green; nothing was merged\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S4"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v\nwant %s", err, want)
	}
}

func TestReleaseLibWaitChecksPendingTimesOutAfter90Polls(t *testing.T) {
	e := newRel(t, map[string][]reply{prCheck: one(reply{out: "build\tpending\n", code: 8})})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	want := "release: S4 merge-admin failed: admin#7: required checks still pending after 45 minutes; nothing was merged. Look at the PR, then --from S4\n" +
		"release: recovery: fix the cause, then run: scripts/release/release.sh --version 1.5.0 --from S4"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v", err)
	}
	if e.run.count("gh pr checks") != 90 || e.sleeps != 89 {
		t.Fatalf("polls %d sleeps %d, want 90 and 89", e.run.count("gh pr checks"), e.sleeps)
	}
}

func TestReleaseLibWaitChecksPendingThenGreen(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: {{out: "x\n", code: 8}, {out: "x\n"}},
		protAPI: one(reply{code: 1, stderr: "404 Not Found"}),
	})
	if err := e.r.WaitChecks("S4", "merge-admin", "admin", "7"); err != nil {
		t.Fatal(err)
	}
	if e.sleeps != 1 {
		t.Fatalf("sleeps %d, want 1", e.sleeps)
	}
}

func TestReleaseLibWaitChecksNoChecksAndNoneRequired(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: one(reply{out: "no required checks reported on the 'x' branch\n", code: 1}),
		protAPI: one(reply{code: 1, stderr: "Branch not protected"}),
	})
	if err := e.r.WaitChecks("S4", "merge-admin", "admin", "7"); err != nil {
		t.Fatal(err)
	}
	want := "  admin#7: no required checks reported on the 'x' branch\n" +
		"S4 merge-admin: admin requires no status checks on main; continuing to the owner prompt\n"
	if e.out.String() != want {
		t.Fatalf("stdout %q, want %q", e.out.String(), want)
	}
}

func TestReleaseLibWaitChecksNoChecksButRequiredGivesUpAfterSix(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: one(reply{out: "no checks reported\n", code: 1}),
		protAPI: one(reply{out: "build\ntest\n"}),
	})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	if err == nil || !strings.Contains(err.Error(), "S4 merge-admin failed: admin#7: required checks not reported as passing after 6 tries (build test); nothing was merged\n") {
		t.Fatalf("got %v", err)
	}
	if e.run.count("gh pr checks") != 6 || e.sleeps != 5 {
		t.Fatalf("polls %d sleeps %d", e.run.count("gh pr checks"), e.sleeps)
	}
}

func TestReleaseLibWaitChecksNoChecksUnreadableProtection(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: one(reply{out: "no checks reported\n", code: 1}),
		protAPI: one(reply{code: 1, stderr: "HTTP 500"}),
	})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	if err == nil || !strings.Contains(err.Error(), "S4 merge-admin failed: cannot read the required checks of admin; nothing was merged\n") {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseLibWaitChecksGreenButContextMissingGivesUp(t *testing.T) {
	e := newRel(t, map[string][]reply{
		prCheck: one(reply{out: "lint\tpass\n"}),
		protAPI: one(reply{out: "build\nlint\n"}), prJSON: one(reply{out: "lint\n"}),
	})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	if err == nil || !strings.Contains(err.Error(), "after 6 tries (build ); nothing was merged\n") {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseLibWaitChecksGreenButUnreadableRequired(t *testing.T) {
	e := newRel(t, map[string][]reply{prCheck: one(reply{out: "ok\n"}), protAPI: one(reply{code: 1, stderr: "HTTP 500"})})
	err := e.r.WaitChecks("S4", "merge-admin", "admin", "7")
	if err == nil || !strings.Contains(err.Error(), "S4 merge-admin failed: cannot read the required checks of admin#7; nothing was merged\n") {
		t.Fatalf("got %v", err)
	}
}
