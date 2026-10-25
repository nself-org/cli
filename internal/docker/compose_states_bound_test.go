package docker

// compose_states_bound_test.go — review round 2: cancellation vs the read's own
// timeout, soft cancel, vanished containers, healthcheck value parsing.

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

// scriptDocker writes body as the docker stub and returns a Compose using it.
func scriptDocker(t *testing.T, body string) (*Compose, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+body), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	c := NewCompose("/m/base.yml")
	c.DockerPath = bin
	return c, dir
}

func TestStateReadParentDeadlineIsNotOwnTimeout(t *testing.T) {
	shortenStateBounds(t)
	stateCallTimeout = 10 * time.Second // the parent's 150ms fires first
	c, dir := scriptDocker(t, "sleep 30\nexit 0\n")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := c.ComposeServiceStates(ctx, dir, "init")
	if !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrStateTimeout) || strings.Contains(err.Error(), "gave no answer") {
		t.Errorf("err = %v, want the parent's DeadlineExceeded and no \"gave no answer\"", err)
	}
}

func TestSoftCancelSendsTerm(t *testing.T) {
	shortenStateBounds(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "term")
	c, _ := scriptDocker(t, "trap 'echo term > '"+marker+"'; exit 0' TERM\nsleep 30 &\nwait\n")
	if _, err := c.ComposeServiceStates(context.Background(), dir, "init"); !errors.Is(err, ErrStateTimeout) {
		t.Fatalf("err = %v, want ErrStateTimeout", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("docker never got SIGTERM: %v", err)
	}
}

func TestCleanExitWithHeldPipeIsSuccess(t *testing.T) {
	shortenStateBounds(t)
	stateCallTimeout = 10 * time.Second
	c, dir := scriptDocker(t, "sleep 30 &\necho '[{\"ID\":\"c1\",\"Name\":\"p-a-1\",\"State\":\"running\"}]'\nexit 0\n")
	begin := time.Now()
	cs, err := c.ComposeServiceStates(context.Background(), dir, "a")
	if err != nil || len(cs) != 1 || time.Since(begin) > 3*time.Second {
		t.Errorf("states %+v, err %v after %s; want the output of a clean exit", cs, err, time.Since(begin))
	}
}

func TestFreshStatesDropsVanishedContainer(t *testing.T) {
	ps := "echo '[{\"ID\":\"c1\",\"Name\":\"p-a-1\",\"State\":\"running\"},{\"ID\":\"c2\",\"Name\":\"p-a-2\",\"State\":\"running\"}]'\n"
	for name, tc := range map[string]struct {
		stderr  string
		wantLen int
		wantErr bool
	}{
		"no such object":    {"Error: No such object: c2", 1, false},
		"no such container": {"Error: No such container: c2", 1, false},
		"other failure":     {"Cannot connect to the Docker daemon", 0, true},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := scriptDocker(t, "if [ \"$1\" = inspect ]; then\n if [ \"$5\" = c2 ]; then echo '"+tc.stderr+"' >&2; exit 1; fi\n echo t1; exit 0; fi\n"+ps)
			cs, err := c.FreshStates(context.Background(), dir, "a", nil)
			if (err != nil) != tc.wantErr || len(cs) != tc.wantLen || (!tc.wantErr && (cs[0].ID != "c1" || cs[0].StartedAt != "t1")) {
				t.Errorf("states %+v, err %v", cs, err)
			}
		})
	}
}

func TestStartMarksDropsVanishedContainer(t *testing.T) {
	ps := "echo '[{\"ID\":\"c1\",\"Name\":\"p-a-1\",\"State\":\"exited\"},{\"ID\":\"c2\",\"Name\":\"p-a-2\",\"State\":\"exited\"},{\"ID\":\"c3\",\"Name\":\"p-a-3\",\"State\":\"running\"}]'\n"
	for name, tc := range map[string]struct {
		stderr  string
		want    map[string]string
		wantErr bool
	}{
		"no such object":    {"Error: No such object: c2", map[string]string{"c1": "t1"}, false},
		"no such container": {"Error: No such container: c2", map[string]string{"c1": "t1"}, false},
		"other failure":     {"Cannot connect to the Docker daemon", nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := scriptDocker(t, "if [ \"$1\" = inspect ]; then\n if [ \"$5\" = c2 ]; then echo '"+tc.stderr+"' >&2; exit 1; fi\n echo t1; exit 0; fi\n"+ps)
			marks, err := c.StartMarks(context.Background(), dir, "a")
			if (err != nil) != tc.wantErr || len(marks) != len(tc.want) || (!tc.wantErr && marks["c1"] != "t1") {
				t.Errorf("marks %+v, err %v; want %+v, err %v", marks, err, tc.want, tc.wantErr)
			}
		})
	}
}

