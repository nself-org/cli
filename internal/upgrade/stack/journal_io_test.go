package stack

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type failingJournalFile struct {
	*os.File
	closeErr error
	writeErr error
	syncErr  error
}

func (f failingJournalFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return f.File.Write(p)
}

func (f failingJournalFile) Sync() error {
	if f.syncErr != nil {
		return f.syncErr
	}
	return f.File.Sync()
}

func (f failingJournalFile) Close() error {
	_ = f.File.Close()
	return f.closeErr
}

func TestStackSaveJournalFilesystemFailures(t *testing.T) {
	for _, failure := range []string{"create", "encode", "sync", "close", "rename", "directory-sync"} {
		t.Run(failure, func(t *testing.T) {
			project := t.TempDir()
			original := newJournal(false)
			if err := saveJournal(project, original); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(journalPath(project))
			if err != nil {
				t.Fatal(err)
			}
			fault := errors.New("injected " + failure)
			ops := journalIO{
				createTemp: func(dir, pattern string) (journalFile, error) {
					if failure == "create" {
						return nil, fault
					}
					f, err := os.CreateTemp(dir, pattern)
					if err == nil {
						switch failure {
						case "close":
							return failingJournalFile{File: f, closeErr: fault}, nil
						case "encode":
							return failingJournalFile{File: f, writeErr: fault}, nil
						case "sync":
							return failingJournalFile{File: f, syncErr: fault}, nil
						}
					}
					return f, err
				},
				rename: func(from, to string) error {
					if failure == "rename" {
						return fault
					}
					return os.Rename(from, to)
				},
				syncDirectory: func(string) error { return fault },
			}
			if err := saveJournalWithIO(project, newJournal(false), ops); !errors.Is(err, fault) {
				t.Fatalf("missing wrapped %s failure: %v", failure, err)
			}
			after, err := os.ReadFile(journalPath(project))
			if err != nil {
				t.Fatal(err)
			}
			if failure != "directory-sync" && string(before) != string(after) {
				t.Fatal("failed save replaced existing journal")
			}
			files, err := filepath.Glob(filepath.Join(filepath.Dir(journalPath(project)), ".update-stack-*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("temporary journal leaked: %v %v", files, err)
			}
		})
	}
}

func TestStackSaveJournalSyncsDirectoryAfterRename(t *testing.T) {
	project := t.TempDir()
	var calls []string
	ops := journalIO{
		createTemp: func(dir, pattern string) (journalFile, error) { return os.CreateTemp(dir, pattern) },
		rename: func(from, to string) error {
			calls = append(calls, "rename")
			return os.Rename(from, to)
		},
		syncDirectory: func(dir string) error {
			calls = append(calls, "sync")
			if dir != filepath.Dir(journalPath(project)) {
				t.Fatalf("synced wrong directory: %s", dir)
			}
			if _, err := loadJournal(project); err != nil {
				t.Fatalf("journal not committed before directory sync: %v", err)
			}
			return syncStateDirectory(dir)
		},
	}
	if err := saveJournalWithIO(project, newJournal(false), ops); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(calls, []string{"rename", "sync"}) {
		t.Fatalf("directory sync must follow rename: %v", calls)
	}
}

func TestStackSyncStateDirectoryFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("directory fsync intentionally skipped on Windows")
	}
	if err := syncStateDirectory(filepath.Join(t.TempDir(), "absent")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing wrapped open error: %v", err)
	}
}

func TestStackArchiveJournalPreservesRecovery(t *testing.T) {
	project := t.TempDir()
	j := newJournal(false)
	j.Status = "rolled-back"
	if err := saveJournal(project, j); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(journalPath(project))
	if err != nil {
		t.Fatal(err)
	}
	if err := archiveJournal(project, j); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(filepath.Dir(journalPath(project)), "update-stack."+j.RunID+".json")
	after, err := os.ReadFile(archive)
	if err != nil || string(after) != string(before) {
		t.Fatalf("recovery evidence changed: %v", err)
	}
	if _, err := os.Stat(journalPath(project)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active journal still exists: %v", err)
	}
	if err := saveJournal(project, j); err != nil {
		t.Fatal(err)
	}
	if err := archiveJournal(project, j); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("existing archive was replaced: %v", err)
	}
}

func TestStackArchiveJournalRejectsUnsafeIdentity(t *testing.T) {
	for _, id := range []string{"", ".", "..", "../outside", `..\outside`} {
		j := newJournal(false)
		j.Status, j.RunID = "rolled-back", id
		if err := archiveJournal(t.TempDir(), j); err == nil {
			t.Fatalf("unsafe archive id accepted: %q", id)
		}
	}
}

func TestStackArchiveJournalMissingSource(t *testing.T) {
	project := t.TempDir()
	j := newJournal(false)
	j.Status = "rolled-back"
	if err := archiveJournal(project, j); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing state error not preserved: %v", err)
	}
	if _, err := stateDirectory(project, true); err != nil {
		t.Fatal(err)
	}
	if err := archiveJournal(project, j); !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "archive recovered") {
		t.Fatalf("missing rename source error not preserved: %v", err)
	}
}

func TestStackCloseJournalPreservesPrimaryFailure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "journal-*")
	if err != nil {
		t.Fatal(err)
	}
	primary, secondary := errors.New("write failed"), errors.New("close failed")
	err = closeJournal(failingJournalFile{File: f, closeErr: secondary}, primary)
	if !errors.Is(err, primary) || !errors.Is(err, secondary) {
		t.Fatalf("primary or secondary failure lost: %v", err)
	}
}
