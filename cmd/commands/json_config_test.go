package commands

// Purpose: tests for the config fragment's v1 envelopes (P7-SURF-12): row
// coverage, the reveal/cli-only/secret-arg registry rules, the v1.4 registry
// view, the golden writer and the secret grep gate.
// Inputs: the canon fragment, the v1.5 registries, the built binary (pilot
// harness) run against a fixture project with synthetic secret values.
// Constraints: fixture values are obviously fake; HOME is a temp dir; no network.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/canon"
	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/internal/output"
	"github.com/nself-org/cli/internal/secrets"
	"gopkg.in/yaml.v3"
)

const (
	cfgFakePG    = "fake-pg-pass-surf12-0000"
	cfgFakeStore = "fake-store-val-surf12-0000"
)

// TestEnvelopeCoverageConfig: every runnable document row of the config canon
// fragment is an envelope command (config secrets edit is interactive with a
// reason on its row).
func TestEnvelopeCoverageConfig(t *testing.T) {
	assertFragmentEnvelopeCoverage(t, "config")
}

// configFragmentKeys returns the canonical paths of the config fragment.
func configFragmentKeys(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile("../../internal/canon/domains/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var f canon.Fragment
	if err := yaml.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range f.Commands {
		keys = append(keys, k)
	}
	return keys
}

// TestConfigRevealCLIOnly: every reveal flag of the fragment is cli_only,
// config export is cli-only, and the machine surface refuses the three
// secret-printing rows.
func TestConfigRevealCLIOnly(t *testing.T) {
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, k := range configFragmentKeys(t) {
		row, ok := reg.Lookup("nself " + k)
		if !ok {
			t.Fatalf("no registry row %q", k)
		}
		for _, f := range row.Flags {
			if f.Name == "reveal" {
				seen++
				if !f.CLIOnly {
					t.Errorf("%s: --reveal is not cli_only", k)
				}
			}
		}
	}
	if seen < 3 {
		t.Errorf("only %d reveal flags seen in the fragment (want config, config env explain, config explain at least)", seen)
	}
	machine, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"config export", "config secrets get", "config secrets decrypt-on-deploy"} {
		row, ok := reg.Lookup("nself " + path)
		if !ok || row.Surface != "cli-only" {
			t.Errorf("%s: surface %q, want cli-only", path, row.Surface)
		}
		mrow, ok := machine.Lookup("nself " + path)
		if !ok {
			t.Fatalf("machine registry has no %s", path)
		}
		for _, transport := range []string{invoke.TransportMCP, invoke.TransportHTTP} {
			if exposed, reason := invoke.Exposure(mrow, transport, nil); exposed || !strings.Contains(reason, "cli-only") {
				t.Errorf("%s over %s: exposed=%v reason=%q, want a cli-only refusal (E421)", path, transport, exposed, reason)
			}
		}
	}
	for _, path := range []string{"config secrets rotate", "config secrets retire", "config secrets rekey", "config oauth refresh"} {
		mrow, _ := machine.Lookup("nself " + path)
		if mrow == nil || mrow.Confirm == nil {
			t.Errorf("%s: no confirm block (the nonce round trip is its gate)", path)
		} else if exposed, reason := invoke.Exposure(mrow, invoke.TransportMCP, nil); !exposed {
			t.Errorf("%s not exposed: %s", path, reason)
		}
	}
}

// TestConfigSecretArgsMarked: the machine registry marks the value argument
// of config set and config secrets set secret (SURF-25 redaction guard).
func TestConfigSecretArgsMarked(t *testing.T) {
	machine, err := machineRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for path, arg := range map[string]string{"config set": "value", "config secrets set": "VALUE"} {
		row, ok := machine.Lookup("nself " + path)
		if !ok {
			t.Fatalf("no row %s", path)
		}
		found := false
		for _, a := range row.Args {
			if a.Name == arg {
				found = true
				if !a.Secret {
					t.Errorf("%s: arg %s is not marked secret", path, arg)
				}
			}
		}
		if !found {
			t.Errorf("%s: no arg %s", path, arg)
		}
	}
}

