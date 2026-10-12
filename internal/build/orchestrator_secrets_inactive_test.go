package build

// orchestrator_secrets_inactive_test.go — D-0506: a credential defined only in
// an env file that is not loaded for the current ENV (.env.prod during ENV=dev).
// Fake values only.

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func inactiveEnvProject(t *testing.T) string {
	t.Helper()
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nENV=dev\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".env.prod"), credsFile, 0600)
	return dir
}

// The lens repro: .env.prod owns both keys, two ENV=dev builds. Build 1 renders
// a generated value and leaves it in compose.env; build 2 must adopt it instead
// of generating again, and nothing is written over .env.prod's keys.
func TestPersistGeneratedSecrets_InactiveEnvKeyStableAcrossDevBuilds(t *testing.T) {
	dir := inactiveEnvProject(t)
	first := buildOnce(t, dir)
	testWriteFile(t, filepath.Join(dir, ".nself", "compose.env"),
		"POSTGRES_PASSWORD="+config.QuoteComposeEnvValue(first.Postgres.Password)+"\n"+
			"HASURA_GRAPHQL_ADMIN_SECRET="+config.QuoteComposeEnvValue(first.Hasura.AdminSecret)+"\n", 0600)
	second := buildOnce(t, dir)

	if first.Postgres.Password == "fake-pg-password-for-test" {
		t.Fatal("inactive .env.prod leaked into the dev cascade")
	}
	if second.Postgres.Password != first.Postgres.Password || second.Hasura.AdminSecret != first.Hasura.AdminSecret {
		t.Error("dev credentials changed between builds; the running dev database would be locked out")
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("%s was written to .env.secrets and would shadow .env.prod", key)
		}
	}
}

// First build, nothing to adopt: keep the generated value for this render, write
// nothing, and warn by key and file name (no values).
func TestPersistGeneratedSecrets_InactiveEnvKeyFirstBuildWarnsAndWritesNothing(t *testing.T) {
	dir := inactiveEnvProject(t)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := buildOnce(t, dir)
	out := logs.String()
	for _, want := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", ".env.prod", "not loaded for this env"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning output lacks %q:\n%s", want, out)
		}
	}
	for _, secret := range []string{"fake-pg-password-for-test", "fake-admin-secret-for-test", cfg.Postgres.Password, cfg.Hasura.AdminSecret} {
		if strings.Contains(out, secret) {
			t.Error("warning leaked a value")
		}
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("%s was written to .env.secrets", key)
		}
	}
}
