package build

// Purpose: verify legacy volume identity with real Compose and CLI arguments.
// Inputs: generated compose YAML, .env and generated compose interpolation env.
// Outputs: resolved project and Postgres volume names, without starting services.
// Constraints: opt in with INTEGRATION=1; requires docker compose, not a daemon.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

func TestComposeProjectNamePrecedence(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to verify project identity with real docker compose")
	}
	t.Cleanup(SnapshotEnv())
	t.Setenv("ENV", "dev")
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=new-stack\nCOMPOSE_PROJECT_NAME=legacy-stack\n", 0600)
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	yml, err := compose.NewGenerator(cfg).WithWorkDir(dir).Generate()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "docker-compose.yml")
	testWriteFile(t, file, string(yml), 0600)
	vars := ComputePluginEnvVars(dir, t.TempDir(), cfg)
	if err := WriteComposeEnv(dir, cfg, nil, vars); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, inherited, project, want string
		generated                      bool
	}{
		{name: "legacy dotenv", want: "legacy-stack"},
		{name: "CLI generated env", generated: true, want: "new-stack"},
		{name: "inherited env wins", generated: true, inherited: "legacy-stack", want: "legacy-stack"},
		{name: "explicit project wins", generated: true, inherited: "legacy-stack", project: "explicit-stack", want: "explicit-stack"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("COMPOSE_PROJECT_NAME", tc.inherited)
			if tc.inherited == "" {
				if err := os.Unsetenv("COMPOSE_PROJECT_NAME"); err != nil {
					t.Fatal(err)
				}
			}
			client := docker.NewCompose(file)
			if tc.generated {
				client.EnvFiles = ComposeEnvFiles(dir)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			out, err := client.OutputProject(ctx, dir, docker.ProjectRun{Project: tc.project}, "config", "--format", "json")
			if err != nil {
				t.Fatal(err)
			}
			var resolved struct {
				Name    string
				Volumes map[string]struct{ Name string }
			}
			if err := json.Unmarshal(out, &resolved); err != nil {
				t.Fatal(err)
			}
			if resolved.Name != tc.want || resolved.Volumes["postgres_data"].Name != tc.want+"_postgres_data" {
				t.Fatalf("resolved name=%q postgres volume=%q, want prefix %q", resolved.Name, resolved.Volumes["postgres_data"].Name, tc.want)
			}
		})
	}
}
