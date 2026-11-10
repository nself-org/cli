package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCtlCreateRetractRetractArgErrors(t *testing.T) {
	e := newEnv(t, "")
	if RunWith(e.d, []string{"retract"}) != 2 {
		t.Error("tag required")
	}
	wantContains(t, "stderr", e.err.String(), "ERROR: usage: releasectl retract <tag>")
	e = newEnv(t, "")
	if RunWith(e.d, []string{"retract", "v1.0.0"}) != 2 {
		t.Error("reason required")
	}
	if want := "ERROR: --reason is required (or set RELEASE_RETRACT_REASON env var)\n"; e.err.String() != want {
		t.Errorf("stderr %q", e.err)
	}
	if len(e.r.calls) != 0 {
		t.Errorf("calls %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractRetractRefusesNonLatest(t *testing.T) {
	e := newEnv(t, "")
	e.r.out["gh release view --json tagName"] = "v2.0.0\n"
	code := RunWith(e.d, []string{"retract", "v1.0.0", "--reason", "bad"})
	if code != 1 {
		t.Fatalf("code %d", code)
	}
	if want := "ERROR: v1.0.0 is not the latest release; use --force to retract anyway\n"; e.err.String() != want {
		t.Errorf("stderr %q", e.err)
	}
	if e.r.called("gh release edit") {
		t.Errorf("edited: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractRetractLatestRunsSteps(t *testing.T) {
	e := newEnv(t, "")
	e.r.out["gh release view --json tagName"] = "v1.0.0\n"
	e.r.out["gh release view v1.0.0 --json"] = "old notes\n"
	e.d.Env = func(k string) string { return map[string]string{"RELEASE_RETRACT_REASON": "env why", "USER": "u"}[k] }
	if code := RunWith(e.d, []string{"retract", "v1.0.0"}); code != 0 {
		t.Fatalf("code %d %s", code, e.err)
	}
	want := []string{
		"gh release view v1.0.0",
		"gh release view --json tagName -q .tagName",
		"gh release edit v1.0.0 --draft",
		"gh release view v1.0.0 --json body -q .body",
		"gh release edit v1.0.0 --notes ## RETRACTED 2026-11-07T01:02:03Z\n\n**Reason:** env why\n\n---\n\nold notes",
	}
	for i, w := range want {
		if i >= len(e.r.calls) {
			t.Fatalf("only %d calls, want %q next (all %v)", len(e.r.calls), w, e.r.calls)
		}
		if e.r.calls[i] != w {
			t.Fatalf("call %d = %q want %q (all %v)", i, e.r.calls[i], w, e.r.calls)
		}
	}
	wantContains(t, "stdout", e.out.String(), "[release-retract] marking release v1.0.0 as draft\n",
		"[release-retract] prepending RETRACTED notice\n", "RETRACTION COMPLETE: v1.0.0", "hotfix-revert-formula-v1.0.0")
	log, _ := os.ReadFile(filepath.Join(e.d.Root, "audit", "release-retractions.log"))
	if w := "2026-11-07T01:02:03Z | sha=abc123 | tag=v1.0.0 | operator=u | deleted_tag=0 | reason=env why\n"; string(log) != w {
		t.Errorf("audit %q", log)
	}
}

func TestReleaseCtlCreateRetractRetractForceAndDeleteTag(t *testing.T) {
	e := newEnv(t, "")
	e.r.out["gh release view --json tagName"] = "v9.0.0\n"
	code := RunWith(e.d, []string{"retract", "v1.0.0", "--force", "--delete-tag", "--reason", "leak"})
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	if e.r.called("gh release view --json tagName") {
		t.Errorf("force must skip latest lookup: %v", e.r.calls)
	}
	if !e.r.called("git -C "+e.d.Root+" tag -d v1.0.0") || !e.r.called("git -C "+e.d.Root+" push origin :refs/tags/v1.0.0") {
		t.Errorf("tag delete missing: %v", e.r.calls)
	}
	wantContains(t, "stderr", e.err.String(), "[release-retract] WARN: Delete tag (local + remote): YES\n",
		"[release-retract] WARN: deleting tag v1.0.0 on origin\n")
}

func TestReleaseCtlCreateRetractRetractMissingRelease(t *testing.T) {
	e := newEnv(t, "")
	e.r.fail["gh release view"] = true
	if RunWith(e.d, []string{"retract", "v1.0.0", "--force", "--reason", "x"}) != 2 {
		t.Error("want exit 2")
	}
	if want := "ERROR: no GitHub release found for tag v1.0.0\n"; e.err.String() != want {
		t.Errorf("stderr %q", e.err)
	}
}

func TestReleaseCtlCreateRetractRetractInteractiveAbort(t *testing.T) {
	e := newEnv(t, "no\n")
	e.d.Interactive = true
	RunWith(e.d, []string{"retract", "v1.0.0", "--force", "--reason", "x"})
	wantContains(t, "stderr", e.err.String(), "Type RETRACT to confirm: Aborted by operator.\n")
	if e.r.called("gh release edit") {
		t.Errorf("edited: %v", e.r.calls)
	}
}

func TestReleaseRunRunDispatch(t *testing.T) {
	e := newEnv(t, "")
	if RunWith(e.d, nil) != 2 || RunWith(e.d, []string{"bogus"}) != 2 {
		t.Error("want exit 2")
	}
	wantContains(t, "stderr", e.err.String(), "usage: releasectl <create|retract|gate|run>", "unknown command: bogus\n")
	e.err.Reset()
	if RunWith(e.d, []string{"run", "--dry-run"}) != 2 {
		t.Error("run without --version must exit 2")
	}
	wantContains(t, "stderr", e.err.String(), "release: --version X.Y.Z is required (got '')\n")
	e.err.Reset()
	if code := RunWith(e.d, []string{"run", "--dry-run", "--version", "1.5.0"}); code != 0 {
		t.Fatalf("dry-run exit %d: %s", code, e.err)
	}
	wantContains(t, "stdout", e.out.String(),
		"S1 preflight cli gh auth status\n",
		"S4 tag cli gh pr list -R nself-org/cli --head release/v1.5.0 --state merged --json mergeCommit (the S3 merge commit)\n",
		"S10 sdk-publish cli gh api repos/nself-org/cli/contents/.github/workflows?ref=main (dry run: not read;")
	if len(e.r.calls) != 0 {
		t.Errorf("dry-run called %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractRetractLatestLookupErrorIsReported(t *testing.T) {
	e := newEnv(t, "")
	e.r.fail["gh release view --json tagName"] = true
	code := RunWith(e.d, []string{"retract", "v1.0.0", "--reason", "x"})
	if code != 1 || !strings.Contains(e.err.String(), "ERROR: cannot read the latest release (gh release view): fake failure\n") ||
		strings.Contains(e.err.String(), "use --force") {
		t.Fatalf("code %d: %s", code, e.err.String())
	}
	if e.r.called("gh release edit") {
		t.Fatal("retracted despite the lookup failure")
	}
}

// The latest release is GitHub's own (gh release view with no tag), which
// leaves out drafts and prereleases, never the newest row of gh release list.
func TestReleaseCtlCreateRetractRetractLatestIgnoresDrafts(t *testing.T) {
	e := newEnv(t, "")
	e.r.out["gh release view --json tagName"] = "v1.0.0\n"
	e.r.out["gh release list"] = "v1.0.1\n" // a drafted next release
	if code := RunWith(e.d, []string{"retract", "v1.0.0", "--reason", "x"}); code != 0 {
		t.Fatalf("code %d: %s", code, e.err)
	}
	if e.r.called("gh release list") {
		t.Errorf("latest read from gh release list: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractRetractReasonNeedsValue(t *testing.T) {
	e := newEnv(t, "")
	e.d.Env = func(k string) string { return map[string]string{"RELEASE_RETRACT_REASON": "env why"}[k] }
	if code := RunWith(e.d, []string{"retract", "v1.0.0", "--reason"}); code != 2 {
		t.Fatalf("code %d", code)
	}
	if want := "ERROR: --reason needs a value\n"; e.err.String() != want {
		t.Errorf("stderr %q", e.err)
	}
	if len(e.r.calls) != 0 {
		t.Errorf("calls %v", e.r.calls)
	}
}
