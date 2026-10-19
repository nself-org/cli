package commands

// Tests for the lifecycle fragment's envelopes (P7-SURF-10).
//
// Purpose: prove init, start, stop and restart answer `--json` with one v1
// envelope whose data is what the command did (not a canned value), that a
// failing run leaves stdout empty so the decorator's error envelope is the only
// document, and that the fragment's registry rows are the ones the ticket
// promises (stop --volumes confirm, start --watch and start dev streams).
// Inputs: fixture projects under t.TempDir, a docker stub on PATH, the real run
// functions behind fresh cobra commands carrying the production flags.
// Outputs: pass/fail. Constraints: no Docker daemon, nothing outside t.TempDir;
// the POSIX stub is skipped on Windows.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/health"
	clonetemplate "github.com/nself-org/cli/internal/templates/clone"
	"github.com/spf13/cobra"
)

func TestEnvelopeCoverageLifecycle(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "lifecycle")
}

// lifecycleRun runs the real RunE of name with flags (and positional args after
// the flags) in dir and returns the pilot stdout and the error.
func lifecycleRun(t *testing.T, name, dir string, flags ...string) (string, error) {
	t.Helper()
	out, _ := pilotBuffers(t)
	c := &cobra.Command{Use: name}
	f := c.Flags()
	f.Bool("json", false, "")
	switch name {
	case "init":
		c.RunE = runInit
		for _, b := range []string{"fast", "interactive", "non-interactive", "no-seed", "dry-run", "skip-validation", "wizard", "demo", "full", "force", "quiet", "no-pgvector", "list-presets"} {
			f.Bool(b, false, "")
		}
		for _, s := range []string{"template", "name", "domain", "profile", "preset", "cs-template"} {
			f.String(s, "", "")
		}
	case "start":
		c.RunE = runStart
		f.Int("timeout", 120, "")
		for _, b := range []string{"verbose", "debug", "skip-health-checks", "fresh", "force-recreate", "clean-start", "quick", "skip-port-check", "skip-build", "skip-plugins", "watch", "quiet", "no-monorepo", "skip-db-init"} {
			f.Bool(b, false, "")
		}
	case "stop":
		c.RunE = runStop
		f.Int("graceful", 30, "")
		for _, b := range []string{"volumes", "rmi", "remove-images", "remove-orphans", "verbose", "no-monorepo"} {
			f.Bool(b, false, "")
		}
	case "restart":
		c.RunE = runRestart
		f.Bool("smart", true, "")
		for _, b := range []string{"all", "verbose", "skip-build", "no-build"} {
			f.Bool(b, false, "")
		}
	}
	c.SetContext(context.Background())
	if err := c.ParseFlags(flags); err != nil {
		t.Fatal(err)
	}
	var err error
	inDir(t, dir, func() { err = c.RunE(c, c.Flags().Args()) })
	return out.String(), err
}

// lifecycleDockerStub answers the docker calls the lifecycle commands make.
// The project has three configured services, postgres and hasura have
// containers (hasura's has exited) and an orphan container is left over:
// redis was enabled after the stack came up and has no container. Everything
// else succeeds and prints nothing.
const lifecycleDockerStub = `#!/bin/sh
case " $* " in
*' ps -a '*)
  printf 'smoke_postgres\trunning\tpostgres\th1\n'
  printf 'smoke_hasura\texited\thasura\th2\n'
  printf 'smoke_old\trunning\told\th3\n' ;;
*' ps --format json '*)
  printf '%s\n' '[{"Name":"smoke_postgres","Service":"postgres","State":"running"},{"Name":"smoke_hasura","Service":"hasura","State":"running"}]' ;;
*' config --services '*)
  printf 'postgres\nhasura\nredis\n' ;;
esac
exit 0
`

// lifecycleProject makes a project with a compose file and a stub docker that
// lists two running services and accepts every other call. t.Setenv keeps the
// run away from the user's HOME and from the real Docker.
func lifecycleProject(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the docker stub is a POSIX shell script")
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		".env":               "PROJECT_NAME=smoke\nENV=dev\nBASE_DOMAIN=example.test\n",
		"docker-compose.yml": "services: {}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stub := t.TempDir()
	script := lifecycleDockerStub
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AI_AUTO_INSTALL", "false")
	return dir
}

