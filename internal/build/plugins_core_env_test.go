package build

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// writePluginManifestJSON plants <pluginDir>/<name>/plugin.json with the
// given port so readPluginManifest (used by normalizeComposePluginCoreEnv)
// resolves SERVICE_PORT.
func writePluginManifestJSON(t *testing.T, pluginDir, name string, port int) {
	t.Helper()
	dir := filepath.Join(pluginDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := `{"name":"` + name + `","port":` + strconv.Itoa(port) + `,"language":"rust"}`
	if err := os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(body), 0644); err != nil {
		t.Fatalf("write plugin.json: %v", err)
	}
}

// TestNormalizeComposePluginCoreEnv_AiShape mirrors the real installed
// plugins/free/ai/docker-compose.plugin.yml fragment (2026-09-21 E2E golden
// path evidence): environment: already declares DATABASE_URL and PORT.
// Every other core key must be appended; the two existing entries must be
// left exactly as authored.
func TestNormalizeComposePluginCoreEnv_AiShape(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "ai", 3709)

	in := `services:
  ai:
    build:
      context: ${NSELF_PLUGIN_DIR}/ai
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME}_ai
    restart: unless-stopped
    hostname: plugin-ai
    networks:
      - ${DOCKER_NETWORK}
    ports:
      - "127.0.0.1:3709:3709"
    environment:
      DATABASE_URL: ${DATABASE_URL}
      PORT: "3709"
    depends_on:
      postgres:
        condition: service_healthy
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "ai"))

	if !strings.Contains(out, "DATABASE_URL: ${DATABASE_URL}") {
		t.Errorf("existing DATABASE_URL entry must be preserved verbatim:\n%s", out)
	}
	if !strings.Contains(out, `PORT: "3709"`) {
		t.Errorf("existing PORT entry must be preserved verbatim:\n%s", out)
	}
	for _, want := range []string{
		"ENV: ${ENV}",
		"NSELF_ENV: ${ENV}",
		"PROJECT_NAME: ${PROJECT_NAME}",
		"COMPOSE_PROJECT_NAME: ${COMPOSE_PROJECT_NAME}",
		"BASE_DOMAIN: ${BASE_DOMAIN}",
		"POSTGRES_HOST: postgres",
		`POSTGRES_PORT: "5432"`,
		"POSTGRES_DB: ${POSTGRES_DB}",
		"POSTGRES_USER: ${POSTGRES_USER}",
		"POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}",
		"HASURA_GRAPHQL_ENDPOINT: http://hasura:8080/v1/graphql",
		"HASURA_GRAPHQL_ADMIN_SECRET: ${HASURA_GRAPHQL_ADMIN_SECRET}",
		"PLUGIN_INTERNAL_SECRET: ${PLUGIN_INTERNAL_SECRET}",
		"NOTIFY_INTERNAL_SECRET: ${NOTIFY_INTERNAL_SECRET}",
		"SERVICE_NAME: ai",
		`SERVICE_PORT: "3709"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing injected core env line %q in:\n%s", want, out)
		}
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}

	// Idempotent: normalizing again must not change the output further.
	again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "ai"))
	if again != out {
		t.Errorf("normalizeComposePluginCoreEnv is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

// TestNormalizeComposePluginCoreEnv_ExplicitValueWins verifies a fragment
// that already declares PLUGIN_INTERNAL_SECRET with its own value is never
// overwritten by the injected ${PLUGIN_INTERNAL_SECRET} reference.
func TestNormalizeComposePluginCoreEnv_ExplicitValueWins(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "mux", 3710)

	in := `services:
  mux:
    image: nself/nself-mux:latest
    networks:
      - ${DOCKER_NETWORK}
    environment:
      DATABASE_URL: ${DATABASE_URL}
      PLUGIN_INTERNAL_SECRET: some-hand-authored-value
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "mux"))
	if !strings.Contains(out, "PLUGIN_INTERNAL_SECRET: some-hand-authored-value") {
		t.Errorf("explicit fragment value must win over the injected reference:\n%s", out)
	}
	if strings.Contains(out, "PLUGIN_INTERNAL_SECRET: ${PLUGIN_INTERNAL_SECRET}") {
		t.Errorf("must not also inject the ${{VAR}} reference alongside the explicit value:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}
}

// TestNormalizeComposePluginCoreEnv_NoEnvironmentBlock covers a fragment
// (the notify/cron shape, per the E2E context) that has no environment:
// block at all yet — one must be inserted anchored before networks:.
func TestNormalizeComposePluginCoreEnv_NoEnvironmentBlock(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "notify", 3711)

	in := `services:
  notify:
    build:
      context: ${NSELF_PLUGIN_DIR}/notify
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME}_notify
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "notify"))
	if !strings.Contains(out, "environment:") {
		t.Fatalf("expected a new environment: block to be inserted:\n%s", out)
	}
	if !strings.Contains(out, "SERVICE_NAME: notify") || !strings.Contains(out, `SERVICE_PORT: "3711"`) {
		t.Errorf("expected SERVICE_NAME/SERVICE_PORT for notify:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}
}

// TestNormalizeComposePluginCoreEnv_UnknownPortOmitsServicePort covers a
// plugin with no resolvable port (missing/unreadable plugin.json) — every
// other key is still injected, but SERVICE_PORT must be omitted rather than
// emitted as a misleading "0".
func TestNormalizeComposePluginCoreEnv_UnknownPortOmitsServicePort(t *testing.T) {
	pluginDir := t.TempDir() // no plugin.json planted for "cron"

	in := `services:
  cron:
    image: nself/nself-cron:latest
    networks:
      - ${DOCKER_NETWORK}
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "cron"))
	if strings.Contains(out, "SERVICE_PORT") {
		t.Errorf("SERVICE_PORT must be omitted when the plugin's port is unknown:\n%s", out)
	}
	if !strings.Contains(out, "SERVICE_NAME: cron") {
		t.Errorf("SERVICE_NAME must still be injected:\n%s", out)
	}
}

// TestAddPluginCoreEnvVars_NilCfgIsNoop preserves ComputePluginEnvVars'
// pre-existing zero-cfg callers/tests.
func TestAddPluginCoreEnvVars_NilCfgIsNoop(t *testing.T) {
	vars := map[string]string{"NSELF_PLUGIN_DIR": "/x"}
	addPluginCoreEnvVars(vars, nil)
	if len(vars) != 1 {
		t.Errorf("nil cfg must be a no-op, got: %v", vars)
	}
}

// TestAddPluginCoreEnvVars_Populated verifies the project-specific values
// land, and that an empty plugin secret is omitted rather than written as "".
func TestAddPluginCoreEnvVars_Populated(t *testing.T) {
	cfg := &config.Config{
		ProjectName: "acme",
		BaseDomain:  "local.nself.org",
		Env:         "dev",
	}
	cfg.Postgres.DB = "nself"
	cfg.Postgres.User = "postgres"
	cfg.PluginSystem.InternalSecret = "plugin-secret-value"
	// NotifySecret intentionally left empty.

	vars := map[string]string{}
	addPluginCoreEnvVars(vars, cfg)

	want := map[string]string{
		"ENV":                    "dev",
		"PROJECT_NAME":           "acme",
		"COMPOSE_PROJECT_NAME":   "acme",
		"BASE_DOMAIN":            "local.nself.org",
		"POSTGRES_DB":            "nself",
		"POSTGRES_USER":          "postgres",
		"PLUGIN_INTERNAL_SECRET": "plugin-secret-value",
	}
	for k, v := range want {
		if vars[k] != v {
			t.Errorf("addPluginCoreEnvVars[%s] = %q, want %q", k, vars[k], v)
		}
	}
	if _, ok := vars["NOTIFY_INTERNAL_SECRET"]; ok {
		t.Errorf("empty NotifySecret must not be written, got: %v", vars["NOTIFY_INTERNAL_SECRET"])
	}
}
