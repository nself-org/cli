package imageswitch

// assets.go — the scripts and SQL the one-shots run, and the stage directory.
//
// Purpose: the dump, the recheck and the restore are shell and SQL run inside
// the target image; they are embedded so one binary carries them.
// Inputs: the stage directory. Outputs: the files written 0600 into it.
// Constraints: the stage is 0700 and its files 0600; only these assets are
// ever written there from the binary.

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed assets/*.sql assets/*.sh
var assetFS embed.FS

// Asset returns an embedded script or SQL file.
func Asset(name string) (string, error) {
	b, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		return "", fmt.Errorf("image switch asset %s: %w", name, err)
	}
	return string(b), nil
}

// stageAssets are written to the stage before a one-shot that needs them.
var stageAssets = []string{"common.sh", "capcluster.sql", "capdb.sql", "dump.sh", "recheck.sh", "restore.sh"}

// prepareStage creates the 0700 stage and writes the assets into it.
func (s *Switch) prepareStage() error {
	if err := os.MkdirAll(s.stage, 0o700); err != nil {
		return fmt.Errorf("creating the stage directory: %w", err)
	}
	if err := os.Chmod(s.stage, 0o700); err != nil {
		return err
	}
	for _, name := range stageAssets {
		body, err := Asset(name)
		if err != nil {
			return err
		}
		if err := writeStage(s.stage, name, []byte(body)); err != nil {
			return err
		}
	}
	return nil
}

// writeStage writes one stage file at 0600 (replacing a root-owned leftover is
// not possible, so a failed replace is reported).
func writeStage(stage, name string, data []byte) error {
	p := filepath.Join(stage, name)
	_ = os.Remove(p)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", p, err)
	}
	return nil
}

// readStage reads one stage file.
func (s *Switch) readStage(name string) (string, error) {
	b, err := os.ReadFile(filepath.Join(s.stage, name))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", name, err)
	}
	return string(b), nil
}

// cleanStage empties the stage directory but keeps the assets' directory.
func (s *Switch) cleanStage() error {
	if err := os.RemoveAll(s.stage); err != nil {
		return fmt.Errorf("removing the stage directory %s: %w", s.stage, err)
	}
	return nil
}

// dbFile names the per-database stage file for database index i.
func dbFile(i int, kind string) string { return fmt.Sprintf("db-%d.%s", i, kind) }

// dblist is the text of dblist.txt: one database per line.
func dblist(dbs []string) string { return strings.Join(dbs, "\n") + "\n" }
