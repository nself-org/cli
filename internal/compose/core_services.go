package compose

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/config"
)

// remoteSchemaEnvVars returns env var map for Hasura remote schema bootstrapping.
func remoteSchemaEnvVars(schemas []config.RemoteSchema) map[string]string {
	env := make(map[string]string, len(schemas)*3)
	for i, rs := range schemas {
		n := i + 1
		key := fmt.Sprintf("HASURA_GRAPHQL_REMOTE_SCHEMA_%d", n)
		env[key+"_NAME"] = rs.Name
		env[key+"_URL"] = rs.URL
		if rs.Headers != "" {
			env[key+"_HEADERS"] = rs.Headers
		}
	}
	return env
}

// buildPostgresService returns the PostgreSQL service configuration.
func (g *Generator) buildPostgresService() ServiceConfig {
	cfg := g.cfg
	image := ResolveImage("postgres", ResolvePostgresImage(cfg.Postgres))
	// Image-aware runtime identity: alpine postgres images run as uid 70 and
	// existing alpine volumes are initialized directly at /var/lib/postgresql/data.
	// Debian-family images (incl. pgvector/pgvector) run as uid 999 with a pgdata
	// subdir. Mismatching these against an existing data volume crash-loops the
	// container (P1 EOP staging incident 2026-06-10).
	user := "999:999"
	env := map[string]string{
		"POSTGRES_USER":             cfg.Postgres.User,
		"POSTGRES_PASSWORD":         cfg.Postgres.Password,
		"POSTGRES_DB":               cfg.Postgres.DB,
		"POSTGRES_HOST_AUTH_METHOD": "scram-sha-256",
		"PGDATA":                    "/var/lib/postgresql/data/pgdata",
	}
	if strings.Contains(image, "alpine") {
		user = "70:70"
		delete(env, "PGDATA")
	}
	// Raise max_connections above the stock 100 so a multi-service stack does not
	// exhaust the connection pool under load (PERF-POOL-01). shared_buffers scales
	// with it. Operators override via POSTGRES_MAX_CONNECTIONS.
	maxConns := cfg.Postgres.MaxConnections
	if maxConns <= 0 {
		maxConns = 200
	}
	command := []string{
		"postgres",
		"-c", fmt.Sprintf("max_connections=%d", maxConns),
		"-c", "shared_buffers=256MB",
	}
	return ServiceConfig{
		Image:         image,
		ContainerName: cfg.ServiceContainerName("postgres"),
		Restart:       "unless-stopped",
		Networks:      []string{cfg.DockerNetwork},
		User:          user,
		Command:       command,
		Environment:   env,
		Volumes: []string{
			"postgres_data:/var/lib/postgresql/data",
			"./postgres/init:/docker-entrypoint-initdb.d:ro",
		},
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:5432", cfg.Postgres.Port),
		},
		ShmSize: "256mb",
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD-SHELL", fmt.Sprintf("pg_isready -U %s", cfg.Postgres.User)},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  5,
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: cfg.Postgres.MemLimit,
					CPUs:   cfg.Postgres.CPULimit,
				},
			},
		},
	}
}

