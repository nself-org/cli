package main

// Port of scripts/release/lib/steps3.sh (part 2): S9, the admin tag and the
// admin image. Admin's docker-publish.yml starts on a v* tag push, so S9 tags
// admin and waits for that run instead of dispatching a second publish.

import "strings"

// adminBumpCommit is admin_bump_commit: no merged lockstep PR was found, so
// AdminSHA = the oldest commit of the newest run of commits that carry
// package.json at the version (the bump itself, not the tip of main).
func (r *Release) adminBumpCommit() error {
	out, err := r.ghNeed("the admin commits that touched package.json", "api", "repos/nself-org/admin/commits?path=package.json&per_page=20", "--jq", ".[].sha")
	if err != nil {
		return err
	}
	prev := ""
	for _, c := range strings.Fields(out) {
		pj, err := r.ghNeed("admin package.json at "+c, "api", "-H", "Accept: application/vnd.github.raw", "repos/nself-org/admin/contents/package.json?ref="+c)
		if err != nil {
			return err
		}
		if jsonVersion(pj) != r.Version {
			break
		}
		prev = c
	}
	if prev == "" {
		return r.die("S9", "admin-image", "no admin commit sets package.json to "+r.Version+" and no merged lockstep PR was found; run S2", "S2")
	}
	r.AdminSHA = prev
	return nil
}

// adminTag is admin_tag: an annotated tag on admin's S2 merge commit, pushed
// from the admin clone. An existing tag on another commit stops the run.
func (r *Release) adminTag() error {
	const s, id = "S9", "admin-image"
	sha, err := r.ghNeed("the admin lockstep PR", "pr", "list", "-R", "nself-org/admin", "--head", "release/v"+r.Version+"-lockstep",
		"--state", "merged", "--json", "mergeCommit", "--jq", ".[0].mergeCommit.oid // empty")
	if err != nil {
		return err
	}
	r.AdminSHA = sha
	if sha == "" {
		if err := r.adminBumpCommit(); err != nil {
			return err
		}
	}
	found, err := r.repoTagSHA("admin", r.Tag)
	if err != nil {
		return err
	}
	if found {
		if r.TagSHA != r.AdminSHA {
			return r.die(s, id, "admin tag "+r.Tag+" is at "+r.TagSHA+" but the lockstep release commit is "+r.AdminSHA+"; tags are never moved, inspect by hand")
		}
		r.say("S9 admin-image: admin tag %s already at %s", r.Tag, r.AdminSHA)
		return nil
	}
	if err := r.stepConfirm(s, id); err != nil {
		return err
	}
	if err := r.ensureClone("admin"); err != nil {
		return err
	}
	if err := r.resetClone("admin"); err != nil {
		return err
	}
	if _, err := r.git("admin", "merge-base", "--is-ancestor", r.AdminSHA, "origin/main"); err != nil {
		return r.die(s, id, "the admin release commit "+r.AdminSHA+" is not in origin/main's history; inspect before tagging")
	}
	local, _ := r.git("admin", "rev-parse", "-q", "--verify", "refs/tags/"+r.Tag+"^{commit}")
	if strings.TrimSpace(local) != r.AdminSHA {
		if _, err := r.git("admin", "tag", "-a", r.Tag, "-m", r.Tag, r.AdminSHA); err != nil {
			return r.die(s, id, "could not create the admin tag (a stale "+r.Tag+" in "+r.Work+"/admin, or git user.name / user.email unset?)")
		}
	}
	if _, err := r.git("admin", "push", "-q", "origin", "refs/tags/"+r.Tag); err != nil {
		return r.die(s, id, "push of the admin tag "+r.Tag+" was refused")
	}
	r.say("S9 admin-image: tagged admin %s -> %s (this starts admin's docker-publish.yml)", r.Tag, r.AdminSHA)
	return nil
}

// StepAdminImage is step_admin_image (S9).
func (r *Release) StepAdminImage() error {
	if r.Plan {
		w, v, t := r.Work, r.Version, r.Tag
		p := func(c string) { r.planLine("S9", "admin-image", "admin", c) }
		p("gh pr list -R nself-org/admin --head release/v" + v + "-lockstep --state merged --json mergeCommit (the S2 merge commit)")
		p("git -C " + w + "/admin tag -a " + t + " -m " + t + " SHA && git -C " + w + "/admin push origin refs/tags/" + t + " (admin's docker-publish.yml starts on a v* tag; hygiene's release-tag-drift check needs the tag)")
		p("gh run list -R nself-org/admin --workflow docker-publish.yml (wait about 3 min for the tag-push run, then for it to finish)")
		p("gh workflow run docker-publish.yml -R nself-org/admin -f version=" + v + " (only if no run publishes the tag)")
		p("curl -fsS " + hubAPI + "/nself/nself-admin/tags/" + v + " (need linux/amd64 and linux/arm64)")
		return nil
	}
	if err := r.adminTag(); err != nil {
		return err
	}
	return r.imageStep("S9", "admin-image", "admin", "nself/nself-admin")
}
