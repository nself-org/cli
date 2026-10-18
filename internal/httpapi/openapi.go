package httpapi

// Purpose: generate the HTTP contract from the machine registry.
// Inputs: registry and committed data-schema lookup.
// Outputs: deterministic OpenAPI 3.1 bytes.
// Constraints: no environment reads, timestamps, CLI version or package-init work.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/invoke"
	"github.com/nself-org/cli/schemas"
)

// OpenAPI describes document commands; streaming remains a transport contract.
func OpenAPI(reg *cmdregistry.Registry, dataSchema func(string) ([]byte, bool)) ([]byte, error) {
	if reg == nil {
		return nil, fmt.Errorf("OpenAPI: nil registry")
	}
	components := map[string]any{}
	for name, path := range map[string]string{
		"envelope": "envelope.v1.schema.json", "error": "error.v1.schema.json",
		"confirmation": "invoke/confirmation.v1.schema.json",
	} {
		b, ok := schemas.Lookup(path)
		if !ok {
			return nil, fmt.Errorf("OpenAPI: missing schema %s", path)
		}
		var schema map[string]any
		if err := json.Unmarshal(b, &schema); err != nil {
			return nil, fmt.Errorf("OpenAPI: %s: %w", path, err)
		}
		delete(schema, "$id")
		rewriteErrorRef(schema)
		components[name] = schema
	}
	components["error_envelope"] = envelopeWith(components["envelope"], "error", schemaRef("error"))
	components["confirmation_envelope"] = envelopeWith(components["envelope"], "data", schemaRef("confirmation"))
	paths := map[string]any{
		"/openapi.json": map[string]any{"get": map[string]any{
			"operationId": "openapi", "summary": "Read this OpenAPI document",
			"responses": map[string]any{"200": jsonResponse("OpenAPI 3.1 document", map[string]any{"type": "object"}),
				"default": jsonResponse("Error envelope", schemaRef("error_envelope"))},
		}},
	}
	names := map[string]string{}
	for i := range reg.Commands {
		cmd := &reg.Commands[i]
		if exposed, _ := invoke.Exposure(cmd, invoke.TransportHTTP, nil); !exposed {
			continue
		}
		name := cmdregistry.ToolName(cmd.Path)
		if len(name) > 64 {
			return nil, fmt.Errorf("OpenAPI: tool name %q for %q exceeds 64 characters", name, cmd.Path)
		}
		if previous, exists := names[name]; exists && previous != cmd.Path {
			return nil, fmt.Errorf("OpenAPI: tool name %q collides for %q and %q", name, previous, cmd.Path)
		}
		names[name] = cmd.Path
		data := map[string]any{"type": "object", "additionalProperties": true}
		if cmd.DataSchema != nil && dataSchema != nil {
			path := strings.TrimPrefix(*cmd.DataSchema, "schemas/")
			if b, ok := dataSchema(path); ok {
				data = nil
				if err := json.Unmarshal(b, &data); err != nil {
					return nil, fmt.Errorf("OpenAPI: %s: %w", path, err)
				}
				if data == nil {
					return nil, fmt.Errorf("OpenAPI: %s: schema must be an object", path)
				}
			}
		}
		components[name+"_data"] = data
		paths[cmdregistry.RoutePath(cmd.Path)] = map[string]any{"post": map[string]any{
			"operationId": name, "summary": cmd.Summary,
			"requestBody": map[string]any{"required": true, "content": map[string]any{
				"application/json": map[string]any{"schema": invoke.ParamsSchemaIn(reg, cmd)},
			}},
			"responses": map[string]any{
				"200": jsonResponse("Command data envelope (including state exits 10–12)",
					envelopeWith(components["envelope"], "data", schemaRef(name+"_data"))),
				"428":     jsonResponse("Confirmation required", schemaRef("confirmation_envelope")),
				"default": jsonResponse("Error envelope", schemaRef("error_envelope")),
			},
		}}
	}
	doc := map[string]any{
		"openapi": "3.1.0", "info": map[string]any{"title": "nSelf CLI HTTP API", "version": "1"},
		"paths":    paths,
		"security": []any{map[string]any{"bearer": []string{}}},
		"components": map[string]any{"schemas": components, "securitySchemes": map[string]any{
			"bearer": map[string]any{"type": "http", "scheme": "bearer"},
		}},
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func schemaRef(name string) map[string]any {
	return map[string]any{"$ref": "#/components/schemas/" + name}
}

func jsonResponse(description string, schema any) map[string]any {
	return map[string]any{"description": description, "content": map[string]any{
		"application/json": map[string]any{"schema": schema},
	}}
}

// envelopeWith copies the committed envelope and narrows it to one member.
func envelopeWith(base any, member string, value any) map[string]any {
	b, _ := json.Marshal(base)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	props := out["properties"].(map[string]any)
	delete(props, "data")
	delete(props, "error")
	props[member] = value
	delete(out, "oneOf")
	out["required"] = []string{"schema_version", "command", member}
	return out
}

func rewriteErrorRef(value any) {
	switch v := value.(type) {
	case map[string]any:
		if v["$ref"] == "urn:nself:cli:schema:error:v1" {
			v["$ref"] = "#/components/schemas/error"
		}
		for _, child := range v {
			rewriteErrorRef(child)
		}
	case []any:
		for _, child := range v {
			rewriteErrorRef(child)
		}
	}
}
