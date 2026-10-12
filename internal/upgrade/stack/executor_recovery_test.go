package stack

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

type recoveryFaultBackend struct {
	*fixtureBackend
	recoveryFault string
}

func (b *recoveryFaultBackend) Restore(ctx context.Context, j *Journal) error {
	if b.recoveryFault == "restore" {
		return errors.New("injected restore failure")
	}
	return b.fixtureBackend.Restore(ctx, j)
}

func (b *recoveryFaultBackend) RestartPrevious(ctx context.Context, j *Journal) error {
	if b.recoveryFault == "restart-previous" {
		return errors.New("injected restart failure")
	}
	return b.fixtureBackend.RestartPrevious(ctx, j)
}

func cliFix(t *testing.T, err error) string {
	t.Helper()
	var e *errs.CLIError
	if !errors.As(err, &e) {
		t.Fatalf("missing coded error: %v", err)
	}
	return e.Fix
}

func TestStackUpdateUnverifiedBackupNeverSuggestsRestore(t *testing.T) {
	for _, fault := range []string{"verify", "missing-proof", "old-backup"} {
		t.Run(fault, func(t *testing.T) {
			b := newFixture()
			b.fail, b.noProof, b.oldBackup = fault, fault == "missing-proof", fault == "old-backup"
			j, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}, b)
			assertCode(t, err, "E403")
			// S3: the unusable evidence is cleared (the file stays) and named.
			if j.BackupID != "" || j.PreviousCompose != "" || j.Steps[0].Result != "failed" || strings.Contains(cliFix(t, err), "backup restore") || !strings.Contains(cliFix(t, err), "backup verify 'step-one-backup'") {
				t.Fatalf("unverified dump suggested for restore or evidence kept: %+v %v", j, err)
			}
		})
	}
}

func TestStackUpdateResumeVerifyFailureNeverSuggestsRestore(t *testing.T) {
	for _, result := range []string{"failed", "running"} {
		t.Run(result, func(t *testing.T) {
			b := newFixture()
			b.fail = "pull"
			o := Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}
			if _, err := Execute(context.Background(), o, b); err == nil {
				t.Fatal("expected pull failure")
			}
			stored, err := loadJournal(o.ProjectDir)
			if err != nil {
				t.Fatal(err)
			}
			stored.Steps[1].Result = result
			if err := saveJournal(o.ProjectDir, stored); err != nil {
				t.Fatal(err)
			}
			b.fail = "verify"
			j, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E403")
			// M1: this run could not re-verify the dump, so restore is never offered.
			fix := cliFix(t, err)
			if j.Steps[0].Result != "ok" || strings.Contains(fix, "backup restore") || !strings.Contains(fix, "nself backup verify 'step-one-backup'") || !strings.Contains(fix, "no longer be trusted") {
				t.Fatalf("resume guidance offers an unverified restore: %+v %q", j, fix)
			}
		})
	}
}

func TestStackUpdateFailedRecoverySuggestsStepOneRestore(t *testing.T) {
	for _, fault := range []string{"restore", "restart-previous"} {
		t.Run(fault, func(t *testing.T) {
			b := &recoveryFaultBackend{fixtureBackend: newFixture(), recoveryFault: fault}
			b.fail = "migrate"
			j, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Yes: true}, b)
			assertCode(t, err, "E139")
			fix := cliFix(t, err)
			if j.Status != "rollback-failed" || !strings.Contains(fix, RestoreCommand("step-one-backup")) || !strings.Contains(fix, RestartCommand("images=N")) || !strings.Contains(fix, "images=N") {
				t.Fatalf("missing step-one recovery guidance: %+v %v", j, err)
			}
		})
	}
}

func TestStackUpdateRecoveredJournalArchived(t *testing.T) {
	b := newFixture()
	b.fail = "migrate"
	o := Options{ProjectDir: t.TempDir(), Yes: true}
	old, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	before, err := os.ReadFile(journalPath(o.ProjectDir))
	if err != nil {
		t.Fatal(err)
	}
	b.fail = ""
	j, err := Execute(context.Background(), o, b)
	if err != nil || j.Status != "complete" || j.RunID == old.RunID {
		t.Fatalf("recovered run blocked: %+v %v", j, err)
	}
	archived, err := os.ReadFile(filepath.Join(o.ProjectDir, ".nself", "state", "update-stack."+old.RunID+".json"))
	if err != nil || string(archived) != string(before) {
		t.Fatalf("old journal was not retained unchanged: %v", err)
	}
}

func TestStackUpdateIncompleteRecoveryBlocked(t *testing.T) {
	for _, status := range []string{"rollback-running", "rollback-failed"} {
		t.Run(status, func(t *testing.T) {
			o := Options{ProjectDir: t.TempDir(), Yes: true}
			j := newJournal(false)
			j.Status = status
			if err := saveJournal(o.ProjectDir, j); err != nil {
				t.Fatal(err)
			}
			b := newFixture()
			_, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E139")
			fix := cliFix(t, err)
			if len(b.calls) != 0 || !strings.Contains(fix, "previous images") || !strings.Contains(fix, "row counts") || !strings.Contains(fix, "step-one backup") {
				t.Fatalf("unsafe recovery resume or incomplete guidance: %v %v", b.calls, err)
			}
		})
	}
}

func TestStackUpdateDryRunExistingJournal(t *testing.T) {
	for _, status := range []string{"running", "rolled-back", "rollback-running", "rollback-failed", "complete"} {
		t.Run(status, func(t *testing.T) {
			o := Options{ProjectDir: t.TempDir(), DryRun: true}
			j := newJournal(false)
			j.Status = status
			j.Steps[0].Result = "ok"
			wantResume := "pull"
			switch status {
			case "rolled-back", "complete":
				wantResume = "backup"
			case "rollback-running", "rollback-failed":
				wantResume = ""
			}
			if status == "complete" {
				for i := range j.Steps {
					j.Steps[i].Result = "ok"
				}
			}
			if err := saveJournal(o.ProjectDir, j); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(journalPath(o.ProjectDir))
			if err != nil {
				t.Fatal(err)
			}
			b := newFixture()
			got, err := Execute(context.Background(), o, b)
			if err != nil || !reflect.DeepEqual(got, j) || ResumeStep(got) != wantResume || len(b.calls) != 0 {
				t.Fatalf("preview differs from durable state: %+v %v %v", got, b.calls, err)
			}
			after, err := os.ReadFile(journalPath(o.ProjectDir))
			if err != nil || string(after) != string(before) {
				t.Fatalf("preview mutated journal: %v", err)
			}
			files, err := os.ReadDir(filepath.Dir(journalPath(o.ProjectDir)))
			if err != nil || len(files) != 1 {
				t.Fatalf("preview archived or created files: %v %v", files, err)
			}
		})
	}
}

func TestStackUpdateExecuteCorruptJournal(t *testing.T) {
	for _, dry := range []bool{false, true} {
		t.Run(map[bool]string{false: "execute", true: "preview"}[dry], func(t *testing.T) {
			o := Options{ProjectDir: t.TempDir(), Yes: true, DryRun: dry}
			if _, err := stateDirectory(o.ProjectDir, true); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(journalPath(o.ProjectDir), []byte("{invalid"), 0600); err != nil {
				t.Fatal(err)
			}
			b := newFixture()
			j, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E139")
			if j != nil || len(b.calls) != 0 {
				t.Fatalf("corrupt journal allowed work: %+v %v", j, b.calls)
			}
		})
	}
}
