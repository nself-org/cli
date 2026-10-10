//go:build integration

package commands

// deploy_strategies_integration_test.go — the rolling restart against a real
// compose project: a base file, a plugin fragment that interpolates
// ${PLUGIN_VAR} from the computed .nself/compose.env, a one-shot with restart
// "no", a service with a healthcheck and one without. Skips unless
// INTEGRATION=1 and a Docker daemon answers.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// integrationGate is the repo-wide switch for Docker-backed tests.
const integrationGate = "INTEGRATION"

// integrationProject writes a project named name into a temp dir.
func integrationProject(t *testing.T, name, base, fragment string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) string {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	b := write("docker-compose.yml", "name: "+name+"\nservices:\n"+base)
	p := write(".nself/plugins/plug.yml", "services:\n"+fragment)
	write(".nself/compose-files.txt", b+"\n"+p+"\n")
	// The root .env holds a different value: compose.env (computed) must win.
	write(".env", "PLUGIN_VAR=from-root-env\n")
	write(".nself/compose.env", "PLUGIN_VAR=computed-value\n")
	compose, _, err := deployCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		_ = compose.ComposeDown(ctx, dir, docker.DownOptions{RemoveVolumes: true, RemoveOrphans: true})
	})
	return dir
}

func TestDeployStrategyIntegration(t *testing.T) {
	if os.Getenv(integrationGate) != "1" {
		t.Skip("set INTEGRATION=1 to run against a Docker daemon")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if _, err := docker.ServerVersion(ctx); err != nil {
		t.Skipf("no Docker daemon: %v", err)
	}
	sleeper := "    image: alpine:3\n    command: [\"sleep\", \"300\"]\n"
	suffix := fmt.Sprintf("%d", os.Getpid())

	t.Run("plugin fragment, one-shot, healthcheck and bare service all deploy", func(t *testing.T) {
		dir := integrationProject(t, "depl16a"+suffix,
			"  web:\n"+sleeper+"    healthcheck: {test: [\"CMD\", \"true\"], interval: 1s, timeout: 2s, retries: 3}\n"+
				"  bare:\n"+sleeper+
				"  init:\n    image: alpine:3\n    command: [\"sh\", \"-c\", \"exit 0\"]\n    restart: \"no\"\n",
			"  plug:\n"+sleeper+"    environment:\n      GOT: ${PLUGIN_VAR}\n")
		steps, err := runRollingRestart(ctx, dir, true)
		if err != nil {
			t.Fatalf("deploy failed: %v (steps %+v)", err, steps)
		}
		var names []string
		for _, s := range steps {
			if s.Status != "done" {
				t.Errorf("step %+v not done", s)
			}
			names = append(names, strings.TrimPrefix(s.Name, "Restart "))
		}
		if len(names) != 4 || names[len(names)-1] != "plug" {
			t.Fatalf("restart order = %v, want 4 services with the plugin service last", names)
		}
		compose, _, _ := deployCompose(dir)
		cs, err := compose.ComposeServiceStates(ctx, dir, "plug")
		if err != nil || len(cs) != 1 {
			t.Fatalf("plug state: %v %v", cs, err)
		}
		info, err := docker.InspectContainer(ctx, cs[0].Name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Env["GOT"]; got != "computed-value" {
			t.Errorf("plugin service got PLUGIN_VAR=%q, want the value computed into .nself/compose.env", got)
		}
	})

	t.Run("a one-shot that exits 1 fails the deploy with E250 and later services never start", func(t *testing.T) {
		dir := integrationProject(t, "depl16b"+suffix,
			// postgres sorts first (core order), so every other service is "later".
			"  postgres:\n    image: alpine:3\n    command: [\"sh\", \"-c\", \"exit 1\"]\n    restart: \"no\"\n"+
				"  zlater:\n"+sleeper,
			"  plug:\n"+sleeper)
		_, err := runRollingRestart(ctx, dir, true)
		var ce *errs.CLIError
		if !errors.As(err, &ce) || ce.Code != "E250" || !strings.Contains(err.Error(), "postgres") {
			t.Fatalf("want E250 naming postgres, got %v", err)
		}
		compose, _, _ := deployCompose(dir)
		for _, svc := range []string{"zlater", "plug"} {
			if cs, _ := compose.ComposeServiceStates(ctx, dir, svc); len(cs) != 0 {
				t.Errorf("%s started after postgres failed: %+v", svc, cs)
			}
		}
	})
}
