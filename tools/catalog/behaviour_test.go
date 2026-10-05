package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/nself-org/cli/internal/plugin/manifestv2"
)

func TestDeterministicAndCheck(t *testing.T) {
	d1, d2 := t.TempDir(), t.TempDir()
	genBoth(t, d1)
	genBoth(t, d2)
	for _, f := range []string{"free", "licensed"} {
		if !bytes.Equal(readFile(t, filepath.Join(d1, f, "registry.json")), readFile(t, filepath.Join(d2, f, "registry.json"))) {
			t.Fatalf("%s registry differs between two runs on identical inputs", f)
		}
	}
	args := tierArgs("free", filepath.Join(td, "free"), filepath.Join(d1, "free"), filepath.Join(d1, "p1l", "registry.json"), "-check")
	if code, se := gen(t, args...); code != 0 {
		t.Fatalf("-check on current output: exit %d\n%s", code, se)
	}
	reg := filepath.Join(d1, "free", "registry.json")
	if err := os.WriteFile(reg, append(readFile(t, reg), ' '), 0o644); err != nil {
		t.Fatal(err)
	}
	code, se := gen(t, args...)
	if code != 1 || !strings.Contains(se, "registry.json differs") {
		t.Fatalf("-check on a modified file: exit %d, want 1\n%s", code, se)
	}
	if string(readFile(t, reg))[len(readFile(t, reg))-1] != ' ' {
		t.Fatal("-check must not write")
	}
}

func TestNoTimestampOutsideGeneratedFrom(t *testing.T) {
	free, _ := genBoth(t, t.TempDir())
	for _, f := range [][]byte{readFile(t, filepath.Join(free, "registry.json")), readFile(t, filepath.Join(td, "golden", "catalog.json"))} {
		// the catalog embeds counts.json, which carries its own generated_from
		if n, k := strings.Count(string(f), "T00:00:00Z"), strings.Count(string(f), `"generated_at"`); n != k || n == 0 {
			t.Errorf("found %d timestamps and %d generated_at keys: a timestamp outside generated_from", n, k)
		}
	}
	if at, err := generatedAt("", ""); err != nil || at != "1970-01-01T00:00:00Z" {
		t.Errorf("default generated_at = %q, %v", at, err)
	}
	if at, _ := generatedAt("", "86400"); at != "1970-01-02T00:00:00Z" {
		t.Errorf("SOURCE_DATE_EPOCH not honoured: %q", at)
	}
	if _, err := generatedAt("yesterday", ""); err == nil {
		t.Error("a non RFC 3339 -generated-at must fail")
	}
}

// TestV1AndV2TwinGiveOneEntry writes the v2 twin of every fixture manifest and
// checks both trees produce the same registry.
func TestV1AndV2TwinGiveOneEntry(t *testing.T) {
	twin := t.TempDir()
	ents, _ := os.ReadDir(filepath.Join(td, "free"))
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		m, err := manifestv2.ParseQuiet(readFile(t, filepath.Join(td, "free", e.Name(), "plugin.json")))
		if err != nil {
			t.Fatal(err)
		}
		b, err := manifestv2.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(b, []byte(`"manifest_version": 2`)) {
			t.Fatalf("%s: twin is not v2", e.Name())
		}
		if err := os.MkdirAll(filepath.Join(twin, e.Name()), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(twin, e.Name(), "plugin.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(twin, "releases.json"), readFile(t, filepath.Join(td, "free", "releases.json")), 0o644); err != nil {
		t.Fatal(err)
	}
	o1, o2 := t.TempDir(), t.TempDir()
	mustGen(t, tierArgs("free", filepath.Join(td, "free"), o1, "")...)
	mustGen(t, tierArgs("free", twin, o2, "")...)
	if !bytes.Equal(readFile(t, filepath.Join(o1, "registry.json")), readFile(t, filepath.Join(o2, "registry.json"))) {
		t.Fatal("a v1 manifest and its v2 twin must give the same registry entry")
	}
}

func TestTierPairListedOncePerTierAndCountedOnce(t *testing.T) {
	c := decodeMap(t, readFile(t, filepath.Join(td, "golden", "catalog.json")))
	rows := 0
	for _, r := range c["plugins"].([]any) {
		if r.(map[string]any)["slug"] == "cron" {
			rows++
		}
	}
	if rows != 2 {
		t.Errorf("cron rows = %d, want one per tier", rows)
	}
	tot := c["counts"].(map[string]any)["totals"].(map[string]any)
	if want := float64(len(c["plugins"].([]any)) - 1); tot["entries"] != want { // one row per tier, minus the one shared slug
		t.Errorf("totals.entries = %v, want rows-1 (cron counts once)", tot["entries"])
	}
}

// TestCatalogValidatesAgainstSchema checks the catalog and releases fixtures
// against the generated schemas.
func TestCatalogValidatesAgainstSchema(t *testing.T) {
	for _, c := range []struct{ schema, doc string }{
		{"catalog.v1.schema.json", filepath.Join(td, "golden", "catalog.json")},
		{"releases.v1.schema.json", filepath.Join(td, "free", "releases.json")},
		{"releases.v1.schema.json", filepath.Join(td, "licensed", "releases.json")},
	} {
		var sch jsonschema.Schema
		if err := sch.UnmarshalJSON(readFile(t, filepath.Join("..", "..", "schemas", c.schema))); err != nil {
			t.Fatal(err)
		}
		res, err := sch.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := res.Validate(decodeMap(t, readFile(t, c.doc))); err != nil {
			t.Errorf("%s does not validate against %s: %v", c.doc, c.schema, err)
		}
	}
}
