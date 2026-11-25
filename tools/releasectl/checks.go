package main

// Port of scripts/release/lib/checks.sh: the required-checks gate that runs
// before any admin-merge call. GET-only, time-bounded, never merges.

import (
	"regexp"
	"strings"
	"time"
)

// checksBudget is CHECKS_BUDGET: polls of 30 s, 45 min of pending checks.
const checksBudget = 90

// checksPoll is the 30 s sleep of wait_checks (its local POLL_SECONDS).
const checksPoll = 30 * time.Second

// unreadable is the "?" answer of required_contexts and missing_required.
const unreadable = "?"

var noChecksRe = regexp.MustCompile(`(?i)no (required )?checks`)

// requiredContexts is required_contexts: the status checks main's protection
// requires. Empty for a 404 (none required); [unreadable] when it cannot be read.
func (r *Release) requiredContexts(repo string) []string {
	g := r.ghTry("api", "repos/nself-org/"+repo+"/branches/main/protection/required_status_checks",
		"--jq", "((.contexts // []) + [(.checks // [])[].context]) | unique | .[]")
	if g.Fail != nil {
		if g.is404() {
			return nil
		}
		return []string{unreadable}
	}
	var out []string
	for _, l := range strings.Split(g.Out, "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// missingRequired is missing_required: required contexts not passing on the PR,
// in order; [unreadable] if unknown; nil when all are present. A skipped check
// counts as satisfied.
func (r *Release) missingRequired(repo, n string) []string {
	ctx := r.requiredContexts(repo)
	if len(ctx) == 0 {
		return nil
	}
	if ctx[0] == unreadable {
		return []string{unreadable}
	}
	g := r.ghTry("pr", "checks", n, "-R", "nself-org/"+repo, "--required", "--json", "name,bucket",
		"--jq", `.[] | select(.bucket == "pass" or .bucket == "skipping") | .name`)
	if g.Fail != nil {
		return []string{unreadable}
	}
	got := map[string]bool{}
	for _, l := range strings.Split(g.Out, "\n") {
		got[l] = true
	}
	var miss []string
	for _, c := range ctx {
		if !got[c] {
			miss = append(miss, c)
		}
	}
	return miss
}

// joinTrail renders names the way the shell printed them: each followed by a space.
func joinTrail(names []string) string {
	var b strings.Builder
	for _, n := range names {
		b.WriteString(n + " ")
	}
	return b.String()
}

// WaitChecks is wait_checks: non-watch `gh pr checks --required` in a loop.
// Exit 0 is green, 8 is pending (bounded wait), anything else is red or "no
// checks". "No checks" passes only when protection requires none; required but
// unreported checks are retried six times, then the run stops.
func (r *Release) WaitChecks(s, id, repo, n string) error {
	pending, tries := 0, 0
	miss := ""
	for {
		out, err := r.Run.Run("gh", "pr", "checks", n, "-R", "nself-org/"+repo, "--required")
		rc := 0
		if err != nil {
			rc = 1
			if re, ok := err.(*RunError); ok {
				rc = re.Code
				out += re.Stderr
			}
		}
		r.echoHead(repo, n, out)
		switch {
		case rc == 8:
			pending++
			if pending >= checksBudget {
				return r.die(s, id, repo+"#"+n+": required checks still pending after 45 minutes; nothing was merged. Look at the PR, then --from "+s)
			}
			r.Sleep(checksPoll)
			continue
		case rc == 0:
			m := r.missingRequired(repo, n)
			if len(m) == 0 {
				return nil
			}
			if m[0] == unreadable {
				return r.die(s, id, "cannot read the required checks of "+repo+"#"+n+"; nothing was merged")
			}
			miss = joinTrail(m)
		case noChecksRe.MatchString(out):
			ctx := r.requiredContexts(repo)
			if len(ctx) == 0 {
				r.say("%s %s: %s requires no status checks on main; continuing to the owner prompt", s, id, repo)
				return nil
			}
			if ctx[0] == unreadable {
				return r.die(s, id, "cannot read the required checks of "+repo+"; nothing was merged")
			}
			miss = strings.Join(ctx, " ")
		default:
			return r.die(s, id, "required checks on "+repo+"#"+n+" are not green; nothing was merged")
		}
		tries++
		if tries >= 6 {
			return r.die(s, id, repo+"#"+n+": required checks not reported as passing after 6 tries ("+miss+"); nothing was merged")
		}
		r.Sleep(checksPoll)
	}
}

// echoHead prints the first four output lines prefixed with "  repo#n: ".
func (r *Release) echoHead(repo, n, out string) {
	lines := strings.SplitAfter(out, "\n")
	for i, l := range lines {
		if i >= 4 || l == "" {
			break
		}
		if !strings.HasSuffix(l, "\n") {
			l += "\n"
		}
		r.Out.Write([]byte("  " + repo + "#" + n + ": " + l))
	}
}
