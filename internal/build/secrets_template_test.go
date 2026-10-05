package build

// Purpose: Repro + regression tests for PCI compose-secret-templating
//          (2026-07-03): the generated docker-compose.yml must contain NO
//          literal secret values — only ${VAR} references resolved from
//          .nself/compose.env at container-start time.

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// secretTestConfig returns a config with distinctive secret values so literal
// leaks are unambiguous in assertions.
func secretTestConfig() *config.Config {
	cfg := &config.Config{
		ProjectName:   "sectest",
		BaseDomain:    "localhost",
		Env:           "dev",
		DockerNetwork: "sectest_network",
		Postgres: config.PostgresConfig{
			Host:     "postgres",
			User:     "postgres",
			Password: "pg-secret-Zx9Qw8Er7T",
			DB:       "nself",
			Port:     5432,
			Version:  "16",
		},
		Hasura: config.HasuraConfig{
			AdminSecret: "hasura-admin-K3v9Bn2Mp5",
			JWTKey:      "jwt-key-A1b2C3d4E5f6G7h8I9j0K1l2M3n4O5p6",
			JWTType:     "HS256",
			Port:        8080,
			Version:     "v2.36.0",
			MemLimit:    "1g",
			CPULimit:    "1.0",
		},
		Auth: config.AuthConfig{
			Version:            "0.36.0",
			Port:               4000,
			ClientURL:          "http://localhost:3000",
			AccessTokenExpiry:  900,
			RefreshTokenExpiry: 2592000,
			SMTPHost:           "mailpit",
			SMTPPort:           1025,
			SMTPPass:           "smtp-pass-Qw4Rt6Yu8I",
			SMTPSender:         "noreply@localhost",
			MemLimit:           "256m",
			CPULimit:           "0.25",
		},
	}
	cfg.Minio.Enabled = true
	cfg.Minio.Version = "latest"
	cfg.Minio.Port = 9000
	cfg.Minio.ConsolePort = 9001
	cfg.Minio.RootUser = "minio-user-F7g8H9j0"
	cfg.Minio.RootPassword = "minio-pass-L5m6N7b8V9"
	cfg.Minio.MemLimit = "1G"
	cfg.Minio.CPULimit = "0.5"
	return cfg
}

// TestTemplateSecrets_NoLiteralSecretsInGeneratedCompose is the PCI repro:
// run the real compose generator, apply secret templating, and assert the
// output contains only ${VAR} references — zero literal secret values.
func TestTemplateSecrets_NoLiteralSecretsInGeneratedCompose(t *testing.T) {
	cfg := secretTestConfig()
	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("compose generation failed: %v", err)
	}

	secrets := SecretEnvMap(cfg)
	templated := TemplateSecrets(raw, secrets)
	doc := string(templated)

	// No secret literal may survive.
	for name, val := range secrets {
		if strings.Contains(doc, val) {
			t.Errorf("literal secret %s (%q) still present in generated compose", name, val)
		}
	}
	if leaks := LiteralSecretLeaks(templated, secrets); len(leaks) != 0 {
		t.Errorf("LiteralSecretLeaks reported %v, want none", leaks)
	}

	// The ${VAR} references must be present instead.
	for _, ref := range []string{
		"POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}",
		"HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}",
		"HASURA_GRAPHQL_JWT_SECRET: ${HASURA_GRAPHQL_JWT_SECRET}",
		"MINIO_ROOT_USER: ${MINIO_ROOT_USER}",
		"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}",
		"AUTH_JWT_SECRET: ${HASURA_JWT_KEY}",
		":${POSTGRES_PASSWORD}@", // DATABASE_URL credential position
	} {
		if !strings.Contains(doc, ref) {
			t.Errorf("expected templated reference %q in generated compose", ref)
		}
	}
}

