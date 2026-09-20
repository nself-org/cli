package compose

// optional_services_apps.go — Admin and Functions optional compose services.
//
// Purpose: build the compose service definitions for the Admin UI and Functions runtime, used by the compose builder alongside optional_services.go, split out for file size.
// Inputs: the loaded Config identifying whether these services are enabled and their settings.
// Outputs: compose service definitions for Admin and Functions.
// Constraints: pure move from optional_services.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
	"log/slog"
)

// buildAdminService returns the nSelf Admin GUI service configuration.
func (g *Generator) buildAdminService() ServiceConfig {
	ac := g.cfg.Admin

	version := ac.Version
	if version == "" {
		version = "latest"
	}
	port := ac.Port
	if port == 0 {
		port = 3021
	}
	if ac.SecretKey == "" {
		slog.Warn("ADMIN_SECRET_KEY is not set — admin UI will start without authentication")
	}
	if ac.PasswordHash == "" {
		slog.Warn("ADMIN_PASSWORD_HASH is not set — admin UI will start without authentication")
	}

	// The admin runs as 1000:1000 but mounts the Docker socket, which is
	// mode 660 owned by root or the docker group. Without the socket's group
	// the container is denied, `docker version` fails inside it, and the admin
	// reports itself unhealthy (503 from /api/health) while still passing its
	// own container healthcheck. Add the group only when we can actually read
	// the socket's gid; see dockerSocketGroup.
	var groupAdd []string
	if gid, ok := dockerSocketGroup(); ok {
		groupAdd = []string{gid}
	}

	// Run as the user that owns the bind-mounted project directory, not a
	// hardcoded 1000:1000. The admin mounts ./ at /workspace read-write and
	// its health check requires W_OK there; a container uid that does not own
	// the directory cannot write to it, so the check fails and /api/health
	// answers 503. This is invisible on macOS, where Docker Desktop remaps
	// ownership for bind mounts, and breaks on Linux whenever the host user is
	// not uid 1000 — GitHub Actions' `runner` is 1001, which is exactly how
	// this surfaced.
	adminUser := "1000:1000"
	if u, ok := hostUser(); ok {
		adminUser = u
	}

	return ServiceConfig{
		Image:         ResolveImage("admin", fmt.Sprintf("%s:%s", AdminImagePath, version)),
		ContainerName: fmt.Sprintf("%s_admin", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		User:          adminUser,
		GroupAdd:      groupAdd,
		Networks:      []string{g.cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"postgres": {Condition: "service_healthy"},
			"hasura":   {Condition: "service_healthy"},
		},
		Environment: map[string]string{
			"NODE_ENV":                    "production",
			"PROJECT_NAME":                g.cfg.ProjectName,
			"BASE_DOMAIN":                 g.cfg.BaseDomain,
			"DATABASE_URL":                fmt.Sprintf("postgres://%s:%s@postgres:5432/%s", g.cfg.Postgres.User, g.cfg.Postgres.Password, g.cfg.Postgres.DB),
			"HASURA_GRAPHQL_ENDPOINT":     "http://hasura:8080/v1/graphql",
			"HASURA_GRAPHQL_ADMIN_SECRET": g.cfg.Hasura.AdminSecret,
			"DOCKER_HOST":                 "unix:///var/run/docker.sock",
			"ADMIN_SECRET_KEY":            ac.SecretKey,
			"ADMIN_PASSWORD_HASH":         ac.PasswordHash,
		},
		Ports: []string{fmt.Sprintf("127.0.0.1:%d:3021", port)},
		Volumes: []string{
			"./:/workspace:rw",
			"nself_admin_data:/app/data",
			"/var/run/docker.sock:/var/run/docker.sock:ro",
		},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD", "curl", "-f", "http://localhost:3021/health"},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     3,
			StartPeriod: "30s",
		},
	}
}

// buildFunctionsService returns the serverless functions service configuration.
// Supports three runtimes: node (default), deno, python.
func (g *Generator) buildFunctionsService() ServiceConfig {
	fc := g.cfg.Functions

	version := fc.Version
	if version == "" {
		version = "latest"
	}
	port := fc.Port
	if port == 0 {
		port = 3008
	}
	memory := fc.Memory
	if memory == "" {
		memory = "256M"
	}
	cpu := fc.CPU
	if cpu == "" {
		cpu = "0.5"
	}
	runtime := fc.Runtime
	if runtime == "" {
		runtime = "node"
	}

	baseEnv := map[string]string{
		"DATABASE_URL":                fmt.Sprintf("postgres://%s:%s@postgres:5432/%s", g.cfg.Postgres.User, g.cfg.Postgres.Password, g.cfg.Postgres.DB),
		"HASURA_GRAPHQL_ENDPOINT":     "http://hasura:8080/v1/graphql",
		"HASURA_GRAPHQL_ADMIN_SECRET": g.cfg.Hasura.AdminSecret,
		"PORT":                        fmt.Sprintf("%d", port),
	}

	var image string
	var command interface{}

	switch runtime {
	case "deno":
		image = "denoland/deno:alpine"
		command = "deno serve --allow-net --allow-read --allow-env /opt/project/server.ts"
	case "python":
		image = "python:3.12-slim"
		command = "sh -c 'pip install -r /opt/project/requirements.txt --quiet && python /opt/project/server.py'"
	default:
		// node is the default (nhost/functions)
		runtime = "node"
		image = ResolveImage("functions", fmt.Sprintf("nhost/functions:%s", version))
		command = nil
	}

	svc := ServiceConfig{
		Image:         image,
		ContainerName: fmt.Sprintf("%s_functions", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		Networks:      []string{g.cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"postgres": {Condition: "service_healthy"},
			"hasura":   {Condition: "service_healthy"},
		},
		Environment: baseEnv,
		Volumes:     []string{"./functions:/opt/project"},
		Ports:       []string{fmt.Sprintf("127.0.0.1:%d:3008", port)},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD-SHELL", fmt.Sprintf("curl -f http://localhost:%d/healthz", port)},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     3,
			StartPeriod: "40s",
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: memory,
					CPUs:   cpu,
				},
			},
		},
	}

	if command != nil {
		svc.Command = command
	}

	_ = runtime // runtime used for image selection above
	return svc
}
