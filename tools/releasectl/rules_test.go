package main

import (
	"strings"
	"testing"
)

// The tag is the first argument that is not --dry-run or --skip-denylist, so
// `create --skip-denylist v1` overrides for v1 instead of tagging the flag.
func TestReleaseCtlCreateRetractCreateFlagsBeforeTag(t *testing.T) {
	e := newEnv(t, "")
	RunWith(e.d, []string{"create", "--skip-denylist", "--dry-run", "v1.2.3", "--draft"})
	wantEq(t, "stdout", e.out.String(), "[release-create] invoking: gh release create v1.2.3 --draft\n")
	e = newEnv(t, "")
	if code := RunWith(e.d, []string{"create", "--draft", "v1.2.3"}); code != 2 {
		t.Fatalf("a gh flag in the tag place must be refused, got %d", code)
	}
	wantContains(t, "stderr", e.err.String(), "ERROR: usage: releasectl create <tag>")
	if len(e.r.calls) != 0 {
		t.Errorf("calls %v", e.r.calls)
	}
}

// gh's exit code becomes ours, as with the script's `exec gh`.
func TestReleaseCtlCreateRetractCreateKeepsGhExitCode(t *testing.T) {
	e := newEnv(t, "")
	e.r.code = map[string]int{"gh release create": 4}
	if code := RunWith(e.d, []string{"create", "v9.9.9"}); code != 4 {
		t.Fatalf("code %d want 4", code)
	}
	if len(e.r.ttys) != 1 {
		t.Errorf("not run on the terminal: %v", e.r.ttys)
	}
}

// DENY-003 fails closed: an unreadable tag list is not a first release.
func TestReleaseCtlCreateRetractTagListErrorBlocks(t *testing.T) {
	e := newEnv(t, "")
	e.r.fail["git -C "+e.d.Root+" tag"] = true
	if e.d.ruleCommits("v1.0.1") {
		t.Fatal("DENY-003 passed with no tag list")
	}
	wantEq(t, "stderr", e.err.String(), "[release-create] FAIL: cannot list git tags (DENY-003): fake failure\n")
	if strings.Contains(e.out.String(), "first release") {
		t.Errorf("treated as a first release: %q", e.out)
	}
	e = newEnv(t, "")
	e.r.fail["git -C "+e.d.Root+" tag"] = true
	if code := RunWith(e.d, []string{"gate", "v1.0.1"}); code != 1 {
		t.Fatalf("gate code %d", code)
	}
	wantContains(t, "stderr", e.err.String(), "::error::  - DENY-003 zero-commits-since-prior-tag\n")
}

// DENY-005 needs exactly one milestone with the title, and it closed: the rule
// S4 applies and the old gate's `[ "$MILESTONE" != closed ]` enforced.
func TestReleaseCtlCreateRetractMilestoneExactlyOneClosed(t *testing.T) {
	for _, c := range []struct {
		out  string
		pass bool
	}{
		{"closed\n", true},
		{"closed", true},
		{"", false},
		{"open\n", false},
		{"open\nclosed\n", false},
		{"closed\nclosed\n", false},
	} {
		e := newEnv(t, "")
		e.r.out["gh api"] = c.out
		if got := e.d.ruleMilestone("v1.0.1"); got != c.pass {
			t.Errorf("milestones %q: pass=%v want %v", c.out, got, c.pass)
		}
	}
}