// TestTemplateSecrets_CommandAndHealthcheckLiteralsRewritten is the P6-E2-
// W2-S3-T9 finding: Redis embeds REDIS_PASSWORD in its `command:` string
// (--requirepass) and healthcheck `test:` args, and Typesense embeds
// TYPESENSE_API_KEY in its `command:` string (--api-key=) and healthcheck
// header — none of these are "KEY: value" env lines or URL credential
// positions, so Phases A/B never touched them and a real `nself build`
// leaked both literally despite REDIS_PASSWORD being in SecretEnvMap and
// LiteralSecretLeaks correctly warning. Phase C must catch both.
func TestTemplateSecrets_CommandAndHealthcheckLiteralsRewritten(t *testing.T) {
	cfg := secretTestConfig()
	cfg.Redis.Enabled = true
	cfg.Redis.Password = "redis-pass-K9m3Bq7Wz2"
	cfg.Search.Enabled = true
	cfg.Search.Engine = "typesense"
	cfg.Search.Typesense.APIKey = "typesense-key-Vb4Xr8Nq1L"

	raw, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("compose generation failed: %v", err)
	}

	secrets := SecretEnvMap(cfg)
	if _, ok := secrets["TYPESENSE_API_KEY"]; !ok {
		t.Fatal("SecretEnvMap must cover TYPESENSE_API_KEY")
	}

	templated := TemplateSecrets(raw, secrets)
	doc := string(templated)

	for _, literal := range []string{cfg.Redis.Password, cfg.Search.Typesense.APIKey} {
		if strings.Contains(doc, literal) {
			t.Errorf("literal secret %q still present in generated compose (command/healthcheck leak):\n%s", literal, doc)
		}
	}
	if leaks := LiteralSecretLeaks(templated, secrets); len(leaks) != 0 {
		t.Errorf("LiteralSecretLeaks reported %v, want none", leaks)
	}
	for _, ref := range []string{
		"--requirepass ${REDIS_PASSWORD}",
		"--api-key=${TYPESENSE_API_KEY}",
	} {
		if !strings.Contains(doc, ref) {
			t.Errorf("expected templated reference %q in generated compose:\n%s", ref, doc)
		}
	}
}

// TestTemplateSecrets_AliasKeyLeftAloneWhenValueDiffers ensures alias env keys
// carrying an independent value are never clobbered.
func TestTemplateSecrets_AliasKeyLeftAloneWhenValueDiffers(t *testing.T) {
	yaml := "services:\n  auth:\n    environment:\n      AUTH_JWT_SECRET: some-other-independent-value\n"
	secrets := map[string]string{"HASURA_JWT_KEY": "jwt-key-A1b2C3d4E5f6"}
	out := string(TemplateSecrets([]byte(yaml), secrets))
	if !strings.Contains(out, "AUTH_JWT_SECRET: some-other-independent-value") {
		t.Errorf("alias key with independent value was clobbered:\n%s", out)
	}
}

// TestTemplateSecrets_URLEscapedPasswordLeftLiteral: passwords that require
// percent-encoding must not be substituted into URL credential positions
// (compose interpolation would inject the raw, unescaped value).
func TestTemplateSecrets_URLEscapedPasswordLeftLiteral(t *testing.T) {
	pw := "has space%pw"
	yaml := "      DATABASE_URL: postgresql://postgres:has%20space%25pw@postgres:5432/db\n"
	secrets := map[string]string{"POSTGRES_PASSWORD": pw}
	out := string(TemplateSecrets([]byte(yaml), secrets))
	if strings.Contains(out, ":${POSTGRES_PASSWORD}@") {
		t.Errorf("URL-escaped password must not be substituted, got:\n%s", out)
	}
}

func TestWriteComposeEnv_ContentAndPermissions(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	secrets := SecretEnvMap(cfg)
	plugins := map[string]string{"NSELF_PLUGIN_DIR": "/tmp/plugins"}

	if err := WriteComposeEnv(workdir, cfg, secrets, plugins); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(workdir, ".nself", "compose.env")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows os.Chmod cannot represent POSIX owner-only bits (NTFS ACLs, not
	// mode bits) — same exception as internal/build/hasura_config_test.go.
	if perm := info.Mode().Perm(); runtime.GOOS != "windows" && perm != 0o600 {
		t.Errorf("compose.env permissions = %o, want 0600", perm)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"POSTGRES_PASSWORD=" + cfg.Postgres.Password,
		"HASURA_GRAPHQL_ADMIN_SECRET=" + cfg.Hasura.AdminSecret,
		"DOCKER_NETWORK=sectest_network",
		"NSELF_PLUGIN_DIR=/tmp/plugins",
		"HASURA_GRAPHQL_ENABLE_CONSOLE=false",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("compose.env missing %q", want)
		}
	}
}

