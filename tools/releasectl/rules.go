package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (d Deps) info(f string, a ...any) { fmt.Fprintf(d.Stdout, "[release-create] "+f+"\n", a...) }
func (d Deps) warn(f string, a ...any) { fmt.Fprintf(d.Stderr, "[release-create] WARN: "+f+"\n", a...) }
func (d Deps) fail(f string, a ...any) { fmt.Fprintf(d.Stderr, "[release-create] FAIL: "+f+"\n", a...) }

func (d Deps) hasDist() bool {
	st, err := os.Stat(filepath.Join(d.Root, "dist"))
	return err == nil && st.IsDir()
}

// ruleSignature is DENY-001: a *.cosign.bundle must exist in dist/.
func (d Deps) ruleSignature() bool {
	if !d.hasDist() {
		d.info("no dist/ yet; skipping DENY-001 signature-check (enforced post-build by release.yml)")
		return true
	}
	m, _ := filepath.Glob(filepath.Join(d.Root, "dist", "*.cosign.bundle"))
	return len(m) > 0
}

// ruleSBOM is DENY-002: SPDX and CycloneDX SBOMs in dist/ or the repo root.
func (d Deps) ruleSBOM() bool {
	if !d.hasDist() {
		d.info("no dist/ yet; skipping DENY-002 missing-sbom (enforced post-build by release.yml)")
		return true
	}
	for _, f := range []string{"sbom.spdx.json", "sbom.cdx.json"} {
		_, e1 := os.Stat(filepath.Join(d.Root, "dist", f))
		_, e2 := os.Stat(filepath.Join(d.Root, f))
		if e1 != nil && e2 != nil {
			return false
		}
	}
	return true
}

// ruleCommits is DENY-003: at least one commit since the prior tag. A tag
// list that cannot be read blocks: it is not evidence of a first release.
func (d Deps) ruleCommits(tag string) bool {
	out, err := d.Runner.Run("git", "-C", d.Root, "tag", "--sort=-version:refname")
	if err != nil {
		d.fail("cannot list git tags (DENY-003): %v", err)
		return false
	}
	prior := ""
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" && l != tag {
			prior = l
			break
		}
	}
	if prior == "" {
		d.info("no prior tag found; treating as first release (PASS)")
		return true
	}
	cnt, err := d.Runner.Run("git", "-C", d.Root, "rev-list", prior+"..HEAD", "--count")
	n := 0
	if err == nil {
		n, _ = strconv.Atoi(strings.TrimSpace(cnt))
	}
	return n > 0
}

// ruleLicense is DENY-004: latest license-audit.yml run on main succeeded.
func (d Deps) ruleLicense() bool {
	if !d.HaveGH() {
		d.warn("gh CLI not found; cannot verify license-audit. Treating as PASS (CI gate handles it).")
		return true
	}
	out, err := d.Runner.Run("gh", "run", "list", "--workflow", "license-audit.yml",
		"--branch", "main", "--limit", "1", "--json", "conclusion", "-q", ".[0].conclusion")
	return err == nil && strings.TrimSpace(out) == "success"
}

// ruleMilestone is DENY-005: exactly one milestone is titled TAG and it is
// closed. An open duplicate or two closed ones block, as S4's denyMilestone
// does and as the gate's old `[ "$MILESTONE" != closed ]` test did.
func (d Deps) ruleMilestone(tag string) bool {
	if !d.HaveGH() {
		d.warn("gh CLI not found; cannot verify milestone. Treating as PASS (CI gate handles it).")
		return true
	}
	out, err := d.Runner.Run("gh", "api", "--paginate", "repos/{owner}/{repo}/milestones?state=all&per_page=100",
		"--jq", fmt.Sprintf(".[] | select(.title==\"%s\") | .state", tag))
	return err == nil && strings.TrimSpace(out) == "closed"
}

// triggered evaluates all rules and returns the ones that block.
func (d Deps) triggered(tag string) []string {
	var t []string
	if !d.ruleSignature() {
		t = append(t, "DENY-001 signature-check")
	}
	if !d.ruleSBOM() {
		t = append(t, "DENY-002 missing-sbom")
	}
	if !d.ruleCommits(tag) {
		t = append(t, "DENY-003 zero-commits-since-prior-tag")
	}
	if !d.ruleLicense() {
		t = append(t, "DENY-004 failed-license-scan")
	}
	if !d.ruleMilestone(tag) {
		t = append(t, "DENY-005 no-milestone-closed")
	}
	return t
}
