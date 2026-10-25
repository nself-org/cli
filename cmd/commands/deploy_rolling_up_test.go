package commands

// deploy_rolling_up_test.go — the rolling `up` is bounded and its failures say
// what happened and how to recover (D-0469). Uses the fake docker of
// deploy_strategies_test.go; nothing here needs a daemon.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

func TestRollingUp(t *testing.T) {
	stack := func(t *testing.T, fixture string) *rollingStack {
		t.Helper()
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		if err := os.WriteFile(filepath.Join(s.fix, fixture), []byte("web"), 0o600); err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("a hung up ends in a timeout error that names the service and the way back", func(t *testing.T) {
		s := stack(t, "uphang")
		old := rollingUpTimeout
		rollingUpTimeout = 300 * time.Millisecond
		t.Cleanup(func() { rollingUpTimeout = old })
		start := time.Now()
		_, err := runRollingRestart(context.Background(), s.dir, true)
		if time.Since(start) > 30*time.Second {
			t.Fatalf("deploy was not cut off: took %s", time.Since(start))
		}
		if err == nil {
			t.Fatal("want a timeout error, got nil")
		}
		for _, want := range []string{"service web did not finish 'up' within 300ms", "nself restart web"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
		if errors.Is(err, context.Canceled) {
			t.Errorf("a timeout must not read as the user's interrupt: %v", err)
		}
	})

	t.Run("docker exiting 130 before nself's handler runs reads as an interrupt", func(t *testing.T) {
		s := stack(t, "up130")
		_, err := runRollingRestart(context.Background(), s.dir, true)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want an interrupt, got %v", err)
		}
		for _, want := range []string{"interrupted while bringing up service web", "nself restart web"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
		if strings.Contains(err.Error(), "restart failed") || errors.Is(err, docker.ErrStateRead) {
			t.Errorf("an interrupt must not read as a failure: %v", err)
		}
	})

	t.Run("a cancelled context prints the recovery line", func(t *testing.T) {
		s := stack(t, "uphang")
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(300*time.Millisecond, cancel)
		_, err := runRollingRestart(ctx, s.dir, true)
		if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "nself restart web") {
			t.Fatalf("want an interrupt with the recovery line, got %v", err)
		}
	})
}

func TestGateKindPrintsTheRealLimit(t *testing.T) {
	for name, tc := range map[string]struct {
		spec docker.ServiceSpec
		want string
	}{
		"healthcheck default":    {docker.ServiceSpec{HasHealthcheck: true}, "healthy, max 60s"},
		"healthcheck slow":       {docker.ServiceSpec{HasHealthcheck: true, HealthTimeout: 105 * time.Second}, "healthy, max 105s"},
		"healthcheck at the cap": {docker.ServiceSpec{HasHealthcheck: true, HealthTimeout: docker.MaxHealthTimeout}, "healthy, max 600s"},
		"one-shot":               {docker.ServiceSpec{Restart: "no"}, "one-shot: exit 0, max 60s"},
		"plain":                  {docker.ServiceSpec{}, "running, max 30s"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := gateKind(tc.spec); got != tc.want {
				t.Errorf("gateKind = %q, want %q", got, tc.want)
			}
		})
	}
}
