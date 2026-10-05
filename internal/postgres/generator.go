package postgres

import (
	"fmt"
	"os"
	"path/filepath"
)

// initScriptSQL is the structural-schema init script (01-init.sql).
const initScriptSQL = `CREATE SCHEMA IF NOT EXISTS auth;
CREATE SCHEMA IF NOT EXISTS storage;

-- Hasura internal schemas (required before Hasura starts)
CREATE SCHEMA IF NOT EXISTS hdb_catalog;
CREATE SCHEMA IF NOT EXISTS hdb_views;

-- Grant permissions on all schemas (supports non-superuser Hasura deployments)
DO $$ BEGIN
  EXECUTE format('GRANT ALL ON SCHEMA auth, storage, hdb_catalog, hdb_views, public TO %I', current_user);
  EXECUTE format('GRANT ALL ON ALL TABLES IN SCHEMA auth, storage, hdb_catalog, hdb_views, public TO %I', current_user);
  EXECUTE format('GRANT ALL ON ALL SEQUENCES IN SCHEMA auth, storage, hdb_catalog, hdb_views, public TO %I', current_user);
  EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA auth, storage, hdb_catalog, hdb_views, public GRANT ALL ON TABLES TO %I', current_user);
  EXECUTE format('ALTER DEFAULT PRIVILEGES IN SCHEMA auth, storage, hdb_catalog, hdb_views, public GRANT ALL ON SEQUENCES TO %I', current_user);
END $$;

-- Extensions
CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
`

// RenderInitScript returns the Postgres init files GenerateInitScript writes,
// keyed by path relative to the project root, without touching the disk.
// Plan mode (internal/build, P7-LIVE-21) writes them through its Sink; both
// modes therefore render the same bytes. workdir only anchors future callers
// root, accepted for symmetry with GenerateInitScript and not read.
func RenderInitScript(_ string) (map[string][]byte, error) {
	return map[string][]byte{
		"postgres/init/01-init.sql":       []byte(initScriptSQL),
		"postgres/init/04-np-plugins.sql": []byte(npPluginsInitSQL),
	}, nil
}

// GenerateInitScript creates the basic Postgres initialization SQL script
// containing the structural schemas required by Nhost containers, and also
// writes the np_plugins bootstrap script so that the table is created
// idempotently on every stack start.
func GenerateInitScript(workdir string) error {
	dir := filepath.Join(workdir, "postgres", "init")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating postgres init dir: %w", err)
	}

	path := filepath.Join(dir, "01-init.sql")
	if err := os.WriteFile(path, []byte(initScriptSQL), 0644); err != nil {
		return fmt.Errorf("writing postgres init script: %w", err)
	}

	// Write the np_plugins bootstrap script as a separate init file so it runs
	// after 01-init.sql. Using CREATE TABLE IF NOT EXISTS ensures idempotency:
	// running nself build a second time on an existing stack never errors.
	if err := writeNpPluginsInitScript(dir); err != nil {
		return fmt.Errorf("writing np_plugins init script: %w", err)
	}

	return nil
}

// npPluginsInitSQL is the canonical schema for the np_plugins table.
// The table tracks every plugin installed into this nSelf stack: its name,
// version, license tier, enabled state, and arbitrary metadata. Using
// CREATE TABLE IF NOT EXISTS guarantees idempotency — running nself build
// twice never errors, never duplicates, and never drops existing rows.
const npPluginsInitSQL = `-- np_plugins: plugin installation registry
-- Created by nself build (idempotent — safe to re-run on existing stacks).
CREATE TABLE IF NOT EXISTS np_plugins (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    version     TEXT        NOT NULL DEFAULT '',
    tier        TEXT        NOT NULL DEFAULT 'free' CHECK (tier IN ('free', 'pro', 'internal')),
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    installed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata    JSONB       NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT np_plugins_name_unique UNIQUE (name)
);

-- Index for fast enabled-plugin lookups (used by the plugin loader at startup).
CREATE INDEX IF NOT EXISTS np_plugins_enabled_idx ON np_plugins (enabled) WHERE enabled = true;
`

// writeNpPluginsInitScript writes postgres/init/04-np-plugins.sql to workdir.
// The file number 04 places it after the existing 01-03 init scripts so that
// the required schemas (auth, storage, extensions) are already present when
// this script runs.
func writeNpPluginsInitScript(initDir string) error {
	path := filepath.Join(initDir, "04-np-plugins.sql")
	if err := os.WriteFile(path, []byte(npPluginsInitSQL), 0644); err != nil {
		return fmt.Errorf("writing 04-np-plugins.sql: %w", err)
	}
	return nil
}

// EnsureNpPluginsTable returns the SQL statement that idempotently creates the
// np_plugins table. Callers (e.g. the live DB bootstrap path) can execute this
// directly against a running Postgres container when the postgres/init/ dir is
// not available (e.g. in upgrade scenarios).
//
// The statement uses CREATE TABLE IF NOT EXISTS so it is always safe to
// execute more than once.
func EnsureNpPluginsTable() string {
	return npPluginsInitSQL
}
