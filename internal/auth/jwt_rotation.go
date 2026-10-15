package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// randReader is the source of randomness for GenerateJWTKey. It defaults to
// crypto/rand.Reader. Tests may replace it with an error-producing reader to
// exercise the error path.
var randReader io.Reader = rand.Reader

const (
	// DefaultRotationWindowDays is the maximum age of a JWT key before
	// nself doctor --deep reports a warning. Override with
	// NSELF_JWT_ROTATION_WINDOW_DAYS.
	DefaultRotationWindowDays = 90

	// DefaultRotationHardDays is the age at which JWT-ROT-01 escalates from
	// warn to fail. Defaults to 2× the rotation window. Override with
	// NSELF_JWT_ROTATION_HARD_DAYS.
	DefaultRotationHardMultiplier = 2

	// DefaultRotationLogPath is where rotation events are recorded.
	// Falls back to XDG_STATE_HOME (or ~/.local/state) when not writable.
	// Override with NSELF_JWT_ROTATION_LOG.
	DefaultRotationLogPath = "/var/lib/nself/jwt-rotation.log"

	// GracePeriodHours is the window during which the old and new keys are
	// both accepted. This allows running tokens signed by the old key to
	// expire naturally without invalidating active sessions.
	GracePeriodHours = 12

	// jwtKeyBytes is the length of the generated random key in bytes.
	// 32 bytes = 256-bit key, safe for HS256/HS512.
	jwtKeyBytes = 32
)

// RotationLogPath returns the effective rotation log path, honouring the
// NSELF_JWT_ROTATION_LOG environment variable override. When the primary path
// is not writable (e.g. /var/lib/nself/ on a user workstation), it falls back
// to ${XDG_STATE_HOME:-$HOME/.local/state}/nself/jwt-rotation.log.
func RotationLogPath() string {
	if v := os.Getenv("NSELF_JWT_ROTATION_LOG"); v != "" {
		return v
	}
	// Test primary path writability by attempting to stat the directory.
	primaryDir := filepath.Dir(DefaultRotationLogPath)
	if info, err := os.Stat(primaryDir); err == nil && info.IsDir() {
		// Directory exists; check write permission via O_WRONLY probe.
		probe := filepath.Join(primaryDir, ".nself-write-probe")
		if f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
			_ = f.Close()
			_ = os.Remove(probe)
			return DefaultRotationLogPath
		}
	}
	// Fall back to XDG_STATE_HOME path.
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		home, _ := os.UserHomeDir()
		stateHome = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(stateHome, "nself", "jwt-rotation.log")
}

// RotationWindowDays returns the effective rotation window, honouring the
// NSELF_JWT_ROTATION_WINDOW_DAYS environment variable override. Returns an
// error on malformed input.
func RotationWindowDays() int {
	if v := os.Getenv("NSELF_JWT_ROTATION_WINDOW_DAYS"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "nself: NSELF_JWT_ROTATION_WINDOW_DAYS=%q is not a valid positive integer, using default %d\n", v, DefaultRotationWindowDays)
			return DefaultRotationWindowDays
		}
		return n
	}
	return DefaultRotationWindowDays
}

// RotationHardDays returns the age at which JWT-ROT-01 escalates to fail
// (not just warn). Defaults to 2× RotationWindowDays. Override with
// NSELF_JWT_ROTATION_HARD_DAYS.
func RotationHardDays() int {
	if v := os.Getenv("NSELF_JWT_ROTATION_HARD_DAYS"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n <= 0 {
			fmt.Fprintf(os.Stderr, "nself: NSELF_JWT_ROTATION_HARD_DAYS=%q is not a valid positive integer, using default\n", v)
			return RotationWindowDays() * DefaultRotationHardMultiplier
		}
		return n
	}
	return RotationWindowDays() * DefaultRotationHardMultiplier
}

// LastRotationTime reads the rotation log and returns the timestamp of the
// most recent successful rotation. Returns the zero time.Time if no rotation
// has ever been recorded or if the log cannot be read.
func LastRotationTime(logPath string) (time.Time, error) {
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return time.Time{}, nil
		}
		return time.Time{}, fmt.Errorf("read rotation log %s: %w", logPath, err)
	}

	var latest time.Time
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Each log line begins with an RFC3339 timestamp followed by a space
		// and the event description, e.g.:
		//   2026-04-30T14:22:00Z rotated HASURA_GRAPHQL_JWT_SECRET ...
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		t, err := time.Parse(time.RFC3339, fields[0])
		if err != nil {
			continue
		}
		// Reject future timestamps (clock skew guard).
		if t.After(time.Now().UTC().Add(time.Minute)) {
			continue
		}
		if t.After(latest) {
			latest = t
		}
	}
	return latest, nil
}

