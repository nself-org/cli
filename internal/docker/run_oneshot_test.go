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
	})
	if err != nil {
		t.Fatalf("RunOneShot: %v", err)
	}
	if !strings.Contains(stdout, "out-text") || !strings.Contains(stderr, "err-text") {
		t.Errorf("output not captured: %q / %q", stdout, stderr)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	want := "run --rm --user 1000:1000 --network acme-net -v /srv/ssl:/ssl -v /x/s.sh:/s.sh:ro -e B_NAME -e SECRET_VAL example/img:1@sha256:abc run --domains a.example.org"
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
