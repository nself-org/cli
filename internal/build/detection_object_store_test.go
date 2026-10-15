package build

import (
	"slices"
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
	"github.com/nself-org/cli/internal/config"
)

// TestDetectServicesSeaweedStore: v1.5 SeaweedFS is a detected service, MinIO is not (D-0683).
func TestDetectServicesSeaweedStore(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("NSELF_PLUGIN_DIR", t.TempDir())
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	got := DetectServices(&config.Config{})
	if !slices.Contains(got, "seaweedfs") || slices.Contains(got, "minio") {
		t.Fatalf("services = %v", got)
	}
	if exp := expectedCoreServices(&config.Config{}); !slices.Contains(exp, "seaweedfs") {
		t.Fatalf("expectedCoreServices = %v", exp)
	}
}