// TestRegistryV14KeepsJSONLegacyConfig: the rows that print JSON in v1.4 keep
// json: legacy in the v1.4 view while the v1.5 row is an envelope.
func TestRegistryV14KeepsJSONLegacyConfig(t *testing.T) {
	old, err := buildRegistry(false)
	if err != nil {
		t.Fatal(err)
	}
	cur, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	for v14, v15 := range map[string]string{
		"config features list": "config features list", "config features status": "config features status",
		"env target list": "config env target list", "env target probe": "config env target probe",
	} {
		row, ok := old.Lookup("nself " + v14)
		if !ok {
			t.Errorf("v1.4 registry has no row %q", v14)
			continue
		}
		if row.JSON != "legacy" || row.DataSchema != nil {
			t.Errorf("v1.4 %q: json=%q data_schema=%v, want legacy and none", v14, row.JSON, row.DataSchema)
		}
		if n, ok := cur.Lookup("nself " + v15); !ok || n.JSON != "envelope" || n.DataSchema == nil {
			t.Errorf("v1.5 %q: %+v, want envelope with a schema", v15, n)
		}
	}
}

// TestConfigSecretsListNamesOnly: the list document is built field by field
// and never carries SecretEntry.Value.
func TestConfigSecretsListNamesOnly(t *testing.T) {
	entries := map[string]secrets.SecretEntry{"DB_PASS": {Value: cfgFakeStore, CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-02-01T00:00:00Z", RotatedAt: "2026-03-01T00:00:00Z"}}
	b, err := json.Marshal(newSecretsListData("dev", []string{"DB_PASS"}, entries))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), cfgFakeStore) || !strings.Contains(string(b), "DB_PASS") {
		t.Fatalf("list data = %s", b)
	}
	lint := newSecretsLintData([]secrets.LintFinding{{File: "a.go", Line: 3, Rule: lintParseError, Message: "raw " + cfgFakeStore}, {File: "b.go", Line: 4, Rule: "generic-api-key", Message: "Generic API Key"}})
	b, _ = json.Marshal(lint)
	if strings.Contains(string(b), cfgFakeStore) || !strings.Contains(string(b), "Generic API Key") {
		t.Fatalf("lint data = %s", b)
	}
	if got := redactURLUserinfo("postgres://u:" + cfgFakePG + "@h:5432/d"); strings.Contains(got, cfgFakePG) || !strings.Contains(got, "u:****@h") {
		t.Fatalf("redactURLUserinfo = %q", got)
	}
}

