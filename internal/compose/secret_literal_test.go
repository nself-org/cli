package compose

import (
	"github.com/nself-org/cli/internal/compat/compattest"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/config"
)

// coreServiceNames are the services built by core_services*.go. Their
// secrets are templated by build.TemplateSecrets; every other service in the
// generated compose is an optional service and must emit references itself.
var coreServiceNames = map[string]bool{
	"postgres": true,
	"hasura":   true,
	"auth":     true,
	"nginx":    true,
}

// secretShapedLine mirrors the nchat Hygiene gate for tracked generated
// compose files: a line naming a credential must carry a ${...} reference.
var secretShapedLine = regexp.MustCompile(`(PASSWORD|SECRET|KEY|TOKEN)[[:space:]]*[:=]`)

// optionalSecretSentinels are distinct, URL-safe values for every secret an
// optional service can carry. None may appear in generated output.
var optionalSecretSentinels = map[string]string{
	"MEILISEARCH_MASTER_KEY":      "meiliSentinel0123456789",
	"TYPESENSE_API_KEY":           "typesenseSentinel0123456789",
	"REDIS_PASSWORD":              "redisSentinel0123456789",
	"MINIO_ROOT_PASSWORD":         "minioSentinel0123456789",
	"MAILPIT_UI_PASSWORD":         "mailpitSentinel0123456789",
	"ADMIN_SECRET_KEY":            "adminKeySentinel0123456789",
	"ADMIN_PASSWORD_HASH":         "adminHashSentinel0123456789",
	"HASURA_GRAPHQL_ADMIN_SECRET": "hasuraSentinel0123456789",
}

// allOptionalServicesConfig enables every optional service the generator
// knows. engine picks the search provider (one per compose). With set=true
// every optional-service secret carries its sentinel; with set=false every
// one is empty, the case that used to emit `KEY: ""` (D-0266).
func allOptionalServicesConfig(engine string, set bool) *config.Config {
	cfg := minimalConfig()
	cfg.Env = "staging" // mailpit adds UI basic auth outside dev
	val := func(name string) string {
		if !set {
			return ""
		}
		return optionalSecretSentinels[name]
	}
	cfg.Hasura.AdminSecret = val("HASURA_GRAPHQL_ADMIN_SECRET")
	cfg.Redis = config.RedisConfig{Enabled: true, Password: val("REDIS_PASSWORD")}
	cfg.Minio = config.MinioConfig{Enabled: true, RootUser: "minio", RootPassword: val("MINIO_ROOT_PASSWORD")}
	cfg.Mailpit = config.MailpitConfig{Enabled: true, UIUser: "admin", UIPassword: val("MAILPIT_UI_PASSWORD")}
	cfg.Admin = config.AdminConfig{Enabled: true, SecretKey: val("ADMIN_SECRET_KEY"), PasswordHash: val("ADMIN_PASSWORD_HASH")}
	cfg.Functions = config.FunctionsConfig{Enabled: true}
	cfg.Search = config.SearchConfig{
		Enabled:     true,
		Engine:      engine,
		MeiliSearch: config.MeiliSearchConfig{MasterKey: val("MEILISEARCH_MASTER_KEY")},
		Typesense:   config.TypesenseConfig{APIKey: val("TYPESENSE_API_KEY")},
	}
	return cfg
}

// optionalServicesYAML generates the full compose for cfg and returns each
// non-core service re-marshalled on its own, keyed by service name.
func optionalServicesYAML(t *testing.T, cfg *config.Config) map[string]string {
	t.Helper()
	out, err := NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	var doc struct {
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal generated compose: %v", err)
	}
	got := make(map[string]string)
	for name, node := range doc.Services {
		if coreServiceNames[name] {
			continue
		}
		b, err := yaml.Marshal(&node)
		if err != nil {
			t.Fatalf("marshal service %s: %v", name, err)
		}
		got[name] = string(b)
	}
	return got
}

