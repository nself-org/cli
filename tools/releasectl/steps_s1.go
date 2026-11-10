package main

// Port of scripts/release/lib/steps1.sh: S1 preflight, S2 admin lockstep PR,
// S3 cli version PR. In plan mode each step prints its rows and calls nothing.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// jsonVersion is `jq -r .version`: "" when the text is not JSON or has none.
func jsonVersion(s string) string {
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal([]byte(s), &v) != nil {
		return ""
	}
	return v.Version
}

// squash is `tr -d '[:space:]'`.
func squash(s string) string { return strings.Join(strings.Fields(s), "") }

func (r *Release) hasFacts() bool {
	fi, err := os.Stat(filepath.Join(r.Work, "cli", "tools", "facts"))
	return err == nil && fi.IsDir()
}

// StepPreflight is step_preflight (S1).
func (r *Release) StepPreflight() error {
	if r.Plan {
		r.planLine("S1", "preflight", "cli", "gh auth status")
		for _, t := range []string{"cli", "admin", "homebrew-nself"} {
			r.planLine("S1", "preflight", t, "gh repo clone nself-org/"+t+" "+r.Work+"/"+t)
		}
		return nil
	}
	for _, t := range []string{"git", "curl", "gh", "node"} {
		if !r.Have(t) {
			return r.die("S1", "preflight", "missing tool: "+t)
		}
	}
	if _, err := r.Run.Run("gh", "auth", "status"); err != nil {
		return r.die("S1", "preflight", "gh is not logged in (run: gh auth login)")
	}
	for _, t := range []string{"cli", "admin", "homebrew-nself"} {
		if err := r.ensureClone(t); err != nil {
			return err
		}
	}
	if r.hasFacts() && !r.Have("go") {
		return r.die("S1", "preflight", "missing tool: go (cli has tools/facts; S3 regenerates facts)")
	}
	b, err := os.ReadFile(filepath.Join(r.Work, "cli", ".github", "VERSION"))
	if err != nil {
		return r.die("S1", "preflight", "cannot read .github/VERSION in the cli clone")
	}
	r.say("S1 preflight: cli main is at %s, releasing %s; work dir %s", squash(string(b)), r.Version, r.Work)
	out, err := r.Run.Run("gh", "api", "repos/nself-org/cli/actions/organization-secrets", "--jq", ".secrets[].name")
	if err != nil || !strings.Contains(out, "DOCKERHUB_TOKEN") {
		r.warn("S1 preflight: warning: DOCKERHUB_TOKEN not visible to nself-org/cli (P7-PROD-61); S8/S9 will fail without it")
	}
	return nil
}

// StepAdminLockstep is step_admin_lockstep (S2).
func (r *Release) StepAdminLockstep() error {
	br := "release/v" + r.Version + "-lockstep"
	if r.Plan {
		w := r.Work
		r.planLine("S2", "admin-lockstep", "admin", "scripts/bump-version.sh "+r.Version+" (cwd "+w+"/cli; edits admin package.json, src/lib/cli-version.ts, Dockerfile)")
		r.planLine("S2", "admin-lockstep", "admin", "git -C "+w+"/admin checkout -b "+br+"; git commit; git push origin "+br)
		r.planLine("S2", "admin-lockstep", "admin", "gh pr create -R nself-org/admin --base main --head "+br)
		r.planLine("S2", "admin-lockstep", "admin", "gh pr checks N -R nself-org/admin --required (polled, 45 min budget)")
		r.planLine("S2", "admin-lockstep", "admin", "scripts/admin-merge.sh --repo nself-org/admin --pr N")
		return nil
	}
	out, err := r.ghNeed("admin package.json", "api", "-H", "Accept: application/vnd.github.raw", "repos/nself-org/admin/contents/package.json")
	if err != nil {
		return err
	}
	if jsonVersion(out) == r.Version {
		r.stepDone("S2", "admin-lockstep")
		return nil
	}
	if err := r.prInfo("admin", br); err != nil {
		return err
	}
	if err := r.stalePR("S2", "admin-lockstep", "admin", br); err != nil {
		return err
	}
	if err := r.stepConfirm("S2", "admin-lockstep"); err != nil {
		return err
	}
	if r.PrInfo == "" {
		if err := r.prepareBump("S2", "admin-lockstep", br, "admin", "chore(release): bump admin to v"+r.Version+" (lockstep with cli)", "--no-goldens"); err != nil {
			return err
		}
		if err := r.resetClone("cli"); err != nil { // the bump also edited cli files
			return err
		}
	} else if err := r.checkoutBranch("admin", br); err != nil {
		return err
	}
	return r.prFlow("S2", "admin-lockstep", "admin", br, "admin_pr", "chore(release): bump admin to v"+r.Version,
		"Lockstep bump for the cli v"+r.Version+" release (package.json, src/lib/cli-version.ts, Dockerfile). Opened by scripts/release/release.sh.")
}