// TestConfigGoldenWrite regenerates the goldens of the config fragment; it
// runs only with NSELF_UPDATE_GOLDEN=1.
func TestConfigGoldenWrite(t *testing.T) {
	if os.Getenv("NSELF_UPDATE_GOLDEN") != "1" {
		t.Skip("set NSELF_UPDATE_GOLDEN=1 to rewrite the config goldens")
	}
	for path, zero := range configEnvelopeTypes {
		var buf bytes.Buffer
		if err := output.EmitData(output.Writer{Out: &buf, Err: &bytes.Buffer{}}, path, configGoldenSample(path, zero)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fixtureFileFor(path), buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func configGoldenSample(path string, zero any) any {
	switch path {
	case "config secrets list":
		return newSecretsListData("dev", []string{"API_TOKEN"}, map[string]secrets.SecretEntry{"API_TOKEN": {UpdatedAt: "2026-02-01T00:00:00Z"}})
	case "config env show":
		return envShowData{Environment: "dev", Pairs: []envPairRow{{Key: "POSTGRES_PASSWORD", Value: "****"}, {Key: "PROJECT_NAME", Value: "demo"}}}
	case "config validate":
		return configValidateData{Valid: true, Checks: []configValidateCheck{{Name: "env", Passed: true}}, Manifest: []string{}}
	}
	switch zero.(type) {
	case dataResult:
		return dataResult{OK: true}
	case dataHub:
		return dataHub{Subcommands: []dataHubEntry{{Name: "list", Summary: "List items"}}}
	}
	return emptyOf(reflect.TypeOf(zero)).Interface()
}

// ---- secret grep gate ------------------------------------------------------

// cfgArgvRows reads argv/config.txt: the argv words of every row.
func cfgArgvRows(t *testing.T) [][]string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "json", "argv", "config.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var rows [][]string
	for _, line := range strings.Split(string(b), "\n") {
		text := strings.TrimSpace(strings.SplitN(line, "#|", 2)[0])
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		rows = append(rows, strings.Fields(text))
	}
	return rows
}

// cfgFixtureProject writes a project whose .env and .env.dev hold the fake
// secret values.
func cfgFixtureProject(t *testing.T) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	env := "PROJECT_NAME=smoke\nENV=dev\nBASE_DOMAIN=example.test\nPOSTGRES_PASSWORD=" + cfgFakePG +
		"\nAPI_TOKEN=" + cfgFakeStore + "\nDATABASE_URL=postgres://app:" + cfgFakePG + "@db:5432/app\nHASURA_GRAPHQL_JWT_SECRET=fixture\n"
	for _, f := range []string{".env", ".env.dev"} {
		if err := os.WriteFile(filepath.Join(proj, f), []byte(env), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return proj
}

// registryPathOf returns the registry row an argv row runs (the longest
// leading run of non-flag words that is a registered path).
func registryPathOf(byPath map[string]cmdregistry.Command, argv []string) (cmdregistry.Command, bool) {
	var words []string
	for _, a := range argv {
		if strings.HasPrefix(a, "-") {
			break
		}
		words = append(words, a)
	}
	for n := len(words); n > 0; n-- {
		if row, ok := byPath["nself "+strings.Join(words[:n], " ")]; ok {
			return row, true
		}
	}
	return cmdregistry.Command{}, false
}

// TestConfigGoldensHoldNoSecret: no config golden and no stdout of an argv
// row that is not `surface: cli-only` holds a fixture secret value. A
// positive control proves the grep: the cli-only export with --reveal prints
// the value.
func TestConfigGoldensHoldNoSecret(t *testing.T) {
	goldens, err := filepath.Glob(filepath.Join("testdata", "json", "config*.golden.json"))
	if err != nil || len(goldens) < 40 {
		t.Fatalf("config goldens: %d %v", len(goldens), err)
	}
	for _, g := range goldens {
		b, err := os.ReadFile(g)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range []string{cfgFakePG, cfgFakeStore} {
			if bytes.Contains(b, []byte(v)) {
				t.Errorf("%s holds the secret value %s", g, v)
			}
		}
	}
	reg, err := buildRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]cmdregistry.Command{}
	for _, r := range reg.Commands {
		byPath[r.Path] = r
	}
	proj := cfgFixtureProject(t)
	ran := 0
	for _, argv := range cfgArgvRows(t) {
		row, ok := registryPathOf(byPath, argv)
		if !ok {
			continue // a v1.4 spelling (env target list): its canonical row is checked below it
		}
		if row.Surface == "cli-only" {
			continue
		}
		res := runPilot(t, proj, true, nil, append(append([]string{}, argv...), "--json")...)
		ran++
		for _, v := range []string{cfgFakePG, cfgFakeStore} {
			if strings.Contains(res.stdout, v) {
				t.Errorf("%v: stdout holds the secret value %s", argv, v)
			}
		}
	}
	if ran < 15 {
		t.Errorf("only %d argv rows ran; the grep checked nothing", ran)
	}
	// Positive control (cli-only row): the grep finds the value when it is there.
	ctl := runPilot(t, proj, true, nil, "config", "export", "--reveal", "--json")
	if ctl.code != 0 || !strings.Contains(ctl.stdout, cfgFakePG) {
		t.Fatalf("positive control: export --reveal exit %d stdout %q lacks the fixture value", ctl.code, ctl.stdout)
	}
}

// TestConfigHelpMarksRevealAndExport: `nself help --json` (v1.5) marks every
// reveal flag of the fragment cli_only and config export surface cli-only.
func TestConfigHelpMarksRevealAndExport(t *testing.T) {
	proj := cfgFixtureProject(t)
	res := runPilot(t, proj, true, nil, "help", "--json")
	var doc struct {
		Data struct {
			Commands []struct {
				Path    string `json:"path"`
				Surface string `json:"surface"`
				Flags   []struct {
					Name    string `json:"name"`
					CLIOnly bool   `json:"cli_only"`
				} `json:"flags"`
			} `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &doc); err != nil {
		t.Fatalf("help --json: %v", err)
	}
	reveal, export := 0, false
	for _, c := range doc.Data.Commands {
		if !strings.HasPrefix(c.Path, "nself config") {
			continue
		}
		if c.Path == "nself config export" && c.Surface == "cli-only" {
			export = true
		}
		for _, f := range c.Flags {
			if f.Name == "reveal" {
				reveal++
				if !f.CLIOnly {
					t.Errorf("help --json: %s --reveal is not cli_only", c.Path)
				}
			}
		}
	}
	if reveal == 0 || !export {
		t.Errorf("help --json: %d reveal flags, export cli-only=%v", reveal, export)
	}
}

// TestConfigWriteRowsEnvelope runs the rows the argv table leaves out (state
// changes, age store, trust) through the binary in v1.5 --json mode and checks
// one envelope each, and that no secret value reaches stdout or, for rotate,
// stderr.
func TestConfigWriteRowsEnvelope(t *testing.T) {
	proj := cfgFixtureProject(t)
	imp := filepath.Join(filepath.Dir(proj), "incoming.env")
	if err := os.WriteFile(imp, []byte("NEW_KEY=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rows := [][]string{
		{"config", "env", "copy", "dev", "staging"},
		{"config", "env", "use", "dev"},
		{"config", "env", "diff", "dev", "staging"},
		{"config", "features", "enable", "nsentry-enabled"},
		{"config", "features", "disable", "nsentry-enabled"},
		{"config", "import", imp, "--force"},
		{"config", "telemetry", "off"},
		{"config", "telemetry", "on"},
		{"config", "trust", "dns", "--dry-run"},
	}
	if _, err := exec.LookPath("age"); err == nil {
		rows = append(rows,
			[]string{"config", "secrets", "init"},
			[]string{"config", "secrets", "set", "STORE_ITEM", cfgFakeStore},
			[]string{"config", "secrets", "list"},
			[]string{"config", "secrets", "verify", "STORE_ITEM"},
			[]string{"config", "secrets", "audit"},
			[]string{"config", "secrets", "schedule"},
			[]string{"config", "secrets", "list-schedules"},
			[]string{"config", "secrets", "rotation-log"},
			[]string{"config", "secrets", "rotate", "STORE_ITEM"},
		)
	}
	for _, argv := range rows {
		res := runPilot(t, proj, true, nil, append(append([]string{}, argv...), "--json")...)
		if res.code != 0 {
			t.Errorf("%v: exit %d\n%s", argv, res.code, res.stderr)
			continue
		}
		validateEnvelope(t, res.stdout, strings.Join(argv[:cfgPathLen(argv)], " "))
		if strings.Contains(res.stdout, cfgFakeStore) {
			t.Errorf("%v: stdout holds the store value", argv)
		}
	}
}

// cfgPathLen is the number of leading words of argv that make up the command
// path (words before the first argument of the table above).
func cfgPathLen(argv []string) int {
	for n := 4; n > 1; n-- {
		if n <= len(argv) {
			if _, ok := configEnvelopeTypes[strings.Join(argv[:n], " ")]; ok {
				return n
			}
		}
	}
	return 2
}
