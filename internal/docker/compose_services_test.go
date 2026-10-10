package docker

// compose_services_test.go — resolved service specs and the per-kind health
// gate. The gate runs on a fake clock and a scripted state source, so the 30 s
// and 60 s limits are asserted without sleeping.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestComposeServiceSpecs(t *testing.T) {
	raw := []byte(`{"name":"p","services":{
	  "web":{"image":"x"},
	  "db":{"restart":"unless-stopped","healthcheck":{"test":["CMD","pg_isready"],"interval":"5s"}},
	  "off":{"healthcheck":{"disable":true}},
	  "none":{"healthcheck":{"test":["NONE"]}},
	  "init":{"restart":"no"}}}`)
	got, err := parseServiceSpecs(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []ServiceSpec{
		{Name: "db", HasHealthcheck: true, Restart: "unless-stopped"},
		{Name: "init", Restart: "no"},
		{Name: "none"},
		{Name: "off"},
		{Name: "web"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("specs = %+v\nwant   %+v", got, want)
	}
	if !got[1].OneShot() || got[4].OneShot() {
		t.Error("only restart \"no\" is a one-shot; an absent restart is long-running")
	}
	if _, err := parseServiceSpecs([]byte("not json")); err == nil {
		t.Error("malformed config JSON must error, not read as zero services")
	}

	t.Run("argv carries the manifest files and env files", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("fake docker is a shell script")
		}
		dir := t.TempDir()
		log := filepath.Join(dir, "log")
		script := "#!/bin/sh\necho \"$*\" > '" + log + "'\necho '{\"services\":{\"a\":{\"restart\":\"no\"}}}'\n"
		bin := filepath.Join(dir, "docker")
		if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
			t.Fatal(err)
		}
		c := NewCompose("/m/base.yml", "/m/plugin.yml")
		c.EnvFiles = []string{"/m/.env", "/m/compose.env"}
		c.DockerPath = bin
		specs, err := ComposeServiceSpecs(context.Background(), c, dir)
		if err != nil || len(specs) != 1 || !specs[0].OneShot() {
			t.Fatalf("specs = %+v, err = %v", specs, err)
		}
		b, _ := os.ReadFile(log)
		wantArgv := "compose -f /m/base.yml -f /m/plugin.yml --env-file /m/.env --env-file /m/compose.env config --format json"
		if strings.TrimSpace(string(b)) != wantArgv {
			t.Errorf("argv = %q, want %q", strings.TrimSpace(string(b)), wantArgv)
		}
	})

	t.Run("a failing docker is an error, never an empty list", func(t *testing.T) {
		c := NewCompose("x.yml")
		c.DockerPath = filepath.Join(t.TempDir(), "no-such-docker")
		if specs, err := ComposeServiceSpecs(context.Background(), c, t.TempDir()); err == nil {
			t.Errorf("want an error, got specs %v", specs)
		}
	})
}

func TestParseContainerStates(t *testing.T) {
	arr := `[{"Name":"a","State":"exited","ExitCode":3},{"Name":"b","State":"running","Health":"healthy"}]`
	nd := `{"Name":"a","State":"exited","ExitCode":3}` + "\n" + `{"Name":"b","State":"running","Health":"healthy"}`
	for name, raw := range map[string]string{"array": arr, "ndjson": nd} {
		got, err := parseContainerStates([]byte(raw))
		if err != nil || len(got) != 2 || got[0].ExitCode != 3 || got[1].Health != "healthy" {
			t.Errorf("%s: got %+v, err %v", name, got, err)
		}
	}
	if got, err := parseContainerStates([]byte("  \n")); err != nil || got != nil {
		t.Errorf("empty output = %v, %v; want nil, nil", got, err)
	}
	if _, err := parseContainerStates([]byte("[{")); err == nil {
		t.Error("truncated JSON must error")
	}
}

// fakeClock advances only when the gate sleeps.
type fakeClock struct {
	start, now time.Time
	sleeps     int
}

func newFakeClock() *fakeClock {
	t := time.Unix(1_700_000_000, 0)
	return &fakeClock{start: t, now: t}
}
func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps++
	c.now = c.now.Add(d)
	return ctx.Err()
}
func (c *fakeClock) elapsed() time.Duration { return c.now.Sub(c.start) }

func fixed(cs ...ContainerState) StateSource {
	return func(context.Context) ([]ContainerState, error) { return cs, nil }
}

