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

	// isoColumnsSQL lists where the two isolation columns exist. Fixed text; run as the
	// project's postgres user, which owns the tables, so information_schema shows every column.
	isoColumnsSQL = "SELECT table_schema, table_name, column_name FROM information_schema.columns " +
		"WHERE column_name IN ('source_account_id', 'tenant_id') " +
		"AND table_schema NOT IN ('pg_catalog', 'information_schema')"
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
	if meta.DefaultDB != "" && meta.DefaultDB != cfg.Postgres.DB {
		return []CheckResult{isoFail("source default", fmt.Sprintf("Hasura's default source reads database %q but the project's database is %q: the columns cannot be matched to the permissions", meta.DefaultDB, cfg.Postgres.DB))}
	}
	container := health.ContainerName(resolveProjectName(projectDir), "postgres")
	out, stderr, err := isoExecCapture(ctx, container, []string{"psql", "-X", "-A", "-t", "-F", isoFieldSep,
		"-U", cfg.Postgres.User, "-d", cfg.Postgres.DB, "-c", isoColumnsSQL})
	if err != nil {
		return []CheckResult{isoFail("columns", fmt.Sprintf("cannot read table columns from %s: %v %s", container, err, strings.TrimSpace(stderr)))}
	}
	cols, err := parseIsolationColumns(out)
	if err != nil {
		return []CheckResult{isoFail("columns", err.Error())}
	}
	results := isolationResults(meta.Tables, cols, strict)
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
}

// isoMeta is a parsed export: the default source's tables, the names of other
// sources that have tables (unchecked), and the database a literal default
// connection URL names ("" when it is not a literal URL).
type isoMeta struct {
	Tables    []isoTable
	Unchecked []string
	DefaultDB string
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
	var out isoMeta
	for _, src := range sources {
		if src.Name != "default" {
			if len(src.Tables) > 0 {
				out.Unchecked = append(out.Unchecked, src.Name)
			}
			continue
		}
		if lit, ok := src.Configuration.ConnectionInfo.DatabaseURL.(string); ok {
			u, uerr := url.Parse(lit)
			if out.DefaultDB = "?"; uerr == nil {
				out.DefaultDB = strings.TrimPrefix(u.Path, "/")
			}
		}
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
			for _, inh := range append(m.InheritedRoles, m.Metadata.InheritedRoles...) {
				var branches []any
				for _, member := range inh.RoleSet {
					if p, ok := t.Perms[member]["select"]; ok {
						branches = append(branches, p.Filter)
					}
				}
				if _, own := t.Perms[inh.RoleName]["select"]; !own && len(branches) > 0 {
					if t.Perms[inh.RoleName] == nil {
						t.Perms[inh.RoleName] = map[string]isoPerm{}
					}
					t.Perms[inh.RoleName]["select"] = isoPerm{Filter: map[string]any{"_or": branches}}
				}
			}
			out.Tables = append(out.Tables, t)
		}
	}
	return out, nil
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
