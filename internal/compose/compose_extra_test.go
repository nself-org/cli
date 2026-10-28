package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// ── ResolveImage ──────────────────────────────────────────────────────────────

func TestResolveImage_WithDigest(t *testing.T) {
	old := ImageDigests
	defer func() { ImageDigests = old }()

	ImageDigests = map[string]string{
		"redis:7.2-alpine": "abc123def456",
	}
	got := ResolveImage("redis", "redis:7.2-alpine")
	if !strings.Contains(got, "@sha256:abc123def456") {
		t.Errorf("ResolveImage with digest = %q, missing @sha256:", got)
	}
}

// ── LoadImageDigests ──────────────────────────────────────────────────────────

func TestLoadImageDigests_MissingFile(t *testing.T) {
	dir := t.TempDir()
	old := ImageDigests
	defer func() { ImageDigests = old }()

	// No file — should return nil (not an error).
	if err := LoadImageDigests(dir); err != nil {
		t.Errorf("LoadImageDigests on missing file error: %v", err)
	}
}

func TestLoadImageDigests_ValidFile(t *testing.T) {
	dir := t.TempDir()
	old := ImageDigests
	defer func() { ImageDigests = old }()

	content := `{"postgres":"deadbeef","redis":"cafebabe"}`
	if err := os.WriteFile(filepath.Join(dir, DigestConfigFile), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	if err := LoadImageDigests(dir); err != nil {
		t.Fatalf("LoadImageDigests error: %v", err)
	}
	if ImageDigests["postgres"] != "deadbeef" {
		t.Errorf("ImageDigests[postgres] = %q, want deadbeef", ImageDigests["postgres"])
	}
}

// ── SaveImageDigests ──────────────────────────────────────────────────────────

func TestSaveImageDigests_WriteAndRead(t *testing.T) {
	dir := t.TempDir()
	old := ImageDigests
	defer func() { ImageDigests = old }()

	digests := map[string]string{
		"postgres": "aabbccdd",
		"hasura":   "11223344",
	}
	if err := SaveImageDigests(dir, digests); err != nil {
		t.Fatalf("SaveImageDigests error: %v", err)
	}

	// Reload and verify.
	if err := LoadImageDigests(dir); err != nil {
		t.Fatalf("LoadImageDigests after save error: %v", err)
	}
	if ImageDigests["postgres"] != "aabbccdd" {
		t.Errorf("after reload postgres digest = %q, want aabbccdd", ImageDigests["postgres"])
	}
}

// ── security ──────────────────────────────────────────────────────────────────

func TestDefaultSecurity_DropAll(t *testing.T) {
	sec := DefaultSecurity()
	if len(sec.CapDrop) == 0 || sec.CapDrop[0] != "ALL" {
		t.Errorf("DefaultSecurity.CapDrop = %v, want [ALL]", sec.CapDrop)
	}
	if !sec.ReadOnly {
		t.Error("DefaultSecurity.ReadOnly should be true")
	}
}

func TestPostgresSecurity_HasCapAdd(t *testing.T) {
	sec := PostgresSecurity()
	if len(sec.CapAdd) == 0 {
		t.Error("PostgresSecurity.CapAdd should be non-empty")
	}
	hasIPCLock := false
	for _, c := range sec.CapAdd {
		if c == "IPC_LOCK" {
			hasIPCLock = true
			break
		}
	}
	if !hasIPCLock {
		t.Error("PostgresSecurity should include IPC_LOCK cap")
	}
}

func TestNginxSecurity_HasNetBindService(t *testing.T) {
	sec := NginxSecurity()
	hasNetBind := false
	for _, c := range sec.CapAdd {
		if c == "NET_BIND_SERVICE" {
			hasNetBind = true
			break
		}
	}
	if !hasNetBind {
		t.Error("NginxSecurity should include NET_BIND_SERVICE cap")
	}
}

func TestRedisSecurity_DropAll(t *testing.T) {
	sec := RedisSecurity()
	if len(sec.CapDrop) == 0 || sec.CapDrop[0] != "ALL" {
		t.Errorf("RedisSecurity.CapDrop = %v, want [ALL]", sec.CapDrop)
	}
}

func TestApplySecurityToService_SetsFields(t *testing.T) {
	svc := &ServiceConfig{}
	sec := DefaultSecurity()
	applySecurityToService(svc, sec)

	if !svc.ReadOnly {
		t.Error("applySecurityToService should set ReadOnly=true")
	}
	if len(svc.CapDrop) == 0 {
		t.Error("applySecurityToService should set CapDrop")
	}
	if len(svc.SecurityOpt) == 0 {
		t.Error("applySecurityToService should set SecurityOpt")
	}
}

func TestApplySecurityToService_MergesTmpfs(t *testing.T) {
	svc := &ServiceConfig{Tmpfs: []string{"/data"}}
	sec := ServiceSecurity{Tmpfs: []string{"/tmp", "/data"}} // /data already exists
	applySecurityToService(svc, sec)

	// /data should not be duplicated.
	count := 0
	for _, t2 := range svc.Tmpfs {
		if t2 == "/data" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("/data should appear once in merged tmpfs, got %d times", count)
	}
}

// ── DockerCompose types ───────────────────────────────────────────────────────

func TestAddService_OrderPreserved(t *testing.T) {
	dc := &DockerCompose{
		Services: make(map[string]ServiceConfig),
	}
	dc.AddService("postgres", ServiceConfig{Image: "postgres:16"})
	dc.AddService("hasura", ServiceConfig{Image: "hasura:v2"})
	dc.AddService("auth", ServiceConfig{Image: "auth:0.36"})

	if len(dc.ServiceOrder) != 3 {
		t.Fatalf("ServiceOrder len = %d, want 3", len(dc.ServiceOrder))
	}
	if dc.ServiceOrder[0] != "postgres" {
		t.Errorf("ServiceOrder[0] = %q, want postgres", dc.ServiceOrder[0])
	}
	if dc.ServiceOrder[2] != "auth" {
		t.Errorf("ServiceOrder[2] = %q, want auth", dc.ServiceOrder[2])
	}
}

func TestAddService_ReplaceDoesNotDuplicateOrder(t *testing.T) {
	dc := &DockerCompose{
		Services: make(map[string]ServiceConfig),
	}
	dc.AddService("postgres", ServiceConfig{Image: "postgres:15"})
	dc.AddService("postgres", ServiceConfig{Image: "postgres:16"})

	if len(dc.ServiceOrder) != 1 {
		t.Errorf("ServiceOrder len = %d, want 1 (no duplicate)", len(dc.ServiceOrder))
	}
	if dc.Services["postgres"].Image != "postgres:16" {
		t.Errorf("replaced service image = %q, want postgres:16", dc.Services["postgres"].Image)
	}
}

// ── buildRedisService ─────────────────────────────────────────────────────────

func minimalCfg() *config.Config {
	cfg := &config.Config{
		ProjectName: "testproj",
	}
	cfg.DockerNetwork = "testnet"
	return cfg
}

func TestBuildRedisService_NoPassword(t *testing.T) {
	cfg := minimalCfg()
	cfg.Redis = config.RedisConfig{
		Enabled: true,
		Engine:  "redis", // the default engine is v1.5-dependent (P7-LIVE-19)
		Version: "7-alpine",
		Port:    6379,
		Memory:  "512M",
		CPU:     "0.5",
	}
	g := NewGenerator(cfg)
	svc := g.buildRedisService()

	if !strings.Contains(svc.Image, "redis") {
		t.Errorf("Redis service image = %q, should contain 'redis'", svc.Image)
	}
	if svc.ContainerName != "testproj_redis" {
		t.Errorf("Redis container name = %q, want testproj_redis", svc.ContainerName)
	}
	// Without password, command should not include requirepass.
	if cmdStr, ok := svc.Command.(string); ok {
		if strings.Contains(cmdStr, "requirepass") {
			t.Error("Redis without password should not have requirepass in command")
		}
	}
}

func TestBuildRedisService_WithPassword(t *testing.T) {
	cfg := minimalCfg()
	cfg.Redis = config.RedisConfig{
		Enabled:  true,
		Engine:   "redis",
		Version:  "7-alpine",
		Port:     6379,
		Password: "strongredispassword123",
	}
	g := NewGenerator(cfg)
	svc := g.buildRedisService()

	cmd, ok := svc.Command.([]string)
	if !ok || len(cmd) != 3 || cmd[0] != "sh" || !strings.Contains(cmd[2], `requirepass "$$esc"`) || strings.Contains(cmd[2], "--requirepass") {
		t.Errorf("Redis with password: command = %#v, want sh -c with requirepass from REDISCLI_AUTH", svc.Command)
	}
	if strings.Contains(strings.Join(cmd, " "), cfg.Redis.Password) {
		t.Error("Redis password must not appear in the command (D-0282)")
	}
	if svc.Environment["REDISCLI_AUTH"] != "${REDIS_PASSWORD}" {
		t.Errorf("REDISCLI_AUTH = %q, want ${REDIS_PASSWORD}", svc.Environment["REDISCLI_AUTH"])
	}
}

// ── buildMailpitService ───────────────────────────────────────────────────────

func TestBuildMailpitService_HasPorts(t *testing.T) {
	cfg := minimalCfg()
	cfg.Mailpit = config.MailpitConfig{
		Enabled:  true,
		SMTPPort: 1025,
		UIPort:   8025,
	}
	g := NewGenerator(cfg)
	svc := g.buildMailpitService()

	if len(svc.Ports) == 0 {
		t.Error("Mailpit service should have ports configured")
	}
}

// ── buildAdminService ─────────────────────────────────────────────────────────

func TestBuildAdminService_ContainerName(t *testing.T) {
	cfg := minimalCfg()
	cfg.Admin = config.AdminConfig{
		Enabled: true,
		Port:    3021,
		Version: "latest",
	}
	g := NewGenerator(cfg)
	svc := g.buildAdminService()

	if svc.ContainerName == "" {
		t.Error("Admin service container name should not be empty")
	}
}

// ── buildFunctionsService ────────────────────────────────────────────────────

func TestBuildFunctionsService_HasImage(t *testing.T) {
	cfg := minimalCfg()
	cfg.Functions = config.FunctionsConfig{
		Enabled: true,
		Version: "latest",
		Port:    3008,
	}
	g := NewGenerator(cfg)
	svc := g.buildFunctionsService()

	if svc.Image == "" {
		t.Error("Functions service should have image")
	}
}

// ── buildTypesenseService ─────────────────────────────────────────────────────

func TestBuildTypesenseService_HasDataVolume(t *testing.T) {
	cfg := minimalCfg()
	cfg.Search = config.SearchConfig{
		Enabled: true,
		Engine:  "typesense",
		Port:    8108,
	}
	g := NewGenerator(cfg)
	svc := g.buildTypesenseService()

	if len(svc.Volumes) == 0 {
		t.Error("Typesense service should have volumes")
	}
}

// ── G14-T02: cron plugin docker-compose.plugin.yml passthrough ───────────────

// TestBuildCronPluginComposePassthrough verifies that the cron plugin's
// docker-compose.plugin.yml declares CRON_JOB_* env vars so they are
// passed from the host environment into the cron container.
// This is the key property that enables rebuild-safe env-driven schedule bootstrap:
// if the env vars are not declared in the compose service, Docker Compose drops
// them even if they are set in the operator's .env file.
func TestBuildCronPluginComposePassthrough(t *testing.T) {
	// Read the cron plugin's docker-compose.plugin.yml relative to the repo root.
	// The file is located at plugins/free/cron/docker-compose.plugin.yml.
	// We verify it contains CRON_JOB_* passthrough entries.
	const cronComposeRelPath = "../../../plugins/free/cron/docker-compose.plugin.yml"

	data, err := os.ReadFile(cronComposeRelPath)
	if err != nil {
		// File may not be reachable in this test context — log and skip rather than fail.
		t.Skipf("cron plugin compose file not found at %s (skip in isolated CLI test run): %v",
			cronComposeRelPath, err)
	}

	content := string(data)

	// Must declare at least CRON_JOB_1_SCHEDULE and CRON_JOB_1_COMMAND passthrough.
	checks := []string{
		"CRON_JOB_1_SCHEDULE",
		"CRON_JOB_1_COMMAND",
		"CRON_JOB_2_SCHEDULE",
		"CRON_JOB_2_COMMAND",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("cron docker-compose.plugin.yml missing env passthrough for %q", check)
		}
	}
}

