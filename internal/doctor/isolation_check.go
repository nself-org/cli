package doctor

// Purpose: doctor --deep section `isolation` (ISO-01..ISO-04, P7-ADOPT-05): the
// ADR 0004 wall checked on every tracked table and every non-admin role.
// Inputs: the project directory. Live Hasura metadata comes from
// export_metadata; the isolation columns come from information_schema.columns
// through psql in the project's postgres container (docker.ExecCapture).
// Outputs: one CheckResult per finding (section "isolation", name
// "ISO-0N: <schema.table> <role> <op>"), one pass per clean rule, or a single
// "not applicable" pass when no table has either column.
// Constraints: read-only. Fail-closed: any read or parse error is one fail
// result, never a pass (Constitution §4.1). The SQL is one fixed statement with
// no parameters, so no table name is ever interpolated (§7.3a). Strict mode
// (NSELF_DOCTOR_STRICT=1) turns the ISO-01 warning into a fail.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/health"
)

const (
	isolationSection = "isolation"
	isolationTimeout = 30 * time.Second
	isolationStrict  = "NSELF_DOCTOR_STRICT"
	isoFieldSep      = "\x1f"

	// isoColumnsSQL lists where the two isolation columns exist. It reads pg_catalog, not
	// information_schema, because information_schema hides materialized views and every
	// column the connecting role has no privilege on. Fixed text, no user input.
	isoColumnsSQL = "SELECT n.nspname, c.relname, a.attname FROM pg_attribute a " +
		"JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace " +
		"WHERE a.attname IN ('source_account_id', 'tenant_id') AND a.attnum > 0 AND NOT a.attisdropped " +
		"AND c.relkind IN ('r', 'v', 'm', 'f', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')"

	// isoRelationsSQL lists every relation the project database has, to prove each table
	// Hasura tracks was seen by the column query.
	isoRelationsSQL = "SELECT n.nspname, c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace " +
		"WHERE c.relkind IN ('r', 'v', 'm', 'f', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')"
)

// isoExecCapture is replaced by tests so no unit test starts docker.
var isoExecCapture = docker.ExecCapture

// IsolationChecks is the DeepChecks entry point for section `isolation`.
func IsolationChecks(ctx context.Context, projectDir string) []CheckResult {
	if projectDir == "" {
		return []CheckResult{isoFail("isolation", "no project directory, so the wall cannot be checked")}
	}
	strict := isoTruthy(os.Getenv(isolationStrict))
	cfg, env, err := LoadConfigIsolatedLookup(projectDir, isolationStrict)
	if err != nil {
		return []CheckResult{isoFail("project config", fmt.Sprintf("project config unreadable: %v", err))}
	}
	strict = strict || isoTruthy(env[isolationStrict])
	ctx, cancel := context.WithTimeout(ctx, isolationTimeout)
	defer cancel()
	live, err := exportLiveMetadata(ctx, cfg)
	if err != nil {
		return []CheckResult{isoFail("metadata", fmt.Sprintf("live Hasura metadata unreadable (is the stack running?): %v", err))}
	}
	meta, err := parseIsolationMetadata(live)
	if err != nil {
		return []CheckResult{isoFail("metadata", fmt.Sprintf("Hasura metadata not understood: %v", err))}
	}
	container := health.ContainerName(resolveProjectName(projectDir), "postgres")
	if why := bindDefaultSource(meta.DefaultConn, cfg.Postgres.DB, container); why != "" {
		return []CheckResult{isoFail("source default", why)}
	}
	out, stderr, err := isoExecCapture(ctx, container, []string{"psql", "-X", "-A", "-t", "-F", isoFieldSep,
		"-U", cfg.Postgres.User, "-d", cfg.Postgres.DB, "-c", isoColumnsSQL})
	if err != nil {
		return []CheckResult{isoFail("columns", fmt.Sprintf("cannot read table columns from %s: %v %s", container, err, strings.TrimSpace(stderr)))}
	}
	cols, err := parseIsolationColumns(out)
	if err != nil {
		return []CheckResult{isoFail("columns", err.Error())}
	}
	relOut, stderr, err := isoExecCapture(ctx, container, []string{"psql", "-X", "-A", "-t", "-F", isoFieldSep,
		"-U", cfg.Postgres.User, "-d", cfg.Postgres.DB, "-c", isoRelationsSQL})
	if err != nil {
		return []CheckResult{isoFail("relations", fmt.Sprintf("cannot list relations from %s: %v %s", container, err, strings.TrimSpace(stderr)))}
	}
	rels, err := parseIsolationRelations(relOut)
	if err != nil {
		return []CheckResult{isoFail("relations", err.Error())}
	}
	for _, t := range meta.Tables {
		if !rels[t.id()] {
			return []CheckResult{isoFail("table "+t.id(), fmt.Sprintf("Hasura tracks %s but the project database has no such relation: the columns cannot be matched to the permissions", t.id()))}
		}
	}
	results := isolationResults(meta.Tables, cols, strict)
	if len(cols) > 0 {
		for _, what := range meta.Surfaces {
			results = append(results, isoFail("surface", fmt.Sprintf("Hasura's default source exposes %s: it reads data outside the table permissions the guard checks", what)))
		}
	}
	for _, src := range meta.Unchecked {
		results = append(results, isoFail("source "+src, fmt.Sprintf("Hasura source %q tracks tables the guard cannot check: it reads columns from the default source's database only", src)))
	}
	return results
}

