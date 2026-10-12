package appwrite

// Purpose: map Appwrite databases, collections and attributes to Postgres DDL and Hasura metadata.
// Inputs: attribute lists from the REST API. Outputs: schema/table/column names, DDL text and
// relationship metadata. Constraints: every identifier is sanitised and checked for collisions;
// `$permissions` is reported, never mapped.
import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/errs"
)

var (
	idRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,35}$`)
	badChar = regexp.MustCompile(`[^A-Za-z0-9_]`)
)

// attribute is the subset of an Appwrite attribute the mapping reads.
type attribute struct {
	Key         string `json:"key"`
	Type        string `json:"type"`
	Array       bool   `json:"array"`
	Required    bool   `json:"required"`
	Status      string `json:"status"`
	Related     string `json:"relatedCollection"`
	RelatedType string `json:"relationType"`
	Side        string `json:"side"`
}

// column is one mapped table column.
type column struct {
	Name, Key, SQLType string
	Kind               string // text, int, float, bool, time, json, rel (single id), reljson (id list)
	Related            string // related collection id for a single-valued relationship
}

// note is one thing the import cannot map; it becomes an exemption so the report shows it.
type note struct{ ID, Reason string }

// collection is a mapped collection: its table and ordered columns.
type collection struct {
	ID, DB, Schema, Table string
	Columns               []column
	Permissions           []string
	notes                 []note
}

// ident turns an Appwrite id or key into a Postgres identifier: other characters become `_`.
func ident(s string) (string, error) {
	out := badChar.ReplaceAllString(s, "_")
	if out == "" || out[0] >= '0' && out[0] <= '9' {
		out = "_" + out
	}
	if _, err := database.SanitizeIdentifier(out); err != nil {
		return "", errs.New("E520", "source tables unavailable")
	}
	return out, nil
}

// reservedSchemas are nSelf-owned or Postgres-owned; a database with such an id gets an aw_ prefix.
var reservedSchemas = map[string]bool{"auth": true, "storage": true, "public": true, "hdb_catalog": true, "nself_ops": true, "information_schema": true}

// schemaName maps an Appwrite database id to a schema that cannot shadow a system schema.
func schemaName(id string) (string, error) {
	n, err := ident(id)
	if err != nil {
		return "", err
	}
	if reservedSchemas[n] || strings.HasPrefix(n, "pg_") || strings.HasPrefix(n, "np_") {
		n = "aw_" + n
	}
	return n, nil
}

// reserved column names written for every document.
var fixedColumns = map[string]bool{"id": true, "created_at": true, "updated_at": true}

// mapAttributes maps attributes (in the order given) to columns after the three fixed ones.
func mapAttributes(attrs []attribute) ([]column, []note, error) {
	cols := []column{{Name: "id", Key: "$id", SQLType: "text PRIMARY KEY", Kind: "text"}, {Name: "created_at", Key: "$createdAt", SQLType: "timestamptz", Kind: "time"}, {Name: "updated_at", Key: "$updatedAt", SQLType: "timestamptz", Kind: "time"}}
	used := map[string]bool{"id": true, "created_at": true, "updated_at": true}
	notes := []note{}
	for _, a := range attrs {
		if a.Status != "" && a.Status != "available" {
			notes = append(notes, note{"attribute:" + a.Key, fmt.Sprintf("attribute %s has status %q and was not imported", a.Key, a.Status)})
			continue
		}
		name, err := ident(a.Key)
		if err != nil {
			return nil, nil, err
		}
		if fixedColumns[name] {
			name = "attr_" + name
		}
		if used[name] {
			return nil, nil, errs.New("E520", "source tables unavailable")
		}
		used[name] = true
		c := column{Name: name, Key: a.Key}
		switch {
		case a.Type == "relationship":
			c.Kind, c.SQLType, c.Related = "rel", "text", a.Related
			if isMany(a) {
				c.Kind, c.SQLType, c.Related = "reljson", "jsonb", ""
				notes = append(notes, note{"relationship:" + a.Key, fmt.Sprintf("%s: %s relationship stored as a JSON id list, no Hasura relationship", a.Key, a.RelatedType)})
			}
		case a.Array:
			c.Kind, c.SQLType = "json", "jsonb"
		default:
			c.Kind, c.SQLType = scalar(a.Type)
			if c.Kind == "json" {
				notes = append(notes, note{"type:" + a.Key, fmt.Sprintf("%s: unknown attribute type kept as jsonb", a.Key)})
			}
		}
		cols = append(cols, c)
	}
	return cols, notes, nil
}

func scalar(t string) (kind, sql string) {
	switch t {
	case "string", "email", "url", "ip", "enum":
		return "text", "text"
	case "integer":
		return "int", "bigint"
	case "double", "float":
		return "float", "double precision"
	case "boolean":
		return "bool", "boolean"
	case "datetime":
		return "time", "timestamptz"
	}
	return "json", "jsonb"
}

// isMany reports whether a relationship attribute holds a list of ids on this side.
func isMany(a attribute) bool {
	switch a.RelatedType {
	case "manyToMany":
		return true
	case "oneToMany":
		return a.Side != "child"
	case "manyToOne":
		return a.Side == "child"
	}
	return false
}

// ddl renders CREATE SCHEMA / CREATE TABLE text for the collections.
func ddl(cols []*collection) (string, error) {
	var b strings.Builder
	schemas := map[string]bool{}
	for _, c := range cols {
		s, err := database.SanitizeIdentifier(c.Schema)
		if err != nil {
			return "", errs.New("E520", "source tables unavailable")
		}
		if !schemas[c.Schema] {
			schemas[c.Schema] = true
			fmt.Fprintf(&b, "CREATE SCHEMA %s;\n", s)
		}
		t, err := database.SanitizeIdentifier(c.Table)
		if err != nil {
			return "", errs.New("E520", "source tables unavailable")
		}
		defs := make([]string, 0, len(c.Columns))
		for _, col := range c.Columns {
			q, err := database.SanitizeIdentifier(col.Name)
			if err != nil {
				return "", errs.New("E520", "source tables unavailable")
			}
			defs = append(defs, q+" "+col.SQLType)
		}
		fmt.Fprintf(&b, "CREATE TABLE %s.%s (%s);\n", s, t, strings.Join(defs, ", "))
	}
	return b.String(), nil
}

// hasuraMetadata builds the metadata document with one object relationship per single-valued
// relationship column. Returns the document and the relationship count.
func hasuraMetadata(cols []*collection) ([]byte, int, error) {
	byKey := map[string]*collection{}
	for _, c := range cols {
		byKey[c.Schema+"\x00"+c.ID] = c
	}
	tables, rels := []map[string]any{}, 0
	for _, c := range cols {
		t := map[string]any{"table": map[string]any{"schema": c.Schema, "name": c.Table}}
		objs := []map[string]any{}
		for _, col := range c.Columns {
			remote, ok := byKey[c.Schema+"\x00"+col.Related]
			if col.Kind != "rel" || !ok {
				continue
			}
			objs = append(objs, map[string]any{"name": col.Name + "_rel", "using": map[string]any{"manual_configuration": map[string]any{"column_mapping": map[string]any{col.Name: "id"}, "insertion_order": nil, "remote_table": map[string]any{"schema": remote.Schema, "name": remote.Table}}}})
		}
		if len(objs) > 0 {
			sort.Slice(objs, func(i, j int) bool { return objs[i]["name"].(string) < objs[j]["name"].(string) })
			t["object_relationships"] = objs
			rels += len(objs)
		}
		tables = append(tables, t)
	}
	doc := map[string]any{"version": 3, "sources": []any{map[string]any{"name": "default", "kind": "postgres", "tables": tables}}}
	b, err := json.MarshalIndent(doc, "", "  ")
	return b, rels, err
}
