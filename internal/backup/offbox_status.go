package backup

// offbox_status.go — off-box backup freshness from heartbeat objects
// (contract:cli.backup-status v1, the `offbox` object).
//
// Purpose: say whether the newest off-box backup and the newest restore drill
// are fresh, from `<project>/backup.json` and `<project>/drill.json` in the
// heartbeat remote, without reaching the production box.
// Inputs: the heartbeat remote URI, the project name, optional thresholds.
// Outputs: an OffboxStatus and, when a threshold is exceeded or a heartbeat
// cannot be trusted, coded errors E217 (backup stale), E218 (drill stale or
// failed), E219 (heartbeat unreadable).
// Constraints: fail closed. A missing, stale, failed, unparseable, wrong-kind,
// wrong-version, wrong-project or future-dated heartbeat is never OK, and the
// age is always shown. Error text never carries the remote, a host or a bucket.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/errs"
)

const (
	maxHeartbeatBytes = 1 << 20
	futureSlack       = 5 * time.Minute
)

// OffboxStatus is the `offbox` object of `nself backup status --format json`.
// Field order is the contract order.
type OffboxStatus struct {
	Source              string     `json:"source"` // "heartbeat" or "none"
	LastBackup          *Heartbeat `json:"last_backup"`
	LastDrill           *Heartbeat `json:"last_drill"`
	BackupAgeSeconds    *int64     `json:"backup_age_seconds"`
	DrillAgeSeconds     *int64     `json:"drill_age_seconds"`
	MaxAgeExceeded      bool       `json:"max_age_exceeded"`
	MaxDrillAgeExceeded bool       `json:"max_drill_age_exceeded"`
}

// OffboxOptions are the thresholds; zero means "not checked".
type OffboxOptions struct {
	MaxAge      time.Duration
	MaxDrillAge time.Duration
}

var ageRe = regexp.MustCompile(`^(\d+)d(.*)$`)

// ParseAge parses a duration that also accepts days: "26h", "35d", "1d12h".
func ParseAge(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	var days int64
	if m := ageRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.ParseInt(m[1], 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid age %q", s)
		}
		days, s = n, m[2]
	}
	var rest time.Duration
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("invalid age %q: use e.g. 26h, 35d or 1d12h", s)
	}
	if s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid age %q: use e.g. 26h, 35d or 1d12h", s)
		}
		rest = d
	}
	total := time.Duration(days)*24*time.Hour + rest
	if total <= 0 {
		return 0, fmt.Errorf("age %q must be positive", s)
	}
	return total, nil
}

// OpenDestination parses uri; a host:// URI loads the inventory of the
// current directory.
func OpenDestination(uri string) (destinations.Destination, error) {
	var inv *destinations.Inventory
	if destinations.KindOf(uri) == destinations.KindHost {
		var err error
		if inv, err = controlplane.Load("."); err != nil {
			return nil, err
		}
	}
	return destinations.Parse(uri, inv)
}

// hbState is the outcome of reading one heartbeat object.
type hbState int

const (
	hbOK hbState = iota
	hbMissing
	hbUnreadable
)

// fetchHeartbeat reads and validates <project>/<kind>.json.
func fetchHeartbeat(ctx context.Context, dest destinations.Destination, project, kind string, now time.Time) (*Heartbeat, hbState, error) {
	dir, err := os.MkdirTemp("", "nself-hb-")
	if err != nil {
		return nil, hbUnreadable, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	local := filepath.Join(dir, kind+".json")
	if err := dest.Get(ctx, project+"/"+kind+".json", local); err != nil {
		msg := strings.ToLower(err.Error())
		if errors.Is(err, fs.ErrNotExist) || strings.Contains(msg, "not found") || strings.Contains(msg, "no such") {
			return nil, hbMissing, nil
		}
		return nil, hbUnreadable, fmt.Errorf("%s heartbeat could not be fetched", kind)
	}
	f, err := os.Open(local)
	if err != nil {
		return nil, hbUnreadable, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxHeartbeatBytes+1))
	if err != nil || len(data) > maxHeartbeatBytes {
		return nil, hbUnreadable, fmt.Errorf("%s heartbeat is unreadable or larger than %d bytes", kind, maxHeartbeatBytes)
	}
	hb, err := ParseHeartbeat(data, project, kind, now)
	if err != nil {
		return nil, hbUnreadable, err
	}
	return hb, hbOK, nil
}

// ParseHeartbeat validates a heartbeat object. Unknown fields are accepted
// (additive fields stay v1); everything else must match the contract.
func ParseHeartbeat(data []byte, project, kind string, now time.Time) (*Heartbeat, error) {
	var hb Heartbeat
	if err := json.Unmarshal(data, &hb); err != nil {
		return nil, fmt.Errorf("%s heartbeat is not valid JSON", kind)
	}
	switch {
	case hb.SchemaVersion != HeartbeatSchemaVersion:
		return nil, fmt.Errorf("%s heartbeat has unsupported schema_version %q", kind, hb.SchemaVersion)
	case hb.Kind != kind:
		return nil, fmt.Errorf("%s heartbeat has kind %q", kind, hb.Kind)
	case hb.Project != project:
		return nil, fmt.Errorf("%s heartbeat belongs to project %q, not %q", kind, hb.Project, project)
	case hb.Result != "ok" && hb.Result != "failed":
		return nil, fmt.Errorf("%s heartbeat has result %q", kind, hb.Result)
	}
	at, err := time.Parse(time.RFC3339, hb.At)
	if err != nil {
		return nil, fmt.Errorf("%s heartbeat has an unparseable time", kind)
	}
	if at.After(now.Add(futureSlack)) {
		return nil, fmt.Errorf("%s heartbeat is dated in the future (%s)", kind, hb.At)
	}
	return &hb, nil
}

