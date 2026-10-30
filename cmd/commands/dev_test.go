package commands

// dev_test.go — `nself dev` compose calls carry the project's env files
// (D-0550), and docker.ProjectEnvFiles (used where internal/build cannot be
// imported) agrees with build.ComposeEnvFiles.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/docker"
)

// devProject writes a project dir; dotenv and built select .env and
// .nself/compose.env.
func devProject(t *testing.T, dotenv, built bool) string {
	t.Helper()
	dir := t.TempDir()
	if dotenv {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if built {
		if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".nself", "compose.env"), []byte("FAKE_ONLY_IN_COMPOSE_ENV=x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestProjectEnvFilesMatchesBuildComposeEnvFiles(t *testing.T) {
	for _, tc := range []struct{ dotenv, built bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		dir := devProject(t, tc.dotenv, tc.built)
		if got, want := docker.ProjectEnvFiles(dir), build.ComposeEnvFiles(dir); !reflect.DeepEqual(got, want) {
			t.Errorf(".env=%v compose.env=%v: docker.ProjectEnvFiles = %q, build.ComposeEnvFiles = %q", tc.dotenv, tc.built, got, want)
		}
	}
}

func TestDevComposeCallsCarryEnvFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := devProject(t, true, true)
	base, plugin := filepath.Join(dir, "docker-compose.yml"), filepath.Join(dir, "plugin.yml")
	for _, path := range []string{base, plugin} {
		if err := os.WriteFile(path, []byte("services: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "compose-files.txt"), []byte(base+"\n"+plugin+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := t.TempDir()
	log := filepath.Join(stub, "log")
	bin := filepath.Join(stub, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$*\" >> '"+log+"'\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	old := newDevCompose
	newDevCompose = func(workdir string) *docker.Compose {
		c := old(workdir)
		c.DockerPath = bin
		return c
	}
	t.Cleanup(func() { newDevCompose = old })

	ctx := context.Background()
	if err := startDevStack(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := rebuildPlugin(ctx, dir, "notify"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	flags := "compose -f " + base + " -f " + plugin + " --env-file " + filepath.Join(dir, ".env") + " --env-file " + filepath.Join(dir, ".nself", "compose.env")
	want := []string{flags + " up -d --build", flags + " up -d --no-deps --build -- notify"}
	if got := strings.Split(strings.TrimRight(string(b), "\n"), "\n"); !reflect.DeepEqual(got, want) {
		t.Errorf("compose calls =\n%q\nwant\n%q", got, want)
	}
}
