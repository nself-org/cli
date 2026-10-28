package stack

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

func TestJournalRejectsContradictoryResume(t *testing.T) {
	for _, fault := range []string{"order", "unknown-result", "success-gap", "complete-pending", "missing-run", "run-id-space", "run-id-shell", "run-id-separator"} {
		t.Run(fault, func(t *testing.T) {
			project := t.TempDir()
			j := newJournal(false)
			switch fault {
			case "order":
				j.Steps[0].Name = "migrate"
			case "unknown-result":
				j.Steps[0].Result = "ignored"
			case "success-gap":
				j.Steps[1].Result = "ok"
			case "complete-pending":
				j.Status = "complete"
			case "missing-run":
				j.RunID = ""
			case "run-id-space":
				j.RunID = "1 2"
			case "run-id-shell":
				j.RunID = "1;rm -rf $HOME"
			case "run-id-separator":
				j.RunID = "../1"
			}
			if err := saveJournal(project, j); err != nil {
				t.Fatal(err)
			}
			b := newFixture()
			_, err := Execute(context.Background(), Options{ProjectDir: project, Yes: true}, b)
			assertCode(t, err, "E139")
			if len(b.calls) != 0 {
				t.Fatalf("malformed journal reached backend: %v", b.calls)
			}
		})
	}
}

func TestJournalRejectsTrailingAndUnknownData(t *testing.T) {
	for _, tail := range []string{"{}", ",\"unknown\":true}"} {
		t.Run(tail, func(t *testing.T) {
			project := t.TempDir()
			j := newJournal(false)
			if err := saveJournal(project, j); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(j)
			if err != nil {
				t.Fatal(err)
			}
			if tail[0] == ',' {
				data = append(data[:len(data)-1], tail...)
			} else {
				data = append(data, tail...)
			}
			if err := os.WriteFile(journalPath(project), data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadJournal(project); err == nil {
				t.Fatal("unsafe journal accepted")
			}
		})
	}
}

func TestInterruptedRollbackRequiresOperator(t *testing.T) {
	project := t.TempDir()
	j := newJournal(false)
	j.Status = "rollback-running"
	for i := 0; i < 4; i++ {
		j.Steps[i].Result = "ok"
	}
	j.Steps[4].Result = "failed"
	if err := saveJournal(project, j); err != nil {
		t.Fatal(err)
	}
	b := newFixture()
	_, err := Execute(context.Background(), Options{ProjectDir: project, Yes: true}, b)
	assertCode(t, err, "E139")
	if len(b.calls) != 0 {
		t.Fatalf("interrupted rollback resumed mutation: %v", b.calls)
	}
}

func TestStackJournalEnvironmentCannotBeDowngraded(t *testing.T) {
	project := t.TempDir()
	j := newJournal(true)
	if err := saveJournal(project, j); err != nil {
		t.Fatal(err)
	}
	b := newFixture()
	_, err := Execute(context.Background(), Options{ProjectDir: project, Yes: true}, b)
	assertCode(t, err, "E403")
	if len(b.calls) != 0 {
		t.Fatalf("environment downgrade reached backend: %v", b.calls)
	}
}

func TestStackDryRunLeavesProjectUnchanged(t *testing.T) {
	project := t.TempDir()
	b := newFixture()
	j, err := Execute(context.Background(), Options{ProjectDir: project, Prod: true, DryRun: true}, b)
	if err != nil || j.Status != "planned" || len(j.Steps) != 7 || len(b.calls) != 0 {
		t.Fatalf("invalid preview: %+v %v %v", j, b.calls, err)
	}
	if _, err := os.Stat(filepath.Join(project, ".nself")); !os.IsNotExist(err) {
		t.Fatalf("preview created state: %v", err)
	}
}

func TestRestoreCommandQuotesBackupID(t *testing.T) {
	if got := RestoreCommand("backup'$(touch injected)"); got != "nself backup restore 'backup'\"'\"'$(touch injected)'" {
		t.Fatalf("backup id was not shell-quoted: %s", got)
	}
}

func TestProdResumeFailedVerificationPersistsOperatorState(t *testing.T) {
	project := t.TempDir()
	b := newFixture()
	b.fail = "start"
	o := Options{ProjectDir: project, Prod: true, Yes: true}
	_, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	b.fail, b.calls = "verify", nil
	j, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E403")
	if j.Status != "awaiting-operator" || len(b.calls) != 1 || b.calls[0] != "verify" {
		t.Fatalf("failed verification reached mutation: %+v %v", j, b.calls)
	}
	stored, loadErr := loadJournal(project)
	if loadErr != nil || stored.Status != "awaiting-operator" {
		t.Fatalf("production refusal not persisted: %+v %v", stored, loadErr)
	}
	// M1: a dump this run could not re-verify is never offered for restore.
	fix := cliFix(t, err)
	if strings.Contains(err.Error(), RestoreCommand(j.BackupID)) || strings.Contains(fix, "backup restore") || !strings.Contains(fix, VerifyCommand(j.BackupID)) {
		t.Fatalf("production refusal offered an unverified restore: %s", fix)
	}
}

func TestJournalRejectsRedirectedState(t *testing.T) {
	for _, leaf := range []string{".nself", "state", "update-stack.json"} {
		t.Run(leaf, func(t *testing.T) {
			project, outside := t.TempDir(), t.TempDir()
			link := filepath.Join(project, ".nself")
			if leaf != ".nself" {
				if err := os.Mkdir(link, 0700); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, "state")
			}
			if leaf == "update-stack.json" {
				if err := os.Mkdir(link, 0700); err != nil {
					t.Fatal(err)
				}
				link = filepath.Join(link, leaf)
			}
			if err := os.Symlink(outside, link); err != nil {
				t.Skipf("symlink unavailable on this host: %v", err)
			}
			b := newFixture()
			_, err := Execute(context.Background(), Options{ProjectDir: project, Yes: true}, b)
			assertCode(t, err, "E139")
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 || len(b.calls) != 0 {
				t.Fatalf("redirected state wrote outside project: %v %v %v", entries, b.calls, err)
			}
		})
	}
}

func TestE139DefaultFixIncludesArchiveStep(t *testing.T) {
	e, ok := errs.Registry["E139"]
	if !ok {
		t.Fatal("E139 is not registered")
	}
	for _, want := range []string{"update-stack.json", "archive"} {
		if !strings.Contains(e.DefaultFix, want) {
			t.Errorf("E139 DefaultFix lacks %q: %s", want, e.DefaultFix)
		}
	}
}