func ageSeconds(hb *Heartbeat, now time.Time) *int64 {
	at, err := time.Parse(time.RFC3339, hb.At)
	if err != nil {
		return nil
	}
	s := int64(now.Sub(at).Seconds())
	if s < 0 {
		s = 0
	}
	return &s
}

// ReadOffbox reads both heartbeats and applies the thresholds. The returned
// status is always usable; err is nil, one coded error, or several joined.
func ReadOffbox(ctx context.Context, remote, project string, opts OffboxOptions, now time.Time) (*OffboxStatus, error) {
	st := &OffboxStatus{Source: "none"}
	if strings.TrimSpace(remote) == "" {
		if opts.MaxAge > 0 || opts.MaxDrillAge > 0 {
			return st, errs.New("E219", "no heartbeat remote is configured, so freshness cannot be checked").
				WithFix("Pass --heartbeat-to <remote> or set NSELF_BACKUP_HEARTBEAT_REMOTE.")
		}
		return st, nil
	}
	if !heartbeatProjectRe.MatchString(project) {
		return st, fmt.Errorf("project name %q cannot be used as a heartbeat key", project)
	}
	st.Source = "heartbeat"
	dest, err := OpenDestination(remote)
	if err != nil {
		return st, errs.Wrap("E219", "the heartbeat remote cannot be opened", err)
	}
	var problems []error
	bk, bState, bErr := fetchHeartbeat(ctx, dest, project, "backup", now)
	dr, dState, dErr := fetchHeartbeat(ctx, dest, project, "drill", now)
	st.LastBackup, st.LastDrill = bk, dr
	if bk != nil {
		st.BackupAgeSeconds = ageSeconds(bk, now)
	}
	if dr != nil {
		st.DrillAgeSeconds = ageSeconds(dr, now)
	}
	if bState == hbUnreadable {
		problems = append(problems, errs.Wrap("E219", "backup heartbeat is unreadable: "+bErr.Error(), bErr))
	}
	if dState == hbUnreadable {
		problems = append(problems, errs.Wrap("E219", "drill heartbeat is unreadable: "+dErr.Error(), dErr))
	}
	if opts.MaxAge > 0 && bState != hbUnreadable {
		if e := checkFresh("E217", "backup", bk, st.BackupAgeSeconds, opts.MaxAge); e != nil {
			st.MaxAgeExceeded = true
			problems = append(problems, e)
		}
	}
	if opts.MaxDrillAge > 0 && dState != hbUnreadable {
		if e := checkFresh("E218", "drill", dr, st.DrillAgeSeconds, opts.MaxDrillAge); e != nil {
			st.MaxDrillAgeExceeded = true
			problems = append(problems, e)
		}
	}
	switch len(problems) {
	case 0:
		return st, nil
	case 1:
		return st, problems[0]
	}
	return st, errors.Join(problems...)
}

// checkFresh returns a coded error when hb is missing, failed or older than max.
func checkFresh(code, what string, hb *Heartbeat, age *int64, max time.Duration) error {
	if hb == nil || age == nil {
		return errs.New(code, fmt.Sprintf("no %s heartbeat found (limit %s)", what, max))
	}
	a := time.Duration(*age) * time.Second
	switch {
	case hb.Result != "ok":
		return errs.New(code, fmt.Sprintf("the last %s reported result %q, %s ago (at %s)", what, hb.Result, a, hb.At))
	case a > max:
		return errs.New(code, fmt.Sprintf("the last %s heartbeat is %s old (limit %s, at %s)", what, a, max, hb.At))
	}
	return nil
}

// Lines renders the two human lines of `backup status`.
func (s *OffboxStatus) Lines() []string {
	line := func(label string, hb *Heartbeat, age *int64, exceeded bool) string {
		switch {
		case hb == nil:
			return fmt.Sprintf("  %-17s none found", label)
		case exceeded || hb.Result != "ok":
			return fmt.Sprintf("  %-17s %s ago, result %s, NOT OK (%s)", label, time.Duration(*age)*time.Second, hb.Result, hb.At)
		}
		return fmt.Sprintf("  %-17s %s ago, result %s (%s)", label, time.Duration(*age)*time.Second, hb.Result, hb.At)
	}
	return []string{
		line("Off-box backup:", s.LastBackup, s.BackupAgeSeconds, s.MaxAgeExceeded),
		line("Off-box drill:", s.LastDrill, s.DrillAgeSeconds, s.MaxDrillAgeExceeded),
	}
}
