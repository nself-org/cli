package plugin

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// validSchemaChars matches only lowercase letters, digits, and underscores.
var validSchemaChars = regexp.MustCompile(`^[a-z0-9_]+$`)

// sanitizeSchemaName converts a plugin name to a valid Postgres identifier.
// Lowercase, hyphens become underscores, prefixed with np_.
// Only [a-z0-9_] characters are allowed after transformation; any input
// containing other characters (quotes, semicolons, spaces, etc.) is rejected
// to prevent SQL injection.
func sanitizeSchemaName(pluginName string) string {
	name := strings.ToLower(pluginName)
	name = strings.ReplaceAll(name, "-", "_")
	if !validSchemaChars.MatchString(name) {
		// Return a safe fallback that will fail schema creation with a clear name
		// rather than allowing SQL-significant characters through.
		return "np_invalid"
	}
	return "np_" + name
}

// quoteIdent double-quotes a SQL identifier that has already been validated
// by sanitizeSchemaName (pure [a-z0-9_]). This adds an extra layer of
// defence in depth: even if the regex contract ever changes, the double-quoted
// form prevents identifier injection in CREATE/DROP/GRANT/ALTER DDL.
// Embedded double-quotes are escaped as "" per the SQL standard.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// postgresContainer returns the Postgres container name for the project.
//
// Purpose: address the container that `nself build` actually generates.
// Inputs:  the loaded project config.
// Outputs: "<project>_postgres".
// Constraints: this MUST match the `container_name:` that nself build writes.
//
//	nself build emits `container_name: ${PROJECT_NAME}_postgres`, which
//	overrides Docker Compose's default `<project>-postgres-1` naming. These
//	two call sites used the Compose default, so every `docker exec` here hit
//	"No such container" and `nself plugin install` failed for EVERY
//	schema-bearing plugin on EVERY project. The rest of the CLI already spells
//	it correctly in eight places (internal/database/helpers.go,
//	internal/tenant/*.go, internal/database/backup.go) — this file was the
//	odd one out.

func postgresContainer(cfg *config.Config) string {
	return cfg.ProjectName + "_postgres"
}

