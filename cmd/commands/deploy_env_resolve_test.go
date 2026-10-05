package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeployEnvCascadeFiles_MatchesConfigLoadOrder verifies the file list per
// target mirrors config.Load's own cascade (internal/config/loader.go):
// config.EnvCascadeOrder, the single owner of the order (.env, the env's own
// layer, .env.secrets, .env.local; a custom env also loads .env.dev). This is
// the contract writeResolvedDeployEnv depends on for gap #13.
func TestDeployEnvCascadeFiles_MatchesConfigLoadOrder(t *testing.T) {
	workdir := "/proj"
	cases := map[string][]string{
		"local":   {".env", ".env.dev", ".env.secrets", ".env.local"},
		"staging": {".env", ".env.staging", ".env.secrets"}, // remote: no .env.local
		"prod":    {".env", ".env.prod", ".env.secrets"},
	}
	for target, wantSuffixes := range cases {
		got := deployEnvCascadeFiles(workdir, target)
		if len(got) != len(wantSuffixes) {
			t.Fatalf("target %s: got %d files, want %d: %v", target, len(got), len(wantSuffixes), got)
		}
		for i, suffix := range wantSuffixes {
			want := filepath.Join(workdir, suffix)
			if got[i] != want {
				t.Errorf("target %s file[%d]: got %q, want %q", target, i, got[i], want)
			}
		}
	}
}

// TestReadEnvFileOverrides_MissingFileIsEmpty verifies a nonexistent file
// returns an empty, error-free result — matching config.Load's "each file is
// optional" semantics.
func TestReadEnvFileOverrides_MissingFileIsEmpty(t *testing.T) {
	pairs, err := readEnvFileOverrides(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pairs) != 0 {
		t.Errorf("expected no pairs for a missing file, got %v", pairs)
	}
}

// TestReadEnvFileOverrides_ParsesAndStripsQuotes verifies basic KEY=VALUE
// parsing, comment/blank-line skipping, and quote stripping.
func TestReadEnvFileOverrides_ParsesAndStripsQuotes(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env.test")
	content := "# a comment\n\nPOSTGRES_DB=nself\nHASURA_GRAPHQL_ADMIN_SECRET=\"s3cr3t\"\nQUOTED_SINGLE='abc'\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	pairs, err := readEnvFileOverrides(envPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := map[string]string{}
	for _, kv := range pairs {
		got[kv.key] = kv.value
	}
	want := map[string]string{
		"POSTGRES_DB":                 "nself",
		"HASURA_GRAPHQL_ADMIN_SECRET": "s3cr3t",
		"QUOTED_SINGLE":               "abc",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("key %s: got %q, want %q", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("expected exactly %d keys, got %d: %v", len(want), len(got), got)
	}
}

// TestWriteResolvedDeployEnv_LaterFilesOverrideEarlier verifies the merge
// order matches config.Load: a key present in both .env.dev and .env.staging
// takes the .env.staging value (later cascade entries win). This is the
// core of the gap #13 fix — without it, the pushed env could silently keep
// a stale dev-tier value that no longer matches what was baked into
// docker-compose.yml.
func TestWriteResolvedDeployEnv_LaterFilesOverrideEarlier(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("POSTGRES_DB=dev_db\nSHARED_KEY=from_dev\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.staging"), []byte("POSTGRES_DB=staging_db\n"), 0o600); err != nil {
		t.Fatalf("write .env.staging: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env.secrets"), []byte("HASURA_GRAPHQL_ADMIN_SECRET=topsecret\n"), 0o600); err != nil {
		t.Fatalf("write .env.secrets: %v", err)
	}

	path, cleanup, err := writeResolvedDeployEnv(dir, "staging")
	defer cleanup()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resolved snapshot: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "POSTGRES_DB=staging_db") {
		t.Errorf("expected POSTGRES_DB to be overridden by .env.staging (staging_db); got:\n%s", content)
	}
	if strings.Contains(content, "POSTGRES_DB=dev_db") {
		t.Errorf("did not expect the stale .env POSTGRES_DB value to survive the merge; got:\n%s", content)
	}
	if !strings.Contains(content, "SHARED_KEY=from_dev") {
		t.Errorf("expected SHARED_KEY (only in .env) to be preserved; got:\n%s", content)
	}
	if !strings.Contains(content, "HASURA_GRAPHQL_ADMIN_SECRET=topsecret") {
		t.Errorf("expected .env.secrets values to be included; got:\n%s", content)
	}
}

// TestWriteResolvedDeployEnv_CleanupRemovesFile verifies the returned cleanup
// function actually deletes the temp snapshot so repeated deploys don't
// accumulate stray files in the project root.
func TestWriteResolvedDeployEnv_CleanupRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path, cleanup, err := writeResolvedDeployEnv(dir, "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected snapshot file to exist before cleanup: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("expected snapshot file to be removed after cleanup, stat err: %v", statErr)
	}
}

// TestLoadDeployEnvCascade_SetsENVForConfigLoad verifies gap #13's root-cause
// fix: loadDeployEnvCascade must set the ENV variable (not just
// NSELF_DEPLOY_ENV) so a subsequently-spawned 'nself build' subprocess's
// config.Load resolves the SAME cascade tier this function just loaded,
// instead of silently falling back to config.Load's "dev" default.
func TestLoadDeployEnvCascade_SetsENVForConfigLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ENV", "")
	t.Setenv("NSELF_DEPLOY_ENV", "")

	loadDeployEnvCascade(dir, "staging")

	if got := os.Getenv("ENV"); got != "staging" {
		t.Errorf("ENV: got %q, want %q (config.Load keys its cascade selection on this variable)", got, "staging")
	}
	if got := os.Getenv("NSELF_DEPLOY_ENV"); got != "staging" {
		t.Errorf("NSELF_DEPLOY_ENV: got %q, want %q", got, "staging")
	}
}
