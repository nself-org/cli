package stack

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/errs"
)

type fixtureBackend struct {
	fail       string
	noProof    bool
	oldBackup  bool
	calls      []string
	rows       map[string]int64
	backupRows map[string]int64
	image      string
	badRestore bool
	// appsStopped is set by StopServices; runningAtRestore records that Restore
	// ran while new-image services were still up (the M2 hazard).
	appsStopped      bool
	runningAtRestore bool
}

func newFixture() *fixtureBackend {
	return &fixtureBackend{rows: map[string]int64{"public.accounts": 7, "np_chat.messages": 23}, image: "N"}
}

func (b *fixtureBackend) called(name string) error {
	b.calls = append(b.calls, name)
	if b.fail == name {
		return errors.New("injected " + name + " failure")
	}
	return nil
}

func (b *fixtureBackend) Backup(_ context.Context, j *Journal) error {
	if err := b.called("backup"); err != nil {
		return err
	}
	j.BackupID = "step-one-backup"
	j.BackupCreatedAt = time.Now().UTC()
	if b.oldBackup {
		j.BackupCreatedAt = j.StartedAt.Add(-time.Second)
	}
	j.PreviousCompose = "images=N"
	b.backupRows = cloneRows(b.rows)
	return nil
}

func (b *fixtureBackend) VerifyBackup(_ context.Context, j *Journal) error {
	if err := b.called("verify"); err != nil {
		return err
	}
	if !b.noProof {
		j.BackupVerifiedAt = time.Now().UTC()
	}
	return nil
}

func (b *fixtureBackend) Run(_ context.Context, name string, _ *Journal) error {
	if name == "start" {
		b.image = "N+1"
	}
	if name == "migrate" {
		b.rows["public.accounts"] = 0
	}
	return b.called(name)
}

func (b *fixtureBackend) CheckPreviousImages(_ context.Context, _ *Journal) error {
	return b.called("check-images")
}

func (b *fixtureBackend) StopServices(_ context.Context, _ *Journal) error {
	if err := b.called("stop-services"); err != nil {
		return err
	}
	b.appsStopped = true
	return nil
}

func (b *fixtureBackend) Restore(_ context.Context, _ *Journal) error {
	if err := b.called("restore"); err != nil {
		return err
	}
	b.runningAtRestore = !b.appsStopped
	b.rows = cloneRows(b.backupRows)
	if b.badRestore {
		b.rows["public.accounts"]++
	}
	return nil
}

func (b *fixtureBackend) RestartPrevious(_ context.Context, j *Journal) error {
	if j.PreviousCompose != "images=N" {
		return errors.New("previous images missing")
	}
	b.image = "N"
	return b.called("restart-previous")
}

func (b *fixtureBackend) CountRows(_ context.Context) (map[string]int64, error) {
	if err := b.called("rows"); err != nil {
		return nil, err
	}
	return cloneRows(b.rows), nil
}

func cloneRows(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func assertCode(t *testing.T, err error, code string) {
	t.Helper()
	var e *errs.CLIError
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func TestStackUpdateResume(t *testing.T) {
	for _, s := range ExecutionPlan() {
		t.Run(s.Name, func(t *testing.T) {
			b := newFixture()
			b.fail = s.Name
			o := Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}
			j, err := Execute(context.Background(), o, b)
			code := "E139"
			if s.Name == "backup" {
				code = "E403"
			}
			assertCode(t, err, code)
			if j == nil {
				t.Fatalf("failure returned no execution journal: %v", err)
			}
			if j.Steps[firstPending(j)].Name != s.Name {
				t.Fatalf("wrong resume point: %+v", j.Steps)
			}
			b.fail, b.calls = "", nil
			j, err = Execute(context.Background(), o, b)
			if err != nil || j.Status != "complete" {
				t.Fatalf("resume failed: %+v %v", j, err)
			}
			first := s.Name
			if first != "backup" {
				first = "verify"
			}
			if b.calls[0] != first {
				t.Fatalf("unexpected resumed calls: %v", b.calls)
			}
			for _, prior := range ExecutionPlan() {
				if prior.Name == s.Name {
					break
				}
				if contains(b.calls, prior.Name) {
					t.Fatalf("repeated completed step %s: %v", prior.Name, b.calls)
				}
			}
		})
	}
	t.Run("interrupted-running-start", func(t *testing.T) {
		b := newFixture()
		o := Options{ProjectDir: t.TempDir(), Yes: true}
		j := newJournal(false)
		if err := runStep(context.Background(), b, j, "backup"); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			j.Steps[i].Result = "ok"
		}
		j.Steps[3].Result = "running"
		if err := saveJournal(o.ProjectDir, j); err != nil {
			t.Fatal(err)
		}
		b.calls = nil
		got, err := Execute(context.Background(), o, b)
		if err != nil || got.Status != "complete" || !reflect.DeepEqual(b.calls[:2], []string{"verify", "start"}) {
			t.Fatalf("did not resume running start: %v %v", b.calls, err)
		}
	})
}

