package commands

// deploy_strategies_test.go — the rolling restart goes through the compose
// manifest and env files and gates every service by kind (D17, D-0041).
// A fake docker on PATH records each call; nothing here needs a daemon.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// rollingFakeDocker answers the four docker compose calls the restart makes.
// Fixtures live next to the log: services (config --services), config.json,
// ps.<svc>.json, psfail (any `ps` exits 1), upfail (a service name whose `up` exits 1) and startfixed
// (a constant `docker inspect` StartedAt, i.e. the container never restarted).
const rollingFakeDocker = `#!/bin/sh
D="$FAKE_DOCKER_DIR"
if [ "$1" = "inspect" ]; then
  # StartedAt of a container: fixed (startfixed) or one value per completed up.
  if [ -f "$D/startfixed" ]; then cat "$D/startfixed"; else echo "start-$(cat "$D/ups" 2>/dev/null | wc -l | tr -d ' ')"; fi
  exit 0
fi
echo "$*" >> "$D/log"
shift
while [ "$1" = "-f" ] || [ "$1" = "--env-file" ]; do shift; shift; done
case "$1" in
config)
  if [ "$2" = "--services" ]; then cat "$D/services"; else cat "$D/config.json"; fi ;;
up)
  for last; do :; done
  if [ -f "$D/upfail" ] && [ "$(cat "$D/upfail")" = "$last" ]; then echo "boom" >&2; exit 1; fi
  echo "$last" >> "$D/ups" ;;
ps)
  if [ -f "$D/psfail" ]; then echo "Cannot connect to the Docker daemon" >&2; exit 1; fi
  for last; do :; done
  # psfaillate holds "<svc> <n>": once n ups have run, ps of that service fails.
  if [ -f "$D/psfaillate" ] && [ "$last" = "$(cut -d' ' -f1 "$D/psfaillate")" ] && [ "$(wc -l < "$D/ups" 2>/dev/null | tr -d ' ')" -ge "$(cut -d' ' -f2 "$D/psfaillate")" ]; then
    echo "Cannot connect to the Docker daemon" >&2; exit 1
  fi
  # late.<svc> holds a count of ups: once that many have run, serve ps.<svc>.late.json (the final re-check).
  if [ -f "$D/late.$last" ] && [ "$(wc -l < "$D/ups" 2>/dev/null | tr -d ' ')" -ge "$(cat "$D/late.$last")" ]; then
    cat "$D/ps.$last.late.json"
  else
    cat "$D/ps.$last.json" 2>/dev/null || true
  fi ;;
esac
exit 0
`

// rollingStack is a workdir with a manifest, env files and a fake docker.
type rollingStack struct {
	dir, fix string
	base     string
	plugin   string
}

// newRollingStack lays out base + plugin fragment, compose.env and .env, and puts
// the fake docker first on PATH.
func newRollingStack(t *testing.T, services []string, configJSON string) *rollingStack {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("needs a fake docker shell script on PATH, which Windows cannot execute")
	}
	s := &rollingStack{dir: t.TempDir(), fix: t.TempDir()}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(rollingFakeDocker), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_DIR", s.fix)
	write := func(path, body string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.base = filepath.Join(s.dir, "docker-compose.yml")
	s.plugin = filepath.Join(s.dir, ".nself", "plugins", "ntask.yml")
	write(s.base, "services:\n  postgres: {image: postgres}\n  web: {image: web}\n")
	write(s.plugin, "services:\n  ntask: {image: ntask}\n  web: {image: web}\n")
	write(filepath.Join(s.dir, ".nself", "compose-files.txt"), s.base+"\n"+s.plugin+"\n")
	write(filepath.Join(s.dir, ".env"), "ROOT=1\n")
	write(filepath.Join(s.dir, ".nself", "compose.env"), "PLUGIN_VAR=computed\n")
	write(filepath.Join(s.fix, "services"), strings.Join(services, "\n")+"\n")
	write(filepath.Join(s.fix, "config.json"), configJSON)
	return s
}

func (s *rollingStack) setPs(t *testing.T, svc, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.fix, "ps."+svc+".json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s *rollingStack) read(name string) string {
	b, _ := os.ReadFile(filepath.Join(s.fix, name))
	return string(b)
}

// stepClock is a fake docker.Clock: Sleep advances time and counts calls.
type stepClock struct {
	now    time.Time
	sleeps int
}

func (c *stepClock) Now() time.Time { return c.now }
func (c *stepClock) Sleep(ctx context.Context, d time.Duration) error {
	c.sleeps++
	c.now = c.now.Add(d)
	return ctx.Err()
}

const psRunning = `[{"Name":"p-web-1","State":"running","Health":""}]`

const stackConfig = `{"services":{
 "postgres":{"healthcheck":{"test":["CMD","pg_isready"]}},
 "web":{},
 "ntask":{},
 "init":{"restart":"no"}}}`

