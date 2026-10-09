package hasura

// Purpose: read Hasura schema and permission metadata for CLI and legacy MCP.
// Inputs: context, base endpoint and admin secret. Outputs: metadata snapshots.
// Constraints: no data query, no secret in errors, shared timeout HTTP client.
// SPORT: P7-SURF-32

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/httptimeout"
)

// Endpoint preserves the legacy MCP endpoint precedence.
func Endpoint() string {
	if u := os.Getenv("NSELF_HASURA_GRAPHQL_URL"); u != "" {
		return u
	}
	if u := os.Getenv("HASURA_GRAPHQL_URL"); u != "" {
		return u
	}
	return "http://127.0.0.1:8080"
}

// SchemaType describes a GraphQL type without reading any rows.
type SchemaType struct {
	Name   string        `json:"name"`
	Kind   string        `json:"kind"`
	Fields []SchemaField `json:"fields,omitempty"`
}

type SchemaField struct {
	Name string          `json:"name"`
	Type SchemaNamedType `json:"type"`
}

type SchemaNamedType struct {
	Name   *string         `json:"name"`
	Kind   string          `json:"kind"`
	OfType *SchemaLeafType `json:"ofType,omitempty"`
}

type SchemaLeafType struct {
	Name *string `json:"name"`
	Kind string  `json:"kind"`
}

// SchemaSnapshot is the command's typed data. Raw retains the exact legacy response.
type SchemaSnapshot struct {
	Types            []SchemaType `json:"types"`
	QueryType        *SchemaRoot  `json:"query_type,omitempty"`
	MutationType     *SchemaRoot  `json:"mutation_type,omitempty"`
	SubscriptionType *SchemaRoot  `json:"subscription_type,omitempty"`
	Raw              []byte       `json:"-"`
	Valid            bool         `json:"-"`
}

type SchemaRoot struct {
	Name string `json:"name"`
}

// TablePermissions preserves the legacy MCP structured result shape.
type TablePermissions struct {
	Table       string                 `json:"table"`
	Permissions map[string]interface{} `json:"permissions"`
}

// PermissionSnapshot is a metadata-only table permission list.
type PermissionSnapshot struct {
	Tables []TablePermissions `json:"tables"`
	Raw    []byte             `json:"-"`
	Valid  bool               `json:"-"`
}

// Schema issues only the standard GraphQL introspection query.
func Schema(ctx context.Context, endpoint, secret string) (SchemaSnapshot, error) {
	raw, err := postJSON(ctx, strings.TrimRight(endpoint, "/")+"/v1/graphql", secret, map[string]string{"query": introspectionQuery})
	if err != nil {
		return SchemaSnapshot{}, err
	}
	var s struct {
		Data struct {
			Schema struct {
				Types            []SchemaType `json:"types"`
				QueryType        *SchemaRoot  `json:"queryType"`
				MutationType     *SchemaRoot  `json:"mutationType"`
				SubscriptionType *SchemaRoot  `json:"subscriptionType"`
			} `json:"__schema"`
		} `json:"data"`
	}
	// Legacy callers passed through malformed JSON. Keep Raw for that path.
	valid := json.Unmarshal(raw, &s) == nil && s.Data.Schema.Types != nil
	return SchemaSnapshot{Types: s.Data.Schema.Types, QueryType: s.Data.Schema.QueryType,
		MutationType: s.Data.Schema.MutationType, SubscriptionType: s.Data.Schema.SubscriptionType, Raw: raw, Valid: valid}, nil
}

// Permissions exports Hasura metadata and selects table permission objects only.
func Permissions(ctx context.Context, endpoint, secret string) (PermissionSnapshot, error) {
	raw, err := postJSON(ctx, strings.TrimRight(endpoint, "/")+"/v1/metadata", secret,
		map[string]interface{}{"type": "export_metadata", "args": map[string]interface{}{}})
	if err != nil {
		return PermissionSnapshot{}, err
	}
	var meta map[string]interface{}
	if json.Unmarshal(raw, &meta) != nil {
		return PermissionSnapshot{Raw: raw}, nil
	}
	sources, ok := meta["sources"].([]interface{})
	if _, failed := meta["error"]; failed || !ok {
		return PermissionSnapshot{Raw: raw}, nil
	}
	var result []TablePermissions
	for _, source := range sources {
		src, ok := source.(map[string]interface{})
		if !ok {
			continue
		}
		tables, _ := src["tables"].([]interface{})
		for _, table := range tables {
			entry, ok := table.(map[string]interface{})
			if !ok {
				continue
			}
			info, _ := entry["table"].(map[string]interface{})
			name, _ := info["name"].(string)
			perms := map[string]interface{}{}
			for _, key := range []string{"select_permissions", "insert_permissions", "update_permissions", "delete_permissions"} {
				if value, found := entry[key]; found {
					perms[key] = value
				}
			}
			result = append(result, TablePermissions{Table: name, Permissions: perms})
		}
	}
	return PermissionSnapshot{Tables: result, Raw: raw, Valid: true}, nil
}

func postJSON(ctx context.Context, url, secret string, payload interface{}) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, legacyError("marshal payload", "marshal Hasura request", err, secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, legacyError("create request", "create Hasura request", err, secret)
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Hasura-Admin-Secret", secret)
	}
	resp, err := httptimeout.Default.Do(req)
	if err != nil {
		return nil, legacyError("request failed", "Hasura request failed", err, secret)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, legacyError("read response", "read Hasura response", err, secret)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &legacyHasuraError{
			legacy:  fmt.Sprintf("HTTP %d: %s", resp.StatusCode, redactSecret(string(raw), secret)),
			current: errs.Newf("E200", "Hasura HTTP %d", resp.StatusCode),
		}
	}
	return raw, nil
}

type legacyHasuraError struct {
	legacy  string
	current error
}

func (e *legacyHasuraError) Error() string { return e.current.Error() }
func (e *legacyHasuraError) Unwrap() error { return e.current }

func legacyError(oldPrefix, currentPrefix string, cause error, secret string) error {
	return &legacyHasuraError{
		legacy:  fmt.Sprintf("%s: %s", oldPrefix, redactSecret(cause.Error(), secret)),
		current: errs.Wrap("E200", currentPrefix, errors.New(redactSecret(cause.Error(), secret))),
	}
}

func redactSecret(body, secret string) string {
	if secret == "" {
		return body
	}
	return strings.ReplaceAll(body, secret, "[REDACTED]")
}

// LegacyErrorText preserves the pre-command MCP error text with secret redaction.
func LegacyErrorText(err error) string {
	var legacy *legacyHasuraError
	if errors.As(err, &legacy) {
		return legacy.legacy
	}
	return err.Error()
}

// CompactSchema returns the legacy text representation byte for byte.
func CompactSchema(raw []byte) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}

const introspectionQuery = `{
  __schema {
    types {
      name
      kind
      fields {
        name
        type { name kind ofType { name kind } }
      }
    }
    queryType { name }
    mutationType { name }
    subscriptionType { name }
  }
}`