func stringList(t *testing.T, v any) []string {
	t.Helper()
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("%v is not a JSON array", v)
	}
	out := []string{}
	for _, it := range items {
		out = append(out, it.(string))
	}
	return out
}

func samePath(t *testing.T, got, want string) {
	t.Helper()
	g, _ := filepath.EvalSymlinks(got)
	w, _ := filepath.EvalSymlinks(want)
	if g == "" || g != w {
		t.Fatalf("path %q does not resolve to %q", got, want)
	}
}

// TestLifecycleJSON covers the four commands' success documents and their
// failure paths (stdout stays empty; the decorator prints the error envelope).
func TestLifecycleJSON(t *testing.T) {
	t.Run("init writes the project and reports it", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()
		out, err := lifecycleRun(t, "init", dir, "--json", "--quiet", "--non-interactive", "--name", "smoke", "--domain", "example.test")
		if err != nil {
			t.Fatal(err)
		}
		data := validateEnvelope(t, out, "init")
		samePath(t, data["project_dir"].(string), dir)
		files := stringList(t, data["files_written"])
		for _, want := range []string{".env", ".env.example", ".gitignore"} {
			if !slices.Contains(files, want) {
				t.Errorf("files_written %v misses %s", files, want)
			}
		}
		for _, f := range files {
			if _, statErr := os.Stat(filepath.Join(dir, f)); statErr != nil {
				t.Errorf("files_written names %s, which init did not write: %v", f, statErr)
			}
		}
		env, _ := os.ReadFile(filepath.Join(dir, ".env"))
		if !strings.Contains(string(env), "\nENV="+data["env"].(string)+"\n") && !strings.HasPrefix(string(env), "ENV="+data["env"].(string)+"\n") {
			t.Errorf("env %q is not the ENV written to .env", data["env"])
		}
		if next := stringList(t, data["next"]); !slices.Equal(next, []string{"nself doctor", "nself start", "nself status", "nself plugin list"}) {
			t.Errorf("next = %v", next)
		}
	})

	t.Run("init without --force on an existing project fails with no document", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=smoke\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := lifecycleRun(t, "init", dir, "--json", "--quiet", "--non-interactive")
		if err == nil || out != "" {
			t.Fatalf("err=%v stdout=%q, want an error and no stdout", err, out)
		}
	})

	t.Run("init --list-presets is an empty result in the current directory", func(t *testing.T) {
		dir := t.TempDir()
		out, err := lifecycleRun(t, "init", dir, "--json", "--list-presets")
		if err != nil {
			t.Fatal(err)
		}
		data := validateEnvelope(t, out, "init")
		samePath(t, data["project_dir"].(string), dir)
		if data["env"] != "" || len(stringList(t, data["files_written"])) != 0 || len(stringList(t, data["next"])) != 0 {
			t.Errorf("--list-presets wrote nothing, got %v", data)
		}
	})

	t.Run("init --template lists the files it scaffolded", func(t *testing.T) {
		useTemplateSource(t)
		want, err := clonetemplate.Scaffold("discord-clone", clonetemplate.ScaffoldOptions{DryRun: true})
		if err != nil {
			t.Fatal(err)
		}
		for i := range want {
			want[i] = filepath.ToSlash(want[i])
		}
		slices.Sort(want)
		dest := filepath.Join(t.TempDir(), "app")
		out, err := lifecycleRun(t, "init", t.TempDir(), "--json", "--quiet", "--template", "discord-clone", dest)
		if err != nil {
			t.Fatal(err)
		}
		data := validateEnvelope(t, out, "init")
		samePath(t, data["project_dir"].(string), dest)
		if got := stringList(t, data["files_written"]); !slices.Equal(got, want) {
			t.Errorf("files_written = %v, want the %d template files %v", got, len(want), want)
		}
		if next := stringList(t, data["next"]); len(next) < 3 || next[0] != "nself start" {
			t.Errorf("next = %v, want the clone template steps", next)
		}

		dry := filepath.Join(t.TempDir(), "dry")
		out, err = lifecycleRun(t, "init", t.TempDir(), "--json", "--quiet", "--dry-run", "--template", "discord-clone", dry)
		if err != nil {
			t.Fatal(err)
		}
		if files := stringList(t, validateEnvelope(t, out, "init")["files_written"]); len(files) != 0 {
			t.Errorf("--dry-run wrote %v, want nothing", files)
		}
		if _, statErr := os.Stat(dry); !os.IsNotExist(statErr) {
			t.Errorf("--dry-run created %s", dry)
		}
	})

	t.Run("stop reports the services it found running", func(t *testing.T) {
		dir := lifecycleProject(t)
		out, err := lifecycleRun(t, "stop", dir, "--json", "--no-monorepo")
		if err != nil {
			t.Fatal(err)
		}
		data := validateEnvelope(t, out, "stop")
		if got := stringList(t, data["stopped"]); !slices.Equal(got, []string{"postgres", "hasura"}) || data["volumes_removed"] != false {
			t.Errorf("stop data = %v", data)
		}
		out, err = lifecycleRun(t, "stop", dir, "--json", "--no-monorepo", "--volumes")
		if err != nil {
			t.Fatal(err)
		}
		if validateEnvelope(t, out, "stop")["volumes_removed"] != true {
			t.Errorf("--volumes must report volumes_removed: %s", out)
		}
		out, err = lifecycleRun(t, "stop", dir, "--json", "--no-monorepo", "hasura")
		if err != nil {
			t.Fatal(err)
		}
		if got := stringList(t, validateEnvelope(t, out, "stop")["stopped"]); !slices.Equal(got, []string{"hasura"}) {
			t.Errorf("stop hasura reported %v", got)
		}
	})

	t.Run("stop and restart fail with no document outside a project", func(t *testing.T) {
		dir := t.TempDir()
		for name, flags := range map[string][]string{"stop": {"--json", "--no-monorepo"}, "restart": {"--json"}} {
			out, err := lifecycleRun(t, name, dir, flags...)
			if err == nil || out != "" {
				t.Errorf("%s: err=%v stdout=%q, want an error and no stdout", name, err, out)
			}
		}
	})

	t.Run("restart reports what compose restarted, not what is configured", func(t *testing.T) {
		if testing.Short() {
			t.Skip("restart runs the stub stack five times")
		}
		// The stub lists two containers but three configured services: redis
		// was enabled after the stack came up and has no container.
		dir := lifecycleProject(t)
		prevSettle, prevDown := restartSettleWait, restartDownWait
		restartSettleWait, restartDownWait = 0, 0
		t.Cleanup(func() { restartSettleWait, restartDownWait = prevSettle, prevDown })
		cases := []struct {
			flags []string
			want  []string
			mode  string
		}{
			{[]string{"--json", "hasura"}, []string{"hasura"}, "services"},
			// compose restart touches the exited hasura container, not redis or the orphan
			{[]string{"--json", "--smart=false"}, []string{"postgres", "hasura"}, "quick"},
			{[]string{"--json"}, []string{"postgres", "hasura"}, "smart"},
			{[]string{"--json", "--all", "--skip-build"}, []string{"postgres", "hasura", "redis"}, "all"},
			// --all wins over named services: the whole stack is cycled
			{[]string{"--json", "--all", "--skip-build", "hasura"}, []string{"postgres", "hasura", "redis"}, "all"},
		}
		start := time.Now()
		t.Cleanup(func() { // D-0736: the unshortened pauses made these five cases take 29 s
			if d := time.Since(start); d > 15*time.Second {
				t.Errorf("five restart cases took %v: a real pause crept back in", d)
			}
		})
		for _, tc := range cases {
			out, err := lifecycleRun(t, "restart", dir, tc.flags...)
			if err != nil {
				t.Fatal(err)
			}
			data := validateEnvelope(t, out, "restart")
			if got := stringList(t, data["restarted"]); !slices.Equal(got, tc.want) || data["mode"] != tc.mode {
				t.Errorf("restart %v = %v (mode %v), want %v (mode %s)", tc.flags, got, data["mode"], tc.want, tc.mode)
			}
		}
	})

	// The ruling for the argv rows that fail on the fixture: the conversion
	// script has no failure-envelope tag, so the decorated binary is run here,
	// against a Docker that cannot be reached, in both compat modes.
	t.Run("decorated failures give one error envelope in v1.5 and stay legacy in v1.4", func(t *testing.T) {
		cases := []struct {
			name string
			args []string
			code string // "" = only the E-class is checked
		}{
			{"start", []string{"start", "--skip-build"}, "E002"},
			{"stop", []string{"stop"}, "E002"},
			{"restart", []string{"restart", "hasura"}, "E002"},
			{"init", []string{"init", "--quiet", "--non-interactive"}, ""},
		}
		for _, v15 := range []bool{false, true} {
			for _, tc := range cases {
				proj := pilotProject(t)
				if err := os.WriteFile(filepath.Join(proj, "docker-compose.yml"), []byte("services: {}\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				human := runPilot(t, proj, v15, nil, tc.args...)
				js := runPilot(t, proj, v15, nil, append(slices.Clone(tc.args), "--json")...)
				// A daemon-unreachable failure is an infra exit (2) in v1.5 JSON; every
				// other run, and v1.4, exits as the human run does.
				wantCode := human.code
				if v15 && tc.code == "E002" {
					wantCode = 2
				}
				if human.code == 0 || js.code != wantCode {
					t.Fatalf("%s v15=%v: exit human=%d json=%d, want json %d", tc.name, v15, human.code, js.code, wantCode)
				}
				if line := lastErrorLine(human.stderr); line != "" && !strings.Contains(js.stderr, line) {
					t.Errorf("%s v15=%v: --json stderr lost the human error line %q:\n%s", tc.name, v15, line, js.stderr)
				}
				if !v15 {
					if js.stdout != "" {
						t.Errorf("%s v1.4: --json stdout = %q, want empty (legacy errors)", tc.name, js.stdout)
					}
					continue
				}
				doc := singleDoc(t, js.stdout)
				if verr := contractValidate(contractResolve(t, "envelope.v1.schema.json"), []byte(js.stdout)); verr != nil {
					t.Fatalf("%s: error envelope invalid: %v\n%s", tc.name, verr, js.stdout)
				}
				e, _ := doc["error"].(map[string]any)
				code, _ := e["code"].(string)
				if _, hasData := doc["data"]; hasData || doc["command"] != tc.name || !strings.HasPrefix(code, "E") || (tc.code != "" && code != tc.code) {
					t.Errorf("%s: envelope = %s, want command %s, an error and code %q", tc.name, js.stdout, tc.name, tc.code)
				}
				t.Logf("%s error code %s exit %d", tc.name, code, js.code)
			}
		}
	})
}

// TestRestartedServicesRebuild: a smart restart that rebuilt (the compose file
// changed) reports the whole manifest, one that did not reports the services
// that had containers.
func TestRestartedServicesRebuild(t *testing.T) {
	dir := lifecycleProject(t)
	ctx := context.Background()
	comp := docker.NewCompose()
	before := snapshotRestart(ctx, comp, dir, "smoke")
	if !slices.Equal(before.existing, []string{"postgres", "hasura"}) {
		t.Fatalf("existing = %v, want the configured services with a container, in manifest order", before.existing)
	}
	if got := restartedServices(ctx, comp, dir, "smart", nil, before); !slices.Equal(got, before.existing) {
		t.Errorf("no rebuild: restarted = %v, want %v", got, before.existing)
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {redis: {}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := restartedServices(ctx, comp, dir, "smart", nil, before); !slices.Equal(got, []string{"postgres", "hasura", "redis"}) {
		t.Errorf("rebuild: restarted = %v, want the whole manifest", got)
	}
	if got := restartedServices(ctx, comp, dir, "services", []string{"hasura"}, before); !slices.Equal(got, []string{"hasura"}) {
		t.Errorf("services: restarted = %v", got)
	}
}

// TestLifecycleDaemonUnreachable: only a verified unreachable daemon is E002; a
// compose failure with a live daemon, or an error that never ran compose, is not.
func TestLifecycleDaemonUnreachable(t *testing.T) {
	cmd := &cobra.Command{Use: "stop"}
	composeFail := errors.New("docker compose -f docker-compose.yml down: exit status 1")
	lifecycleProject(t) // stub whose `docker info` succeeds
	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"compose failure, live daemon": {composeFail, false},
		"no compose run":               {errors.New("no docker-compose.yml found"), false},
		"sentinel":                     {fmt.Errorf("start: %w", errs.ErrDockerNotRunning), true},
		"daemon message in the tail":   {errors.New("docker compose down: exit status 1: Cannot connect to the Docker daemon at unix:///x"), true},
		"nil":                          {nil, false},
	} {
		if got := daemonUnreachable(cmd, tc.err); got != tc.want {
			t.Errorf("%s: daemonUnreachable = %v, want %v", name, got, tc.want)
		}
	}
	stub := t.TempDir()
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if !daemonUnreachable(cmd, composeFail) {
		t.Error("a failed compose run with `docker info` failing must be daemon-unreachable")
	}
	if daemonUnreachable(cmd, errors.New("no docker-compose.yml found")) {
		t.Error("an error that never ran compose must not be classified from `docker info`")
	}
}

// lastErrorLine is the last "Error: ..." line of a human stderr, or "", cut at
// the compose exit status: the stderr tail docker.Compose appends after it is
// not stable from one run to the next.
func lastErrorLine(stderr string) string {
	last := ""
	for _, l := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(l, "Error: ") {
			last, _, _ = strings.Cut(l, ": exit status")
		}
	}
	return last
}

// TestLifecycleGoldens: the committed fixtures are what the commands' own
// emission writes for the same values, in document form, and a `start --watch`
// result is the last line of a stream.
func TestLifecycleGoldens(t *testing.T) {
	cases := map[string]any{
		"init": InitResult{ProjectDir: "/work/demo", Env: "dev", FilesWritten: []string{".env", ".env.example", ".gitignore", "AGENTS.md"}, Next: []string{"nself doctor", "nself start", "nself status", "nself plugin list"}},
		"start": StartResult{
			Services: []StartService{{"postgres", "running", "healthy"}, {"hasura", "running", "healthy"}, {"auth", "running", "starting"}, {"nginx", "running", "none"}},
			URLs:     []string{"http://localhost:8080/v1/graphql", "http://localhost:8080/console", "http://localhost:4000"}, DurationMS: 18432,
		},
		"stop":    StopResult{Stopped: []string{"postgres", "hasura", "auth", "nginx"}},
		"restart": RestartResult{Restarted: []string{"postgres", "hasura", "auth", "nginx"}, Mode: "smart"},
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			out, _ := pilotBuffers(t)
			c := &cobra.Command{Use: name}
			c.Flags().Bool("json", true, "")
			if err := emitLifecycle(c, name, data); err != nil {
				t.Fatal(err)
			}
			if out.String() != goldenFor(t, name) {
				t.Fatalf("emission differs from golden\n--- got ---\n%s\n--- want ---\n%s", out, goldenFor(t, name))
			}
		})
	}

	t.Run("start --watch ends a stream", func(t *testing.T) {
		out, _ := pilotBuffers(t)
		c := &cobra.Command{Use: "start"}
		c.Flags().Bool("json", true, "")
		c.Flags().Bool("watch", true, "")
		if err := emitLifecycle(c, "start", cases["start"]); err != nil {
			t.Fatal(err)
		}
		line := strings.TrimSuffix(out.String(), "\n")
		var env struct {
			Command string
			Data    map[string]any
		}
		if err := json.Unmarshal([]byte(line), &env); err != nil || strings.Contains(line, "\n") {
			t.Fatalf("stream result is not one JSON line: %q (%v)", out, err)
		}
		if env.Command != "start" || env.Data["type"] != "result" || env.Data["duration_ms"] != float64(18432) {
			t.Fatalf("stream result = %s", line)
		}
	})

	t.Run("no JSON flag, no output", func(t *testing.T) {
		out, _ := pilotBuffers(t)
		c := &cobra.Command{Use: "stop"}
		c.Flags().Bool("json", false, "")
		if err := emitLifecycle(c, "stop", StopResult{}); err != nil || out.Len() != 0 {
			t.Fatalf("err=%v stdout=%q, want nothing", err, out)
		}
	})
}