func TestStackUpdateProdGuardE403(t *testing.T) {
	for _, fault := range []string{"backup", "verify", "missing-proof", "old-backup"} {
		t.Run(fault, func(t *testing.T) {
			b := newFixture()
			b.fail, b.noProof, b.oldBackup = fault, fault == "missing-proof", fault == "old-backup"
			_, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}, b)
			assertCode(t, err, "E403")
			if contains(b.calls, "pull") {
				t.Fatalf("production guard allowed mutation: %v", b.calls)
			}
		})
	}
}

func TestStackUpdateRollbackNonProd(t *testing.T) {
	b := newFixture()
	before := cloneRows(b.rows)
	b.fail = "migrate"
	o := Options{ProjectDir: t.TempDir(), Yes: true}
	j, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	if j.Status != "rolled-back" || b.image != "N" || !reflect.DeepEqual(before, b.rows) || !reflect.DeepEqual(j.BeforeRows, j.AfterRows) {
		t.Fatalf("rollback did not recover images and per-table rows: %+v %+v", j, b)
	}
	if !contains(b.calls, "restore") || !contains(b.calls, "restart-previous") || contains(b.calls, "readiness") {
		t.Fatalf("wrong recovery calls: %v", b.calls)
	}
	b.calls = nil
	b.fail = ""
	recoveredID := j.RunID
	j, err = Execute(context.Background(), o, b)
	if err != nil || j.Status != "complete" || j.RunID == recoveredID || b.calls[0] != "backup" {
		t.Fatalf("did not start new run after recovery: %+v %v %v", j, b.calls, err)
	}
	t.Run("row-count-negative-control", func(t *testing.T) {
		b := newFixture()
		b.fail, b.badRestore = "migrate", true
		j, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Yes: true}, b)
		assertCode(t, err, "E139")
		var ce *errs.CLIError
		if !errors.As(err, &ce) || j.Status != "rollback-failed" || !strings.Contains(ce.Wrapped.Error(), "row counts differ") {
			t.Fatalf("bad restore was accepted: %+v %v", j, err)
		}
	})
}

func TestStackUpdateProdFailureE139(t *testing.T) {
	b := newFixture()
	b.fail = "migrate"
	o := Options{ProjectDir: t.TempDir(), Prod: true, Yes: true}
	j, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	if j.Status != "awaiting-operator" || contains(b.calls, "restore") || contains(b.calls, "restart-previous") {
		t.Fatalf("production auto-restored: %+v %v", j, b.calls)
	}
	if !strings.Contains(err.Error(), "nself backup restore 'step-one-backup'") {
		t.Fatalf("missing exact recovery guidance: %v", err)
	}
	// After restoring by hand the operator must archive the journal; a plain retry
	// would resume at migrate against the restored old stack.
	if !strings.Contains(err.Error(), "move .nself/state/update-stack.json to .nself/state/update-stack."+j.RunID+".json") {
		t.Fatalf("recovery guidance does not say to archive the journal: %v", err)
	}
	stored, err := loadJournal(o.ProjectDir)
	if err != nil || stored.Status != "awaiting-operator" {
		t.Fatalf("journal did not retain operator status: %+v %v", stored, err)
	}
}

func TestStackUpdateNoTTYE403(t *testing.T) {
	b := newFixture()
	project := t.TempDir()
	_, err := Execute(context.Background(), Options{ProjectDir: project}, b)
	assertCode(t, err, "E403")
	if len(b.calls) != 0 {
		t.Fatalf("backend called without consent: %v", b.calls)
	}
	if _, err := os.Stat(journalPath(project)); !os.IsNotExist(err) {
		t.Fatalf("journal created without consent: %v", err)
	}
}