func TestSecretRef(t *testing.T) {
	cases := []struct {
		value, want string
	}{
		{"s3cret-value", "${MEILISEARCH_MASTER_KEY}"},
		{"", "${MEILISEARCH_MASTER_KEY:-}"},
		{"   ", "${MEILISEARCH_MASTER_KEY:-}"},
		{"ab12", "${MEILISEARCH_MASTER_KEY}"},
		{"a/b@c:d?e%f", "${MEILISEARCH_MASTER_KEY}"},
		// compose.env quotes these, so they are references too (D-0333).
		{"ab #cd", "${MEILISEARCH_MASTER_KEY}"},
		{"ab$cd", "${MEILISEARCH_MASTER_KEY}"},
		{"ab'cd", "${MEILISEARCH_MASTER_KEY}"},
		{`ab\`, "${MEILISEARCH_MASTER_KEY}"},
	}
	for _, c := range cases {
		if got := secretRef("MEILISEARCH_MASTER_KEY", c.value); got != c.want {
			t.Errorf("secretRef(%q) = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestSecretURLUserInfo(t *testing.T) {
	cases := []struct {
		user, password, want string
	}{
		{"postgres", "p/w@x", "postgres:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}"},
		{"", "pw", ":${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}"},
		{"postgres", "", "postgres"},
	}
	for _, c := range cases {
		if got := secretURLUserInfo(c.user, c.password, "POSTGRES_PASSWORD"); got != c.want {
			t.Errorf("secretURLUserInfo(%q, %q) = %q, want %q", c.user, c.password, got, c.want)
		}
	}
}

// TestOptionalServicesSecretLiterals is the D-0266 guard: no optional service
// writes a secret value, or an empty literal, into the generated compose.
func TestOptionalServicesSecretLiterals(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	for _, engine := range []string{"meilisearch", "typesense"} {
		for _, set := range []bool{true, false} {
			name := engine + "/unset"
			if set {
				name = engine + "/set"
			}
			t.Run(name, func(t *testing.T) {
				services := optionalServicesYAML(t, allOptionalServicesConfig(engine, set))
				for _, want := range []string{"redis", "minio", "mailpit", AdminServiceName, "functions", engine} {
					if _, ok := services[want]; !ok {
						t.Fatalf("service %q missing from generated compose; the guard would not cover it", want)
					}
				}
				for svc, text := range services {
					for _, line := range strings.Split(text, "\n") {
						if secretShapedLine.MatchString(line) && !strings.Contains(line, "${") {
							t.Errorf("%s: secret-shaped line carries a literal: %q", svc, strings.TrimSpace(line))
						}
					}
					for key, sentinel := range optionalSecretSentinels {
						if strings.Contains(text, sentinel) {
							t.Errorf("%s: literal value of %s in generated compose", svc, key)
						}
					}
				}
			})
		}
	}
}

// TestOptionalServicesSecretRefsPerService pins the reference each secret
// position emits, so a refactor cannot silently drop one back to a literal.
func TestOptionalServicesSecretRefsPerService(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	cases := []struct {
		engine, service string
		set             bool
		want            []string
	}{
		{"meilisearch", "meilisearch", true, []string{"MEILI_MASTER_KEY: ${MEILISEARCH_MASTER_KEY}"}},
		{"meilisearch", "meilisearch", false, []string{"MEILI_MASTER_KEY: ${MEILISEARCH_MASTER_KEY:-}"}},
		// The key reaches Typesense only through its environment; command and
		// healthcheck no longer carry it (D-0282).
		{"typesense", "typesense", true, []string{
			"TYPESENSE_API_KEY: ${TYPESENSE_API_KEY}",
			"command: --data-dir /data --enable-cors",
			"- http://localhost:8108/health",
		}},
		// Redis gets the password as REDISCLI_AUTH: redis-cli reads it itself
		// and the shell expands $$REDISCLI_AUTH for redis-server (D-0282).
		{"meilisearch", "redis", true, []string{
			"REDISCLI_AUTH: ${REDIS_PASSWORD}",
			`/run/nself-cache/redis.conf`,
		}},
		{"meilisearch", "minio", true, []string{"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD}"}},
		{"meilisearch", "minio", false, []string{"MINIO_ROOT_PASSWORD: ${MINIO_ROOT_PASSWORD:-}"}},
		{"meilisearch", "mailpit", true, []string{"MP_UI_BASICAUTH: admin:${MAILPIT_UI_PASSWORD}"}},
		{"meilisearch", AdminServiceName, true, []string{
			"HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}",
			"ADMIN_SECRET_KEY: ${ADMIN_SECRET_KEY}",
			"ADMIN_PASSWORD_HASH: ${ADMIN_PASSWORD_HASH}",
		}},
		{"meilisearch", AdminServiceName, false, []string{
			"ADMIN_SECRET_KEY: ${ADMIN_SECRET_KEY:-}",
			"ADMIN_PASSWORD_HASH: ${ADMIN_PASSWORD_HASH:-}",
		}},
		{"meilisearch", "functions", true, []string{
			"HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}",
			"DATABASE_URL: postgres://postgres:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/nself",
		}},
		{"meilisearch", AdminServiceName, true, []string{
			"DATABASE_URL: postgres://postgres:${POSTGRES_PASSWORD_URLENC:-${POSTGRES_PASSWORD}}@postgres:5432/nself",
		}},
	}
	for _, c := range cases {
		text := optionalServicesYAML(t, allOptionalServicesConfig(c.engine, c.set))[c.service]
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s (set=%v): want %q in\n%s", c.service, c.set, w, text)
			}
		}
	}
}