// ── S32-T01: postProcess default resource limits ──────────────────────────────

// TestPostProcess_DefaultLimitsApplied verifies that every long-running service
// receives a Deploy.Resources.Limits block after postProcess runs, even when
// the per-service builder did not set one. This guarantees no container ships
// without CPU+memory caps in production.
func TestPostProcess_DefaultLimitsApplied(t *testing.T) {
	cfg := minimalConfig()
	cfg.DockerLogMaxSize = "10m"
	cfg.DockerLogMaxFile = "3"
	cfg.DockerStopGrace = "30s"
	// Enable a service known to skip per-service limits (mailpit).
	cfg.Mailpit = config.MailpitConfig{Enabled: true}

	g := NewGenerator(cfg)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose: %v", err)
	}

	for name, svc := range dc.Services {
		if name == "meilisearch-init" {
			// Init containers are exempt — they exit before limits matter.
			if svc.Deploy != nil && svc.Deploy.Resources != nil && svc.Deploy.Resources.Limits != nil {
				t.Errorf("init container %q unexpectedly has Deploy.Resources.Limits", name)
			}
			continue
		}
		if svc.Deploy == nil || svc.Deploy.Resources == nil || svc.Deploy.Resources.Limits == nil {
			t.Errorf("service %q missing Deploy.Resources.Limits after postProcess", name)
			continue
		}
		if svc.Deploy.Resources.Limits.Memory == "" {
			t.Errorf("service %q missing memory limit", name)
		}
		if svc.Deploy.Resources.Limits.CPUs == "" {
			t.Errorf("service %q missing CPU limit", name)
		}
		if svc.Deploy.Resources.Reservations == nil || svc.Deploy.Resources.Reservations.Memory == "" {
			t.Errorf("service %q missing memory reservation", name)
		}
	}
}

