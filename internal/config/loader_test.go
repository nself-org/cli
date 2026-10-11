package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── getEnvOr ─────────────────────────────────────────────────────────────────

// TestGetEnvOr_ReturnsDefault verifies that getEnvOr returns the fallback value
// when the env var is unset.
func TestGetEnvOr_ReturnsDefault(t *testing.T) {
	_ = os.Unsetenv("NSELF_TEST_GETENVOOR_UNSET")
	got := getEnvOr("NSELF_TEST_GETENVOOR_UNSET", "default-val")
	if got != "default-val" {
		t.Errorf("getEnvOr() = %q, want %q", got, "default-val")
	}
}

// TestGetEnvOr_ReturnsEnvValue verifies that getEnvOr returns the actual env
// value when the variable is set to a non-empty string.
func TestGetEnvOr_ReturnsEnvValue(t *testing.T) {
	_ = os.Setenv("NSELF_TEST_GETENVOOR_SET", "actual-val")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_TEST_GETENVOOR_SET") })

	got := getEnvOr("NSELF_TEST_GETENVOOR_SET", "default-val")
	if got != "actual-val" {
		t.Errorf("getEnvOr() = %q, want %q", got, "actual-val")
	}
}

// TestGetEnvOr_EmptyStringUsesDefault verifies that an explicitly empty env var
// triggers the fallback (empty string is treated as unset).
func TestGetEnvOr_EmptyStringUsesDefault(t *testing.T) {
	_ = os.Setenv("NSELF_TEST_GETENVOOR_EMPTY", "")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_TEST_GETENVOOR_EMPTY") })

	got := getEnvOr("NSELF_TEST_GETENVOOR_EMPTY", "fallback")
	if got != "fallback" {
		t.Errorf("getEnvOr() = %q, want %q", got, "fallback")
	}
}

// ── getEnvInt ────────────────────────────────────────────────────────────────

// TestGetEnvInt_ReturnsDefault verifies that getEnvInt returns the fallback
// when the env var is unset.
func TestGetEnvInt_ReturnsDefault(t *testing.T) {
	_ = os.Unsetenv("NSELF_TEST_GETENVINT_UNSET")
	got := getEnvInt("NSELF_TEST_GETENVINT_UNSET", 42)
	if got != 42 {
		t.Errorf("getEnvInt() = %d, want %d", got, 42)
	}
}

// TestGetEnvInt_ReturnsIntValue verifies that getEnvInt correctly parses a
// valid integer string.
func TestGetEnvInt_ReturnsIntValue(t *testing.T) {
	_ = os.Setenv("NSELF_TEST_GETENVINT_SET", "8080")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_TEST_GETENVINT_SET") })

	got := getEnvInt("NSELF_TEST_GETENVINT_SET", 0)
	if got != 8080 {
		t.Errorf("getEnvInt() = %d, want %d", got, 8080)
	}
}

// TestGetEnvInt_InvalidValueReturnsDefault verifies that a non-numeric env var
// causes getEnvInt to return the fallback instead of panicking or returning 0.
func TestGetEnvInt_InvalidValueReturnsDefault(t *testing.T) {
	_ = os.Setenv("NSELF_TEST_GETENVINT_INVALID", "not-a-number")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_TEST_GETENVINT_INVALID") })

	got := getEnvInt("NSELF_TEST_GETENVINT_INVALID", 99)
	if got != 99 {
		t.Errorf("getEnvInt() = %d, want fallback %d", got, 99)
	}
}

// TestGetEnvInt_EmptyStringReturnsDefault verifies that an empty string env var
// causes getEnvInt to return the fallback.
func TestGetEnvInt_EmptyStringReturnsDefault(t *testing.T) {
	_ = os.Setenv("NSELF_TEST_GETENVINT_EMPTY", "")
	t.Cleanup(func() { _ = os.Unsetenv("NSELF_TEST_GETENVINT_EMPTY") })

	got := getEnvInt("NSELF_TEST_GETENVINT_EMPTY", 7)
	if got != 7 {
		t.Errorf("getEnvInt() = %d, want fallback %d", got, 7)
	}
}

