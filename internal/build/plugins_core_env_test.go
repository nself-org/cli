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

// TestNormalizeComposePluginCoreEnv_ListFormShape covers the list-form
// "environment: [- KEY=VALUE, ...]" shape used by 40+ installed plugin
// fragments (paid/browser, paid/google, paid/cms, paid/moderation,
// paid/social, paid/support and free plugins like cron) — the defect this
// change fixes silently returned such fragments unchanged. Missing keys are
// appended at the existing entries' indentation; a key already present as
// either "- KEY=val" or a bare passthrough "- KEY" must not be duplicated
// or overwritten.
func TestNormalizeComposePluginCoreEnv_ListFormShape(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "browser", 3712)

	in := `services:
  browser:
    build:
      context: ${NSELF_PLUGIN_DIR}/browser
      dockerfile: Dockerfile
    container_name: ${COMPOSE_PROJECT_NAME}_browser
    restart: unless-stopped
    networks:
      - ${DOCKER_NETWORK}
    environment:
      - DATABASE_URL=${DATABASE_URL}
      - PLUGIN_INTERNAL_SECRET=hand-authored-value
      - DEBUG
    depends_on:
      postgres:
        condition: service_healthy
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "browser"))

	for _, want := range []string{
		"- DATABASE_URL=${DATABASE_URL}",
		"- PLUGIN_INTERNAL_SECRET=hand-authored-value",
		"- DEBUG",
		"- ENV=${ENV}",
		"- NSELF_ENV=${ENV}",
		"- PROJECT_NAME=${PROJECT_NAME}",
		"- COMPOSE_PROJECT_NAME=${COMPOSE_PROJECT_NAME}",
		"- BASE_DOMAIN=${BASE_DOMAIN}",
		"- POSTGRES_HOST=postgres",
		"- POSTGRES_PORT=5432",
		"- POSTGRES_DB=${POSTGRES_DB}",
		"- POSTGRES_USER=${POSTGRES_USER}",
		"- POSTGRES_PASSWORD=${POSTGRES_PASSWORD}",
		"- HASURA_GRAPHQL_ENDPOINT=http://hasura:8080/v1/graphql",
		"- HASURA_GRAPHQL_ADMIN_SECRET=${HASURA_GRAPHQL_ADMIN_SECRET}",
		"- NOTIFY_INTERNAL_SECRET=${NOTIFY_INTERNAL_SECRET}",
		"- SERVICE_NAME=browser",
		"- SERVICE_PORT=3712",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing injected list-form line %q in:\n%s", want, out)
		}
	}
	// No quoting must leak from the map-form-only literals (POSTGRES_PORT,
	// SERVICE_PORT are `"..."`-wrapped in map form).
	if strings.Contains(out, `POSTGRES_PORT="5432"`) || strings.Contains(out, `SERVICE_PORT="3712"`) {
		t.Errorf("list-form values must not carry map-form literal quoting:\n%s", out)
	}
	// The explicit fragment value must win — not be duplicated with the
	// injected ${PLUGIN_INTERNAL_SECRET} reference.
	if strings.Contains(out, "PLUGIN_INTERNAL_SECRET=${PLUGIN_INTERNAL_SECRET}") {
		t.Errorf("explicit list-form value must not be duplicated by the injected reference:\n%s", out)
	}
	if n := strings.Count(out, "PLUGIN_INTERNAL_SECRET"); n != 1 {
		t.Errorf("PLUGIN_INTERNAL_SECRET must appear exactly once, got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "- DEBUG"); n != 1 {
		t.Errorf("bare passthrough DEBUG entry must not be duplicated, got %d:\n%s", n, out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}

	// Idempotent, same guarantee as the map-form path.
	again := string(normalizeComposePluginCoreEnv([]byte(out), pluginDir, "browser"))
	if again != out {
		t.Errorf("list-form injection is not idempotent:\nfirst:\n%s\nsecond:\n%s", out, again)
	}
}

// TestNormalizeComposePluginCoreEnv_EmptyListBlock covers an inline
// "environment: []" — it must become a populated list-form block rather
// than being left as an empty literal.
func TestNormalizeComposePluginCoreEnv_EmptyListBlock(t *testing.T) {
	pluginDir := t.TempDir()
	writePluginManifestJSON(t, pluginDir, "support", 3713)

	in := `services:
  support:
    image: nself/nself-support:latest
    environment: []
    networks:
      - ${DOCKER_NETWORK}
