package compose

// Purpose: select the core/plugin object store and render its services.
// Inputs: loaded config, process env, installed MinIO plugin manifest.
// Outputs: compose services and connection env; E138 before build writes.
// Constraints: v1.4 output is unchanged; credentials enter only via env refs.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// minioPluginInstalled is a package seam for filesystem-independent tests.
var minioPluginInstalled = func() bool {
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

// ActiveStore identifies the store; missing-plugin intent is validated separately.
func ActiveStore(cfg *config.Config, installed bool) string {
	// compat.V15(P7-PLUG-60): optional core MinIO -> core SeaweedFS or MinIO plugin
	if compat.V15() {
		if objectStorageEnabled() {
			return "seaweedfs"
		}
		if installed {
			return "minio-plugin"
		}
		return "none"
	}
	if cfg.Minio.Enabled {
		return "minio-core"
	}
	return "none"
}

func objectStorageEnabled() bool {
	switch strings.ToLower(os.Getenv("OBJECT_STORAGE_ENABLED")) {
	case "true", "1", "yes", "on", "enabled":
		return true
	}
	return false
}

func (g *Generator) addObjectStorage(dc *DockerCompose) error {
	if !g.profile.Minio {
		return nil
	}
	installed := minioPluginInstalled()
	// compat.V15(P7-PLUG-60): core MinIO generation -> missing plugin fails before writes
	if compat.V15() && g.cfg.Minio.Enabled && !installed {
		return errs.New("E138", "MinIO is no longer a core service in v1.5: run nself add minio to keep the minio_data volume, or set OBJECT_STORAGE_ENABLED=true and remove MINIO_ENABLED for a new empty SeaweedFS store (see Object-Storage.md)")
	}
	store := ActiveStore(g.cfg, installed)
	switch store {
	case "minio-core":
		dc.AddService("minio", g.buildMinioService())
	case "seaweedfs":
		svc, err := g.buildSeaweedService()
		if err != nil {
			return err
		}
		dc.Volumes["seaweedfs_data"] = VolumeConfig{}
		dc.AddService("seaweedfs", svc)
		dc.AddService("seaweedfs-init", g.buildSeaweedInit())
	}
	return nil
}

func (g *Generator) buildSeaweedService() (ServiceConfig, error) {
	port := 8333
	if raw := os.Getenv("SEAWEEDFS_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return ServiceConfig{}, fmt.Errorf("SEAWEEDFS_PORT must be an integer from 1 to 65535")
		}
		port = parsed
	}
	return ServiceConfig{
		Image:         ResolveImage("seaweedfs", ImageRef("seaweedfs", os.Getenv("SEAWEEDFS_VERSION"))),
		ContainerName: g.cfg.ProjectName + "_seaweedfs", Restart: "unless-stopped",
		Networks: []string{g.cfg.DockerNetwork},
		// The image's seaweed user (uid 1000) owns /data. CapDrop ALL removes root's
		// DAC_OVERRIDE, so root could not write a fresh volume; run as that user.
		User:       seaweedUser,
		Entrypoint: []string{"/bin/sh", "-c"}, Command: []string{seaweedServerScript},
		Environment: map[string]string{
			"S3_ACCESS_KEY": secretRef("S3_ACCESS_KEY", g.cfg.Minio.S3AccessKey),
			"S3_SECRET_KEY": secretRef("S3_SECRET_KEY", g.cfg.Minio.S3SecretKey),
		},
		Volumes: []string{"seaweedfs_data:/data"}, Tmpfs: []string{"/tmp"},
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:8333", port)},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", "wget", "-q", "--spider", "http://127.0.0.1:8333/healthz"},
			Interval: "10s", Timeout: "5s", Retries: 10, StartPeriod: "30s",
		},
	}, nil
}

// seaweedUser is the uid:gid of the seaweed account baked into the SeaweedFS image.
const seaweedUser = "1000:1000"

const seaweedServerScript = `set -eu
for key in "$${S3_ACCESS_KEY:-}" "$${S3_SECRET_KEY:-}"; do
  case "$$key" in
    ''|*'"'*|*'\'*|*[[:cntrl:]]*)
      printf '%s\n' 'S3_ACCESS_KEY and S3_SECRET_KEY must be nonempty and contain no quotes, backslashes or control characters' >&2
      exit 1 ;;
  esac
done
umask 077
printf '{"identities":[{"name":"nself","credentials":[{"accessKey":"%s","secretKey":"%s"}],"actions":["Admin","Read","Write","List","Tagging"]}]}\n' "$$S3_ACCESS_KEY" "$$S3_SECRET_KEY" > /tmp/s3.json
exec weed server -dir=/data -s3 -s3.port=8333 -s3.config=/tmp/s3.json`

