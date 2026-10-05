//go:build integration

package requires

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
)

// startPostgres runs image as <project>_postgres with trust auth on the local
// socket (no password anywhere) and waits until it accepts connections.
func startPostgres(t *testing.T, project, image string) {
	t.Helper()
	name := project + "_postgres"
	out, err := exec.Command("docker", "run", "-d", "--name", name,
		"-e", "POSTGRES_HOST_AUTH_METHOD=trust", image).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", image, err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for {
		// Two consecutive successes: the image's init server restarts once.
		if _, _, err := docker.ExecCapture(ctx, name, []string{"psql", "-X", "-U", "postgres", "-c", "SELECT 1"}); err == nil {
			time.Sleep(2 * time.Second)
			if _, _, err := docker.ExecCapture(ctx, name, []string{"psql", "-X", "-U", "postgres", "-c", "SELECT 1"}); err == nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("%s never became ready", image)
		case <-time.After(time.Second):
		}
	}
}

// TestRequiresIntegration: against real containers, Available() lacks vector
// on postgres:16-alpine and has it on pgvector/pgvector:pg16, and Check
// refuses with E507 on the first and passes on the second.
func TestRequiresIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 (needs a Docker daemon)")
	}
	for _, tc := range []struct {
		image      string
		wantVector bool
	}{
		{"postgres:16-alpine", false},
		{"pgvector/pgvector:pg16", true},
	} {
		t.Run(tc.image, func(t *testing.T) {
			project := fmt.Sprintf("a06it%d", time.Now().UnixNano()%1000000)
			startPostgres(t, project, tc.image)
			cfg := &config.Config{ProjectName: project, Postgres: config.PostgresConfig{User: "postgres", DB: "postgres", Image: tc.image}}
			probe := DockerProbe(cfg)
			ctx := context.Background()
			if ok, err := probe.Running(ctx); err != nil || !ok {
				t.Fatalf("Running = %v, %v", ok, err)
			}
			have, err := probe.Available(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if have["vector"] != tc.wantVector || !have["pgcrypto"] {
				t.Fatalf("vector=%v pgcrypto=%v, want vector=%v", have["vector"], have["pgcrypto"], tc.wantVector)
			}
			err = Check(ctx, cfg, []string{"vector"}, nil)
			var ce *errs.CLIError
			if tc.wantVector && err != nil {
				t.Fatalf("Check: %v", err)
			}
			if !tc.wantVector && (!errors.As(err, &ce) || ce.Code != "E507") {
				t.Fatalf("Check: want E507, got %v", err)
			}
			// A stopped database falls back to the image table.
			if out, err := exec.Command("docker", "stop", project+"_postgres").CombinedOutput(); err != nil {
				t.Fatalf("stop: %v %s", err, out)
			}
			err = Check(ctx, cfg, []string{"vector"}, nil)
			if tc.wantVector && err != nil {
				t.Fatalf("stopped Check: %v", err)
			}
			if !tc.wantVector && (!errors.As(err, &ce) || ce.Code != "E507") {
				t.Fatalf("stopped Check: want E507, got %v", err)
			}
		})
	}
}
