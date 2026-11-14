package main

import "fmt"

// runGate is `releasectl gate <tag>`: evaluates the denylist rules and exits 1
// when any triggers. It is what .github/workflows/pre-release-gate.yml runs, so
// CI and `releasectl create` share one set of rules. If GITHUB_STEP_SUMMARY is
// set, the result is also appended there.
func runGate(d Deps, args []string) int {
	if len(args) != 1 {
		fmt.Fprintf(d.Stderr, "ERROR: usage: releasectl gate <tag>\n")
		return 2
	}
	tag := args[0]
	d.info("evaluating denylist for tag: %s", tag)
	t := d.triggered(tag)
	sum := d.Env("GITHUB_STEP_SUMMARY")
	if len(t) == 0 {
		d.info("denylist: PASS (0 rules triggered)")
		if sum != "" {
			appendFile(sum, fmt.Sprintf("## Pre-Release Gate: PASS\nAll evaluable denylist rules passed for %s.\n", tag))
		}
		return 0
	}
	d.fail("denylist: %d rule(s) triggered", len(t))
	body := "## Pre-Release Gate: BLOCKED\nTriggered rules:\n"
	for _, r := range t {
		fmt.Fprintf(d.Stderr, "::error::  - %s\n", r)
		body += "- " + r + "\n"
	}
	body += fmt.Sprintf("\nOverride via: `go run ./tools/releasectl create %s --skip-denylist`\n", tag)
	if sum != "" {
		appendFile(sum, body)
	}
	return 1
}
