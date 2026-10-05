package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func runTool(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func copyFixture(t *testing.T, rel string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(p, mustRead(t, filepath.Join(fixtureRoot, rel)), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMigrateTool(t *testing.T) {
	in := copyFixture(t, "v1/tenant.json")
	code, out, stderr := runTool(t, "-in", in)
	if code != 0 {
		t.Fatalf("convert: %d %s", code, stderr)
	}
	if _, err := manifestv2.Parse([]byte(out)); err != nil {
		t.Fatalf("output is not a valid v2 manifest: %v", err)
	}
	if out != string(mustRead(t, filepath.Join(fixtureRoot, "v2", "tenant.json"))) {
		t.Error("output differs from the committed v2 twin")
	}
	if !strings.HasSuffix(out, "}\n") || !strings.Contains(out, "\n  \"") {
		t.Error("output must be 2-space indented with a trailing newline")
	}
	// Idempotent: running it on its own output is a no-op, in print and write mode.
	if code, _, e := runTool(t, "-in", in, "-write"); code != 0 {
		t.Fatalf("write: %s", e)
	}
	if code, again, _ := runTool(t, "-in", in); code != 0 || again != out {
		t.Error("second run changed the output")
	}
	if code, _, e := runTool(t, "-in", in, "-write"); code != 0 || string(mustRead(t, in)) != out {
		t.Errorf("second -write changed the file: %s", e)
	}
	if code, _, e := runTool(t, "-in", in, "-check"); code != 0 {
		t.Errorf("-check on canonical v2: %s", e)
	}
	if code, _, _ := runTool(t, "-in", copyFixture(t, "v1/tenant.json"), "-check"); code != 1 {
		t.Errorf("-check on v1 must exit 1, got %d", code)
	}
	if code, tgt, _ := runTool(t, "-in", in, "-targets"); code != 0 || tgt != "nself-tenant\ttenant\n" {
		t.Errorf("-targets: %d %q", code, tgt)
	}
	if code, tgt, _ := runTool(t, "-in", copyFixture(t, "v1/ci.json"), "-targets"); code != 0 || tgt != "" {
		t.Errorf("-targets for a plugin without commands: %q", tgt)
	}
	if code, _, _ := runTool(t); code != 2 {
		t.Errorf("no -in must exit 2, got %d", code)
	}
}

func TestMigrateToolCompatRewritesOnlyCompatKeys(t *testing.T) {
	in := copyFixture(t, "v2/surface.json")
	var before map[string]any
	_ = json.Unmarshal(mustRead(t, in), &before)
	before["maturity"] = "planned"
	before["status"] = "wrong"
	b, _ := json.Marshal(before)
	if err := os.WriteFile(in, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, e := runTool(t, "-in", in, "-compat", "-write"); code != 0 {
		t.Fatalf("-compat: %s", e)
	}
	var after map[string]any
	_ = json.Unmarshal(mustRead(t, in), &after)
	if after["status"] != "planned" {
		t.Errorf("status not regenerated: %v", after["status"])
	}
	compat := map[string]bool{}
	for _, k := range []string{"pluginType", "binaryName", "cliCommands", "entryPoint", "runtime", "cli", "minNselfVersion", "status", "isCommercial", "licenseType", "requires_license", "tier"} {
		compat[k] = true
	}
	for k, v := range before {
		if compat[k] {
			continue
		}
		a, _ := json.Marshal(after[k])
		o, _ := json.Marshal(v)
		if string(a) != string(o) {
			t.Errorf("non-compat key %q changed: %s -> %s", k, o, a)
		}
	}
	if code, _, e := runTool(t, "-in", in, "-check"); code != 0 {
		t.Errorf("rewritten file must be canonical: %s", e)
	}
}

// TestWikiTableCurrent keeps the generated field table of the wiki page in step
// with the schema (regenerate with: manifestv2migrate -wiki .github/wiki/Plugin-Manifest.md).
func TestWikiTableCurrent(t *testing.T) {
	root := repoRoot(t)
	page, err := os.ReadFile(filepath.Join(root, ".github", "wiki", "Plugin-Manifest.md"))
	if err != nil {
		t.Fatal(err)
	}
	sch, err := os.ReadFile(filepath.Join(root, "schemas", "plugin-manifest.v2.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	table, err := fieldTable(sch)
	if err != nil {
		t.Fatal(err)
	}
	want, err := spliceTable(string(page), table)
	if err != nil {
		t.Fatal(err)
	}
	if want != string(page) {
		t.Fatal("Plugin-Manifest.md field table is stale: run go run ./tools/manifestv2migrate -wiki .github/wiki/Plugin-Manifest.md")
	}
}
