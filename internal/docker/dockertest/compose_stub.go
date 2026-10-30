package dockertest

// Purpose: exercise project compose callers without a Docker daemon.
// Inputs: a test and the stdout the fake docker should return.
// Outputs: a project root and a reader of recorded argument vectors.
// Constraints: the stub rejects calls missing either project env file.
import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func ComposeProject(t testing.TB, stdout string) (string, func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("compose stub requires a POSIX shell")
	}
	root, bin := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nself"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		".env": "PROJECT_NAME=demo\n", ".nself/compose.env": "ONLY_IN_COMPOSE_ENV=value\n",
		"docker-compose.yml": "services: {}\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	log, output := filepath.Join(bin, "calls"), filepath.Join(bin, "stdout")
	if err := os.WriteFile(output, []byte(stdout), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COMPOSE_STUB_ROOT", root)
	t.Setenv("COMPOSE_STUB_LOG", log)
	t.Setenv("COMPOSE_STUB_OUTPUT", output)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$COMPOSE_STUB_LOG"
base=0
built=0
while [ "$#" -gt 0 ]; do
  if [ "$1" = --env-file ]; then
    shift
    [ "$1" = "$COMPOSE_STUB_ROOT/.env" ] && base=1
    [ "$1" = "$COMPOSE_STUB_ROOT/.nself/compose.env" ] && built=1
  fi
  shift
done
[ "$base$built" = 11 ] || { echo 'missing project env files' >&2; exit 51; }
cat "$COMPOSE_STUB_OUTPUT"
`
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return root, func() []string {
		t.Helper()
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		calls := strings.Split(strings.TrimSpace(string(b)), "\n")
		want := "--env-file " + filepath.Join(root, ".env") + " --env-file " + filepath.Join(root, ".nself/compose.env")
		for _, call := range calls {
			if !strings.Contains(call, want) {
				t.Errorf("compose call missing ordered env files: %s", call)
			}
		}
		return calls
	}
}
