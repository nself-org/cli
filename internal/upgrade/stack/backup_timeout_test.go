package stack

import (
	"context"
	"errors"
	"testing"
	"time"
)

type blockingBackupBackend struct {
	*fixtureBackend
	stage   string
	bounded bool
	cancel  context.CancelFunc
}

func (b *blockingBackupBackend) block(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	b.bounded = ok && time.Until(deadline) <= stepTimeout("migrate")
	if !ok {
		return errors.New("missing backup deadline")
	}
	// A blocking backend must observe cancellation, without waiting out the budget.
	b.cancel()
	<-ctx.Done()
	return ctx.Err()
}
func (b *blockingBackupBackend) Backup(ctx context.Context, j *Journal) error {
	if b.stage == "backup" {
		return b.block(ctx)
	}
	return b.fixtureBackend.Backup(ctx, j)
}
func (b *blockingBackupBackend) VerifyBackup(ctx context.Context, j *Journal) error {
	if b.stage == "verify" {
		return b.block(ctx)
	}
	return b.fixtureBackend.VerifyBackup(ctx, j)
}
func (b *blockingBackupBackend) CountRows(ctx context.Context) (map[string]int64, error) {
	return nil, b.block(ctx)
}
func TestBackupStepBoundsEveryOperation(t *testing.T) {
	for _, stage := range []string{"backup", "verify", "count"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			b := &blockingBackupBackend{fixtureBackend: newFixture(), stage: stage, cancel: cancel}
			err := runStep(ctx, b, newJournal(false), "backup")
			if !b.bounded || !errors.Is(err, context.Canceled) {
				t.Fatalf("bounded=%v error=%v", b.bounded, err)
			}
		})
	}
}
