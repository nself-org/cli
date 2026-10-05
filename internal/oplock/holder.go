package oplock

// Holder file of the project operation lock (contract:cli.oplock v1).
//
// Purpose: describe who holds the lock, so a refused command can name the
// holder and a child process can prove it descends from the holder.
//
// Inputs: the project directory (read), the holder record (write).
//
// Outputs: `<project>/.nself/op.lock` JSON content while the lock is held:
// {"pid", "command", "started_at" (RFC 3339), "token" (32 hex), "host"}.
//
// Constraints: the file persists after release or a crash. Only the flock
// means "held"; the content of an unlocked file is history and is never read
// as a claim. Only the holder writes it, and only after it owns the flock.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Holder is the JSON record written into the lock file.
type Holder struct {
	PID       int    `json:"pid"`
	Command   string `json:"command"`
	StartedAt string `json:"started_at"`
	Token     string `json:"token"`
	Host      string `json:"host"`
}

// LockPath returns the lock file path of a project directory.
func LockPath(projectDir string) string {
	return filepath.Join(projectDir, ".nself", "op.lock")
}

// ReadHolder parses the holder record of a project. A missing, empty or
// partial file is an error: callers treat it as "holder unknown".
func ReadHolder(projectDir string) (Holder, error) {
	var h Holder
	raw, err := os.ReadFile(LockPath(projectDir))
	if err != nil {
		return h, err
	}
	if err := json.Unmarshal(raw, &h); err != nil {
		return Holder{}, err
	}
	if h.Token == "" {
		return Holder{}, errors.New("oplock: holder file has no token")
	}
	return h, nil
}

// newToken returns 32 random hex characters.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// writeHolder replaces the file content with h. The caller owns the flock.
func writeHolder(f *os.File, h Holder) error {
	raw, err := json.Marshal(h)
	if err != nil {
		return err
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err = f.WriteAt(append(raw, '\n'), 0)
	return err
}
