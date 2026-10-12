package stack

import (
	"fmt"
	"io"
	"os"
	"runtime"
)

// journalFile describes the durable temporary file operations.
// Inputs: encoded journal bytes. Outputs: synced file for atomic rename.
// Constraints: implemented by os.File; tests may inject operation failures.
type journalFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

// journalIO provides local filesystem seams for journal persistence.
// Inputs: temporary file and final paths. Outputs: durable journal changes.
// Constraints: production uses OS operations without mutable global hooks.
type journalIO struct {
	createTemp    func(string, string) (journalFile, error)
	rename        func(string, string) error
	syncDirectory func(string) error
}

// syncStateDirectory makes journal rename metadata durable.
// Inputs: validated state directory. Outputs: sync or close error.
// Constraints: Windows does not support directory fsync and skips this step.
func syncStateDirectory(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open stack update state directory for sync: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync stack update state directory: %w", closeJournal(f, err))
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close stack update state directory: %w", err)
	}
	return nil
}
