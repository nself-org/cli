package build

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// D-0707: inactive prod credentials must neither prevent dev persistence nor
// be shadowed by dev's generated values when the project switches to prod.
func TestPersistGeneratedSecrets_InactiveEnvCredentials(t *testing.T) {
	for _, env := range []string{"dev", "local", "qa"} {
		t.Run(env, func(t *testing.T) {
			clearSecretEnv(t)
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
			prod := "POSTGRES_PASSWORD=fake-prod-password\n" +
				"HASURA_GRAPHQL_ADMIN_SECRET=fake-prod-admin-secret\n" +
				"PLUGIN_INTERNAL_SECRET=fake-prod-plugin-secret\n" +
				"NOTIFY_INTERNAL_SECRET=fake-prod-notify-secret\n" +
				"CRON_INTERNAL_SECRET=fake-prod-cron-secret\n" +
				"HASURA_GRAPHQL_JWT_SECRET='{\"type\":\"HS256\",\"key\":\"fake-prod-jwt-key\"}'\n"
			testWriteFile(t, filepath.Join(dir, ".env.prod"), prod, 0600)
			t.Setenv("ENV", env)
			first := buildOnce(t, dir)
			second := buildOnce(t, dir)
			if !reflect.DeepEqual(ownedSecretValues(first), ownedSecretValues(second)) {
				t.Error("generated credentials changed on the second build")
			}
			active := ".env." + env
			if env == "local" {
				active = ".env.dev"
			}
			data, err := os.ReadFile(filepath.Join(dir, active))
			if err != nil {
				t.Fatalf("read active env file: %v", err)
			}
			for _, key := range persistedSecretKeys {
				if strings.Count(string(data), key+"=") != 1 {
					t.Errorf("active env must contain exactly one %s", key)
				}
			}
			info, err := os.Stat(filepath.Join(dir, active))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatalf("active env must have mode 0600: %v", err)
			}
			t.Setenv("ENV", "prod")
			prodConfig := buildOnce(t, dir)
			if prodConfig.Postgres.Password != "fake-prod-password" || prodConfig.Hasura.JWTKey != "fake-prod-jwt-key" {
				t.Error("dev persistence shadowed prod credentials")
			}
			gotProd, err := os.ReadFile(filepath.Join(dir, ".env.prod"))
			if err != nil || string(gotProd) != prod {
				t.Fatal("prod env file was changed")
			}
			t.Setenv("ENV", env)
			if !reflect.DeepEqual(ownedSecretValues(first), ownedSecretValues(buildOnce(t, dir))) {
				t.Error("dev credentials changed after switching back from prod")
			}
		})
	}
}

func ownedSecretValues(cfg *config.Config) []string {
	return []string{cfg.Postgres.Password, cfg.Hasura.AdminSecret, cfg.Hasura.JWTKey,
		cfg.PluginSystem.InternalSecret, cfg.PluginConfig.NotifySecret, cfg.PluginConfig.CronSecret}
}

func TestPersistGeneratedSecrets_RemoteInactiveLocalOverride(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n", 0600)
	testWriteFile(t, filepath.Join(dir, ".env.local"), "POSTGRES_PASSWORD=fake-local-password\n", 0600)
	remoteBuild := func() *config.Config {
		t.Helper()
		restore := SnapshotEnv()
		defer restore()
		cfg, err := config.LoadWithOptions(dir, config.LoadOptions{RemoteDeploy: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := persistGeneratedSecretsFx(dir, cfg, writeEffects{}, newDiskSink(dir), true); err != nil {
			t.Fatal(err)
		}
		return cfg
	}
	first, second := remoteBuild(), remoteBuild()
	if first.Postgres.Password != second.Postgres.Password || first.Postgres.Password == "fake-local-password" {
		t.Fatal("remote password changed or adopted the local override")
	}
	if buildOnce(t, dir).Postgres.Password != "fake-local-password" {
		t.Fatal("remote persistence shadowed the local override")
	}
}

func TestPersistGeneratedSecrets_InactiveEnvPlanOverlay(t *testing.T) {
	clearSecretEnv(t)
	dir := t.TempDir()
	testWriteFile(t, filepath.Join(dir, ".env.prod"), credsFile, 0600)
	testWriteFile(t, filepath.Join(dir, ".env.dev"), "CUSTOM=original\n", 0644)
	sink := newEnvOverlaySink(newMemSink(dir, ""), map[string][]byte{
		".env.dev": []byte("CUSTOM=pending"), // no final newline
	})
	cfg := genCfg()
	cfg.Env = "dev"
	if err := persistGeneratedSecretsFx(dir, cfg, &planEffects{}, sink, false); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, ".env.dev")
	planned, err := sink.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	values, err := config.ParseEnvBytes(path, planned)
	if err != nil || values["CUSTOM"] != "pending" || values["POSTGRES_PASSWORD"] != cfg.Postgres.Password {
		t.Fatalf("plan lost pending env bytes or generated password: %v", err)
	}
	info, err := sink.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("planned env file must have mode 0600: %v", err)
	}
	disk, err := os.ReadFile(path)
	if err != nil || string(disk) != "CUSTOM=original\n" {
		t.Fatal("plan mutated the active env file")
	}
}

func TestPersistGeneratedSecrets_InactiveEnvConfiguredValues(t *testing.T) {
	for _, source := range []string{"process", "active-file"} {
		t.Run(source, func(t *testing.T) {
			clearSecretEnv(t)
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\nENV=dev\n", 0600)
			testWriteFile(t, filepath.Join(dir, ".env.prod"), credsFile+"HASURA_GRAPHQL_JWT_SECRET=inactive\n", 0600)
			if source == "process" {
				t.Setenv("POSTGRES_PASSWORD", "fake-configured-password")
				t.Setenv("HASURA_JWT_KEY", "fake-configured-jwt-key")
			} else {
				testWriteFile(t, filepath.Join(dir, ".env.local"), "POSTGRES_PASSWORD=fake-configured-password\nHASURA_JWT_KEY=fake-configured-jwt-key\n", 0600)
			}
			buildOnce(t, dir)
			data, err := os.ReadFile(filepath.Join(dir, ".env.dev"))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			for _, key := range []string{"POSTGRES_PASSWORD", "HASURA_GRAPHQL_JWT_SECRET"} {
				if strings.Contains(string(data), key+"=") {
					t.Errorf("configured %s was copied into the active env file", key)
				}
			}
		})
	}
}

func TestPersistGeneratedSecrets_InactiveEnvReservedNames(t *testing.T) {
	for _, env := range []string{"secrets", "ai", "example", "computed", "../escape"} {
		t.Run(env, func(t *testing.T) {
			clearSecretEnv(t)
			dir := t.TempDir()
			testWriteFile(t, filepath.Join(dir, ".env.prod"), credsFile, 0600)
			cfg := genCfg()
			cfg.Env = env
			if err := persistGeneratedSecrets(dir, cfg); err != nil {
				t.Fatal(err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.IsDir() || entry.Name() == ".env.prod" {
					continue
				}
				data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
				if err != nil || strings.Contains(string(data), "POSTGRES_PASSWORD=") {
					t.Fatalf("reserved ENV persisted a password into %s: %v", entry.Name(), err)
				}
			}
		})
	}
}
