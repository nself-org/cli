package dockertest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ProjectEnv installs a docker stub and a project with computed interpolation
// values. Check verifies every recorded call, including calls whose errors the
// caller intentionally ignores. The stub never contacts a daemon.
func ProjectEnv(t testing.TB, output string) (root string, check func()) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("docker stub uses a shell script")
	}
	root = t.TempDir()
	bin := t.TempDir()
	log := filepath.Join(root, "calls")
	for name, content := range map[string]string{
		".env":               "PROJECT_NAME=demo\n",
		".nself/compose.env": "COMPUTED=present\n",
		"docker-compose.yml": "services: {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NSELF_ENV_CALLS", log)
	t.Setenv("NSELF_ENV_OUTPUT", output)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$NSELF_ENV_CALLS\"\nprintf '%s\\n' \"$NSELF_ENV_OUTPUT\"\n"
	if err := os.WriteFile(filepath.Join(bin, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	} //nolint:gosec // executable test stub
	return root, func() {
		t.Helper()
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		flags := "--env-file " + filepath.Join(root, ".env") + " --env-file " + filepath.Join(root, ".nself/compose.env")
		for _, call := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			if !strings.HasPrefix(call, "compose ") {
				continue
			}
			if !strings.Contains(call, flags) {
				t.Errorf("compose call missing project env files: %s", call)
			}
		}
	}
}
