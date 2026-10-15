package docker

// run_oneshot_test.go — RunOneShot and ExecCapture with a fake docker on PATH.
// Properties: a secret value is never in argv; the docker socket is refused;
// stdout, stderr and the exit status come back as the container produced them.

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// installRecordingDocker puts a `docker` script on PATH that records argv and
// the value of SECRET_VAL to $dir, prints OUT on stdout and ERR on stderr, and
// exits with $FAKE_RC.
func installRecordingDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > \"$FAKE_DIR/argv\"\necho \"$SECRET_VAL\" > \"$FAKE_DIR/env\"\n" +
		"echo out-text\necho err-text >&2\nexit ${FAKE_RC:-0}\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

func TestRunOneShot_SecretOnlyInEnv(t *testing.T) {
	dir := installRecordingDocker(t)
	const secret = "s3cr3t-token-value"
	stdout, stderr, err := RunOneShot(context.Background(), RunSpec{
		Image:   "example/img:1@sha256:abc",
		Args:    []string{"run", "--domains", "a.example.org"},
		EnvPass: map[string]string{"SECRET_VAL": secret, "B_NAME": "x"},
		Mounts:  []Mount{{Source: "/srv/ssl", Destination: "/ssl"}, {Source: "/x/s.sh", Destination: "/s.sh", ReadOnly: true}},
		User:    "1000:1000",
		Network: "acme-net",
		Name:    "acme-run",
	})
	if err != nil {
		t.Fatalf("RunOneShot: %v", err)
	}
	if !strings.Contains(stdout, "out-text") || !strings.Contains(stderr, "err-text") {
		t.Errorf("output not captured: %q / %q", stdout, stderr)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	want := "run --rm --name acme-run --user 1000:1000 --network acme-net -v /srv/ssl:/ssl -v /x/s.sh:/s.sh:ro -e B_NAME -e SECRET_VAL example/img:1@sha256:abc run --domains a.example.org"
	if got := strings.TrimSpace(string(argv)); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if strings.Contains(string(argv), secret) {
		t.Error("secret value leaked into argv")
	}
	env, _ := os.ReadFile(filepath.Join(dir, "env"))
	if strings.TrimSpace(string(env)) != secret {
		t.Errorf("secret not delivered through the client environment: %q", env)
	}
}

func TestRunOneShot_RefusesSocketMount(t *testing.T) {
	_, err := buildRunArgs(RunSpec{Image: "i", Mounts: []Mount{{Source: "/var/run/docker.sock", Destination: "/s"}}})
	if err == nil || !strings.Contains(err.Error(), "Docker socket") {
		t.Fatalf("want socket refusal, got %v", err)
	}
}

func TestRunOneShot_NonZeroExit(t *testing.T) {
	installRecordingDocker(t)
	t.Setenv("FAKE_RC", "3")
	_, stderr, err := RunOneShot(context.Background(), RunSpec{Image: "i"})
	if err == nil || !strings.Contains(err.Error(), "err-text") || !strings.Contains(stderr, "err-text") {
		t.Fatalf("want error quoting stderr, got %v / %q", err, stderr)
	}
}

func TestBuildRunArgs_Hardening(t *testing.T) {
	args, err := buildRunArgs(RunSpec{Image: "img:1", Args: []string{"x"}, PullNever: true, ReadOnly: true,
		CapDrop: []string{"ALL"}, SecurityOpt: []string{"no-new-privileges:true"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "run --rm --pull never --read-only --cap-drop ALL --security-opt no-new-privileges:true img:1 x"
	if got := strings.Join(args, " "); got != want {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

// installHangingDocker puts a `docker` script on PATH whose `run` records its
// argv, signals that it started and then blocks; `rm` appends its argv to
// $FAKE_DIR/rm. It is the stand-in for a one-shot container that outlives a
// cancelled client.
func installHangingDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\nif [ \"$1\" = rm ]; then echo \"$@\" >> \"$FAKE_DIR/rm\"; exit 0; fi\n" +
		"echo \"$@\" > \"$FAKE_DIR/run\"\ntouch \"$FAKE_DIR/started\"\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

// runCancelled runs spec against the hanging fake docker, cancels the context
// once the fake has started, and returns the fake's dir and RunOneShot's error.
func runCancelled(t *testing.T, spec RunSpec) (string, error) {
	t.Helper()
	dir := installHangingDocker(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := RunOneShot(ctx, spec)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "started")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fake docker run never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		return dir, err
	case <-time.After(10 * time.Second):
		t.Fatal("RunOneShot did not return after cancel")
		return dir, nil
	}
}

// A cancelled run must force-remove its container: killing only the docker
// client leaves the container running against the live database.
func TestRunOneShot_CancelRemovesContainer(t *testing.T) {
	dir, err := runCancelled(t, RunSpec{Image: "i"})
	if err == nil {
		t.Fatal("want an error after cancel")
	}
	run, _ := os.ReadFile(filepath.Join(dir, "run"))
	fields := strings.Fields(string(run))
	name := ""
	for i, f := range fields {
		if f == "--name" && i+1 < len(fields) {
			name = fields[i+1]
		}
	}
	if name == "" {
		t.Fatalf("run argv has no generated --name: %q", run)
	}
	rm, _ := os.ReadFile(filepath.Join(dir, "rm"))
	if got := strings.TrimSpace(string(rm)); got != "rm -f "+name {
		t.Errorf("cleanup = %q, want %q", got, "rm -f "+name)
	}
}

// A caller-chosen name is kept and is the one removed on cancel.
func TestRunOneShot_CancelRemovesCallerNamedContainer(t *testing.T) {
	dir, _ := runCancelled(t, RunSpec{Image: "i", Name: "caller-named"})
	rm, _ := os.ReadFile(filepath.Join(dir, "rm"))
	if got := strings.TrimSpace(string(rm)); got != "rm -f caller-named" {
		t.Errorf("cleanup = %q, want rm -f caller-named", got)
	}
}

// A run that finishes on its own must not call docker rm.
func TestRunOneShot_NoCleanupOnSuccess(t *testing.T) {
	dir := installRecordingDocker(t)
	if _, _, err := RunOneShot(context.Background(), RunSpec{Image: "i"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "rm")); err == nil {
		t.Error("docker rm ran after a successful run")
	}
}
