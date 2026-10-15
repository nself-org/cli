package compose

import (
	"testing"

	"github.com/nself-org/cli/internal/compat/compattest"
)

// TestCurrentStoreFollowsMachineState checks the shared store answer used by routes, SSL,
// backup and doctor (D-0683).
func TestCurrentStoreFollowsMachineState(t *testing.T) {
	compattest.Set(t, true)
	t.Setenv("OBJECT_STORAGE_ENABLED", "true")
	storagePluginSeam(t, false)
	cfg := minimalConfig()
	if got := CurrentStore(cfg); got != "seaweedfs" {
		t.Fatalf("CurrentStore = %q, want seaweedfs", got)
	}
	t.Setenv("OBJECT_STORAGE_ENABLED", "false")
	if got := CurrentStore(cfg); got != "none" {
		t.Fatalf("CurrentStore = %q, want none", got)
	}
}
