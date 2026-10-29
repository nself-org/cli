package compose_test

// compose_config_integration_test.go — D-0668: the interpolation model in
// secret_literal_roundtrip_test.go (interpolateCompose, a port of compose-go/v2)
// is checked against the real thing. The test builds a synthetic project with
// "$"-holding fake passwords the way nself build does (Generate, TemplateSecrets,
// WriteComposeEnv), runs `docker compose --env-file .nself/compose.env config`
// and compares every environment value, command, entrypoint and healthcheck of
// every service with the model's answer and with the configured bytes.
//
// It needs the compose CLI only (config reads no daemon). It skips when neither
// `docker compose` nor `docker-compose` is installed.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// composeCLI returns the argv prefix of an installed compose CLI, or skips.
func composeCLI(t *testing.T) []string {
	t.Helper()
	if docker, err := exec.LookPath("docker"); err == nil {
		if exec.Command(docker, "compose", "version").Run() == nil {
			return []string{docker, "compose"}
		}
	}
	if standalone, err := exec.LookPath("docker-compose"); err == nil {
		return []string{standalone}
	}
	t.Skip("docker compose is not installed")
	return nil
}

// realService is the slice of `docker compose config --format json` this test reads.
type realService struct {
	Environment map[string]*string `json:"environment"`
	Command     any                `json:"command"`
	Entrypoint  any                `json:"entrypoint"`
	Healthcheck struct {
		Test any `json:"test"`
	} `json:"healthcheck"`
}

// realCompose runs the compose CLI over the project in dir and decodes its answer.
// The process env is scrubbed of every name compose.env defines (and COMPOSE_*), because
// compose lets the shell win over --env-file and a developer's own exports would mask a gap.
func realCompose(t *testing.T, cli []string, dir string, envFile map[string]string) map[string]realService {
	t.Helper()
	args := append(append([]string{}, cli[1:]...), "--project-name", "nselftest", "--env-file", ".nself/compose.env",
		"-f", "docker-compose.yml", "--profile", "*", "config", "--format", "json")
	cmd := exec.Command(cli[0], args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if _, shadow := envFile[name]; shadow || strings.HasPrefix(name, "COMPOSE_") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s config: %v\n%s", strings.Join(cli, " "), err, stderr.String())
	}
	var doc struct {
		Services map[string]realService `json:"services"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("decode compose config output: %v", err)
	}
	if len(doc.Services) == 0 {
		t.Fatal("compose config returned no services")
	}
	return doc.Services
}

// argvList returns the words of a JSON or YAML command, entrypoint or healthcheck test.
// A list keeps its elements whole; a string is split into shell words, as compose does.
func argvList(v any) []string {
	switch x := v.(type) {
	case string:
		return shellWords(x)
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// shellWords splits s on blanks, honouring single and double quotes (no escapes: the
// generated string commands hold none).
func shellWords(s string) []string {
	var words []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote != 0:
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if started {
				words = append(words, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if started {
		words = append(words, cur.String())
	}
	return words
}

// unescapeDollar undoes the "$$" escaping `docker compose config` applies to every "$" in its
// output so the output can be fed back to compose.
func unescapeDollar(s string) string { return strings.ReplaceAll(s, "$$", "$") }

func unescapeAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = unescapeDollar(s)
	}
	return out
}

// modelService is a service of the templated YAML after the port interpolated it.
type modelService struct {
	env                         map[string]string
	command, entrypoint, hcTest []string
}

// modelCompose interpolates every environment value, command, entrypoint and
// healthcheck of the templated YAML with the ported compose interpolator.
func modelCompose(t *testing.T, templated []byte, env map[string]string) map[string]modelService {
	t.Helper()
	var doc struct {
		Services map[string]struct {
			Environment map[string]string `yaml:"environment"`
			Command     any               `yaml:"command"`
			Entrypoint  any               `yaml:"entrypoint"`
			Healthcheck struct {
				Test any `yaml:"test"`
			} `yaml:"healthcheck"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(templated, &doc); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}
	words := func(where string, v any) []string {
		var out []string
		for _, s := range argvList(v) {
			r, err := interpolateCompose(s, env)
			if err != nil {
				t.Fatalf("%s %q: invalid compose template: %v", where, s, err)
			}
			out = append(out, r)
		}
		return out
	}
	out := map[string]modelService{}
	for name, svc := range doc.Services {
		m := modelService{env: map[string]string{}}
		for k, v := range svc.Environment {
			r, err := interpolateCompose(v, env)
			if err != nil {
				t.Fatalf("%s %s: invalid compose template: %v", name, k, err)
			}
			m.env[k] = r
		}
		m.command, m.entrypoint = words(name+" command", svc.Command), words(name+" entrypoint", svc.Entrypoint)
		m.hcTest = words(name+" healthcheck", svc.Healthcheck.Test)
		out[name] = m
	}
	return out
}

