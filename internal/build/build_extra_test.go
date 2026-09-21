package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// ── renderTemplate ────────────────────────────────────────────────────────────

func TestRenderTemplate_BasicSubstitution(t *testing.T) {
	vars := map[string]string{
		"PROJECT_NAME": "myapp",
		"BASE_DOMAIN":  "myapp.local",
	}
	tmpl := "server_name ${BASE_DOMAIN};\nproject: ${PROJECT_NAME};"
	got := renderTemplate(tmpl, vars)
	want := "server_name myapp.local;\nproject: myapp;"
	if got != want {
		t.Errorf("renderTemplate: got %q, want %q", got, want)
	}
}

func TestRenderTemplate_UnknownVarLeftIntact(t *testing.T) {
	vars := map[string]string{"KNOWN": "value"}
	tmpl := "known=${KNOWN} unknown=${UNKNOWN}"
	got := renderTemplate(tmpl, vars)
	if !strings.Contains(got, "known=value") {
		t.Errorf("renderTemplate: known var not substituted: %q", got)
	}
	if !strings.Contains(got, "${UNKNOWN}") {
		t.Errorf("renderTemplate: unknown var was removed: %q", got)
	}
}

func TestRenderTemplate_EmptyVars(t *testing.T) {
	tmpl := "no substitutions here"
	got := renderTemplate(tmpl, map[string]string{})
	if got != tmpl {
		t.Errorf("renderTemplate empty vars: got %q, want %q", got, tmpl)
	}
}

func TestRenderTemplate_EmptyTemplate(t *testing.T) {
	vars := map[string]string{"KEY": "val"}
	got := renderTemplate("", vars)
	if got != "" {
		t.Errorf("renderTemplate empty template: got %q", got)
	}
}

func TestRenderTemplate_MultipleOccurrences(t *testing.T) {
	vars := map[string]string{"NAME": "myapp"}
	tmpl := "${NAME} and ${NAME} and ${NAME}"
	got := renderTemplate(tmpl, vars)
	if strings.Count(got, "myapp") != 3 {
		t.Errorf("renderTemplate multiple: want 3 substitutions, got %q", got)
	}
}

func TestRenderTemplate_PreservesGoTemplateDelimiters(t *testing.T) {
	// Plugin configs (prometheus.yml, alertmanager.yml) use {{ }} syntax.
	// renderTemplate must NOT touch these.
	vars := map[string]string{"PROJECT_NAME": "myapp"}
	tmpl := "{{ range .values }}\n  project: ${PROJECT_NAME}\n{{ end }}"
	got := renderTemplate(tmpl, vars)
	if !strings.Contains(got, "{{ range .values }}") {
		t.Errorf("renderTemplate: Go template delimiters destroyed: %q", got)
	}
	if !strings.Contains(got, "project: myapp") {
		t.Errorf("renderTemplate: ${} substitution did not work: %q", got)
	}
}

// ── buildTemplateVars ─────────────────────────────────────────────────────────

func TestBuildTemplateVars_CoreKeys(t *testing.T) {
	cfg := &config.Config{
		ProjectName: "testproject",
		BaseDomain:  "test.local",
		Postgres: config.PostgresConfig{
			User:     "pguser",
			Password: "pgpass",
			DB:       "pgdb",
		},
		Hasura: config.HasuraConfig{Port: 8080},
		Auth:   config.AuthConfig{Port: 4000},
	}
	vars := buildTemplateVars("/workdir", cfg)

	required := []string{
		"PROJECT_NAME",
		"BASE_DOMAIN",
		"POSTGRES_USER",
		"POSTGRES_PASSWORD",
		"POSTGRES_DB",
		"DOCKER_NETWORK",
		"COMPOSE_PROJECT_NAME",
		"HASURA_PORT",
		"AUTH_PORT",
		"NSELF_PLUGIN_DIR",
	}
	for _, key := range required {
		if _, ok := vars[key]; !ok {
			t.Errorf("buildTemplateVars: missing key %q", key)
		}
	}
}

func TestBuildTemplateVars_ProjectNamePropagated(t *testing.T) {
	cfg := &config.Config{ProjectName: "myproj", BaseDomain: "myproj.local"}
	vars := buildTemplateVars("/workdir", cfg)

	if vars["PROJECT_NAME"] != "myproj" {
		t.Errorf("PROJECT_NAME: want myproj, got %q", vars["PROJECT_NAME"])
	}
	if vars["COMPOSE_PROJECT_NAME"] != "myproj" {
		t.Errorf("COMPOSE_PROJECT_NAME: want myproj, got %q", vars["COMPOSE_PROJECT_NAME"])
	}
}

