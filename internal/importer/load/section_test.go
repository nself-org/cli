package load

import (
	"context"
	"errors"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

func failingOneShot(t *testing.T, fail string) func() {
	t.Helper()
	old := runOneShot
	runOneShot = func(_ context.Context, spec docker.RunSpec) (string, string, error) {
		for _, a := range spec.Args {
			if a == "--section="+fail {
				return "", "", errors.New("boom")
			}
		}
		return "", "", nil
	}
	return func() { runOneShot = old }
}

// A pre-data failure rolls back (--single-transaction): with no compat.sql run, nothing was written.
func TestRunSectionPreDataFailureWithoutCompatWroteNothing(t *testing.T) {
	defer failingOneShot(t, "pre-data")()
	p := &Progress{}
	if err := runSection(context.Background(), docker.RunSpec{}, &config.Config{}, "pre-data", p); err == nil {
		t.Fatal("expected failure")
	}
	if p.Wrote {
		t.Fatal("pre-data rolled back, so Wrote must be false")
	}
}

func TestRunSectionPreDataFailureAfterCompatStaysWritten(t *testing.T) {
	defer failingOneShot(t, "pre-data")()
	p := &Progress{Step: "compat"}
	if err := runSection(context.Background(), docker.RunSpec{}, &config.Config{}, "pre-data", p); err == nil {
		t.Fatal("expected failure")
	}
	if !p.Wrote {
		t.Fatal("compat.sql ran, so Wrote must stay true")
	}
}

func TestRunSectionCancelAndLaterSectionsStayWritten(t *testing.T) {
	defer failingOneShot(t, "pre-data")()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &Progress{}
	_ = runSection(ctx, docker.RunSpec{}, &config.Config{}, "pre-data", p)
	if !p.Wrote {
		t.Fatal("a cancelled run may still have a running one-shot: Wrote must stay true")
	}
	defer failingOneShot(t, "data")()
	p = &Progress{Step: "pre-data"}
	_ = runSection(context.Background(), docker.RunSpec{}, &config.Config{}, "data", p)
	if !p.Wrote || p.Step != "pre-data" {
		t.Fatalf("a data failure follows a committed pre-data: %+v", p)
	}
	if err := runSection(context.Background(), docker.RunSpec{}, &config.Config{}, "post-data", p); err != nil || p.Step != "post-data" {
		t.Fatalf("success must record the step: %v %+v", err, p)
	}
}
