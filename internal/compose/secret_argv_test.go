package compose_test

// secret_argv_test.go — D-0282 guard: no generated command, entrypoint or
// healthcheck carries a secret. A literal there is visible in the compose
// file; a ${SECRET} reference is interpolated by compose into the container's
// Cmd or healthcheck, where `docker ps --no-trunc`, `docker inspect` and host
// `ps` show the value (the prod Redis leak). A secret reaches a process
// through its environment instead, like every other secret ($$NAME, expanded
// by a shell inside the container, is fine).

import (
	"github.com/nself-org/cli/internal/compat/compattest"
	"os"
	"path/filepath"
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

// allSecretsConfig is optionalSecretsConfig with a synthetic value in every
// config field build.SecretEnvMap reads (SMTP, Grafana, S3, search, plugin
// secrets) and two custom services, so the guard sees a secret for every key it
// could meet on an argv (D-0588).
func allSecretsConfig(engine string) *config.Config {
	cfg := optionalSecretsConfig(engine, setSecrets)
	cfg.Auth.SMTPPass = "smtpSentinel0123456789"
	cfg.Monitoring.GrafanaAdminPassword = "grafanaSentinel0123456789"
	cfg.Minio.S3AccessKey = "s3AccessSentinel0123456789"
	cfg.Minio.S3SecretKey = "s3SecretSentinel0123456789"
	cfg.Search.APIKey = "searchApiSentinel0123456789"
	cfg.PluginSystem.InternalSecret = "pluginInternalSentinel0123456789"
	cfg.PluginConfig.NotifySecret = "notifyInternalSentinel0123456789"
	cfg.CustomServices = []config.CustomService{
		{Name: "api", Port: 3000},
		{Name: "worker", Port: 3001, HealthCheck: "/internal/health", EnvPassthrough: "REDIS_PASSWORD,SMTP_PASSWORD"},
	}
	return cfg
}

// expectedSecretKeys is every build.SecretEnvMap key allSecretsConfig must
// populate. A key added to SecretEnvMap without a fixture value, or a fixture
// field that stops flowing into the map, fails TestAllSecretsConfigCoversSecretEnvMap.
var expectedSecretKeys = []string{
	"POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "HASURA_JWT_KEY", "HASURA_GRAPHQL_JWT_SECRET",
	"MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "S3_ACCESS_KEY", "S3_SECRET_KEY", "REDIS_PASSWORD",
	"AUTH_SMTP_PASS", "MAILPIT_UI_PASSWORD", "ADMIN_SECRET_KEY", "ADMIN_PASSWORD_HASH",
	"GRAFANA_ADMIN_PASSWORD", "SEARCH_API_KEY", "MEILISEARCH_MASTER_KEY", "TYPESENSE_API_KEY",
	"PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET",
}

// TestAllSecretsConfigCoversSecretEnvMap: the argv guard only knows the keys the
// fixture fills, so the fixture must fill them all with distinct values.
func TestAllSecretsConfigCoversSecretEnvMap(t *testing.T) {
	secrets := build.SecretEnvMap(allSecretsConfig("meilisearch"))
	want := make(map[string]bool, len(expectedSecretKeys))
	for _, k := range expectedSecretKeys {
		want[k] = true
		if secrets[k] == "" {
			t.Errorf("SecretEnvMap key %s is empty in the fixture: the argv guard cannot see it", k)
		}
	}
	for k := range secrets {
		if !want[k] {
			t.Errorf("SecretEnvMap gained key %s: add it to expectedSecretKeys and fill it in allSecretsConfig", k)
		}
	}
	seen := make(map[string]string)
	for k, v := range secrets {
		if other, dup := seen[v]; dup && k != "HASURA_JWT_KEY" && other != "HASURA_JWT_KEY" {
			t.Errorf("%s and %s share a value; the guard could not tell which leaked", k, other)
		}
		seen[v] = k
	}
}

// TestArgvGuardSeesPreviouslyUncoveredSecrets: with the full fixture the guard
// flags the keys it used to miss (Grafana, SMTP, S3 access key, search, plugin).
func TestArgvGuardSeesPreviouslyUncoveredSecrets(t *testing.T) {
	secrets := build.SecretEnvMap(allSecretsConfig("meilisearch"))
	argv := map[string][]string{
		"grafana healthcheck.test": {"CMD", "curl", "-u", "admin:${GRAFANA_ADMIN_PASSWORD}", "http://localhost:3000"},
		"mail command":             {"sh", "-c", "send --pass " + secrets["AUTH_SMTP_PASS"]},
		"s3 entrypoint":            {"tool", "--key", "${S3_ACCESS_KEY}"},
		"search command":           {"search", "--api-key", "${SEARCH_API_KEY}"},
	}
	got := strings.Join(argvSecretFindings(argv, secrets), "\n")
	for _, want := range []string{"GRAFANA_ADMIN_PASSWORD", "AUTH_SMTP_PASS", "S3_ACCESS_KEY", "SEARCH_API_KEY"} {
		if !strings.Contains(got, want) {
			t.Errorf("guard missed %s:\n%s", want, got)
		}
	}
}

// TestNoSecretOnGeneratedArgv generates the full compose (core and every
// optional service, both search engines), templates it as nself build does,
// and checks every command, entrypoint and healthcheck.
func TestNoSecretOnGeneratedArgv(t *testing.T) {
	compattest.Set(t, false) // core MinIO exists only in v1.4 (P7-PLUG-60)
	for _, engine := range []string{"meilisearch", "typesense"} {
		// Both cache engines (P7-LIVE-19): neither may carry the password on argv,
		// and each runs its own server and CLI binary.
		for _, cache := range []string{"redis", "valkey"} {
			cfg := allSecretsConfig(engine)
			cfg.Redis.Engine = cache
			out, err := compose.NewGenerator(cfg).Generate()
			if err != nil {
				t.Fatalf("%s/%s: Generate() error: %v", engine, cache, err)
			}
			secrets := build.SecretEnvMap(cfg)
			argv := argvStrings(t, build.TemplateSecrets(out, secrets))
			for _, svc := range []string{"postgres", "hasura", "auth", "redis", "minio", "mailpit", engine} {
				if _, ok := argv[svc+" healthcheck.test"]; !ok {
					t.Fatalf("%s/%s: service %q missing; the guard would not cover it", engine, cache, svc)
				}
			}
			for _, f := range argvSecretFindings(argv, secrets) {
				t.Errorf("%s/%s: %s", engine, cache, f)
			}
			if got := argv["redis healthcheck.test"]; strings.Join(got, " ") != "CMD "+cache+"-cli ping" {
				t.Errorf("%s/%s: redis healthcheck = %q, want %s-cli reading REDISCLI_AUTH", engine, cache, got, cache)
			}
			if got := strings.Join(argv["redis command"], " "); !strings.Contains(got, `exec `+cache+`-server `) || !strings.Contains(got, `requirepass "$$esc"`) || strings.Contains(got, "--requirepass") {
				t.Errorf("%s/%s: redis command = %q, want %s-server reading the password from stdin", engine, cache, got, cache)
			}
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
	secrets := build.SecretEnvMap(allSecretsConfig("meilisearch"))
	argv := argvStrings(t, data)
	if len(argv) == 0 {
		t.Fatal("no services parsed from docker-compose.monitoring.yml")
	}
	for _, f := range argvSecretFindings(argv, secrets) {
		t.Error(f)
	}
}

// TestNoSecretOnV15ObjectStoreArgv: with SeaweedFS as the core store the S3
// credentials are secrets too (v1.5), and neither the server nor the init
// service may carry them on argv.
func TestNoSecretOnV15ObjectStoreArgv(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	t.Setenv("OBJECT_STORAGE_BUCKETS", "uploads,archives")
	cfg := allSecretsConfig("meilisearch")
	cfg.Minio.Enabled = false
	out, err := compose.NewGenerator(cfg).Generate()
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	secrets := build.SecretEnvMap(cfg)
	argv := argvStrings(t, build.TemplateSecrets(out, secrets))
	for _, svc := range []string{"seaweedfs", "seaweedfs-init", "api", "worker"} {
		if _, ok := argv[svc+" healthcheck.test"]; !ok && svc != "seaweedfs-init" {
			t.Fatalf("service %q missing; the guard would not cover it", svc)
		}
	}
	if _, ok := secrets["S3_SECRET_KEY"]; !ok {
		t.Fatal("S3_SECRET_KEY missing from the secret map")
	}
	for _, f := range argvSecretFindings(argv, secrets) {
		t.Error(f)
	}
}

// pluginFragment is a representative installed plugin: a command and healthcheck that
// read their secrets through the container environment ($$NAME), which is safe.
const pluginFragment = `services:
  demo:
    image: nself/demo:latest
    restart: unless-stopped
    command: ["sh", "-c", "exec demo --listen :3900 --secret \"$$PLUGIN_INTERNAL_SECRET\""]
    healthcheck:
      test: ["CMD-SHELL", "wget -q --spider --header \"X-Secret: $$NOTIFY_INTERNAL_SECRET\" http://localhost:3900/health"]
    environment:
      - PORT=3900
      - DATABASE_URL=${DATABASE_URL}
`

// pluginLeakFragment puts plugin secrets on argv through compose interpolation.
const pluginLeakFragment = `services:
  leaky:
    image: nself/leaky:latest
    environment:
      - PORT=3901
    command: ["leaky", "--secret", "${PLUGIN_INTERNAL_SECRET}"]
    healthcheck:
      test: ["CMD", "wget", "-q", "--header", "X-Secret: ${NOTIFY_INTERNAL_SECRET}", "http://localhost:3901/health"]
`

// pluginArgv installs fragment as plugin "name", runs nself's own fragment
// normalization and returns the argv strings of the resulting overlay.
func pluginArgv(t *testing.T, name, fragment string) map[string][]string {
	t.Helper()
	pluginDir, workdir := t.TempDir(), t.TempDir()
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"manifest_version":2,"name":"` + name + `","service":{"kind":"compose"}}`
	for file, body := range map[string]string{"plugin.json": manifest, "docker-compose.plugin.yml": fragment} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := build.DiscoverPluginComposeFiles(workdir, pluginDir)
	if err != nil || len(files) != 1 {
		t.Fatalf("DiscoverPluginComposeFiles: files=%v err=%v", files, err)
	}
	overlay, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	return argvStrings(t, overlay)
}

// TestNoSecretOnPluginFragmentArgv runs the guard over plugin output after the
// CLI's fragment normalization, with a control that proves a leaking fragment fails it.
func TestNoSecretOnPluginFragmentArgv(t *testing.T) {
	compattest.Set(t, false)
	secrets := build.SecretEnvMap(allSecretsConfig("meilisearch"))
	argv := pluginArgv(t, "demo", pluginFragment)
	if len(argv["demo command"]) == 0 || len(argv["demo healthcheck.test"]) == 0 {
		t.Fatalf("plugin argv not parsed: %v", argv)
	}
	for _, f := range argvSecretFindings(argv, secrets) {
		t.Error(f)
	}
	leak := argvSecretFindings(pluginArgv(t, "leaky", pluginLeakFragment), secrets)
	want := []string{
		"leaky command: compose interpolates ${PLUGIN_INTERNAL_SECRET} onto argv",
		"leaky healthcheck.test: compose interpolates ${NOTIFY_INTERNAL_SECRET} onto argv",
	}
	if strings.Join(leak, "\n") != strings.Join(want, "\n") {
		t.Fatalf("control findings:\n%s\nwant:\n%s", strings.Join(leak, "\n"), strings.Join(want, "\n"))
	}
}
