package bluegreen

// bluegreen_exec_test.go — every docker compose call of the blue/green
// pipeline carries the project's env files (D-0550), so a ${VAR} value that
// lives only in .nself/compose.env resolves for both stacks.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
)

// fakeBlueGreenDocker installs a docker stub behind newCompose. Each call
// appends its argv, then the PORT_OFFSET and NSELF_ENV it saw, then "--END--";
// a `ps` call prints one running, healthy container. It returns the project
// root (with .env and .nself/compose.env) and a reader of the recorded calls.
func fakeBlueGreenDocker(t *testing.T) (string, func() [][]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	root, stub := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{".env": "PROJECT_NAME=demo\n", ".nself/compose.env": "FAKE_ONLY_IN_COMPOSE_ENV=x\n"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log := filepath.Join(stub, "log")
	bin := filepath.Join(stub, "docker")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$@\" \"env:PORT_OFFSET=${PORT_OFFSET-} NSELF_ENV=${NSELF_ENV-}\" --END-- >> '" + log + "'\n" +
		"case \" $* \" in *' ps '*) printf 'nself-green-api\\trunning\\thealthy\\n' ;; esac\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	old := newCompose
	newCompose = func(projectRoot string) *docker.Compose {
		c := old(projectRoot)
		c.DockerPath = bin
		return c
	}
	t.Cleanup(func() { newCompose = old })
	return root, func() [][]string {
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		var calls [][]string
		var cur []string
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			if l == "--END--" {
				calls, cur = append(calls, cur), nil
				continue
			}
			cur = append(cur, l)
		}
		return calls
	}
}

// missingEnvFiles returns the calls that do not pass every file in want as an
// --env-file value.
func missingEnvFiles(calls [][]string, want []string) []string {
	var bad []string
	for _, argv := range calls {
		got := map[string]bool{}
		for i := 0; i+1 < len(argv); i++ {
			if argv[i] == "--env-file" {
				got[argv[i+1]] = true
			}
		}
		for _, w := range want {
			if !got[w] {
				bad = append(bad, strings.Join(argv, " "))
				break
			}
		}
	}
	return bad
}

func TestBlueGreenComposeCallsCarryEnvFiles(t *testing.T) {
	root, calls := fakeBlueGreenDocker(t)
	cfg := DeployConfig{ProjectRoot: root, GreenPortOffset: 100}
	ctx := context.Background()
	steps := []struct {
		name string
		run  func() error
	}{
		{"pull", func() error { return pullImages(ctx, root) }},
		{"start green", func() error { return startGreen(ctx, cfg) }},
		{"wait green", func() error { return waitForHealth(ctx, cfg, EnvGreen, 5*time.Second) }},
		{"stop green", func() error { return stopGreen(ctx, cfg) }},
		{"stop blue", func() error { return stopBlue(ctx, cfg) }},
		{"nginx reload", func() error { return reloadNginx(root) }},
		{"soak measurement", func() error { _, err := measureErrorRate(ctx, cfg); return err }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}
	got := calls()
	if len(got) != len(steps) {
		t.Fatalf("recorded %d compose calls, want %d: %q", len(got), len(steps), got)
	}
	want := []string{filepath.Join(root, ".env"), filepath.Join(root, ".nself", "compose.env")}
	if bad := missingEnvFiles(got, want); len(bad) > 0 {
		t.Errorf("compose calls without the project env files:\n%s", strings.Join(bad, "\n"))
	}
	for i, wantSub := range []string{
		" pull env:PORT_OFFSET= NSELF_ENV=",
		" -p nself-green up -d env:PORT_OFFSET=100 NSELF_ENV=green",
		" -p nself-green ps --format ",
		" -p nself-green down --remove-orphans ",
		" -p nself-blue down --remove-orphans ",
		" exec -T nginx nginx -s reload ",
		" -p nself-green ps --format ",
	} {
		if line := strings.Join(got[i], " "); !strings.Contains(line, wantSub) {
			t.Errorf("call %d (%s) = %q, want it to contain %q", i, steps[i].name, line, wantSub)
		}
	}
	// Positive control: the checker reports a call that drops the env files.
	if bad := missingEnvFiles([][]string{{"compose", "-p", "nself-green", "up", "-d"}}, want); len(bad) != 1 {
		t.Errorf("missingEnvFiles missed a call without --env-file: %q", bad)
	}
}