// TestNonProdResumeResetAfterPullOkBuildFailed covers the regression where
// pull succeeded before build failed, and a subsequent resume discards the
// backup. Without resetting every step, the journal would read
// pending,ok,failed,... which validateJournal rejects as noncontiguous.
func TestNonProdResumeResetAfterPullOkBuildFailed(t *testing.T) {
	o := Options{ProjectDir: t.TempDir(), Yes: true}
	b := newFixture()
	j := newJournal(false)
	// Simulate: backup ok, pull ok, build failed.
	if err := runStep(context.Background(), b, j, "backup"); err != nil {
		t.Fatal(err)
	}
	for i := range j.Steps {
		switch j.Steps[i].Name {
		case "backup":
			j.Steps[i].Result = "ok"
		case "pull":
			if err := runStep(context.Background(), b, j, "pull"); err != nil {
				t.Fatal(err)
			}
			j.Steps[i].Result = "ok"
		case "build":
			j.Steps[i].Result = "failed"
		}
	}
	if err := saveJournal(o.ProjectDir, j); err != nil {
		t.Fatal(err)
	}
	// On resume, verify fails so backup evidence is cleared pre-mutation.
	b.fail, b.calls = "verify", nil
	got, err := Execute(context.Background(), o, b)
	assertCode(t, err, "E139")
	disk, loadErr := loadJournal(o.ProjectDir)
	if loadErr != nil {
		t.Fatalf("journal unreadable after reset: %v", loadErr)
	}
	if got.BackupID != "" || disk.BackupID != "" {
		t.Fatalf("stale backup retained after reset: %+v", disk)
	}
	for i, s := range disk.Steps {
		if s.Result != "pending" {
			t.Fatalf("step %d (%s) not reset to pending: %q", i, s.Name, s.Result)
		}
	}
	// A fresh retry must succeed without hitting the noncontiguous guard.
	b.fail, b.calls = "", nil
	got, err = Execute(context.Background(), o, b)
	if err != nil || got.Status != "complete" || b.calls[0] != "backup" {
		t.Fatalf("fresh retry after mid-run reset: %v %v", b.calls, err)
	}
}

func contains(items []string, name string) bool {
	for _, item := range items {
		if item == name {
			return true
		}
	}
	return false
}

// boundedBackupBackend blocks in each backup phase until its step context ends.
type boundedBackupBackend struct {
	*fixtureBackend
	phase string
}

func (b *boundedBackupBackend) block(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("backup phase has no deadline")
	}
	if time.Until(deadline) > stepTimeout("migrate") {
		return errors.New("backup deadline exceeds migration budget")
	}
	// A shortened child keeps this regression test fast while exercising cancellation.
	child, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	<-child.Done()
	return child.Err()
}
func (b *boundedBackupBackend) Backup(ctx context.Context, j *Journal) error {
	if b.phase == "backup" {
		return b.block(ctx)
	}
	return b.fixtureBackend.Backup(ctx, j)
}
func (b *boundedBackupBackend) VerifyBackup(ctx context.Context, j *Journal) error {
	if b.phase == "verify" {
		return b.block(ctx)
	}
	return b.fixtureBackend.VerifyBackup(ctx, j)
}
func (b *boundedBackupBackend) CountRows(ctx context.Context) (map[string]int64, error) {
	if b.phase == "rows" {
		return nil, b.block(ctx)
	}
	return b.fixtureBackend.CountRows(ctx)
}
func TestBackupPhasesHaveStepDeadline(t *testing.T) {
	for _, phase := range []string{"backup", "verify", "rows"} {
		t.Run(phase, func(t *testing.T) {
			b := &boundedBackupBackend{fixtureBackend: newFixture(), phase: phase}
			err := runStep(context.Background(), b, newJournal(false), "backup")
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("phase did not receive bounded context: %v", err)
			}
		})
	}
}

func TestNonProdResumeDamagedBackup(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-start", true: "after-start"}[started], func(t *testing.T) {
			o := Options{ProjectDir: t.TempDir(), Yes: true}
			b := newFixture()
			j := newJournal(false)
			if err := runStep(context.Background(), b, j, "backup"); err != nil {
				t.Fatal(err)
			}
			for i := range j.Steps {
				if j.Steps[i].Name == "backup" {
					j.Steps[i].Result = "ok"
				}
				if started {
					if j.Steps[i].Name == "start" {
						j.Steps[i].Result = "running"
						break
					}
					j.Steps[i].Result = "ok"
				}
			}
			if err := saveJournal(o.ProjectDir, j); err != nil {
				t.Fatal(err)
			}
			b.fail = "verify"
			got, err := Execute(context.Background(), o, b)
			assertCode(t, err, "E139")
			var e *errs.CLIError
			if !errors.As(err, &e) {
				t.Fatal(err)
			}
			if started {
				if got.BackupID == "" || !strings.Contains(e.Fix, "archive") || !strings.Contains(e.Fix, "verify") {
					t.Fatalf("unsafe evidence handling or missing guidance: %+v %v", got, e)
				}
				return
			}
			disk, loadErr := loadJournal(o.ProjectDir)
			if loadErr != nil || got.BackupID != "" || disk.BackupID != "" || ResumeStep(disk) != "backup" {
				t.Fatalf("stale backup retained: %+v %v", disk, loadErr)
			}
			b.fail, b.calls = "", nil
			got, err = Execute(context.Background(), o, b)
			if err != nil || got.Status != "complete" || b.calls[0] != "backup" {
				t.Fatalf("fresh backup retry: %v %v", b.calls, err)
			}
		})
	}
}
