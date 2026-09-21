package compose

// Purpose: the auth and nginx core-service builders for the Docker Compose generator, split out of core_services.go's postgres/hasura builders.
// Inputs: a *Generator holding the resolved *config.Config.
// Outputs: ServiceConfig entries for the auth and nginx services.
// Constraints: split out of core_services.go as a pure move (CLI-R12); no behavior change.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// buildAuthService returns the authentication service configuration.
// Auth depends on BOTH postgres (database) and hasura (metadata validation at startup).
func (g *Generator) buildAuthService() (ServiceConfig, error) {
	cfg := g.cfg
	smtpSecureStr := "false"
	if cfg.Auth.SMTPSecure {
		smtpSecureStr = "true"
	}
	webAuthnStr := "false"
	if cfg.Auth.WebAuthnEnabled {
		webAuthnStr = "true"
	}

	jwtSecret, err := config.BuildJWTSecret(cfg)
	if err != nil {
		return ServiceConfig{}, fmt.Errorf("building auth JWT secret: %w", err)
	}

	env := map[string]string{
		"AUTH_HOST":                                 "0.0.0.0",
		"AUTH_PORT":                                 fmt.Sprintf("%d", cfg.Auth.Port),
		"AUTH_LOG_LEVEL":                            cfg.Auth.LogLevel,
		"AUTH_WEBAUTHN_ENABLED":                     webAuthnStr,
		"DATABASE_URL":                              cfg.DatabaseURL(),
		"AUTH_DATABASE_URL":                         cfg.DatabaseURL(),
		"HASURA_GRAPHQL_DATABASE_URL":               cfg.DatabaseURL(),
		"POSTGRES_HOST":                             cfg.Postgres.Host,
		"POSTGRES_PORT":                             "5432",
		"AUTH_SERVER_URL":                           fmt.Sprintf("http://localhost:%d", cfg.Auth.Port),
		"AUTH_CLIENT_URL":                           cfg.Auth.ClientURL,
		"AUTH_JWT_SECRET":                           cfg.Hasura.JWTKey,
		"AUTH_JWT_TYPE":                             cfg.Hasura.JWTType,
		"HASURA_GRAPHQL_JWT_SECRET":                 jwtSecret,
		"HASURA_GRAPHQL_GRAPHQL_URL":                "http://hasura:8080/v1/graphql",
		"HASURA_GRAPHQL_ADMIN_SECRET":               cfg.Hasura.AdminSecret,
		"AUTH_ACCESS_TOKEN_EXPIRES_IN":              fmt.Sprintf("%d", cfg.Auth.AccessTokenExpiry),
		"AUTH_REFRESH_TOKEN_EXPIRES_IN":             fmt.Sprintf("%d", cfg.Auth.RefreshTokenExpiry),
		"AUTH_SMTP_HOST":                            cfg.Auth.SMTPHost,
		"AUTH_SMTP_PORT":                            fmt.Sprintf("%d", cfg.Auth.SMTPPort),
		"AUTH_SMTP_USER":                            cfg.Auth.SMTPUser,
		"AUTH_SMTP_PASS":                            cfg.Auth.SMTPPass,
		"AUTH_SMTP_SECURE":                          smtpSecureStr,
		"AUTH_SMTP_SENDER":                          cfg.Auth.SMTPSender,
		"AUTH_EMAIL_SIGNIN_EMAIL_VERIFIED_REQUIRED": "false",
	}

	// T06: Add AUTH_DB_* alias vars required by Nhost Auth.
	for k, v := range authPGAliasVars(cfg.Postgres) {
		env[k] = v
	}

	// T07: Inject AUTH_ALLOWED_REDIRECT_URLS for OAuth and magic link support.
	env["AUTH_ALLOWED_REDIRECT_URLS"] = computeAllowedRedirectURLs(cfg)

	// Passthrough: forward AUTH_PROVIDER_* (OAuth provider config) plus the
	// rest of hasura-auth's own AUTH_*/HASURA_AUTH_* namespace from .env.
	// Same curated-list gap as Hasura (see buildHasuraService): vars like
	// AUTH_ANONYMOUS_USERS_ENABLED, AUTH_REQUIRE_EMAIL_VERIFICATION,
	// AUTH_MODE, HASURA_AUTH_SMTP_*, etc. are recognized by the loader
	// (loader_known_vars_core.go) but were never written into the generated
	// compose env. Additive only — the curated values above (JWT secret,
	// SMTP config, allowed redirect URLs, ...) always win.
	if cfg.Passthrough != nil {
		keys := make([]string, 0, len(cfg.Passthrough))
		for k := range cfg.Passthrough {
			if strings.HasPrefix(k, "AUTH_") || strings.HasPrefix(k, "HASURA_AUTH_") {
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
		Image:         ResolveImage("auth", fmt.Sprintf("nhost/hasura-auth:%s", cfg.Auth.Version)),
		ContainerName: fmt.Sprintf("%s_auth", cfg.ProjectName),
		Restart:       "unless-stopped",
		Networks:      []string{cfg.DockerNetwork},
		DependsOn: map[string]DepOn{
			"postgres": {Condition: "service_healthy"},
			"hasura":   {Condition: "service_healthy"},
		},
		Environment: env,
		Ports: []string{
			fmt.Sprintf("127.0.0.1:%d:%d", cfg.Auth.Port, cfg.Auth.Port),
		},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD", "wget", "-qO-", fmt.Sprintf("http://localhost:%d/healthz", cfg.Auth.Port)},
			Interval:    "30s",
			Timeout:     "10s",
			Retries:     3,
			StartPeriod: "30s",
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: cfg.Auth.MemLimit,
					CPUs:   cfg.Auth.CPULimit,
				},
			},
		},
	}, nil
}