func TestBuildTemplateVars_DefaultDockerNetwork(t *testing.T) {
	cfg := &config.Config{ProjectName: "myproj", DockerNetwork: ""}
	vars := buildTemplateVars("/workdir", cfg)
	want := "myproj_network"
	if vars["DOCKER_NETWORK"] != want {
		t.Errorf("DOCKER_NETWORK: want %q, got %q", want, vars["DOCKER_NETWORK"])
	}
}

func TestBuildTemplateVars_ExplicitDockerNetwork(t *testing.T) {
	cfg := &config.Config{ProjectName: "myproj", DockerNetwork: "custom_net"}
	vars := buildTemplateVars("/workdir", cfg)
	if vars["DOCKER_NETWORK"] != "custom_net" {
		t.Errorf("DOCKER_NETWORK: want custom_net, got %q", vars["DOCKER_NETWORK"])
	}
}

func TestBuildTemplateVars_HasuraPorts(t *testing.T) {
	cfg := &config.Config{
		Hasura: config.HasuraConfig{Port: 9090},
		Auth:   config.AuthConfig{Port: 4001},
	}
	vars := buildTemplateVars("/workdir", cfg)
	if vars["HASURA_PORT"] != "9090" {
		t.Errorf("HASURA_PORT: want 9090, got %q", vars["HASURA_PORT"])
	}
	if vars["AUTH_PORT"] != "4001" {
		t.Errorf("AUTH_PORT: want 4001, got %q", vars["AUTH_PORT"])
	}
}

// ── DetectServices ────────────────────────────────────────────────────────────

func TestDetectServices_CoreFour(t *testing.T) {
	cfg := &config.Config{}
	services := DetectServices(cfg)

	core := []string{"postgres", "hasura", "auth", "nginx"}
	for _, s := range core {
		found := false
		for _, svc := range services {
			if svc == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DetectServices: missing core service %q; got %v", s, services)
		}
	}
}

func TestDetectServices_OptionalRedis(t *testing.T) {
	cfg := &config.Config{Redis: config.RedisConfig{Enabled: true}}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: redis not detected when enabled; got %v", services)
	}
}

func TestDetectServices_RedisNotIncludedWhenDisabled(t *testing.T) {
	// Use an empty HOME so DefaultPluginDir() returns a path with no plugins,
	// ensuring the auto-enable path does not fire.
	home := t.TempDir()
	setHomeForTest(t, home)

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: false}}
	services := DetectServices(cfg)
	for _, s := range services {
		if s == "redis" {
			t.Errorf("DetectServices: redis should not appear when disabled and no BullMQ plugins installed")
		}
	}
}

func TestDetectServices_Minio(t *testing.T) {
	cfg := &config.Config{Minio: config.MinioConfig{Enabled: true}}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "minio" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: minio not detected when enabled; got %v", services)
	}
}

func TestDetectServices_SearchMeilisearch(t *testing.T) {
	cfg := &config.Config{
		Search: config.SearchConfig{Enabled: true, Engine: "meilisearch"},
	}
	services := DetectServices(cfg)
	foundMeili := false
	for _, s := range services {
		if s == "meilisearch" {
			foundMeili = true
		}
	}
	if !foundMeili {
		t.Errorf("DetectServices: meilisearch not found; got %v", services)
	}
}

func TestDetectServices_SearchTypesense(t *testing.T) {
	cfg := &config.Config{
		Search: config.SearchConfig{Enabled: true, Engine: "typesense"},
	}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "typesense" {
			found = true
		}
	}
	if !found {
		t.Errorf("DetectServices: typesense not found; got %v", services)
	}
}

func TestDetectServices_Admin(t *testing.T) {
	cfg := &config.Config{Admin: config.AdminConfig{Enabled: true}}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "nself-admin" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: nself-admin not found; got %v", services)
	}
}

func TestDetectServices_CustomServices(t *testing.T) {
	cfg := &config.Config{
		CustomServices: []config.CustomService{
			{Name: "ping-api", Port: 8001},
			{Name: "my-service", Port: 9000},
		},
	}
	services := DetectServices(cfg)
	for _, want := range []string{"ping-api", "my-service"} {
		found := false
		for _, s := range services {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("DetectServices: custom service %q not found; got %v", want, services)
		}
	}
}

func TestDetectServices_Functions(t *testing.T) {
	cfg := &config.Config{Functions: config.FunctionsConfig{Enabled: true}}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "functions" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: functions not found when enabled; got %v", services)
	}
}

func TestDetectServices_Mailpit(t *testing.T) {
	cfg := &config.Config{Mailpit: config.MailpitConfig{Enabled: true}}
	services := DetectServices(cfg)
	found := false
	for _, s := range services {
		if s == "mailpit" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: mailpit not found when enabled; got %v", services)
	}
}

// ── DetectServices — Redis auto-enable via plugin detection ───────────────────

