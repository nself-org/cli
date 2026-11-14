package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// override confirms a --skip-denylist run and appends the audit record.
func (d Deps) override(tag string, triggered []string) int {
	d.warn("--skip-denylist provided. Interactive confirmation required.")
	var reason string
	if d.Interactive {
		fmt.Fprint(d.Stderr, "Type the word OVERRIDE to confirm: ")
		if d.readLine() != "OVERRIDE" {
			d.fail("override aborted by operator")
			return 1
		}
		fmt.Fprint(d.Stderr, "Reason for override (one line, will be logged): ")
		if reason = d.readLine(); reason == "" {
			d.fail("override reason required")
			return 1
		}
	} else if reason = d.Env("RELEASE_OVERRIDE_REASON"); reason == "" {
		d.fail("non-interactive override requires RELEASE_OVERRIDE_REASON env var")
		return 1
	}
	dir := filepath.Join(d.Root, "audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		d.fail("cannot create %s: %v", dir, err)
		return 1
	}
	sha, _ := d.Runner.Run("git", "-C", d.Root, "rev-parse", "HEAD")
	rec := fmt.Sprintf("%s | sha=%s | tag=%s | operator=%s | skipped_rules=[%s] | reason=%s\n",
		d.Now().UTC().Format("2006-01-02T15:04:05Z"), strings.TrimSpace(sha), tag,
		d.operator(), strings.Join(triggered, ","), reason)
	log := filepath.Join(dir, "release-overrides.log")
	if err := appendFile(log, rec); err != nil {
		d.fail("cannot write %s: %v", log, err)
		return 1
	}
	d.warn("override logged to %s", log)
	return 0
}

// operator names who ran the command.
func (d Deps) operator() string {
	for _, k := range []string{"GITHUB_ACTOR", "USER"} {
		if v := d.Env(k); v != "" {
			return v
		}
	}
	return "unknown"
}

func appendFile(path, s string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(s)
	return err
}
