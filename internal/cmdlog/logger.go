package cmdlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Entry represents a single command execution log record.
type Entry struct {
	Timestamp time.Time `json:"ts"`
	Command   string    `json:"cmd"`  // full argv joined, secrets redacted
	User      string    `json:"user"` // os.Getenv("USER")
	Duration  float64   `json:"dur_s"`
	ExitCode  int       `json:"exit"`
	Error     string    `json:"error,omitempty"`
}

// Logger appends JSON log lines to {logDir}/nself.log with rotation support.
type Logger struct {
	path string
	mu   sync.Mutex
}

// New creates a Logger that writes to {logDir}/nself.log.
// The directory is created if it does not exist.
func New(logDir string) *Logger {
	return &Logger{
		path: filepath.Join(logDir, "nself.log"),
	}
}

// Begin records the command start. The returned function must be deferred by
// the caller; it computes duration and appends the log entry.
// If NSELF_CMD_LOG_ENABLED is "false" or "0", a no-op function is returned.
//
// secrets are values the caller knows are secret (a positional or flag value);
// they join the MaskValues set and are scrubbed from the argv and the error.
func (l *Logger) Begin(argv []string, secrets ...string) func(exitCode int, err error) {
	if !loggingEnabled() {
		return func(int, error) {}
	}

	cmd := redactArgsWith(argv, secrets)
	user := os.Getenv("USER")
	start := time.Now()

	return func(exitCode int, err error) {
		dur := time.Since(start).Seconds()
		entry := Entry{
			Timestamp: start,
			Command:   cmd,
			User:      user,
			Duration:  dur,
			ExitCode:  exitCode,
		}
		if err != nil {
			entry.Error = scrubText(err.Error(), secretSet(secrets))
		}
		// Silently discard errors — logging must never fail a command.
		_ = l.write(entry)
	}
}

// write appends a JSON line to the log file, rotating first if needed.
func (l *Logger) write(entry Entry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logDir := filepath.Dir(l.path)
	// One Stat on the common path. A missing directory is created (the parent
	// keeps its historic 0755, only the logs directory is private). An existing
	// one wider than 0700 is a leftover of an older release: narrow it and its
	// rotated logs once. A directory already at 0700 holds nothing to narrow.
	if info, serr := os.Stat(logDir); serr != nil {
		if err := os.MkdirAll(filepath.Dir(logDir), 0o755); err != nil {
			return fmt.Errorf("cmdlog: mkdir %s: %w", filepath.Dir(logDir), err)
		}
		if err := os.MkdirAll(logDir, 0o700); err != nil {
			return fmt.Errorf("cmdlog: mkdir %s: %w", logDir, err)
		}
	} else if info.Mode().Perm()&0o077 != 0 {
		l.narrowExisting(logDir)
	}

	if err := l.rotateIfNeeded(); err != nil {
		// Non-fatal — still attempt to write.
		_ = err
	}

	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("cmdlog: marshal entry: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cmdlog: open %s: %w", l.path, err)
	}
	defer func() { _ = f.Close() }()
	// A log made before D-0473 may be 0644: narrow it, never widen it.
	if info, serr := f.Stat(); serr == nil && info.Mode().Perm()&^0o600 != 0 {
		_ = f.Chmod(info.Mode().Perm() & 0o600)
	}

	_, err = f.Write(line)
	return err
}

// narrowExisting tightens what an older release left behind: a logs directory
// wider than 0700 and every rotated nself.log.N wider than 0600. The current
// nself.log is narrowed by write.
func (l *Logger) narrowExisting(logDir string) {
	narrowTo(logDir, 0o700)
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}
	prefix := filepath.Base(l.path) + "."
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if n, perr := strconv.Atoi(strings.TrimPrefix(name, prefix)); perr == nil && n > 0 {
			narrowTo(filepath.Join(logDir, name), 0o600)
		}
	}
}

// rotateIfNeeded renames nself.log → nself.log.1 → … → nself.log.N when the
// current file exceeds the configured maximum size.
func (l *Logger) rotateIfNeeded() error {
	maxBytes := int64(maxSizeMB()) * 1024 * 1024
	keepFiles := keepCount()

	info, err := os.Stat(l.path)
	if err != nil {
		// File doesn't exist yet — no rotation needed.
		return nil
	}
	if info.Size() < maxBytes {
		return nil
	}

	// Shift existing rotated files: .N-1 → .N (drop if > keepFiles).
	for i := keepFiles - 1; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", l.path, i)
		dst := fmt.Sprintf("%s.%d", l.path, i+1)

		if i+1 > keepFiles {
			_ = os.Remove(src)
			continue
		}

		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}

	// Rename the current log to .1.
	return os.Rename(l.path, l.path+".1")
}

// ── Secret redaction ─────────────────────────────────────────────────────────

// secretFlags are flag names whose values must be redacted.
var secretFlags = []string{"--password", "--token", "--key", "--secret", "--api-key"}

// redactArgsWith returns the argv joined as a string with secret values
// replaced by [REDACTED]: the flag-name rules run first (the five secretFlags,
// then any --name whose name looks secret, names.go), then URL credentials,
// then every element holding a registered secret value is scrubbed (mask.go).
func redactArgsWith(argv []string, extra []string) string {
	if len(argv) == 0 {
		return ""
	}

	out := make([]string, len(argv))
	copy(out, argv)

	for i, arg := range out {
		// Handle --flag=VALUE form.
		for _, flag := range secretFlags {
			prefix := flag + "="
			if strings.HasPrefix(strings.ToLower(arg), prefix) {
				out[i] = flag + "=[REDACTED]"
				break
			}
		}

		// Handle --flag VALUE form: redact the next arg.
		lower := strings.ToLower(arg)
		for _, flag := range secretFlags {
			if lower == flag && i+1 < len(out) {
				out[i+1] = "[REDACTED]"
				break
			}
		}

		// Any other flag whose name looks secret (--db-password, --auth-token).
		if name, _, ok := strings.Cut(arg, "="); ok {
			if secretFlagName(name) {
				out[i] = name + "=[REDACTED]"
			}
		} else if secretFlagName(arg) && i+1 < len(out) {
			out[i+1] = "[REDACTED]"
		}
	}

	secrets := secretSet(extra)
	for i, arg := range out {
		out[i] = scrubElem(maskURLCredentials(arg), secrets)
	}
	return strings.Join(out, " ")
}

// ── Environment helpers ───────────────────────────────────────────────────────

func loggingEnabled() bool {
	v := os.Getenv("NSELF_CMD_LOG_ENABLED")
	if v == "false" || v == "0" {
		return false
	}
	return true
}

func maxSizeMB() int {
	v := os.Getenv("NSELF_CMD_LOG_MAX_SIZE_MB")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 10
}

func keepCount() int {
	v := os.Getenv("NSELF_CMD_LOG_KEEP_FILES")
	if n, err := strconv.Atoi(v); err == nil && n > 0 {
		return n
	}
	return 5
}
