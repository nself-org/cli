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
// on whether the runner has Docker. The stub records every invocation and
// answers `docker info` with exit 0, so the Docker preflight passes and the
// command proceeds to the compose-file check, config load and compose. Every
// other subcommand (compose up, compose down, ...) prints "Cannot connect to
// the Docker daemon" on stderr and exits 1, so the command fails at the compose
// step the same way on every OS, quickly, with no child process left behind.
//
// It uses t.Setenv, so the test must not run in parallel; PATH is restored when
// the test ends. On Windows the stub is docker.bat, elsewhere an executable sh
// script.
func stubDockerOnPath(t *testing.T) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "docker-calls.log")

	name, body := "docker", "#!/bin/sh\necho \"$*\" >> '"+logPath+"'\ncase \"$1\" in info) exit 0;; esac\necho 'Cannot connect to the Docker daemon' 1>&2\nexit 1\n"
	if runtime.GOOS == "windows" {
		name = "docker.bat"
		body = "@echo %* >> \"" + logPath + "\"\r\n@if \"%1\"==\"info\" exit /b 0\r\n@echo Cannot connect to the Docker daemon 1>&2\r\n@exit /b 1\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatalf("writing stub docker: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// stubDockerCalls returns the stub's recorded invocations, one per line.
func stubDockerCalls(t *testing.T, logPath string) []string {
	t.Helper()
	raw, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("reading stub log: %v", err)
	}
	var calls []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			calls = append(calls, l)
		}
	}
	return calls
}

// stubDockerSawCompose reports whether the stub was asked to run a compose
// subcommand, which proves the start command got past preflight and config.
func stubDockerSawCompose(t *testing.T, logPath string) bool {
	t.Helper()
	for _, c := range stubDockerCalls(t, logPath) {
		if strings.HasPrefix(c, "compose") {
			return true
		}
	}
	return false
}

// requireReachedCompose fails unless the start command got past the Docker
// preflight, compose-file check and config load and failed at the compose step:
// the error is non-nil, is not the preflight error, and the stub was asked to
// run a compose subcommand. It makes the "must not fail early" assertions of the
// start tests meaningful.
func requireReachedCompose(t *testing.T, err error, logPath string) {
	t.Helper()
	if err == nil {
		t.Fatal("start succeeded, but the stub docker fails every compose call")
	}
	if strings.Contains(err.Error(), "docker info failed") {
		t.Fatalf("start stopped at the Docker preflight, never reached compose: %v", err)
	}
	if !strings.Contains(err.Error(), "compose") {
		t.Fatalf("start failed before the compose step: %v", err)
	}
	if !stubDockerSawCompose(t, logPath) {
		t.Fatalf("stub docker log has no compose call: %q", stubDockerCalls(t, logPath))
	}
}

// TestStubDockerOnPath checks the helper: docker resolves to the stub,
// `docker info` succeeds, other subcommands fail with the daemon message, and
// every call is recorded in the log.
func TestStubDockerOnPath(t *testing.T) {
	logPath := stubDockerOnPath(t)

	found, err := exec.LookPath("docker")
	if err != nil {
		t.Fatalf("docker not found on PATH after stub: %v", err)
	}
	if filepath.Dir(found) != filepath.Dir(logPath) {
		t.Fatalf("docker resolved to %s, want the stub in %s", found, filepath.Dir(logPath))
	}

	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Fatalf("stub docker info failed, want exit 0: %v", err)
	}
	out, runErr := exec.Command("docker", "compose", "up").CombinedOutput()
	if runErr == nil {
		t.Fatal("stub docker exited 0, want exit 1")
	}
	if !strings.Contains(string(out), "Cannot connect to the Docker daemon") {
		t.Fatalf("stub output = %q", out)
	}
	if calls := stubDockerCalls(t, logPath); len(calls) != 2 || calls[0] != "info" || calls[1] != "compose up" {
		t.Fatalf("stub log = %q, want [info, compose up]", calls)
	}
	if !stubDockerSawCompose(t, logPath) {
		t.Fatal("stubDockerSawCompose = false after a compose call")
	}
}
