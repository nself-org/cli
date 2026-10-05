package backup

// heartbeat.go — the off-box backup heartbeat (contract:cli.backup-heartbeat v1).
//
// Purpose: after a successful streamed backup, write `<project>/backup.json`
// to a separate heartbeat remote so backup freshness can be checked without
// reaching the box. Also reads the row estimates (`approx_rows`) that a later
// restore drill compares against.
// Inputs: the project config, the heartbeat remote, the finished StreamResult
// and the estimates read just before the dump.
// Outputs: one JSON object uploaded through the same rclone rcat funnel as the
// backup itself; an error when it cannot be written.
// Constraints: written only after the backup upload returned success and only
// when bytes were uploaded, so a failed backup never leaves a success heartbeat.
// The object never carries a secret, hostname or bucket name: only the project
// name, the object key (no remote), size, version and table row estimates.
// 2-space indent, sorted keys, trailing newline, schema_version "1".

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/cli/internal/version"
)

// HeartbeatSchemaVersion is the contract version written to every object.
const HeartbeatSchemaVersion = "1"

// approxRowsSQL reads the live-row estimates. Same statement as the contract.
const approxRowsSQL = `SELECT schemaname||'.'||relname, n_live_tup FROM pg_stat_user_tables ORDER BY 1`

// heartbeatProjectRe limits the project name used in the object key, so it can
// never add a path segment.
var heartbeatProjectRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Heartbeat is the backup.json object. Fields are declared in sorted JSON-key
// order so encoding/json writes them sorted; TestHeartbeatGoldenShape pins it.
type Heartbeat struct {
	ApproxRows    map[string]int64 `json:"approx_rows"` // backup: estimates, null when unreadable
	At            string           `json:"at"`          // RFC 3339 UTC, upload finish time
	BackupKey     string           `json:"backup_key"`  // object key, never the remote
	Bytes         int64            `json:"bytes"`       // size of the (encrypted) object
	CLIVersion    string           `json:"cli_version"`
	Encrypted     bool             `json:"encrypted"`
	Kind          string           `json:"kind"`
	Mismatches    []string         `json:"mismatches"` // drill only; [] for a backup
	Project       string           `json:"project"`
	RestoredRows  map[string]int64 `json:"restored_rows"` // drill only; null for a backup
	Result        string           `json:"result"`
	SchemaVersion string           `json:"schema_version"`
}

// Marshal renders the object: 2-space indent, sorted keys, trailing newline.
func (h Heartbeat) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(h); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// newBackupHeartbeat builds the object for a finished, successful backup.
func newBackupHeartbeat(project, key string, bytesUp int64, encrypted bool, approx map[string]int64, at time.Time, cliVersion string) Heartbeat {
	return Heartbeat{
		ApproxRows:    approx,
		At:            at.UTC().Format(time.RFC3339),
		BackupKey:     key,
		Bytes:         bytesUp,
		CLIVersion:    cliVersion,
		Encrypted:     encrypted,
		Kind:          "backup",
		Mismatches:    []string{},
		Project:       project,
		Result:        "ok",
		SchemaVersion: HeartbeatSchemaVersion,
	}
}

// WriteHeartbeat uploads hb to <remote>/<project>/backup.json through the same
// rclone rcat funnel the backup uses.
func WriteHeartbeat(ctx context.Context, remote string, hb Heartbeat) error {
	if strings.TrimSpace(remote) == "" {
		return fmt.Errorf("heartbeat remote is empty")
	}
	if !heartbeatProjectRe.MatchString(hb.Project) {
		return fmt.Errorf("project name %q cannot be used as a heartbeat key", hb.Project)
	}
	data, err := hb.Marshal()
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	return rcloneRcat(ctx, bytes.NewReader(data), remote, hb.Project+"/backup.json")
}

// publishHeartbeat writes the heartbeat for a backup that just uploaded. It
// refuses to write when nothing was uploaded.
func publishHeartbeat(ctx context.Context, cfgProject, remote string, res *StreamResult, approx map[string]int64) error {
	if res == nil || res.Bytes <= 0 {
		return fmt.Errorf("no bytes were uploaded, so no heartbeat is written")
	}
	hb := newBackupHeartbeat(cfgProject, res.BackupID, res.Bytes, res.Encrypted, approx, time.Now(), version.Version)
	if err := WriteHeartbeat(ctx, remote, hb); err != nil {
		return err
	}
	slog.Info("backup heartbeat written", "project", cfgProject)
	return nil
}

// approxQuery runs the row-estimate statement and returns psql's raw output.
// It is a variable so tests can substitute it.
var approxQuery = func(ctx context.Context, pgURL string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "psql", "-X", "-A", "-t", "-F", "\t",
		"--no-password", "-v", "ON_ERROR_STOP=1", "-c", approxRowsSQL, pgURL)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		// The DSN carries the password: never let it reach an error message.
		msg := strings.TrimSpace(errOut.String())
		msg = strings.ReplaceAll(msg, pgURL, redactURL(pgURL))
		return "", fmt.Errorf("psql: %v: %s", err, msg)
	}
	return out.String(), nil
}

// ReadApproxRows returns {"<schema>.<table>": n_live_tup} from
// pg_stat_user_tables. It must run before the dump starts.
func ReadApproxRows(ctx context.Context, pgURL string) (map[string]int64, error) {
	out, err := approxQuery(ctx, pgURL)
	if err != nil {
		return nil, err
	}
	return parseApproxRows(out)
}

// parseApproxRows parses psql's "name<TAB>count" lines. An empty database
// gives an empty, non-nil map.
func parseApproxRows(out string) (map[string]int64, error) {
	rows := map[string]int64{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		i := strings.LastIndex(line, "\t")
		if i <= 0 {
			return nil, fmt.Errorf("unexpected psql output line %q", line)
		}
		n, err := strconv.ParseInt(strings.TrimSpace(line[i+1:]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("unexpected row count in %q: %w", line, err)
		}
		rows[line[:i]] = n
	}
	return rows, nil
}

// countingReader counts the bytes read through it, so the heartbeat can report
// the size of the (encrypted) object that was uploaded.
type countingReader struct {
	r interface{ Read([]byte) (int, error) }
	n atomic.Int64
}

// Read implements io.Reader.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}
