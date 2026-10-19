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
	pinOneShotName(t, "fixed-name")
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
	want := "run --rm --name fixed-name --user 1000:1000 --network acme-net -v /srv/ssl:/ssl -v /x/s.sh:/s.sh:ro -e B_NAME -e SECRET_VAL example/img:1@sha256:abc run --domains a.example.org"
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

// pinOneShotName fixes the generated container name for the test.
func pinOneShotName(t *testing.T, name string) {
	t.Helper()
	prev := newOneShotName
	newOneShotName = func() string { return name }
	t.Cleanup(func() { newOneShotName = prev })
}

// installHangingDocker puts a `docker` on PATH whose `run` records its argv
// and blocks, and whose `rm` records its argv to $FAKE_DIR/rm.
func installHangingDocker(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a POSIX shell script")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\ncase \"$1\" in\nrun) echo \"$@\" > \"$FAKE_DIR/run\"; exec sleep 30;;\nrm) echo \"$@\" > \"$FAKE_DIR/rm\";;\nesac\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DIR", dir)
	return dir
}

// TestRunOneShot_CancelRemovesContainer: a cancelled run force-removes the
// named container (D-0712, D-0724), whether the name was generated or the
// caller's.
func TestRunOneShot_CancelRemovesContainer(t *testing.T) {
	cases := map[string]struct{ name, want string }{
		"generated name": {"", "gen-name"},
		"caller's name":  {"caller-name", "caller-name"},
	}
	for label, tc := range cases {
		t.Run(label, func(t *testing.T) {
			dir := installHangingDocker(t)
			pinOneShotName(t, "gen-name")
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			go func() { // cancel once the fake docker has seen the run
				for i := 0; i < 600; i++ {
					if _, err := os.Stat(filepath.Join(dir, "run")); err == nil {
						time.Sleep(100 * time.Millisecond)
						cancel()
						return
					}
					time.Sleep(50 * time.Millisecond)
				}
				cancel()
			}()
			start := time.Now()
			_, _, err := RunOneShot(ctx, RunSpec{Image: "img", Name: tc.name})
			if err == nil {
				t.Fatal("want an error from the cancelled run")
			}
			if time.Since(start) > 10*time.Second {
				t.Errorf("cancelled run took %v", time.Since(start))
			}
			run, _ := os.ReadFile(filepath.Join(dir, "run"))
			if !strings.Contains(string(run), "--name "+tc.want+" ") {
				t.Errorf("run argv %q lacks --name %s", run, tc.want)
			}
			rm, _ := os.ReadFile(filepath.Join(dir, "rm"))
			if got := strings.TrimSpace(string(rm)); got != "rm -f "+tc.want {
				t.Errorf("docker rm argv = %q, want %q", got, "rm -f "+tc.want)
			}
		})
	}
}

// TestRunOneShot_NoRemoveOnSuccess: a run that finishes on its own is left to --rm.
func TestRunOneShot_NoRemoveOnSuccess(t *testing.T) {
	dir := installRecordingDocker(t)
	if _, _, err := RunOneShot(context.Background(), RunSpec{Image: "i"}); err != nil {
		t.Fatal(err)
	}
	argv, _ := os.ReadFile(filepath.Join(dir, "argv"))
	if !strings.HasPrefix(string(argv), "run ") {
		t.Errorf("last docker call = %q, want only the run", argv)
	}
}

// TestRunOneShot_PreCancelledRemovesNothing: a context that ended before docker
// started leaves a caller-named container (which may be another run's) alone.
func TestRunOneShot_PreCancelledRemovesNothing(t *testing.T) {
	dir := installHangingDocker(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := RunOneShot(ctx, RunSpec{Image: "img", Name: "someone-elses"}); err == nil {
		t.Fatal("want an error")
	}
	if _, err := os.Stat(filepath.Join(dir, "rm")); err == nil {
		t.Error("docker rm ran although the run never started")
	}
}