// TestPostProcess_PidsLimitApplied verifies that every non-init service
// receives pids_limit=100 after postProcess. This is the fork-bomb prevention
// guard required by S43-T12 (Security-Always-Free Doctrine).
func TestPostProcess_PidsLimitApplied(t *testing.T) {
	cfg := minimalConfig()
	cfg.DockerLogMaxSize = "10m"
	cfg.DockerLogMaxFile = "3"
	cfg.DockerStopGrace = "30s"

	g := NewGenerator(cfg)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose: %v", err)
	}

	for name, svc := range dc.Services {
		if name == "meilisearch-init" {
			continue // init containers are exempt
		}
		if svc.Deploy == nil || svc.Deploy.Resources == nil || svc.Deploy.Resources.Limits == nil || svc.Deploy.Resources.Limits.Pids == 0 {
			t.Errorf("service %q has missing or zero pids limit (fork-bomb guard missing)", name)
		} else if svc.Deploy.Resources.Limits.Pids != defaultPidsLimit {
			// Services may override; just confirm it's set.
			t.Logf("service %q has custom pids limit=%d (ok)", name, svc.Deploy.Resources.Limits.Pids)
		}
	}
}

// TestPostProcess_PreservesExplicitLimits verifies that services that already
// set Deploy.Resources.Limits (postgres, hasura, auth) keep their explicit
// values and are not overwritten by defaults.
func TestPostProcess_PreservesExplicitLimits(t *testing.T) {
	cfg := minimalConfig()
	cfg.Postgres.MemLimit = "2g"
	cfg.Postgres.CPULimit = "2.0"
	cfg.DockerLogMaxSize = "10m"
	cfg.DockerLogMaxFile = "3"

	g := NewGenerator(cfg)
	dc, err := g.buildDockerCompose()
	if err != nil {
		t.Fatalf("buildDockerCompose: %v", err)
	}

	pg, ok := dc.Services["postgres"]
	if !ok {
		t.Fatal("postgres service missing")
	}
	if pg.Deploy.Resources.Limits.Memory != "2g" {
		t.Errorf("postgres MemLimit = %q, want 2g", pg.Deploy.Resources.Limits.Memory)
	}
	if pg.Deploy.Resources.Limits.CPUs != "2.0" {
		t.Errorf("postgres CPULimit = %q, want 2.0", pg.Deploy.Resources.Limits.CPUs)
	}
}
