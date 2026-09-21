package config

import "testing"

// TestPgvectorEnabled_AddsExtensionWhenDefaultList covers the defect found
// on the 2026-09-21 E2E golden path run: `nself init` (and every preset)
// writes PGVECTOR_ENABLED=true to .env by default, but nothing previously
// connected that toggle to POSTGRES_EXTENSIONS — the field
// internal/compose/images.go's pgvector image selection actually reads — so
// a fresh project silently built on plain postgres:16-alpine instead of
// pgvector/pgvector:pg16 and the claw plugin's `CREATE EXTENSION vector`
// failed. With no POSTGRES_EXTENSIONS override, PGVECTOR_ENABLED=true must
// append "pgvector" onto the default extension list.
func TestPgvectorEnabled_AddsExtensionWhenDefaultList(t *testing.T) {
	t.Setenv("PGVECTOR_ENABLED", "true")
	t.Setenv("POSTGRES_EXTENSIONS", "")

	cfg := &Config{}
	parseEnvCore(cfg)

	if !hasExtensionCI(cfg.Postgres.Extensions, "pgvector") {
		t.Errorf("Extensions = %v, want it to contain pgvector", cfg.Postgres.Extensions)
	}
	// The default set must still be present — this is additive, not a replace.
	for _, want := range []string{"uuid-ossp", "pgcrypto", "pg_trgm"} {
		if !hasExtensionCI(cfg.Postgres.Extensions, want) {
			t.Errorf("Extensions = %v, want it to still contain default %q", cfg.Postgres.Extensions, want)
		}
	}
}

// TestPgvectorEnabled_AddsExtensionOntoExplicitList covers an operator who
// set POSTGRES_EXTENSIONS explicitly (without pgvector) alongside
// PGVECTOR_ENABLED=true — the toggle must still win and append pgvector.
func TestPgvectorEnabled_AddsExtensionOntoExplicitList(t *testing.T) {
	t.Setenv("PGVECTOR_ENABLED", "true")
	t.Setenv("POSTGRES_EXTENSIONS", "uuid-ossp")

	cfg := &Config{}
	parseEnvCore(cfg)

	if !hasExtensionCI(cfg.Postgres.Extensions, "pgvector") {
		t.Errorf("Extensions = %v, want it to contain pgvector", cfg.Postgres.Extensions)
	}
}

// TestPgvectorEnabled_NoDuplicateWhenAlreadyListed covers an operator who
// already listed pgvector (any case) in POSTGRES_EXTENSIONS — the toggle
// must not duplicate the entry.
func TestPgvectorEnabled_NoDuplicateWhenAlreadyListed(t *testing.T) {
	t.Setenv("PGVECTOR_ENABLED", "true")
	t.Setenv("POSTGRES_EXTENSIONS", "uuid-ossp,PgVector")

	cfg := &Config{}
	parseEnvCore(cfg)

	count := 0
	for _, e := range cfg.Postgres.Extensions {
		if hasExtensionCI([]string{e}, "pgvector") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("Extensions = %v, want exactly one pgvector entry, got %d", cfg.Postgres.Extensions, count)
	}
}

// TestPgvectorEnabled_FalseOrUnsetLeavesExtensionsAlone is the backward-compat
// guard: a project with PGVECTOR_ENABLED unset (every pre-existing project)
// or explicitly false must never gain pgvector.
func TestPgvectorEnabled_FalseOrUnsetLeavesExtensionsAlone(t *testing.T) {
	for _, val := range []string{"", "false"} {
		t.Setenv("PGVECTOR_ENABLED", val)
		t.Setenv("POSTGRES_EXTENSIONS", "")

		cfg := &Config{}
		parseEnvCore(cfg)

		if hasExtensionCI(cfg.Postgres.Extensions, "pgvector") {
			t.Errorf("PGVECTOR_ENABLED=%q: Extensions = %v, must not contain pgvector", val, cfg.Postgres.Extensions)
		}
	}
}
