package controlplane

// snapshot.go: the verified database backup that precedes a production-class
// remote apply (D24 step 8, review F8), shared with promote (P7-DEPL-15).
//
// Purpose: on the host, `nself backup create --tag <tag>`, find the backup the
//          tag names in `nself backup list --format json`, then require
//          `nself backup verify <id>` to exit 0. Returns the backup id.
// Inputs:  a deploy.Remote, the remote project path and the tag.
// Outputs: the backup id, or E206 with nothing applied.
// Constraints: the commands are the host's own nself (cd <remote_path> first, as
//              the legacy metadata step does). Any failure at any of the three
//              steps is E206; a host with no nself says so. Before this
//              returns nil, no compose command has run.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// VerifiedSnapshot takes and verifies a backup tagged tag on the host behind r.
func VerifiedSnapshot(ctx context.Context, r deploy.Remote, remotePath, tag string) (string, error) {
	fail := func(what string, err error) (string, error) {
		hint := ""
		if err != nil && strings.Contains(err.Error(), "not found") {
			hint = " (is nself installed on the host and on the ssh PATH?)"
		}
		return "", errs.Wrap("E206", fmt.Sprintf("verified backup on %s failed at %s%s: %v; nothing was applied", r.Label(), what, hint, err), errs.ErrBackupVerifyFailed).
			WithFix("fix the backup on the host (`nself backup status` there), then deploy again")
	}
	cd := "cd " + remote.ShellQuote(remotePath) + " && "
	if _, err := r.Run(ctx, cd+"nself backup create --tag "+remote.ShellQuote(tag)); err != nil {
		return fail("backup create", err)
	}
	out, err := r.Run(ctx, cd+"nself backup list --format json")
	if err != nil {
		return fail("backup list", err)
	}
	id, err := backupIDByTag(out, tag)
	if err != nil {
		return fail("backup list", err)
	}
	if _, err := r.Run(ctx, cd+"nself backup verify "+remote.ShellQuote(id)); err != nil {
		return fail("backup verify "+id, err)
	}
	return id, nil
}

// tagged reports whether a list entry carries tag. `backup create` names the
// file <project>_<type>_<date>_<time>_<tag>, so the listed Tag is "<time>_<tag>"
// and the id ends in "_<tag>"; the tag we pass embeds a fresh random id, so a
// suffix match cannot hit another backup.
func tagged(id, entryTag, tag string) bool {
	return id != "" && (entryTag == tag || strings.HasSuffix(entryTag, "_"+tag) || strings.HasSuffix(id, "_"+tag))
}

// backupIDByTag finds the newest entry carrying tag in the list document: the
// bare JSON array `backup list --format json` prints, or an object with a
// "data" array (the --json envelope).
func backupIDByTag(raw []byte, tag string) (string, error) {
	var entries []struct {
		ID   string `json:"id"`
		Tag  string `json:"tag"`
		Date string `json:"date"`
	}
	body := strings.TrimSpace(string(raw))
	if i := strings.IndexAny(body, "[{"); i > 0 {
		body = body[i:]
	}
	if strings.HasPrefix(body, "{") {
		var env struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &env); err != nil {
			return "", fmt.Errorf("parsing the backup list: %w", err)
		}
		body = string(env.Data)
	}
	if err := json.Unmarshal([]byte(body), &entries); err != nil {
		return "", fmt.Errorf("parsing the backup list: %w", err)
	}
	best, bestDate := "", ""
	for _, e := range entries {
		if tagged(e.ID, e.Tag, tag) && e.Date >= bestDate {
			best, bestDate = e.ID, e.Date
		}
	}
	if best == "" {
		return "", fmt.Errorf("no backup tagged %q in the list", tag)
	}
	return best, nil
}
