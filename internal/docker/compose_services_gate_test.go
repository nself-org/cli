package docker

// compose_services_gate_test.go — bounded docker reads (D-0373) and the
// per-service healthcheck timing (D-0314): a hung daemon is cut off and named,
// and a slow healthcheck gets its declared time.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// shortenStateBounds makes the docker read bounds test-sized.
func shortenStateBounds(t *testing.T) {
	t.Helper()
	oldT, oldW := stateCallTimeout, stateWaitDelay
	stateCallTimeout, stateWaitDelay = 200*time.Millisecond, 200*time.Millisecond
	t.Cleanup(func() { stateCallTimeout, stateWaitDelay = oldT, oldW })
}

// hangingDocker is a docker stub whose "hang" subcommand (the first argument
// after any compose flags is not inspected: $1 is "inspect" or "compose")
// sleeps in a CHILD process, as the compose plugin does under the docker CLI.
func hangingDocker(t *testing.T, hangInspect bool) *Compose {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n"
	if hangInspect {
		script += "if [ \"$1\" = inspect ]; then sleep 30; exit 0; fi\n" +
			"echo '[{\"ID\":\"c1\",\"Name\":\"p-init-1\",\"State\":\"exited\"}]'\n"
	} else {
		script += "sleep 30\nexit 0\n"
	}
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	c := NewCompose("/m/base.yml")
	c.DockerPath = bin
	return c
}

func TestStateReadsAreBounded(t *testing.T) {
	shortenStateBounds(t)
	ctx := context.Background()
	for name, tc := range map[string]struct {
		hangInspect bool
		run         func(c *Compose) error
		want        string
	}{
		"ps":                      {false, func(c *Compose) error { _, err := c.ComposeServiceStates(ctx, t.TempDir(), "init"); return err }, "docker compose ps for init"},
		"inspect via StartMarks":  {true, func(c *Compose) error { _, err := c.StartMarks(ctx, t.TempDir(), "init"); return err }, "docker inspect c1"},
		"inspect via FreshStates": {true, func(c *Compose) error { _, err := c.FreshStates(ctx, t.TempDir(), "init", nil); return err }, "docker inspect c1"},
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			err := tc.run(hangingDocker(t, tc.hangInspect))
			if took := time.Since(start); took > 3*time.Second {
				t.Errorf("took %s, want the call cut off near %s", took, stateCallTimeout)
			}
			if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "200ms") {
				t.Errorf("err = %v, want a DeadlineExceeded naming %q and the limit", err, tc.want)
			}
		})
	}
}

func TestStateReadParentCancelStaysCancel(t *testing.T) {
	shortenStateBounds(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := hangingDocker(t, false).ComposeServiceStates(ctx, t.TempDir(), "init")
	if err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a cancelled parent must not read as the call's own timeout: %v", err)
	}
}

// clockJump reads start once, then start+d forever: the gate's deadline is
// already past when it sizes the first read.
type clockJump struct {
	start time.Time
	d     time.Duration
	calls int
}

func (c *clockJump) Now() time.Time {
	c.calls++
	if c.calls == 1 {
		return c.start
	}
	return c.start.Add(c.d)
}
func (c *clockJump) Sleep(ctx context.Context, _ time.Duration) error { return ctx.Err() }

