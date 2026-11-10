package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// planRows are the `S<n> <id> <repo> <command>` rows of stdout, in order.
func planRows(out string) []string {
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		if regexp.MustCompile(`^S[0-9]+ `).MatchString(l) {
			rows = append(rows, l)
		}
	}
	return rows
}

// TestReleaseRunPlanListsAllStepsGETOnly is case 1: --plan prints S1..S10 in order, makes
// only `gh api` GET calls, and writes nothing.
func TestReleaseRunPlanListsAllStepsGETOnly(t *testing.T) {
	f := newFixture(t, "", false)
	wantCode(t, f.run("--plan", "--version", "9.9.9"), 0, f)
	var seq, ids []string
	for _, r := range planRows(f.out.String()) {
		p := strings.Fields(r)
		if len(seq) == 0 || seq[len(seq)-1] != p[0] {
			seq, ids = append(seq, p[0]), append(ids, p[1])
		}
	}
	if got, want := strings.Join(seq, " "), "S1 S2 S3 S4 S5 S6 S7 S8 S9 S10"; got != want {
		t.Errorf("steps %q want %q", got, want)
	}
	if got, want := strings.Join(ids, " "), "preflight admin-lockstep cli-version tag build-wait homebrew-formula release-wait docker-verify admin-image sdk-publish"; got != want {
		t.Errorf("ids %q want %q", got, want)
	}
	out := f.out.String()
	for _, r := range []string{"admin", "cli", "homebrew-nself"} {
		stdoutHas(t, f, "scripts/admin-merge.sh --repo nself-org/"+r+" --pr N")
	}
	stdoutHas(t, f, "tools/facts -readme README.md", "update-formula.sh --version v9.9.9 --checksums-file",
		"version-hash goldens", "S7 release-wait web node org/scripts/sources/update-lock.mjs cli v9.9.9",
		"S9 admin-image admin git -C "+f.workDirFor("9.9.9")+"/admin tag -a v9.9.9 -m v9.9.9 SHA && git -C "+f.workDirFor("9.9.9")+"/admin push origin refs/tags/v9.9.9",
		"S4 tag cli git -C "+f.workDirFor("9.9.9")+"/cli tag -a v9.9.9 -m v9.9.9 SHA && git -C "+f.workDirFor("9.9.9")+"/cli push origin refs/tags/v9.9.9",
		"S10 sdk-publish cli gh workflow run sdk-ts-publish.yml -R nself-org/cli --ref v9.9.9 (skipped when a run on v9.9.9 already succeeded)",
		"S10 sdk-publish cli gh workflow run sdk-publish-go.yml -R nself-org/cli --ref v9.9.9 -f version=9.9.9 (skipped when",
		"S10 sdk-publish cli manual: sdk-manual-publish.yml has no workflow_dispatch; publish it by hand")
	if n := len(regexp.MustCompile(`(?m)^S4 tag cli .*DENY-00[345]`).FindAllString(out, -1)); n != 3 {
		t.Errorf("S4 DENY rows = %d, want 3 (DENY-003, DENY-004 on main, DENY-005)", n)
	}
	if strings.Contains(out, "release-create") {
		t.Error("plan mentions release-create")
	}
	for _, c := range f.w.calls {
		if !strings.HasPrefix(c, "gh api ") || strings.Contains(c, " -X ") || strings.Contains(c, " --method ") || strings.Contains(c, " -f ") {
			t.Errorf("plan made a non-GET call: %q", c)
		}
	}
	stderrHas(t, f, "release: plan for v9.9.9 from S1 (read-only: gh api GETs only)\n")
	if _, err := os.Stat(f.workDirFor("9.9.9")); err == nil {
		t.Error("plan created a work dir")
	}
	if _, err := os.Stat(f.workDirFor("9.9.9") + ".state"); err == nil {
		t.Error("plan wrote a state file")
	}
	if f.w.called("gh repo clone") != 0 || f.w.called("git") != 0 {
		t.Errorf("plan cloned or ran git: %v", f.w.calls)
	}
}

// TestReleaseRunDryRunPrintsPlanAndCallsNothing: --dry-run prints the same rows as
// --plan and calls nothing at all.
func TestReleaseRunDryRunPrintsPlanAndCallsNothing(t *testing.T) {
	plan := newFixture(t, "", false)
	wantCode(t, plan.run("--plan", "--version", "9.9.9"), 0, plan)
	dry := newFixture(t, "", false)
	wantCode(t, dry.run("--dry-run", "--version", "9.9.9"), 0, dry)
	if len(dry.w.calls) != 0 {
		t.Fatalf("dry-run called: %v", dry.w.calls)
	}
	pr, dr := planRows(plan.out.String()), planRows(dry.out.String())
	var pOther, dOther []string
	for _, r := range pr {
		if !strings.HasPrefix(r, "S10 ") {
			pOther = append(pOther, r)
		}
	}
	for _, r := range dr {
		if !strings.HasPrefix(r, "S10 ") {
			dOther = append(dOther, r)
		}
	}
	if strings.Join(pOther, "\n") != strings.Join(dOther, "\n") || len(dOther) == 0 {
		t.Errorf("S1..S9 rows differ:\nplan:\n%s\ndry:\n%s", strings.Join(pOther, "\n"), strings.Join(dOther, "\n"))
	}
	want := "S10 sdk-publish cli gh api repos/nself-org/cli/contents/.github/workflows?ref=main (dry run: not read; each sdk-*publish*.yml with workflow_dispatch gets gh workflow run FILE -R nself-org/cli --ref v9.9.9, skipped when a run on v9.9.9 already succeeded)"
	if dr[len(dr)-1] != want {
		t.Errorf("S10 dry row %q", dr[len(dr)-1])
	}
}

// TestReleaseRunDryRunFromStep prints only the steps from --from on.
func TestReleaseRunDryRunFromStep(t *testing.T) {
	f := newFixture(t, "", false)
	wantCode(t, f.run("--dry-run", "--version", "1.5.0", "--from", "S8"), 0, f)
	rows := planRows(f.out.String())
	if len(rows) == 0 || !strings.HasPrefix(rows[0], "S8 docker-verify cli curl -fsS https://hub.docker.com/v2/repositories/nself/nself/tags/1.5.0") {
		t.Errorf("rows %v", rows)
	}
	for _, r := range rows {
		if strings.HasPrefix(r, "S1 ") || strings.HasPrefix(r, "S7 ") {
			t.Errorf("row before S8: %q", r)
		}
	}
	if len(f.w.calls) != 0 {
		t.Errorf("calls %v", f.w.calls)
	}
}

func (f *rfix) workDirFor(v string) string {
	return filepath.Join(filepath.Dir(f.w.work), "nself-release-"+v)
}
