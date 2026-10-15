package commands

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

func TestRollingGateKindLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		spec docker.ServiceSpec
		want string
	}{
		{"slow healthcheck", docker.ServiceSpec{HasHealthcheck: true, HealthTimeout: 3 * time.Minute}, "healthy, max 3m0s"},
		{"short healthcheck", docker.ServiceSpec{HasHealthcheck: true, HealthTimeout: time.Second}, "healthy, max 1m0s"},
		{"one-shot", docker.ServiceSpec{Restart: "no"}, "one-shot: exit 0, max 60s"},
		{"running", docker.ServiceSpec{}, "running, max 30s"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := gateKind(tc.spec); got != tc.want {
				t.Fatalf("gateKind = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRollingChildInterrupt(t *testing.T) {
	for _, phase := range []string{"marks", "up", "gate", "recheck"} {
		t.Run(phase, func(t *testing.T) {
			s := newRollingStack(t, []string{"web", "ntask"}, stackConfig)
			s.setPs(t, "web", psRunning)
			s.setPs(t, "ntask", psRunning)
			// Exit before the parent's signal handler cancels its context.
			condition := map[string]string{
				"marks":   "[ \"$1\" = ps ]",
				"up":      "[ \"$1\" = up ]",
				"gate":    "[ \"$1\" = ps ] && [ -f \"$D/ups\" ]",
				"recheck": "[ \"$1\" = ps ] && [ \"$last\" = web ] && [ \"$(wc -l < \"$D/ups\" 2>/dev/null)\" -ge 2 ]",
			}[phase]
			script := strings.Replace(rollingFakeDocker, "case \"$1\" in", "for last; do :; done\nif "+condition+"; then exit 130; fi\ncase \"$1\" in", 1)
			bin := filepath.Join(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0], "docker")
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // executable test stub
				t.Fatal(err)
			}
			steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
			if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "cannot read") {
				t.Fatalf("want interrupt classification, got %v", err)
			}
			if phase == "up" && !strings.Contains(err.Error(), "nself start web") {
				t.Fatalf("missing recovery hint: %v", err)
			}
			if len(steps) == 0 || steps[0].Status != "failed" || (phase != "recheck" && strings.Contains(s.read("ups"), "ntask")) {
				t.Fatalf("roll continued after interrupt: steps=%+v ups=%q", steps, s.read("ups"))
			}
		})
	}
}

func TestRollingUpBound(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "parent cancellation"}[interrupted], func(t *testing.T) {
			s := newRollingStack(t, []string{"web", "ntask"}, stackConfig)
			bin := filepath.Join(strings.Split(os.Getenv("PATH"), string(os.PathListSeparator))[0], "docker")
			script := strings.Replace(rollingFakeDocker, "up)\n", "up)\necho started > \"$D/upstarted\"\nexec sleep 30\n", 1)
			if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // executable test stub
				t.Fatal(err)
			}
			compose, _, err := deployCompose(s.dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			limit := 250 * time.Millisecond
			want, detail := context.DeadlineExceeded, "up timed out after 250ms"
			if interrupted {
				limit = time.Minute
				want, detail = context.Canceled, "interrupted during up"
				go cancelRollingUpWhenStarted(ctx, s, cancel)
			}
			plan := rollingPlan{compose: compose, workdir: s.dir, order: []string{"web", "ntask"},
				specs: []docker.ServiceSpec{{Name: "web"}, {Name: "ntask"}}, jsonOut: true, upTimeout: limit}
			steps, err := rollingRestart(ctx, plan)
			if !errors.Is(err, want) || !strings.Contains(err.Error(), detail) || !strings.Contains(err.Error(), "nself start web") {
				t.Fatalf("want %v and recovery hint, got %v", want, err)
			}
			if s.read("upstarted") == "" || len(steps) != 1 || steps[0].Status != "failed" || s.read("ups") != "" {
				t.Fatalf("up was not interrupted or roll continued: steps=%+v", steps)
			}
		})
	}
}

func cancelRollingUpWhenStarted(ctx context.Context, s *rollingStack, cancel context.CancelFunc) {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if s.read("upstarted") != "" {
				cancel()
				return
			}
		}
	}
}
