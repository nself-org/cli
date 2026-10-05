package backup

// drill_verify.go — what the drill accepts as proof of a good restore.
//
// Purpose: a drill passes only when the restored row counts match the source
// counts recorded in the backup heartbeat, and no pg_restore error went
// unexplained. Without source counts it cannot verify and fails (E218).
// Inputs: restored counts, the backup heartbeat counts, pg_restore stderr.
// Outputs: mismatch strings, the reason counts are missing, fatal restore text.
// Constraints: the counts are n_live_tup estimates, so a restored count may
// fall short of the estimate by up to 10% before it is a mismatch; a table the
// backup counted non-empty must have rows. Every pg_restore line with "error:"
// is fatal unless allowedRestoreErrors lists it.

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
)

const cannotVerify = "cannot verify: no source row counts for this backup"

// allowedRestoreErrors are substrings of pg_restore "error:" lines that are
// known to be harmless in the throwaway container. Empty: no entry has been
// proven harmless against a real stream backup, so every error fails the
// drill. Add an entry only with a comment saying why it cannot lose data.
var allowedRestoreErrors = []string{}

// fatalRestoreText returns the text that makes a restore fail: a connection or
// input failure (either tool), or, for pg_restore, any "error:" line that is
// not allowlisted. An empty result means the restore stands.
func fatalRestoreText(tool, stderr string) string {
	for _, fatal := range []string{"FATAL", "unrecognized archive", "input file", "No such file", "connection to server"} {
		if strings.Contains(stderr, fatal) {
			return strings.TrimSpace(stderr)
		}
	}
	if tool != "pg_restore" {
		return ""
	}
	var bad []string
lines:
	for _, l := range strings.Split(stderr, "\n") {
		if !strings.Contains(strings.ToLower(l), "error:") {
			continue
		}
		for _, ok := range allowedRestoreErrors {
			if strings.Contains(l, ok) {
				continue lines
			}
		}
		bad = append(bad, strings.TrimSpace(l))
	}
	return strings.Join(bad, "; ")
}

// compareRows compares the restored counts with the source counts. With no
// source counts it returns the single "cannot verify" mismatch: an unverified
// restore is never a pass.
func compareRows(estimates, restored map[string]int64) []string {
	if len(estimates) == 0 {
		return []string{cannotVerify}
	}
	m := []string{}
	for t, est := range estimates {
		got := restored[t]
		switch {
		case est > 0 && got <= 0:
			m = append(m, fmt.Sprintf("%s: no rows restored, the backup had about %d", t, est))
		case est-got > est/10:
			m = append(m, fmt.Sprintf("%s: %d rows restored, the backup had about %d", t, got, est))
		}
	}
	sort.Strings(m)
	return m
}

// loadEstimates reads the source counts from the backup heartbeat. It returns
// nil counts and a warning when they cannot be trusted for obj: no heartbeat
// remote, no readable heartbeat, a heartbeat for another object, or no counts.
func loadEstimates(ctx context.Context, hbDest destinations.Destination, project string, obj destinations.Object) (map[string]int64, []string) {
	warn := func(msg string) (map[string]int64, []string) {
		slog.Warn(msg)
		return nil, []string{msg}
	}
	if hbDest == nil {
		return warn("no heartbeat remote: the backup's source row counts are not available")
	}
	b, st, _ := fetchHeartbeat(ctx, hbDest, project, "backup", time.Now())
	switch {
	case st != hbOK:
		return warn("the backup heartbeat is missing or unreadable: source row counts are not available")
	case b.BackupKey != path.Base(obj.Key):
		return warn(fmt.Sprintf("the backup heartbeat describes %s, not the chosen %s: treated as no source row counts", b.BackupKey, path.Base(obj.Key)))
	case len(b.ApproxRows) == 0:
		return warn("the backup heartbeat has no row counts")
	}
	return b.ApproxRows, nil
}
