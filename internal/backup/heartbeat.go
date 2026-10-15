package backup

// heartbeat.go — the off-box backup heartbeat (contract:cli.backup-heartbeat v1).
//
// Purpose: after a successful streamed backup, write `<project>/backup.json`
// to a separate heartbeat remote so backup freshness can be checked without
// reaching the box. Also reads the row estimates (`approx_rows`) that a later
// restore drill compares against.
// Inputs: the project config, the heartbeat remote, the finished StreamResult
// and the estimates read just before the dump.
// Outputs: one JSON object uploaded through the destination's Streamer (the
// same destinations grammar as the backup itself); an error when it cannot be
// written.
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
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nself-org/cli/internal/backup/destinations"
	"github.com/nself-org/cli/internal/errs"
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

// WriteHeartbeat uploads hb to <remote>/<project>/backup.json. The remote is
// parsed by the destinations package, so path://, host:// and rclone remotes
// work and option-shaped or credential-carrying values are refused.
func WriteHeartbeat(ctx context.Context, remote string, hb Heartbeat) error {
	if strings.TrimSpace(remote) == "" {
		return fmt.Errorf("heartbeat remote is empty")
	}
	st, _, err := streamerFor(remote)
	if err != nil {
		return fmt.Errorf("%w: heartbeat remote: %v", errs.ErrBackupRemoteFailed, err)
	}
	return writeHeartbeatTo(ctx, st, hb)
}

// writeHeartbeatTo marshals hb and writes <project>/backup.json through st.
func writeHeartbeatTo(ctx context.Context, st destinations.Streamer, hb Heartbeat) error {
	if !heartbeatProjectRe.MatchString(hb.Project) {
		return fmt.Errorf("project name %q cannot be used as a heartbeat key", hb.Project)
	}
	data, err := hb.Marshal()
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	if err := st.PutStream(ctx, bytes.NewReader(data), hb.Project+"/backup.json"); err != nil {
		return fmt.Errorf("%w: %v", errs.ErrBackupRemoteFailed, err)
	}
	return nil
}

// publishHeartbeat writes the heartbeat for a backup that just uploaded to
// remote (parsed here); see publishHeartbeatTo.
func publishHeartbeat(ctx context.Context, cfgProject, remote string, res *StreamResult, approx map[string]int64) error {
	st, _, err := streamerFor(remote)
	if err != nil {
		return fmt.Errorf("%w: heartbeat remote: %v", errs.ErrBackupRemoteFailed, err)
	}
	return publishHeartbeatTo(ctx, cfgProject, st, res, approx)
}

// publishHeartbeatTo is publishHeartbeat with the destination already
// resolved (Stream resolves it before the dump). It refuses to write when
// nothing was uploaded.
func publishHeartbeatTo(ctx context.Context, cfgProject string, st destinations.Streamer, res *StreamResult, approx map[string]int64) error {
	if res == nil || res.Bytes <= 0 {
		return fmt.Errorf("no bytes were uploaded, so no heartbeat is written")
	}
	hb := newBackupHeartbeat(cfgProject, res.BackupID, res.Bytes, res.Encrypted, approx, time.Now(), version.Version)
	if err := writeHeartbeatTo(ctx, st, hb); err != nil {
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
	// The password travels in PGPASSWORD, not in argv, so it is not visible in
	// `ps` on a shared host.
	connURL, password, err := splitPgPassword(pgURL)
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "psql", "-X", "-A", "-t", "-F", "\t",
		"--no-password", "-v", "ON_ERROR_STOP=1", "-c", approxRowsSQL, connURL)
	if password != "" {
		cmd.Env = pgPasswordEnv(password)
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	if err := cmd.Run(); err != nil {
		// The DSN carries the password: never let it reach an error message.
		msg := strings.TrimSpace(errOut.String())
		msg = strings.ReplaceAll(msg, pgURL, redactURL(pgURL))
		if password != "" {
			msg = strings.ReplaceAll(msg, password, "***")
		}
		return "", fmt.Errorf("psql: %v: %s", err, msg)
	}
	return out.String(), nil
}

// splitPgPassword returns the DSN without its password and the password
// itself. It never hands back a DSN that still carries a password: one that
// cannot be parsed, that holds the password in a query parameter (including a
// query string Go cannot parse cleanly, such as a ";" separator or a broken
// escape), or that uses the keyword form ("host=... password=..."), is an
// error, so the secret cannot land in argv. A URL DSN with no password is
// returned unchanged. Error text is fixed and never echoes the DSN.
func splitPgPassword(dsn string) (string, string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", "", fmt.Errorf("the database DSN cannot be parsed, so it is not passed to a child process (its password would be visible in argv)")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		if strings.Contains(strings.ToLower(dsn), "password") {
			return "", "", fmt.Errorf("the database DSN is not a postgres:// URL and names a password, so it is not passed to a child process (its password would be visible in argv)")
		}
		return dsn, "", nil
	}
	// ParseQuery reports the pairs u.Query() silently drops, and those can
	// still hide a password.
	q, err := url.ParseQuery(u.RawQuery)
	if err != nil {
		return "", "", fmt.Errorf("the database DSN has a query string that cannot be parsed, so it is not passed to a child process (a password in it would be visible in argv)")
	}
	for k := range q {
		if strings.EqualFold(k, "password") {
			return "", "", fmt.Errorf("the database DSN carries the password as a query parameter, so it is not passed to a child process (its password would be visible in argv)")
		}
	}
	if u.User == nil {
		return dsn, "", nil
	}
	pw, ok := u.User.Password()
	if !ok {
		return dsn, "", nil
	}
	u.User = url.User(u.User.Username())
	return u.String(), pw, nil
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
