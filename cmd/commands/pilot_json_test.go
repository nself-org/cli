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
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/health"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/plugin"
	"github.com/spf13/cobra"
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
		cmd := exec.Command("go", "build", "-mod=vendor", "-o", pilotBin, "./cmd/nself")
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
	env = append(env, "HOME="+home, "AI_AUTO_INSTALL=false", "PATH="+stub+string(os.PathListSeparator)+os.Getenv("PATH"))
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

// ---- status ----------------------------------------------------------------

func statusReport(results ...health.HealthResult) *health.HealthReport {
	r := &health.HealthReport{Timestamp: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC), Results: results, Total: len(results)}
	for _, res := range results {
		if res.OK() {
			r.Healthy++
		} else {
			r.Unhealthy++
		}
	}
	return r
}

func TestPilotJSONStatus(t *testing.T) {
	up := health.HealthResult{Service: "postgres", Status: "healthy", Details: "ok"}
	starting := health.HealthResult{Service: "hasura", Status: "starting", Details: "starting"}
	down := health.HealthResult{Service: "auth", Status: "unhealthy", Details: "down"}

	t.Run("emit", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			cases := []struct {
				name  string
				rep   *health.HealthReport
				state string
				v15   int
			}{
				{"healthy", statusReport(up), "ok", 0},
				{"starting", statusReport(up, starting), "transitional", 11},
				{"unhealthy", statusReport(up, starting, down), "unhealthy", 10},
			}
			for _, tc := range cases {
				out, _ := pilotBuffers(t)
				code, err := printStatusJSON(tc.rep)
				if err != nil {
					t.Fatal(err)
				}
				doc := singleDoc(t, out.String())
				if _, v15 := doc["schema_version"]; v15 {
					// v1.5: envelope with state, exit code from the state.
					data := validateEnvelope(t, out.String(), "status")
					if data["state"] != tc.state || code != tc.v15 {
						t.Fatalf("%s: state=%v code=%d, want %s/%d", tc.name, data["state"], code, tc.state, tc.v15)
					}
					continue
				}
				// v1.4: the bare pre-contract payload, no state, exit 0.
				if _, has := doc["state"]; has || code != 0 {
					t.Fatalf("%s: v1.4 payload has state or requests code %d: %s", tc.name, code, out)
				}
				if _, ok := doc["services"]; !ok {
					t.Fatalf("%s: v1.4 payload lacks services: %s", tc.name, out)
				}
			}
		})
	})

	t.Run("legacy env omits state", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		t.Setenv(output.LegacyEnvVar, "1")
		out, errOut := pilotBuffers(t)
		if _, err := printStatusJSON(statusReport(up)); err != nil {
			t.Fatal(err)
		}
		doc := singleDoc(t, out.String())
		if _, has := doc["state"]; has {
			t.Fatalf("legacy output carries state: %s", out)
		}
		if _, has := doc["schema_version"]; has || !strings.Contains(errOut.String(), "NSELF_JSON_LEGACY") {
			t.Fatalf("legacy output enveloped or warning missing: %s / %s", out, errOut)
		}
	})

	t.Run("live", func(t *testing.T) {
		proj := pilotProject(t)
		r := runPilot(t, proj, true, nil, "status", "--json")
		data := validateEnvelope(t, r.stdout, "status")
		if data["state"] != "unhealthy" || r.code != 10 {
			t.Fatalf("v1.5 status --json: state=%v exit=%d, want unhealthy/10\n%s", data["state"], r.code, r.stderr)
		}
		legacy := runPilot(t, proj, true, []string{"NSELF_JSON_LEGACY=1"}, "status", "--json")
		bare := singleDoc(t, legacy.stdout)
		if !equalJSON(t, withoutKeys(bare, "timestamp"), withoutKeys(data, "timestamp", "state")) {
			t.Fatalf("NSELF_JSON_LEGACY output differs from .data minus state:\n%s", legacy.stdout)
		}
		old := runPilot(t, proj, false, nil, "status", "--json")
		if _, has := singleDoc(t, old.stdout)["schema_version"]; has || old.code != 0 {
			t.Fatalf("v1.4 status --json must stay bare with exit 0, got exit %d\n%s", old.code, old.stdout)
		}
		human := runPilot(t, proj, true, nil, "status")
		if human.code != 10 {
			t.Fatalf("v1.5 human status exit = %d, want 10", human.code)
		}
		if h14 := runPilot(t, proj, false, nil, "status"); h14.code != 2 {
			t.Fatalf("v1.4 human status exit = %d, want 2", h14.code)
		}
	})
}