func TestHealthGateBoundsEachRead(t *testing.T) {
	old := gateReadGrace
	gateReadGrace = 50 * time.Millisecond
	t.Cleanup(func() { gateReadGrace = old })
	spec := ServiceSpec{Name: "web"}

	t.Run("a hung read is a gate timeout, not a state-read failure", func(t *testing.T) {
		hung := func(ctx context.Context) ([]ContainerState, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		clk := &clockJump{start: time.Unix(1_700_000_000, 0), d: RunningTimeout + time.Second}
		begin := time.Now()
		err := HealthGate(context.Background(), spec, hung, clk)
		if !errors.Is(err, ErrGateTimeout) || errors.Is(err, ErrStateRead) || !strings.Contains(err.Error(), "web") {
			t.Fatalf("err = %v, want ErrGateTimeout naming the service", err)
		}
		if took := time.Since(begin); took > 2*time.Second {
			t.Errorf("took %s, want the read cut off near the grace", took)
		}
	})

	t.Run("a deadline error on a live ctx is a gate timeout", func(t *testing.T) {
		src := func(context.Context) ([]ContainerState, error) {
			return nil, errors.Join(errors.New("x"), context.DeadlineExceeded)
		}
		err := HealthGate(context.Background(), spec, src, newFakeClock())
		if !errors.Is(err, ErrGateTimeout) || errors.Is(err, ErrStateRead) {
			t.Errorf("err = %v, want ErrGateTimeout", err)
		}
	})

	t.Run("a late healthy read after the deadline still counts", func(t *testing.T) {
		clk := &clockJump{start: time.Unix(1_700_000_000, 0), d: time.Hour}
		src := fixed(ContainerState{ID: "c", Name: "w", State: "running", Health: "healthy"})
		hc := ServiceSpec{Name: "web", HasHealthcheck: true}
		if err := HealthGate(context.Background(), hc, src, clk); err != nil {
			t.Errorf("late healthy read: %v", err)
		}
	})
}

func TestHealthGateHealthTimeout(t *testing.T) {
	starting := fixed(ContainerState{Name: "p", State: "running", Health: "starting"})
	for name, tc := range map[string]struct {
		spec ServiceSpec
		want time.Duration
	}{
		"slow healthcheck keeps polling to its declared time": {ServiceSpec{Name: "pg", HasHealthcheck: true, HealthTimeout: 3 * time.Minute}, 3 * time.Minute},
		"short declared time keeps HealthyTimeout":            {ServiceSpec{Name: "pg", HasHealthcheck: true, HealthTimeout: 10 * time.Second}, HealthyTimeout},
		"unknown (zero) keeps HealthyTimeout":                 {ServiceSpec{Name: "pg", HasHealthcheck: true}, HealthyTimeout},
		"no healthcheck ignores it":                           {ServiceSpec{Name: "pg", HealthTimeout: 3 * time.Minute}, RunningTimeout},
	} {
		t.Run(name, func(t *testing.T) {
			clk := newFakeClock()
			src := starting
			if !tc.spec.HasHealthcheck {
				src = fixed(ContainerState{Name: "p", State: "created"})
			}
			err := HealthGate(context.Background(), tc.spec, src, clk)
			if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < tc.want || clk.elapsed() > tc.want+3*time.Second {
				t.Errorf("err %v after %s, want a timeout at %s", err, clk.elapsed(), tc.want)
			}
		})
	}
}

func TestParseServiceSpecsHealthTiming(t *testing.T) {
	hcOf := func(hc string) string { return `{"services":{"s":{"healthcheck":` + hc + `}}}` }
	for name, tc := range map[string]struct {
		hc   string
		has  bool
		want time.Duration
	}{
		"declared timing":                 {`{"test":["CMD","true"],"interval":"1m30s","timeout":"5s","start_period":"2m0s","retries":4}`, true, 2*time.Minute + 4*(95*time.Second)},
		"test only: defaults":             {`{"test":["CMD","true"]}`, true, 3 * (30*time.Second + 30*time.Second)},
		"partial: interval":               {`{"test":["CMD","true"],"interval":"10s"}`, true, 3 * (10*time.Second + 30*time.Second)},
		"one retry, start_period":         {`{"test":["CMD","true"],"retries":1,"start_period":"5s"}`, true, 65 * time.Second},
		"retries 0 is Docker's default 3": {`{"test":["CMD","true"],"retries":0}`, true, 180 * time.Second},
		"interval 0s is the default 30s":  {`{"test":["CMD","true"],"interval":"0s","timeout":"10s"}`, true, 3 * 40 * time.Second},
		"disabled":                        {`{"disable":true,"interval":"5m"}`, false, 0},
		"NONE":                            {`{"test":["NONE"],"interval":"5m"}`, false, 0},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := parseServiceSpecs([]byte(hcOf(tc.hc)))
			if err != nil || len(got) != 1 || got[0].HasHealthcheck != tc.has || got[0].HealthTimeout != tc.want {
				t.Errorf("got %+v, err %v; want has=%v HealthTimeout=%s", got, err, tc.has, tc.want)
			}
		})
	}
	for _, field := range []string{"interval", "timeout", "start_period"} {
		raw := hcOf(`{"test":["CMD","true"],"` + field + `":"soon"}`)
		if _, err := parseServiceSpecs([]byte(raw)); err == nil || !strings.Contains(err.Error(), field) {
			t.Errorf("bad %s duration must error naming the field, got %v", field, err)
		}
	}
}
