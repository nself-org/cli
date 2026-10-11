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