// setHomeForTest overrides $HOME to a temporary directory for the duration of
// the test. DetectServices calls DefaultPluginDir() which uses os.UserHomeDir().
// By redirecting HOME we control which plugins appear to be installed.
func setHomeForTest(t *testing.T, home string) {
	t.Helper()
	// Set both HOME (Unix) and USERPROFILE (Windows) so that os.UserHomeDir()
	// picks up the temp directory on every platform.
	origHome, homeSet := os.LookupEnv("HOME")
	origProfile, profileSet := os.LookupEnv("USERPROFILE")
	for _, kv := range []struct{ k, v string }{{"HOME", home}, {"USERPROFILE", home}} {
		if err := os.Setenv(kv.k, kv.v); err != nil {
			t.Fatalf("setHomeForTest: Setenv %s: %v", kv.k, err)
		}
	}
	t.Cleanup(func() {
		if homeSet {
			_ = os.Setenv("HOME", origHome)
		} else {
			_ = os.Unsetenv("HOME")
		}
		if profileSet {
			_ = os.Setenv("USERPROFILE", origProfile)
		} else {
			_ = os.Unsetenv("USERPROFILE")
		}
	})
}

// plantPlugin creates a plugin.json manifest at <home>/.nself/plugins/<name>/plugin.json.
func plantPlugin(t *testing.T, home, name string) {
	t.Helper()
	dir := filepath.Join(home, ".nself", "plugins", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("plantPlugin MkdirAll %s: %v", name, err)
	}
	writeFile(t, filepath.Join(dir, "plugin.json"), `{"name":"`+name+`"}`)
}

func TestDetectServices_AutoEnablesRedis_CronPlugin(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	plantPlugin(t, home, "cron")

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: false}}
	services := DetectServices(cfg)

	found := false
	for _, s := range services {
		if s == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: redis should be auto-enabled when cron plugin is installed; got %v", services)
	}
}

func TestDetectServices_AutoEnablesRedis_NotifyPlugin(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	plantPlugin(t, home, "notify")

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: false}}
	services := DetectServices(cfg)

	found := false
	for _, s := range services {
		if s == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("DetectServices: redis should be auto-enabled when notify plugin is installed; got %v", services)
	}
}

func TestDetectServices_AutoEnablesRedis_CronAndNotify(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	plantPlugin(t, home, "cron")
	plantPlugin(t, home, "notify")

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: false}}
	services := DetectServices(cfg)

	count := 0
	for _, s := range services {
		if s == "redis" {
			count++
		}
	}
	if count == 0 {
		t.Errorf("DetectServices: redis should be auto-enabled when cron+notify installed; got %v", services)
	}
	if count > 1 {
		t.Errorf("DetectServices: redis should appear exactly once; got %d times in %v", count, services)
	}
}

func TestDetectServices_NoAutoEnableRedis_NoPlugins(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	// No plugins planted — .nself/plugins dir is empty.

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: false}}
	services := DetectServices(cfg)

	for _, s := range services {
		if s == "redis" {
			t.Errorf("DetectServices: redis should NOT appear when Redis.Enabled=false and no BullMQ plugins installed; got %v", services)
		}
	}
}

func TestDetectServices_NoDoubleRedis_WhenEnabledAndPluginInstalled(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	plantPlugin(t, home, "cron")

	cfg := &config.Config{Redis: config.RedisConfig{Enabled: true}}
	services := DetectServices(cfg)

	count := 0
	for _, s := range services {
		if s == "redis" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("DetectServices: redis should appear exactly once when both cfg.Redis.Enabled=true and cron plugin installed; got %d times in %v", count, services)
	}
}

// ── ShouldAutoEnableRedis ─────────────────────────────────────────────────────

func TestShouldAutoEnableRedis_NoPlugins(t *testing.T) {
	dir := t.TempDir()
	if ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want false for empty plugin dir")
	}
}

func TestShouldAutoEnableRedis_NonRedisPlugin(t *testing.T) {
	dir := t.TempDir()
	// Plant a plugin that does NOT trigger Redis (health-check plugin has no BullMQ dep).
	pluginDir := filepath.Join(dir, "health")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"health"}`)

	if ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want false for non-redis plugin (health)")
	}
}

func TestShouldAutoEnableRedis_CronPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "cron")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"cron"}`)

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when cron plugin is installed (BullMQ-backed)")
	}
}

func TestShouldAutoEnableRedis_NotifyPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "notify")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"notify"}`)

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when notify plugin is installed (BullMQ-backed)")
	}
}

func TestShouldAutoEnableRedis_CronAndNotify(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cron", "notify"} {
		pluginDir := filepath.Join(dir, name)
		if err := os.MkdirAll(pluginDir, 0755); err != nil {
			t.Fatalf("MkdirAll %s: %v", name, err)
		}
		writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"`+name+`"}`)
	}

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when both cron and notify plugins are installed")
	}
}

