package stack

import (
	"context"
	"strings"
	"testing"
)

// indexOf returns the position of name in calls, or -1.
func indexOf(calls []string, name string) int {
	for i, c := range calls {
		if c == name {
			return i
		}
	}
	return -1
}

// TestStackUpdateRollbackOrderStopsServicesBeforeRestore (M2) proves the order
// check images, stop services, restore, restart, count rows, and that no
// new-image service is running when the restore starts.
func TestStackUpdateRollbackOrderStopsServicesBeforeRestore(t *testing.T) {
	b := newFixture()
	b.fail = "migrate"
	j, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Yes: true}, b)
	assertCode(t, err, "E139")
	if j.Status != "rolled-back" || b.runningAtRestore {
		t.Fatalf("restore ran with new-image services up: %+v running=%v", j, b.runningAtRestore)
	}
	last := -1
	for _, name := range []string{"check-images", "stop-services", "restore", "restart-previous"} {
		i := indexOf(b.calls, name)
		if i <= last {
			t.Fatalf("rollback order wrong at %s: %v", name, b.calls)
		}
		last = i
	}
	if indexOf(b.calls[last:], "rows") < 0 {
		t.Fatalf("rows not counted after restart: %v", b.calls)
	}
}

// TestStackUpdateRollbackFailurePoints (M2) proves each failure point stops the
// sequence there and ends rollback-failed with the restore and restart guidance.
func TestStackUpdateRollbackFailurePoints(t *testing.T) {
	cases := []struct {
		fault    string
		never    []string
		wantNote string
	}{
		{"check-images", []string{"stop-services", "restore", "restart-previous"}, "database is untouched"},
		{"stop-services", []string{"restore", "restart-previous"}, ""},
		{"restore", []string{"restart-previous"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.fault, func(t *testing.T) {
			b := &migrateThenFault{fixtureBackend: newFixture(), fault: tc.fault}
			j, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Yes: true}, b)
			assertCode(t, err, "E139")
			if j.Status != "rollback-failed" {
				t.Fatalf("status = %s", j.Status)
			}
			for _, never := range tc.never {
				if contains(b.calls, never) {
					t.Fatalf("%s ran after %s failed: %v", never, tc.fault, b.calls)
				}
			}
			fix := cliFix(t, err)
			if !strings.Contains(fix, RestoreCommand("step-one-backup")) || !strings.Contains(fix, RestartCommand("images=N")) || !strings.Contains(fix, tc.wantNote) {
				t.Fatalf("guidance incomplete: %q", fix)
			}
		})
	}
}

// migrateThenFault fails migrate, then fails the named rollback hook.
type migrateThenFault struct {
	*fixtureBackend
	fault string
}

func (b *migrateThenFault) Run(ctx context.Context, name string, j *Journal) error {
	if name == "migrate" {
		b.fixtureBackend.Run(ctx, name, j) //nolint:errcheck // seeds new-version state; the failure is injected below
		b.calls = append(b.calls, "migrate-failed")
		b.fail = b.fault
		return errInjectedMigrate
	}
	return b.fixtureBackend.Run(ctx, name, j)
}

var errInjectedMigrate = &injected{"injected migrate failure"}

type injected struct{ s string }

func (e *injected) Error() string { return e.s }

// TestStackUpdateProdFixOnlyWhenDatabaseMayHaveChanged (S1) proves a prod pull or
// build failure says nothing changed, while start and migrate failures carry the restore.
func TestStackUpdateProdFixOnlyWhenDatabaseMayHaveChanged(t *testing.T) {
	for _, tc := range []struct {
		step    string
		restore bool
	}{{"pull", false}, {"build", false}, {"start", true}, {"migrate", true}, {"readiness", true}, {"health", true}} {
		t.Run(tc.step, func(t *testing.T) {
			b := newFixture()
			b.fail = tc.step
			_, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}, b)
			assertCode(t, err, "E139")
			fix := cliFix(t, err)
			if got := strings.Contains(fix, "backup restore"); got != tc.restore {
				t.Fatalf("restore guidance = %v, want %v: %q", got, tc.restore, fix)
			}
			if !tc.restore && !strings.Contains(fix, "Nothing was changed") {
				t.Fatalf("missing retry guidance: %q", fix)
			}
			if tc.restore && (!strings.Contains(fix, RestartCommand("images=N")) || !strings.Contains(fix, "images=N")) {
				t.Fatalf("missing snapshot and restart command: %q", fix)
			}
		})
	}
}

// TestStackUpdateBackupVerifyFailureClearsEvidenceForFreshDump (S3) proves a
// failed step-one verification clears the journal evidence, names it, keeps the
// file reference in the message, and lets the retry take a fresh dump.
func TestStackUpdateBackupVerifyFailureClearsEvidenceForFreshDump(t *testing.T) {
	b := newFixture()
	b.fail = "verify"
	o := Options{ProjectDir: t.TempDir(), Yes: true}
	j, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	stored, loadErr := loadJournal(o.ProjectDir)
	if loadErr != nil || stored.BackupID != "" || stored.BackupSHA256 != "" || stored.PreviousCompose != "" || stored.BeforeRows != nil {
		t.Fatalf("evidence not cleared on disk: %+v %v", stored, loadErr)
	}
	fix := cliFix(t, err)
	if j.BackupID != "" || !strings.Contains(fix, "step-one-backup") || !strings.Contains(fix, "fresh dump") {
		t.Fatalf("message does not name the cleared backup: %q", fix)
	}
	b.fail, b.calls = "", nil
	j, err = Execute(context.Background(), o, b)
	if err != nil || j.Status != "complete" || b.calls[0] != "backup" {
		t.Fatalf("retry did not take a fresh dump: %v %v %+v", b.calls, err, j)
	}
}

// TestStackUpdateBlockedRecoveryNamesSnapshotAndRestart (S5) proves the blocked
// rollback states print the snapshot path and the exact restart command.
func TestStackUpdateBlockedRecoveryNamesSnapshotAndRestart(t *testing.T) {
	for _, status := range []string{"rollback-running", "rollback-failed"} {
		t.Run(status, func(t *testing.T) {
			o := Options{ProjectDir: t.TempDir(), Yes: true}
			j := newJournal(false)
			j.Status, j.BackupID, j.PreviousCompose = status, "step-one-backup", "/p/.nself/state/update-stack-previous-1.json"
			if err := saveJournal(o.ProjectDir, j); err != nil {
				t.Fatal(err)
			}
			_, err := Execute(context.Background(), o, newFixture())
			assertCode(t, err, "E139")
			fix := cliFix(t, err)
			for _, want := range []string{RestoreCommand("step-one-backup"), RestartCommand(j.PreviousCompose), j.PreviousCompose, "--remove-orphans"} {
				if !strings.Contains(fix, want) {
					t.Fatalf("blocked guidance omits %q: %q", want, fix)
				}
			}
		})
	}
}
