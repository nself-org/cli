package commands

// Purpose: security gates of the config fragment (P7-SURF-12): the real age
// store, the failure paths of the two verdict rows, and the machine surface
// (internal/invoke) with `--reveal` smuggled through args.
// Inputs: the built binary (pilot harness) and the machine registry, run
// against the fixture project of json_config_test.go with synthetic values.
// Constraints: no network; HOME is a temp dir; the invoke sweep runs read
// rows only.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
)

// requireAgeLoud skips loudly when the age binaries are missing; with
// NSELF_REQUIRE_AGE=1 (set it in CI) a missing binary fails instead, so the
// secrets rows can never pass unchecked.
func requireAgeLoud(t *testing.T) {
	t.Helper()
	for _, bin := range []string{"age", "age-keygen"} {
		if _, err := exec.LookPath(bin); err != nil {
			if os.Getenv("NSELF_REQUIRE_AGE") == "1" {
				t.Fatalf("%s is not on PATH and NSELF_REQUIRE_AGE=1", bin)
			}
			t.Skipf("SKIPPED: %s is not on PATH, so the config secrets rows were NOT exercised (install age; set NSELF_REQUIRE_AGE=1 to fail instead)", bin)
		}
	}
}

// TestConfigSecretsStoreHoldsNoSecret plants a value in a real age store with
// `config secrets set` and greps every row that reads config or the store.
func TestConfigSecretsStoreHoldsNoSecret(t *testing.T) {
	requireAgeLoud(t)
	proj := cfgFixtureProject(t)
	const storeVal = "fake-store-real-surf12-0000"
	needles := append(append([]string{}, cfgNeedles...), storeVal)
	clean := func(label string, r pilotRun) {
		t.Helper()
		for _, v := range needles {
			if strings.Contains(r.stdout, v) || strings.Contains(r.stderr, v) {
				t.Errorf("%s: output holds %q", label, v)
			}
		}
	}
	run := func(argv ...string) pilotRun {
		t.Helper()
		return runPilot(t, proj, true, nil, append(argv, "--json")...)
	}
	for _, argv := range [][]string{{"config", "secrets", "init"}, {"config", "secrets", "set", "STORE_ITEM", storeVal}} {
		if r := run(argv...); r.code != 0 {
			t.Fatalf("%v: exit %d\n%s", argv, r.code, r.stderr)
		}
	}
	// Positive controls: the two cli-only rows print the stored value.
	for _, argv := range [][]string{{"config", "secrets", "get", "STORE_ITEM"}, {"config", "secrets", "decrypt-on-deploy"}} {
		if r := run(argv...); r.code != 0 || !strings.Contains(r.stdout, storeVal) {
			t.Fatalf("positive control %v: exit %d stdout %q lacks the store value", argv, r.code, r.stdout)
		}
	}
	rows := [][]string{
		{"config", "secrets", "list"}, {"config", "secrets", "verify", "STORE_ITEM"}, {"config", "secrets", "audit"},
		{"config", "secrets", "lint"}, {"config", "secrets", "schedule"}, {"config", "secrets", "list-schedules"},
		{"config", "secrets", "rotation-log"}, {"config", "show"}, {"config", "list"}, {"config", "get", "DATABASE_URL"},
		{"config", "env", "show"}, {"config", "env", "diff", "dev", "prod"}, {"config", "env", "explain", "POSTGRES_PASSWORD"},
		{"config", "explain", "POSTGRES_PASSWORD"}, {"config", "validate"},
	}
	for _, argv := range rows {
		r := run(argv...)
		clean(strings.Join(argv, " "), r)
		if len(argv) > 2 && argv[1] == "secrets" && argv[2] == "list" && !strings.Contains(r.stdout, "STORE_ITEM") {
			t.Errorf("secrets list lacks the secret name: %s", r.stdout)
		}
	}
	rot := run("config", "secrets", "rotate", "STORE_ITEM")
	clean("config secrets rotate", rot)
	if strings.Contains(rot.stderr, "New value") {
		t.Errorf("rotate prints the new value in envelope mode: %s", rot.stderr)
	}
}

