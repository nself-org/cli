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
// ps.<svc>.json, and upfail (a service name whose `up` exits 1).
const rollingFakeDocker = `#!/bin/sh
D="$FAKE_DOCKER_DIR"
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
  for last; do :; done
  cat "$D/ps.$last.json" 2>/dev/null || true ;;
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

func TestDeployStrategyManifest(t *testing.T) {
	t.Run("manifest and env files reach every call, order from the manifest, plugin last", func(t *testing.T) {
		s := newRollingStack(t, []string{"ntask", "web", "postgres"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "web", psRunning)
		s.setPs(t, "ntask", psRunning)
		steps, err := runRollingRestart(context.Background(), s.dir, true)
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
		s.setPs(t, "init", `[{"Name":"p-init-1","State":"exited","ExitCode":1}]`)
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
		s.setPs(t, "init", `[{"Name":"p-init-1","State":"exited","ExitCode":0}]`)
		if _, err := runRollingRestart(context.Background(), s.dir, true); err != nil {
			t.Fatalf("exit 0 one-shot must pass: %v", err)
		}
	})

	t.Run("an unhealthy service fails with E251 after 60s on the fake clock", func(t *testing.T) {
		s := newRollingStack(t, []string{"postgres", "web"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"unhealthy"}]`)
		s.setPs(t, "web", psRunning)
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
		clk := &stepClock{now: time.Unix(0, 0)}
		steps, err := rollingRestart(context.Background(), rollingPlan{compose: compose, workdir: s.dir, order: order, specs: specs, clock: clk, jsonOut: true})
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
		s := newRollingStack(t, []string{"postgres", "minio"}, stackConfig)
		s.setPs(t, "postgres", `[{"Name":"p-pg-1","State":"running","Health":"healthy"}]`)
		s.setPs(t, "minio", psRunning)
		if _, err := runRollingRestart(context.Background(), s.dir, true); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(s.read("ups"), "storage") {
			t.Errorf("restarted a name no compose reported: %q", s.read("ups"))
		}
	})
}
