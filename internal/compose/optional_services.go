package compose

import (
	"fmt"
)

// buildRedisService returns the Redis cache/queue service configuration.
// Command and health check are conditional on whether a password is set.
// With a password, no argv carries it (D-0282): compose would interpolate a
// ${REDIS_PASSWORD} argument into the container config, where docker inspect
// shows it. The password reaches the container as REDISCLI_AUTH, which
// redis-cli reads on its own (so the healthcheck needs no -a), and a shell
// expands it for redis-server ($$ survives compose interpolation as $).
func (g *Generator) buildRedisService() ServiceConfig {
	rc := g.cfg.Redis

	version := rc.Version
	if version == "" {
		version = "7-alpine"
	}
	port := rc.Port
	if port == 0 {
		port = 6379
	}
	memory := rc.Memory
	if memory == "" {
		memory = "512M"
	}
	cpu := rc.CPU
	if cpu == "" {
		cpu = "0.5"
	}
	poolSize := rc.PoolSize
	if poolSize == 0 {
		poolSize = 50
	}

	var command interface{}
	var env map[string]string

	if rc.Password != "" {
		env = map[string]string{"REDISCLI_AUTH": secretRef("REDIS_PASSWORD", rc.Password)}
		command = []string{"sh", "-c", fmt.Sprintf(
			`exec redis-server --appendonly yes --protected-mode yes --requirepass "$$REDISCLI_AUTH" --maxclients %d`,
			poolSize,
		)}
	} else {
		command = fmt.Sprintf(
			"redis-server --appendonly yes --protected-mode no --maxclients %d",
			poolSize,
		)
	}

	return ServiceConfig{
		Image:         ResolveImage("redis", ImageRef("redis", version)),
		ContainerName: fmt.Sprintf("%s_redis", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		User:          "999:999",
		Networks:      []string{g.cfg.DockerNetwork},
		Environment:   env,
		Command:       command,
		Volumes:       []string{"redis_data:/data"},
		Ports:         []string{fmt.Sprintf("127.0.0.1:%d:6379", port)},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", "redis-cli", "ping"},
			Interval: "10s",
			Timeout:  "5s",
			Retries:  5,
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
}

// buildMinioService returns the MinIO object storage service configuration.
func (g *Generator) buildMinioService() ServiceConfig {
	mc := g.cfg.Minio

	version := mc.Version
	if version == "" {
		version = "latest"
	}
	port := mc.Port
	if port == 0 {
		port = 9000
	}
	consolePort := mc.ConsolePort
	if consolePort == 0 {
		consolePort = 9001
	}

	return ServiceConfig{
		// The minio entry in images.yaml is registry-qualified (docker.io/pgsty/minio);
		// the Docker Hub minio/minio repository no longer exists (ADR 0028).
		Image:         ResolveImage("minio", ImageRef("minio", version)),
		ContainerName: fmt.Sprintf("%s_minio", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		Networks:      []string{g.cfg.DockerNetwork},
		Environment: map[string]string{
			"MINIO_ROOT_USER":       mc.RootUser,
			"MINIO_ROOT_PASSWORD":   secretRef("MINIO_ROOT_PASSWORD", mc.RootPassword),
			"MINIO_DEFAULT_BUCKETS": mc.DefaultBuckets,
			// Disable extended attributes — required on Docker Desktop macOS
			// (VirtioFS does not support xattrs and MinIO errors with
			// "file access denied: Invalid arguments specified" without this).
			"MINIO_DISABLE_XATTR": "on",
		},
		Command: `server /data --console-address ":9001"`,
		Volumes: []string{"minio_data:/data"},
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:9000", port),
			fmt.Sprintf("127.0.0.1:%d:9001", consolePort),
		},
		Tmpfs: []string{"/tmp"},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD", "mc", "ready", "local"},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     5,
			StartPeriod: "30s",
		},
	}
}

// buildMailpitService returns the Mailpit email testing service configuration.
func (g *Generator) buildMailpitService() ServiceConfig {
	mp := g.cfg.Mailpit

	version := mp.Version
	if version == "" {
		version = "latest"
	}
	smtpPort := mp.SMTPPort
	if smtpPort == 0 {
		smtpPort = 1025
	}
	uiPort := mp.UIPort
	if uiPort == 0 {
		uiPort = 8025
	}
	maxMessages := mp.MaxMessages
	if maxMessages == 0 {
		maxMessages = 500
	}

	env := map[string]string{
		"MP_UI_BIND_ADDR":             "0.0.0.0:8025",
		"MP_SMTP_BIND_ADDR":           "0.0.0.0:1025",
		"MP_SMTP_AUTH_ACCEPT_ANY":     "1",
		"MP_SMTP_AUTH_ALLOW_INSECURE": "1",
		"MP_MAX_MESSAGES":             fmt.Sprintf("%d", maxMessages),
	}
	// Add UI basic auth for non-dev environments when password is configured
	if g.cfg.Env != "dev" && g.cfg.Mailpit.UIPassword != "" {
		// Format: "user:password" — Mailpit supports MP_UI_BASICAUTH for HTTP basic auth
		env["MP_UI_BASICAUTH"] = fmt.Sprintf("%s:%s", g.cfg.Mailpit.UIUser, secretRef("MAILPIT_UI_PASSWORD", g.cfg.Mailpit.UIPassword))
	}

	return ServiceConfig{
		Image:         ResolveImage("mailpit", ImageRef("mailpit", version)),
		ContainerName: fmt.Sprintf("%s_mailpit", g.cfg.ProjectName),
		Restart:       "unless-stopped",
		User:          "1000:1000",
		Networks:      []string{g.cfg.DockerNetwork},
		Environment:   env,
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:1025", smtpPort),
			fmt.Sprintf("127.0.0.1:%d:8025", uiPort),
		},
		Healthcheck: &Healthcheck{
			Test:     []string{"CMD", "nc", "-z", "localhost", "8025"},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
		},
	}
}
