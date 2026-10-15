package docker

// Tests for network.go. internal/docker has no exec seam, so a fake `docker`
// shell script goes first on PATH, logs its calls and answers per subcommand.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeNetDocker installs a docker that logs every call to the returned file
// and runs body (a POSIX shell fragment) to answer it.
func fakeNetDocker(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + log + "'\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func callLog(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

const inspectOnNets = `if [ "$1" = inspect ]; then printf '%s' '{"proj_network":{},"bridge":{}}'; exit 0; fi`

func TestNetworkConnect(t *testing.T) {
	ctx := context.Background()

	// Not connected yet: one connect call, names after "--".
	log := fakeNetDocker(t, `if [ "$1" = inspect ]; then printf '%s' '{"bridge":{}}'; exit 0; fi`)
	if err := NetworkConnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Fatal(err)
	}
	if got, want := callLog(t, log)[1], "network connect -- proj_network owner_nginx"; got != want {
		t.Errorf("connect call = %q, want %q", got, want)
	}

	// Already connected: no connect call (idempotent).
	log = fakeNetDocker(t, inspectOnNets)
	if err := NetworkConnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Fatal(err)
	}
	if n := len(callLog(t, log)); n != 1 {
		t.Errorf("an attached container made %d docker calls, want only the inspect", n)
	}

	// A race: docker says the endpoint exists. Still success.
	fakeNetDocker(t, `if [ "$1" = inspect ]; then printf '%s' '{}'; exit 0; fi
echo "Error response from daemon: endpoint with name owner_nginx already exists in network proj_network" >&2; exit 1`)
	if err := NetworkConnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Errorf("already-exists race must be success: %v", err)
	}

	// A real failure is returned with docker's line.
	fakeNetDocker(t, `if [ "$1" = inspect ]; then printf '%s' '{}'; exit 0; fi
echo "Error response from daemon: boom" >&2; exit 1`)
	if err := NetworkConnect(ctx, "proj_network", "owner_nginx"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("a docker failure must surface: %v", err)
	}
}

func TestNetworkDisconnect(t *testing.T) {
	ctx := context.Background()

	log := fakeNetDocker(t, inspectOnNets)
	if err := NetworkDisconnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Fatal(err)
	}
	if got, want := callLog(t, log)[1], "network disconnect -- proj_network owner_nginx"; got != want {
		t.Errorf("disconnect call = %q, want %q", got, want)
	}

	// Not connected: no disconnect call.
	log = fakeNetDocker(t, `if [ "$1" = inspect ]; then printf '%s' '{"bridge":{}}'; exit 0; fi`)
	if err := NetworkDisconnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Fatal(err)
	}
	if n := len(callLog(t, log)); n != 1 {
		t.Errorf("a detached container made %d docker calls, want only the inspect", n)
	}

	// The container is gone: nothing to disconnect.
	fakeNetDocker(t, `echo "Error response from daemon: No such object: owner_nginx" >&2; exit 1`)
	if err := NetworkDisconnect(ctx, "proj_network", "owner_nginx"); err != nil {
		t.Errorf("an absent container must be success: %v", err)
	}

	// Anything else surfaces.
	fakeNetDocker(t, inspectOnNets+`
echo "Error response from daemon: boom" >&2; exit 1`)
	if err := NetworkDisconnect(ctx, "proj_network", "owner_nginx"); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("a docker failure must surface: %v", err)
	}
}

func TestNetworkExists(t *testing.T) {
	ctx := context.Background()
	fakeNetDocker(t, `echo "Error response from daemon: network x not found" >&2; echo "Error: No such network: x" >&2; exit 1`)
	if ok, err := NetworkExists(ctx, "x"); err != nil || ok {
		t.Errorf("an absent network: exists %v err %v", ok, err)
	}
	fakeNetDocker(t, `printf 'x\n'`)
	if ok, err := NetworkExists(ctx, "x"); err != nil || !ok {
		t.Errorf("a present network: exists %v err %v", ok, err)
	}
	fakeNetDocker(t, `printf 'other\n'`) // id-prefix match of a different name
	if ok, _ := NetworkExists(ctx, "x"); ok {
		t.Error("a network with a different name must not count")
	}
	fakeNetDocker(t, `echo "Cannot connect to the Docker daemon" >&2; exit 1`)
	if ok, err := NetworkExists(ctx, "x"); err == nil || ok {
		t.Errorf("an unreachable daemon must not read as absent: %v %v", ok, err)
	}
}
