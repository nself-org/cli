package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// coreEnvVars returns the standard env vars injected into every custom service.
//
// By design, custom services receive only this fixed generic set (project
// identity, Postgres, Hasura, Auth, and the optional Redis/Minio blocks) plus
// whatever the service opts into via CS_N_ENV_PASSTHROUGH or CS_N_ENV — never
// the full project .env. This keeps each service's declared surface area
// explicit and auditable instead of silently coupling it to every var another
// part of the stack happens to define. See
// .github/wiki/Config-Custom-Services.md for the user-facing version of this
// decision.
//
// Precedence (lowest to highest): fixed defaults → CS_N_ENV_PASSTHROUGH
// (named allowlist forwarded from the project's resolved env) → CS_N_ENV_FILE
// (envFileVars, pre-loaded by the caller from the dotenv file CS_N_ENV_FILE
// names) → CS_N_ENV (explicit overrides, always win).
//
// envFileVars is nil when the service has no CS_N_ENV_FILE — callers pass
// the map already resolved (rather than a file path) so this function stays
// pure and easy to unit test without touching the filesystem.
func coreEnvVars(cfg *config.Config, svc config.CustomService, envFileVars map[string]string) map[string]string {
	env := fixedCoreEnvVars(cfg, svc)
	addOptionalStoreEnvVars(env, cfg)
	applyEnvPassthrough(env, svc)
	// CS_N_ENV_FILE — merged after passthrough, before CS_N_ENV, so an
	// explicit CS_N_ENV entry still wins on conflict.
	for k, v := range envFileVars {
		env[k] = v
	}
	applyExtraEnv(env, svc)
	return env
}

// fixedCoreEnvVars returns the always-present base set: project identity,
// Postgres, Hasura, Auth, and this service's own identity fields.
func fixedCoreEnvVars(cfg *config.Config, svc config.CustomService) map[string]string {
	return map[string]string{
		"PROJECT_NAME":      cfg.ProjectName,
		"BASE_DOMAIN":       cfg.BaseDomain,
		"ENV":               cfg.Env,
		"POSTGRES_HOST":     "postgres",
		"POSTGRES_PORT":     "5432",
		"POSTGRES_DB":       cfg.Postgres.DB,
		"POSTGRES_USER":     cfg.Postgres.User,
		"POSTGRES_PASSWORD": cfg.Postgres.Password,
		"DATABASE_URL":      cfg.DatabaseURL(),
		// Gap #7: Hasura always listens on 8080 INSIDE its container regardless
		// of the host-mapped cfg.Hasura.Port (compose maps
		// "127.0.0.1:<Hasura.Port>:8080" — see buildHasuraService). Custom
		// services reach Hasura over the same Docker network, so this must be
		// the fixed container port, not the host-exposed one.
		"HASURA_GRAPHQL_ENDPOINT":     "http://hasura:8080/v1/graphql",
		"HASURA_GRAPHQL_ADMIN_SECRET": cfg.Hasura.AdminSecret,
		"AUTH_SERVER_URL":             fmt.Sprintf("http://auth:%d", cfg.Auth.Port),
		"AUTH_CLIENT_URL":             cfg.Auth.ClientURL,
		"SERVICE_NAME":                svc.Name,
		"SERVICE_PORT":                fmt.Sprintf("%d", svc.Port),
		"SERVICE_ROUTE":               svc.Route,
		"TABLE_PREFIX":                svc.TablePrefix,
	}
}

// addOptionalStoreEnvVars adds REDIS_URL / S3_* connection vars when the
// corresponding optional service is enabled on the project.
func addOptionalStoreEnvVars(env map[string]string, cfg *config.Config) {
	if cfg.Redis.Enabled {
		env["REDIS_URL"] = fmt.Sprintf("redis://:%s@redis:%d", cfg.Redis.Password, cfg.Redis.Port)
	}
	if cfg.Minio.Enabled {
		env["S3_ENDPOINT"] = fmt.Sprintf("http://minio:%d", cfg.Minio.Port)
		env["S3_ACCESS_KEY"] = cfg.Minio.RootUser
		env["S3_SECRET_KEY"] = cfg.Minio.RootPassword
		env["S3_BUCKET"] = cfg.Minio.DefaultBuckets
	}
}

