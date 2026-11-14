package main

import (
	"fmt"
	"strings"
)

func (d Deps) rinfo(f string, a ...any) { fmt.Fprintf(d.Stdout, "[release-retract] "+f+"\n", a...) }
func (d Deps) rwarn(f string, a ...any) {
	fmt.Fprintf(d.Stderr, "[release-retract] WARN: "+f+"\n", a...)
}

// runRetract ports scripts/release-retract.sh through the Runner.
func runRetract(d Deps, args []string) int {
	force, del, tag, reason := false, false, "", ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--force":
			force = true
		case a == "--delete-tag":
			del = true
		case a == "--reason":
			if i+1 >= len(args) {
				fmt.Fprintf(d.Stderr, "ERROR: --reason needs a value\n")
				return 2
			}
			i++
			reason = args[i]
		case tag == "" && !strings.HasPrefix(a, "--"):
			tag = a
		default:
			fmt.Fprintf(d.Stderr, "ERROR: unknown arg: %s\n", a)
			return 2
		}
	}
	if tag == "" {
		fmt.Fprintf(d.Stderr, "ERROR: usage: releasectl retract <tag> [--force] [--delete-tag] [--reason <text>]\n")
		return 2
	}
	if reason == "" {
		reason = d.Env("RELEASE_RETRACT_REASON")
	}
	if reason == "" && d.Interactive {
		fmt.Fprint(d.Stderr, "Reason for retraction (required, will be logged): ")
		reason = d.readLine()
	}
	if reason == "" {
		fmt.Fprintf(d.Stderr, "ERROR: --reason is required (or set RELEASE_RETRACT_REASON env var)\n")
		return 2
	}
	if !d.HaveGH() {
		fmt.Fprintf(d.Stderr, "ERROR: gh CLI not found\n")
		return 2
	}
	if _, err := d.Runner.Run("gh", "release", "view", tag); err != nil {
		fmt.Fprintf(d.Stderr, "ERROR: no GitHub release found for tag %s\n", tag)
		return 2
	}
	if !force {
		// `gh release view` with no tag is GitHub's latest release: published,
		// not a draft and not a prerelease. `gh release list --limit 1` is the
		// newest by creation date and counts drafts, so an earlier retraction
		// or a drafted next release would hide the real latest.
		out, err := d.Runner.Run("gh", "release", "view", "--json", "tagName", "-q", ".tagName")
		if err != nil {
			fmt.Fprintf(d.Stderr, "ERROR: cannot read the latest release (gh release view): %v\n", err)
			return 1
		}
		if strings.TrimSpace(out) != tag {
			fmt.Fprintf(d.Stderr, "ERROR: %s is not the latest release; use --force to retract anyway\n", tag)
			return 1
		}
	}
	return d.retract(tag, reason, del)
}

// retract performs the confirmed steps: draft, notes, tag delete, audit, hint.
func (d Deps) retract(tag, reason string, del bool) int {
	d.rwarn("About to retract release: %s", tag)
	d.rwarn("Reason: %s", reason)
	yn := "NO"
	if del {
		yn = "YES"
	}
	d.rwarn("Delete tag (local + remote): %s", yn)
	if d.Interactive {
		fmt.Fprint(d.Stderr, "Type RETRACT to confirm: ")
		if d.readLine() != "RETRACT" {
			fmt.Fprintln(d.Stderr, "Aborted by operator.")
			return 1
		}
		if del {
			fmt.Fprint(d.Stderr, "Tag deletion is destructive. Type DELETE to confirm: ")
			if d.readLine() != "DELETE" {
				fmt.Fprintln(d.Stderr, "Tag deletion aborted; proceeding with draft + note update only.")
				del = false
			}
		}
	}
	d.rinfo("marking release %s as draft", tag)
	if !d.step("gh", "release", "edit", tag, "--draft") {
		return 1
	}
	d.rinfo("fetching current release notes")
	cur, _ := d.Runner.Run("gh", "release", "view", tag, "--json", "body", "-q", ".body")
	ts := d.Now().UTC().Format("2006-01-02T15:04:05Z")
	notes := fmt.Sprintf("## RETRACTED %s\n\n**Reason:** %s\n\n---\n\n%s", ts, reason, strings.TrimRight(cur, "\n"))
	d.rinfo("prepending RETRACTED notice")
	if !d.step("gh", "release", "edit", tag, "--notes", notes) {
		return 1
	}
	if del {
		d.rwarn("deleting tag %s locally", tag)
		d.Runner.Run("git", "-C", d.Root, "tag", "-d", tag)
		d.rwarn("deleting tag %s on origin", tag)
		if _, err := d.Runner.Run("git", "-C", d.Root, "push", "origin", ":refs/tags/"+tag); err != nil {
			d.rwarn("remote tag delete failed (may already be gone)")
		}
	}
	return d.finishRetract(tag, reason, ts, del)
}

func (d Deps) step(name string, args ...string) bool {
	if _, err := d.Runner.Run(name, args...); err != nil {
		fmt.Fprintf(d.Stderr, "ERROR: %s %s failed: %v\n", name, joinArgs(args[:2]), err)
		return false
	}
	return true
}
