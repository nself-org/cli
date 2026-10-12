package main

// Purpose: minimal OpenAPI 3.1 model for the SDK generator.
// Inputs: .github/openapi.json bytes. Outputs: operations and component schemas.
// Constraints: encoding/json only; unknown keywords are ignored, not guessed.

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Schema is the JSON Schema subset the control API uses.
type Schema struct {
	Ref                  string             `json:"$ref"`
	Type                 json.RawMessage    `json:"type"`
	Description          string             `json:"description"`
	Properties           map[string]*Schema `json:"properties"`
	Required             []string           `json:"required"`
	Items                json.RawMessage    `json:"items"`
	PrefixItems          []*Schema          `json:"prefixItems"`
	MinItems             *int               `json:"minItems"`
	MaxItems             *int               `json:"maxItems"`
	Enum                 []json.RawMessage  `json:"enum"`
	Const                json.RawMessage    `json:"const"`
	AdditionalProperties json.RawMessage    `json:"additionalProperties"`
	OneOf                []*Schema          `json:"oneOf"`
	AnyOf                []*Schema          `json:"anyOf"`
	AllOf                []*Schema          `json:"allOf"`
}

// UnmarshalJSON accepts boolean schemas (true/false) as the empty schema.
func (s *Schema) UnmarshalJSON(b []byte) error {
	switch strings.TrimSpace(string(b)) {
	case "true", "false":
		*s = Schema{}
		return nil
	}
	type plain Schema
	return json.Unmarshal(b, (*plain)(s))
}

type content struct {
	Schema *Schema `json:"schema"`
}

type operationDoc struct {
	OperationID string `json:"operationId"`
	Summary     string `json:"summary"`
	RequestBody *struct {
		Content map[string]content `json:"content"`
	} `json:"requestBody"`
}

type document struct {
	Paths      map[string]map[string]operationDoc `json:"paths"`
	Components struct {
		Schemas map[string]*Schema `json:"schemas"`
	} `json:"components"`
}

// Operation is one POST command route.
type Operation struct {
	ID      string // nself_config_get
	Path    string // /v1/commands/config/get
	Summary string
	Request *Schema
}

// Model is the parsed contract.
type Model struct {
	Ops     []Operation
	Schemas map[string]*Schema
}

// ParseOpenAPI reads the document and keeps the command operations sorted by path.
func ParseOpenAPI(b []byte) (*Model, error) {
	var d document
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("sdkgen: parse openapi: %w", err)
	}
	m := &Model{Schemas: d.Components.Schemas}
	paths := make([]string, 0, len(d.Paths))
	for p := range d.Paths {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if !strings.HasPrefix(p, "/v1/commands/") {
			continue
		}
		op, ok := d.Paths[p]["post"]
		if !ok {
			return nil, fmt.Errorf("sdkgen: %s has no POST operation", p)
		}
		if !strings.HasPrefix(op.OperationID, "nself_") {
			return nil, fmt.Errorf("sdkgen: %s operationId %q lacks the nself_ prefix", p, op.OperationID)
		}
		var req *Schema
		if op.RequestBody != nil {
			req = op.RequestBody.Content["application/json"].Schema
		}
		m.Ops = append(m.Ops, Operation{ID: op.OperationID, Path: p, Summary: op.Summary, Request: req})
	}
	if len(m.Ops) == 0 {
		return nil, fmt.Errorf("sdkgen: no /v1/commands operations found")
	}
	return m, nil
}