// isolationResults runs the rules and renders them as check results.
func isolationResults(tables []isoTable, cols isoColumns, strict bool) []CheckResult {
	tracked := 0
	for _, t := range tables {
		if len(cols[t.id()]) > 0 {
			tracked++
		}
	}
	if tracked == 0 {
		return []CheckResult{{Section: isolationSection, Name: "ISO", Status: "pass",
			Message: "no table has source_account_id or tenant_id: nothing to isolate"}}
	}
	findings := isolationRules(tables, cols, strict)
	byRule := map[string]int{}
	var results []CheckResult
	for _, f := range findings {
		byRule[f.Rule]++
		status := "warn"
		if f.Fail {
			status = "fail"
		}
		name := f.Rule + ": " + f.Table
		msg := f.Table + ": " + f.Detail
		if f.Role != "" {
			name += " " + f.Role + " " + f.Op
			msg = fmt.Sprintf("%s role=%s op=%s: %s", f.Table, f.Role, f.Op, f.Detail)
		}
		results = append(results, CheckResult{Section: isolationSection, Name: name, Status: status, Message: msg,
			FixCmd: "fix the Hasura permission, then run nself db reconcile --apply (see the Multi-App-Isolation wiki page)"})
	}
	for _, rule := range []string{"ISO-01", "ISO-02", "ISO-03", "ISO-04"} {
		if byRule[rule] == 0 {
			results = append(results, CheckResult{Section: isolationSection, Name: rule, Status: "pass",
				Message: fmt.Sprintf("%d tracked table(s) with an isolation column checked, no violation", tracked)})
		}
	}
	return results
}

func isoFail(what, msg string) CheckResult {
	return CheckResult{Section: isolationSection, Name: "ISO: " + what, Status: "fail", Message: msg,
		FixCmd: "nself start, then nself doctor --deep --only isolation"}
}

func isoTruthy(v string) bool {
	v = strings.TrimSpace(v)
	return v == "1" || strings.EqualFold(v, "true")
}

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

// isoMeta is a parsed export: the default source's tables, the names of other
// sources that have tables (unchecked), and the default source's database_url
// as exported (nil when the export states none).
type isoMeta struct {
	Tables      []isoTable
	Unchecked   []string
	Surfaces    []string // default-source features outside table permissions (reported when a wall exists)
	DefaultConn any
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
	for _, src := range sources {
		if src.Name != "default" {
			if len(src.Tables) > 0 {
				out.Unchecked = append(out.Unchecked, src.Name)
			}
			continue
		}
		out.DefaultConn = src.Configuration.ConnectionInfo.DatabaseURL
		out.Surfaces = isoSurfaces(src)
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

// isoSurfaces names the default-source features that read or write data outside table
// permissions: logical models, native queries, stored procedures, and tracked mutation
// functions callable by a non-admin role (a volatile function runs as Hasura's DB user).
func isoSurfaces(src isoMetaSource) []string {
	var out []string
	for kind, n := range map[string]int{"logical models": len(src.LogicalModels), "native queries": len(src.NativeQueries), "stored procedures": len(src.StoredProcedures)} {
		if n > 0 {
			out = append(out, kind)
		}
	}
	sort.Strings(out)
	for _, f := range src.Functions {
		if f.Configuration.ExposedAs != "mutation" {
			continue
		}
		for _, p := range f.Permissions {
			if p.Role != "admin" {
				out = append(out, fmt.Sprintf("mutation function %s.%s for role %s", f.Function.Schema, f.Function.Name, p.Role))
			}
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

// parseIsolationRelations reads the psql rows "schema<US>table" into a set of ids.
func parseIsolationRelations(out string) (map[string]bool, error) {
	rels := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(strings.TrimRight(line, "\r"), isoFieldSep)
		if len(f) != 2 {
			return nil, fmt.Errorf("unexpected relation row from psql: %q", line)
		}
		rels[f[0]+"."+f[1]] = true
	}
	return rels, nil
}

// parseIsolationColumns reads the psql rows "schema<US>table<US>column".
func parseIsolationColumns(out string) (isoColumns, error) {
	cols := isoColumns{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(strings.TrimRight(line, "\r"), isoFieldSep)
		if len(f) != 3 {
			return nil, fmt.Errorf("unexpected column row from psql: %q", line)
		}
		id := f[0] + "." + f[1]
		if cols[id] == nil {
			cols[id] = map[string]bool{}
		}
		cols[id][f[2]] = true
	}
	return cols, nil
}