func TestHealthGateOneShot(t *testing.T) {
	spec := ServiceSpec{Name: "init", Restart: "no"}
	ctx := context.Background()

	if err := HealthGate(ctx, spec, fixed(ContainerState{Name: "i", State: "exited"}), newFakeClock()); err != nil {
		t.Errorf("exit 0 must pass: %v", err)
	}

	clk := newFakeClock()
	err := HealthGate(ctx, spec, fixed(ContainerState{Name: "i", State: "exited", ExitCode: 1}), clk)
	if !errors.Is(err, ErrOneShotFailed) || !strings.Contains(err.Error(), "init") {
		t.Fatalf("exit 1 must fail naming the service, got %v", err)
	}
	if clk.sleeps != 0 {
		t.Errorf("a non-zero exit must fail at once, slept %d times", clk.sleeps)
	}

	// Still running when polled, then it exits 0: passes after waiting.
	n := 0
	src := func(context.Context) ([]ContainerState, error) {
		n++
		if n < 3 {
			return []ContainerState{{Name: "i", State: "running"}}, nil
		}
		return []ContainerState{{Name: "i", State: "exited"}}, nil
	}
	clk = newFakeClock()
	if err := HealthGate(ctx, spec, src, clk); err != nil || clk.sleeps != 2 {
		t.Errorf("late exit 0: err %v, sleeps %d", err, clk.sleeps)
	}

	// Never exits: times out at 60 s, not the 30 s running limit.
	clk = newFakeClock()
	err = HealthGate(ctx, spec, fixed(ContainerState{Name: "i", State: "running"}), clk)
	if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < 60*time.Second {
		t.Errorf("never-exiting one-shot: err %v after %s", err, clk.elapsed())
	}

	// One replica exited 0, the other exited 1: the failure wins.
	err = HealthGate(ctx, spec, fixed(
		ContainerState{Name: "a", State: "exited"}, ContainerState{Name: "b", State: "exited", ExitCode: 2}), newFakeClock())
	if !errors.Is(err, ErrOneShotFailed) {
		t.Errorf("mixed replicas: %v", err)
	}
}

func TestHealthGateUnhealthy(t *testing.T) {
	ctx := context.Background()
	hc := ServiceSpec{Name: "pg", HasHealthcheck: true, Restart: "unless-stopped"}

	clk := newFakeClock()
	err := HealthGate(ctx, hc, fixed(ContainerState{Name: "p", State: "running", Health: "unhealthy"}), clk)
	if !errors.Is(err, ErrGateTimeout) || !strings.Contains(err.Error(), "pg") {
		t.Fatalf("stuck unhealthy must time out naming the service, got %v", err)
	}
	if got := clk.elapsed(); got < 60*time.Second || got > 62*time.Second {
		t.Errorf("healthcheck limit = %s, want 60s", got)
	}

	// running but still "starting" is not healthy yet; healthy later passes.
	n := 0
	src := func(context.Context) ([]ContainerState, error) {
		n++
		h := "starting"
		if n >= 4 {
			h = "healthy"
		}
		return []ContainerState{{Name: "p", State: "running", Health: h}}, nil
	}
	if err := HealthGate(ctx, hc, src, newFakeClock()); err != nil {
		t.Errorf("healthy after 3 polls must pass: %v", err)
	}

	// No healthcheck: running passes at once; not running times out at 30 s.
	plain := ServiceSpec{Name: "web"}
	if err := HealthGate(ctx, plain, fixed(ContainerState{Name: "w", State: "running"}), newFakeClock()); err != nil {
		t.Errorf("running without healthcheck must pass: %v", err)
	}
	clk = newFakeClock()
	err = HealthGate(ctx, plain, fixed(ContainerState{Name: "w", State: "restarting"}), clk)
	if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < 30*time.Second || clk.elapsed() > 32*time.Second {
		t.Errorf("not running: err %v after %s, want a 30s timeout", err, clk.elapsed())
	}

	// No container at all counts as not ready, and a state-read error is
	// surfaced, never read as healthy.
	if err := HealthGate(ctx, plain, fixed(), newFakeClock()); !errors.Is(err, ErrGateTimeout) {
		t.Errorf("no container: %v", err)
	}
	boom := errors.New("daemon down")
	err = HealthGate(ctx, plain, func(context.Context) ([]ContainerState, error) { return nil, boom }, newFakeClock())
	if !errors.Is(err, boom) {
		t.Errorf("state read error must surface, got %v", err)
	}

	// A cancelled context stops the poll.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := HealthGate(cctx, hc, fixed(ContainerState{Name: "p", State: "running", Health: "starting"}), SystemClock{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx: %v", err)
	}
}

// TestComposeServiceStatesArgv: a one-shot is only visible once it has exited,
// so the state read must pass -a, and it must carry the manifest and env files.
func TestComposeServiceStatesArgv(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\necho \"$*\" > '" + log + "'\necho '[{\"Name\":\"p-init-1\",\"State\":\"exited\",\"ExitCode\":0}]'\n"
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	c := NewCompose("/m/base.yml")
	c.EnvFiles = []string{"/m/compose.env"}
	c.DockerPath = bin
	cs, err := c.ComposeServiceStates(context.Background(), dir, "init")
	if err != nil || len(cs) != 1 || cs[0].State != "exited" || cs[0].ExitCode != 0 {
		t.Fatalf("states = %+v, err = %v", cs, err)
	}
	b, _ := os.ReadFile(log)
	want := "compose -f /m/base.yml --env-file /m/compose.env ps -a --format json init"
	if strings.TrimSpace(string(b)) != want {
		t.Errorf("argv = %q, want %q", strings.TrimSpace(string(b)), want)
	}
}
