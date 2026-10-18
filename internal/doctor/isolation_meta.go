package doctor

// isolation_meta.go: the Hasura metadata model behind the isolation guard.
//
// Inputs: an export_metadata document. Outputs: the default source's tracked tables
// with per-role permissions (inherited roles resolved), the other sources that hold
// something the guard cannot check, and the facts bindDefaultSource needs.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// isoMetaPerm is one {role, permission} entry of a Hasura permission list.
type isoMetaPerm struct {
	Role       string         `json:"role"`
	Permission map[string]any `json:"permission"`
}

type isoMetaTable struct {
	Table struct {
		Schema string `json:"schema"`
		Name   string `json:"name"`
	} `json:"table"`
	Select []isoMetaPerm `json:"select_permissions"`
	Insert []isoMetaPerm `json:"insert_permissions"`
	Update []isoMetaPerm `json:"update_permissions"`
	Delete []isoMetaPerm `json:"delete_permissions"`
}

type isoMetaSource struct {
	Name          string `json:"name"`
	Configuration struct {
		ConnectionInfo struct {
			DatabaseURL any `json:"database_url"` // string, or {"from_env": NAME} (trusted: nSelf generates it)
		} `json:"connection_info"`
		ReadReplicas       []json.RawMessage `json:"read_replicas"`
		ConnectionTemplate any               `json:"connection_template"`
	} `json:"configuration"`
	Tables []isoMetaTable `json:"tables"`

	LogicalModels    []json.RawMessage `json:"logical_models"`
	NativeQueries    []json.RawMessage `json:"native_queries"`
	StoredProcedures []json.RawMessage `json:"stored_procedures"`
	Functions        []struct {
		Function struct {
			Schema string `json:"schema"`
			Name   string `json:"name"`
		} `json:"function"`
		Configuration struct {
			ExposedAs string `json:"exposed_as"`
		} `json:"configuration"`
		Permissions []struct {
			Role string `json:"role"`
		} `json:"permissions"`
	} `json:"functions"`
}

// hasContent reports whether a source tracks anything the guard would have to check.
func (s isoMetaSource) hasContent() bool {
	return len(s.Tables) > 0 || len(s.LogicalModels) > 0 || len(s.NativeQueries) > 0 || len(s.StoredProcedures) > 0 || len(s.Functions) > 0
}

// isoMeta is a parsed export: the default source's tables, the names of other
// sources that have tables (unchecked), and the default source's database_url
// as exported (nil when the export states none).
type isoMeta struct {
	Tables      []isoTable
	Unchecked   []string
	Surfaces    []string // default-source features outside table permissions (reported when a wall exists)
	Funcs       []isoFunc
	DefaultConn any
	BindProblem string // the default source cannot be bound to one database (replicas, templates, duplicates)
}

// isoFunc is a tracked function that a non-admin role may call.
type isoFunc struct {
	ID    string // schema.name
	Roles []string
}

// isoNselfURLVar is the variable nSelf's generated default source reads its
// connection from (compose points it at postgres:5432/<project database>).
const isoNselfURLVar = "HASURA_GRAPHQL_DATABASE_URL"

// bindDefaultSource proves the default source reads the project's own database,
// the one the column query runs against. It returns "" when proven, else the
// reason. Anything it cannot prove fails closed: a source on another server or
// database would make the local column list say nothing about the tables
// Hasura really serves.
func bindDefaultSource(conn any, db, container string) string {
	const tail = ": the columns cannot be matched to the permissions"
	switch v := conn.(type) {
	case nil:
		return "Hasura's default source states no connection" + tail
	case map[string]any:
		if name, _ := v["from_env"].(string); len(v) == 1 && name == isoNselfURLVar {
			return ""
		}
		return fmt.Sprintf("Hasura's default source connection is not nSelf's generated %s reference", isoNselfURLVar) + tail
	case string:
		u, err := url.Parse(v)
		if err != nil || u.Host == "" {
			return "Hasura's default source URL is not understood" + tail
		}
		if u.RawQuery != "" {
			return "Hasura's default source URL carries connection parameters (host, dbname and port can be overridden there)" + tail
		}
		got := strings.TrimPrefix(u.Path, "/")
		if got != db {
			return fmt.Sprintf("Hasura's default source reads database %q but the project's database is %q%s", got, db, tail)
		}
		if h := u.Hostname(); (h != "postgres" && h != container) || (u.Port() != "" && u.Port() != "5432") {
			return fmt.Sprintf("Hasura's default source reads %q, not the project's postgres container%s", u.Host, tail)
		}
		return ""
	}
	return "Hasura's default source connection has an unknown shape" + tail
}

type isoMetaBody struct {
	Sources        []isoMetaSource `json:"sources"`
	InheritedRoles []struct {
		RoleName string   `json:"role_name"`
		RoleSet  []string `json:"role_set"`
	} `json:"inherited_roles"`
}

