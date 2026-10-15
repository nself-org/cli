package config

// Purpose: defer missing-plugin v1.5 MinIO intent to the E138 build preflight.
// Inputs: compat mode and the same plugin manifest paths used by compose.
// Outputs: whether load-time MinIO credential enforcement is applicable.
// Constraints: v1.4 and plugin-backed MinIO retain the credential guard.
// SPORT: D-0685.

import (
	"os"
	"path/filepath"

	"github.com/nself-org/cli/internal/compat"
)

func minioCredentialGuardRequired() bool {
	// compat.V15(D-0685): guard all enabled MinIO credentials -> defer missing-plugin intent to E138
	if !compat.V15() {
		return true
	}
	dir := os.Getenv("NSELF_PLUGIN_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		dir = filepath.Join(home, ".nself", "plugins")
	}
	info, err := os.Stat(filepath.Join(dir, "minio", "plugin.json"))
	return err == nil && !info.IsDir()
}
