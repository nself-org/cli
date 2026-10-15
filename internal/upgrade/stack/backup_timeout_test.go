package stack

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

type blockingBackupBackend struct {
	*fixtureBackend
	t       *testing.T
	blockAt string
	seen    context.Context
}

func (b *blockingBackupBackend) block(ctx context.Context, name string) error {
	b.t.Helper()
	if b.seen != nil && ctx != b.seen {
		b.t.Fatal("backup operations must share one timeout context")
	}
	b.seen = ctx
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) != stepTimeout("migrate") {
		b.t.Fatalf("%s deadline = %v, present = %v; want migration timeout", name, deadline, ok)
	}
	if name == b.blockAt {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (b *blockingBackupBackend) Backup(ctx context.Context, j *Journal) error {
	if err := b.block(ctx, "backup"); err != nil {
		return err
	}
	return b.fixtureBackend.Backup(ctx, j)
}

func (b *blockingBackupBackend) VerifyBackup(ctx context.Context, j *Journal) error {
	if err := b.block(ctx, "verify"); err != nil {
		return err
	}
	return b.fixtureBackend.VerifyBackup(ctx, j)
}

func (b *blockingBackupBackend) CountRows(ctx context.Context) (map[string]int64, error) {
	if err := b.block(ctx, "rows"); err != nil {
		return nil, err
	}
	return b.fixtureBackend.CountRows(ctx)
}

func TestBackupStepTimeout(t *testing.T) {
	for _, phase := range []string{"backup", "verify", "rows"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				b := &blockingBackupBackend{fixtureBackend: newFixture(), t: t, blockAt: phase}
				started := time.Now()
				j := &Journal{StartedAt: started}
				err := runStep(context.Background(), b, j, "backup")
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("blocked %s: want deadline exceeded, got %v", phase, err)
				}
				if elapsed := time.Since(started); elapsed != stepTimeout("migrate") {
					t.Fatalf("backup elapsed %s, want migration timeout", elapsed)
				}
				if j.BeforeRows != nil {
					t.Fatal("timed-out backup recorded row evidence")
				}
			})
		})
	}
}