// ── getEnvBool ───────────────────────────────────────────────────────────────

// TestGetEnvBool_ReturnsDefault verifies that getEnvBool returns the fallback
// when the env var is unset.
func TestGetEnvBool_ReturnsDefault(t *testing.T) {
	_ = os.Unsetenv("NSELF_TEST_GETENVBOOL_UNSET")
	got := getEnvBool("NSELF_TEST_GETENVBOOL_UNSET", true)
	if !got {
		t.Errorf("getEnvBool() = false, want true (fallback)")
	}
}

// TestGetEnvBool_TrueValues verifies that all accepted truthy strings are
// parsed as true.
func TestGetEnvBool_TrueValues(t *testing.T) {
	trueVals := []string{"true", "1", "yes", "on", "enabled", "TRUE", "YES"}
	for _, v := range trueVals {
		_ = os.Setenv("NSELF_TEST_GETENVBOOL_TRUE", v)
		got := getEnvBool("NSELF_TEST_GETENVBOOL_TRUE", false)
		if !got {
			t.Errorf("getEnvBool(%q) = false, want true", v)
		}
	}
	_ = os.Unsetenv("NSELF_TEST_GETENVBOOL_TRUE")
}

// TestGetEnvBool_FalseValues verifies that non-truthy strings are parsed as false.
func TestGetEnvBool_FalseValues(t *testing.T) {
	falseVals := []string{"false", "0", "no", "off", "disabled", "nope"}
	for _, v := range falseVals {
		_ = os.Setenv("NSELF_TEST_GETENVBOOL_FALSE", v)
		got := getEnvBool("NSELF_TEST_GETENVBOOL_FALSE", true)
		if got {
			t.Errorf("getEnvBool(%q) = true, want false", v)
		}
	}
	_ = os.Unsetenv("NSELF_TEST_GETENVBOOL_FALSE")
}

// ── ApplyDefaults ────────────────────────────────────────────────────────────

// TestApplyDefaults_CoreDefaults verifies that an empty Config gets the
// expected project name, domain, and env defaults.
func TestApplyDefaults_CoreDefaults(t *testing.T) {
	cfg, err := ApplyDefaults(&Config{})
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	if cfg.ProjectName != "myproject" {
		t.Errorf("ProjectName = %q, want %q", cfg.ProjectName, "myproject")
	}
	if cfg.BaseDomain != "local.nself.org" {
		t.Errorf("BaseDomain = %q, want %q", cfg.BaseDomain, "local.nself.org")
	}
	if cfg.Env != "dev" {
		t.Errorf("Env = %q, want %q", cfg.Env, "dev")
	}
	// Gap #5: AppName must default to empty (bare nginx subdomain scheme) —
	// ApplyDefaults deliberately never fills it in, so existing single-app
	// deployments (ummat, unity) that never set APP_NAME are unaffected.
	if cfg.AppName != "" {
		t.Errorf("AppName = %q, want empty (opt-in only, no default)", cfg.AppName)
	}
}

// TestApplyDefaults_PostgresDefaults verifies that the postgres service gets
// its canonical default port, user, and database.
func TestApplyDefaults_PostgresDefaults(t *testing.T) {
	cfg, err := ApplyDefaults(&Config{})
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	if cfg.Postgres.Port != 5432 {
		t.Errorf("Postgres.Port = %d, want %d", cfg.Postgres.Port, 5432)
	}
	if cfg.Postgres.User != "postgres" {
		t.Errorf("Postgres.User = %q, want %q", cfg.Postgres.User, "postgres")
	}
	if cfg.Postgres.DB != "nself" {
		t.Errorf("Postgres.DB = %q, want %q", cfg.Postgres.DB, "nself")
	}
	if cfg.Postgres.Host != "postgres" {
		t.Errorf("Postgres.Host = %q, want %q", cfg.Postgres.Host, "postgres")
	}
}

