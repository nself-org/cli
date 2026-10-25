package compose

import (
	"fmt"
)

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
		ContainerName: g.cfg.ServiceContainerName("minio"),
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
		ContainerName: g.cfg.ServiceContainerName("mailpit"),
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
