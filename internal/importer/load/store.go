package load

// Purpose: host-side access to the active core or plugin S3 store.
// Inputs: loaded config and storage mode. Outputs: loopback client, never keys in errors.
// Constraints: uses compose's active-store policy and E138 preflight.
import (
	"fmt"
	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/objstore"
	"os"
	"path/filepath"
	"strconv"
)

func Store(cfg *config.Config) (*objstore.Client, error) {
	if err := compose.ObjectStoragePreflight(cfg, ""); err != nil {
		return nil, err
	}
	dir := os.Getenv("NSELF_PLUGIN_DIR")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".nself", "plugins")
	}
	info, err := os.Stat(filepath.Join(dir, "minio", "plugin.json"))
	installed := err == nil && !info.IsDir()
	switch compose.ActiveStore(cfg, installed) {
	case "none":
		return nil, nil
	case "seaweedfs":
		port := 8333
		if raw := os.Getenv("SEAWEEDFS_PORT"); raw != "" {
			n, e := strconv.Atoi(raw)
			if e != nil || n < 1 || n > 65535 {
				return nil, errs.New("E529", "invalid SEAWEEDFS_PORT")
			}
			port = n
		}
		if cfg.Minio.S3AccessKey == "" || cfg.Minio.S3SecretKey == "" {
			return nil, errs.New("E529", "object store credentials are unavailable")
		}
		return &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port), AccessKey: cfg.Minio.S3AccessKey, SecretKey: cfg.Minio.S3SecretKey, Region: cfg.Minio.Region}, nil
	default:
		if cfg.Minio.RootUser == "" || cfg.Minio.RootPassword == "" {
			return nil, errs.New("E529", "object store credentials are unavailable")
		}
		return &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", cfg.Minio.Port), AccessKey: cfg.Minio.RootUser, SecretKey: cfg.Minio.RootPassword, Region: cfg.Minio.Region}, nil
	}
}
