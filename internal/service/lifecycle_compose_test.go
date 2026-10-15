package service

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// TestLifecycleRecordedComposeInputs guards D-0703 at the Docker process
// boundary: a plugin-only service and its ${VAR} secret need both manifests.
func TestLifecycleRecordedComposeInputs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker uses a POSIX shell")
	}
	for name, run := range map[string]func(context.Context, string) error{
		"start": Start, "stop": Stop, "restart": Restart,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeLifecycleFile(t, dir, ".env", "MINIO_ROOT_PASSWORD=old-secret\n", 0o600)
			writeLifecycleFile(t, dir, "docker-compose.yml", "services: {}\n", 0o600)
			plugin := writeLifecycleFile(t, dir, ".nself/plugins/minio.yml", "services:\n  minio:\n    image: minio/minio\n    environment:\n      MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}\n", 0o600)
			env := writeLifecycleFile(t, dir, ".nself/compose.env", "MINIO_ROOT_PASSWORD=project-secret\n", 0o600)
			writeLifecycleFile(t, dir, ".nself/compose-files.txt", filepath.Join(dir, "docker-compose.yml")+"\n"+plugin+"\n", 0o600)
			writeLifecycleFile(t, dir, ".nself/compose-env-files.txt", filepath.Join(dir, ".env")+"\n"+env+"\n", 0o600)
			writeLifecycleFile(t, dir, "bin/docker", `#!/bin/sh
printf '%s\n' "$@" > "$PWD/args"
shift
plugin=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    -f) case "$2" in */plugins/minio.yml) plugin=true;; esac; shift 2 ;;
    --env-file) . "$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ "$plugin" = true ] || { echo 'plugin service missing' >&2; exit 1; }
[ "$MINIO_ROOT_PASSWORD" = project-secret ] || { echo 'secret missing' >&2; exit 1; }
`, 0o700)
			t.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("MINIO_ROOT_PASSWORD", "")
			t.Chdir(dir)
			if err := run(context.Background(), "minio"); err != nil {
				t.Fatal(err)
			}
			args, err := os.ReadFile(filepath.Join(dir, "args"))
			if err != nil {
				t.Fatal(err)
			}
			prefix := strings.Join([]string{"compose", "-f", filepath.Join(dir, "docker-compose.yml"), "-f", plugin, "--env-file", filepath.Join(dir, ".env"), "--env-file", env, ""}, "\n")
			if !strings.HasPrefix(string(args), prefix) {
				t.Fatalf("compose inputs = %s, want prefix %s", args, prefix)
			}
		})
	}
}

func TestLifecycleComposeLegacyInputs(t *testing.T) {
	dir := t.TempDir()
	base := writeLifecycleFile(t, dir, "docker-compose.yml", "services: {}\n", 0o600)
	user := writeLifecycleFile(t, dir, "docker-compose.override.yml", "services: {}\n", 0o600)
	env := writeLifecycleFile(t, dir, ".env", "PROJECT_NAME=test\n", 0o600)
	c, err := lifecycleCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.ComposeFiles, []string{base, user}) || len(c.EnvFiles) != 0 {
		t.Fatalf("legacy inputs = %+v", c)
	}
	computed := writeLifecycleFile(t, dir, ".nself/compose.env", "SECRET=computed\n", 0o600)
	writeLifecycleFile(t, dir, ".nself/compose-files.txt", "docker-compose.yml\nmissing-plugin.yml\ndocker-compose.override.yml\n", 0o600)
	c, err = lifecycleCompose(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(c.ComposeFiles, []string{base, user}) || !reflect.DeepEqual(c.EnvFiles, []string{env, computed}) {
		t.Fatalf("legacy computed inputs = %+v", c)
	}
}

func TestLifecycleComposeManifestReadErrors(t *testing.T) {
	for _, name := range []string{"compose-files.txt", "compose-env-files.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".nself", name), 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := lifecycleCompose(dir); err == nil || !strings.Contains(err.Error(), name) {
				t.Fatalf("manifest error = %v, want %s", err, name)
			}
		})
	}
}

func writeLifecycleFile(t *testing.T, dir, name, body string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	return path
}
