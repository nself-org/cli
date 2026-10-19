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

// Purpose: report whether the minio plugin manifest is installed on this machine.
// Inputs: NSELF_PLUGIN_DIR, else ~/.nself/plugins (the internal/build default).
// Outputs: true when <dir>/minio/plugin.json is a regular file.
// Constraints: package-level seam so tests need no filesystem; compose cannot import build.
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

// Purpose: name the object store custom services should reach.
// Inputs: loaded config and whether the minio plugin is installed.
// Outputs: "seaweedfs", "minio-plugin", "minio-core" (v1.4 only) or "none".
// Constraints: v1.4 follows cfg.Minio.Enabled unchanged; in v1.5 the core store wins over
// the plugin, the plugin counts only with MinIO intent (cfg.Minio.Enabled), and
// missing-plugin MinIO intent is rejected separately (E138).
func ActiveStore(cfg *config.Config, installed bool) string {
	// compat.V15(P7-PLUG-60): optional core MinIO -> core SeaweedFS or MinIO plugin
	if compat.V15() {
		if objectStorageEnabled() {
			return "seaweedfs"
		}
		if installed && cfg.Minio.Enabled {
			return "minio-plugin"
		}
		return "none"
	}
	if cfg.Minio.Enabled {
		return "minio-core"
	}
	return "none"
}

// Purpose: decide whether an installed plugin's compose fragment joins this project's stack.
// Inputs: the plugin directory name and the loaded config.
// Outputs: false only for the minio plugin in v1.5 when MinIO is not this project's active store.
// Constraints: the plugin is installed machine-wide, so intent comes from the project (D-0702);
// every other plugin, and v1.4, merge as before.
func PluginFragmentWanted(name string, cfg *config.Config) bool {
	if name != "minio" {
		return true
	}
	// compat.V15(P7-PLUG-60): the minio fragment merges into every project once installed -> only where ActiveStore picks the plugin
	if compat.V15() {
		return ActiveStore(cfg, true) == "minio-plugin"
	}
	return true
}

// ActiveStoreConnection returns the internal endpoint and configured region for
// the store selected by ActiveStore.
// Inputs: loaded config and whether the MinIO plugin is installed.
// Outputs: endpoint, region and true for an active store; empty values and false otherwise.
// Constraints: endpoints address containers on the project network; host port overrides
// apply only to v1.4 core MinIO, matching the existing custom-service contract.
func ActiveStoreConnection(cfg *config.Config, installed bool) (string, string, bool) {
	switch ActiveStore(cfg, installed) {
	case "seaweedfs":
		return "http://seaweedfs:8333", cfg.Minio.Region, true
	case "minio-plugin":
		return "http://minio:9000", cfg.Minio.Region, true
	case "minio-core":
		port := cfg.Minio.Port
		if port == 0 {
			port = 9000
		}
		return fmt.Sprintf("http://minio:%d", port), cfg.Minio.Region, true
	default:
		return "", "", false
	}
}

// Purpose: read OBJECT_STORAGE_ENABLED from the process env.
// Inputs: env (the config loader has already loaded .env into it).
// Outputs: true for true, 1, yes, on or enabled in any case, matching config getEnvBool.
// Constraints: pure read, no side effects.
func objectStorageEnabled() bool {
	switch strings.ToLower(os.Getenv("OBJECT_STORAGE_ENABLED")) {
	case "true", "1", "yes", "on", "enabled":
		return true
	}
	return false
}