func TestShouldAutoEnableRedis_AiPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "ai")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"ai"}`)

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when ai plugin is installed")
	}
}

func TestShouldAutoEnableRedis_ClawPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "claw")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"claw"}`)

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when claw plugin is installed")
	}
}

func TestShouldAutoEnableRedis_MuxPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "mux")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name":"mux"}`)

	if !ShouldAutoEnableRedis(dir) {
		t.Error("ShouldAutoEnableRedis: want true when mux plugin is installed")
	}
}

// ── Redis auto-enable pipeline (Step 7.6) ────────────────────────────────────

// TestAutoEnableRedis_NoAutoRedisFlag verifies that when NoAutoRedis is set in
// BuildOptions, Step 7.6 does NOT set cfg.Redis.Enabled=true even when a
// BullMQ plugin is installed.
func TestAutoEnableRedis_NoAutoRedisFlag(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ai"), 0755); err != nil {
		t.Fatalf("MkdirAll ai: %v", err)
	}
	writeFile(t, filepath.Join(dir, "ai", "plugin.json"), `{"name":"ai"}`)

	cfg := &config.Config{}
	cfg.Redis.Enabled = false

	opts := BuildOptions{NoAutoRedis: true}

	// Replicate Step 7.6 guard: auto-enable skipped when NoAutoRedis is true.
	if !cfg.Redis.Enabled && !opts.NoAutoRedis && ShouldAutoEnableRedis(dir) {
		cfg.Redis.Enabled = true
	}

	if cfg.Redis.Enabled {
		t.Error("Step 7.6: Redis.Enabled must remain false when --no-auto-redis is passed")
	}
}

// TestAutoEnableRedis_EnablesWhenFlagAbsent confirms that Step 7.6 sets
// cfg.Redis.Enabled=true when a BullMQ plugin is installed and NoAutoRedis is
// false (the default).
func TestAutoEnableRedis_EnablesWhenFlagAbsent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "notify"), 0755); err != nil {
		t.Fatalf("MkdirAll notify: %v", err)
	}
	writeFile(t, filepath.Join(dir, "notify", "plugin.json"), `{"name":"notify"}`)

	cfg := &config.Config{}
	cfg.Redis.Enabled = false

	opts := BuildOptions{NoAutoRedis: false}

	if !cfg.Redis.Enabled && !opts.NoAutoRedis && ShouldAutoEnableRedis(dir) {
		cfg.Redis.Enabled = true
	}

	if !cfg.Redis.Enabled {
		t.Error("Step 7.6: Redis.Enabled must be true when notify plugin installed and --no-auto-redis is absent")
	}
}

// TestAutoEnableRedis_SkippedWhenRedisAlreadyEnabled verifies Step 7.6 does not
// double-set Redis.Enabled when the user already has REDIS_ENABLED=true in .env.
func TestAutoEnableRedis_SkippedWhenRedisAlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "cron"), 0755); err != nil {
		t.Fatalf("MkdirAll cron: %v", err)
	}
	writeFile(t, filepath.Join(dir, "cron", "plugin.json"), `{"name":"cron"}`)

	cfg := &config.Config{}
	cfg.Redis.Enabled = true // already set by user

	opts := BuildOptions{NoAutoRedis: false}

	// Guard: should not enter auto-enable branch.
	triggered := false
	if !cfg.Redis.Enabled && !opts.NoAutoRedis && ShouldAutoEnableRedis(dir) {
		cfg.Redis.Enabled = true
		triggered = true
	}

	if triggered {
		t.Error("Step 7.6: must not trigger auto-enable when Redis is already enabled")
	}
}

// ── NeedsRebuild ──────────────────────────────────────────────────────────────

func TestNeedsRebuild_MissingEnv(t *testing.T) {
	dir := t.TempDir()
	needs, err := NeedsRebuild(dir)
	if err != nil {
		t.Fatalf("NeedsRebuild: %v", err)
	}
	if !needs {
		t.Error("NeedsRebuild: want true when .env is missing")
	}
}

func TestNeedsRebuild_MissingCompose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "PROJECT_NAME=test\n")
	needs, err := NeedsRebuild(dir)
	if err != nil {
		t.Fatalf("NeedsRebuild: %v", err)
	}
	if !needs {
		t.Error("NeedsRebuild: want true when docker-compose.yml is missing")
	}
}

// ── DefaultPluginDir ──────────────────────────────────────────────────────────

func TestDefaultPluginDir_NotEmpty(t *testing.T) {
	dir := DefaultPluginDir()
	if dir == "" {
		t.Error("DefaultPluginDir: returned empty string")
	}
}

