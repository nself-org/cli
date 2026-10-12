// atomic.go — the durable file writer shared by the ledger and the remove intent.
//
// Purpose: one temp + fsync + rename + directory-sync routine for every file
// the store keeps in <pluginDir>/.nself-state.
// Inputs: the target path, the temp-file pattern and the bytes to write.
// Outputs: the target replaced atomically; a reader sees the old or the new
// file, never a partial one.
// Constraints: the caller holds the writer lock. Files are 0644 (no secrets).
// Every temp pattern must be listed in tempPatterns so stale ones are swept.
package ledger

import (
	"fmt"
	"os"
)

// tempPatterns lists the temp-file globs removeStaleTemps sweeps.
var tempPatterns = []string{ledgerTempPattern, intentTempPattern}

// writeAtomic replaces path with data. pattern is the os.CreateTemp pattern
// for the temp file, created in the same directory as path.
func (s *Store) writeAtomic(path, pattern string, data []byte) error {
	if s.writeHook != nil {
		if err := s.writeHook(path); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(s.Dir(), pattern)
	if err != nil {
		return fmt.Errorf("ledger: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: write temp: %w", err)
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: chmod temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("ledger: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("ledger: close temp: %w", err)
	}
	if s.beforeRename != nil {
		if err := s.beforeRename(); err != nil {
			cleanup()
			return err
		}
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("ledger: replace %s: %w", path, err)
	}
	if err := syncDir(s.Dir()); err != nil {
		return fmt.Errorf("ledger: %s was replaced but its directory could not be flushed, so the change may not survive a crash: %w", path, err)
	}
	return nil
}