// Purpose: add the object store services the profile and mode call for.
// Inputs: the generator config and profile; the compose being built.
// Outputs: minio (v1.4), or seaweedfs, seaweedfs-init and the seaweedfs_data volume (v1.5).
// Constraints: returns E138 in v1.5 when MinIO is wanted without the plugin, before any write;
// profiles without object storage (ops) never fail or add services.
func (g *Generator) addObjectStorage(dc *DockerCompose) error {
	if !g.profile.Minio {
		return nil
	}
	installed := minioPluginInstalled()
	if err := minioWithoutPlugin(g.cfg, installed); err != nil {
		return err
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

// Purpose: stop a v1.5 build that wants MinIO without the minio plugin before it writes anything.
// Inputs: the loaded config and the build profile ("" means app).
// Outputs: E138 under the same rule as Generate; nil for profiles without object storage.
// Constraints: read-only; internal/build calls it right after config load (before secrets,
// dirs, SSL and nginx), and Generate keeps its own check for other callers.
func ObjectStoragePreflight(cfg *config.Config, profile ProfileName) error {
	if set, _ := ProfileForName(profile); !set.Minio {
		return nil
	}
	return minioWithoutPlugin(cfg, minioPluginInstalled())
}

// Purpose: the E138 rule shared by ObjectStoragePreflight and addObjectStorage.
// Inputs: the loaded config and whether the minio plugin is installed.
// Outputs: E138 in v1.5 when MinIO is wanted and the plugin is missing, else nil.
// Constraints: pure; v1.4 never fails.
func minioWithoutPlugin(cfg *config.Config, installed bool) error {
	// compat.V15(P7-PLUG-60): core MinIO generation -> missing plugin fails before writes
	if compat.V15() && cfg.Minio.Enabled && !installed {
		return errs.New("E138", "MinIO is no longer a core service in v1.5: run nself add minio to keep the minio_data volume, or set OBJECT_STORAGE_ENABLED=true and MINIO_ENABLED=false for a new empty SeaweedFS store, or set MINIO_ENABLED=false if this project does not use MinIO (see Object-Storage.md)")
	}
	return nil
}

// Purpose: render the SeaweedFS server service.
// Inputs: SEAWEEDFS_PORT and SEAWEEDFS_VERSION from env; S3 identity from config.
// Outputs: the service, or an error naming SEAWEEDFS_PORT when it is not 1..65535.
// Constraints: image comes from the lock; credentials are ${...} refs only; S3 publishes on
// 127.0.0.1 alone; the identity file lives on tmpfs and never reaches a project file.
func (g *Generator) buildSeaweedService() (ServiceConfig, error) {
	port := 8333
	if raw := os.Getenv("SEAWEEDFS_PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return ServiceConfig{}, fmt.Errorf("SEAWEEDFS_PORT must be an integer from 1 to 65535")
		}
		port = parsed
	}
	for _, key := range []struct{ name, value string }{
		{"S3_ACCESS_KEY", g.cfg.Minio.S3AccessKey}, {"S3_SECRET_KEY", g.cfg.Minio.S3SecretKey},
	} {
		if problem := objectStorageKeyProblem(key.value); problem != "" {
			return ServiceConfig{}, errs.New("E142", key.name+" is "+problem+": OBJECT_STORAGE_ENABLED=true needs S3_ACCESS_KEY and S3_SECRET_KEY in .env.secrets (no quote, backslash or control character)")
		}
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

// Purpose: check one S3 identity value against the rule the container script enforces.
// Inputs: the value of S3_ACCESS_KEY or S3_SECRET_KEY. Outputs: "" when usable, else a short
// reason ("empty" or "unusable"); the value itself is never returned.
// Constraints: keep in step with seaweedServerScript (empty, quote, backslash, control char).
func objectStorageKeyProblem(value string) string {
	if value == "" {
		return "empty"
	}
	if strings.ContainsAny(value, "\"\\\x7f") || strings.IndexFunc(value, func(r rune) bool { return r < 0x20 }) >= 0 {
		return "unusable"
	}
	return ""
}

// Purpose: render the one-shot bucket bootstrap service.
// Inputs: OBJECT_STORAGE_BUCKETS and SEAWEEDFS_VERSION from env.
// Outputs: a restart-no service that waits for a healthy SeaweedFS.
// Constraints: shares the seaweedfs lock entry and pin; carries no credentials.
func (g *Generator) buildSeaweedInit() ServiceConfig {
	return ServiceConfig{
		Image:         ResolveImage("seaweedfs", ImageRef("seaweedfs", os.Getenv("SEAWEEDFS_VERSION"))),
		ContainerName: g.cfg.ProjectName + "_seaweedfs_init", Restart: "no",
		// Shares the server's network namespace: it reaches the master on 127.0.0.1 and
		// needs no network or port of its own (the master and filer stay closed to the network).
		NetworkMode: "service:seaweedfs",
		DependsOn:   map[string]DepOn{"seaweedfs": {Condition: "service_healthy"}},
		Entrypoint:  []string{"/bin/sh", "-c"}, Command: []string{seaweedBucketScript},
		Environment: map[string]string{"OBJECT_STORAGE_BUCKETS": os.Getenv("OBJECT_STORAGE_BUCKETS")},
		Tmpfs:       []string{"/tmp"},
		Labels: map[string]string{
			"nself.type": "init-container", "nself.service": "seaweedfs", "nself.auto-remove": "true",
		},
	}
}

// Purpose: add S3_* connection vars for the active store to a custom service.
// Inputs: the env map being built and the loaded config.
// Outputs: S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET; nothing with no store.
// Constraints: v1.5 only. ActiveStore names the plugin only with MinIO intent
// (cfg.Minio.Enabled), so a machine-wide plugin never injects empty keys into an unrelated
// project. Secrets are env refs.
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

// Purpose: remove S3_* vars from a custom service env for a profile without object storage.
// Inputs: the env map being built. Outputs: none; mutates env.
// Constraints: v1.5 only; v1.4 keeps injecting whatever cfg.Minio asks for.
func dropStoreEnvVars(env map[string]string) {
	// compat.V15(P7-PLUG-60): S3 vars for every profile -> none for profiles without object storage
	if !compat.V15() {
		return
	}
	for _, key := range []string{"S3_ENDPOINT", "S3_ACCESS_KEY", "S3_SECRET_KEY", "S3_BUCKET"} {
		delete(env, key)
	}
}
