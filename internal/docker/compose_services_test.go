package docker

// compose_services_test.go — resolved service specs and the per-kind health
// gate. The gate runs on a fake clock and a scripted state source, so the 30 s
// and 60 s limits are asserted without sleeping.

import (
	"context"
	"errors"
	"fmt"
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
	  "echo":{"healthcheck":{"test":["CMD","echo","NONE"]}},
	  "dbg":{"restart":"no","healthcheck":{"test":["CMD","true"]}},
	  "init":{"restart":"no"}}}`)
	got, err := parseServiceSpecs(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := []ServiceSpec{
		{Name: "db", HasHealthcheck: true, Restart: "unless-stopped", HealthTimeout: 105 * time.Second},
		{Name: "dbg", HasHealthcheck: true, Restart: "no", HealthTimeout: 180 * time.Second},
		{Name: "echo", HasHealthcheck: true, HealthTimeout: 180 * time.Second},
		{Name: "init", Restart: "no"},
		{Name: "none"},
		{Name: "off"},
		{Name: "web"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("specs = %+v\nwant   %+v", got, want)
	}
	if !got[3].OneShot() || got[6].OneShot() || got[1].OneShot() {
		t.Error("only restart \"no\" without a healthcheck is a one-shot; a healthcheck or an absent restart is long-running")
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

	// A container that has not started since the baseline is not an exit yet.
	clk = newFakeClock()
	err = HealthGate(ctx, spec, fixed(ContainerState{Name: "i", State: StateNotRestarted}), clk)
	if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < 60*time.Second {
		t.Errorf("stale exit: err %v after %s", err, clk.elapsed())
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

	// restart "no" with a healthcheck is long-running: the healthcheck gate
	// applies (healthy passes, starting times out at 60 s), never the exit rule.
	dbg := ServiceSpec{Name: "dbg", HasHealthcheck: true, Restart: "no"}
	if err := HealthGate(ctx, dbg, fixed(ContainerState{Name: "d", State: "running", Health: "healthy"}), newFakeClock()); err != nil {
		t.Errorf("healthy restart-no service with a healthcheck must pass: %v", err)
	}
	clk = newFakeClock()
	err = HealthGate(ctx, dbg, fixed(ContainerState{Name: "d", State: "running", Health: "starting"}), clk)
	if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < 60*time.Second {
		t.Errorf("starting restart-no service: err %v after %s", err, clk.elapsed())
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

	// No healthcheck: running on two polls with the same StartedAt passes (one
	// poll is not enough, see TestHealthGateRunning); a container that is not
	// running times out at 30 s.
	plain := ServiceSpec{Name: "web"}
	if err := HealthGate(ctx, plain, fixed(ContainerState{Name: "w", State: "running"}), newFakeClock()); err != nil {
		t.Errorf("running without healthcheck must pass: %v", err)
	}
	clk = newFakeClock()
	err = HealthGate(ctx, plain, fixed(ContainerState{Name: "w", State: "created"}), clk)
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
	if !errors.Is(err, boom) || !errors.Is(err, ErrStateRead) || errors.Is(err, ErrOneShotFailed) || errors.Is(err, ErrServiceCrashed) {
		t.Errorf("state read error must surface as ErrStateRead and not as a broken service, got %v", err)
	}

	// A cancelled context stops the poll.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if err := HealthGate(cctx, hc, fixed(ContainerState{Name: "p", State: "running", Health: "starting"}), SystemClock{}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled ctx: %v", err)
	}
}

// TestHealthGateRunning: the running gate (no healthcheck, not a one-shot)
// fails fast on a crash, and needs the same run on two polls (S1, round 2).
func TestHealthGateRunning(t *testing.T) {
	ctx := context.Background()
	always := ServiceSpec{Name: "web", Restart: "always"}
	unlessStopped := ServiceSpec{Name: "web", Restart: "unless-stopped"}
	noLine := ServiceSpec{Name: "web"}
	up := func(at string) ContainerState {
		return ContainerState{ID: "c1", Name: "w", State: "running", StartedAt: at}
	}

	// Fail fast, whatever the kind: no sleep, E250 sentinel.
	for name, tc := range map[string]struct {
		spec ServiceSpec
		cs   []ContainerState
		want error
	}{
		"restart always, exited 1":  {always, []ContainerState{{Name: "w", State: "exited", ExitCode: 1}}, ErrServiceCrashed},
		"restarting":                {always, []ContainerState{{Name: "w", State: "restarting"}}, ErrServiceCrashed},
		"no restart line, exited 2": {noLine, []ContainerState{{Name: "w", State: "exited", ExitCode: 2}}, ErrServiceCrashed},
		"one replica restarting":    {always, []ContainerState{up("t1"), {ID: "c2", Name: "w2", State: "restarting"}}, ErrServiceCrashed},
		"healthcheck, exited 1":     {ServiceSpec{Name: "web", HasHealthcheck: true, Restart: "always"}, []ContainerState{{Name: "w", State: "exited", ExitCode: 1}}, ErrServiceCrashed},
		"healthcheck, restarting":   {ServiceSpec{Name: "web", HasHealthcheck: true}, []ContainerState{{Name: "w", State: "restarting", Health: "starting"}}, ErrServiceCrashed},
		"one-shot, exited 1":        {ServiceSpec{Name: "web", Restart: "no"}, []ContainerState{{Name: "w", State: "exited", ExitCode: 1}}, ErrOneShotFailed},
	} {
		clk := newFakeClock()
		err := HealthGate(ctx, tc.spec, fixed(tc.cs...), clk)
		if !errors.Is(err, tc.want) || clk.sleeps != 0 {
			t.Errorf("%s: err %v, sleeps %d; want %v at once", name, err, clk.sleeps, tc.want)
		}
	}

	// One poll is not proof: the same run on a second poll passes.
	clk := newFakeClock()
	if err := HealthGate(ctx, unlessStopped, fixed(up("t1")), clk); err != nil || clk.sleeps != 1 {
		t.Errorf("stable run: err %v, sleeps %d, want pass after exactly one wait", err, clk.sleeps)
	}
	// A different StartedAt on the second poll is a restart in between: keep
	// waiting, and give up at 30 s (not 60 s) if it never settles.
	n := 0
	flapping := func(context.Context) ([]ContainerState, error) {
		n++
		return []ContainerState{up(fmt.Sprintf("t%d", n))}, nil
	}
	clk = newFakeClock()
	err := HealthGate(ctx, unlessStopped, flapping, clk)
	if !errors.Is(err, ErrGateTimeout) || clk.elapsed() < 30*time.Second || clk.elapsed() > 32*time.Second {
		t.Errorf("never-settling service: err %v after %s, want a 30s timeout", err, clk.elapsed())
	}
	// It settles on the third poll: passes on the fourth.
	n = 0
	settling := func(context.Context) ([]ContainerState, error) {
		n++
		return []ContainerState{up(fmt.Sprintf("t%d", min(n, 3)))}, nil
	}
	clk = newFakeClock()
	if err := HealthGate(ctx, unlessStopped, settling, clk); err != nil || clk.sleeps != 3 {
		t.Errorf("settling service: err %v, sleeps %d", err, clk.sleeps)
	}
	// A state source without StartedAt (a remote host) still gets two polls.
	clk = newFakeClock()
	if err := HealthGate(ctx, unlessStopped, fixed(ContainerState{Name: "w", State: "running"}), clk); err != nil || clk.sleeps != 1 {
		t.Errorf("no StartedAt: err %v, sleeps %d", err, clk.sleeps)
	}

	// No restart line plus exit 0 is a finished run (Docker's default policy): done.
	clk = newFakeClock()
	if err := HealthGate(ctx, noLine, fixed(ContainerState{Name: "w", State: "exited"}), clk); err != nil || clk.sleeps != 0 {
		t.Errorf("no restart line, exit 0: err %v, sleeps %d, want done at once", err, clk.sleeps)
	}
	// With a restart policy, exit 0 means it is about to come back: not done.
	clk = newFakeClock()
	err = HealthGate(ctx, unlessStopped, fixed(ContainerState{Name: "w", State: "exited"}), clk)
	if !errors.Is(err, ErrGateTimeout) {
		t.Errorf("unless-stopped, exit 0: %v", err)
	}
	// A stale exit (not restarted since the baseline) is not a finished run.
	clk = newFakeClock()
	err = HealthGate(ctx, noLine, fixed(ContainerState{Name: "w", State: StateNotRestarted}), clk)
	if !errors.Is(err, ErrGateTimeout) {
		t.Errorf("stale exit: %v", err)
	}
}

// TestHealthGateInterrupt: a cancelled context comes back as the context's own
// error, also when the cancellation killed the state read.
func TestHealthGateInterrupt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	src := func(c context.Context) ([]ContainerState, error) {
		cancel() // ctrl-c lands while docker is being asked
		return nil, errors.New("signal: killed")
	}
	err := HealthGate(ctx, ServiceSpec{Name: "web"}, src, newFakeClock())
	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrStateRead) {
		t.Errorf("interrupted read must be context.Canceled, not a state-read failure: %v", err)
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
	want := "compose -f /m/base.yml --env-file /m/compose.env ps -a --format json -- init"
	if strings.TrimSpace(string(b)) != want {
		t.Errorf("argv = %q, want %q", strings.TrimSpace(string(b)), want)
	}
}

// TestFreshStates: an exited container whose StartedAt equals the pre-up mark
// is reported as not restarted; once StartedAt changes it is a real exit.
func TestFreshStates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = inspect ]; then cat '" + filepath.Join(dir, "started") + "' || exit 1; exit 0; fi\n" +
		"echo '[{\"ID\":\"c1\",\"Name\":\"p-init-1\",\"State\":\"exited\",\"ExitCode\":0},{\"ID\":\"c2\",\"Name\":\"p-init-2\",\"State\":\"running\"}]'\n"
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	setStarted := func(v string) {
		if err := os.WriteFile(filepath.Join(dir, "started"), []byte(v+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := NewCompose("/m/base.yml")
	c.DockerPath = bin
	ctx := context.Background()

	setStarted("t0")
	marks, err := c.StartMarks(ctx, dir, "init")
	if err != nil || !reflect.DeepEqual(marks, map[string]string{"c1": "t0"}) {
		t.Fatalf("marks = %v, err = %v (only exited containers are marked)", marks, err)
	}
	cs, err := c.FreshStates(ctx, dir, "init", marks)
	if err != nil || cs[0].State != StateNotRestarted || cs[1].State != "running" || cs[1].StartedAt != "t0" {
		t.Fatalf("same StartedAt: states %+v, err %v", cs, err)
	}
	setStarted("t1")
	cs, err = c.FreshStates(ctx, dir, "init", marks)
	if err != nil || cs[0].State != "exited" {
		t.Fatalf("new StartedAt: states %+v, err %v", cs, err)
	}
	// A new container (no mark) is never masked.
	cs, err = c.FreshStates(ctx, dir, "init", map[string]string{})
	if err != nil || cs[0].State != "exited" {
		t.Fatalf("unmarked container: states %+v, err %v", cs, err)
	}
	// A failing inspect is an error, not a silent pass.
	if err := os.Remove(filepath.Join(dir, "started")); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FreshStates(ctx, dir, "init", marks); err == nil {
		t.Error("a failed docker inspect must surface")
	}
}
