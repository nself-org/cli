package docker

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Model Compose's image reuse contract: a tagged image survives an up unless
// --build is supplied. The installed plugin context has already been upgraded.
func TestComposeUpRebuildsInstalledPlugin(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	for _, services := range [][]string{nil, {"plugin"}} {
		name := "all services"
		if len(services) > 0 {
			name = "named service"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "project-plugin-latest"), "1.0.0")
			writeFile(t, filepath.Join(dir, "plugin-version"), "2.0.0")
			bin := filepath.Join(dir, "docker")
			writeExec(t, bin, `#!/bin/sh
for arg do
 if [ "$arg" = "--build" ]; then
  cp plugin-version project-plugin-latest || exit 1
 fi
done
cp project-plugin-latest served-version
`)
			c := NewCompose("docker-compose.yml")
			c.DockerPath = bin
			if err := c.ComposeUp(context.Background(), dir, services...); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(dir, "served-version"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.TrimSpace(string(got)) != "2.0.0" {
				t.Fatalf("served plugin version %s, want upgraded 2.0.0", got)
			}
		})
	}
}