// GenerateJWTKey returns a securely-random hex-encoded key suitable for use
// as the HASURA_GRAPHQL_JWT_SECRET key material (HS256/HS512).
func GenerateJWTKey() (string, error) {
	buf := make([]byte, jwtKeyBytes)
	if _, err := io.ReadFull(randReader, buf); err != nil {
		return "", fmt.Errorf("generate jwt key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// RotateResult holds the outcome of a rotation attempt.
//
// OldKey always holds the raw hex key string that was previously in use (not
// the JSON envelope). The env var HASURA_GRAPHQL_JWT_SECRET contains that key
// wrapped in a JSON envelope: {"type":"HS256","key":"<OldKey>"}. Callers that
// need the envelope format must construct it themselves.
type RotateResult struct {
	NewKey     string
	OldKey     string // raw hex key, NOT the JSON envelope
	GraceUntil time.Time
	LogEntry   string
}

// RotateJWTKey generates a new JWT key, writes a rotation log entry, and
// returns the result. It does NOT modify any env file — callers are responsible
// for writing the new key into .env.secrets and reloading Hasura.
//
// currentKey must be the raw hex key (not the JSON envelope). If currentKey is
// empty, RotateJWTKey returns an error rather than silently storing an empty
// OldKey — an empty old key most likely indicates the caller did not read the
// current value before rotating.
//
// The dual-key grace period is signalled via RotateResult.GraceUntil.
// During [now, GraceUntil] both the old key and the new key are valid for
// verifying tokens. After GraceUntil, only the new key should be used.
//
// This function NEVER reads from or writes to production secrets files — it
// only generates the key material and records the event in the rotation log.
func RotateJWTKey(currentKey string) (*RotateResult, error) {
	return RotateJWTKeyStaged(currentKey, nil)
}

// RotateJWTKeyStaged generates a key and calls stage before recording rotation.
// Inputs: the current key and an optional durable-staging callback.
// Outputs: rotation metadata or an error; a staging failure never updates the log.
// Constraints: stage must not publish the key until the caller receives success.
func RotateJWTKeyStaged(currentKey string, stage func(string) error) (*RotateResult, error) {
	if currentKey == "" {
		return nil, fmt.Errorf("RotateJWTKey: currentKey is empty — read HASURA_GRAPHQL_JWT_SECRET before rotating")
	}

	newKey, err := GenerateJWTKey()
	if err != nil {
		return nil, err
	}

	if stage != nil {
		if err := stage(newKey); err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC()
	graceUntil := now.Add(GracePeriodHours * time.Hour)

	logEntry := fmt.Sprintf("%s rotated HASURA_GRAPHQL_JWT_SECRET (grace period ends %s)\n",
		now.Format(time.RFC3339), graceUntil.Format(time.RFC3339))

	if err := writeRotationLog(logEntry); err != nil {
		return nil, fmt.Errorf("write rotation log: %w", err)
	}

	return &RotateResult{
		NewKey:     newKey,
		OldKey:     currentKey, // raw hex key, not the JSON envelope
		GraceUntil: graceUntil,
		LogEntry:   strings.TrimSpace(logEntry),
	}, nil
}

// writeRotationLog appends a single log entry to the rotation log file using
// an exclusive flock(2) to prevent torn writes from concurrent rotations.
// Creates the log file and any required parent directories if they do not exist.
func writeRotationLog(entry string) error {
	logPath := RotationLogPath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return fmt.Errorf("create rotation log dir: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open rotation log: %w", err)
	}
	defer func() { _ = f.Close() }()

	// Acquire an exclusive advisory lock before writing so concurrent calls
	// cannot produce torn (interleaved) log lines. lockExclusive is a per-OS
	// helper: flock(2) on Unix, no-op on Windows (where O_APPEND is atomic for
	// writes <= PIPE_BUF and cross-process concurrent CLI use is the rare path).
	unlock, err := lockExclusive(f)
	if err != nil {
		return fmt.Errorf("lock rotation log: %w", err)
	}
	defer unlock()

	if _, err := f.WriteString(entry); err != nil {
		return fmt.Errorf("append rotation log: %w", err)
	}
	return nil
}
