package build

// orchestrator_secrets_stability_test.go — D-0506: generated Postgres/Hasura
// credentials must survive a rebuild. All values are fake test fixtures.

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// clearSecretEnv blanks every key the loader reads that could leak in from the
// caller's environment (t.Setenv restores them when the test ends).
func clearSecretEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_GRAPHQL_JWT_SECRET",
		"HASURA_JWT_KEY", "HASURA_JWT_TYPE", "AUTH_JWT_SECRET", "AUTH_JWT_KEY", "AUTH_JWT_TYPE",
		"PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET", "CRON_INTERNAL_SECRET",
		"HASURA_ADMIN_SECRET", "ENV",
	} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

// buildOnce mimics one build: load the config, persist generated secrets, and
// restore the process environment the way a separate command would find it.
func buildOnce(t *testing.T, dir string) *config.Config {
	t.Helper()
	restore := SnapshotEnv()
	defer restore()
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}
	return cfg
}

func readSecrets(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".env.secrets"))
	if err != nil {
		t.Fatalf("read .env.secrets: %v", err)
	}
	return string(data)
}

func TestPersistGeneratedSecrets_CredentialsStableAcrossRebuild(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)

	first := buildOnce(t, dir)
	second := buildOnce(t, dir)

	if first.Postgres.Password == "" || first.Hasura.AdminSecret == "" || first.Hasura.JWTKey == "" {
		t.Fatal("build 1 did not produce all three credentials")
	}
	if first.Postgres.Password != second.Postgres.Password {
		t.Error("POSTGRES_PASSWORD changed between builds")
	}
	if first.Hasura.AdminSecret != second.Hasura.AdminSecret {
		t.Error("HASURA_GRAPHQL_ADMIN_SECRET changed between builds")
	}
	if first.Hasura.JWTKey != second.Hasura.JWTKey {
		t.Error("Hasura JWT key changed between builds")
	}

	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_GRAPHQL_JWT_SECRET"} {
		if n := strings.Count(data, "\n"+key+"=") + b2i(strings.HasPrefix(data, key+"=")); n != 1 {
			t.Errorf("%s appears %d times in .env.secrets, want 1", key, n)
		}
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(filepath.Join(dir, ".env.secrets"))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0600 {
			t.Errorf(".env.secrets mode = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestPersistGeneratedSecrets_ProcessEnvValueNotPersisted(t *testing.T) {
	clearSecretEnv(t)
	t.Setenv("POSTGRES_PASSWORD", "fake-ci-exported-password")
	t.Setenv("HASURA_GRAPHQL_ADMIN_SECRET", "fake-ci-exported-admin-secret")
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)

	cfg := &config.Config{
		Postgres: config.PostgresConfig{Password: "fake-ci-exported-password"},
		Hasura:   config.HasuraConfig{AdminSecret: "fake-ci-exported-admin-secret", JWTKey: "fake-jwt-key-for-test", JWTType: "HS256"},
	}
	if err := persistGeneratedSecrets(dir, cfg); err != nil {
		t.Fatalf("persistGeneratedSecrets: %v", err)
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("process-env-owned %s was written to .env.secrets", key)
		}
	}
}

func TestPersistGeneratedSecrets_KeyInEnvFileNotRepersisted(t *testing.T) {
	for _, name := range []string{".env", ".env.dev"} {
		t.Run(name, func(t *testing.T) {
			clearSecretEnv(t)
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
			testWriteFile(t, filepath.Join(dir, name), "POSTGRES_PASSWORD=fake-pg-password-for-test\nHASURA_GRAPHQL_ADMIN_SECRET=fake-admin-secret-for-test\n", 0600)

			cfg := &config.Config{
				Postgres: config.PostgresConfig{Password: "fake-pg-password-for-test"},
				Hasura:   config.HasuraConfig{AdminSecret: "fake-admin-secret-for-test", JWTKey: "fake-jwt-key-for-test", JWTType: "HS256"},
			}
			if err := persistGeneratedSecrets(dir, cfg); err != nil {
				t.Fatalf("persistGeneratedSecrets: %v", err)
			}
			data := readSecrets(t, dir)
			for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
				if strings.Contains(data, key) {
					t.Errorf("%s in %s was re-persisted to .env.secrets", key, name)
				}
			}
		})
	}
}

// credsFile is a fake env file body carrying the two core credentials.
const credsFile = "POSTGRES_PASSWORD=fake-pg-password-for-test\nHASURA_GRAPHQL_ADMIN_SECRET=fake-admin-secret-for-test\n"

// Keys in the active env's file (.env.dev, ENV=dev) load back every build, so
// they are stable and must not be copied into .env.secrets.
func TestPersistGeneratedSecrets_LoadKeysInActiveEnvFileNotPersisted(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nENV=dev\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".env.dev"), credsFile, 0600)

	first := buildOnce(t, dir)
	second := buildOnce(t, dir)
	if first.Postgres.Password != "fake-pg-password-for-test" || second.Postgres.Password != first.Postgres.Password {
		t.Error("POSTGRES_PASSWORD from .env.dev is not stable across builds")
	}
	if first.Hasura.AdminSecret != "fake-admin-secret-for-test" || second.Hasura.AdminSecret != first.Hasura.AdminSecret {
		t.Error("HASURA_GRAPHQL_ADMIN_SECRET from .env.dev is not stable across builds")
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("%s from .env.dev was copied into .env.secrets", key)
		}
	}
}

// Pins the lead ruling: a key defined only in an inactive env's file (.env.prod
// while ENV=dev) is NOT persisted to .env.secrets, which is shared by every env and
// loads after .env.<env>, so a dev-generated value written there would shadow
// prod's real value on the next prod build. D-0707 persists into .env.dev instead.
func TestPersistGeneratedSecrets_LoadKeysOnlyInInactiveEnvNotPersisted(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nENV=dev\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".env.prod"), credsFile, 0600)

	cfg := buildOnce(t, dir)
	if cfg.Postgres.Password == "fake-pg-password-for-test" {
		t.Fatal("inactive .env.prod leaked into the dev cascade")
	}
	data := readSecrets(t, dir)
	for _, key := range []string{"POSTGRES_PASSWORD=", "HASURA_GRAPHQL_ADMIN_SECRET="} {
		if strings.Contains(data, key) {
			t.Errorf("%s was persisted although .env.prod defines it; it would shadow prod", key)
		}
	}
}
