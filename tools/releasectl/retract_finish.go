package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// finishRetract writes the audit record and prints the cascade-rollback hint.
func (d Deps) finishRetract(tag, reason, ts string, deleted bool) int {
	dir := filepath.Join(d.Root, "audit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(d.Stderr, "ERROR: %v\n", err)
		return 1
	}
	sha, _ := d.Runner.Run("git", "-C", d.Root, "rev-parse", "HEAD")
	del := 0
	if deleted {
		del = 1
	}
	log := filepath.Join(dir, "release-retractions.log")
	rec := fmt.Sprintf("%s | sha=%s | tag=%s | operator=%s | deleted_tag=%d | reason=%s\n",
		ts, strings.TrimSpace(sha), tag, d.operator(), del, reason)
	if err := appendFile(log, rec); err != nil {
		fmt.Fprintf(d.Stderr, "ERROR: %v\n", err)
		return 1
	}
	d.rinfo("retraction logged to %s", log)
	fmt.Fprint(d.Stdout, strings.ReplaceAll(retractHint, "{TAG}", tag))
	return 0
}

const retractHint = `
==============================================================================
RETRACTION COMPLETE: {TAG}
==============================================================================
Next steps (manual — file PCIs to each affected inbox):

  1. homebrew-nself:  ~/Sites/nself/.claude/inbox/ (slug: hotfix-revert-formula-{TAG})
     → Roll formula back to prior version
  2. plugins-pro:     ~/Sites/nself/.claude/inbox/ (slug: hotfix-pin-prior-cli-{TAG})
     → Update integration tests + pinned CLI version
  3. admin:           ~/Sites/nself/.claude/inbox/ (slug: hotfix-cli-lockstep-{TAG})
     → Roll admin Docker image tag back if lockstep was bumped
  4. web/docs:        ~/Sites/nself/.claude/inbox/ (slug: hotfix-docs-retract-{TAG})
     → Add deprecation notice + redirect installer to prior version
  5. web/org:         ~/Sites/nself/.claude/inbox/ (slug: hotfix-changelog-retract-{TAG})
     → Mark changelog entry RETRACTED + reason
  6. ping_api:        Confirm license-validate route still resolves prior CLI

Use: pci-send nself <slug> high hotfix "..."
==============================================================================
`
