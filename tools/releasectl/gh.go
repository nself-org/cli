package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// Release carries the globals of the shell release run (VERSION TAG WORK STATE
// CUR_S CUR_ID and the values the helpers leave behind).
type Release struct {
	Run      Runner
	Out      io.Writer
	Sleep    func(time.Duration)
	Poll     time.Duration // POLL_SECONDS
	Version  string
	Tag      string
	Work     string
	State    string // state file path
	Plan     bool
	Step, ID string // CUR_S, CUR_ID

	TagSHA, RunID, ChecksumsSrc, ChecksumsFile string
	FVer, FArm, FIntel, Gate                   string

	// Yes is --yes: skip release.sh's own "Run S<n>?" prompt (never admin-merge's).
	Yes bool
	// DryRun is --dry-run: print the plan rows and call nothing at all.
	DryRun bool
	// Err receives the stderr lines of the shell (err, warnings).
	Err io.Writer
	// In answers the "Run S<n>? [y/N]" prompts.
	In *bufio.Reader
	// AdminMerge is the path of scripts/admin-merge.sh.
	AdminMerge string
	// Have reports whether a tool is on PATH (command -v).
	Have func(string) bool

	stepState
}

// StepError is die_step: a step failed and says how to resume.
type StepError struct {
	Step, ID, What, Resume, Version string
}

// Error renders the two stderr lines die_step prints.
func (e *StepError) Error() string {
	return fmt.Sprintf("release: %s %s failed: %s\nrelease: recovery: fix the cause, then run: scripts/release/release.sh --version %s --from %s",
		e.Step, e.ID, e.What, e.Version, e.Resume)
}

// FatalError is err + exit 1: one "release: ..." line.
type FatalError struct{ Msg string }

// Error renders the stderr line.
func (e *FatalError) Error() string { return "release: " + e.Msg }

// die is die_step with an optional resume step (default: the same step).
func (r *Release) die(s, id, what string, resume ...string) error {
	from := s
	if len(resume) > 0 {
		from = resume[0]
	}
	return &StepError{Step: s, ID: id, What: what, Resume: from, Version: r.Version}
}

func (r *Release) say(format string, a ...any) { fmt.Fprintf(r.Out, format+"\n", a...) }

var notFoundRe = regexp.MustCompile(`404|Not Found|not protected|not enabled`)

// ghResult is gh_try: stdout, stderr and the error of one gh call.
type ghResult struct {
	Out, Err string
	Fail     error
	Code     int
}

// is404 matches the stderr texts gh prints for an absent resource.
func (g ghResult) is404() bool { return notFoundRe.MatchString(g.Err) }

// ghTry runs gh with stdout and stderr kept apart.
func (r *Release) ghTry(args ...string) ghResult {
	out, err := r.Run.Run("gh", args...)
	g := ghResult{Out: strings.TrimRight(out, "\n"), Fail: err}
	var re *RunError
	if errors.As(err, &re) {
		g.Err, g.Code = strings.TrimRight(re.Stderr, "\n"), re.Code
	} else if err != nil {
		g.Err, g.Code = err.Error(), 1
	}
	return g
}

// ghNeed is gh_need: a failure ends the step with a recovery line.
func (r *Release) ghNeed(what string, args ...string) (string, error) {
	g := r.ghTry(args...)
	if g.Fail != nil {
		msg := g.Err
		if msg == "" {
			msg = "gh failed"
		}
		return "", r.die(r.Step, r.ID, "cannot read "+what+": "+msg)
	}
	return g.Out, nil
}

// repoTagSHA is repo_tag_sha: true and TagSHA when the tag exists, false when
// absent; any other failure is fatal.
func (r *Release) repoTagSHA(repo, tag string) (bool, error) {
	g := r.ghTry("api", "repos/nself-org/"+repo+"/commits/"+tag, "--jq", ".sha")
	if g.Fail == nil {
		r.TagSHA = g.Out
		return true, nil
	}
	if g.is404() || strings.Contains(g.Err, "422") || strings.Contains(g.Err, "No commit found") {
		return false, nil
	}
	return false, &FatalError{"cannot resolve " + tag + " on " + repo + ": " + g.Err}
}

// stateGet is state_get: the last value of key in the state file.
func (r *Release) stateGet(key string) string {
	b, err := os.ReadFile(r.State)
	if err != nil {
		return ""
	}
	val := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, key+"=") {
			val = strings.TrimPrefix(l, key+"=")
		}
	}
	return val
}

// stateSet is state_set: plan mode never writes.
func (r *Release) stateSet(key, val string) error {
	if r.Plan {
		return nil
	}
	var keep []string
	if b, err := os.ReadFile(r.State); err == nil {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if l != "" && !strings.HasPrefix(l, key+"=") {
				keep = append(keep, l)
			}
		}
	}
	keep = append(keep, key+"="+val)
	return os.WriteFile(r.State, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}

// poll is poll: try cond up to tries times, sleeping between tries.
func (r *Release) poll(what string, tries int, cond func() (bool, error)) (bool, error) {
	for i := 0; i < tries; {
		ok, err := cond()
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
		i++
		r.say("  waiting for %s (%d/%d)", what, i, tries)
		r.Sleep(r.Poll)
	}
	return false, nil
}
