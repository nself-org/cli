package invoke

// Purpose: prove the child environment, deadline, limits and kill behaviour of
// Exec with the stub child.
// Constraints: signal-dependent cases are skipped on Windows, where a child
// cannot be asked to stop.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

type stubData struct {
	Data struct {
		Argv []string          `json:"argv"`
		Env  map[string]string `json:"env"`
		Cwd  string            `json:"cwd"`
	} `json:"data"`
}

func runStub(t *testing.T, s Spec) (Result, stubData) {
	t.Helper()
	res, err := Exec(context.Background(), s)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	var d stubData
	if res.ExitCode == 0 && len(res.Stdout) > 0 {
		if err := json.Unmarshal(res.Stdout, &d); err != nil {
			t.Fatalf("stub output: %v\n%s", err, res.Stdout)
		}
	}
	return res, d
}

func TestExecChildEnv(t *testing.T) {
	useStub(t)
	t.Setenv(TokenEnv, "parent-token-value")
	t.Setenv("NSELF_V15", "0")
	t.Setenv("NSELF_NONINTERACTIVE", "0")
	t.Setenv("CI", "0")
	t.Setenv("NO_COLOR", "0")
	t.Setenv(InvokedByEnv, "attacker")
	t.Setenv(DeadlineEnv, "999999")
	t.Setenv("KEEP_ME", "kept")
	for transport, by := range map[string]string{TransportMCP: "mcp", TransportHTTP: "http", TransportHTTPStream: "http"} {
		_, d := runStub(t, Spec{Argv: []string{"x"}, Transport: transport})
		env := d.Data.Env
		for k, want := range map[string]string{"NSELF_V15": "1", "NSELF_NONINTERACTIVE": "1", "CI": "1", "NO_COLOR": "1", InvokedByEnv: by, "KEEP_ME": "kept"} {
			if env[k] != want {
				t.Errorf("%s: %s=%q, want %q", transport, k, env[k], want)
			}
		}
		if _, has := env[TokenEnv]; has {
			t.Errorf("%s: the parent's %s reached the child", transport, TokenEnv)
		}
		if _, has := env[DeadlineEnv]; has {
			t.Errorf("%s: an inherited %s must not reach a child with no timeout", transport, DeadlineEnv)
		}
	}
	// Exactly one entry per owned name, whatever the parent held.
	env, err := childEnv([]string{"CI=0", "CI=2", TokenEnv + "=x", "A=1", "noequals", DeadlineEnv + "=5"}, TransportMCP, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := map[string]int{}
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		count[k]++
	}
	for _, k := range []string{"CI", "NSELF_V15", "NSELF_NONINTERACTIVE", "NO_COLOR", InvokedByEnv, "A"} {
		if count[k] != 1 {
			t.Errorf("%s appears %d times in %v", k, count[k], env)
		}
	}
	if count[TokenEnv] != 0 || count[DeadlineEnv] != 0 || count["noequals"] != 0 {
		t.Errorf("owned or malformed entries survived: %v", env)
	}
	if _, err := childEnv(nil, "carrier-pigeon", 0); err == nil {
		t.Error("an unknown transport must be refused")
	}
}

func TestExecDeadlineEnv(t *testing.T) {
	useStub(t)
	_, d := runStub(t, Spec{Argv: []string{"x"}, Transport: TransportHTTP, Timeout: 30 * time.Second})
	ms, err := strconv.Atoi(d.Data.Env[DeadlineEnv])
	if err != nil || ms > 30000 || ms < 29000 {
		t.Errorf("%s=%q, want within 1s of 30000", DeadlineEnv, d.Data.Env[DeadlineEnv])
	}
	t.Setenv(DeadlineEnv, "12345")
	_, d = runStub(t, Spec{Argv: []string{"x"}, Transport: TransportHTTPStream})
	if v, has := d.Data.Env[DeadlineEnv]; has {
		t.Errorf("an NDJSON request has no deadline, child saw %q", v)
	}
}