// TestLifecycleResultAssembly: the start result is built from the health report
// and the stack URLs, with the display labels stripped.
func TestLifecycleResultAssembly(t *testing.T) {
	report := &health.HealthReport{Results: []health.HealthResult{
		{Service: "postgres", Status: "healthy"}, {Service: "hasura", Status: "unhealthy"},
		{Service: "auth", Status: "starting"}, {Service: "nginx", Status: "running"},
		{Service: "ghost", Status: "not_found"}, {Service: "odd", Status: "error"},
	}}
	cfg := &config.Config{BaseDomain: "example.test", Env: "dev"}
	got := startResult(&cobra.Command{Use: "start"}, startOutcome{cfg: cfg, dir: t.TempDir(), report: report}, 7)
	want := []StartService{
		{"postgres", "running", "healthy"}, {"hasura", "running", "unhealthy"}, {"auth", "running", "starting"},
		{"nginx", "running", "none"}, {"ghost", "missing", "none"}, {"odd", "unknown", "none"},
	}
	if !reflect.DeepEqual(got.Services, want) || got.DurationMS != 7 {
		t.Errorf("services = %+v, want %+v", got.Services, want)
	}
	for _, u := range got.URLs {
		if !strings.HasPrefix(u, "http://example.test") {
			t.Errorf("url %q is not a bare URL on the base domain", u)
		}
	}
	if len(got.URLs) == 0 {
		t.Error("no urls reported")
	}
	if empty := startResult(&cobra.Command{Use: "start"}, startOutcome{}, 0); empty.Services == nil || empty.URLs == nil {
		t.Error("an unfinished start must still report empty arrays, not null")
	}
	if restartMode(true, true, []string{"x"}) != "all" || restartMode(false, true, []string{"x"}) != "services" || restartMode(false, true, nil) != "smart" || restartMode(false, false, nil) != "quick" {
		t.Error("restartMode does not follow runRestart's dispatch order")
	}
}

