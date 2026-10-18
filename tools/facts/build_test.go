package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

const testRegistry = `{"verbs":["init","start"],"counts":{"commands":5,"top_level":2,"core":2,"core_missing":[]},"x":1}`

func TestBuildProjection(t *testing.T) {
	f, err := Build([]byte("1.4.14\n"), []byte(testRegistry), nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.Version != "1.4.14" || f.Counts.TopLevel != 2 || f.Counts.Commands != 5 || f.Counts.Core != 2 {
		t.Fatalf("projection wrong: %+v", f)
	}
	if f.Sources.OpenAPISHA256 != nil || len(f.Sources.RegistrySHA256) != 64 {
		t.Fatalf("sources wrong: %+v", f.Sources)
	}
	if got := strings.Join(f.Verbs, ","); got != "init,start" {
		t.Fatalf("verb order lost: %s", got)
	}
}

func TestBuildOpenAPIHashAndChange(t *testing.T) {
	a, _ := Build([]byte("1.5.0"), []byte(testRegistry), []byte(`{"a":1}`))
	b, _ := Build([]byte("1.5.0"), []byte(testRegistry), []byte(`{"a":2}`))
	if a.Sources.OpenAPISHA256 == nil || *a.Sources.OpenAPISHA256 == *b.Sources.OpenAPISHA256 {
		t.Fatal("openapi hash must be set and follow the file bytes")
	}
}

func TestBuildRejects(t *testing.T) {
	cases := map[string][2]string{
		"bad version":  {"v1.5", testRegistry},
		"empty":        {"", testRegistry},
		"bad registry": {"1.5.0", `{`},
		"no verbs":     {"1.5.0", `{"counts":{}}`},
		"no counts":    {"1.5.0", `{"verbs":["init"]}`},
		"count drift":  {"1.5.0", `{"verbs":["init"],"counts":{"top_level":2}}`},
	}
	for name, c := range cases {
		if _, err := Build([]byte(c[0]), []byte(c[1]), nil); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestMarshalShape(t *testing.T) {
	f, _ := Build([]byte("1.5.0"), []byte(`{"verbs":["a"],"counts":{"top_level":1}}`), nil)
	b, err := Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(b, []byte("}\n")) || !strings.HasPrefix(string(b), "{\n  \"_generated\"") {
		t.Fatalf("not canonical:\n%s", b)
	}
	// core_missing is [] never null, openapi_sha256 is an explicit null.
	if !strings.Contains(string(b), `"core_missing": []`) || !strings.Contains(string(b), `"openapi_sha256": null`) {
		t.Fatalf("nullability wrong:\n%s", b)
	}
	// Top-level keys are sorted.
	dec := json.NewDecoder(bytes.NewReader(b))
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for dec.More() {
		k, _ := dec.Token()
		keys = append(keys, k.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] >= keys[i] {
			t.Fatalf("keys not sorted: %v", keys)
		}
	}
}

func TestBelowV15(t *testing.T) {
	for v, want := range map[string]bool{"1.4.14": true, "1.4.99": true, "1.5.0": false, "1.5.0-rc.1": true, "1.5.1-rc.1": false, "1.5.0+build": false, "2.0.0": false, "0.9.0": true, "x": true} {
		if got := belowV15(v); got != want {
			t.Errorf("belowV15(%q) = %v, want %v", v, got, want)
		}
	}
}
