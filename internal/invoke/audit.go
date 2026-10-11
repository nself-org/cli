package invoke

// Purpose: the machine audit log of contract:cli.machine-audit v1: one compact
// JSON line per mutation and per refusal, appended to a private file.
// Inputs: the Guard audit path, a finished invocation (trace, request, result).
// Outputs: ~/.nself/logs/machine-audit.jsonl (DefaultAuditPath) with keys in
// the contract order: time, transport, project, command, side_effect,
// decision, confirm_kind, request_id, request, exit_code, error_code,
// duration_ms.
// Constraints: dir 0700 (only dirs this code created are chmod-ed), file 0600,
// O_APPEND with flock LOCK_EX on the open file; after locking the path must
// still be the same file (else another process rotated: reopen). At
// auditMaxBytes the file rotates to .1 .. .3 (older dropped). Registry secret
// args and flags, flag values whose name matches config.IsSecretKey and all
// free-form argv items are replaced by "[REDACTED]", then every string goes
// through observability.Redact. Nonces are never logged. Reads that run are
// not logged. A failed write is reported on stderr without the request and
// never stops the invocation.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/observability"
)

// auditMaxBytes is the size at which the audit file rotates. A variable so a
// test can lower it; production keeps the 10 MiB of the contract.
var auditMaxBytes int64 = 10 << 20

// auditKeep is the number of rotated files kept (.1 to .3).
const auditKeep = 3

// auditFileMu serialises every audit write of this process, across Guards:
// the file lock covers other processes, and on Windows (no flock) it is the
// only thing keeping two in-process writers from racing a rotation.
var auditFileMu sync.Mutex

// auditErrOut receives the one-line notice of a failed audit write.
var auditErrOut io.Writer = os.Stderr

// DefaultAuditPath is ~/.nself/logs/machine-audit.jsonl.
func DefaultAuditPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nself", "logs", "machine-audit.jsonl"), nil
}

// trace is what Invoke learned about one invocation, for the audit line.
type trace struct {
	command     string
	side        string
	decision    string
	confirmKind string
	cmd         *cmdregistry.Command
}

// auditRecord is one line; field order is the contract's.
type auditRecord struct {
	Time        string       `json:"time"`
	Transport   string       `json:"transport"`
	Project     string       `json:"project"`
	Command     string       `json:"command"`
	SideEffect  string       `json:"side_effect"`
	Decision    string       `json:"decision"`
	ConfirmKind *string      `json:"confirm_kind"`
	RequestID   string       `json:"request_id"`
	Request     auditRequest `json:"request"`
	ExitCode    *int         `json:"exit_code"`
	ErrorCode   *string      `json:"error_code"`
	DurationMS  int64        `json:"duration_ms"`
}

type auditRequest struct {
	Args  []string       `json:"args"`
	Argv  []string       `json:"argv"`
	Flags map[string]any `json:"flags"`
}

// surfaceOf is the audit transport label for o.
func surfaceOf(o Options) string {
	switch {
	case o.Surface != "":
		return o.Surface
	case transportOf(o) == TransportMCP:
		return "mcp-stdio"
	}
	return "http"
}

// audit appends the line for one invocation. A nil Guard or an empty audit
// path writes nothing, and so does a read that ran.
func (g *Guard) audit(o Options, tr *trace, req Request, res Result, runErr error, start time.Time) {
	if g == nil || g.auditPath == "" {
		return
	}
	if tr.decision == DecisionRun && tr.side == canon.SideEffectRead {
		return
	}
	end := g.now()
	rec := auditRecord{
		Time: end.UTC().Format("2006-01-02T15:04:05.000Z"), Transport: surfaceOf(o), Project: projectDir(o.Dir),
		Command: observability.Redact(tr.command), SideEffect: tr.side, Decision: tr.decision,
		RequestID: res.RequestID, Request: auditRequestOf(tr.cmd, req), DurationMS: end.Sub(start).Milliseconds(),
	}
	if tr.confirmKind != "" {
		k := tr.confirmKind
		rec.ConfirmKind = &k
	}
	if runErr == nil && tr.decision != DecisionConfirm {
		if res.ErrorCode != "" {
			c := res.ErrorCode
			rec.ErrorCode = &c
		} else {
			x := res.ExitCode
			rec.ExitCode = &x
			rec.ErrorCode = childErrorCode(res.Stdout)
		}
	}
	var line bytes.Buffer
	enc := json.NewEncoder(&line)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		_, _ = fmt.Fprintln(auditErrOut, "nself: machine audit line could not be encoded")
		return
	}
	if err := g.appendLine(line.Bytes()); err != nil {
		_, _ = fmt.Fprintf(auditErrOut, "nself: machine audit write failed: %v\n", err)
	}
}

