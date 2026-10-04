package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// stubDockerOnPath puts a stub `docker` first on PATH for the rest of the test
// and returns the path of the file the stub appends its arguments to.
//
// Start-command tests reach the compose step. Without a stub they run the
// host's real docker: a compose child can outlive the test and hold t.TempDir()
// open, which fails cleanup on windows-2022 (D-0125), and the outcome depends
// on whether the runner has Docker. The stub records every invocation, prints
// "Cannot connect to the Docker daemon" on stderr and exits 1, so the command
// fails the same way on every OS, quickly, with no child process left behind.
//
// It uses t.Setenv, so the test must not run in parallel; PATH is restored when
// the test ends. On Windows the stub is docker.bat, elsewhere an executable sh
// script.
func stubDockerOnPath(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "docker-calls.log")

	name, body := "docker", "#!/bin/sh\necho \"$*\" >> '"+logPath+"'\necho 'Cannot connect to the Docker daemon' 1>&2\nexit 1\n"
	if runtime.GOOS == "windows" {
		name = "docker.bat"
		body = "@echo %* >> \"" + logPath + "\"\r\n@echo Cannot connect to the Docker daemon 1>&2\r\n@exit /b 1\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatalf("writing stub docker: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// TestStubDockerOnPath checks the helper: docker resolves to the stub, running
// it fails with the daemon message, and the call is recorded in the log.
func TestStubDockerOnPath(t *testing.T) {
	logPath := stubDockerOnPath(t)

	found, err := exec.LookPath("docker")
	if err != nil {
		t.Fatalf("docker not found on PATH after stub: %v", err)
	}
	if filepath.Dir(found) != filepath.Dir(logPath) {
		t.Fatalf("docker resolved to %s, want the stub in %s", found, filepath.Dir(logPath))
	}

	out, runErr := exec.Command("docker", "compose", "up").CombinedOutput()
	if runErr == nil {
		t.Fatal("stub docker exited 0, want exit 1")
	}
	if !strings.Contains(string(out), "Cannot connect to the Docker daemon") {
		t.Fatalf("stub output = %q", out)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading stub log: %v", err)
	}
	if !strings.Contains(string(logged), "compose up") {
		t.Fatalf("stub log = %q, want the recorded arguments", logged)
	}
}