// prepareBump refreshes both clones and runs the bump (S2 and S3 share it).
func (r *Release) prepareBump(s, id, br, repo, msg string, bumpArgs ...string) error {
	for _, f := range []func(string) error{r.ensureClone, r.resetClone} {
		for _, c := range []string{"cli", "admin"} {
			if err := f(c); err != nil {
				return err
			}
		}
	}
	if err := r.bumpAll(bumpArgs...); err != nil {
		return r.die(s, id, "scripts/bump-version.sh failed in the cli clone")
	}
	if s == "S3" {
		if err := r.regenFacts(); err != nil {
			return err
		}
	}
	if _, err := r.git(repo, "checkout", "-q", "-B", br); err != nil {
		return r.die(s, id, "could not create branch "+br)
	}
	changed, cerr := r.commitAll(repo, msg)
	if cerr != nil {
		return r.die(s, id, cerr.Error())
	}
	if !changed {
		return r.die(s, id, "bump changed nothing in "+repo)
	}
	return nil
}

// regenFacts runs tools/facts in the cli clone when it exists (S3).
func (r *Release) regenFacts() error {
	if !r.hasFacts() {
		r.say("S3 cli-version: tools/facts absent at this commit; no facts regeneration")
		return nil
	}
	if !r.Have("go") {
		return r.die("S3", "cli-version", "go is not installed (needed for tools/facts)")
	}
	cli := filepath.Join(r.Work, "cli")
	if _, err := r.runIn(cli, false, "go", "run", "-mod=vendor", "./tools/facts"); err != nil {
		return r.die("S3", "cli-version", "facts regeneration failed")
	}
	if _, err := r.runIn(cli, false, "go", "run", "-mod=vendor", "./tools/facts", "-readme", "README.md"); err != nil {
		return r.die("S3", "cli-version", "facts regeneration failed")
	}
	return nil
}

// StepCliVersion is step_cli_version (S3).
func (r *Release) StepCliVersion() error {
	br := "release/v" + r.Version
	if r.Plan {
		w := r.Work
		r.planLine("S3", "cli-version", "cli", "scripts/bump-version.sh "+r.Version+" (cwd "+w+"/cli; also regenerates the 4 version-hash goldens: UPDATE_GOLDEN=1 go test ./internal/build ./cmd/commands, then re-runs them plain)")
		r.planLine("S3", "cli-version", "cli", "go run -mod=vendor ./tools/facts && go run -mod=vendor ./tools/facts -readme README.md (facts regeneration, only when tools/facts exists)")
		r.planLine("S3", "cli-version", "cli", "git -C "+w+"/cli checkout -b "+br+"; git commit; git push origin "+br+"; gh pr create -R nself-org/cli --base main --head "+br)
		r.planLine("S3", "cli-version", "cli", "gh pr checks N -R nself-org/cli --required (polled, 45 min budget)")
		r.planLine("S3", "cli-version", "cli", "scripts/admin-merge.sh --repo nself-org/cli --pr N")
		return nil
	}
	out, err := r.ghNeed("cli .github/VERSION", "api", "-H", "Accept: application/vnd.github.raw", "repos/nself-org/cli/contents/.github/VERSION")
	if err != nil {
		return err
	}
	if squash(out) == r.Version {
		r.stepDone("S3", "cli-version")
		return nil
	}
	if err := r.prInfo("cli", br); err != nil {
		return err
	}
	if err := r.stalePR("S3", "cli-version", "cli", br); err != nil {
		return err
	}
	if err := r.stepConfirm("S3", "cli-version"); err != nil {
		return err
	}
	if r.PrInfo == "" {
		if err := r.prepareBump("S3", "cli-version", br, "cli", "chore(release): bump to v"+r.Version); err != nil {
			return err
		}
	} else if err := r.checkoutBranch("cli", br); err != nil {
		return err
	}
	return r.prFlow("S3", "cli-version", "cli", br, "cli_pr", "chore(release): bump to v"+r.Version,
		"Version bump for v"+r.Version+", with facts regenerated when tools/facts exists. Opened by scripts/release/release.sh.")
}
