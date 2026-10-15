package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// seaweedProject writes a project dir whose .env selects SeaweedFS and nothing else.
func seaweedProject(t *testing.T) string {
	t.Helper()
	compattest.Set(t, true)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	dir := t.TempDir()
	env := "PROJECT_NAME=demo\nBASE_DOMAIN=example.test\nOBJECT_STORAGE_ENABLED=true\nS3_ACCESS_KEY=short\nS3_SECRET_KEY=short\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestDoctorChecksSeaweedStore: with SeaweedFS the doctor checks the S3 secret, not
// MINIO_ROOT_PASSWORD, and lists no MinIO console route or port (D-0683).
func TestDoctorChecksSeaweedStore(t *testing.T) {
	dir := seaweedProject(t)
	var names []string
	for _, r := range checkPasswordStrength(dir, false, false) {
		names = append(names, r.Name)
	}
	joined := strings.Join(names, "|")
	if strings.Contains(joined, "MINIO_ROOT_PASSWORD") || !strings.Contains(joined, "S3_SECRET_KEY") {
		t.Fatalf("password checks = %v, want S3_SECRET_KEY and no MINIO_ROOT_PASSWORD", names)
	}
	for _, r := range checkPortRangeSanity(dir, false) {
		if strings.Contains(r.Name, "MINIO") {
			t.Fatalf("port check %q must not appear for SeaweedFS", r.Name)
		}
	}
}