func TestComposeEnvFiles_OrderAndFallback(t *testing.T) {
	// Legacy project: no compose.env → nil (docker compose default discovery).
	legacy := t.TempDir()
	if err := os.WriteFile(filepath.Join(legacy, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := ComposeEnvFiles(legacy); got != nil {
		t.Errorf("legacy project: expected nil env files, got %v", got)
	}

	// Templated project: .env + .nself/compose.env, compose.env last (wins).
	templated := t.TempDir()
	if err := os.WriteFile(filepath.Join(templated, ".env"), []byte("A=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(templated, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(templated, ".nself", "compose.env"), []byte("B=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := ComposeEnvFiles(templated)
	if len(got) != 2 {
		t.Fatalf("expected 2 env files, got %v", got)
	}
	if filepath.Base(got[0]) != ".env" || filepath.Base(got[1]) != "compose.env" {
		t.Errorf("env file order wrong: %v", got)
	}
}

// urlEncPasswords are test-only values covering every URL-reserved character.
// Assertions never print them: only the case index appears in failures.
var urlEncPasswords = []string{
	"a/b@c:d#e?f%g",
	"p@ss",
	"abc",
	"a+b c",
	"pa$$word$HOME",
	"@:/?#%+ ",
	"100%25",
}

// readComposeEnvMap parses .nself/compose.env the way docker compose reads a
// dotenv file for the values this test cares about: KEY=VALUE per line, one
// layer of single quotes removed.
func readComposeEnvMap(t *testing.T, workdir string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(workdir, ".nself", "compose.env"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed compose.env line (key %q)", k)
		}
		out[k] = config.UnquoteEnvValue(v)
	}
	return out
}

// TestWriteComposeEnv_URLEncVars: compose.env carries both encoded twins; the
// parsed twin decodes (url.Parse) back to the original; the raw variables and
// DATABASE_URL are untouched and not double-encoded.
func TestWriteComposeEnv_URLEncVars(t *testing.T) {
	for i, pw := range urlEncPasswords {
		workdir := t.TempDir()
		cfg := secretTestConfig()
		cfg.Postgres.Password = pw
		cfg.Redis.Enabled = true
		cfg.Redis.Password = pw + "r"
		if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		env := readComposeEnvMap(t, workdir)

		for _, c := range []struct{ key, raw string }{
			{"POSTGRES_PASSWORD", pw},
			{"REDIS_PASSWORD", pw + "r"},
		} {
			if env[c.key] != c.raw {
				t.Errorf("case %d: %s changed", i, c.key)
			}
			enc, ok := env[c.key+"_URLENC"]
			if !ok {
				t.Fatalf("case %d: %s_URLENC missing", i, c.key)
			}
			if enc != config.URLPassword(c.raw) {
				t.Errorf("case %d: %s_URLENC is not the single-encoded password", i, c.key)
			}
			u, err := url.Parse("redis://:" + enc + "@redis:6379")
			if err != nil {
				t.Fatalf("case %d: %s_URLENC does not parse in a URL: %v", i, c.key, err)
			}
			if got, _ := u.User.Password(); got != c.raw || u.Host != "redis:6379" {
				t.Errorf("case %d: %s_URLENC does not round-trip", i, c.key)
			}
		}
		// DATABASE_URL was already encoded once by cfg.DatabaseURL(); the twin
		// must not alter or re-encode it.
		du, err := url.Parse(env["DATABASE_URL"])
		if err != nil {
			t.Fatalf("case %d: DATABASE_URL: %v", i, err)
		}
		if got, _ := du.User.Password(); got != pw {
			t.Errorf("case %d: DATABASE_URL password no longer round-trips", i)
		}
	}
}

func TestWriteComposeEnv_URLEncOmittedWhenPasswordEmpty(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	cfg.Redis.Password = ""
	if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatal(err)
	}
	env := readComposeEnvMap(t, workdir)
	if _, ok := env["REDIS_PASSWORD_URLENC"]; ok {
		t.Error("REDIS_PASSWORD_URLENC written for an empty password")
	}
	if _, ok := env["POSTGRES_PASSWORD_URLENC"]; !ok {
		t.Error("POSTGRES_PASSWORD_URLENC missing")
	}
}

// A twin that contains "$" must be single-quoted or compose expands "$name"
// inside the dotenv value and silently truncates the password.
func TestWriteComposeEnv_URLEncDollarIsQuoted(t *testing.T) {
	workdir := t.TempDir()
	cfg := secretTestConfig()
	cfg.Postgres.Password = "pa$$word$HOME"
	if err := WriteComposeEnv(workdir, cfg, SecretEnvMap(cfg), nil); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(workdir, ".nself", "compose.env"))
	var line string
	for _, l := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(l, "POSTGRES_PASSWORD_URLENC=") {
			line = strings.TrimPrefix(l, "POSTGRES_PASSWORD_URLENC=")
		}
	}
	if len(line) < 2 || line[0] != '\'' || line[len(line)-1] != '\'' {
		t.Fatal("a twin containing $ must be single-quoted in compose.env")
	}
	if strings.Contains(line, "'") && strings.Count(line, "'") != 2 {
		t.Fatal("encoded value must not contain a single quote")
	}
}
