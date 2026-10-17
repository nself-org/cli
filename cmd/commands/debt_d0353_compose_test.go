package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/docker"
)

func TestD0353RealComposeRenderedHash(t *testing.T) {
	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		t.Skip("Docker Compose plugin unavailable; deterministic lifecycle interpolation regression still runs")
	}
	dir := t.TempDir()
	t.Setenv("ENV", "dev")
	t.Setenv("PROJECT_NAME", "")
	t.Setenv("D0353_VALUE", "shell-conflict")
	for name, value := range map[string]string{
		".env":               "PROJECT_NAME=d0353\nD0353_VALUE=base\n",
		".env.dev":           "D0353_VALUE=dev-conflict\n",
		".nself/compose.env": "D0353_VALUE=running-definition\n",
		"docker-compose.yml": "services:\n  hasura:\n    image: example.invalid/hasura:fixture\n    environment:\n      JWT_KEY: ${D0353_VALUE}\n",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files := []string{filepath.Join(dir, "docker-compose.yml")}
	envFiles := build.ComposeEnvFiles(dir)
	var baseline map[string]string
	if err := config.WithComposeEnvironment(func() error {
		var err error
		baseline, err = docker.ComposeConfigHashes(context.Background(), files, envFiles, dir)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(dir); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := config.WithComposeEnvironment(func() error {
		var err error
		got, err = docker.ComposeConfigHashes(context.Background(), files, envFiles, dir)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got["hasura"] == "" || got["hasura"] != baseline["hasura"] {
		t.Fatalf("loaded dev cascade changed actual Compose hash: got %v, want %v", got, baseline)
	}
}
