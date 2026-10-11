package compose_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// secretRefPattern matches the ${NAME} and ${NAME:-} references secretRef emits.
var secretRefPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(:-)?\}`)

// composeRefPattern matches ${A}, ${A:-} and ${A:-${B}} for resolveRefs.
var composeRefPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(?::-(\$\{([A-Z0-9_]+)\})?)?\}`)

// coreServices are templated by internal/build, not by secretRef.
var coreServices = map[string]bool{"postgres": true, "hasura": true, "auth": true, "nginx": true}

// optionalSecretsConfig enables every optional service; sec maps a long
// URL-safe base value to the value under test (identity = "set").
func optionalSecretsConfig(engine string, sec func(string) string) *config.Config {
	return &config.Config{
		ProjectName:   "test",
		BaseDomain:    "localhost",
		Env:           "staging",
		DockerNetwork: "test_network",
		Postgres:      config.PostgresConfig{Host: "postgres", User: "postgres", Password: sec("pgSentinel0123456789"), DB: "nself", Port: 5432},
		Hasura: config.HasuraConfig{
			AdminSecret: sec("hasuraSentinel0123456789"), JWTKey: "testsecretkey12345678901234567890",
			JWTType: "HS256", Port: 8080, Version: "v2.36.0", MemLimit: "1g", CPULimit: "1.0",
		},
		Auth:      config.AuthConfig{Version: "0.36.0", Port: 4000, ClientURL: "http://localhost:3000", MemLimit: "256m", CPULimit: "0.25"},
		Redis:     config.RedisConfig{Enabled: true, Password: sec("redisSentinel0123456789")},
		Minio:     config.MinioConfig{Enabled: true, RootUser: "minio", RootPassword: sec("minioSentinel0123456789")},
		Mailpit:   config.MailpitConfig{Enabled: true, UIUser: "admin", UIPassword: sec("mailpitSentinel0123456789")},
		Admin:     config.AdminConfig{Enabled: true, SecretKey: sec("adminKeySentinel0123456789"), PasswordHash: sec("adminHashSentinel0123456789")},
		Functions: config.FunctionsConfig{Enabled: true},
		Search: config.SearchConfig{
			Enabled:     true,
			Engine:      engine,
			MeiliSearch: config.MeiliSearchConfig{MasterKey: sec("meiliSentinel0123456789")},
			Typesense:   config.TypesenseConfig{APIKey: sec("typesenseSentinel0123456789")},
		},
	}
}

func setSecrets(v string) string { return v }

// optionalText returns the templated YAML of every non-core service.
func optionalText(t *testing.T, templated []byte) string {
	t.Helper()
	var doc struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(templated, &doc); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}
	var sb strings.Builder
	for name, node := range doc.Services {
		if coreServices[name] {
			continue
		}
		b, err := yaml.Marshal(&node)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		sb.Write(b)
	}
	return sb.String()
}

// readComposeEnv writes .nself/compose.env the way nself build does and
// returns it as a map (single-quoted values unwrapped, as compose reads them).
func readComposeEnv(t *testing.T, cfg *config.Config, secrets map[string]string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	if err := build.WriteComposeEnv(dir, cfg, secrets, nil); err != nil {
		t.Fatalf("WriteComposeEnv: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".nself", "compose.env"))
	if err != nil {
		t.Fatalf("read compose.env: %v", err)
	}
	env := make(map[string]string)
	for _, line := range strings.Split(string(raw), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.HasPrefix(line, "#") {
			continue
		}
		env[k] = config.UnquoteEnvValue(v)
	}
	return env
}

// resolveRefs substitutes compose references from env, as docker compose does.
func resolveRefs(text string, env map[string]string) string {
	return composeRefPattern.ReplaceAllStringFunc(text, func(ref string) string {
		m := composeRefPattern.FindStringSubmatch(ref)
		if v, ok := env[m[1]]; ok && v != "" {
			return v
		}
		return env[m[3]] // fallback ${B}, or "" for ${A:-} / unset ${A}
	})
}

// TestSecretRefNamesAreComposeEnvKeys: every ${NAME} an optional service
// emits must be a build.SecretEnvMap key, or .nself/compose.env never carries
// the value and the container starts with an empty secret.
func TestSecretRefNamesAreComposeEnvKeys(t *testing.T) {
	for _, engine := range []string{"meilisearch", "typesense"} {
		cfg := optionalSecretsConfig(engine, setSecrets)
		out, err := compose.NewGenerator(cfg).Generate()
		if err != nil {
			t.Fatalf("%s: Generate() error: %v", engine, err)
		}
		secrets := build.SecretEnvMap(cfg)
		seen := 0
		for _, m := range secretRefPattern.FindAllStringSubmatch(optionalText(t, out), -1) {
			if _, ok := secrets[m[1]]; ok {
				seen++
				continue
			}
			t.Errorf("%s: ${%s} is not a build.SecretEnvMap key; compose.env will not resolve it", engine, m[1])
		}
		if seen == 0 {
			t.Fatalf("%s: no secret references found; the generator no longer emits them", engine)
		}
	}
}

// TestOptionalSecretsAfterTemplating runs the nself build path (Generate,
// TemplateSecrets, WriteComposeEnv) for each shape of secret and checks the
// file holds no secret, while containers resolve the values they got before.
func TestOptionalSecretsAfterTemplating(t *testing.T) {
	modes := []struct {
		name       string
		sec        func(string) string
		wantLeaked bool // dotenv-unsafe values stay literal until D-0329
	}{
		{"long", setSecrets, false},
		{"short", func(v string) string { return v[:3] + "7" }, false},
		{"url-unsafe", func(v string) string { return v[:6] + "/S@x:9?y%" }, false},
		{"dotenv-unsafe", func(v string) string { return v[:6] + " #c$d" }, true},
	}
	for _, engine := range []string{"meilisearch", "typesense"} {
		for _, mode := range modes {
			t.Run(engine+"/"+mode.name, func(t *testing.T) {
				cfg := optionalSecretsConfig(engine, mode.sec)
				out, err := compose.NewGenerator(cfg).Generate()
				if err != nil {
					t.Fatalf("Generate() error: %v", err)
				}
				secrets := build.SecretEnvMap(cfg)
				text := optionalText(t, build.TemplateSecrets(out, secrets))

				key := cfg.Search.MeiliSearch.MasterKey
				if engine == "typesense" {
					key = cfg.Search.Typesense.APIKey
				}
				values := []string{cfg.Redis.Password, cfg.Mailpit.UIPassword, key}
				for _, v := range values {
					if got := strings.Contains(text, v); got != mode.wantLeaked {
						t.Errorf("literal %q in generated compose = %v, want %v", v, got, mode.wantLeaked)
					}
				}
				if enc := config.URLPassword(cfg.Postgres.Password); strings.Contains(text, ":"+enc+"@") {
					t.Errorf("encoded postgres password left in a DATABASE_URL")
				}
				if mode.wantLeaked {
					return
				}

				resolved := resolveRefs(text, readComposeEnv(t, cfg, secrets))
				want := []string{
					"--requirepass " + cfg.Redis.Password,
					"admin:" + cfg.Mailpit.UIPassword,
					"postgres://" + config.URLUserInfo(cfg.Postgres.User, cfg.Postgres.Password) + "@postgres:5432/nself",
					key,
				}
				for _, w := range want {
					if !strings.Contains(resolved, w) {
						t.Errorf("resolved compose lacks %q", w)
					}
				}
			})
		}
	}
}
