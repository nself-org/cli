package compose_test

import (
	"regexp"
	"testing"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// secretRefPattern matches the ${NAME} and ${NAME:-} references secretRef emits.
var secretRefPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)(:-)?\}`)

// optionalSecretsConfig enables every optional service with every secret set.
func optionalSecretsConfig(engine string) *config.Config {
	return &config.Config{
		ProjectName:   "test",
		BaseDomain:    "localhost",
		Env:           "staging",
		DockerNetwork: "test_network",
		Postgres:      config.PostgresConfig{Host: "postgres", User: "postgres", Password: "pgSentinel0123456789", DB: "nself", Port: 5432},
		Hasura: config.HasuraConfig{
			AdminSecret: "hasuraSentinel0123456789", JWTKey: "testsecretkey12345678901234567890",
			JWTType: "HS256", Port: 8080, Version: "v2.36.0", MemLimit: "1g", CPULimit: "1.0",
		},
		Auth:      config.AuthConfig{Version: "0.36.0", Port: 4000, ClientURL: "http://localhost:3000", MemLimit: "256m", CPULimit: "0.25"},
		Redis:     config.RedisConfig{Enabled: true, Password: "redisSentinel0123456789"},
		Minio:     config.MinioConfig{Enabled: true, RootUser: "minio", RootPassword: "minioSentinel0123456789"},
		Mailpit:   config.MailpitConfig{Enabled: true, UIUser: "admin", UIPassword: "mailpitSentinel0123456789"},
		Admin:     config.AdminConfig{Enabled: true, SecretKey: "adminKeySentinel0123456789", PasswordHash: "adminHashSentinel0123456789"},
		Functions: config.FunctionsConfig{Enabled: true},
		Search: config.SearchConfig{
			Enabled:     true,
			Engine:      engine,
			MeiliSearch: config.MeiliSearchConfig{MasterKey: "meiliSentinel0123456789"},
			Typesense:   config.TypesenseConfig{APIKey: "typesenseSentinel0123456789"},
		},
	}
}

// TestSecretRefNamesAreComposeEnvKeys: every ${NAME} an optional service
// emits must be a build.SecretEnvMap key, or .nself/compose.env never carries
// the value and the container starts with an empty secret.
func TestSecretRefNamesAreComposeEnvKeys(t *testing.T) {
	for _, engine := range []string{"meilisearch", "typesense"} {
		cfg := optionalSecretsConfig(engine)
		out, err := compose.NewGenerator(cfg).Generate()
		if err != nil {
			t.Fatalf("%s: Generate() error: %v", engine, err)
		}
		secrets := build.SecretEnvMap(cfg)
		seen := 0
		for _, m := range secretRefPattern.FindAllStringSubmatch(string(out), -1) {
			if _, ok := secrets[m[1]]; ok {
				seen++
				continue
			}
			// Core services carry non-secret interpolations of their own.
			if m[1] == "HASURA_GRAPHQL_ENABLE_CONSOLE" || m[1] == "HASURA_GRAPHQL_DEV_MODE" {
				continue
			}
			t.Errorf("%s: ${%s} is not a build.SecretEnvMap key; compose.env will not resolve it", engine, m[1])
		}
		if seen == 0 {
			t.Fatalf("%s: no secret references found; the generator no longer emits them", engine)
		}
	}
}

// TestOptionalSecretRefsMatchTemplatedOutput: with every secret set, the
// references the generator now emits are exactly what build.TemplateSecrets
// produced from the old literals, so templating is a no-op and nself build
// writes the same bytes as before D-0266 for a project with its secrets set.
func TestOptionalSecretRefsMatchTemplatedOutput(t *testing.T) {
	for _, engine := range []string{"meilisearch", "typesense"} {
		cfg := optionalSecretsConfig(engine)
		out, err := compose.NewGenerator(cfg).Generate()
		if err != nil {
			t.Fatalf("%s: Generate() error: %v", engine, err)
		}
		secrets := build.SecretEnvMap(cfg)
		templated := string(build.TemplateSecrets(out, secrets))

		// Rebuild the pre-D-0266 literal form, then template it the way
		// nself build does. It must land on the same bytes.
		legacy := secretRefPattern.ReplaceAllStringFunc(string(out), func(ref string) string {
			m := secretRefPattern.FindStringSubmatch(ref)
			if v, ok := secrets[m[1]]; ok && m[2] == "" {
				return v
			}
			return ref
		})
		if legacy == string(out) {
			t.Fatalf("%s: no references substituted; the comparison proves nothing", engine)
		}
		if got := string(build.TemplateSecrets([]byte(legacy), secrets)); got != templated {
			t.Errorf("%s: templated literal form differs from templated reference form\nliteral form:\n%s\nreference form:\n%s", engine, got, templated)
		}
		if leaks := build.LiteralSecretLeaks([]byte(templated), secrets); len(leaks) != 0 {
			t.Errorf("%s: literal secrets after templating: %v", engine, leaks)
		}
	}
}
