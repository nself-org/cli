package stack

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestStackUpdateNonProdResumeInvalidBackupBeforeStart(t *testing.T) {
	for _, step := range []string{"pull", "build"} {
		t.Run(step, func(t *testing.T) {
			b := newFixture()
			b.fail = step
			o := Options{ProjectDir: t.TempDir(), Yes: true}
			_, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E139")
			b.fail, b.calls = "verify", nil
			j, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E139")
			if !reflect.DeepEqual(b.calls, []string{"verify"}) {
				t.Fatalf("invalid backup allowed operations: %v", b.calls)
			}
			stored, loadErr := loadJournal(o.ProjectDir)
			if loadErr != nil || !reflect.DeepEqual(stored, j) {
				t.Fatalf("reset was not persisted: %+v %v", stored, loadErr)
			}
			if j.BackupID != "" || j.BackupSHA256 != "" || !j.BackupCreatedAt.IsZero() || !j.BackupVerifiedAt.IsZero() || j.PreviousCompose != "" || j.PreviousComposeSHA256 != "" || j.BeforeRows != nil || ResumeStep(j) != "backup" {
				t.Fatalf("stale recovery evidence retained: %+v", j)
			}
			fix := cliFix(t, err)
			if !strings.Contains(fix, "fresh dump") || !strings.Contains(fix, VerifyCommand("step-one-backup")) || strings.Contains(fix, "backup restore") {
				t.Fatalf("unsafe or missing retry guidance: %q", fix)
			}
			b.fail, b.calls = "", nil
			j, err = Execute(context.Background(), o, b)
			if err != nil || j.Status != "complete" || b.calls[0] != "backup" {
				t.Fatalf("retry did not take a fresh dump: %+v %v %v", j, b.calls, err)
			}
		})
	}
}

func TestStackUpdateNonProdResumeInvalidBackupAfterStart(t *testing.T) {
	for _, step := range []string{"start", "migrate"} {
		for _, result := range []string{"running", "failed", "ok"} {
			t.Run(step+"/"+result, func(t *testing.T) {
				o := Options{ProjectDir: t.TempDir(), Yes: true}
				b := newFixture()
				j := newJournal(false)
				if err := runStep(context.Background(), b, j, "backup"); err != nil {
					t.Fatal(err)
				}
				for i := range j.Steps {
					j.Steps[i].Result = "ok"
					if j.Steps[i].Name == step {
						j.Steps[i].Result = result
						break
					}
				}
				if err := saveJournal(o.ProjectDir, j); err != nil {
					t.Fatal(err)
				}
				b.fail, b.calls = "verify", nil
				got, err := Execute(context.Background(), o, b)
				assertCode(t, err, "E139")
				if !reflect.DeepEqual(got, j) || !reflect.DeepEqual(b.calls, []string{"verify"}) {
					t.Fatalf("unsafe resume changed evidence or ran operations: %+v %v", got, b.calls)
				}
				fix := cliFix(t, err)
				for _, want := range []string{VerifyCommand(j.BackupID), "moving .nself/state/update-stack.json to .nself/state/update-stack." + j.RunID + ".json"} {
					if !strings.Contains(fix, want) {
						t.Fatalf("missing operator guidance %q: %q", want, fix)
					}
				}
				if strings.Contains(fix, "backup restore") {
					t.Fatalf("suggested unverified restore: %q", fix)
				}
			})
		}
	}
}
