package docker

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDoctorDockerVersionAndRestart(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$DOCKER_LOG\"\nif [ \"$1\" = version ]; then echo 27.3.1; fi\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_LOG", log)
	got, err := ServerVersion(context.Background())
	if err != nil || got != "27.3.1" {
		t.Fatalf("version %q, %v", got, err)
	}
	if err := RestartContainer(context.Background(), "demo_redis"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "version --format {{.Server.Version}}\nrestart demo_redis\n" {
		t.Fatalf("argv: %s", raw)
	}
}