func TestExecArgvIsNotAShell(t *testing.T) {
	useStub(t)
	dir := t.TempDir()
	marker := filepath.Join(dir, "pwned")
	argv := []string{"a b", "$(touch " + marker + ")", ";touch " + marker, "`touch " + marker + "`", "x\ny", "--json", ""}
	res, d := runStub(t, Spec{Argv: argv, Transport: TransportMCP, Dir: dir})
	if len(d.Data.Argv) != len(argv) {
		t.Fatalf("child got %d elements, want %d: %q (%s)", len(d.Data.Argv), len(argv), d.Data.Argv, res.Stdout)
	}
	for i := range argv {
		if d.Data.Argv[i] != argv[i] {
			t.Errorf("element %d is %q, want %q", i, d.Data.Argv[i], argv[i])
		}
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a shell metacharacter executed")
	}
	want, _ := filepath.EvalSymlinks(dir)
	if got, _ := filepath.EvalSymlinks(d.Data.Cwd); got != want {
		t.Errorf("cwd %q, want %q", got, want)
	}
	if _, err := Exec(context.Background(), Spec{Transport: TransportMCP}); err == nil {
		t.Error("an empty argv must be refused")
	}
}

func TestExecLimits(t *testing.T) {
	useStub(t, "STUBCLI_MODE", "big")
	res, err := Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP})
	if err != nil || !res.StdoutTruncated || len(res.Stdout) != MaxStdoutBytes || res.ExitCode != 0 {
		t.Errorf("stdout cap: err=%v truncated=%v len=%d exit=%d", err, res.StdoutTruncated, len(res.Stdout), res.ExitCode)
	}
	useStub(t, "STUBCLI_MODE", "text", "STUBCLI_STDERR", strings.Repeat("A", 80000)+"TAILMARK")
	res, err = Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP})
	if err != nil || len(res.Stderr) != StderrRingBytes || !bytes.HasSuffix(bytes.TrimSpace(res.Stderr), []byte("TAILMARK")) {
		t.Errorf("stderr ring: err=%v len=%d tail=%q", err, len(res.Stderr), tailOf(res.Stderr, 12))
	}
	useStub(t, "STUBCLI_MODE", "raw", "STUBCLI_STDOUT", "{}", "STUBCLI_EXIT", "7")
	res, err = Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP})
	if err != nil || res.ExitCode != 7 {
		t.Errorf("a non-zero exit is a Result, not an error: exit=%d err=%v", res.ExitCode, err)
	}
	var sink bytes.Buffer
	useStub(t, "STUBCLI_MODE", "stream")
	res, err = Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportHTTPStream, Stdout: &sink})
	if err != nil || res.Stdout != nil || strings.Count(sink.String(), "\n") != 3 {
		t.Errorf("streaming: err=%v captured=%v sink=%q", err, res.Stdout, sink.String())
	}
	t.Setenv(SelfExecOverrideEnv, filepath.Join(t.TempDir(), "missing-binary"))
	if _, err := Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP}); err == nil {
		t.Error("a binary that cannot start must be an error")
	}
}

func tailOf(b []byte, n int) string {
	if len(b) < n {
		n = len(b)
	}
	return string(b[len(b)-n:])
}

func TestExecKill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("child signalling differs on Windows")
	}
	useStub(t, "STUBCLI_MODE", "sleep")
	start := time.Now()
	res, err := Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP, Timeout: time.Second})
	if err != nil || !res.TimedOut || res.ExitCode != -1 {
		t.Fatalf("timeout: err=%v timedOut=%v exit=%d", err, res.TimedOut, res.ExitCode)
	}
	if took := time.Since(start); took > 6*time.Second {
		t.Errorf("child still alive after %s", took)
	}
	// SIGTERM is ignored: SIGKILL follows after killDelay.
	old := killDelay
	killDelay = 300 * time.Millisecond
	defer func() { killDelay = old }()
	useStub(t, "STUBCLI_MODE", "sleep-ignore")
	start = time.Now()
	res, err = Exec(context.Background(), Spec{Argv: []string{"x"}, Transport: TransportMCP, Timeout: 500 * time.Millisecond})
	if err != nil || !res.TimedOut {
		t.Fatalf("sigkill path: err=%v timedOut=%v", err, res.TimedOut)
	}
	if took := time.Since(start); took > 3*time.Second {
		t.Errorf("a child that ignores SIGTERM survived %s", took)
	}
	// Context cancellation (client gone) stops the child and returns the context error.
	killDelay = old
	useStub(t, "STUBCLI_MODE", "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start = time.Now()
	res, err = Exec(ctx, Spec{Argv: []string{"x"}, Transport: TransportHTTPStream})
	if !errors.Is(err, context.DeadlineExceeded) || res.TimedOut || time.Since(start) > 6*time.Second {
		t.Errorf("cancel: err=%v timedOut=%v after %s", err, res.TimedOut, time.Since(start))
	}
	if old != 5*time.Second {
		t.Errorf("production kill delay is %s, EPIC D2 says 5s", old)
	}
}
