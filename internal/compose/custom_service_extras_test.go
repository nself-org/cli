package compose

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: buildCustomService/coreEnvVars coverage for the G-013 additions —
// CS_N_IMAGE (pre-built image instead of Dockerfile build), CS_N_ENV_FILE
// (dotenv-sourced env injection), and CS_N_VOLUMES (extra bind mounts).
// Inputs: config.CustomService fixtures built via testCS() (custom_service_test.go).
// Outputs: none (t.Fatal/t.Error on mismatch).
// Constraints: co-located with custom_service_test.go's existing fixtures;
// reuses minimalConfigWithCS/testCS rather than redefining them.

// ── CS_N_IMAGE ───────────────────────────────────────────────────────────────

// TestBuildCustomService_ImageSkipsBuild verifies that setting CS_N_IMAGE
// emits `image:` and omits `build:` entirely.
func TestBuildCustomService_ImageSkipsBuild(t *testing.T) {
	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg)
	cs := testCS()
	cs.Image = "minio/minio:RELEASE.2024-01-16T16-07-38Z@sha256:abc123"

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	if svc.Image != cs.Image {
		t.Errorf("Image = %q, want %q", svc.Image, cs.Image)
	}
	if svc.Build != nil {
		t.Errorf("Build = %+v, want nil when CS_N_IMAGE is set", svc.Build)
	}
}

// TestBuildCustomService_NoImageStillBuilds is a regression check that the
// default (no CS_N_IMAGE) path is unchanged: it still emits a build: block.
func TestBuildCustomService_NoImageStillBuilds(t *testing.T) {
	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg)
	cs := testCS()

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	if svc.Image != "" {
		t.Errorf("Image = %q, want empty when CS_N_IMAGE is unset", svc.Image)
	}
	if svc.Build == nil {
		t.Fatal("Build is nil, want a build context when CS_N_IMAGE is unset")
	}
}

// ── CS_N_VOLUMES ─────────────────────────────────────────────────────────────

// TestBuildCustomService_VolumesAppended verifies CS_N_VOLUMES entries are
// split and passed through to ServiceConfig.Volumes.
func TestBuildCustomService_VolumesAppended(t *testing.T) {
	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg)
	cs := testCS()
	cs.Volumes = "./email-templates:/app/templates:ro, my_data:/data"

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	want := []string{"./email-templates:/app/templates:ro", "my_data:/data"}
	if len(svc.Volumes) != len(want) {
		t.Fatalf("Volumes = %v, want %v", svc.Volumes, want)
	}
	for i, v := range want {
		if svc.Volumes[i] != v {
			t.Errorf("Volumes[%d] = %q, want %q", i, svc.Volumes[i], v)
		}
	}
}

// TestBuildCustomService_NoVolumesIsNil verifies that an unset CS_N_VOLUMES
// leaves ServiceConfig.Volumes nil (so it's omitted from the generated YAML,
// not emitted as an empty list).
func TestBuildCustomService_NoVolumesIsNil(t *testing.T) {
	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg)
	cs := testCS()

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	if svc.Volumes != nil {
		t.Errorf("Volumes = %v, want nil", svc.Volumes)
	}
}

// ── CS_N_ENV_FILE ────────────────────────────────────────────────────────────

// TestBuildCustomService_EnvFileInjected verifies CS_N_ENV_FILE vars are
// read from disk (resolved against the Generator's workDir) and merged into
// the container environment.
func TestBuildCustomService_EnvFileInjected(t *testing.T) {
	dir := t.TempDir()
	envFile := "smtp.env"
	content := "SMTP_HOST=smtp.example.com\nSMTP_PASS=has,a,comma\n"
	if err := os.WriteFile(filepath.Join(dir, envFile), []byte(content), 0600); err != nil {
		t.Fatalf("writing fixture env file: %v", err)
	}

	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg).WithWorkDir(dir)
	cs := testCS()
	cs.EnvFile = envFile

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	if got := svc.Environment["SMTP_HOST"]; got != "smtp.example.com" {
		t.Errorf("SMTP_HOST = %q, want %q", got, "smtp.example.com")
	}
	// The value containing commas is exactly the case CS_N_ENV (a single
	// comma-joined line) cannot represent safely — proves the env-file path
	// handles it correctly.
	if got := svc.Environment["SMTP_PASS"]; got != "has,a,comma" {
		t.Errorf("SMTP_PASS = %q, want %q", got, "has,a,comma")
	}
}

// TestBuildCustomService_EnvFilePrecedence verifies CS_N_ENV still wins over
// a conflicting CS_N_ENV_FILE value (fixed precedence order documented on
// coreEnvVars).
func TestBuildCustomService_EnvFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	envFile := "extra.env"
	if err := os.WriteFile(filepath.Join(dir, envFile), []byte("SHARED_KEY=from_file\n"), 0600); err != nil {
		t.Fatalf("writing fixture env file: %v", err)
	}

	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg).WithWorkDir(dir)
	cs := testCS()
	cs.EnvFile = envFile
	cs.ExtraEnv = "SHARED_KEY=from_cs_n_env"

	svc, err := g.buildCustomService(cs)
	if err != nil {
		t.Fatalf("buildCustomService returned error: %v", err)
	}
	if got := svc.Environment["SHARED_KEY"]; got != "from_cs_n_env" {
		t.Errorf("SHARED_KEY = %q, want %q (CS_N_ENV must win over CS_N_ENV_FILE)", got, "from_cs_n_env")
	}
}

// TestBuildCustomService_EnvFileMissingErrors verifies a CS_N_ENV_FILE
// naming a nonexistent file fails the build loudly rather than silently
// dropping the vars the service needs.
func TestBuildCustomService_EnvFileMissingErrors(t *testing.T) {
	cfg := minimalConfigWithCS()
	g := NewGenerator(cfg).WithWorkDir(t.TempDir())
	cs := testCS()
	cs.EnvFile = "does-not-exist.env"

	if _, err := g.buildCustomService(cs); err == nil {
		t.Fatal("expected an error for a missing CS_N_ENV_FILE, got nil")
	}
}

// TestGenerate_CustomServiceEnvFileError verifies a bad CS_N_ENV_FILE fails
// the whole Generate() call with a clear error rather than a partial compose.
func TestGenerate_CustomServiceEnvFileError(t *testing.T) {
	cfg := minimalConfigWithCS()
	cs := testCS()
	cs.EnvFile = "does-not-exist.env"
	cfg.CustomServices = []config.CustomService{cs}

	g := NewGenerator(cfg).WithWorkDir(t.TempDir())
	if _, err := g.Generate(); err == nil {
		t.Fatal("expected Generate() to fail on a missing CS_N_ENV_FILE")
	}
}
