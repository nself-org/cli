// Package updatecheck is the opt-in, once-per-24h "newer nself exists" hint.
//
// Nothing here runs unless NSELF_UPDATE_CHECK=1 (VMI: nothing is sent by
// default). A background goroutine refreshes a small cache from ping's
// GET /version at most once per 24 h; the hint itself reads only the cache and
// never touches the network. Contract: cap:cli.version-nudge.
package updatecheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Cache is the on-disk state at ~/.nself/cache/update-check.json.
//
// CheckedAt is the time of the last refresh attempt (success or failure), so a
// stale cache is visible to `nself doctor`. LastError is empty after a good
// refresh. HintedAt is the time the hint was last printed.
type Cache struct {
	CheckedAt time.Time `json:"checked_at"`
	Latest    string    `json:"latest"`
	LastError string    `json:"last_error,omitempty"`
	HintedAt  time.Time `json:"hinted_at"`
}

// DefaultPath returns ~/.nself/cache/update-check.json, or "" when the home
// directory cannot be resolved (the feature is then off).
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".nself", "cache", "update-check.json")
}

// Load reads the cache. A missing, unreadable or corrupt file yields the zero
// Cache: the next Save replaces it.
func Load(path string) Cache {
	var c Cache
	if path == "" {
		return c
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Cache{}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return Cache{}
	}
	return c
}

// Save writes the cache atomically (temp file in the same directory, then
// rename) with mode 0600 inside a 0700 directory.
func Save(path string, c Cache) error {
	if path == "" {
		return os.ErrInvalid
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".update-check-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
