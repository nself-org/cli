package mcpgen

// Purpose: input and output schemas of generated tools.
// Inputs: a command, a legacy row, the committed schema lookup.
// Outputs: JSON Schema documents (draft 2020-12) as maps or raw bytes.
// Constraints: the input is invoke.ParamsSchemaIn plus the row's legacy
// properties; the output is the committed data schema, wrapped when the data
// is not an object (structuredContent must be an object) and joined with the
// confirmation schema when the command can answer with a confirmation.

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
)

const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// confirmationSchema is the committed schema of a confirmation answer.
const confirmationSchema = "invoke/confirmation.v1.schema.json"

// addLegacyProps adds the row's legacy properties to a request schema. Names
// must be disjoint from the machine members and the generated properties.
func addLegacyProps(schema map[string]any, row *Row, withConfirm bool) error {
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
		schema["properties"] = props
	}
	for _, p := range row.Params {
		if _, clash := props[p.Name]; clash {
			return fmt.Errorf("mcpgen: legacy property %q of %s collides with a machine request member", p.Name, row.Name)
		}
		desc := p.Description
		if !row.Shortcut {
			desc = DeprecatedPrefix + desc
		}
		props[p.Name] = map[string]any{"type": p.Type, "description": desc}
	}
	if row.Kind != KindMerged && withConfirm {
		if _, has := props["confirm"]; !has {
			props["confirm"] = map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$",
				"description": "a nonce the server issued for this identical call"}
		}
	}
	return nil
}

// outputSchema is the output schema of cmd as raw JSON.
func (g *gen) outputSchema(cmd *cmdregistry.Command) json.RawMessage {
	data := g.dataSchema(cmd)
	if cmd != nil && cmd.Confirm != nil && g.Schema != nil {
		if b, ok := g.Schema(confirmationSchema); ok {
			var conf any
			if json.Unmarshal(b, &conf) == nil {
				data = map[string]any{"type": "object", "anyOf": []any{data, conf}}
			}
		}
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return json.RawMessage(`{"type":"object"}`)
	}
	return raw
}

// dataSchema is the committed data schema of cmd as an object schema.
func (g *gen) dataSchema(cmd *cmdregistry.Command) map[string]any {
	generic := map[string]any{"type": "object"}
	if cmd == nil || cmd.DataSchema == nil || g.Schema == nil {
		return generic
	}
	b, ok := g.Schema(strings.TrimPrefix(*cmd.DataSchema, "schemas/"))
	if !ok {
		return generic
	}
	var s map[string]any
	if json.Unmarshal(b, &s) != nil {
		return generic
	}
	if t, _ := s["type"].(string); t == "object" {
		return s
	}
	return map[string]any{"type": "object", "properties": map[string]any{"value": s}, "required": []string{"value"}}
}
