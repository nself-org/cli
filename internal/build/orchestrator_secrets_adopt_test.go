package build

// orchestrator_secrets_adopt_test.go — D-0506: adopt the live credential from
// the previous build, warn on shadowing, and the plan-mode path. Fake values.

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func genCfg() *config.Config {
	return &config.Config{
		Postgres: config.PostgresConfig{Password: "fake-generated-pg-password"},
		Hasura:   config.HasuraConfig{AdminSecret: "fake-generated-admin-secret", JWTKey: "fake-jwt-key-for-test", JWTType: "HS256"},
	}
}

func projectWithComposeEnv(t *testing.T, composeEnv string) string {
	t.Helper()
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".nself", "compose.env"), composeEnv, 0600)
	return dir
}

func TestPersistGeneratedSecrets_AdoptsLiveCredentials(t *testing.T) {
	dir := projectWithComposeEnv(t, "# GENERATED\nPOSTGRES_PASSWORD=fake-live-pg-password\nHASURA_GRAPHQL_ADMIN_SECRET='fake-live$admin-secret'\n")
	cfg := genCfg()
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "fake-live-pg-password" || cfg.Hasura.AdminSecret != "fake-live$admin-secret" {
		t.Fatal("cfg did not adopt the live credentials")
	}
	data := readSecrets(t, dir)
	if !strings.Contains(data, "POSTGRES_PASSWORD=fake-live-pg-password\n") || !strings.Contains(data, "HASURA_GRAPHQL_ADMIN_SECRET=fake-live$admin-secret\n") {
		t.Errorf(".env.secrets lacks the adopted values:\n%s", data)
	}
	if strings.Contains(data, "fake-generated") {
		t.Error("generated value was persisted although a live one exists")
	}
}

func TestPersistGeneratedSecrets_ComposeEnvReferenceIgnored(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=${POSTGRES_PASSWORD}\nHASURA_GRAPHQL_ADMIN_SECRET=\n")
	cfg := genCfg()
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "fake-generated-pg-password" || cfg.Hasura.AdminSecret != "fake-generated-admin-secret" {
		t.Fatal("a reference or empty value must not be adopted")
	}
	data := readSecrets(t, dir)
	if !strings.Contains(data, "POSTGRES_PASSWORD=fake-generated-pg-password\n") {
		t.Errorf("generated value not persisted:\n%s", data)
	}
}

func TestPersistGeneratedSecrets_AdoptNotWhenProcessEnvOwnsKey(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=fake-live-pg-password\n")
	t.Setenv("POSTGRES_PASSWORD", "fake-ci-exported-password")
	cfg := genCfg()
	cfg.Postgres.Password = "fake-ci-exported-password"
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Postgres.Password != "fake-ci-exported-password" || strings.Contains(readSecrets(t, dir), "POSTGRES_PASSWORD=") {
		t.Error("process-env-owned value was replaced or persisted")
	}
}

// captureWarnings runs persist on a project with the given env files and
// returns what slog wrote.
func captureWarnings(t *testing.T, env string, files map[string]string) string {
	t.Helper()
	clearSecretEnv(t)
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	dir := t.TempDir()
	for name, body := range files {
		testWriteFile(t, filepath.Join(dir, name), body, 0600)
	}
	cfg := genCfg()
	cfg.Env = env
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatal(err)
	}
	return logs.String()
}

const shadowMsg = "wins because it loads last"

func TestPersistGeneratedSecrets_WarnsWhenSecretsFileShadowsEarlierFile(t *testing.T) {
	for _, earlier := range []string{".env.prod", ".env"} { // both load before .env.secrets (canonical)
		out := captureWarnings(t, "prod", map[string]string{
			earlier:        "POSTGRES_PASSWORD=fake-earlier-password\nCRON_INTERNAL_SECRET=same-value\n",
			".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\nCRON_INTERNAL_SECRET=same-value\n",
		})
		if n := strings.Count(out, shadowMsg); n != 1 {
			t.Fatalf("%s: got %d shadow warnings, want 1 (equal values must not warn):\n%s", earlier, n, out)
		}
		if !strings.Contains(out, "POSTGRES_PASSWORD") || !strings.Contains(out, earlier) {
			t.Errorf("%s: warning must name the key and file:\n%s", earlier, out)
		}
		if strings.Contains(out, "fake-earlier-password") || strings.Contains(out, "fake-secrets-password") {
			t.Error("warning leaked a value")
		}
	}
}

// .env.local loads AFTER .env.secrets, so it wins: nothing is shadowed.
func TestPersistGeneratedSecrets_NoWarningForFileLoadedAfterSecrets(t *testing.T) {
	out := captureWarnings(t, "dev", map[string]string{
		".env.local":   "POSTGRES_PASSWORD=fake-local-password\n",
		".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\n",
	})
	if strings.Contains(out, shadowMsg) {
		t.Errorf(".env.local beats .env.secrets; no warning expected:\n%s", out)
	}
}

// A file outside the active cascade (.env.prod while ENV=dev) never loads.
func TestPersistGeneratedSecrets_NoWarningForInactiveEnvFile(t *testing.T) {
	out := captureWarnings(t, "dev", map[string]string{
		".env.prod":    "POSTGRES_PASSWORD=fake-prod-password\n",
		".env.secrets": "POSTGRES_PASSWORD=fake-secrets-password\n",
	})
	if strings.Contains(out, shadowMsg) {
		t.Errorf("inactive .env.prod is not loaded; no warning expected:\n%s", out)
	}
}

// Plan mode: the planned .env.secrets carries both keys (and the adopted
// value), and nothing is written to disk.
func TestPersistGeneratedSecrets_PlanModeWritesNothingToDisk(t *testing.T) {
	dir := projectWithComposeEnv(t, "POSTGRES_PASSWORD=fake-live-pg-password\n")
	sink := newMemSink(dir, "")
	fx := &planEffects{}
	cfg := genCfg()
	if err := persistGeneratedSecretsFx(dir, cfg, fx, sink); err != nil {
		t.Fatal(err)
	}
	planned, err := sink.ReadFile(filepath.Join(dir, ".env.secrets"))
	if err != nil {
		t.Fatalf("planned .env.secrets: %v", err)
	}
	for _, want := range []string{"POSTGRES_PASSWORD=fake-live-pg-password\n", "HASURA_GRAPHQL_ADMIN_SECRET=fake-generated-admin-secret\n"} {
		if !strings.Contains(string(planned), want) {
			t.Errorf("planned bytes lack %q:\n%s", want, planned)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.secrets")); !os.IsNotExist(err) {
		t.Errorf(".env.secrets reached disk in plan mode (stat err = %v)", err)
	}
}
