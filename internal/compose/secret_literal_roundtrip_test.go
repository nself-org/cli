package compose_test

// secret_literal_roundtrip_test.go — D-0635: every consumer of a secret in the
// generated compose set resolves the same bytes.
//
// The nself build path is Generate, TemplateSecrets, WriteComposeEnv. docker
// compose then reads .nself/compose.env (parseComposeDotenv) and interpolates
// every string in docker-compose.yml from it (interpolateCompose, a port of
// compose-go/v2 template.Substitute for the forms compose accepts). A secret
// left literal in the YAML is interpolated too, so a "$" inside it is
// expanded (or, by the strict reading, rejected): cli#666 made postgres read a "$" password verbatim
// while the database URLs still carried the mangled literal.

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// errInvalidTemplate stands for a "$" followed by anything but "$", a name or
// a "{". compose-go's template grammar names it "invalid"; compose 5.6.0
// keeps such a "$" as written (checked with docker-compose config). The model
// takes the strict reading: no generated value may depend on either.
var errInvalidTemplate = errors.New("invalid interpolation format")

// interpolateCompose resolves s the way compose-go/v2 template.Substitute
// does: "$$" is a literal "$", "$NAME" and "${NAME}" read env (unset is ""),
// "${NAME:-d}" / "${NAME-d}" fall back to d (itself interpolated, so defaults
// nest), and any other "$" is an error.
func interpolateCompose(s string, env map[string]string) (string, error) {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			sb.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", errInvalidTemplate
		}
		switch c := s[i+1]; {
		case c == '$':
			sb.WriteByte('$')
			i++
		case c == '{':
			end := matchBrace(s, i+1)
			if end < 0 {
				return "", errInvalidTemplate
			}
			v, err := resolveBraced(s[i+2:end], env)
			if err != nil {
				return "", err
			}
			sb.WriteString(v)
			i = end
		case isNameStart(c):
			j := i + 1
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			sb.WriteString(env[s[i+1:j]])
			i = j - 1
		default:
			return "", errInvalidTemplate
		}
	}
	return sb.String(), nil
}

// matchBrace returns the index of the "}" closing the "{" at open, or -1.
func matchBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// resolveBraced resolves the inside of ${...}: NAME, NAME:-default or NAME-default.
func resolveBraced(body string, env map[string]string) (string, error) {
	j := 0
	for j < len(body) && isNameChar(body[j]) {
		j++
	}
	name, rest := body[:j], body[j:]
	if name == "" || !isNameStart(name[0]) {
		return "", errInvalidTemplate
	}
	switch {
	case rest == "":
		return env[name], nil
	case strings.HasPrefix(rest, ":-"):
		if v := env[name]; v != "" {
			return v, nil
		}
		return interpolateCompose(rest[2:], env)
	case strings.HasPrefix(rest, "-"):
		if v, ok := env[name]; ok {
			return v, nil
		}
		return interpolateCompose(rest[1:], env)
	}
	return "", errInvalidTemplate
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameChar(c byte) bool { return isNameStart(c) || (c >= '0' && c <= '9') }

// TestInterpolateComposeModelsCompose is the positive control: the
// interpolator expands "$name" the way compose does and rejects a bare "$",
// so the round trip below can fail.
func TestInterpolateComposeModelsCompose(t *testing.T) {
	env := map[string]string{"A": "a", "B": "b"}
	cases := []struct {
		in, want string
		err      bool
	}{
		{in: "x$$y", want: "x$y"},
		{in: "pw$abc#d", want: "pw#d"},
		{in: "${A}${B}", want: "ab"},
		{in: "${UNSET:-${B}}", want: "b"},
		{in: "${A:-${B}}", want: "a"},
		{in: "${UNSET-d}", want: "d"},
		{in: "pw$1", err: true},
		{in: "pw$", err: true},
		{in: "pw${", err: true},
	}
	for _, c := range cases {
		got, err := interpolateCompose(c.in, env)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("interpolateCompose(%q) = %q, %v; want %q, err=%v", c.in, got, err, c.want, c.err)
		}
	}
}

