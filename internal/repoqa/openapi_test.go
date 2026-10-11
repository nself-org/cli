package repoqa

// Purpose: prevent OpenAPI drift and validate the vendored OAS 3.1 contract.
// Inputs: committed OpenAPI, registry, and pinned official meta-schema.
// Outputs: drift, structural validity and exposure assertions.
// Constraints: vendor-only; no network or developer plugin discovery.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestOpenAPIIsCurrent(t *testing.T) {
	gen := exec.Command("go", "run", "-mod=vendor", "./tools/openapigen", "-check")
	gen.Dir = repoRoot(t)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("OpenAPI drift: %v\n%s", err, out)
	}
}

func TestOpenAPIValidatesOAS31(t *testing.T) {
	root := repoRoot(t)
	readJSON := func(path string) any {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		v, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	meta := readJSON("tools/openapigen/testdata/oas-3.1-schema.json")
	doc := readJSON(".github/openapi.json")
	const location = "https://spec.openapis.org/oas/3.1/schema/2022-10-07"
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(location, meta); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiled.Validate(doc); err != nil {
		t.Fatalf("OpenAPI is not OAS 3.1: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(root, ".github/command-registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var reg cmdregistry.Registry
	if err := json.Unmarshal(b, &reg); err != nil {
		t.Fatal(err)
	}
	paths := doc.(map[string]any)["paths"].(map[string]any)
	for i := range reg.Commands {
		cmd := &reg.Commands[i]
		exposed, reason := invoke.Exposure(cmd, invoke.TransportHTTP, nil)
		operation, present := paths[cmdregistry.RoutePath(cmd.Path)]
		if !exposed && present {
			t.Errorf("unexposed %s has an operation (%s): %v", cmd.Path, reason, operation)
		}
		if exposed && !present {
			t.Errorf("exposed document command %s is missing", cmd.Path)
		}
	}
	for _, path := range []string{"nself config export", "nself config set", "nself admin serve"} {
		if _, present := paths[cmdregistry.RoutePath(path)]; present {
			t.Errorf("%s must not have an operation", path)
		}
	}
}