// TestConfigVerdictFailurePaths: config validate exits 1 and config trust
// status exits 2 (D-0430) on a failed verdict, and each still writes exactly
// one envelope. A project
// that cannot even load is the error-envelope path (one document, exit 1).
func TestConfigVerdictFailurePaths(t *testing.T) {
	proj := cfgFixtureProject(t)
	// An nself.yaml with an unknown key is an error finding in v1.5 mode.
	if err := os.WriteFile(filepath.Join(proj, "nself.yaml"), []byte("surf12_unknown_key: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	val := runPilot(t, proj, true, nil, "config", "validate", "--json")
	if val.code != 1 {
		t.Fatalf("config validate on a failing project: exit %d, want 1\n%s", val.code, val.stderr)
	}
	data := validateEnvelope(t, val.stdout, "config validate")
	if data["valid"] != false || data["error_count"] == float64(0) {
		t.Fatalf("validate data = %v, want valid:false and errors", data)
	}
	if findings, _ := data["manifest_findings"].([]any); len(findings) == 0 {
		t.Fatalf("validate data lists no manifest finding: %v", data)
	}
	cfgAssertClean(t, "config validate failure", val.stdout+val.stderr)

	unloadable := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(unloadable, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "PROJECT_NAME=smoke\nENV=prod\nBASE_DOMAIN=example.test\nPOSTGRES_PASSWORD=change_me_surf12\n"
	for _, f := range []string{".env", ".env.prod"} {
		if err := os.WriteFile(filepath.Join(unloadable, f), []byte(bad), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	load := runPilot(t, unloadable, true, nil, "config", "validate", "--json")
	if load.code != 1 {
		t.Fatalf("config validate on an unloadable project: exit %d, want 1", load.code)
	}
	// One document, no value. (The child inherits env that earlier tests left in
	// this process, so the project may load: then the verdict row answers instead.)
	_ = singleDoc(t, load.stdout)
	if strings.Contains(load.stdout, "change_me_surf12") {
		t.Fatalf("unloadable project: the value reached stdout: %s", load.stdout)
	}
	tr := runPilot(t, cfgFixtureProject(t), true, nil, "config", "trust", "status", "--json")
	if tr.code != 2 {
		t.Fatalf("config trust status with warnings: exit %d, want 2 (canon warning class, D-0430)\n%s", tr.code, tr.stderr)
	}
	if data := validateEnvelope(t, tr.stdout, "config trust status"); data["warnings"] != true {
		t.Fatalf("trust status data = %v, want warnings:true", data)
	}
}

// machineReadArgs are the positional arguments the invoke sweep gives a read
// row that needs them.
var machineReadArgs = map[string][]string{
	"config get": {"DATABASE_URL"}, "config env diff": {"dev", "prod"},
	"config env explain": {"POSTGRES_PASSWORD"}, "config explain": {"POSTGRES_PASSWORD"},
}

// TestConfigMachineSurfaceNeverPrintsSecrets drives invoke.Invoke (the path MCP
// and HTTP use: Exposure, Gate, BuildArgv, the real child) for every exposed
// read row of the fragment, with and without `--reveal` smuggled into args
// (M1: after the invoker's `--` it must stay a word), and greps the document.
func TestConfigMachineSurfaceNeverPrintsSecrets(t *testing.T) {
	bin := pilotBinary(t)
	proj := cfgFixtureProject(t)
	home := filepath.Join(filepath.Dir(proj), "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("AI_AUTO_INSTALL", "false")
	t.Setenv(invoke.SelfExecOverrideEnv, bin)
	machine, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	guard := invoke.NewGuard("")
	ran := 0
	for _, key := range configFragmentKeys(t) {
		row, ok := machine.Lookup("nself " + key)
		if !ok {
			continue
		}
		if exposed, _ := invoke.Exposure(row, invoke.TransportMCP, nil); !exposed || cmdregistry.EffectiveSideEffect(row, nil) != canon.SideEffectRead {
			continue
		}
		base := machineReadArgs[key]
		for _, args := range [][]string{base, append(append([]string{}, base...), "--reveal"), append(append([]string{}, base...), "--reveal=1")} {
			for _, transport := range []string{invoke.TransportMCP, invoke.TransportHTTP} {
				res, err := invoke.Invoke(context.Background(), machine, "nself "+key, invoke.Request{Args: args}, invoke.Options{Transport: transport, Dir: proj, Guard: guard})
				out := string(res.Stdout)
				if err != nil {
					out += err.Error()
				}
				ran++
				cfgAssertClean(t, key+" "+strings.Join(args, " ")+" over "+transport, out)
				var doc struct {
					Data struct {
						Revealed bool `json:"revealed"`
					} `json:"data"`
				}
				if json.Unmarshal(res.Stdout, &doc) == nil && doc.Data.Revealed {
					t.Errorf("%s %v over %s: revealed is true", key, args, transport)
				}
			}
		}
	}
	if ran < 40 {
		t.Errorf("only %d machine invocations ran; the sweep checked nothing", ran)
	}
	// The M1 shape exactly: the value must not come back, and it is a refusal or a plain unknown key.
	res, _ := invoke.Invoke(context.Background(), machine, "nself config explain", invoke.Request{Args: []string{"POSTGRES_PASSWORD", "--reveal"}}, invoke.Options{Transport: invoke.TransportMCP, Dir: proj, Guard: guard})
	if strings.Contains(string(res.Stdout), cfgFakePG) {
		t.Fatalf("config explain POSTGRES_PASSWORD --reveal over MCP returned the value: %s", res.Stdout)
	}
}

// TestParseExplainArgsDashes: a bare "--" ends the options, so a --reveal after
// it is a word (the invoker puts every caller arg after it).
func TestParseExplainArgsDashes(t *testing.T) {
	in, err := parseExplainArgs([]string{"--json", "--", "POSTGRES_PASSWORD", "--reveal"})
	if err != nil {
		t.Fatal(err)
	}
	if in.reveal || !in.json || len(in.words) != 2 || in.words[1] != "--reveal" {
		t.Fatalf("parsed %+v: --reveal after -- must be a word", in)
	}
	if in, _ := parseExplainArgs([]string{"POSTGRES_PASSWORD", "--reveal"}); !in.reveal {
		t.Fatal("--reveal before any -- must still be the option (CLI use)")
	}
}