// parseIsolationMetadata reads an export_metadata document (with or without the
// {"resource_version", "metadata"} wrapper) into the default source's tracked
// tables. Other sources with tables are returned by name: their columns live in
// another database, so they are reported, never passed. Inherited roles get the
// union of their members' select filters (an `_or`), so an unrestricted member
// such as nself_admin shows up on every role that inherits it.
func parseIsolationMetadata(doc []byte) (isoMeta, error) {
	var m struct {
		isoMetaBody
		Metadata isoMetaBody `json:"metadata"`
	}
	if err := json.Unmarshal(doc, &m); err != nil {
		return isoMeta{}, err
	}
	sources := append(m.Sources, m.Metadata.Sources...)
	if len(sources) == 0 {
		return isoMeta{}, fmt.Errorf("no sources in the metadata document")
	}
	inherited := append(m.InheritedRoles, m.Metadata.InheritedRoles...)
	byName := map[string][]string{}
	for _, inh := range inherited {
		byName[inh.RoleName] = inh.RoleSet
	}
	var out isoMeta
	seenDefault := false
	for _, src := range sources {
		if src.Name != "default" {
			if src.hasContent() {
				out.Unchecked = append(out.Unchecked, src.Name)
			}
			continue
		}
		if seenDefault {
			out.BindProblem = "the metadata has more than one source named default"
		}
		seenDefault = true
		if len(src.Configuration.ReadReplicas) > 0 || src.Configuration.ConnectionTemplate != nil {
			out.BindProblem = "Hasura's default source has read replicas or a connection template, which may serve another database"
		}
		out.DefaultConn = src.Configuration.ConnectionInfo.DatabaseURL
		out.Surfaces = append(out.Surfaces, isoSurfaces(src)...)
		out.Funcs = append(out.Funcs, isoFuncs(src)...)
		for _, mt := range src.Tables {
			t := isoTable{Schema: mt.Table.Schema, Name: mt.Table.Name, Perms: map[string]map[string]isoPerm{}}
			for op, list := range map[string][]isoMetaPerm{"select": mt.Select, "insert": mt.Insert, "update": mt.Update, "delete": mt.Delete} {
				for _, p := range list {
					if t.Perms[p.Role] == nil {
						t.Perms[p.Role] = map[string]isoPerm{}
					}
					t.Perms[p.Role][op] = isoPerm{Filter: p.Permission["filter"], Check: p.Permission["check"],
						Set: p.Permission["set"], Columns: p.Permission["columns"]}
				}
			}
			for _, inh := range inherited {
				inheritPerms(&t, inh.RoleName, flattenRoleSet(inh.RoleSet, byName, map[string]bool{inh.RoleName: true}))
			}
			out.Tables = append(out.Tables, t)
		}
	}
	return out, nil
}

// flattenRoleSet expands members that are themselves inherited roles into their own
// members (Hasura allows no nesting, but a hand-edited export may have it), so the
// result does not depend on the order of the inherited_roles list. seen breaks cycles.
func flattenRoleSet(members []string, byName map[string][]string, seen map[string]bool) []string {
	var out []string
	for _, m := range members {
		if set, ok := byName[m]; ok {
			if !seen[m] {
				seen[m] = true
				out = append(out, flattenRoleSet(set, byName, seen)...)
			}
			continue
		}
		out = append(out, m)
	}
	return out
}

// isoSurfaces names the default-source features that read data outside table
// permissions: logical models, native queries and stored procedures.
func isoSurfaces(src isoMetaSource) []string {
	var out []string
	for _, k := range []struct {
		kind string
		n    int
	}{{"logical models", len(src.LogicalModels)}, {"native queries", len(src.NativeQueries)}, {"stored procedures", len(src.StoredProcedures)}} {
		if k.n > 0 {
			out = append(out, k.kind)
		}
	}
	return out
}

// isoFuncs lists the tracked functions a non-admin role may call. Whether one is
// volatile (runs as Hasura's DB user, so table filters do not limit what it
// writes) is read from pg_proc, because an export need not state exposed_as.
func isoFuncs(src isoMetaSource) []isoFunc {
	var out []isoFunc
	for _, f := range src.Functions {
		schema := f.Function.Schema
		if schema == "" {
			schema = "public"
		}
		fn := isoFunc{ID: schema + "." + f.Function.Name}
		for _, p := range f.Permissions {
			if p.Role != "admin" {
				fn.Roles = append(fn.Roles, p.Role)
			}
		}
		if len(fn.Roles) > 0 {
			out = append(out, fn)
		}
	}
	return out
}

// inheritPerms gives inherited role `name` the permissions Hasura derives from its
// members, for every operation the role does not define itself. Select gets one
// synthetic filter, the `_or` of the members' filters. A mutation permission is
// inherited when the members agree (Hasura marks it inconsistent otherwise); here
// each member's permission is checked as its own role named "name (via member)",
// which fails closed when they disagree.
func inheritPerms(t *isoTable, name string, members []string) {
	set := func(role, op string, p isoPerm) {
		if t.Perms[role] == nil {
			t.Perms[role] = map[string]isoPerm{}
		}
		t.Perms[role][op] = p
	}
	var branches []any
	for _, member := range members {
		if p, ok := t.Perms[member]["select"]; ok {
			branches = append(branches, p.Filter)
		}
	}
	if _, own := t.Perms[name]["select"]; !own && len(branches) > 0 {
		set(name, "select", isoPerm{Filter: map[string]any{"_or": branches}})
	}
	for _, op := range []string{"insert", "update", "delete"} {
		if _, own := t.Perms[name][op]; own {
			continue
		}
		for _, member := range members {
			if p, ok := t.Perms[member][op]; ok {
				set(name+" (via "+member+")", op, p)
			}
		}
	}
}
