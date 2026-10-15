package compose

// Purpose: let code outside the generator ask which object store the project runs.
// Inputs: loaded config, OBJECT_STORAGE_ENABLED, SEAWEEDFS_PORT, the minio plugin manifest.
// Outputs: the active store name, its compose service name and its host S3 port.
// Constraints: v1.4 still resolves to "minio-core" or "none"; consumers must not read
// cfg.Minio.Enabled alone (D-0683).

import (
	"fmt"
	"os"
	"strconv"

	"github.com/nself-org/cli/internal/config"
)

// Purpose: name the object store this machine and project actually use.
// Inputs: loaded config; the minio plugin manifest and OBJECT_STORAGE_ENABLED from the machine.
// Outputs: the ActiveStore name ("seaweedfs", "minio-plugin", "minio-core" or "none").
// Constraints: the shared answer for routes, SSL, backup and doctor.
func CurrentStore(cfg *config.Config) string {
	return ActiveStore(cfg, minioPluginInstalled())
}

// Purpose: name the compose service that serves a store.
// Inputs: a store name from ActiveStore. Outputs: "seaweedfs", "minio" or "" for none.
func StoreService(store string) string {
	switch store {
	case "seaweedfs":
		return "seaweedfs"
	case "minio-core", "minio-plugin":
		return "minio"
	}
	return ""
}

// Purpose: the host port SeaweedFS publishes S3 on (127.0.0.1 only).
// Inputs: SEAWEEDFS_PORT from env, default 8333.
// Outputs: the port, or an error naming SEAWEEDFS_PORT when it is not 1..65535.
func SeaweedPort() (int, error) {
	raw := os.Getenv("SEAWEEDFS_PORT")
	if raw == "" {
		return 8333, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("SEAWEEDFS_PORT must be an integer from 1 to 65535")
	}
	return port, nil
}
