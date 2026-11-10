package main

// Port of scripts/release/release.sh: the `releasectl run` command. It parses
// the flags, applies the version guards, then runs steps S1..S10 in order.
// Every external command goes through the Runner.

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const runUsage = `usage: releasectl run --version X.Y.Z [--plan | --dry-run] [--from S<n>] [--yes]
  --version  target release, X.Y.Z (no v)
  --plan     print "S<n> <step-id> <repo> <command>" rows; only gh api GETs
  --dry-run  print the same rows and call nothing at all
  --from     resume at step S<n> (steps recover their inputs from state or GitHub)
  --yes      skip the own "Run S<n>? [y/N]" prompt only; admin-merge.sh always asks the owner
`

var (
	semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	latestRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+`)
	fromRe   = regexp.MustCompile(`^S([1-9]|10)$`)
)

// runOpts are the parsed flags of `run`.
type runOpts struct {
	Version           string
	Plan, DryRun, Yes bool
	From              int
}

// stepFn is one release step.
type stepFn struct {
	ID  string
	Run func(*Release) error
}

// releaseSteps is the ordered S1..S10 sequence.
var releaseSteps = []stepFn{
	{"preflight", (*Release).StepPreflight},
	{"admin-lockstep", (*Release).StepAdminLockstep},
	{"cli-version", (*Release).StepCliVersion},
	{"tag", (*Release).StepTag},
	{"build-wait", (*Release).StepBuildWait},
	{"homebrew-formula", (*Release).StepHomebrewFormula},
	{"release-wait", (*Release).StepReleaseWait},
	{"docker-verify", (*Release).StepDockerVerify},
	{"admin-image", (*Release).StepAdminImage},
	{"sdk-publish", (*Release).StepSdkPublish},
}

// parseRunArgs reads the flags; ok is false (with the exit code) on refusal.
func parseRunArgs(d Deps, args []string) (o runOpts, code int, ok bool) {
	o.From = 1
	fail := func(msg string) (runOpts, int, bool) {
		fmt.Fprintf(d.Stderr, "release: %s\n", msg)
		return o, 2, false
	}
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--version":
			if i+1 < len(args) {
				i++
				o.Version = args[i]
			}
		case "--plan":
			o.Plan = true
		case "--dry-run":
			o.Plan, o.DryRun = true, true
		case "--yes":
			o.Yes = true
		case "--from":
			if i+1 >= len(args) || !fromRe.MatchString(args[i+1]) {
				return fail("--from needs S1..S10")
			}
			i++
			o.From, _ = strconv.Atoi(args[i][1:])
		case "-h", "--help":
			fmt.Fprint(d.Stderr, runUsage)
			return o, 0, false
		default:
			fmt.Fprintf(d.Stderr, "release: unknown argument: %s\n%s", a, runUsage)
			return o, 2, false
		}
	}
	if !semverRe.MatchString(o.Version) {
		return fail("--version X.Y.Z is required (got '" + o.Version + "')")
	}
	return o, 0, true
}

// verGT is ver_gt: semver a > b (X.Y.Z only).
func verGT(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < 3; i++ {
		var p, q int
		if i < len(x) {
			p, _ = strconv.Atoi(x[i])
		}
		if i < len(y) {
			q, _ = strconv.Atoi(y[i])
		}
		if p != q {
			return p > q
		}
	}
	return false
}

// newRelease builds the Release of one run from the injected dependencies.
func newRelease(d Deps, o runOpts) *Release {
	tmp := strings.TrimRight(d.Env("TMPDIR"), "/")
	if d.Env("TMPDIR") == "" {
		tmp = "/tmp"
	}
	work := tmp + "/nself-release-" + o.Version
	have := d.Have
	if have == nil {
		have = func(string) bool { return true }
	}
	sleep := d.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	return &Release{
		Run: d.Runner, Out: d.Stdout, Err: d.Stderr, In: d.Stdin, Sleep: sleep, Poll: pollSeconds,
		Version: o.Version, Tag: "v" + o.Version, Work: work, State: work + ".state",
		Plan: o.Plan, DryRun: o.DryRun, Yes: o.Yes, Have: have,
		AdminMerge: d.Root + "/scripts/admin-merge.sh", Step: "S?", ID: "?",
	}
}

// RunRelease is `releasectl run`: the port of release.sh.
func RunRelease(d Deps, args []string) int {
	o, code, ok := parseRunArgs(d, args)
	if !ok {
		return code
	}
	r := newRelease(d, o)
	if !o.Plan && !d.Interactive {
		r.warn("stdin is not a terminal; a real run needs the owner at the keyboard (use --plan to preview)")
		return 2
	}
	if !o.DryRun {
		if !d.HaveGH() {
			r.warn("gh is not installed")
			return 1
		}
		if !o.Plan {
			if _, err := os.Stat(r.AdminMerge); err != nil {
				r.warn("admin-merge.sh missing next to scripts/release/")
				return 1
			}
		}
		if code := r.guardVersion(o.From); code != 0 {
			return code
		}
	}
	return r.runSteps(o)
}

// runSteps runs S<from>..S10; a failing step stops the run.
func (r *Release) runSteps(o runOpts) int {
	if o.Plan {
		r.warn("plan for %s from S%d (read-only: gh api GETs only)", r.Tag, o.From)
	}
	for n := o.From; n <= len(releaseSteps); n++ {
		st := releaseSteps[n-1]
		r.Step, r.ID = fmt.Sprintf("S%d", n), st.ID
		if err := st.Run(r); err != nil {
			fmt.Fprintln(r.Err, err.Error())
			if _, isStep := err.(*StepError); !isStep {
				r.warn("%s %s failed", r.Step, r.ID)
			}
			return 1
		}
	}
	if !o.Plan {
		r.say("release %s: S%d..S10 complete", r.Tag, o.From)
		r.cleanup()
	}
	return 0
}

// cleanup is release.sh's last line: `case "$WORK" in */nself-release-"$VERSION")
// rm -rf "$WORK" "$STATE"`. The guard keeps a Release whose Work was pointed
// anywhere else from deleting it.
func (r *Release) cleanup() {
	if !strings.HasSuffix(r.Work, "/nself-release-"+r.Version) {
		return
	}
	os.RemoveAll(r.Work)
	os.Remove(r.State)
}
