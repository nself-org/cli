package httpapi

// Purpose: exercise OpenAPI generation independently of HTTP execution.
// Inputs: synthetic registry and synthetic data schemas.
// Outputs: deterministic schemas, exposure and error assertions.
// Constraints: no installed plugins, credentials or child processes.

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/nself-org/cli/internal/cmdregistry"
)

func TestOpenAPIBuilder(t *testing.T) {
	schema := "schemas/commands/config-get.v1.schema.json"
	stream := "stream"
	reg := &cmdregistry.Registry{Commands: []cmdregistry.Command{
		{Path: "nself config get", Runnable: true, Canon: "subcommand", SideEffect: "read",
			Output: "document", JSON: "envelope", DataSchema: &schema,
			Flags: []cmdregistry.Flag{{Name: "follow", Type: "bool", Output: &stream}}},
		{Path: "nself config hidden", Runnable: true, Canon: "subcommand", SideEffect: "read",
			Output: "document", JSON: "envelope", Hidden: true},
	}}
	lookup := func(path string) ([]byte, bool) {
		if path != "commands/config-get.v1.schema.json" {
			t.Errorf("unexpected schema path: %s", path)
		}
		return []byte(`{"type":"object","properties":{"fake":{"type":"string"}}}`), true
	}
	first, err := OpenAPI(reg, lookup)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenAPI(reg, lookup)
	if err != nil || !bytes.Equal(first, second) || !bytes.HasSuffix(first, []byte("\n")) {
		t.Fatalf("non-deterministic OpenAPI: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(first, &doc); err != nil {
		t.Fatal(err)
	}
	paths := doc["paths"].(map[string]any)
	if len(paths) != 2 {
		t.Fatalf("unexpected operations: %v", paths)
	}
	op := paths[cmdregistry.RoutePath("nself config get")].(map[string]any)["post"].(map[string]any)
	if op["operationId"] != cmdregistry.ToolName("nself config get") {
		t.Fatalf("operation ID: %v", op["operationId"])
	}
	params := op["requestBody"].(map[string]any)["content"].(map[string]any)["application/json"].(map[string]any)["schema"].(map[string]any)
	flags := params["properties"].(map[string]any)["flags"].(map[string]any)["properties"].(map[string]any)
	if _, exists := flags["follow"]; exists {
		t.Fatal("stream override leaked into document request schema")
	}
	components := doc["components"].(map[string]any)["schemas"].(map[string]any)
	if _, exists := components[cmdregistry.ToolName("nself config get")+"_data"]; !exists {
		t.Fatal("data schema component missing")
	}
	if _, err := OpenAPI(nil, nil); err == nil {
		t.Fatal("nil registry accepted")
	}
	if _, err := OpenAPI(reg, func(string) ([]byte, bool) { return []byte("bad"), true }); err == nil {
		t.Fatal("invalid data schema accepted")
	}
}
