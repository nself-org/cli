package main

// Port of scripts/release/lib/hashes.sh (part 1): the release run, the
// checksums it produced and the Homebrew formula hashes. GET-only.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// FindRun is find_run: RunID = the release.yml run built from the tag's commit
// ("" if none yet). A run id kept in the state file is used only if its
// headSha is the tag's commit.
func (r *Release) FindRun() error {
	r.RunID = ""
	ok, err := r.repoTagSHA("cli", r.Tag)
	if err != nil || !ok {
		return err
	}
	id := r.stateGet("run")
	if id != "" {
		g := r.ghTry("run", "view", id, "-R", "nself-org/cli", "--json", "headSha", "--jq", ".headSha")
		if g.Fail != nil || g.Out != r.TagSHA {
			r.say("  run %s from an earlier attempt is not the build of %s; looking it up again", id, r.Tag)
			id = ""
		}
	}
	if id == "" {
		out, err := r.ghNeed("the release.yml runs", "run", "list", "-R", "nself-org/cli", "--workflow", "release.yml",
			"--branch", r.Tag, "--limit", "10", "--json", "databaseId,headSha",
			"--jq", `.[] | select(.headSha == "`+r.TagSHA+`") | .databaseId`)
		if err != nil {
			return err
		}
		id = strings.SplitN(out, "\n", 2)[0]
	}
	if id != "" {
		if err := r.stateSet("run", id); err != nil {
			return err
		}
	}
	r.RunID = id
	return nil
}

// RunPresent is run_present.
func (r *Release) RunPresent() (bool, error) {
	err := r.FindRun()
	return r.RunID != "", err
}

// EnsureChecksums is ensure_checksums: ChecksumsFile = checksums.txt, fetched
// fresh every time. The published release asset wins; before publish it is the
// build run's release-checksums artifact. Returns false if neither exists.
func (r *Release) EnsureChecksums() (bool, error) {
	d := filepath.Join(r.Work, "checksums")
	r.ChecksumsFile = ""
	if err := r.FindRun(); err != nil {
		return false, err
	}
	os.RemoveAll(d)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return false, err
	}
	file := filepath.Join(d, "checksums.txt")
	nonEmpty := func() bool { fi, err := os.Stat(file); return err == nil && fi.Size() > 0 }
	_, err := r.Run.Run("gh", "release", "download", r.Tag, "-R", "nself-org/cli", "-p", "checksums.txt", "-D", d, "--clobber")
	if err == nil && nonEmpty() {
		r.ChecksumsSrc = "the published release"
	} else if _, err2 := r.runIf(r.RunID != "", "gh", "run", "download", r.RunID, "-R", "nself-org/cli",
		"-n", "release-checksums", "-D", d); r.RunID != "" && err2 == nil && nonEmpty() {
		r.ChecksumsSrc = "the build artifact of run " + r.RunID
	} else {
		os.RemoveAll(d)
		return false, nil
	}
	r.ChecksumsFile = file
	return true, nil
}

func (r *Release) runIf(cond bool, name string, args ...string) (string, error) {
	if !cond {
		return "", nil
	}
	return r.Run.Run(name, args...)
}

// SumsHash is sums_hash: the darwin tarball's sha256 from checksums.txt text
// (arch is arm64 or amd64); the first line whose second field matches.
func (r *Release) SumsHash(sums, arch string) string {
	want := "nself-" + r.Version + "-darwin-" + arch + ".tar.gz"
	for _, l := range strings.Split(sums, "\n") {
		f := strings.Fields(l)
		if len(f) >= 2 && f[1] == want {
			return f[0]
		}
	}
	return ""
}

var (
	onArmRe   = regexp.MustCompile(`on_arm do`)
	onIntelRe = regexp.MustCompile(`on_intel do`)
	endLineRe = regexp.MustCompile(`^[ \t\r\n\f\v]*end[ \t\r\n\f\v]*$`)
	quotedRe  = regexp.MustCompile(`^[^"]*"([^"]*)"`)
	versionRe = regexp.MustCompile(`^[ \t\r\n\f\v]*version "([^"]*)"`)
)

// FormulaBlockSHA is formula_block_sha: the first sha256 line inside the
// on_arm or on_intel block (want is "arm" or "intel"), its quoted value.
func FormulaBlockSHA(formula, want string) string {
	block := ""
	for _, l := range strings.Split(formula, "\n") {
		if onArmRe.MatchString(l) {
			block = "arm"
		}
		if onIntelRe.MatchString(l) {
			block = "intel"
		}
		if endLineRe.MatchString(l) {
			block = ""
		}
		if block == want && strings.Contains(l, "sha256") {
			if m := quotedRe.FindStringSubmatch(l); m != nil {
				return m[1]
			}
			return ""
		}
	}
	return ""
}

// formulaVersion is the sed in load_formula: the first `version "x"` line.
func formulaVersion(formula string) string {
	for _, l := range strings.Split(formula, "\n") {
		if m := versionRe.FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// LoadFormula is load_formula: FVer FArm FIntel of Formula/nself.rb on
// homebrew-nself main.
func (r *Release) LoadFormula() error {
	out, err := r.ghNeed("Formula/nself.rb on homebrew-nself", "api", "-H", "Accept: application/vnd.github.raw",
		"repos/nself-org/homebrew-nself/contents/Formula/nself.rb")
	if err != nil {
		return err
	}
	r.FVer, r.FArm, r.FIntel = formulaVersion(out), FormulaBlockSHA(out, "arm"), FormulaBlockSHA(out, "intel")
	return nil
}
