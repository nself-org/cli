package stack

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

type processFailure struct{ code int }

func (e *processFailure) Error() string { return "injected process failure" }
func (e *processFailure) ExitCode() int { return e.code }

type processFailureBackend struct {
	*fixtureBackend
	failure *processFailure
	backup  bool
}

func (b *processFailureBackend) Backup(ctx context.Context, j *Journal) error {
	if b.backup {
		return b.failure
	}
	return b.fixtureBackend.Backup(ctx, j)
}

func (b *processFailureBackend) Run(context.Context, string, *Journal) error {
	return b.failure
}

func TestStackExitClassOverridesNestedProcessStatus(t *testing.T) {
	for _, status := range []int{1, 42} {
		for _, productionBackup := range []bool{false, true} {
			b := &processFailureBackend{fixtureBackend: newFixture(), failure: &processFailure{status}, backup: productionBackup}
			_, err := Execute(context.Background(), Options{ProjectDir: t.TempDir(), Prod: productionBackup, Yes: true}, b)
			wantCode, wantExit := "E139", 2
			if productionBackup {
				wantCode, wantExit = "E403", 4
			}
			assertCode(t, err, wantCode)
			var coder errs.ExitCoder
			if !errors.As(err, &coder) || coder.ExitCode() != wantExit || errs.ExitCodeFor(err) != wantExit {
				t.Fatalf("%s nested process status %d overrode class %d: %v", wantCode, status, wantExit, err)
			}
			if !errors.Is(err, b.failure) {
				t.Fatal("original process error was lost")
			}
		}
	}
}

func TestStackWithExitClassFallback(t *testing.T) {
	if err := WithExitClass(nil); err != nil {
		t.Fatalf("nil became failure: %v", err)
	}
	if got := errs.ExitCodeFor(WithExitClass(&processFailure{42})); got != 42 {
		t.Fatalf("unclassified process status changed: %d", got)
	}
	if got := errs.ExitCodeFor(WithExitClass(errors.New("uncoded failure"))); got != 1 {
		t.Fatalf("uncoded fallback changed: %d", got)
	}
}
