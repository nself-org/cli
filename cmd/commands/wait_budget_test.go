package commands

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/ssl"
)

// D-0736: the cmd/commands test binary ran ~550 s of its 600 s package timeout
// on macos-15-intel because production waits (the 500 ms served-certificate
// poll, the 5 s and 2 s restart settle pauses) ran in wall-clock time inside
// unit tests. TestMain shortens them; these two tests fail if that regresses.
// The budgets are far above the shortened waits and far below the real ones.

const waitBudget = 3 * time.Second

func TestPollServedFailureDoesNotWaitWallClock(t *testing.T) {
	old := acmeD
	t.Cleanup(func() { acmeD = old })
	acmeD.probe = func(context.Context, string, string, time.Duration) (ssl.ServedCert, error) {
		return ssl.ServedCert{}, errors.New("dial refused")
	}
	start := time.Now()
	if err := pollServed(context.Background(), "127.0.0.1:1", "app.example.org", "want"); err == nil {
		t.Fatal("pollServed succeeded against a failing probe")
	}
	if got := time.Since(start); got > waitBudget {
		t.Errorf("pollServed slept %v between probes (budget %v): TestMain must shorten acmeVerifyWait", got, waitBudget)
	}
}

func TestRestartDoesNotWaitWallClock(t *testing.T) {
	dir := lifecycleProject(t)
	start := time.Now()
	if _, err := lifecycleRun(t, "restart", dir, "--json", "hasura"); err != nil {
		t.Fatal(err)
	}
	if got := time.Since(start); got > waitBudget {
		t.Errorf("restart took %v (budget %v): TestMain must shorten the settle pause", got, waitBudget)
	}
}