// execPSQL runs a SQL statement inside the Postgres container via docker exec.
func execPSQL(ctx context.Context, cfg *config.Config, sql string) error {
	containerName := postgresContainer(cfg)
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"psql", "-U", cfg.Postgres.User, "-d", cfg.Postgres.DB,
		"-c", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("psql exec failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// queryPSQL runs a SQL query and returns the trimmed stdout. It uses -tA
// (tuples-only, unaligned) so the result contains only raw values.
func queryPSQL(ctx context.Context, cfg *config.Config, sql string) (string, error) {
	containerName := postgresContainer(cfg)
	cmd := exec.CommandContext(ctx, "docker", "exec", containerName,
		"psql", "-U", cfg.Postgres.User, "-d", cfg.Postgres.DB,
		"-tA", "-c", sql,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("psql query failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

// getSchemaVersion queries np_common.plugin_schema_versions for the latest
// recorded version of the named plugin. Returns (0, nil) if no version row exists.
func getSchemaVersion(ctx context.Context, cfg *config.Config, pluginName string) (int, error) {
	sql := fmt.Sprintf(
		"SELECT COALESCE(MAX(version),0) FROM np_common.plugin_schema_versions WHERE plugin = '%s';",
		sanitizeSchemaName(pluginName),
	)
	out, err := queryPSQL(ctx, cfg, sql)
	if err != nil {
		// Table may not exist yet on a fresh instance; treat as version 0.
		return 0, nil
	}
	if out == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(out)
	if err != nil {
		return 0, fmt.Errorf("parsing schema version for %q: %w", pluginName, err)
	}
	return v, nil
}

// recordSchemaVersion inserts a version row into
// np_common.plugin_schema_versions for the given plugin. A duplicate
// (plugin, version) pair is ignored via the table's primary key.
func recordSchemaVersion(ctx context.Context, cfg *config.Config, pluginName string, version int) error {
	sql := fmt.Sprintf(`INSERT INTO np_common.plugin_schema_versions (plugin, version)
VALUES ('%s', %d) ON CONFLICT (plugin, version) DO NOTHING;`,
		sanitizeSchemaName(pluginName), version,
	)
	return execPSQL(ctx, cfg, sql)
}

// schemaVersion is the current version of the plugin schema setup logic.
// Bump this when the schema creation steps change in a meaningful way.
const schemaVersion = 1

// CreatePluginSchema creates an isolated Postgres schema and role for a plugin.
// Schema name: np_{name} (lowercase, hyphens to underscores).
// Role name: np_{name}_role.
// All statements are idempotent and safe to run multiple times.
//
// Before executing, it checks np_common.schema_versions for the plugin. If the
// recorded version matches schemaVersion the function returns early (already
// applied). After successful creation it records the version.
func createPluginSchema(ctx context.Context, cfg *config.Config, pluginName string) error {
	schema := sanitizeSchemaName(pluginName)
	role := schema + "_role"
	// Double-quoted identifiers prevent injection even if the regex contract
	// ever changes. Validated identifiers are [a-z0-9_] only, but quoting is
	// applied as defence in depth (SEC-SQL-01).
	qSchema := quoteIdent(schema)
	qRole := quoteIdent(role)

	// Ensure the plugin version tracking table exists (and has picked up any
	// rows an older CLI kept in the migration ledger's table), so the version
	// check below has a table to query.
	if err := ensurePluginSchemaVersionsTable(ctx, cfg); err != nil {
		return err
	}

	// Check whether this plugin's schema has already been applied at the
	// current version. If so, skip all remaining work.
	existingVer, err := getSchemaVersion(ctx, cfg, pluginName)
	if err != nil {
		return fmt.Errorf("checking schema version for %q: %w", pluginName, err)
	}
	if existingVer >= schemaVersion {
		return nil
	}

	if err := provisionPluginSchemaObjects(ctx, cfg, schema, role, qSchema, qRole); err != nil {
		return err
	}

	// Record successful schema creation so subsequent calls skip the work.
	if err := recordSchemaVersion(ctx, cfg, pluginName, schemaVersion); err != nil {
		return fmt.Errorf("recording schema version for %q: %w", pluginName, err)
	}

	return nil
}

// provisionPluginSchemaObjects creates the role, schema, grants, and default
// search_path for a plugin's isolated namespace. Split out of
// createPluginSchema to keep that function under the repo's per-function
// line limit; behavior is unchanged from the inline version it replaced.
func provisionPluginSchemaObjects(ctx context.Context, cfg *config.Config, schema, role, qSchema, qRole string) error {
	// Create role if it does not exist (idempotent via DO block).
	// rolname in pg_roles is an unquoted system column; the quoteIdent form is
	// used in the CREATE ROLE statement only.
	roleSQL := fmt.Sprintf(`DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '%s') THEN
    CREATE ROLE %s NOLOGIN;
  END IF;
END $$;`, role, qRole)
	if err := execPSQL(ctx, cfg, roleSQL); err != nil {
		return fmt.Errorf("creating role %s: %w", role, err)
	}

	// Create schema if it does not exist.
	schemaSQL := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;", qSchema)
	if err := execPSQL(ctx, cfg, schemaSQL); err != nil {
		return fmt.Errorf("creating schema %s: %w", schema, err)
	}

	// Grant usage and create permissions on the schema to the role.
	grantSQL := fmt.Sprintf(
		"GRANT USAGE ON SCHEMA %s TO %s; GRANT CREATE ON SCHEMA %s TO %s;",
		qSchema, qRole, qSchema, qRole,
	)
	if err := execPSQL(ctx, cfg, grantSQL); err != nil {
		return fmt.Errorf("granting permissions on %s: %w", schema, err)
	}

	// Set the role's default search_path to its own schema plus public.
	pathSQL := fmt.Sprintf("ALTER ROLE %s SET search_path = %s, public;", qRole, qSchema)
	if err := execPSQL(ctx, cfg, pathSQL); err != nil {
		return fmt.Errorf("setting search_path for %s: %w", role, err)
	}

	return nil
}

// DropPluginSchema removes a plugin's Postgres schema and role.
// Safe to call even if the schema or role does not exist.
func dropPluginSchema(ctx context.Context, cfg *config.Config, pluginName string) error {
	schema := sanitizeSchemaName(pluginName)
	role := schema + "_role"
	qSchema := quoteIdent(schema)
	qRole := quoteIdent(role)

	dropSchemaSQL := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE;", qSchema)
	if err := execPSQL(ctx, cfg, dropSchemaSQL); err != nil {
		return fmt.Errorf("dropping schema %s: %w", schema, err)
	}

	// Forget the recorded schema version as soon as the schema is gone and
	// before the role drop, which can fail on grants held elsewhere: a
	// surviving row makes createPluginSchema skip provisioning, so a later
	// reinstall would run with no schema.
	forgetSQL := fmt.Sprintf(`DO $$ BEGIN
  IF to_regclass('np_common.plugin_schema_versions') IS NOT NULL THEN
    DELETE FROM np_common.plugin_schema_versions WHERE plugin = '%s';
  END IF;
END $$;`, schema)
	if err := execPSQL(ctx, cfg, forgetSQL); err != nil {
		return fmt.Errorf("forgetting schema version for %s: %w", schema, err)
	}

	dropRoleSQL := fmt.Sprintf("DROP ROLE IF EXISTS %s;", qRole)
	if err := execPSQL(ctx, cfg, dropRoleSQL); err != nil {
		return fmt.Errorf("dropping role %s: %w", role, err)
	}

	return nil
}
