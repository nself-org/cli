package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// This file proves the exit contract end to end on the real binary: a stub
// `docker` on PATH fails `docker info`, and `nself start` (the one command that
// wraps ErrDockerNotRunning, before any write) is run in a temp project with a
// temp HOME in both compat modes. It is skipped under -short (the Windows CI leg
// runs -short) and when /bin/sh is missing.

var (
	buildOnce sync.Once
	builtBin  string
	buildErr  error
)

// binary builds ./cmd/nself once per test process.
func binary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "nself-exit-contract")
		if err != nil {
			buildErr = err
			return
		}
		builtBin = filepath.Join(dir, "nself")
		cmd := exec.Command("go", "build", "-mod=vendor", "-o", builtBin, ".")
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = errors.New(string(out))
		}
	})
	if buildErr != nil {
		t.Fatalf("build nself: %v", buildErr)
	}
	return builtBin
}

// startRun runs `nself start <args>` with the failing docker stub and returns
// the exit status, stdout and stderr. v15 sets NSELF_V15=1, otherwise it is unset.
func startRun(t *testing.T, bin string, v15 bool, args ...string) (int, string, string) {
	t.Helper()
	proj, stub, home := t.TempDir(), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(proj, ".env"), []byte("PROJECT_NAME=e01\nENV=dev\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"Cannot connect to the Docker daemon\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "NSELF_") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	env = append(env, "HOME="+home, "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if v15 {
		env = append(env, "NSELF_V15=1")
	}
	cmd := exec.Command(bin, append([]string{"start"}, args...)...)
	cmd.Dir, cmd.Env = proj, env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run nself: %v", err)
	}
	return code, o.String(), e.String()
}

func skipUnlessSubprocess(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("subprocess test skipped under -short")
	}
	if runtime.GOOS == "windows" {
		t.Skip("needs /bin/sh")
	}
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh unavailable")
	}
}

func TestExitContractDockerDownV15(t *testing.T) {
	skipUnlessSubprocess(t)
	code, _, stderr := startRun(t, binary(t), true)
	if code != 2 || !strings.Contains(stderr, "[E002]") {
		t.Fatalf("v1.5: want exit 2 with [E002], got %d\n%s", code, stderr)
	}
}

func TestExitContractDockerDownV14(t *testing.T) {
	skipUnlessSubprocess(t)
	code, _, stderr := startRun(t, binary(t), false)
	want := "Error: docker info failed — start Docker and try again: docker daemon is not running\n"
	if code != 1 || !strings.HasSuffix(stderr, want) || strings.Contains(stderr, "[E002]") {
		t.Fatalf("v1.4: want exit 1 and the plain line, got %d\n%s", code, stderr)
	}
}

// start is `json: none`, so --json is refused with E402; the point is that
// stdout is exactly one error envelope whose exit_code is the process status.
func TestExitContractJSONEnvelopeV15(t *testing.T) {
	skipUnlessSubprocess(t)
	code, stdout, _ := startRun(t, binary(t), true, "--json")
	if code == 0 {
		t.Fatal("expected a failure status")
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	var doc struct {
		Error struct {
			Code     string `json:"code"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := dec.Decode(&doc); err != nil || dec.More() {
		t.Fatalf("stdout is not exactly one document (err=%v):\n%s", err, stdout)
	}
	if doc.Error.ExitCode != code || len(doc.Error.Code) != 4 || doc.Error.Code[0] != 'E' {
		t.Fatalf("envelope %+v does not match exit status %d", doc.Error, code)
	}
}