// TestLifecycleRegistryRows: the fragment's promises reach the built registry.
func TestLifecycleRegistryRows(t *testing.T) {
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	row := map[string]int{}
	for i, c := range reg.Commands {
		row[c.Path] = i
	}
	for _, p := range []string{"init", "start", "stop", "restart"} {
		c := reg.Commands[row["nself "+p]]
		if c.JSON != canon.JSONEnvelope || jsonV15OnlyEnvelope[p] {
			t.Errorf("%s: json=%s v15-only=%v, want an envelope in both modes", p, c.JSON, jsonV15OnlyEnvelope[p])
		}
	}
	stop := reg.Commands[row["nself stop"]]
	if stop.Confirm == nil || len(stop.Confirm.Flags) != 0 {
		t.Errorf("stop confirm = %+v, want a confirm block with no flags", stop.Confirm)
	}
	var volumes string
	for _, f := range stop.Flags {
		if f.Name == "volumes" && f.SideEffect != nil {
			volumes = *f.SideEffect
		}
	}
	if volumes != canon.SideEffectDestructive {
		t.Errorf("stop --volumes side_effect = %q, want destructive", volumes)
	}
	start := reg.Commands[row["nself start"]]
	watch := ""
	for _, f := range start.Flags {
		if f.Name == "watch" && f.Output != nil {
			watch = *f.Output
		}
	}
	if watch != canon.OutputStream {
		t.Errorf("start --watch output = %q, want stream", watch)
	}
	if dev := reg.Commands[row["nself start dev"]]; dev.Output != canon.OutputStream {
		t.Errorf("start dev output = %q, want stream", dev.Output)
	}
}
