package stack

import (
	"context"
	"testing"
	"time"
)

type recoveryDeadlineBackend struct {
	*fixtureBackend
	t               *testing.T
	cancel          context.CancelFunc
	recoveryStarted time.Time
	restored        bool
}

func (b *recoveryDeadlineBackend) Run(ctx context.Context, name string, j *Journal) error {
	err := b.fixtureBackend.Run(ctx, name, j)
	if name == "migrate" {
		b.cancel()
		b.recoveryStarted = time.Now()
	}
	return err
}

func (b *recoveryDeadlineBackend) Restore(ctx context.Context, j *Journal) error {
	b.t.Helper()
	b.restored = true
	if err := ctx.Err(); err != nil {
		b.t.Fatalf("restore inherited migration cancellation: %v", err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		b.t.Fatal("restore has no recovery deadline")
	}
	if budget := deadline.Sub(b.recoveryStarted); budget < stepTimeout("migrate") {
		b.t.Fatalf("restore budget=%s, want >= migration budget %s", budget, stepTimeout("migrate"))
	}
	return b.fixtureBackend.Restore(ctx, j)
}

func TestStackUpdateRecoveryDeadlineReachesRestore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	b := &recoveryDeadlineBackend{fixtureBackend: newFixture(), t: t, cancel: cancel}
	b.fail = "migrate"
	j, err := Execute(ctx, Options{ProjectDir: t.TempDir(), Yes: true}, b)
	assertCode(t, err, "E139")
	if !b.restored || j.Status != "rolled-back" {
		t.Fatalf("recovery did not complete: restored=%v journal=%+v", b.restored, j)
	}
}