// applyEnvPassthrough forwards the CS_N_ENV_PASSTHROUGH allowlist of project
// env var names into env. Applied before CS_N_ENV_FILE/CS_N_ENV so either
// still wins on a name conflict. Names not present in the resolved env are
// silently skipped (not an error) so an allowlist can be shared across
// environments where a var may be optional.
func applyEnvPassthrough(env map[string]string, svc config.CustomService) {
	if svc.EnvPassthrough == "" {
		return
	}
	for _, name := range strings.Split(svc.EnvPassthrough, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if val, ok := os.LookupEnv(name); ok {
			env[name] = val
		}
	}
}

// applyExtraEnv merges CS_N_ENV "KEY=VALUE,KEY=VALUE" pairs into env. Always
// applied last — an explicit CS_N_ENV entry wins over every other source.
func applyExtraEnv(env map[string]string, svc config.CustomService) {
	if svc.ExtraEnv == "" {
		return
	}
	for _, pair := range strings.Split(svc.ExtraEnv, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			env[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
}

// buildHealthcheck renders the Docker healthcheck for a custom service,
// honoring CS_N_HEALTHCHECK. The raw value may be:
//
//   - empty                                — default: GET /health on the
//     service's own port via wget (the prior hardcoded behavior).
//   - a path (e.g. "/auth/health")         — GET that path instead of /health,
//     still via wget on the service's own port.
//   - a full command starting with "CMD"
//     or "CMD-SHELL" (e.g. "CMD-SHELL curl
//     -f http://localhost:4002/status")    — passed through verbatim as the
//     compose healthcheck test, split on whitespace. Use this when the
//     service needs curl, a non-HTTP probe, or a port other than its own.
//   - "disabled" / "none" / "false"
//     (case-insensitive)                   — no healthcheck is emitted.
//
// Without this, a service whose health endpoint isn't literally /health on
// its own port (e.g. auth_server serving /auth/health) is reported unhealthy
// forever regardless of its actual state.
func buildHealthcheck(cs config.CustomService) *Healthcheck {
	raw := strings.TrimSpace(cs.HealthCheck)
	switch strings.ToLower(raw) {
	case "disabled", "none", "false":
		return nil
	}

	var test []string
	if strings.HasPrefix(raw, "CMD") {
		test = strings.Fields(raw)
	} else {
		path := raw
		if path == "" {
			path = "/health"
		} else if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		test = []string{"CMD", "wget", "-qO-", fmt.Sprintf("http://localhost:%d%s", cs.Port, path)}
	}

	return &Healthcheck{
		Test:        test,
		Interval:    "30s",
		Timeout:     "10s",
		Retries:     3,
		StartPeriod: "60s",
	}
}

// buildCustomService returns the service configuration for a user-defined
// custom service (CS_1..CS_10). Each custom service is built from a Dockerfile
// in ./services/{name}/ by default, or from CS_N_PATH when set — unless
// CS_N_IMAGE names a pre-built image, in which case the service pulls that
// image and no build: block is emitted at all (G-013).
//
// Inputs: cs — the parsed CustomService (CS_N_* env vars already validated
// by config.parseCustomServices). g.workDir anchors CS_N_ENV_FILE reads.
// Outputs: the ServiceConfig to emit, or an error if CS_N_ENV_FILE names a
// file that cannot be read/parsed — a build-time failure is preferred over
// silently omitting the vars a service needs (e.g. SMTP credentials).
func (g *Generator) buildCustomService(cs config.CustomService) (ServiceConfig, error) {
	cfg := g.cfg

	var envFileVars map[string]string
	if cs.EnvFile != "" {
		vars, err := loadCustomServiceEnvFile(g.workDir, cs.EnvFile)
		if err != nil {
			return ServiceConfig{}, fmt.Errorf("CS_%d_ENV_FILE: %w", cs.Index, err)
		}
		envFileVars = vars
	}

	svc := ServiceConfig{
		ContainerName: fmt.Sprintf("%s_%s", cfg.ProjectName, cs.Name),
		Restart:       "unless-stopped",
		Networks:      []string{cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"postgres": {Condition: "service_healthy"},
		},
		Environment: coreEnvVars(cfg, cs, envFileVars),
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:%d", cs.Port, cs.Port),
		},
		Volumes:     parseCustomServiceVolumes(cs.Volumes),
		Healthcheck: buildHealthcheck(cs),
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: cs.Memory,
					CPUs:   cs.CPU,
				},
			},
		},
	}

	if cs.Image != "" {
		svc.Image = cs.Image
	} else {
		buildContext := cs.BuildPath
		if buildContext == "" {
			buildContext = defaultCustomServiceBuildContext(cs, g.workDir)
		}
		svc.Build = &BuildConfig{
			Context:    buildContext,
			Dockerfile: "Dockerfile",
		}
	}

	return svc, nil
}

