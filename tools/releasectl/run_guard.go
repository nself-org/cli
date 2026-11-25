package main

// The version guards of release.sh, run before any step (GET only):
//   - older than the latest release is always refused;
//   - a fresh start (--from S1..S3) also needs version > latest and no tag;
//   - S4 may run with the tag present (it checks the tag sits on the merged
//     commit and never moves it);
//   - equal to latest is allowed only from S5, when the release is in flight
//     or published and S5..S10 are finishing it.

import "strings"

// tagExists is tag_exists: false when absent (404); any other failure stops
// the run, because an unreadable answer must never read as "absent".
func (r *Release) tagExists() (bool, error) {
	g := r.ghTry("api", "repos/nself-org/cli/git/ref/tags/"+r.Tag)
	if g.Fail == nil {
		return true, nil
	}
	if g.is404() {
		return false, nil
	}
	return false, &FatalError{"cannot check tag " + r.Tag + ": " + g.Err}
}

// guardVersion returns 0 when the run may start, else the exit code (message
// printed to stderr).
func (r *Release) guardVersion(from int) int {
	latest := ""
	g := r.ghTry("api", "repos/nself-org/cli/releases/latest", "--jq", ".tag_name")
	switch {
	case g.Fail == nil:
		latest = strings.TrimPrefix(strings.SplitN(g.Out, "\n", 2)[0], "v")
		if !latestRe.MatchString(latest) {
			r.warn("unexpected latest release tag '%s'", latest)
			return 1
		}
	case g.is404():
	default:
		r.warn("cannot read the latest cli release: %s", g.Err)
		return 1
	}
	if latest != "" && verGT(latest, r.Version) {
		r.warn("version %s is older than the latest release v%s", r.Version, latest)
		return 2
	}
	if from <= 4 && latest != "" && latest == r.Version {
		r.warn("version %s is already the latest release; resume a finished release with --from S5 or later", r.Version)
		return 2
	}
	if from <= 3 {
		exists, err := r.tagExists()
		if err != nil {
			r.warn("%s", strings.TrimPrefix(err.Error(), "release: "))
			return 1
		}
		if exists {
			r.warn("tag %s already exists; resume with --from S4 or later", r.Tag)
			return 2
		}
	}
	return 0
}