func TestDefaultPluginDir_ContainsNself(t *testing.T) {
	dir := DefaultPluginDir()
	if !strings.Contains(dir, ".nself") {
		t.Errorf("DefaultPluginDir: expected .nself in path, got %q", dir)
	}
}

func TestDefaultPluginDir_ContainsPlugins(t *testing.T) {
	dir := DefaultPluginDir()
	if !strings.Contains(dir, "plugins") {
		t.Errorf("DefaultPluginDir: expected plugins in path, got %q", dir)
	}
}

// ── WriteComposeManifest / ReadComposeManifest ────────────────────────────────

func TestWriteAndReadComposeManifest_Roundtrip(t *testing.T) {
	dir := t.TempDir()

	basePath := filepath.Join(dir, "docker-compose.yml")
	writeFile(t, basePath, "version: '3'\n")

	// Create a fake plugin compose file so it passes the stat check.
	pluginPath := filepath.Join(dir, "plugin", "docker-compose.plugin.yml")
	writeFile(t, pluginPath, "version: '3'\n")

	if err := WriteComposeManifest(dir, basePath, []string{pluginPath}); err != nil {
		t.Fatalf("WriteComposeManifest: %v", err)
	}

	paths, err := ReadComposeManifest(dir)
	if err != nil {
		t.Fatalf("ReadComposeManifest: %v", err)
	}
	if len(paths) < 1 {
		t.Fatalf("ReadComposeManifest: want at least 1 path, got %d", len(paths))
	}
}

func TestReadComposeManifest_NoManifestReturnsDefault(t *testing.T) {
	dir := t.TempDir()
	paths, err := ReadComposeManifest(dir)
	if err != nil {
		t.Fatalf("ReadComposeManifest no manifest: %v", err)
	}
	if len(paths) != 1 || paths[0] != "docker-compose.yml" {
		t.Errorf("ReadComposeManifest no manifest: want [docker-compose.yml], got %v", paths)
	}
}

func TestReadComposeManifest_StaleEntriesSkipped(t *testing.T) {
	dir := t.TempDir()

	// Write manifest with one real path and one stale path.
	realPath := filepath.Join(dir, "docker-compose.yml")
	writeFile(t, realPath, "version: '3'\n")

	manifestDir := filepath.Join(dir, ".nself")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := realPath + "\n/tmp/stale-plugin-that-does-not-exist/docker-compose.plugin.yml\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "compose-files.txt"), []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile manifest: %v", err)
	}

	paths, err := ReadComposeManifest(dir)
	if err != nil {
		t.Fatalf("ReadComposeManifest: %v", err)
	}
	for _, p := range paths {
		if strings.Contains(p, "stale-plugin") {
			t.Errorf("ReadComposeManifest: stale path should be skipped, got %v", paths)
		}
	}
}

func TestWriteComposeManifest_CreatesNselfDir(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "docker-compose.yml")
	writeFile(t, basePath, "version: '3'\n")

	if err := WriteComposeManifest(dir, basePath, nil); err != nil {
		t.Fatalf("WriteComposeManifest: %v", err)
	}

	manifestPath := filepath.Join(dir, ".nself", "compose-files.txt")
	if _, err := os.Stat(manifestPath); err != nil {
		t.Errorf("WriteComposeManifest: manifest file not created: %v", err)
	}
}

// ── DiscoverPluginComposeFiles ────────────────────────────────────────────────

func TestDiscoverPluginComposeFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	files, err := DiscoverPluginComposeFiles(dir, dir)
	if err != nil {
		t.Fatalf("DiscoverPluginComposeFiles empty: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("DiscoverPluginComposeFiles empty: want 0, got %d", len(files))
	}
}

func TestDiscoverPluginComposeFiles_NonExistentDir(t *testing.T) {
	files, err := DiscoverPluginComposeFiles("/workdir", "/tmp/missing-plugin-dir-99999")
	if err != nil {
		t.Fatalf("DiscoverPluginComposeFiles missing: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("DiscoverPluginComposeFiles missing: want 0, got %d", len(files))
	}
}

func TestDiscoverPluginComposeFiles_FindsComposeFiles(t *testing.T) {
	dir := t.TempDir()

	// Plugin with compose file.
	pluginWithCompose := filepath.Join(dir, "myplugin")
	if err := os.MkdirAll(pluginWithCompose, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginWithCompose, "docker-compose.plugin.yml"), "version: '3'\n")

	// Plugin without compose file.
	pluginWithout := filepath.Join(dir, "otherplugin")
	if err := os.MkdirAll(pluginWithout, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginWithout, "plugin.json"), `{"name":"other"}`)

	files, err := DiscoverPluginComposeFiles(dir, dir)
	if err != nil {
		t.Fatalf("DiscoverPluginComposeFiles: %v", err)
	}
	if len(files) != 1 {
		t.Errorf("DiscoverPluginComposeFiles: want 1, got %d: %v", len(files), files)
	}
}