// defaultCustomServiceBuildContext derives the default Dockerfile build
// context for a custom service when CS_N_PATH is not set.
//
// Purpose: a CS_N name may contain underscores (e.g. "ping_api"), but
// config.parseCustomServices sanitizes cs.Name into a hyphenated Docker
// service/container name ("ping-api") — see CustomService.RawName. Before
// this existed, the default build context was built from the sanitized
// cs.Name, so a project with a real `services/ping_api/` directory on disk
// got a build context of `./services/ping-api`, which doesn't exist, and
// `nself build`/`docker compose build` failed with a missing-context error
// even though the service directory was present.
//
// Inputs: cs — the parsed CustomService (cs.RawName is the pre-sanitization
// name, cs.Name the sanitized one; RawName is empty for a CustomService
// built by hand rather than via parseCustomServices, e.g. in tests).
// workDir — the project root used to probe which directory actually exists
// on disk; empty resolves relative to the process's current directory, same
// as loadCustomServiceEnvFile.
//
// Outputs: a "./services/<dir>" build context string.
//
// Constraints: prefers the raw (originally configured) name's directory
// when it's the one that exists on disk, since that's the name the user
// actually typed and the one `nself service create`-style scaffolding
// creates (internal/scaffold/service.go uses the raw name verbatim). Falls
// back to the sanitized name's directory when only that one exists (covers
// a service directory created under the hyphenated name directly). When
// RawName equals Name (no sanitization changed anything, e.g. an
// underscore-free name), no disk probe is needed. When neither directory
// exists, still returns the raw-name path so the resulting "context not
// found" build error names the directory the user actually configured
// rather than a hyphenated variant they never typed.
func defaultCustomServiceBuildContext(cs config.CustomService, workDir string) string {
	rawName := cs.RawName
	if rawName == "" {
		// Backward-compat: a hand-built CustomService (tests, or any future
		// caller that skips config.parseCustomServices) has no RawName —
		// cs.Name is the only name available.
		rawName = cs.Name
	}
	rawContext := fmt.Sprintf("./services/%s", rawName)
	if rawName == cs.Name {
		return rawContext
	}

	if customServiceDirExists(workDir, rawName) {
		return rawContext
	}
	if customServiceDirExists(workDir, cs.Name) {
		return fmt.Sprintf("./services/%s", cs.Name)
	}
	return rawContext
}

// customServiceDirExists reports whether services/<name> exists as a
// directory under workDir (or under the process's current directory when
// workDir is empty, mirroring loadCustomServiceEnvFile's empty-workDir
// fallback).
func customServiceDirExists(workDir, name string) bool {
	relPath := filepath.Join("services", name)
	path := relPath
	if workDir != "" {
		path = filepath.Join(workDir, relPath)
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
