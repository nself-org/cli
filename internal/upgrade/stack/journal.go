package stack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// journalPath identifies machine-owned project state.
// Inputs: project directory. Outputs: absolute or project-relative state path.
// Constraints: callers hold the project operation lock.
func journalPath(project string) string {
	return filepath.Join(project, ".nself", "state", "update-stack.json")
}

// loadJournal reads durable state without reconstructing missing evidence.
// Inputs: project directory. Outputs: validated journal or nil for a new run.
// Constraints: malformed state fails closed; only absence starts a fresh run.
func loadJournal(project string) (*Journal, error) {
	if _, err := stateDirectory(project, false); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	info, err := os.Lstat(journalPath(project))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect stack update journal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("stack update journal must be a regular file")
	}
	if info.Size() > 4<<20 {
		return nil, fmt.Errorf("stack update journal exceeds the size limit")
	}
	data, err := os.ReadFile(journalPath(project))
	if err != nil {
		return nil, fmt.Errorf("read stack update journal: %w", err)
	}
	if len(data) > 4<<20 {
		return nil, fmt.Errorf("stack update journal exceeds the size limit")
	}
	var j Journal
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&j); err != nil {
		return nil, fmt.Errorf("decode stack update journal: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("stack update journal has trailing data: %v", err)
	}
	if err := validateJournal(&j); err != nil {
		return nil, err
	}
	return &j, nil
}

// validateJournal rejects contradictory states instead of guessing a resume point.
// Inputs: decoded journal. Outputs: structural validation error.
// Constraints: successful steps are a contiguous prefix in the shared order.
func validateJournal(j *Journal) error {
	steps := ExecutionPlan()
	if !runtimeRunID.MatchString(j.RunID) || j.StartedAt.IsZero() || len(j.Steps) != len(steps) {
		return fmt.Errorf("stack update journal lacks run identity or ordered steps")
	}
	switch j.Status {
	case "running", "failed", "awaiting-operator", "rollback-running", "rolled-back", "rollback-failed", "complete":
	default:
		return fmt.Errorf("stack update journal has invalid status %q", j.Status)
	}
	pending := false
	for i, s := range j.Steps {
		if s.Name != steps[i].Name || s.DurationMS < 0 {
			return fmt.Errorf("stack update journal has invalid step %d", i)
		}
		switch s.Result {
		case "ok":
			if pending {
				return fmt.Errorf("stack update journal has noncontiguous successful steps")
			}
		case "pending", "running", "failed":
			pending = true
		default:
			return fmt.Errorf("stack update journal has invalid result %q", s.Result)
		}
	}
	if j.Status == "complete" && pending {
		return fmt.Errorf("completed stack update journal has unfinished steps")
	}
	return nil
}

// saveJournal commits state by syncing a private temporary file then renaming it.
// Inputs: project directory and execution state. Outputs: durable atomic journal.
// Constraints: temporary files contain no secrets and are removed on errors.
func saveJournal(project string, j *Journal) error {
	return saveJournalWithIO(project, j, journalIO{
		createTemp: func(dir, pattern string) (journalFile, error) { return os.CreateTemp(dir, pattern) },
		rename:     os.Rename, syncDirectory: syncStateDirectory,
	})
}

// saveJournalWithIO exposes filesystem failure seams without global hooks.
// Inputs: project, execution state and file operations. Outputs: atomic journal.
// Constraints: failed writes leave the previous journal intact.
func saveJournalWithIO(project string, j *Journal, ops journalIO) error {
	path := journalPath(project)
	if _, err := stateDirectory(project, true); err != nil {
		return err
	}
	f, err := ops.createTemp(filepath.Dir(path), ".update-stack-*")
	if err != nil {
		return fmt.Errorf("create stack update journal temporary file: %w", err)
	}
	name := f.Name()
	// Best-effort cleanup of the temp file: after a successful rename it is already gone,
	// and after a failure the write error is the one worth reporting.
	defer func() { _ = os.Remove(name) }()
	if err := json.NewEncoder(f).Encode(j); err != nil {
		return fmt.Errorf("encode stack update journal: %w", closeJournal(f, err))
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync stack update journal: %w", closeJournal(f, err))
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close stack update journal: %w", err)
	}
	if err := ops.rename(name, path); err != nil {
		return fmt.Errorf("replace stack update journal: %w", err)
	}
	return ops.syncDirectory(filepath.Dir(path))
}

// stateDirectory validates machine-owned ancestors before any state access.
// Inputs: project and creation policy. Outputs: real private state directory.
// Constraints: symlinks never redirect journal writes outside the project.
func stateDirectory(project string, create bool) (string, error) {
	p := project
	for _, name := range []string{".nself", "state"} {
		p = filepath.Join(p, name)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) && create {
			if err := os.Mkdir(p, 0700); err != nil {
				return "", fmt.Errorf("create stack update state directory: %w", err)
			}
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect stack update state directory: %w", err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("stack update state must use real directories")
		}
	}
	return p, nil
}

// closeJournal preserves a write error and reports a secondary close failure.
// Inputs: open file and primary error. Outputs: wrapped operation error.
// Constraints: failures are never silently replaced by a successful close.
func closeJournal(f journalFile, primary error) error {
	if err := f.Close(); err != nil {
		return errors.Join(primary, fmt.Errorf("close temporary journal: %w", err))
	}
	return primary
}

// newJournal creates the ordered initial state without touching the project.
// Inputs: production policy. Outputs: unique run identity and pending steps.
// Constraints: backup evidence is initially empty.
func newJournal(prod bool) *Journal {
	now := time.Now().UTC()
	j := &Journal{RunID: fmt.Sprintf("%d", now.UnixNano()), StartedAt: now, Status: "running", Prod: prod}
	for _, s := range ExecutionPlan() {
		j.Steps = append(j.Steps, Step{Name: s.Name, Result: "pending"})
	}
	return j
}