func (g *Generator) buildSeaweedInit() ServiceConfig {
	return ServiceConfig{
		Image:         ResolveImage("seaweedfs", ImageRef("seaweedfs", os.Getenv("SEAWEEDFS_VERSION"))),
		ContainerName: g.cfg.ProjectName + "_seaweedfs_init", Restart: "no",
		Networks:   []string{g.cfg.DockerNetwork},
		DependsOn:  map[string]DepOn{"seaweedfs": {Condition: "service_healthy"}},
		Entrypoint: []string{"/bin/sh", "-c"}, Command: []string{seaweedBucketScript},
		Environment: map[string]string{"OBJECT_STORAGE_BUCKETS": os.Getenv("OBJECT_STORAGE_BUCKETS")},
		Tmpfs:       []string{"/tmp"},
		Labels: map[string]string{
			"nself.type": "init-container", "nself.service": "seaweedfs", "nself.auto-remove": "true",
		},
	}
}

const seaweedBucketScript = `set -eu
set -f
[ -n "$${OBJECT_STORAGE_BUCKETS:-}" ] || exit 0
case "$$OBJECT_STORAGE_BUCKETS" in
  ,*|*,|*,,*) printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS: empty bucket name' >&2; exit 1 ;;
esac
IFS=,
for bucket in $$OBJECT_STORAGE_BUCKETS; do
  if [ "$${#bucket}" -lt 3 ] || [ "$${#bucket}" -gt 63 ]; then
    printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: expected 3-63 characters' >&2
    exit 1
  fi
  case "$$bucket" in
    *[!a-z0-9.-]*|[!a-z0-9]*|*[!a-z0-9]|*..*|*.-*|*-.*|xn--*|sthree-*|amzn-s3-demo-*|*-s3alias|*--ol-s3|*.mrap|*--x-s3|*--table-s3)
      printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: expected a lowercase S3 bucket name' >&2
      exit 1 ;;
  esac
  if printf '%s\n' "$$bucket" | awk -F. 'NF == 4 && $$1 ~ /^[0-9]+$$/ && $$2 ~ /^[0-9]+$$/ && $$3 ~ /^[0-9]+$$/ && $$4 ~ /^[0-9]+$$/ { found=1 } END { exit !found }'; then
    printf '%s\n' 'Invalid OBJECT_STORAGE_BUCKETS name: IP address names are not allowed' >&2
    exit 1
  fi
done
for bucket in $$OBJECT_STORAGE_BUCKETS; do
  printf 's3.bucket.list\n' | weed shell -master=seaweedfs:9333 > /tmp/buckets
  if ! awk '{print $$1}' /tmp/buckets | grep -F -x "$$bucket" >/dev/null; then
    if ! printf 's3.bucket.create -name %s\n' "$$bucket" | weed shell -master=seaweedfs:9333; then
      printf 'Failed to create OBJECT_STORAGE_BUCKETS bucket: %s\n' "$$bucket" >&2
      exit 1
    fi
  fi
done`

func addActiveStoreEnvVars(env map[string]string, cfg *config.Config) {
	switch ActiveStore(cfg, minioPluginInstalled()) {
	case "seaweedfs":
		env["S3_ENDPOINT"] = "http://seaweedfs:8333"
		env["S3_ACCESS_KEY"] = secretRef("S3_ACCESS_KEY", cfg.Minio.S3AccessKey)
		env["S3_SECRET_KEY"] = secretRef("S3_SECRET_KEY", cfg.Minio.S3SecretKey)
		env["S3_BUCKET"] = os.Getenv("OBJECT_STORAGE_BUCKETS")
	case "minio-plugin":
		env["S3_ENDPOINT"] = "http://minio:9000"
		env["S3_ACCESS_KEY"] = cfg.Minio.RootUser
		env["S3_SECRET_KEY"] = secretRef("MINIO_ROOT_PASSWORD", cfg.Minio.RootPassword)
		env["S3_BUCKET"] = cfg.Minio.DefaultBuckets
	}
}
