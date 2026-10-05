package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lossyKeys are v1 keys no v2 field holds. One fixture carries all of them.
var lossyKeys = []string{"config", "hooks", "actions", "routes", "migrations", "env", "notes", "api_version", "entry"}

func lossyFile(t *testing.T, extra map[string]any) (string, []byte) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(mustRead(t, filepath.Join(fixtureRoot, "v1", "status-stable.json")), &doc); err != nil {
		t.Fatal(err)
	}
	for _, k := range lossyKeys {
		doc[k] = map[string]any{"value": k}
	}
	for k, v := range extra {
		doc[k] = v
	}
	b, _ := json.Marshal(doc)
	p := filepath.Join(t.TempDir(), "plugin.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, b
}

// -write never drops data silently: it refuses, names every key and its value
// path, and leaves the file as it was. Default and -check report the same list.
func TestMigrateRefusesToDropV1Data(t *testing.T) {
	in, orig := lossyFile(t, nil)
	var lists []string
	for _, mode := range [][]string{{"-write"}, {}, {"-check"}} {
		args := append([]string{"-in", in}, mode...)
		code, out, stderr := runTool(t, args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1", mode, code)
		}
		if out != "" {
			t.Errorf("%v: nothing may reach stdout on refusal, got %q", mode, out)
		}
		for _, k := range lossyKeys {
			if !strings.Contains(stderr, "$."+k+" (object, 1 entry)") {
				t.Errorf("%v: stderr does not name $.%s:\n%s", mode, k, stderr)
			}
		}
		lists = append(lists, stderr[strings.Index(stderr, "refusing"):])
		if got := string(mustRead(t, in)); got != string(orig) {
			t.Errorf("%v: the file changed on refusal", mode)
		}
		if entries, _ := os.ReadDir(filepath.Dir(in)); len(entries) != 1 {
			t.Errorf("%v: stray files left behind: %v", mode, entries)
		}
	}
	if lists[0] != lists[1] || lists[1] != lists[2] {
		t.Errorf("modes report different lists:\n%s\n---\n%s\n---\n%s", lists[0], lists[1], lists[2])
	}
}

// Each key on its own is enough to refuse, and empty values are not data.
func TestMigrateRefusesEachKey(t *testing.T) {
	for _, k := range lossyKeys {
		var doc map[string]any
		_ = json.Unmarshal(mustRead(t, filepath.Join(fixtureRoot, "v1", "status-stable.json")), &doc)
		doc[k] = []any{"x"}
		b, _ := json.Marshal(doc)
		p := filepath.Join(t.TempDir(), "plugin.json")
		_ = os.WriteFile(p, b, 0o644)
		code, _, stderr := runTool(t, "-in", p, "-write")
		if code != 1 || !strings.Contains(stderr, "$."+k+" (array, 1 entry)") {
			t.Errorf("%s: exit %d, stderr %q", k, code, stderr)
		}
		if string(mustRead(t, p)) != string(b) {
			t.Errorf("%s: file changed", k)
		}
	}
	in, _ := lossyFile(t, nil)
	var doc map[string]any
	_ = json.Unmarshal(mustRead(t, filepath.Join(fixtureRoot, "v1", "status-stable.json")), &doc)
	for _, k := range lossyKeys {
		doc[k] = []any{}
	}
	doc["notes"], doc["entry"], doc["env"] = "", nil, map[string]any{}
	b, _ := json.Marshal(doc)
	_ = os.WriteFile(in, b, 0o644)
	if code, _, stderr := runTool(t, "-in", in, "-write"); code != 0 {
		t.Errorf("empty values must not block the write: %s", stderr)
	}
}

// Registry-owned keys are removed on purpose and said so; they do not refuse.
func TestMigrateNotesRegistryKeys(t *testing.T) {
	in := copyLossless(t, "v1/tenant.json")
	var doc map[string]any
	_ = json.Unmarshal(mustRead(t, in), &doc)
	doc["bundles"], doc["checksum"] = []any{"x"}, "abc"
	b, _ := json.Marshal(doc)
	_ = os.WriteFile(in, b, 0o644)
	code, _, stderr := runTool(t, "-in", in, "-write")
	if code != 0 || !strings.Contains(stderr, "bundles, checksum") {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}

// A v1 file valid for v1.4.12 but missing category normalizes, yet the tool
// does not write an invalid v2 file: it fails naming category.
func TestMigrateNeedsCategoryForV2(t *testing.T) {
	in := filepath.Join(t.TempDir(), "plugin.json")
	_ = os.WriteFile(in, []byte(`{"name":"internal","version":"1.0.0","language":"go","status":"stable","installable":false,"description":"Shared libraries."}`), 0o644)
	before := mustRead(t, in)
	code, _, stderr := runTool(t, "-in", in, "-write")
	if code != 1 || !strings.Contains(stderr, "category") || string(mustRead(t, in)) != string(before) {
		t.Errorf("exit %d, stderr %q", code, stderr)
	}
}