func projectDir(dir string) string {
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// auditRequestOf redacts req for the log.
func auditRequestOf(cmd *cmdregistry.Command, req Request) auditRequest {
	red := RedactRequest(cmd, req)
	out := auditRequest{Args: []string{}, Argv: []string{}, Flags: map[string]any{}}
	for _, a := range red.Args {
		out.Args = append(out.Args, observability.Redact(a))
	}
	for _, a := range red.Argv {
		out.Argv = append(out.Argv, observability.Redact(a))
	}
	for name, v := range red.Flags {
		key := observability.Redact(name)
		if config.IsSecretKey(name) {
			out.Flags[key] = Redacted
			continue
		}
		out.Flags[key] = redactValue(v)
	}
	return out
}

// redactValue runs every string inside a flag value through observability.Redact.
func redactValue(v any) any {
	switch x := v.(type) {
	case string:
		return observability.Redact(x)
	case []string:
		out := make([]string, len(x))
		for i, s := range x {
			out[i] = observability.Redact(s)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redactValue(e)
		}
		return out
	}
	return v
}

// childErrorCode returns error.code of a child error envelope, else nil.
func childErrorCode(stdout []byte) *string {
	var doc struct {
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(stdout), &doc) != nil || doc.Error == nil || doc.Error.Code == "" {
		return nil
	}
	return &doc.Error.Code
}

// appendLine writes one line under the in-process mutex and the file lock.
func (g *Guard) appendLine(line []byte) error {
	auditFileMu.Lock()
	defer auditFileMu.Unlock()
	if err := ensureAuditDir(filepath.Dir(g.auditPath)); err != nil {
		return err
	}
	for attempt := 0; attempt < 8; attempt++ {
		f, err := os.OpenFile(g.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if err := lockFile(f); err != nil {
			_ = f.Close()
			return err
		}
		fi, err := f.Stat()
		if err != nil {
			_ = f.Close()
			return err
		}
		if cur, err := os.Stat(g.auditPath); err != nil || !os.SameFile(cur, fi) {
			_ = f.Close() // another process rotated it between open and lock
			continue
		}
		if fi.Mode().Perm()&0o077 != 0 {
			_ = f.Chmod(0o600)
		}
		if fi.Size() >= auditMaxBytes {
			rerr := rotateAudit(g.auditPath, f)
			if !closeBeforeRotate {
				_ = f.Close()
			}
			if rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
				return rerr // ErrNotExist: another writer rotated first (no flock on Windows); reopen
			}
			continue
		}
		_, err = f.Write(line)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
	return errors.New("the audit file kept changing under the lock")
}

// ensureAuditDir creates dir at 0700 and narrows an existing dir to 0700 on
// unix (group and other bits removed, never widened: the command log creates
// ~/.nself/logs at 0755 first). A failed narrowing is not fatal: the file
// itself is 0600.
func ensureAuditDir(dir string) error {
	var made []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || filepath.Dir(d) == d {
			break
		}
		made = append(made, d)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	for _, d := range made {
		if err := os.Chmod(d, 0o700); err != nil {
			return err
		}
	}
	if st, err := os.Stat(dir); err == nil && runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(dir, st.Mode().Perm()&^0o077)
	}
	return nil
}

// rotateAudit shifts path to .1 (and .1 to .2, .2 to .3), dropping the oldest.
// f is the open, locked file; platforms that cannot rename an open file close
// it first (closeBeforeRotate).
func rotateAudit(path string, f *os.File) error {
	if closeBeforeRotate {
		_ = f.Close()
	}
	_ = os.Remove(fmt.Sprintf("%s.%d", path, auditKeep))
	for i := auditKeep - 1; i >= 1; i-- {
		if err := os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return os.Rename(path, path+".1")
}