`
	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "support"))
	if strings.Contains(out, "environment: []") {
		t.Errorf("environment: [] must be rewritten to a populated block:\n%s", out)
	}
	if !strings.Contains(out, "- SERVICE_NAME=support") {
		t.Errorf("expected list-form SERVICE_NAME entry:\n%s", out)
	}
	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten fragment is not valid YAML: %v\n%s", err, out)
	}
}

// captureStderr runs fn and returns whatever it wrote to os.Stderr —
// ui.Warn's destination — so tests can assert on the unrecognised-shape
// warning without depending on ui package internals.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = w
	defer func() { os.Stderr = orig }()

	fn()

	_ = w.Close()
	var buf strings.Builder
	buf.Grow(256)
	tmp := make([]byte, 256)
	for {
		n, readErr := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if readErr != nil {
			break
		}
	}
	return buf.String()
}

// TestNormalizeComposePluginCoreEnv_MergeKeyWarnsAndLeavesUnchanged covers
// an environment: block using a YAML merge key ("<<: *anchor") — its
// expanded keys aren't visible to a text-level rewrite, so the fragment
// must be left byte-for-byte unchanged and a warning naming the plugin and
// file emitted, rather than silently doing nothing (the defect this change
// fixes for list form) or guessing.
func TestNormalizeComposePluginCoreEnv_MergeKeyWarnsAndLeavesUnchanged(t *testing.T) {
	pluginDir := t.TempDir()

	in := `services:
  cms:
    image: nself/nself-cms:latest
    environment:
      <<: *common-env
      DATABASE_URL: ${DATABASE_URL}
    networks:
      - ${DOCKER_NETWORK}
`
	var out string
	stderr := captureStderr(t, func() {
		out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "cms"))
	})
	if out != in {
		t.Errorf("merge-key fragment must be left byte-for-byte unchanged:\ngot:\n%s\nwant:\n%s", out, in)
	}
	if !strings.Contains(stderr, "cms") {
		t.Errorf("expected a warning naming the plugin, got stderr: %q", stderr)
	}
}

// TestNormalizeComposePluginCoreEnv_UnanchoredUnrecognisedWarns covers a
// fragment with neither an environment: block nor a short-form networks:
// list to anchor a new one on (the env_file-only shape) — must warn and
// leave the fragment unchanged rather than silently no-op.
func TestNormalizeComposePluginCoreEnv_UnanchoredUnrecognisedWarns(t *testing.T) {
	pluginDir := t.TempDir()

	in := `services:
  legacy:
    image: nself/nself-legacy:latest
    env_file:
      - legacy.env
`
	var out string
	stderr := captureStderr(t, func() {
		out = string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "legacy"))
	})
	if out != in {
		t.Errorf("unanchored fragment must be left unchanged:\ngot:\n%s\nwant:\n%s", out, in)
	}
	if !strings.Contains(stderr, "legacy") {
		t.Errorf("expected a warning naming the plugin, got stderr: %q", stderr)
	}
}

// TestNormalizeComposePluginCoreEnv_RealCronFragmentGolden is the golden
// test against a real shipped fragment: plugins/free/cron's
// docker-compose.plugin.yml (mirrored at
// testdata/plugin-compose-fixtures/cron.yml, reused from
// plugins_build_context_test.go's readFixture) uses list-form environment:
// with a comment interleaved among entries — exactly the shape that
// silently fell through untouched before this change. The rewritten
// fragment must still parse as valid YAML and must carry
// PLUGIN_INTERNAL_SECRET in the cron service's environment.
func TestNormalizeComposePluginCoreEnv_RealCronFragmentGolden(t *testing.T) {
	pluginDir := t.TempDir() // no plugin.json — port is unknown, fine for this test
	in := readFixture(t, "cron.yml")

	out := string(normalizeComposePluginCoreEnv([]byte(in), pluginDir, "cron"))

	if err := assertValidComposeYAML(out); err != nil {
		t.Fatalf("rewritten real cron fragment is not valid YAML: %v\n%s", err, out)
	}
	if !strings.Contains(out, "- PLUGIN_INTERNAL_SECRET=${PLUGIN_INTERNAL_SECRET}") {
		t.Errorf("expected PLUGIN_INTERNAL_SECRET to be injected into the cron service environment:\n%s", out)
	}
	// Every pre-existing entry (including the interleaved comment) must
	// survive untouched.
	if !strings.Contains(out, "- DATABASE_URL=postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@postgres:5432/${POSTGRES_DB}") {
		t.Errorf("existing DATABASE_URL entry must be preserved verbatim:\n%s", out)
	}
	if !strings.Contains(out, "# Env-driven schedule bootstrap: declare jobs as infrastructure-as-code.") {
		t.Errorf("interleaved comment must be preserved:\n%s", out)
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
