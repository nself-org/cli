package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type scratch struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

func scratchSpec(out string) Spec {
	return Spec{
		Out:  out,
		Type: scratch{},
		Overrides: []Override{
			{Pointer: "/properties/kind", Set: map[string]any{"enum": []any{"a", "b"}}},
		},
	}
}

func runIn(ss *specSet, dir string, check bool) (int, string) {
	var out, errb bytes.Buffer
	code := run(ss, dir, check, &out, &errb)
	return code, errb.String()
}

// TestRegisteredSpecIsGeneratedIndexedAndChecked: a test-local registration is
// written into a temp dir, listed in index.json, and -check fails once the
// file is edited.
func TestRegisteredSpecIsGeneratedIndexedAndChecked(t *testing.T) {
	dir := t.TempDir()
	ss := newSpecSet()
	ss.register(scratchSpec("zz/scratch.v1.schema.json"))
	if code, msg := runIn(ss, dir, false); code != 0 {
		t.Fatalf("generate exit %d: %s", code, msg)
	}
	b, err := os.ReadFile(filepath.Join(dir, "zz", "scratch.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sch map[string]any
	if err := json.Unmarshal(b, &sch); err != nil {
		t.Fatal(err)
	}
	if sch["$id"] != "urn:nself:cli:schema:zz:scratch:v1" || sch["$schema"] != draftURI || sch["$comment"] != generatedTag {
		t.Fatalf("bad header: %v", sch)
	}
	kind := sch["properties"].(map[string]any)["kind"].(map[string]any)
	if !reflect.DeepEqual(kind["enum"], []any{"a", "b"}) {
		t.Fatalf("override not applied: %v", kind)
	}
	if !bytes.HasSuffix(b, []byte("}\n")) || bytes.Contains(b, []byte("\t")) {
		t.Fatal("want 2-space indent and one trailing newline")
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.json"))
	if !strings.Contains(string(idx), `"path": "zz/scratch.v1.schema.json"`) || !strings.Contains(string(idx), `"id": "urn:nself:cli:schema:zz:scratch:v1"`) {
		t.Fatalf("index.json misses the registration:\n%s", idx)
	}
	if code, msg := runIn(ss, dir, true); code != 0 {
		t.Fatalf("check on fresh output exit %d: %s", code, msg)
	}
	if err := os.WriteFile(filepath.Join(dir, "zz", "scratch.v1.schema.json"), append(b, ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _ := runIn(ss, dir, true); code != 1 {
		t.Fatalf("check after edit: exit %d, want 1", code)
	}
}

// TestCheckMissingExtraAndNonJSON: a missing file and a stray *.json fail
// -check; a non-JSON file (schemas/embed.go) is ignored.
func TestCheckMissingExtraAndNonJSON(t *testing.T) {
	dir := t.TempDir()
	ss := newSpecSet()
	ss.register(scratchSpec("a.v1.schema.json"))
	if code, msg := runIn(ss, dir, false); code != 0 {
		t.Fatalf("generate: %d %s", code, msg)
	}
	if err := os.WriteFile(filepath.Join(dir, "embed.go"), []byte("package schemas\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, msg := runIn(ss, dir, true); code != 0 {
		t.Fatalf("non-JSON file must be ignored: exit %d %s", code, msg)
	}
	if err := os.WriteFile(filepath.Join(dir, "stray.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, msg := runIn(ss, dir, true); code != 1 || !strings.Contains(msg, "extra stray.json") {
		t.Fatalf("stray *.json: exit %d %s", code, msg)
	}
	if err := os.Remove(filepath.Join(dir, "stray.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "a.v1.schema.json")); err != nil {
		t.Fatal(err)
	}
	if code, msg := runIn(ss, dir, true); code != 1 || !strings.Contains(msg, "missing a.v1.schema.json") {
		t.Fatalf("missing file: exit %d %s", code, msg)
	}
}

// TestGenerateRemovesStaleJSON: regenerating deletes an unregistered *.json.
func TestGenerateRemovesStaleJSON(t *testing.T) {
	dir := t.TempDir()
	ss := newSpecSet()
	ss.register(scratchSpec("a.v1.schema.json"))
	stale := filepath.Join(dir, "old.v1.schema.json")
	if err := os.WriteFile(stale, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, msg := runIn(ss, dir, false); code != 0 {
		t.Fatalf("generate: %d %s", code, msg)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale file kept: %v", err)
	}
}

// TestDuplicateAndBadOutAreProblems: recorded, never a panic, and -check
// exits 1.
func TestDuplicateAndBadOutAreProblems(t *testing.T) {
	dir := t.TempDir()
	ss := newSpecSet()
	ss.register(scratchSpec("dup.v1.schema.json"))
	ss.register(scratchSpec("dup.v1.schema.json"))
	ss.register(scratchSpec("Bad Name.schema.json"))
	ss.register(Spec{Out: "nil.v1.schema.json"})
	probs := ss.problemList()
	if len(probs) != 3 {
		t.Fatalf("problems = %v, want 3", probs)
	}
	if !strings.Contains(probs[0], "duplicate Out") {
		t.Fatalf("first problem = %q", probs[0])
	}
	if code, _ := runIn(ss, dir, true); code != 1 {
		t.Fatalf("check with problems: exit %d, want 1", code)
	}
	if code, _ := runIn(ss, dir, false); code != 1 {
		t.Fatalf("generate with problems: exit %d, want 1", code)
	}
}

// TestBadOverridePointerFails: an override that names no node is a generation
// failure (exit 2), not silently ignored.
func TestBadOverridePointerFails(t *testing.T) {
	ss := newSpecSet()
	s := scratchSpec("x.v1.schema.json")
	s.Overrides = []Override{{Pointer: "/properties/nope", Set: map[string]any{"enum": []any{1}}}}
	ss.register(s)
	if code, msg := runIn(ss, t.TempDir(), false); code != 2 || !strings.Contains(msg, "no object at") {
		t.Fatalf("exit %d %s", code, msg)
	}
}

// TestTypeOverridesApplyToEverySchemaOfThatType.
func TestTypeOverridesApplyToEverySchemaOfThatType(t *testing.T) {
	type shared struct {
		Name string `json:"name"`
	}
	RegisterTypeOverrides(shared{}, Override{Pointer: "/properties/name", Set: map[string]any{"minLength": 1}})
	t.Cleanup(func() { delete(typeOverrides, reflect.TypeOf(shared{})) })
	for _, out := range []string{"p.v1.schema.json", "q.v1.schema.json"} {
		b, err := build(Spec{Out: out, Type: shared{}})
		if err != nil || !strings.Contains(string(b), `"minLength": 1`) {
			t.Fatalf("%s: err=%v\n%s", out, err, b)
		}
	}
}

// TestCommittedSchemasAreCurrent: the schemas/ directory matches the default
// registrations (the same check `-check` and `make schemas-check` run).
func TestCommittedSchemasAreCurrent(t *testing.T) {
	if code, msg := runIn(defaultSet, filepath.Join("..", "..", "schemas"), true); code != 0 {
		t.Fatalf("schemas are stale (run `make schemas`): exit %d\n%s", code, msg)
	}
}