// buildNginxService returns the Nginx reverse proxy service configuration.
// It receives the full DockerCompose to compute depends_on from existing services.
func (g *Generator) buildNginxService(dc *DockerCompose) ServiceConfig {
	cfg := g.cfg

	// Build depends_on: always hasura + auth, plus any custom services
	dependsOn := map[string]DepOn{
		"hasura": {Condition: "service_healthy"},
		"auth":   {Condition: "service_healthy"},
	}
	// Add custom services that exist in dc.Services
	for _, cs := range cfg.CustomServices {
		if _, exists := dc.Services[cs.Name]; exists {
			dependsOn[cs.Name] = DepOn{Condition: "service_started"}
		}
	}

	return ServiceConfig{
		Image:         ResolveImage("nginx", "nginx:alpine"),
		ContainerName: fmt.Sprintf("%s_nginx", cfg.ProjectName),
		Restart:       "unless-stopped",
		// No User override on purpose. The nginx.conf this same tool generates
		// declares `user nginx;` on line 4, which requires the MASTER process to
		// start as root: it reads the TLS material and binds 80/443, then drops
		// the workers to the nginx user itself.
		//
		// Forcing User: "101:101" here contradicted that config, and nginx said
		// so on every boot ("the \"user\" directive makes sense only if the master
		// process runs with super-user privileges, ignored"). It also broke TLS
		// outright: ./ssl is bind-mounted, so the certificates keep their host
		// ownership and mode, and uid 101 is neither their owner nor in their
		// group. nginx died with
		//   [emerg] cannot load certificate ".../fullchain.pem": Permission denied
		// and crash-looped, on any host whose invoking uid is not 101 -- which on
		// Linux is every host. Docker Desktop on macOS masks it by remapping
		// ownership, so it only showed up on Linux and in CI.
		//
		// Workers still run unprivileged; the tmpfs uid/gid below stay 101 to
		// match them.
		Networks:  []string{cfg.DockerNetwork},
		DependsOn: dependsOn,
		Environment: map[string]string{
			"BASE_DOMAIN":  cfg.BaseDomain,
			"PROJECT_NAME": cfg.ProjectName,
			"ENV":          cfg.Env,
		},
		Ports: []string{
			fmt.Sprintf("%s:%d:80", cfg.Nginx.BindIP, cfg.Nginx.HTTPPort),
			fmt.Sprintf("%s:%d:443", cfg.Nginx.BindIP, cfg.Nginx.SSLPort),
		},
		Volumes: []string{
			"./nginx/nginx.conf:/etc/nginx/nginx.conf:ro",
			fmt.Sprintf("./%s:/etc/nginx/conf.d:ro", NginxConfDDir),
			fmt.Sprintf("./nginx/conf.d-%s:/etc/nginx/conf.d-%s:ro", cfg.Env, cfg.Env),
			fmt.Sprintf("./%s:/etc/nginx/sites:ro", NginxSitesDir),
			"./nginx/includes:/etc/nginx/includes:ro",
			// Mount target must match nginxtopo.NginxSSLContainerPath — the
			// same constant the generated confs' ssl_certificate directives
			// are rooted at (internal/nginx/generator.go). A production box
			// was found where these had drifted into two independent
			// hand-typed literals; this is now the one place either side
			// would need to change.
			fmt.Sprintf("./ssl:%s:ro", nginxtopo.NginxSSLContainerPath),
		},
		// mode=0777 because two different uids write here. The master starts as
		// root (the generated nginx.conf declares `user nginx;`, which requires
		// it) and creates /var/cache/nginx/client_temp and the pid file at
		// startup; the workers it forks run as uid 101 and write to the same
		// paths afterwards. CapDrop: ALL removes root's write bypass, so a tmpfs
		// owned by 101 alone left the master unable to mkdir and nginx exited:
		//   [emerg] mkdir() "/var/cache/nginx/client_temp" failed (13: Permission denied)
		//
		// The uid/gid stay 101 so the workers own what they create. These are
		// per-container tmpfs mounts holding scratch state, not a boundary
		// between principals, so the permissive mode costs nothing. Widening
		// the capability set to DAC_OVERRIDE instead would grant write bypass
		// across the whole filesystem to fix two scratch directories.
		Tmpfs: []string{
			"/var/cache/nginx:uid=101,gid=101,mode=0777",
			"/var/run:uid=101,gid=101,mode=0777",
		},
		Healthcheck: &Healthcheck{
			Test:        []string{"CMD-SHELL", "wget --no-check-certificate --no-verbose --tries=1 -O /dev/null https://127.0.0.1/health 2>/dev/null || exit 1"},
			Interval:    "30s",
			Timeout:     "5s",
			Retries:     3,
			StartPeriod: "10s",
		},
		Deploy: &DeployConfig{
			Resources: &Resources{
				Limits: &ResourceLimits{
					Memory: "256m",
					CPUs:   "0.5",
				},
			},
		},
	}
}