// TestComposeConfigMatchesInterpolationModel is the D-0668 check. Every "$" shape of
// d0635Passwords goes through the real compose CLI; the model and the CLI must agree on
// every service, and the credential consumers must resolve to the configured bytes.
func TestComposeConfigMatchesInterpolationModel(t *testing.T) {
	cli := composeCLI(t)
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	names := make([]string, 0, len(d0635Passwords))
	for name := range d0635Passwords {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pw := d0635Passwords[name]
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

			dir := t.TempDir()
			if err := build.WriteComposeEnv(dir, cfg, secrets, nil); err != nil {
				t.Fatalf("WriteComposeEnv: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), templated, 0o600); err != nil {
				t.Fatal(err)
			}
			envFile := readComposeEnv(t, cfg, secrets)
			real := realCompose(t, cli, dir, envFile)
			model := modelCompose(t, templated, envFile)

			for svc, want := range model {
				got, ok := real[svc]
				if !ok {
					t.Errorf("%s: missing from compose config output", svc)
					continue
				}
				for key, w := range want.env {
					g := ""
					if p := got.Environment[key]; p != nil {
						g = unescapeDollar(*p)
					}
					if g != w {
						t.Errorf("%s %s: compose resolves %q, model %q", svc, key, g, w)
					}
				}
				for key := range got.Environment {
					if _, ok := want.env[key]; !ok {
						t.Errorf("%s %s: compose added a variable the model did not see", svc, key)
					}
				}
				for field, pair := range map[string][2][]string{
					"command":          {unescapeAll(argvList(got.Command)), want.command},
					"entrypoint":       {unescapeAll(argvList(got.Entrypoint)), want.entrypoint},
					"healthcheck.test": {unescapeAll(argvList(got.Healthcheck.Test)), want.hcTest},
				} {
					if strings.Join(pair[0], "\x00") != strings.Join(pair[1], "\x00") {
						t.Errorf("%s %s: compose resolves %q, model %q", svc, field, pair[0], pair[1])
					}
				}
			}

			// The model and compose could drift the same way: pin the consumers to the configured bytes.
			pg, rd := cfg.Postgres.Password, cfg.Redis.Password
			for _, c := range []struct{ svc, key, want string }{
				{"postgres", "POSTGRES_PASSWORD", pg},
				{"auth", "AUTH_DB_PASSWORD", pg},
				{"redis", "REDISCLI_AUTH", rd},
				{"minio", "MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword},
				{"api", "S3_SECRET_KEY", cfg.Minio.RootPassword},
				{"hasura", "HASURA_GRAPHQL_ADMIN_SECRET", cfg.Hasura.AdminSecret},
				{"api", "APP_DB_PASS", strings.TrimSpace(pg)},
			} {
				p := real[c.svc].Environment[c.key]
				if p == nil || unescapeDollar(*p) != c.want {
					t.Errorf("compose %s %s does not resolve to the configured bytes %q (got %v)", c.svc, c.key, c.want, p != nil)
				}
			}
			for _, c := range []struct{ svc, key, want string }{
				{"hasura", "HASURA_GRAPHQL_DATABASE_URL", pg},
				{"auth", "DATABASE_URL", pg},
				{"api", "DATABASE_URL", pg},
				{"api", "REDIS_URL", rd},
			} {
				p := real[c.svc].Environment[c.key]
				if p == nil {
					t.Errorf("compose %s has no %s", c.svc, c.key)
					continue
				}
				if got := urlPassword(t, c.svc+" "+c.key, unescapeDollar(*p)); got != c.want {
					t.Errorf("compose %s %s password resolves to %q, want %q", c.svc, c.key, got, c.want)
				}
			}
		})
	}
}
