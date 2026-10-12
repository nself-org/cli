package stack

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// archiveJournal preserves recovered run evidence before a fresh update.
// Inputs: project and completed recovery journal. Outputs: atomic archive.
// Constraints: caller holds the project lock; existing archives are never replaced.
func archiveJournal(project string, j *Journal) error {
	if j.Status != "rolled-back" || j.RunID == "" || j.RunID == "." || j.RunID == ".." || strings.ContainsAny(j.RunID, `/\`) {
		return fmt.Errorf("stack update recovery journal has unsafe archive identity")
	}
	dir, err := stateDirectory(project, false)
	if err != nil {
		return err
	}
	archive := filepath.Join(dir, "update-stack."+j.RunID+".json")
	if _, err := os.Lstat(archive); err == nil {
		return fmt.Errorf("stack update journal archive already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect stack update journal archive: %w", err)
	}
	if err := os.Rename(journalPath(project), archive); err != nil {
		return fmt.Errorf("archive recovered stack update journal: %w", err)
	}
	return syncStateDirectory(dir)
}