// runPlan runs rollingRestart for the stack with an injected clock.
func runPlan(t *testing.T, s *rollingStack, clk docker.Clock) ([]deployStep, error) {
	t.Helper()
	compose, files, err := deployCompose(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	order, err := projectServiceOrder(context.Background(), compose, s.dir, files)
	if err != nil {
		t.Fatal(err)
	}
	specs, err := docker.ComposeServiceSpecs(context.Background(), compose, s.dir)
	if err != nil {
		t.Fatal(err)
	}
	return rollingRestart(context.Background(), rollingPlan{compose: compose, workdir: s.dir, order: order, specs: specs, clock: clk, jsonOut: true})
}

func TestDeployStrategyManifest(t *testing.T) {
	t.Run("manifest and env files reach every call, order from the manifest, plugin last", func(t *testing.T) {
		s := newRollingStack(t, []string{"ntask", "web", "postgres"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "web", psRunning)
		s.setPs(t, "ntask", psRunning)
		steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
		if err != nil {
			t.Fatalf("rolling restart: %v", err)
		}
		if got, want := strings.Fields(s.read("ups")), []string{"postgres", "web", "ntask"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("restart order = %v, want %v", got, want)
		}
		for _, st := range steps {
			if st.Status != "done" {
				t.Errorf("step %+v not done", st)
			}
		}
		wantFiles := "-f " + s.base + " -f " + s.plugin
		wantEnv := "--env-file " + filepath.Join(s.dir, ".env") + " --env-file " + filepath.Join(s.dir, ".nself", "compose.env")
		lines := strings.Split(strings.TrimSpace(s.read("log")), "\n")
		if len(lines) < 8 {
			t.Fatalf("expected config x2 + up/ps per service, got %d calls:\n%s", len(lines), s.read("log"))
		}
		for _, l := range lines {
			if !strings.Contains(l, wantFiles) || !strings.Contains(l, wantEnv) {
				t.Errorf("call lacks manifest or env files: %q", l)
			}
		}
	})

	t.Run("a one-shot that exits 1 fails with E250 and no later service is restarted", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "init", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "init", `[{"ID":"c1","Name":"p-init-1","State":"exited","ExitCode":1}]`)
		s.setPs(t, "web", psRunning)
		steps, err := runRollingRestart(context.Background(), s.dir, true)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E250" {
			t.Fatalf("want E250, got %v", err)
		}
		if !strings.Contains(err.Error(), "init") {
			t.Errorf("error must name the service: %v", err)
		}
		if strings.Contains(s.read("ups"), "web") {
			t.Errorf("web was restarted after init failed: %q", s.read("ups"))
		}
		if last := steps[len(steps)-1]; last.Name != "Restart init" || last.Status != "failed" {
			t.Errorf("last step = %+v", last)
		}
	})

	t.Run("a one-shot that exits 0 passes", func(t *testing.T) {
		s := newRollingStack(t, []string{"init"}, stackConfig)
		s.setPs(t, "init", `[{"ID":"c1","Name":"p-init-1","State":"exited","ExitCode":0}]`)
		if _, err := runRollingRestart(context.Background(), s.dir, true); err != nil {
			t.Fatalf("exit 0 one-shot must pass: %v", err)
		}
	})

	t.Run("an exit 0 from before the restart does not pass the one-shot gate", func(t *testing.T) {
		s := newRollingStack(t, []string{"init"}, stackConfig)
		s.setPs(t, "init", `[{"ID":"c1","Name":"p-init-1","State":"exited","ExitCode":0}]`)
		if err := os.WriteFile(filepath.Join(s.fix, "startfixed"), []byte("2026-10-10T00:00:00Z"), 0o600); err != nil {
			t.Fatal(err)
		}
		clk := &stepClock{now: time.Unix(0, 0)}
		steps, err := runPlan(t, s, clk)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E251" {
			t.Fatalf("a container that never restarted must time out with E251, got %v", err)
		}
		if got := clk.now.Sub(time.Unix(0, 0)); got < 60*time.Second {
			t.Errorf("gave up after %s, want at least 60s", got)
		}
		if last := steps[len(steps)-1]; last.Status != "unhealthy" {
			t.Errorf("last step = %+v", last)
		}
	})

	t.Run("a service missing from the resolved config is E252 before anything is restarted", func(t *testing.T) {
		// ghost comes after postgres: the spec check must run over the whole order first.
		s := newRollingStack(t, []string{"postgres", "ghost"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E252" || !strings.Contains(err.Error(), "ghost") {
			t.Fatalf("want E252 naming ghost, got %v", err)
		}
		if got := s.read("ups"); got != "" {
			t.Errorf("services were restarted before the spec check failed: %q", got)
		}
		if last := steps[len(steps)-1]; last.Name != "Restart ghost" || last.Status != "failed" {
			t.Errorf("last step = %+v", last)
		}
	})

	t.Run("a long-running service with restart no and a healthcheck uses the healthcheck gate", func(t *testing.T) {
		cfg := `{"services":{"dbg":{"restart":"no","healthcheck":{"test":["CMD","true"]}}}}`
		s := newRollingStack(t, []string{"dbg"}, cfg)
		s.setPs(t, "dbg", `[{"ID":"c2","Name":"p-dbg-1","State":"running","Health":"healthy"}]`)
		if _, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)}); err != nil {
			t.Fatalf("healthy restart-no service with a healthcheck must pass: %v", err)
		}
	})

	t.Run("an unhealthy service fails with E251 after 60s on the fake clock", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"unhealthy"}]`)
		s.setPs(t, "web", psRunning)
		clk := &stepClock{now: time.Unix(0, 0)}
		steps, err := runPlan(t, s, clk)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E251" {
			t.Fatalf("want E251, got %v", err)
		}
		if got := clk.now.Sub(time.Unix(0, 0)); got < 60*time.Second {
			t.Errorf("gave up after %s, want at least 60s", got)
		}
		if last := steps[len(steps)-1]; last.Status != "unhealthy" {
			t.Errorf("last step = %+v", last)
		}
		if strings.Contains(s.read("ups"), "web") {
			t.Errorf("web was restarted after postgres stayed unhealthy")
		}
	})

	t.Run("a failed up names the service and stops", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		if err := os.WriteFile(filepath.Join(s.fix, "upfail"), []byte("web"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := runRollingRestart(context.Background(), s.dir, true)
		if err == nil || !strings.Contains(err.Error(), "service web restart failed") {
			t.Fatalf("want a web restart failure, got %v", err)
		}
	})

	t.Run("never restarts a service the compose does not report", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "web", psRunning)
		if _, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(s.read("ups"), "storage") {
			t.Errorf("restarted a name no compose reported: %q", s.read("ups"))
		}
	})

	t.Run("a crash-looping service fails with E250 at once instead of passing a poll", func(t *testing.T) {
		for name, ps := range map[string]string{
			"restarting":  `[{"ID":"c3","Name":"p-web-1","State":"restarting","ExitCode":1}]`,
			"exited 1":    `[{"ID":"c3","Name":"p-web-1","State":"exited","ExitCode":1}]`,
			"one replica": `[{"ID":"c3","Name":"p-web-1","State":"running"},{"ID":"c4","Name":"p-web-2","State":"restarting"}]`,
			"healthcheck": `[{"ID":"c3","Name":"p-pg-1","State":"restarting","Health":"starting"}]`,
		} {
			svc := "web"
			if name == "healthcheck" {
				svc = "postgres"
			}
			s := newRollingStack(t, []string{svc, "ntask"}, stackConfig)
			s.setPs(t, svc, ps)
			clk := &stepClock{now: time.Unix(0, 0)}
			steps, err := runPlan(t, s, clk)
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E250" || !strings.Contains(err.Error(), svc) {
				t.Fatalf("%s: want E250 naming %s, got %v", name, svc, err)
			}
			if clk.sleeps != 0 || strings.Contains(s.read("ups"), "ntask") {
				t.Errorf("%s: must fail at once and stop the roll (sleeps %d, ups %q)", name, clk.sleeps, s.read("ups"))
			}
			if last := steps[len(steps)-1]; last.Status != "failed" {
				t.Errorf("%s: last step = %+v", name, last)
			}
		}
	})

	t.Run("a service with no restart line that exited 0 is done", func(t *testing.T) {
		s := newRollingStack(t, []string{"web"}, stackConfig)
		s.setPs(t, "web", `[{"ID":"c3","Name":"p-web-1","State":"exited","ExitCode":0}]`)
		clk := &stepClock{now: time.Unix(0, 0)}
		if _, err := runPlan(t, s, clk); err != nil || clk.sleeps != 0 {
			t.Fatalf("exit 0 without a restart line must pass at once: %v (sleeps %d)", err, clk.sleeps)
		}
	})

	t.Run("an unreadable docker state is its own error, not E250", func(t *testing.T) {
		s := newRollingStack(t, []string{"web"}, stackConfig)
		if err := os.WriteFile(filepath.Join(s.fix, "psfail"), []byte("1"), 0o600); err != nil {
			t.Fatal(err)
		}
		steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
		var ce *errs.CLIError
		if err == nil || errors.As(err, &ce) || !errors.Is(err, docker.ErrStateRead) || !strings.Contains(err.Error(), "Cannot connect") {
			t.Fatalf("want an uncoded state-read error carrying docker's message, got %v", err)
		}
		if last := steps[len(steps)-1]; last.Status != "failed" {
			t.Errorf("last step = %+v", last)
		}
	})

	t.Run("an interrupt passes through as context.Canceled, not E250", func(t *testing.T) {
		s := newRollingStack(t, []string{"web"}, stackConfig)
		s.setPs(t, "web", psRunning)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		compose, files, _ := deployCompose(s.dir)
		order, _ := projectServiceOrder(ctx, compose, s.dir, files)
		specs, _ := docker.ComposeServiceSpecs(ctx, compose, s.dir)
		// The first sleep is the interrupt: the user hits ctrl-c while the gate waits.
		clk := &cancelClock{stepClock: stepClock{now: time.Unix(0, 0)}, cancel: cancel}
		_, err := rollingRestart(ctx, rollingPlan{compose: compose, workdir: s.dir, order: order, specs: specs, clock: clk, jsonOut: true})
		var ce *errs.CLIError
		if !errors.Is(err, context.Canceled) || errors.As(err, &ce) {
			t.Fatalf("want an uncoded context.Canceled, got %v", err)
		}
	})
}

// recheckStack is two running services: web is gated first, then ntask. The ps
// for web changes to lateWeb once both have been brought up, which is what the
// final re-check sees. fixedStart pins StartedAt so only lateWeb can differ.
func recheckStack(t *testing.T, lateWeb string, fixedStart bool) *rollingStack {
	t.Helper()
	s := newRollingStack(t, []string{"web", "ntask"}, stackConfig)
	s.setPs(t, "web", `[{"ID":"w1","Name":"p-web-1","State":"running"}]`)
	s.setPs(t, "ntask", `[{"ID":"n1","Name":"p-ntask-1","State":"running"}]`)
	for name, body := range map[string]string{"ps.web.late.json": lateWeb, "late.web": "2"} {
		if err := os.WriteFile(filepath.Join(s.fix, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if fixedStart {
		if err := os.WriteFile(filepath.Join(s.fix, "startfixed"), []byte("2026-10-10T00:00:00Z"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestDeployFinalRecheck(t *testing.T) {
	cases := []struct {
		name, lateWeb string
		fixedStart    bool
		wantFail      bool
		wantIn        string
	}{
		{"unchanged services pass", `[{"ID":"w1","Name":"p-web-1","State":"running"}]`, true, false, ""},
		{"exited non-zero since the gate passed", `[{"ID":"w1","Name":"p-web-1","State":"exited","ExitCode":1}]`, true, true, "exited 1"},
		{"restarting since the gate passed", `[{"ID":"w1","Name":"p-web-1","State":"restarting"}]`, true, true, "restarting"},
		{"started again since the gate passed", `[{"ID":"w1","Name":"p-web-1","State":"running"}]`, false, true, "restarted"},
		{"exit 0 of a finished container is not a crash", `[{"ID":"w1","Name":"p-web-1","State":"exited","ExitCode":0}]`, true, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := recheckStack(t, tc.lateWeb, tc.fixedStart)
			steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
			if !tc.wantFail {
				if err != nil {
					t.Fatalf("want a pass, got %v", err)
				}
				return
			}
			var ce *errs.CLIError
			if !errors.As(err, &ce) || ce.Code != "E250" || !strings.Contains(err.Error(), "web") || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("want E250 naming web and %q, got %v", tc.wantIn, err)
			}
			// Both services were brought up: the crash was only visible at the end.
			if got := strings.Fields(s.read("ups")); len(got) != 2 {
				t.Errorf("ups = %v, want both services restarted before the re-check", got)
			}
			if steps[0].Name != "Restart web" || steps[0].Status != "failed" || steps[1].Status != "done" {
				t.Errorf("steps = %+v, want web failed and ntask done", steps)
			}
		})
	}

	t.Run("a docker read failure during the re-check is uncoded, not E250", func(t *testing.T) {
		s := recheckStack(t, `[{"ID":"w1","Name":"p-web-1","State":"running"}]`, true)
		if err := os.WriteFile(filepath.Join(s.fix, "psfaillate"), []byte("web 2"), 0o600); err != nil {
			t.Fatal(err)
		}
		steps, err := runPlan(t, s, &stepClock{now: time.Unix(0, 0)})
		var ce *errs.CLIError
		if err == nil || errors.As(err, &ce) || !errors.Is(err, docker.ErrStateRead) || !strings.Contains(err.Error(), "Cannot connect") {
			t.Fatalf("want an uncoded state-read error carrying docker's message, got %v", err)
		}
		if steps[0].Status != "failed" {
			t.Errorf("steps = %+v", steps)
		}
	})
}

// cancelClock cancels the context on its first Sleep.
type cancelClock struct {
	stepClock
	cancel context.CancelFunc
}

func (c *cancelClock) Sleep(ctx context.Context, d time.Duration) error {
	c.cancel()
	return c.stepClock.Sleep(ctx, d)
}
