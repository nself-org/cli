// Package dockertest decides whether a test that needs a Linux docker daemon
// runs, skips or fails on the current host (D-0286).
//
// A test that starts containers calls Require first. On a GitHub Actions Linux
// runner docker ships in the image, so an unusable daemon there is a broken
// runner and the test fails. Everywhere else (a laptop with Colima stopped, the
// cam CI host whose cam-ci user cannot open the socket, macOS and Windows
// runners) an unusable daemon skips the test with the reason.
//
// Layer: L0. Imports the standard library only. Lives under internal/docker so
// its `docker info` probe stays inside the docker funnel (repoqa TestDockerFunnel).
package dockertest

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// probeTimeout caps the `docker info` probe.
const probeTimeout = 15 * time.Second

// Require skips t when no usable Linux docker daemon is reachable, and fails t
// instead when Strict reports a GitHub Actions Linux runner. It returns only
// when `docker info` answers with a Linux daemon.
func Require(t testing.TB) {
	t.Helper()
	why := probeDocker()
	if why == "" {
		return
	}
	if Strict(runtime.GOOS, os.Getenv("GITHUB_ACTIONS")) {
		t.Fatalf("docker is required on a GitHub Linux runner: %s", why)
		return
	}
	t.Skipf("docker unusable on this host: %s", why)
}

// infraSigns are fragments of errors that mean the daemon or its shim is
// unusable on this host. Errors that can also come from the test's own
// container spec (a bad mount source, a missing binary: "oci runtime create
// failed", "failed to create task") are deliberately NOT listed: they must
// stay failures.
var infraSigns = []string{
	"failed to create shim task",
	"cannot connect to the docker daemon",
	"permission denied while trying to connect to the docker daemon",
}

// RuntimeFailure is for a test whose `docker run` itself failed. When the error
// text shows the host cannot start containers it skips t (or fails on a GitHub
// Linux runner, like Require, and under INTEGRATION=1) and returns true; otherwise it returns false and
// the caller reports the failure as a real one. Matching is case-insensitive.
func RuntimeFailure(t testing.TB, err error, stderr string) bool {
	t.Helper()
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error() + "\n" + stderr)
	for _, sign := range infraSigns {
		if strings.Contains(text, sign) {
			if Strict(runtime.GOOS, os.Getenv("GITHUB_ACTIONS")) || os.Getenv("INTEGRATION") == "1" {
				t.Fatalf("docker cannot start a container on a GitHub Linux runner or under INTEGRATION=1: %v\n%s", err, stderr)
				return true
			}
			t.Skipf("docker cannot start a container on this host: %v", err)
			return true
		}
	}
	return false
}

// Strict reports whether an unusable docker must fail rather than skip: only on
// a GitHub Actions Linux runner (GITHUB_ACTIONS=true, the exact value GitHub
// sets). CI alone is not enough: the cam CI host sets CI=1 for a user without
// socket access.
func Strict(goos, githubActions string) bool {
	return goos == "linux" && githubActions == "true"
}

// probeDocker is probe; tests replace it.
var probeDocker = probe

// probe returns "" when `docker info` reports a Linux daemon, else the reason
// the daemon is unusable.
func probe() string {
	if _, err := exec.LookPath("docker"); err != nil {
		return "docker CLI not found: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.OSType}}").CombinedOutput()
	return reason(strings.TrimSpace(string(out)), err)
}

// reason turns the probe's output and error into "" (usable) or a reason.
func reason(out string, err error) string {
	switch {
	case err != nil:
		return "docker info: " + err.Error() + ": " + out
	case out != "linux":
		return "docker daemon OS type is " + out + ", want linux"
	default:
		return ""
	}
}