// resolvedServices interpolates every string of the templated compose YAML
// with env and returns service -> environment.
func resolvedServices(t *testing.T, templated []byte, env map[string]string) map[string]map[string]string {
	t.Helper()
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
			Healthcheck struct {
				Test any `yaml:"test"`
			} `yaml:"healthcheck"`
			Command any `yaml:"command"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(templated, &doc); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}
	out := map[string]map[string]string{}
	for name, svc := range doc.Services {
		m := map[string]string{}
		for k, v := range svc.Environment {
			r, err := interpolateCompose(v, env)
			if err != nil {
				t.Fatalf("%s %s: invalid compose template: %v", name, k, err)
			}
			m[k] = r
		}
		// Healthchecks and commands are interpolated too.
		for _, s := range flattenStrings(svc.Healthcheck.Test, svc.Command) {
			if _, err := interpolateCompose(s, env); err != nil {
				t.Fatalf("%s healthcheck/command %q: invalid compose template: %v", name, s, err)
			}
		}
		out[name] = m
	}
	return out
}

func flattenStrings(vals ...any) []string {
	var out []string
	for _, v := range vals {
		switch x := v.(type) {
		case string:
			out = append(out, x)
		case []any:
			for _, e := range x {
				out = append(out, flattenStrings(e)...)
			}
		}
	}
	return out
}

// urlPassword returns the decoded password of a connection URL.
func urlPassword(t *testing.T, where, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("%s: unparseable URL: %v", where, err)
	}
	pw, _ := u.User.Password()
	return pw
}

// d0635Passwords are synthetic, obviously fake passwords holding every
// character that compose's dotenv or interpolation treats specially.
var d0635Passwords = map[string]string{
	"dollar-name":      "fakepw$abc0001",
	"dollar-name-hash": `fakepw$abc#def ghi"jk`,
	"dollar-digit":     "fakepw$1notreal",
	"dollar-brace":     "fakepw${x}notreal",
	"double-dollar":    "fakepw$$notreal",
	"trailing-dollar":  "fakepwnotreal$",
	"single-quote":     "fake'pw'notreal",
	"double-quote":     `fake"pw"notreal`,
	"backtick":         "fake`pw`notreal",
	"hash":             "fakepw#notreal",
	"space-hash":       "fakepw #notreal",
	"spaces":           " fake pw notreal ",
	"backslash":        `fake\pw\notreal`,
	"trailing-bslash":  `fakepwnotreal\`,
	"url-reserved":     "fake@pw:/?%notreal",
	"everything":       "fa$k'e\"p`w #x\\y$1@:/%",
	"url-safe":         "fakepwnotreal0123456789",
	// A quote at the edge of a YAML-quoted scalar: a raw byte replace cut
	// through the '' escape and wrote YAML compose could not load.
	"quote-edges":  "'fakepwnotreal'",
	"quote-end":    "@fake$abcpw'",
	"tab":          "fake\tpwnotreal",
	"spells-a-ref": "PASSWORD}fakepw",
}

