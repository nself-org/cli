package commands

// Tests for the v1 envelope pilots: status, doctor, config show, config get,
// config list (P7-REG-09).
//
// Purpose: prove the pilots speak the contract in both compat modes.
//   - In-process tests drive the emit paths with fixture reports and a fixture
//     project, in v1.4 and v1.5 mode (compattest.Both).
//   - Live tests run the real binary (skipped under -short, like the other
//     binary tests) on a fixture project with a failing `docker` stub, and run
//     every `--json` stdout through the generated envelope and data schemas
//     (schemas/commands/<cmd>.v1.schema.json). This closes the "live output
//     validates against its schema" gap (debt D-0243) for these commands.
//   - Human (non-JSON) output of the config pilots is compared to goldens
//     captured from the base binary (testdata/json/human/).
//
// Constraints: nothing here writes outside t.TempDir; the binary is built once
// per test process.

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/cli/internal/output"
)

const pilotSecret = "pilot-fixture-value-1"

// pilotBuffers swaps the pilot output seam for buffers for the test.
func pilotBuffers(t *testing.T) (out, errOut *bytes.Buffer) {
	t.Helper()
	out, errOut = &bytes.Buffer{}, &bytes.Buffer{}
	old := pilotWriter
	pilotWriter = func() output.Writer { return output.Writer{Out: out, Err: errOut} }
	output.ResetState()
	t.Cleanup(func() { pilotWriter = old; output.ResetState() })
	return out, errOut
}

// singleDoc decodes stdout as exactly one JSON document.
func singleDoc(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil {
		t.Fatalf("stdout is not a JSON document: %v\n%s", err, stdout)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout holds more than one document (err=%v):\n%s", err, stdout)
	}
	return doc
}

// validateEnvelope checks stdout is one v1 envelope for command that validates
// against the envelope schema and the command's generated data schema, and
// returns the decoded data.
func validateEnvelope(t *testing.T, stdout, command string) map[string]any {
	t.Helper()
	doc := singleDoc(t, stdout)
	if err := contractValidate(contractResolve(t, "envelope.v1.schema.json"), []byte(stdout)); err != nil {
		t.Fatalf("stdout does not validate against the envelope schema: %v\n%s", err, stdout)
	}
	if doc["schema_version"] != "1" || doc["command"] != command {
		t.Fatalf("envelope header = %v/%v, want 1/%s", doc["schema_version"], doc["command"], command)
	}
	raw, err := json.Marshal(doc["data"])
	if err != nil {
		t.Fatal(err)
	}
	if err := contractValidate(contractResolve(t, schemaFileFor(command)), raw); err != nil {
		t.Fatalf("data does not validate against %s: %v\n%s", schemaFileFor(command), err, raw)
	}
	data, _ := doc["data"].(map[string]any)
	return data
}

// withoutKeys returns a copy of m minus the named keys.
func withoutKeys(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for k, v := range m {
		out[k] = v
	}
	for _, k := range keys {
		delete(out, k)
	}
	return out
}

// ---- live binary harness ---------------------------------------------------

var (
	pilotBuildOnce sync.Once
	pilotBin       string
	pilotBuildErr  error
)

// pilotBinary builds ./cmd/nself once per test process.
func pilotBinary(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("builds a binary; skipped under -short")
	}
	if runtime.GOOS == "windows" {
		t.Skip("the docker stub is a POSIX shell script")
	}
	pilotBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "nself-pilot")
		if err != nil {
			pilotBuildErr = err
			return
		}
		pilotBin = filepath.Join(dir, "nself")
		cmd := exec.Command("go", "build", "-mod=vendor", "-buildvcs=false", "-o", pilotBin, "./cmd/nself")
		cmd.Dir = findRepoRootForTest(t)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := cmd.CombinedOutput(); err != nil {
			pilotBuildErr = errors.New(string(out))
		}
	})
	if pilotBuildErr != nil {
		t.Fatalf("build nself: %v", pilotBuildErr)
	}
	return pilotBin
}

// pilotProject writes the fixture project: a .env with one secret.
func pilotProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := "PROJECT_NAME=e01\nENV=dev\nPOSTGRES_PASSWORD=" + pilotSecret + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

type pilotRun struct {
	code           int
	stdout, stderr string
}

// runPilot runs the binary in proj with a failing docker stub and a temp HOME.
// v15 sets NSELF_V15=1; extra are additional KEY=VALUE pairs.
func runPilot(t *testing.T, proj string, v15 bool, extra []string, args ...string) pilotRun {
	t.Helper()
	bin := pilotBinary(t)
	// HOME and the stub live next to the project, stable across runs of one
	// test, so checks that echo them print the same text every time.
	stub, home := filepath.Join(filepath.Dir(proj), "stub"), filepath.Join(filepath.Dir(proj), "home")
	if err := os.MkdirAll(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"Cannot connect to the Docker daemon\" >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(stub, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "NSELF_") && !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "PATH=") {
			env = append(env, kv)
		}
	}
	// doctor's LIC-MIGRATION-01 check calls the ping API; point it at a loopback
	// server so the run never depends on the real endpoint (it answers 429 under
	// repeated runs, which made two doctor runs differ).
	ping := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(ping.Close)
	env = append(env, "HOME="+home, "AI_AUTO_INSTALL=false", "NSELF_PING_API_URL="+ping.URL, "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	if v15 {
		env = append(env, "NSELF_V15=1")
	}
	env = append(env, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Dir, cmd.Env = proj, env
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return pilotRun{code: code, stdout: o.String(), stderr: e.String()}
}

// skipLiveUnderV15Guard skips a subprocess ("live") subtest when it runs inside
// TestPilotJSONUnderV15Guard. runPilot rebuilds the child environment without
// any NSELF_ variable and sets NSELF_V15 itself, so the guard's t.Setenv changes
// nothing for a live run: the standalone test already covers it, and running it
// twice doubled the slowest tests of the package (D-0735).
func skipLiveUnderV15Guard(t *testing.T) {
	t.Helper()
	if strings.HasPrefix(t.Name(), "TestPilotJSONUnderV15Guard/") {
		t.Skip("live runs rebuild the child environment; the standalone test covers them")
	}
}