// TestApplyDefaults_AuthDefaults verifies that the auth service gets port 4000
// as its default (not Hasura's 8080).
func TestApplyDefaults_AuthDefaults(t *testing.T) {
	cfg, err := ApplyDefaults(&Config{})
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	if cfg.Auth.Port != 4000 {
		t.Errorf("Auth.Port = %d, want %d", cfg.Auth.Port, 4000)
	}
}

// TestApplyDefaults_NginxDefaults verifies HTTP and SSL port defaults.
func TestApplyDefaults_NginxDefaults(t *testing.T) {
	cfg, err := ApplyDefaults(&Config{})
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}

	if cfg.Nginx.HTTPPort != 80 {
		t.Errorf("Nginx.HTTPPort = %d, want %d", cfg.Nginx.HTTPPort, 80)
	}
	if cfg.Nginx.SSLPort != 443 {
		t.Errorf("Nginx.SSLPort = %d, want %d", cfg.Nginx.SSLPort, 443)
	}
}

// TestApplyDefaults_DoesNotOverrideExistingValues verifies that ApplyDefaults
// never clobbers values that are already set.
func TestApplyDefaults_DoesNotOverrideExistingValues(t *testing.T) {
	cfg := &Config{
		ProjectName: "myapp",
		BaseDomain:  "example.com",
		Env:         "prod",
		// Provide strong MinIO credentials to satisfy the prod guard; the test
		// is only asserting that ProjectName/BaseDomain/Env are not overwritten.
		Minio: MinioConfig{
			RootUser:     "prod-minio-user",
			RootPassword: "str0ngProdP@ssword!!",
		},
	}
	var applyErr error
	cfg, applyErr = ApplyDefaults(cfg)
	if applyErr != nil {
		t.Fatalf("ApplyDefaults() error: %v", applyErr)
	}

	if cfg.ProjectName != "myapp" {
		t.Errorf("ProjectName overwritten: got %q, want %q", cfg.ProjectName, "myapp")
	}
	if cfg.BaseDomain != "example.com" {
		t.Errorf("BaseDomain overwritten: got %q, want %q", cfg.BaseDomain, "example.com")
	}
	if cfg.Env != "prod" {
		t.Errorf("Env overwritten: got %q, want %q", cfg.Env, "prod")
	}
}

// TestApplyDefaults_PostgresPasswordGenerated verifies that a missing
// POSTGRES_PASSWORD is filled with a non-empty generated value.
func TestApplyDefaults_PostgresPasswordGenerated(t *testing.T) {
	cfg, err := ApplyDefaults(&Config{})
	if err != nil {
		t.Fatalf("ApplyDefaults() error: %v", err)
	}
	if cfg.Postgres.Password == "" {
		t.Error("Postgres.Password should be auto-generated, got empty string")
	}
}

// ── normalizeEnv ─────────────────────────────────────────────────────────────

// TestNormalizeEnv_Aliases verifies that env name aliases are normalized to
// their canonical short forms.
func TestNormalizeEnv_Aliases(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"development", "dev"},
		{"develop", "dev"},
		{"devel", "dev"},
		{"dev", "dev"},
		{"production", "prod"},
		{"prod", "prod"},
		{"stage", "staging"},
		{"staging", "staging"},
		{"test", "test"},
		{"DEVELOPMENT", "dev"},
		{"PRODUCTION", "prod"},
	}

	for _, c := range cases {
		got := normalizeEnv(c.input)
		if got != c.want {
			t.Errorf("normalizeEnv(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestLoadDiskDotenvErrorCarriesNoValue proves a bad file on disk reports
// "invalid dotenv syntax in <path>" and never the parser text, which quotes
// the offending line (a secret value).
func TestLoadDiskDotenvErrorCarriesNoValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.secrets")
	if err := os.WriteFile(path, []byte(fakeBadLine), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadWithOptions(dir, LoadOptions{})
	want := "invalid dotenv syntax in " + path
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
	if strings.Contains(err.Error(), "s3cr3t-FAKE-0000") {
		t.Fatalf("error holds the secret: %v", err)
	}
}
