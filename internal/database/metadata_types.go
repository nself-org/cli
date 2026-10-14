package database

// Purpose: shared Hasura metadata types + repo-side loading for the
// drift/reconcile/verify trio (metadata_drift.go, metadata_reconcile.go).
// Inputs: the project directory containing hasura/metadata/**.
// Outputs: parsed table metadata keyed by "schema.name".
// Constraints: split out of metadata_drift.go to respect the 300-line file
// cap (ASI Policy 3) — pure move, no behavior change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// hasuraAuthOwnedTables lists "schema.name" keys hasura-auth creates that are
// legitimately absent from project metadata. The ntask inbox report
// (2026-08-22) named eight of these from memory; verified exhaustively here
// 2026-08-31 against a live `nself start` Postgres instance's
// information_schema.tables for the auth schema — that live check caught two
// the inbox report's list actually had wrong: auth.providers (bare, distinct
// from auth.user_providers, a separate join table) and auth.migrations
// (hasura-auth's own internal ledger) were both missing from the
// hand-remembered list and would have produced false-positive drift.
var hasuraAuthOwnedTables = map[string]bool{
	"auth.users":               true,
	"auth.roles":               true,
	"auth.refresh_tokens":      true,
	"auth.refresh_token_types": true,
	"auth.providers":           true,
	"auth.provider_requests":   true,
	"auth.user_providers":      true,
	"auth.user_roles":          true,
	"auth.user_security_keys":  true,
	"auth.migrations":          true,
}

// permissionKinds maps a Hasura metadata list to its short name, used in both
// drift finding output and reconcile op-type construction
// (pg_create_<short>_permission / pg_drop_<short>_permission).
var permissionKinds = []struct {
	short string
}{
	{"insert"},
	{"select"},
	{"update"},
	{"delete"},
}

type hasuraTableRef struct {
	Schema string `yaml:"schema" json:"schema"`
	Name   string `yaml:"name" json:"name"`
}

func (t hasuraTableRef) key() string { return t.Schema + "." + t.Name }

type hasuraRelationship struct {
	Name  string                 `yaml:"name" json:"name"`
	Using map[string]interface{} `yaml:"using" json:"using"`
}

type hasuraPermission struct {
	Role       string                 `yaml:"role" json:"role"`
	Permission map[string]interface{} `yaml:"permission" json:"permission"`
}

type hasuraTable struct {
	Table               hasuraTableRef        `yaml:"table" json:"table"`
	ObjectRelationships []hasuraRelationship  `yaml:"object_relationships,omitempty" json:"object_relationships,omitempty"`
	ArrayRelationships  []hasuraRelationship  `yaml:"array_relationships,omitempty" json:"array_relationships,omitempty"`
	ComputedFields      []hasuraComputedField `yaml:"computed_fields,omitempty" json:"computed_fields,omitempty"`
	InsertPermissions   []hasuraPermission    `yaml:"insert_permissions,omitempty" json:"insert_permissions,omitempty"`
	SelectPermissions   []hasuraPermission    `yaml:"select_permissions,omitempty" json:"select_permissions,omitempty"`
	UpdatePermissions   []hasuraPermission    `yaml:"update_permissions,omitempty" json:"update_permissions,omitempty"`
	DeletePermissions   []hasuraPermission    `yaml:"delete_permissions,omitempty" json:"delete_permissions,omitempty"`
}

// tablePermissions returns the permission list for the given short kind
// ("insert"/"select"/"update"/"delete").
func (t hasuraTable) tablePermissions(short string) []hasuraPermission {
	switch short {
	case "insert":
		return t.InsertPermissions
	case "select":
		return t.SelectPermissions
	case "update":
		return t.UpdatePermissions
	case "delete":
		return t.DeletePermissions
	}
	return nil
}

type hasuraSource struct {
	Name   string        `yaml:"name" json:"name"`
	Tables []hasuraTable `yaml:"tables" json:"tables"`
}

type hasuraMetadataDoc struct {
	Sources []hasuraSource `yaml:"sources" json:"sources"`
}

func tablesByKey(tables []hasuraTable) map[string]hasuraTable {
	out := make(map[string]hasuraTable, len(tables))
	for _, t := range tables {
		out[t.Table.key()] = t
	}
	return out
}

// loadRepoTables loads the committed Hasura table metadata from projectDir,
// supporting both formats HasuraApplyMetadata supports (hasura_apply.go):
// the Hasura CLI YAML directory format (with !include directives, resolved
// via resolveIncludes) and the legacy nSelf JSON format.
func loadRepoTables(projectDir string) (map[string]hasuraTable, error) {
	hasuraDir := filepath.Join(projectDir, "hasura")

	yamlCandidates := []string{
		filepath.Join(hasuraDir, "metadata", "databases", "default", "tables", "tables.yaml"),
		filepath.Join(hasuraDir, "metadata", "tables.yaml"),
	}
	for _, tablesFile := range yamlCandidates {
		if _, err := os.Stat(tablesFile); err == nil {
			resolved, err := resolveIncludes(filepath.Dir(tablesFile), tablesFile)
			if err != nil {
				return nil, fmt.Errorf("resolve %s includes: %w", tablesFile, err)
			}
			var tables []hasuraTable
			if err := yaml.Unmarshal(resolved, &tables); err != nil {
				return nil, fmt.Errorf("parse %s: %w", tablesFile, err)
			}
			return tablesByKey(tables), nil
		}
	}

	metadataFile := filepath.Join(hasuraDir, "metadata.json")
	data, err := os.ReadFile(metadataFile)
	if err != nil {
		return nil, fmt.Errorf("no hasura metadata found (tried %s and %s): %w",
			yamlCandidates[0], metadataFile, err)
	}
	var doc hasuraMetadataDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", metadataFile, err)
	}
	var all []hasuraTable
	for _, src := range doc.Sources {
		all = append(all, src.Tables...)
	}
	return tablesByKey(all), nil
}