// TestSecretConsumersResolveIdentically runs the nself build path for each
// password shape and checks that every consumer of POSTGRES_PASSWORD and
// REDIS_PASSWORD resolves the configured bytes: the postgres server, the
// hasura/auth/custom-service database URLs, the auth AUTH_DB_* aliases,
// REDISCLI_AUTH (redis server and healthcheck) and the custom-service
// REDIS_URL, plus DATABASE_URL in compose.env for host-side CLI paths.
func TestSecretConsumersResolveIdentically(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	for name, pw := range d0635Passwords {
		t.Run(name, func(t *testing.T) {
			cfg := optionalSecretsConfig("meilisearch", setSecrets)
			cfg.Postgres.Password = pw
			cfg.Redis.Password = "redis" + pw
			cfg.Redis.Port = 6379
			cfg.Minio.RootPassword = pw + "m"
			cfg.Hasura.AdminSecret = "adm" + pw
			cfg.CustomServices = []config.CustomService{
				{Index: 1, Name: "api", Template: "go", Port: 8001, Route: "api", Memory: "256m", CPU: "0.5",
					ExtraEnv: "APP_DB_PASS=" + pw},
			}
			out, err := compose.NewGenerator(cfg).WithWorkDir(t.TempDir()).Generate()
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}
			secrets := build.SecretEnvMap(cfg)
			templated := build.TemplateSecrets(out, secrets)
			env := readComposeEnv(t, cfg, secrets)
			svcs := resolvedServices(t, templated, env)

			pg, rd := cfg.Postgres.Password, cfg.Redis.Password
			plain := []struct{ svc, key, want string }{
				{"postgres", "POSTGRES_PASSWORD", pg},
				{"auth", "AUTH_DB_PASSWORD", pg},
				{"redis", "REDISCLI_AUTH", rd},
				{"minio", "MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword},
				{"api", "S3_SECRET_KEY", cfg.Minio.RootPassword},
				{"hasura", "HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret},
				{"api", "HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret},
				// CS_N_ENV values are trimmed by the parser (applyExtraEnv).
				{"api", "APP_DB_PASS", strings.TrimSpace(pg)},
			}
			for _, c := range plain {
				got, ok := svcs[c.svc][c.key]
				if !ok {
					t.Errorf("%s has no %s; the consumer list is stale", c.svc, c.key)
					continue
				}
				if got != c.want {
					t.Errorf("%s %s resolves to %q, want %q", c.svc, c.key, got, c.want)
				}
			}
			urls := []struct{ svc, key, want string }{
				{"hasura", "HASURA_GRAPHQL_DATABASE_URL", pg},
				{"auth", "DATABASE_URL", pg},
				{"auth", "AUTH_DATABASE_URL", pg},
				{"auth", "HASURA_GRAPHQL_DATABASE_URL", pg},
				{"auth", "AUTH_DB_URL", pg},
				{"api", "DATABASE_URL", pg},
				{"api", "REDIS_URL", rd},
			}
			for _, c := range urls {
				raw, ok := svcs[c.svc][c.key]
				if !ok {
					t.Errorf("%s has no %s; the consumer list is stale", c.svc, c.key)
					continue
				}
				if got := urlPassword(t, c.svc+" "+c.key, raw); got != c.want {
					t.Errorf("%s %s password resolves to %q, want %q", c.svc, c.key, got, c.want)
				}
			}
			if got := urlPassword(t, "compose.env DATABASE_URL", env["DATABASE_URL"]); got != pg {
				t.Errorf("compose.env DATABASE_URL password resolves to %q, want %q", got, pg)
			}
			if leaks := build.LiteralSecretLeaks(templated, secrets); len(leaks) > 0 {
				t.Errorf("literal secrets left in generated compose: %v", leaks)
			}
			for _, leak := range decodedLeaks(t, templated, []string{pg, rd, cfg.Minio.RootPassword, cfg.Hasura.AdminSecret}) {
				t.Errorf("secret left literal in generated compose: %s", leak)
			}
		})
	}
}

// TestSecretConsumersResolveIdenticallyV15 sends the same hostile shapes through the
// v1.5 core object store: S3_ACCESS_KEY and S3_SECRET_KEY reach seaweedfs and the custom
// service as ${S3_*} references only and resolve to the configured bytes. Shapes the
// SeaweedFS identity file cannot hold (double quote, backslash, control character) are
// refused at build time with E142 instead.
func TestSecretConsumersResolveIdenticallyV15(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("OBJECT_STORAGE_BUCKETS", "")
	t.Setenv("SEAWEEDFS_PORT", "")
	t.Setenv("SEAWEEDFS_VERSION", "")
	for name, pw := range d0635Passwords {
		t.Run(name, func(t *testing.T) {
			cfg := optionalSecretsConfig("meilisearch", setSecrets)
			cfg.Minio = config.MinioConfig{S3AccessKey: "test-access-key", S3SecretKey: pw}
			cfg.CustomServices = []config.CustomService{
				{Index: 1, Name: "api", Template: "go", Port: 8001, Route: "api", Memory: "256m", CPU: "0.5"},
			}
			out, err := compose.NewGenerator(cfg).WithWorkDir(t.TempDir()).Generate()
			if strings.ContainsAny(pw, "\"\\") || strings.IndexFunc(pw, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
				if err == nil || !strings.Contains(err.Error(), "E142") || strings.Contains(err.Error(), pw) {
					t.Fatalf("want E142 without the value, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Generate() error: %v", err)
			}
			secrets := build.SecretEnvMap(cfg)
			templated := build.TemplateSecrets(out, secrets)
			env := readComposeEnv(t, cfg, secrets)
			svcs := resolvedServices(t, templated, env)
			for _, svc := range []string{"seaweedfs", "api"} {
				if got := svcs[svc]["S3_SECRET_KEY"]; got != pw {
					t.Errorf("%s S3_SECRET_KEY resolves to %q, want %q", svc, got, pw)
				}
				if got := svcs[svc]["S3_ACCESS_KEY"]; got != "test-access-key" {
					t.Errorf("%s S3_ACCESS_KEY resolves to %q", svc, got)
				}
			}
			for _, ref := range []string{"${S3_ACCESS_KEY}", "${S3_SECRET_KEY}"} {
				if !strings.Contains(string(out), ref) {
					t.Errorf("generated compose has no %s reference", ref)
				}
			}
			if leaks := build.LiteralSecretLeaks(templated, secrets); len(leaks) > 0 {
				t.Errorf("literal secrets left in generated compose: %v", leaks)
			}
			for _, leak := range decodedLeaks(t, templated, []string{pw}) {
				t.Errorf("secret left literal in generated compose: %s", leak)
			}
		})
	}
}

// decodedLeaks returns "service KEY" for every unresolved environment value
// of the templated YAML that still holds one of secrets, raw or URL-encoded.
// It reads YAML-decoded values, so a secret the YAML escaper rewrote (quotes,
// backslashes) cannot hide from it the way it hides from a byte search.
func decodedLeaks(t *testing.T, templated []byte, secrets []string) []string {
	t.Helper()
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(templated, &doc); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}
	var out []string
	for name, svc := range doc.Services {
		for k, v := range svc.Environment {
			for _, s := range secrets {
				if strings.Contains(v, s) || strings.Contains(v, ":"+config.URLPassword(s)+"@") {
					out = append(out, name+" "+k)
				}
			}
		}
	}
	return out
}