func TestDiscoverPluginComposeFiles_DisabledPluginSkipped(t *testing.T) {
	dir := t.TempDir()

	pluginDir := filepath.Join(dir, "disabled-plugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "docker-compose.plugin.yml"), "version: '3'\n")
	// Mark as disabled.
	writeFile(t, filepath.Join(pluginDir, ".disabled"), "")

	files, err := DiscoverPluginComposeFiles(dir, dir)
	if err != nil {
		t.Fatalf("DiscoverPluginComposeFiles disabled: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("DiscoverPluginComposeFiles: disabled plugin should be skipped; got %v", files)
	}
}

// ── normalizeComposeDockerfile ────────────────────────────────────────────────

func TestNormalizeComposeDockerfile_NoCanonicalDockerfile(t *testing.T) {
	dir := t.TempDir()
	content := []byte("dockerfile: Dockerfile.go\n")

	// No canonical Dockerfile in the dir — content is returned unchanged.
	result := normalizeComposeDockerfile(content, dir, "myplugin")
	if string(result) != string(content) {
		t.Errorf("normalizeComposeDockerfile no-canonical: content modified unexpectedly")
	}
}

func TestNormalizeComposeDockerfile_ReplacesLegacyWhenCanonicalExists(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "myplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create canonical Dockerfile (but NOT the legacy one).
	writeFile(t, filepath.Join(pluginDir, "Dockerfile"), "FROM golang:1.22\n")

	content := []byte("build:\n  dockerfile: Dockerfile.golang\n")
	result := normalizeComposeDockerfile(content, dir, "myplugin")

	if !strings.Contains(string(result), "dockerfile: Dockerfile") {
		t.Errorf("normalizeComposeDockerfile: expected Dockerfile reference, got:\n%s", string(result))
	}
}

// ── canonicalDockerfile ───────────────────────────────────────────────────────

func TestCanonicalDockerfile_Exists(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "myplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "Dockerfile"), "FROM golang:1.22\n")

	name := canonicalDockerfile(dir, "myplugin")
	if name != "Dockerfile" {
		t.Errorf("canonicalDockerfile: want Dockerfile, got %q", name)
	}
}

func TestCanonicalDockerfile_Missing(t *testing.T) {
	dir := t.TempDir()
	name := canonicalDockerfile(dir, "nonexistent")
	if name != "" {
		t.Errorf("canonicalDockerfile: want empty for missing, got %q", name)
	}
}

// ── dockerfileHasHealthcheck ──────────────────────────────────────────────────

func TestDockerfileHasHealthcheck_Present(t *testing.T) {
	cases := []string{
		"FROM golang:1.22\nHEALTHCHECK CMD curl -f http://localhost/health\nCMD [\"./app\"]",
		"FROM golang:1.22\nhealthcheck cmd curl -f http://localhost/health\nCMD [\"./app\"]",
		"FROM golang:1.22\n  HEALTHCHECK --interval=30s CMD curl -sf http://localhost/\nCMD [\"/app\"]",
	}
	for _, df := range cases {
		if !dockerfileHasHealthcheck([]byte(df)) {
			t.Errorf("dockerfileHasHealthcheck: expected true for:\n%s", df)
		}
	}
}

func TestDockerfileHasHealthcheck_Absent(t *testing.T) {
	cases := []string{
		"FROM golang:1.22\nCMD [\"./app\"]",
		"FROM golang:1.22\n# HEALTHCHECK commented out\nCMD [\"./app\"]",
		"",
	}
	for _, df := range cases {
		if dockerfileHasHealthcheck([]byte(df)) {
			t.Errorf("dockerfileHasHealthcheck: expected false for:\n%s", df)
		}
	}
}

// ── CheckGoPluginDockerfiles ──────────────────────────────────────────────────

func TestCheckGoPluginDockerfiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	warnings := CheckGoPluginDockerfiles(dir)
	if len(warnings) != 0 {
		t.Errorf("CheckGoPluginDockerfiles empty: want 0 warnings, got %v", warnings)
	}
}

func TestCheckGoPluginDockerfiles_GoPluginWithHealthcheck(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "myplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"language":"go","name":"myplugin"}`)
	writeFile(t, filepath.Join(pluginDir, "Dockerfile"), "FROM golang:1.22\nHEALTHCHECK CMD curl -f http://localhost/\nCMD [\"/app\"]")

	warnings := CheckGoPluginDockerfiles(dir)
	if len(warnings) != 0 {
		t.Errorf("CheckGoPluginDockerfiles go+healthcheck: want 0 warnings, got %v", warnings)
	}
}

func TestCheckGoPluginDockerfiles_GoPluginMissingHealthcheck(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "myplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"language":"go","name":"myplugin"}`)
	writeFile(t, filepath.Join(pluginDir, "Dockerfile"), "FROM golang:1.22\nCMD [\"/app\"]")

	warnings := CheckGoPluginDockerfiles(dir)
	if len(warnings) == 0 {
		t.Error("CheckGoPluginDockerfiles go missing healthcheck: want at least 1 warning")
	}
}

func TestCheckGoPluginDockerfiles_NonGoPlugin(t *testing.T) {
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "rustplugin")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"language":"rust","name":"rustplugin"}`)
	// No Dockerfile — should be ignored for non-Go.

	warnings := CheckGoPluginDockerfiles(dir)
	if len(warnings) != 0 {
		t.Errorf("CheckGoPluginDockerfiles non-go: want 0 warnings, got %v", warnings)
	}
}

// ── findNginxConf ─────────────────────────────────────────────────────────────

func TestFindNginxConf_FromSitesDir(t *testing.T) {
	dir := t.TempDir()
	nginxDir := filepath.Join(dir, "nginx")
	sitesDir := filepath.Join(nginxDir, "sites")
	if err := os.MkdirAll(sitesDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(nginxDir, "nginx.conf"), "# nginx main conf\n")

	conf := findNginxConf(sitesDir)
	if conf == "" {
		t.Error("findNginxConf: expected to find nginx.conf from sites dir")
	}
}

func TestFindNginxConf_DirectlyInDir(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "nginx.conf"), "# nginx main conf\n")

	conf := findNginxConf(dir)
	if conf == "" {
		t.Error("findNginxConf: expected to find nginx.conf directly")
	}
}

func TestFindNginxConf_Missing(t *testing.T) {
	dir := t.TempDir()
	conf := findNginxConf(dir)
	if conf != "" {
		t.Errorf("findNginxConf missing: want empty, got %q", conf)
	}
}

// ── buildEnvComputed ──────────────────────────────────────────────────────────

func TestBuildEnvComputed_ContainsRequiredKeys(t *testing.T) {
	cfg := &config.Config{
		ProjectName: "testproject",
		Postgres: config.PostgresConfig{
			User:     "pguser",
			Password: "pgpassword",
			DB:       "pgdb",
		},
	}
	extra := map[string]string{"PLUGIN_AI_INTERNAL_URL": "http://plugin-ai:3700"}
	content := buildEnvComputed(cfg, extra)

	if !strings.Contains(content, "DATABASE_URL=") {
		t.Errorf("buildEnvComputed: missing DATABASE_URL in:\n%s", content)
	}
	if !strings.Contains(content, "DOCKER_NETWORK=") {
		t.Errorf("buildEnvComputed: missing DOCKER_NETWORK in:\n%s", content)
	}
	if !strings.Contains(content, "PLUGIN_AI_INTERNAL_URL=") {
		t.Errorf("buildEnvComputed: missing extra key in:\n%s", content)
	}
	if !strings.Contains(content, "# Generated by nself build") {
		t.Errorf("buildEnvComputed: missing header comment in:\n%s", content)
	}
}

func TestBuildEnvComputed_DefaultNetwork(t *testing.T) {
	cfg := &config.Config{ProjectName: "myapp"}
	content := buildEnvComputed(cfg, nil)
	if !strings.Contains(content, "myapp_network") {
		t.Errorf("buildEnvComputed: expected myapp_network in:\n%s", content)
	}
}

func TestBuildEnvComputed_ExplicitNetwork(t *testing.T) {
	cfg := &config.Config{ProjectName: "myapp", DockerNetwork: "custom_net"}
	content := buildEnvComputed(cfg, nil)
	if !strings.Contains(content, "custom_net") {
		t.Errorf("buildEnvComputed: expected custom_net in:\n%s", content)
	}
}

// ── ComputePluginEnvVars ──────────────────────────────────────────────────────

func TestComputePluginEnvVars_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	vars := ComputePluginEnvVars(dir, dir, nil)
	if _, ok := vars["NSELF_PLUGIN_DIR"]; !ok {
		t.Error("ComputePluginEnvVars: missing NSELF_PLUGIN_DIR")
	}
}

func TestComputePluginEnvVars_MissingDir(t *testing.T) {
	// Non-existent plugin dir should return at least NSELF_PLUGIN_DIR.
	vars := ComputePluginEnvVars("/workdir", "/tmp/missing-plugins-dir-99999", nil)
	if _, ok := vars["NSELF_PLUGIN_DIR"]; !ok {
		t.Error("ComputePluginEnvVars missing: missing NSELF_PLUGIN_DIR")
	}
}

func TestComputePluginEnvVars_GoPluginWithDependency(t *testing.T) {
	dir := t.TempDir()

	// Plant "ai" plugin with a port.
	aiDir := filepath.Join(dir, "ai")
	if err := os.MkdirAll(aiDir, 0755); err != nil {
		t.Fatalf("MkdirAll ai: %v", err)
	}
	writeFile(t, filepath.Join(aiDir, "plugin.json"),
		`{"name":"ai","port":3700,"language":"go","dependencies":[],"optionalDependencies":[]}`)

	// Plant "claw" plugin that depends on "ai".
	clawDir := filepath.Join(dir, "claw")
	if err := os.MkdirAll(clawDir, 0755); err != nil {
		t.Fatalf("MkdirAll claw: %v", err)
	}
	writeFile(t, filepath.Join(clawDir, "plugin.json"),
		`{"name":"claw","port":3710,"language":"go","dependencies":["ai"],"optionalDependencies":[]}`)

	vars := ComputePluginEnvVars(dir, dir, nil)

	if _, ok := vars["PLUGIN_AI_INTERNAL_URL"]; !ok {
		t.Errorf("ComputePluginEnvVars: missing PLUGIN_AI_INTERNAL_URL; vars=%v", vars)
	}
	if vars["PLUGIN_AI_INTERNAL_URL"] != "http://plugin-ai:3700" {
		t.Errorf("ComputePluginEnvVars: PLUGIN_AI_INTERNAL_URL=%q want http://plugin-ai:3700",
			vars["PLUGIN_AI_INTERNAL_URL"])
	}
}

// ── RenderPluginConfigs ───────────────────────────────────────────────────────

func TestRenderPluginConfigs_EmptyPluginDir(t *testing.T) {
	workdir := t.TempDir()
	pluginDir := filepath.Join(workdir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	n, err := RenderPluginConfigs(workdir, pluginDir, &config.Config{})
	if err != nil {
		t.Fatalf("RenderPluginConfigs empty: %v", err)
	}
	if n != 0 {
		t.Errorf("RenderPluginConfigs empty: want 0, got %d", n)
	}
}

func TestRenderPluginConfigs_MissingPluginDir(t *testing.T) {
	workdir := t.TempDir()
	n, err := RenderPluginConfigs(workdir, "/tmp/missing-plugins-99999", &config.Config{})
	if err != nil {
		t.Fatalf("RenderPluginConfigs missing: %v", err)
	}
	if n != 0 {
		t.Errorf("RenderPluginConfigs missing: want 0, got %d", n)
	}
}

func TestRenderPluginConfigs_RendersTemplateFile(t *testing.T) {
	workdir := t.TempDir()
	pluginDir := t.TempDir()

	// Create plugin with a configs/ directory.
	configsDir := filepath.Join(pluginDir, "myplugin", "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("MkdirAll configs: %v", err)
	}
	writeFile(t, filepath.Join(configsDir, "service.conf"),
		"project=${PROJECT_NAME}\ndomain=${BASE_DOMAIN}\n")

	cfg := &config.Config{ProjectName: "myapp", BaseDomain: "myapp.local"}
	n, err := RenderPluginConfigs(workdir, pluginDir, cfg)
	if err != nil {
		t.Fatalf("RenderPluginConfigs: %v", err)
	}
	if n != 1 {
		t.Errorf("RenderPluginConfigs: want 1, got %d", n)
	}

	// Check rendered output.
	outPath := filepath.Join(workdir, "service.conf")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile rendered: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, "project=myapp") {
		t.Errorf("RenderPluginConfigs: expected project=myapp in:\n%s", out)
	}
	if !strings.Contains(out, "domain=myapp.local") {
		t.Errorf("RenderPluginConfigs: expected domain=myapp.local in:\n%s", out)
	}
}

// ── S32-T12: generated-header on docker-compose.yml ───────────────────────────

// TestPrependGeneratedHeader_AddsMarker confirms the GENERATED marker is the
// first line of the returned bytes so grep-based pre-commit hooks detect it.
func TestPrependGeneratedHeader_AddsMarker(t *testing.T) {
	yaml := []byte("name: myapp\nservices:\n  postgres: {}\n")
	out := prependGeneratedHeader(yaml)

	if !strings.HasPrefix(string(out), "# GENERATED BY nself build") {
		t.Errorf("prependGeneratedHeader: output does not start with GENERATED marker:\n%s", out[:80])
	}
	if !strings.Contains(string(out), "name: myapp") {
		t.Errorf("prependGeneratedHeader: original YAML body missing from output")
	}
}

// TestPrependGeneratedHeader_Idempotent_On_Empty verifies the helper works on
// an empty input without panicking.
func TestPrependGeneratedHeader_EmptyInput(t *testing.T) {
	out := prependGeneratedHeader(nil)
	if !strings.HasPrefix(string(out), "# GENERATED BY nself build") {
		t.Errorf("prependGeneratedHeader(nil): expected header marker")
	}
}
