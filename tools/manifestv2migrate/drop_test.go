package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func stableDoc(t *testing.T, set map[string]any) (string, []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(mustRead(t, filepath.Join(fixtureRoot, "v1", "status-stable.json")), &doc); err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		doc[k] = v
	}
	b, _ := json.Marshal(doc)
	p := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, b
}

func dropFile(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "drop.txt")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// -drop discards only listed keys: default and -check say what would be
// dropped, -write drops and says so; anything else keeps refusing.
func TestMigrateDropListedKeys(t *testing.T) {
	list := dropFile(t, "# evidence\nhooks  # no reader\nnotes # none either\n")
	in, orig := stableDoc(t, map[string]any{"hooks": map[string]any{"a": 1}, "notes": "x"})

	if code, _, e := runTool(t, "-in", in); code != 1 || !strings.Contains(e, "$.hooks") {
		t.Errorf("without -drop the keys still refuse: %d %s", code, e)
	}
	code, out, stderr := runTool(t, "-drop", list, "-in", in)
	if code != 0 || !strings.Contains(stderr, "would drop 2 dead v1 keys") || !strings.Contains(stderr, "$.hooks (object, 1 entry)") || !strings.Contains(stderr, "$.notes (string)") {
		t.Errorf("dry run: %d %s", code, stderr)
	}
	if strings.Contains(out, "hooks") || !strings.Contains(out, `"manifest_version": 2`) {
		t.Errorf("dry run stdout is not the v2 file:\n%s", out)
	}
	if string(mustRead(t, in)) != string(orig) {
		t.Error("dry run changed the file")
	}
	if code, _, e := runTool(t, "-drop", list, "-in", in, "-check"); code != 1 || !strings.Contains(e, "would drop 2") {
		t.Errorf("-check on v1 must exit 1 and report: %d %s", code, e)
	}
	code, _, stderr = runTool(t, "-drop", list, "-in", in, "-write")
	if code != 0 || !strings.Contains(stderr, "dropping 2 dead v1 keys") {
		t.Fatalf("-write: %d %s", code, stderr)
	}
	written := string(mustRead(t, in))
	if written != out || strings.Contains(written, "hooks") {
		t.Errorf("-write output differs from the dry run:\n%s", written)
	}
	if code, _, e := runTool(t, "-in", in, "-check"); code != 0 {
		t.Errorf("written file must be canonical v2: %s", e)
	}
}

func TestMigrateDropStillRefusesOthers(t *testing.T) {
	list := dropFile(t, "hooks # dead\n")
	in, orig := stableDoc(t, map[string]any{"hooks": []any{"x"}, "config": map[string]any{"a": 1}})
	for _, mode := range [][]string{{"-write"}, {}, {"-check"}} {
		args := append([]string{"-drop", list, "-in", in}, mode...)
		code, out, stderr := runTool(t, args...)
		if code != 1 || out != "" || !strings.Contains(stderr, "refusing: 1 v1 key") || !strings.Contains(stderr, "$.config") {
			t.Errorf("%v: %d %q %s", mode, code, out, stderr)
		}
		if strings.Contains(stderr[strings.Index(stderr, "refusing"):], "$.hooks") {
			t.Errorf("%v: a listed key must not be reported as refused:\n%s", mode, stderr)
		}
		if string(mustRead(t, in)) != string(orig) {
			t.Errorf("%v: the file changed", mode)
		}
	}
}

func TestMigrateDropListErrors(t *testing.T) {
	in, orig := stableDoc(t, map[string]any{"hooks": []any{"x"}})
	for name, body := range map[string]string{
		"mapped key":   "routes # mapped to rest_routes\n",
		"name":         "name\n",
		"capabilities": "capabilities # now a v2 key\n",
		"two words":    "hooks notes\n",
	} {
		code, _, stderr := runTool(t, "-drop", dropFile(t, body), "-in", in, "-write")
		if code != 1 || stderr == "" || string(mustRead(t, in)) != string(orig) {
			t.Errorf("%s: %d %q", name, code, stderr)
		}
	}
	if code, _, _ := runTool(t, "-drop", filepath.Join(t.TempDir(), "missing.txt"), "-in", in, "-write"); code != 1 {
		t.Errorf("a missing drop file must exit 1, got %d", code)
	}
}

// The shipped list: every key carries an evidence comment, none is mapped, and
// none of the dev-tool, build-time or runtime keys is on it.
func TestDeadKeysFile(t *testing.T) {
	raw := string(mustRead(t, "dead-keys.txt"))
	if _, err := loadDropList("dead-keys.txt"); err != nil {
		t.Fatal(err)
	}
	keep := map[string]bool{}
	for _, k := range []string{"actions", "config", "binary_name", "download_url", "tarball", "security_always_free", "min_nself_version",
		"defaultPort", "env", "env_vars", "env_required", "env_optional", "capabilities", "routes"} {
		keep[k] = true
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, evidence, ok := strings.Cut(line, "#")
		key = strings.TrimSpace(key)
		if !ok || len(strings.TrimSpace(evidence)) < 10 {
			t.Errorf("%q has no evidence comment", line)
		}
		if keep[key] || seen[key] {
			t.Errorf("%q must not be on the dead list (reader exists, or duplicate)", key)
		}
		seen[key] = true
	}
	if len(seen) < 50 {
		t.Errorf("only %d dead keys listed", len(seen))
	}
}