// stableDoctor drops what legitimately differs between two runs of doctor: the
// timestamp and the free-disk figure in the "Disk space" message.
func stableDoctor(doc map[string]any) map[string]any {
	out := withoutKeys(doc, "timestamp")
	checks, _ := out["checks"].([]any)
	stable := make([]any, 0, len(checks))
	for _, c := range checks {
		row, _ := c.(map[string]any)
		if strings.HasPrefix(row["name"].(string), "Disk space") {
			row = withoutKeys(row, "message")
		}
		stable = append(stable, row)
	}
	out["checks"] = stable
	return out
}

// equalJSON compares two decoded documents by their canonical encoding.
func equalJSON(t *testing.T, a, b map[string]any) bool {
	t.Helper()
	x, err1 := json.Marshal(a)
	y, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	return bytes.Equal(x, y)
}

// ---- doctor ----------------------------------------------------------------

func doctorFixture(statuses ...string) *doctorReport {
	var checks []doctorCheckResult
	for _, s := range statuses {
		checks = append(checks, doctorCheckResult{Name: "check " + s, Status: s, Message: "m"})
	}
	return buildDoctorReport(checks)
}

func TestPilotJSONDoctor(t *testing.T) {
	t.Run("emit", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			cases := []struct {
				name  string
				rep   *doctorReport
				state string
				v15   int
			}{
				{"pass", doctorFixture("pass"), "ok", 0},
				{"warn", doctorFixture("pass", "warn"), "warnings", 12},
				{"fail", doctorFixture("pass", "warn", "fail"), "unhealthy", 10},
			}
			for _, tc := range cases {
				out, _ := pilotBuffers(t)
				err := printDoctorJSON(tc.rep)
				doc := singleDoc(t, out.String())
				if _, v15 := doc["schema_version"]; v15 {
					data := validateEnvelope(t, out.String(), "doctor")
					var ec *plugin.ExitCodeError
					got := 0
					if errors.As(err, &ec) {
						got = ec.Code
					} else if err != nil {
						t.Fatal(err)
					}
					if data["state"] != tc.state || got != tc.v15 {
						t.Fatalf("%s: state=%v exit=%d, want %s/%d", tc.name, data["state"], got, tc.state, tc.v15)
					}
					continue
				}
				// v1.4: bare report, state absent, --json exits 0.
				if _, has := doc["state"]; has || err != nil {
					t.Fatalf("%s: v1.4 doctor --json has state or err=%v", tc.name, err)
				}
			}
		})
	})

	t.Run("human exit codes", func(t *testing.T) {
		for _, tc := range []struct {
			rep      *doctorReport
			v14, v15 int
		}{
			{doctorFixture("pass"), 0, 0},
			{doctorFixture("warn"), 2, 12},
			{doctorFixture("warn", "fail"), 1, 10},
		} {
			for v15, want := range map[bool]int{false: tc.v14, true: tc.v15} {
				compattest.Set(t, v15)
				got := 0
				var ec *plugin.ExitCodeError
				if err := doctorExit(tc.rep); errors.As(err, &ec) {
					got = ec.Code
				}
				if got != want {
					t.Fatalf("doctorExit v15=%v = %d, want %d", v15, got, want)
				}
			}
		}
	})

	t.Run("live", func(t *testing.T) {
		proj := pilotProject(t)
		for _, args := range [][]string{{"doctor", "--json"}, {"doctor", "--format", "json"}, {"doctor", "--json", "--deep"}} {
			r := runPilot(t, proj, true, nil, args...)
			data := validateEnvelope(t, r.stdout, "doctor")
			if data["state"] != "unhealthy" || r.code != 10 {
				t.Fatalf("v1.5 %v: state=%v exit=%d, want unhealthy/10\n%s", args, data["state"], r.code, r.stderr)
			}
			if strings.Contains(r.stdout, pilotSecret) {
				t.Fatalf("%v printed the secret on stdout", args)
			}
		}
		env := runPilot(t, proj, true, nil, "doctor", "--json")
		legacy := runPilot(t, proj, true, []string{"NSELF_JSON_LEGACY=1"}, "doctor", "--json")
		want := stableDoctor(withoutKeys(singleDoc(t, env.stdout)["data"].(map[string]any), "state"))
		got := stableDoctor(singleDoc(t, legacy.stdout))
		if !equalJSON(t, got, want) {
			a, _ := json.Marshal(got)
			b, _ := json.Marshal(want)
			t.Fatalf("NSELF_JSON_LEGACY output differs from .data minus state:\nlegacy %s\nwant   %s", a, b)
		}
		if h := runPilot(t, proj, true, nil, "doctor"); h.code != 10 {
			t.Fatalf("v1.5 human doctor exit = %d, want 10", h.code)
		}
		if h := runPilot(t, proj, false, nil, "doctor"); h.code != 1 {
			t.Fatalf("v1.4 human doctor exit = %d, want 1", h.code)
		}
		if old := runPilot(t, proj, false, nil, "doctor", "--json"); old.code != 0 || !strings.Contains(old.stdout, `"checks"`) {
			t.Fatalf("v1.4 doctor --json must exit 0 with the bare report, got exit %d", old.code)
		}
	})
}