// buildHasuraService returns the Hasura GraphQL engine service configuration.
func (g *Generator) buildHasuraService() (ServiceConfig, error) {
	cfg := g.cfg
	// Use env substitution with safe defaults so that if docker-compose is invoked
	// before the .env file is written (e.g. during initial server setup), Hasura
	// starts with console and dev mode OFF rather than exposing them publicly.
	// The config booleans still control the .env.* values written by nself build.
	_ = cfg.Hasura.Console // value written to .env; docker-compose reads it via substitution
	_ = cfg.Hasura.DevMode // value written to .env; docker-compose reads it via substitution
	consoleStr := "${HASURA_GRAPHQL_ENABLE_CONSOLE:-false}"
	devModeStr := "${HASURA_GRAPHQL_DEV_MODE:-false}"

	jwtSecret, err := config.BuildJWTSecret(cfg)
	if err != nil {
		return ServiceConfig{}, fmt.Errorf("building hasura JWT secret: %w", err)
	}

	env := map[string]string{
		"HASURA_GRAPHQL_DATABASE_URL":      databaseURLRef(cfg),
		"HASURA_GRAPHQL_ADMIN_SECRET":      cfg.Hasura.AdminSecret,
		"HASURA_GRAPHQL_ENABLE_CONSOLE":    consoleStr,
		"HASURA_GRAPHQL_DEV_MODE":          devModeStr,
		"HASURA_GRAPHQL_ENABLE_TELEMETRY":  "false",
		"HASURA_GRAPHQL_CORS_DOMAIN":       cfg.Hasura.CORSDomain,
		"HASURA_GRAPHQL_LOG_LEVEL":         cfg.Hasura.LogLevel,
		"HASURA_GRAPHQL_JWT_SECRET":        jwtSecret,
		"HASURA_GRAPHQL_UNAUTHORIZED_ROLE": "public",
	}

	// Gap #11: Hasura Actions/cron webhooks call back into the functions
	// service to execute their handler code, but the generated Hasura env was
	// missing ACTION_HANDLER_URL entirely — only the functions service itself
	// received a matching endpoint (HASURA_GRAPHQL_ENDPOINT, in
	// buildFunctionsService/coreEnvVars). Wire the same in-network base URL
	// into Hasura's env so url_from_env-based Action/event-trigger handlers
	// can resolve ACTION_HANDLER_URL. Only added when functions is actually
	// enabled — Hasura's own action handler_webhook still works fine without
	// it (Actions can also target absolute URLs directly).
	if cfg.Functions.Enabled {
		functionsPort := cfg.Functions.Port
		if functionsPort == 0 {
			functionsPort = 3008
		}
		env["ACTION_HANDLER_URL"] = fmt.Sprintf("http://functions:%d", functionsPort)
	}

	// Merge remote schema env vars
	for k, v := range remoteSchemaEnvVars(cfg.RemoteSchemas) {
		env[k] = v
	}

	// Passthrough: forward REMOTE_SCHEMA_*, HASURA_EXTRA_*, and the rest of
	// Hasura's own HASURA_GRAPHQL_* namespace from .env to the container.
	// Hasura's url_from_env feature reads Remote Schema URLs directly from the
	// container environment, so REMOTE_SCHEMA_*/HASURA_EXTRA_* must be
	// present at runtime. The HASURA_GRAPHQL_* forward closes a gap where
	// engine tuning vars declared in .env (HASURA_GRAPHQL_ENABLE_ALLOWLIST,
	// _NODE_LIMIT, _DEPTH_LIMIT, _BATCH_SIZE, _LIVE_QUERIES_*, etc.) were
	// recognized by the loader (loader_known_vars_core.go, to suppress
	// unknown-var warnings) but never actually written into the generated
	// compose file — additive only, so the curated values set above (admin
	// secret, JWT secret, console/dev-mode substitution strings, ...) always
	// win and are never clobbered by a raw .env echo of the same key.
	if cfg.Passthrough != nil {
		keys := make([]string, 0, len(cfg.Passthrough))
		for k := range cfg.Passthrough {
			if strings.HasPrefix(k, "REMOTE_SCHEMA_") || strings.HasPrefix(k, "HASURA_EXTRA_") || strings.HasPrefix(k, "HASURA_GRAPHQL_") {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			if _, exists := env[k]; exists {
				continue
			}
			env[k] = cfg.Passthrough[k]
		}
	}

	return ServiceConfig{
		Image:         hasuraImage(cfg.Hasura.Version),
		ContainerName: cfg.ServiceContainerName("hasura"),
		Restart:       "unless-stopped",
		User:          "1001:1001",
		Networks:      []string{cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"postgres": {Condition: "service_healthy"},
		},
		Environment: env,
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:8080", cfg.Hasura.Port),
		},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD", "curl", "-f", "http://localhost:8080/healthz"},
			Interval:    "10s",
			Timeout:     "5s",
			Retries:     5,
			StartPeriod: "15s",
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: cfg.Hasura.MemLimit,
					CPUs:   cfg.Hasura.CPULimit,
				},
			},
		},
	}, nil
}

// HasuraEntry names the image lock entry the hasura service runs by default
// (EPIC D17, P7-LIVE-23): hasura-v14 (graphql-engine v2.44.0) in v1.4, hasura
// (the current v2.51.0) in v1.5. The update plan asks it too.
//
// The metadata catalog version is 48 on both v2.44.0 and v2.51.0 (P7-LIVE-23
// smoke), and v2.44.0 started cleanly on a database v2.51.0 had run on. Rollback
// is still supported only as restore-then-pin-back, by policy: the CLI does not
// check the catalog version (Image-Lock wiki page, Roll back).
func HasuraEntry() string {
	// compat.V15(P7-LIVE-23): hasura runs graphql-engine v2.44.0 (hasura-v14) -> v2.51.0 (hasura)
	if compat.V15() {
		return "hasura"
	}
	return "hasura-v14"
}

// hasuraImage renders HASURA_VERSION. Blank selects the mode default, yielding
// to a project pin. Every nonblank version is explicit; known releases select
// their own lock entry in either mode, and other tags render unpinned.
func hasuraImage(version string) string {
	version = strings.TrimSpace(version)
	entry := HasuraEntry()
	v14, _ := LockedRef("hasura-v14")
	cur, _ := LockedRef("hasura")
	switch {
	case version != "" && v14.IsDefaultVersion(version):
		entry = "hasura-v14"
	case version != "" && cur.IsDefaultVersion(version):
		entry = "hasura"
	}
	if entry == "hasura" {
		return ResolveImage("hasura", ImageRef("hasura", version))
	}
	return ResolveImage("hasura", ImageRef("hasura-v14", version))
}
