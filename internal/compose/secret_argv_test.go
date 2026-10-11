package compose_test

// secret_argv_test.go — D-0282 guard: no generated command, entrypoint or
// healthcheck carries a secret. A literal there is visible in the compose
// file; a ${SECRET} reference is interpolated by compose into the container's
// Cmd or healthcheck, where `docker ps --no-trunc`, `docker inspect` and host
// `ps` show the value (the prod Redis leak). A secret reaches a process
// through its environment instead, like every other secret ($$NAME, expanded
// by a shell inside the container, is fine).

import (
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/cli/internal/build"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// argvService holds the argv-shaped fields of one compose service.
type argvService struct {
	Command     yaml.Node `yaml:"command"`
	Entrypoint  yaml.Node `yaml:"entrypoint"`
	Healthcheck struct {
		Test yaml.Node `yaml:"test"`
	} `yaml:"healthcheck"`
}

// argvStrings returns every scalar of the argv fields, keyed "service field".
func argvStrings(t *testing.T, composeYAML []byte) map[string][]string {
	t.Helper()
	var doc struct {
		Services map[string]argvService `yaml:"services"`
	}
	if err := yaml.Unmarshal(composeYAML, &doc); err != nil {
		t.Fatalf("unmarshal compose: %v", err)
	}
	out := make(map[string][]string)
	for name, svc := range doc.Services {
		for field, node := range map[string]*yaml.Node{
			"command": &svc.Command, "entrypoint": &svc.Entrypoint, "healthcheck.test": &svc.Healthcheck.Test,
		} {
			out[name+" "+field] = append(out[name+" "+field], scalars(node)...)
		}
	}
	return out
}

func scalars(n *yaml.Node) []string {
	if n.Kind == yaml.ScalarNode {
		return []string{n.Value}
	}
	var s []string
	for _, c := range n.Content {
		s = append(s, scalars(c)...)
	}
	return s
}

// interpolatedNames returns the variable names compose interpolates in s:
// $NAME and ${NAME...}, skipping the $$ escape.
func interpolatedNames(s string) []string {
	var names []string
	for i := 0; i < len(s); i++ {
		if s[i] != '$' || i+1 == len(s) {
			continue
		}
		if s[i+1] == '$' {
			i++
			continue
		}
		j := i + 1
		if s[j] == '{' {
			j++
		}
		k := j
		for k < len(s) && (s[k] == '_' || s[k] >= 'A' && s[k] <= 'Z' || s[k] >= 'a' && s[k] <= 'z' || k > j && s[k] >= '0' && s[k] <= '9') {
			k++
		}
		if k > j {
			names = append(names, s[j:k])
		}
	}
	return names
}

// argvSecretFindings lists every argv string holding a secret value or a
// compose reference to a secret variable.
func argvSecretFindings(argv map[string][]string, secrets map[string]string) []string {
	var findings []string
	for where, strs := range argv {
		for _, s := range strs {
			for key, val := range secrets {
				if len(val) >= 4 && strings.Contains(s, val) {
					findings = append(findings, where+": literal value of "+key)
				}
			}
			for _, name := range interpolatedNames(s) {
				if _, ok := secrets[strings.TrimSuffix(name, config.URLPasswordEnvSuffix)]; ok {
					findings = append(findings, where+": compose interpolates ${"+name+"} onto argv")
				}
			}
		}
	}
	sort.Strings(findings)
	return findings
}

// TestArgvSecretFindingsControl proves the checker flags both leak shapes and
// accepts a shell-expanded environment variable.
func TestArgvSecretFindingsControl(t *testing.T) {
	secrets := map[string]string{"REDIS_PASSWORD": "fakeRedisPw0123"}
	argv := map[string][]string{
		"redis healthcheck.test": {"CMD", "redis-cli", "-a", "${REDIS_PASSWORD}", "ping"},
		"redis command":          {"redis-server --requirepass fakeRedisPw0123"},
		"x command":              {"sh", "-c", `exec x --pw "$$REDISCLI_AUTH" --u $REDIS_PASSWORD_URLENC`},
		"ok command":             {"sh", "-c", `exec x --pw "$$REDIS_PASSWORD"`},
	}
	got := argvSecretFindings(argv, secrets)
	want := []string{
		"redis command: literal value of REDIS_PASSWORD",
		"redis healthcheck.test: compose interpolates ${REDIS_PASSWORD} onto argv",
		"x command: compose interpolates ${REDIS_PASSWORD_URLENC} onto argv",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("findings:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestNoSecretOnGeneratedArgv generates the full compose (core and every
// optional service, both search engines), templates it as nself build does,
// and checks every command, entrypoint and healthcheck.
func TestNoSecretOnGeneratedArgv(t *testing.T) {
	for _, engine := range []string{"meilisearch", "typesense"} {
		cfg := optionalSecretsConfig(engine, setSecrets)
		cfg.Minio.S3SecretKey = "s3SecretSentinel0123456789"
		out, err := compose.NewGenerator(cfg).Generate()
		if err != nil {
			t.Fatalf("%s: Generate() error: %v", engine, err)
		}
		secrets := build.SecretEnvMap(cfg)
		argv := argvStrings(t, build.TemplateSecrets(out, secrets))
		for _, svc := range []string{"postgres", "hasura", "auth", "redis", "minio", "mailpit", engine} {
			if _, ok := argv[svc+" healthcheck.test"]; !ok {
				t.Fatalf("%s: service %q missing; the guard would not cover it", engine, svc)
			}
		}
		for _, f := range argvSecretFindings(argv, secrets) {
			t.Errorf("%s: %s", engine, f)
		}
		if got := argv["redis healthcheck.test"]; strings.Join(got, " ") != "CMD redis-cli ping" {
			t.Errorf("%s: redis healthcheck = %q, want redis-cli reading REDISCLI_AUTH", engine, got)
		}
	}
}

// TestNoSecretOnMonitoringArgv applies the same check to the shipped
// monitoring compose file.
func TestNoSecretOnMonitoringArgv(t *testing.T) {
	data, err := os.ReadFile("docker-compose.monitoring.yml")
	if err != nil {
		t.Fatal(err)
	}
	secrets := build.SecretEnvMap(optionalSecretsConfig("meilisearch", setSecrets))
	argv := argvStrings(t, data)
	if len(argv) == 0 {
		t.Fatal("no services parsed from docker-compose.monitoring.yml")
	}
	for _, f := range argvSecretFindings(argv, secrets) {
		t.Error(f)
	}
}
