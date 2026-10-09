package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.json")
	for _, tc := range []struct {
		body  string
		valid bool
	}{
		{`{"a":1}`, true},
		{`{"a":1} {"b":2}`, false},
		{`{"a":`, false},
	} {
		if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := readOne(path)
		if (err == nil) != tc.valid {
			t.Fatalf("%q: err=%v, valid=%v", tc.body, err, tc.valid)
		}
	}
}

func TestRunRejectsUnknownSchema(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	path := filepath.Join(t.TempDir(), "doc.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-schema", "missing.json", path}); err == nil {
		t.Fatal("unknown schema accepted")
	}
}

func TestRunEnvelopeAndDataSchema(t *testing.T) {
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir("../.."); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	golden := "cmd/commands/testdata/json/status.golden.json"
	if err := run([]string{"-schema", "envelope.v1.schema.json", "-data-from-registry", "status", golden}); err != nil {
		t.Fatalf("valid status fixture rejected: %v", err)
	}
	b, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	doc["data"] = "wrong type"
	corrupt, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-schema", "envelope.v1.schema.json", "-data-from-registry", "status", path}); err == nil {
		t.Fatal("invalid data shape accepted")
	}
}