func TestHealthGateCancelAndKilledRead(t *testing.T) {
	old := gateReadGrace
	gateReadGrace = 50 * time.Millisecond
	t.Cleanup(func() { gateReadGrace = old })
	spec := ServiceSpec{Name: "web"}

	// S1: cancelled while the read was in flight, healthy rows returned.
	ctx, cancel := context.WithCancel(context.Background())
	src := func(context.Context) ([]ContainerState, error) {
		cancel()
		return []ContainerState{{ID: "c", Name: "w", State: "exited"}}, nil
	}
	if err := HealthGate(ctx, spec, src, newFakeClock()); !errors.Is(err, context.Canceled) {
		t.Errorf("cancel during a successful read: %v, want context.Canceled", err)
	}

	// S9: a killed child's plain error, no DeadlineExceeded in the chain: the
	// cut read alone maps it to ErrGateTimeout.
	killed := func(c context.Context) ([]ContainerState, error) {
		<-c.Done()
		return nil, errors.New("signal: killed")
	}
	clk := &clockJump{start: time.Unix(1_700_000_000, 0), d: RunningTimeout + time.Second}
	err := HealthGate(context.Background(), spec, killed, clk)
	if !errors.Is(err, ErrGateTimeout) || errors.Is(err, ErrStateRead) {
		t.Errorf("killed read: %v, want ErrGateTimeout", err)
	}

	// S2: a dead container fails at once, like exited non-zero.
	dead := fixed(ContainerState{Name: "w", State: "dead"})
	if err := HealthGate(context.Background(), spec, dead, newFakeClock()); !errors.Is(err, ErrServiceCrashed) || !strings.Contains(err.Error(), "w is dead") {
		t.Errorf("dead: %v", err)
	}
	if err := HealthGate(context.Background(), ServiceSpec{Name: "i", Restart: "no"}, dead, newFakeClock()); !errors.Is(err, ErrOneShotFailed) {
		t.Errorf("dead one-shot: %v", err)
	}
}

func TestParseHealthValues(t *testing.T) {
	hc := func(body string) string {
		return `{"services":{"s":{"healthcheck":{"test":["CMD","true"],` + body + `}}}}`
	}
	ok := map[string]struct {
		body string
		want time.Duration
	}{
		"huge retries cap at 10m":         {`"retries":1000000000`, MaxHealthTimeout},
		"retries beyond int32 still cap":  {`"retries":9000000000000`, MaxHealthTimeout},
		"2m + 1m30s x3 caps from 10m30s":  {`"interval":"2m0s","timeout":"1m30s","retries":3`, MaxHealthTimeout},
		"huge interval caps":              {`"interval":"100000h"`, MaxHealthTimeout},
		"numeric nanoseconds":             {`"interval":10000000000,"timeout":20000000000,"retries":2`, 60 * time.Second},
		"numeric start_period":            {`"start_period":5000000000,"retries":1`, 65 * time.Second},
		"null values take defaults":       {`"interval":null,"retries":null`, 180 * time.Second},
		"zero interval number is default": {`"interval":0,"timeout":"10s"`, 120 * time.Second},
	}
	for name, tc := range ok {
		t.Run(name, func(t *testing.T) {
			got, err := ParseServiceSpecs([]byte(hc(tc.body)))
			if err != nil || len(got) != 1 || got[0].HealthTimeout != tc.want {
				t.Errorf("got %+v, err %v; want %s", got, err, tc.want)
			}
		})
	}
	for name, tc := range map[string]struct{ body, field string }{
		"interval is a bool":      {`"interval":true`, "interval"},
		"timeout is an object":    {`"timeout":{"a":1}`, "timeout"},
		"start_period fractional": {`"start_period":1.5`, "start_period"},
		"retries is a string":     {`"retries":"3"`, "retries"},
		"retries fractional":      {`"retries":2.5`, "retries"},
		"retries overflows int64": {`"retries":99999999999999999999`, "retries"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseServiceSpecs([]byte(hc(tc.body)))
			if err == nil || !strings.Contains(err.Error(), "service s") || !strings.Contains(err.Error(), tc.field) {
				t.Errorf("err = %v, want one naming service s and %s", err, tc.field)
			}
		})
	}
}

// TestConfigReadsAreBounded: ComposeServiceNames and ComposeServiceSpecs
// (config reads through captureBounded) give up on a hung docker.
func TestConfigReadsAreBounded(t *testing.T) {
	shortenStateBounds(t)
	for name, run := range map[string]func(c *Compose, dir string) error{
		"ComposeServiceNames": func(c *Compose, dir string) error {
			_, err := c.ComposeServiceNames(context.Background(), dir)
			return err
		},
		"ComposeServiceSpecs": func(c *Compose, dir string) error {
			_, err := ComposeServiceSpecs(context.Background(), c, dir)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			c, dir := scriptDocker(t, "sleep 30\nexit 0\n")
			begin := time.Now()
			err := run(c, dir)
			if took := time.Since(begin); took > 3*time.Second {
				t.Errorf("took %s, want the read cut off near %s", took, stateCallTimeout)
			}
			if !errors.Is(err, ErrStateTimeout) || errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("err = %v, want ErrStateTimeout and not DeadlineExceeded", err)
			}
		})
	}
}
