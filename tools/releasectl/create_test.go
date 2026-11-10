package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func wantContains(t *testing.T, label, got string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(got, s) {
			t.Errorf("%s missing %q in:\n%s", label, s, got)
		}
	}
}

func TestReleaseCtlCreateRetractCreateNoDist(t *testing.T) {
	e := newEnv(t, "")
	if code := RunWith(e.d, []string{"create", "v9.9.9"}); code != 0 {
		t.Fatalf("code %d stderr %s", code, e.err)
	}
	wantContains(t, "stdout", e.out.String(),
		"[release-create] no dist/ yet; skipping DENY-001 signature-check (enforced post-build by release.yml)\n",
		"[release-create] no dist/ yet; skipping DENY-002 missing-sbom (enforced post-build by release.yml)\n",
		"[release-create] denylist: PASS (0 rules triggered)\n",
		"[release-create] invoking: gh release create v9.9.9\n")
	if !e.r.called("gh release create v9.9.9") {
		t.Errorf("gh release create not run: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateDraft(t *testing.T) {
	e := newEnv(t, "")
	RunWith(e.d, []string{"create", "v9.9.9", "--draft"})
	wantContains(t, "stdout", e.out.String(), "skipping DENY-001", "invoking: gh release create v9.9.9 --draft\n")
	if !e.r.called("gh release create v9.9.9 --draft") {
		t.Errorf("calls: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateEmptyDistBlocks(t *testing.T) {
	e := newEnv(t, "")
	os.Mkdir(filepath.Join(e.d.Root, "dist"), 0o755)
	if code := RunWith(e.d, []string{"create", "v9.9.9"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	want := "[release-create] FAIL: denylist: 2 rule(s) triggered\n  - DENY-001 signature-check\n  - DENY-002 missing-sbom\n" +
		"[release-create] FAIL: release blocked. Re-run with --skip-denylist + justification to override.\n"
	if e.err.String() != want {
		t.Errorf("stderr:\n%q\nwant\n%q", e.err.String(), want)
	}
	if e.r.called("gh release create") {
		t.Errorf("release must not run: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateLicenseQueryOnMain(t *testing.T) {
	e := newEnv(t, "")
	RunWith(e.d, []string{"create", "v9.9.9"})
	want := "gh run list --workflow license-audit.yml --branch main --limit 1 --json conclusion -q .[0].conclusion"
	if !e.r.called(want) {
		t.Errorf("DENY-004 call missing: %v", e.r.calls)
	}
	if !e.r.called("gh api --paginate repos/{owner}/{repo}/milestones?state=all&per_page=100 --jq .[] | select(.title==\"v9.9.9\") | .state") {
		t.Errorf("DENY-005 call missing: %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateDryRun(t *testing.T) {
	e := newEnv(t, "")
	code := RunWith(e.d, []string{"create", "v1.2.3", "--dry-run", "--notes", "two words", "--title=it's"})
	if code != 0 {
		t.Fatalf("code %d", code)
	}
	want := "[release-create] invoking: gh release create v1.2.3 --notes 'two words' '--title=it'\\''s'\n"
	if e.out.String() != want {
		t.Errorf("stdout %q want %q", e.out.String(), want)
	}
	if len(e.r.calls) != 0 {
		t.Errorf("dry-run ran %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateOverrideNonInteractive(t *testing.T) {
	e := newEnv(t, "")
	os.Mkdir(filepath.Join(e.d.Root, "dist"), 0o755)
	RunWith(e.d, []string{"create", "v9.9.9"}) // blocked
	e.r.calls = nil
	if code := RunWith(e.d, []string{"create", "v9.9.9", "--skip-denylist"}); code != 1 {
		t.Fatalf("missing reason must fail, got %d", code)
	}
	wantContains(t, "stderr", e.err.String(), "FAIL: non-interactive override requires RELEASE_OVERRIDE_REASON env var")
	e.d.Env = func(k string) string {
		return map[string]string{"RELEASE_OVERRIDE_REASON": "hotfix", "GITHUB_ACTOR": "bot"}[k]
	}
	if code := RunWith(e.d, []string{"create", "v9.9.9", "--skip-denylist", "--draft"}); code != 0 {
		t.Fatalf("code %d %s", code, e.err)
	}
	log, _ := os.ReadFile(filepath.Join(e.d.Root, "audit", "release-overrides.log"))
	want := "2026-11-07T01:02:03Z | sha=abc123 | tag=v9.9.9 | operator=bot | skipped_rules=[DENY-001 signature-check,DENY-002 missing-sbom] | reason=hotfix\n"
	if string(log) != want {
		t.Errorf("audit %q want %q", log, want)
	}
	if !e.r.called("gh release create v9.9.9 --draft") {
		t.Errorf("calls %v", e.r.calls)
	}
}

func TestReleaseCtlCreateRetractCreateOverrideInteractiveAbort(t *testing.T) {
	e := newEnv(t, "nope\n")
	e.d.Interactive = true
	os.Mkdir(filepath.Join(e.d.Root, "dist"), 0o755)
	if code := RunWith(e.d, []string{"create", "v9.9.9", "--skip-denylist"}); code != 1 {
		t.Fatalf("code %d", code)
	}
	wantContains(t, "stderr", e.err.String(), "Type the word OVERRIDE to confirm: ", "FAIL: override aborted by operator")
}

func TestReleaseCtlCreateRetractCreateUsageAndMissingDenylist(t *testing.T) {
	e := newEnv(t, "")
	if RunWith(e.d, []string{"create"}) != 2 || !strings.Contains(e.err.String(), "ERROR: usage: releasectl create <tag>") {
		t.Errorf("usage: %q", e.err)
	}
	e = newEnv(t, "")
	os.Remove(filepath.Join(e.d.Root, ".github", "release-denylist.json"))
	if RunWith(e.d, []string{"create", "v1"}) != 2 || !strings.Contains(e.err.String(), "ERROR: denylist file not found: ") {
		t.Errorf("denylist: %q", e.err)
	}
}

func TestReleaseCtlCreateRetractJoinArgs(t *testing.T) {
	got := joinArgs([]string{"a", "b c", "d$e", "", "x;y", "plain-1.2"})
	if want := "a 'b c' 'd$e' '' 'x;y' plain-1.2"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
