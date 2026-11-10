package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runCreate ports scripts/release-create.sh: denylist, optional override, gh release create.
func runCreate(d Deps, args []string) int {
	dryRun, skip, tag := false, false, ""
	var pass []string
	for _, a := range args {
		switch {
		case a == "--dry-run":
			dryRun = true
		case a == "--skip-denylist":
			skip = true
		case tag == "":
			tag = a
		default:
			pass = append(pass, a)
		}
	}
	// The tag is the first argument that is not one of our own flags; a flag
	// meant for gh in that place means the tag is missing.
	if tag == "" || strings.HasPrefix(tag, "-") {
		fmt.Fprintf(d.Stderr, "ERROR: usage: releasectl create <tag> [--dry-run] [--skip-denylist] [gh-release-args...]\n")
		return 2
	}
	ghArgs := append([]string{"release", "create", tag}, pass...)
	if dryRun {
		d.info("invoking: gh %s", joinArgs(ghArgs))
		return 0
	}
	deny := filepath.Join(d.Root, ".github", "release-denylist.json")
	if _, err := os.Stat(deny); err != nil {
		fmt.Fprintf(d.Stderr, "ERROR: denylist file not found: %s\n", deny)
		return 2
	}
	d.info("evaluating denylist for tag: %s", tag)
	t := d.triggered(tag)
	if len(t) == 0 {
		d.info("denylist: PASS (0 rules triggered)")
	} else {
		d.fail("denylist: %d rule(s) triggered", len(t))
		for _, r := range t {
			fmt.Fprintf(d.Stderr, "  - %s\n", r)
		}
		if !skip {
			d.fail("release blocked. Re-run with --skip-denylist + justification to override.")
			return 1
		}
		if code := d.override(tag, t); code != 0 {
			return code
		}
	}
	d.info("invoking: gh %s", joinArgs(ghArgs))
	return d.execTTY("gh", ghArgs...)
}

// execTTY is the script's `exec gh ...`: the command runs on the operator's
// terminal (stdin, stdout and stderr pass through, so gh can prompt and its
// output is not buffered) and its exit code becomes ours.
func (d Deps) execTTY(name string, args ...string) int {
	var err error
	if t, ok := d.Runner.(DirRunner); ok {
		_, err = t.RunIn(d.Root, true, name, args...)
	} else {
		var out string
		out, err = d.Runner.Run(name, args...)
		fmt.Fprint(d.Stdout, out)
	}
	if err == nil {
		return 0
	}
	var re *RunError
	if errors.As(err, &re) && re.Code > 0 {
		return re.Code
	}
	d.fail("%s %s failed: %v", name, joinArgs(args[:min(2, len(args))]), err)
	return 1
}
