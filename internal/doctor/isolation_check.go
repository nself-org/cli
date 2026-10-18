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
	"fmt"
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

	// isoColumnsSQL lists where the two isolation columns exist. It reads pg_catalog, not
	// information_schema, because information_schema hides materialized views and every
	// column the connecting role has no privilege on. Fixed text, no user input.
	isoColumnsSQL = "SELECT n.nspname, c.relname, a.attname FROM pg_attribute a " +
		"JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace " +
		"WHERE a.attname IN ('source_account_id', 'tenant_id') AND a.attnum > 0 AND NOT a.attisdropped " +
		"AND c.relkind IN ('r', 'v', 'm', 'f', 'p') AND n.nspname NOT IN ('pg_catalog', 'information_schema')"

	// isoVolatileSQL lists the volatile functions (provolatile 'v') outside the system schemas.
	isoVolatileSQL = "SELECT n.nspname, p.proname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace " +
		"WHERE p.provolatile = 'v' AND n.nspname NOT IN ('pg_catalog', 'information_schema')"

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
	if meta.BindProblem != "" {
		return []CheckResult{isoFail("source default", meta.BindProblem+": the columns cannot be matched to the permissions")}
	}
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
	if len(cols) > 0 { // a wall exists in the database, tracked or not
		surfaces := meta.Surfaces
		if len(meta.Funcs) > 0 {
			volOut, stderr, err := isoExecCapture(ctx, container, []string{"psql", "-X", "-A", "-t", "-F", isoFieldSep,
				"-U", cfg.Postgres.User, "-d", cfg.Postgres.DB, "-c", isoVolatileSQL})
			if err != nil {
				return []CheckResult{isoFail("functions", fmt.Sprintf("cannot list volatile functions from %s: %v %s", container, err, strings.TrimSpace(stderr)))}
			}
			vol, err := parseIsolationRelations(volOut)
			if err != nil {
				return []CheckResult{isoFail("functions", err.Error())}
			}
			for _, f := range meta.Funcs {
				if vol[f.ID] {
					surfaces = append(surfaces, fmt.Sprintf("volatile function %s callable by %s", f.ID, strings.Join(f.Roles, ", ")))
				}
			}
		}
		for _, what := range surfaces {
			results = append(results, isoFail("surface", fmt.Sprintf("Hasura's default source exposes %s: it reads or writes outside the table permissions the guard checks", what)))
		}
	}
	for _, src := range meta.Unchecked {
		results = append(results, isoFail("source "+src, fmt.Sprintf("Hasura source %q tracks tables the guard cannot check: it reads columns from the default source's database only", src)))
	}
	return results
}

// isoTracked counts the tracked tables that have an isolation column.
func isoTracked(tables []isoTable, cols isoColumns) int {
	n := 0
	for _, t := range tables {
		if len(cols[t.id()]) > 0 {
			n++
		}
	}
	return n
}

// isolationResults runs the rules and renders them as check results.
func isolationResults(tables []isoTable, cols isoColumns, strict bool) []CheckResult {
	tracked := isoTracked(tables, cols)
	if tracked == 0 {
		return []CheckResult{{Section: isolationSection, Name: "ISO", Status: "pass",
			Message: "no tracked table has source_account_id or tenant_id: nothing to isolate"}}
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