// ---- config ----------------------------------------------------------------

// pilotConfigCmd is a fresh command with the config flags, parsed from args.
func pilotConfigCmd(t *testing.T, args ...string) (*cobra.Command, []string) {
	t.Helper()
	c := &cobra.Command{Use: "x"}
	c.Flags().String("env", "", "")
	c.Flags().Bool("reveal", false, "")
	c.Flags().Bool("json", false, "")
	c.Flags().String("format", "table", "")
	if err := c.Flags().Parse(args); err != nil {
		t.Fatal(err)
	}
	return c, c.Flags().Args()
}

// inDir runs fn with the working directory set to dir (the schema and golden
// paths of this package are relative, so t.Chdir for the whole test is not an
// option).
func inDir(t *testing.T, dir string, fn func()) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(old); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

func TestPilotJSONConfig(t *testing.T) {
	proj := pilotProject(t)

	t.Run("in-process", func(t *testing.T) {
		compattest.Both(t, func(t *testing.T) {
			v15 := os.Getenv("NSELF_V15") == "1"
			run := func(fn func(*cobra.Command, []string) error, args ...string) (jsonOut, human string) {
				out, _ := pilotBuffers(t)
				c, pos := pilotConfigCmd(t, args...)
				var err error
				inDir(t, proj, func() {
					human, err = captureStdout(t, func() error { return fn(c, pos) })
				})
				if err != nil {
					t.Fatal(err)
				}
				return out.String(), human
			}

			// get: masked, then revealed.
			j, h := run(runConfigGet, "POSTGRES_PASSWORD", "--json")
			if !v15 {
				if j != "" || h != "***\n" {
					t.Fatalf("v1.4 config get --json must print the human value: json=%q human=%q", j, h)
				}
			} else {
				data := validateEnvelope(t, j, "config get")
				if h != "" || data["value"] != "***" || data["masked"] != true || data["key"] != "POSTGRES_PASSWORD" || data["file"] != ".env" {
					t.Fatalf("config get data = %v (human %q)", data, h)
				}
				j, _ = run(runConfigGet, "POSTGRES_PASSWORD", "--json", "--reveal")
				data = validateEnvelope(t, j, "config get")
				if data["value"] != pilotSecret || data["masked"] != false {
					t.Fatalf("config get --reveal data = %v", data)
				}
				j, _ = run(runConfigGet, "PROJECT_NAME", "--json")
				if data = validateEnvelope(t, j, "config get"); data["masked"] != false || data["value"] != "e01" {
					t.Fatalf("non-secret config get data = %v", data)
				}
			}

			// list: --reveal never reveals, exactly like the table.
			j, h = run(runConfigList, "--json", "--reveal")
			if !v15 {
				if j != "" || !strings.HasPrefix(h, "KEY ") {
					t.Fatalf("v1.4 config list --json must print the table: json=%q", j)
				}
			} else {
				data := validateEnvelope(t, j, "config list")
				if strings.Contains(j, pilotSecret) || h != "" {
					t.Fatalf("config list leaked the secret or printed the table: %s", j)
				}
				seen := map[string]string{}
				for _, k := range data["keys"].([]any) {
					row := k.(map[string]any)
					seen[row["key"].(string)] = row["source"].(string) + ":" + row["value"].(string)
				}
				if seen["POSTGRES_PASSWORD"] != "file:***" || seen["PROJECT_NAME"] != "file:e01" ||
					seen["BASE_DOMAIN"] != "default:local.nself.org" || seen["PROJECT_DOMAIN"] != "unset:" {
					t.Fatalf("config list rows = %v", seen)
				}
			}

			// show: --json and --format json are the same envelope in v1.5;
			// --format json is the bare map in v1.4 (--json ignored).
			j, h = run(runConfigShow, "--json")
			jf, hf := run(runConfigShow, "--format", "json")
			if !v15 {
				// --format json is the bare map (written through the pilot writer).
				if j != "" || !strings.Contains(h, "POSTGRES_PASSWORD=***") || hf != "" || !strings.HasPrefix(jf, "{\n") {
					t.Fatalf("v1.4 config show: --json=%q/%q --format json=%q/%q", j, h, jf, hf)
				}
				return
			}
			if j != jf || h != "" || hf != "" {
				t.Fatalf("v1.5 config show --json and --format json differ:\n%s\n%s", j, jf)
			}
			data := validateEnvelope(t, j, "config show")
			if data["POSTGRES_PASSWORD"] != "***" || data["PROJECT_NAME"] != "e01" || strings.Contains(j, pilotSecret) {
				t.Fatalf("config show data = %v", data)
			}
		})
	})

	t.Run("live", func(t *testing.T) {
		p := pilotProject(t)
		// v1.5: schema-valid envelopes, secrets masked, --reveal honoured.
		for _, tc := range []struct {
			cmd  string
			args []string
		}{
			{"config get", []string{"config", "get", "POSTGRES_PASSWORD", "--json"}},
			{"config list", []string{"config", "list", "--json"}},
			{"config show", []string{"config", "show", "--json"}},
			{"config show", []string{"config", "show", "--format", "json"}},
		} {
			r := runPilot(t, p, true, nil, tc.args...)
			validateEnvelope(t, r.stdout, tc.cmd)
			if r.code != 0 || strings.Contains(r.stdout+r.stderr, pilotSecret) {
				t.Fatalf("%v: exit %d or secret leaked\n%s%s", tc.args, r.code, r.stdout, r.stderr)
			}
		}
		rev := runPilot(t, p, true, nil, "config", "get", "POSTGRES_PASSWORD", "--json", "--reveal")
		if validateEnvelope(t, rev.stdout, "config get")["value"] != pilotSecret {
			t.Fatalf("--reveal did not reveal: %s", rev.stdout)
		}
		legacy := runPilot(t, p, true, []string{"NSELF_JSON_LEGACY=1"}, "config", "show", "--format", "json")
		if _, has := singleDoc(t, legacy.stdout)["schema_version"]; has {
			t.Fatalf("NSELF_JSON_LEGACY must restore the bare config show --format json: %s", legacy.stdout)
		}
		// Error envelope for a missing key (v1.5); plain stderr text in v1.4.
		miss := runPilot(t, p, true, nil, "config", "get", "NOPE", "--json")
		doc := singleDoc(t, miss.stdout)
		errObj, _ := doc["error"].(map[string]any)
		if miss.code != 1 || doc["command"] != "config get" || errObj["code"] != "E400" || errObj["exit_code"] != float64(1) {
			t.Fatalf("v1.5 config get NOPE --json: exit %d, doc %v", miss.code, doc)
		}
		if err := contractValidate(contractResolve(t, "envelope.v1.schema.json"), []byte(miss.stdout)); err != nil {
			t.Fatalf("error envelope invalid: %v", err)
		}
		old := runPilot(t, p, false, nil, "config", "get", "NOPE", "--json")
		if old.code != 1 || old.stdout != "" || old.stderr != "Error: key not found: NOPE\n" {
			t.Fatalf("v1.4 config get NOPE --json: exit %d stdout=%q stderr=%q", old.code, old.stdout, old.stderr)
		}
	})

	// Human output (and v1.4 --json, which is accepted and ignored) is byte
	// identical to the base binary's, captured in testdata/json/human/.
	t.Run("human goldens", func(t *testing.T) {
		p := pilotProject(t)
		for _, tc := range []struct {
			golden string
			args   []string
		}{
			{"config-show.txt", []string{"config", "show"}},
			{"config-show.txt", []string{"config", "show", "--json"}},
			{"config-show-reveal.txt", []string{"config", "show", "--reveal"}},
			{"config-show-format-yaml.txt", []string{"config", "show", "--format", "yaml"}},
			{"config-show-format-json.txt", []string{"config", "show", "--format", "json"}},
			{"config-get-secret.txt", []string{"config", "get", "POSTGRES_PASSWORD"}},
			{"config-get-secret.txt", []string{"config", "get", "POSTGRES_PASSWORD", "--json"}},
			{"config-list.txt", []string{"config", "list"}},
			{"config-list.txt", []string{"config", "list", "--json"}},
		} {
			want, err := os.ReadFile(filepath.Join("testdata", "json", "human", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			r := runPilot(t, p, false, nil, tc.args...)
			if r.code != 0 || r.stdout+r.stderr != string(want) {
				t.Fatalf("%v differs from %s (exit %d)\n--- got ---\n%s%s\n--- want ---\n%s", tc.args, tc.golden, r.code, r.stdout, r.stderr, want)
			}
		}
	})
}
