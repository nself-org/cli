package config

// loader_parse_env_core.go — env var -> Config field mapping: Core, Postgres,
// Hasura, Auth, Nginx, SSL, WAF, Redis. Split from loader_parse_env.go
// (T-P6-E2-W1-S1-T3).
// Inputs:  os.Environ, read into the cfg passed in by parseEnvToConfig.
// Outputs: none — mutates cfg in place.
// Constraints: pure os.Getenv reads only, same rules as loader_parse_env.go.

import "os"

// parseEnvCore fills the Core/Postgres/Hasura/Auth/Nginx/SSL/WAF/Redis fields.
func parseEnvCore(cfg *Config) {
	// ── Core ─────────────────────────────────────────────────────────
	cfg.ProjectName = os.Getenv("PROJECT_NAME")
	cfg.BaseDomain = os.Getenv("BASE_DOMAIN")
	if cfg.BaseDomain == "" {
		cfg.BaseDomain = os.Getenv("PROJECT_DOMAIN")
	}
	cfg.Env = normalizeEnv(getEnvOr("ENV", "dev"))
	cfg.ProjectDescription = os.Getenv("PROJECT_DESCRIPTION")
	cfg.AdminEmail = os.Getenv("ADMIN_EMAIL")
	cfg.DBEnvSeeds = getEnvBool("DB_ENV_SEEDS", true)
	// APP_NAME: opt-in subdomain prefix (gap #5). Empty by default, preserving
	// the bare "api.{BASE_DOMAIN}" scheme for existing single-app deployments.
	cfg.AppName = os.Getenv("APP_NAME")

	// ── PostgreSQL ───────────────────────────────────────────────────
	cfg.Postgres = PostgresConfig{
		Version:    os.Getenv("POSTGRES_VERSION"),
		Image:      os.Getenv("POSTGRES_IMAGE"),
		Host:       os.Getenv("POSTGRES_HOST"),
		Port:       getEnvInt("POSTGRES_PORT", 0),
		DB:         os.Getenv("POSTGRES_DB"),
		User:       os.Getenv("POSTGRES_USER"),
		Password:   os.Getenv("POSTGRES_PASSWORD"),
		Extensions: parseExtensionList(getEnvOr("POSTGRES_EXTENSIONS", "uuid-ossp,pgcrypto,pg_trgm")),
		ExposePort: os.Getenv("POSTGRES_EXPOSE_PORT"),
		MemLimit:   os.Getenv("POSTGRES_MEM_LIMIT"),
		CPULimit:   os.Getenv("POSTGRES_CPU_LIMIT"),
	}
	// PGVECTOR_ENABLED (written by `nself init`, default true — see
	// internal/setup/setup.go's pgvectorEnabled) is a separate convenience
	// toggle from POSTGRES_EXTENSIONS, and nothing previously connected the
	// two: internal/compose/images.go's pgvector image selection keys off
	// Extensions containing "pgvector", so a fresh project's .env (which only
	// ever wrote PGVECTOR_ENABLED=true) silently built on plain
	// postgres:16-alpine instead of pgvector/pgvector:pg16 (E2E golden path
	// step 13, claw plugin: CREATE EXTENSION vector failed). Purely additive:
	// an explicit POSTGRES_EXTENSIONS without "pgvector" is unaffected unless
	// PGVECTOR_ENABLED=true is also set, matching the toggle's documented
	// intent ("Skip pgvector extension ... sets PGVECTOR_ENABLED=false").
	if getEnvBool("PGVECTOR_ENABLED", false) && !hasExtensionCI(cfg.Postgres.Extensions, "pgvector") {
		cfg.Postgres.Extensions = append(cfg.Postgres.Extensions, "pgvector")
	}

	// ── Hasura ───────────────────────────────────────────────────────
	cfg.Hasura = HasuraConfig{
		Version:     os.Getenv("HASURA_VERSION"),
		AdminSecret: os.Getenv("HASURA_GRAPHQL_ADMIN_SECRET"),
		JWTKey:      os.Getenv("HASURA_JWT_KEY"),
		JWTType:     os.Getenv("HASURA_JWT_TYPE"),
		Console:     getEnvBool("HASURA_GRAPHQL_ENABLE_CONSOLE", false),
		DevMode:     getEnvBool("HASURA_GRAPHQL_DEV_MODE", false),
		CORSDomain:  os.Getenv("HASURA_GRAPHQL_CORS_DOMAIN"),
		Route:       os.Getenv("HASURA_ROUTE"),
		Port:        getEnvInt("HASURA_PORT", 0),
		MemLimit:    os.Getenv("HASURA_MEM_LIMIT"),
		CPULimit:    os.Getenv("HASURA_CPU_LIMIT"),
		LogLevel:    os.Getenv("HASURA_GRAPHQL_LOG_LEVEL"),
	}
	// HASURA_DEV_MODE backward-compat alias: v1 used HASURA_DEV_MODE, v2 uses HASURA_GRAPHQL_DEV_MODE.
	// Only apply alias if HASURA_GRAPHQL_DEV_MODE was not explicitly set.
	if alias := os.Getenv("HASURA_DEV_MODE"); alias != "" {
		if _, explicitly := os.LookupEnv("HASURA_GRAPHQL_DEV_MODE"); !explicitly {
			cfg.Hasura.DevMode = alias == "true" || alias == "1" || alias == "yes"
		}
	}
	// JWT-ALGO-01 / gap #4: populate cfg.Hasura.JWTKey/JWTType from whatever
	// source already has the value, so a previously-generated (or user-supplied)
	// key survives every rebuild — including --force — instead of ApplyDefaults
	// generating a brand new one because HASURA_JWT_KEY specifically was unset.
	// Priority (highest first): HASURA_JWT_KEY/HASURA_JWT_TYPE (already applied
	// above) > HASURA_GRAPHQL_JWT_SECRET JSON (the full persisted secret written
	// to .env.secrets by persistGeneratedSecrets) > AUTH_JWT_SECRET/AUTH_JWT_TYPE
	// and AUTH_JWT_KEY (the real-world var names apps like ntask declare in
	// their .env for the same underlying key material).
	if cfg.Hasura.JWTKey == "" {
		if key, typ, ok := parseHasuraJWTSecretJSON(os.Getenv("HASURA_GRAPHQL_JWT_SECRET")); ok {
			cfg.Hasura.JWTKey = key
			if cfg.Hasura.JWTType == "" {
				cfg.Hasura.JWTType = typ
			}
		}
	}
	if cfg.Hasura.JWTKey == "" {
		cfg.Hasura.JWTKey = os.Getenv("AUTH_JWT_SECRET")
	}
	if cfg.Hasura.JWTKey == "" {
		cfg.Hasura.JWTKey = os.Getenv("AUTH_JWT_KEY")
	}
	if cfg.Hasura.JWTType == "" {
		cfg.Hasura.JWTType = os.Getenv("AUTH_JWT_TYPE")
	}

	// ── Auth ─────────────────────────────────────────────────────────
	cfg.Auth = AuthConfig{
		Version:            os.Getenv("AUTH_VERSION"),
		Port:               getEnvInt("AUTH_PORT", 0),
		ClientURL:          os.Getenv("AUTH_CLIENT_URL"),
		AccessTokenExpiry:  getEnvInt("AUTH_ACCESS_TOKEN_EXPIRES_IN", 0),
		RefreshTokenExpiry: getEnvInt("AUTH_REFRESH_TOKEN_EXPIRES_IN", 0),
		Route:              os.Getenv("AUTH_ROUTE"),
		SMTPHost:           os.Getenv("AUTH_SMTP_HOST"),
		SMTPPort:           getEnvInt("AUTH_SMTP_PORT", 0),
		SMTPUser:           os.Getenv("AUTH_SMTP_USER"),
		SMTPPass:           os.Getenv("AUTH_SMTP_PASS"),
		SMTPSecure:         getEnvBool("AUTH_SMTP_SECURE", false),
		SMTPSender:         os.Getenv("AUTH_SMTP_SENDER"),
		MemLimit:           os.Getenv("AUTH_MEM_LIMIT"),
		CPULimit:           os.Getenv("AUTH_CPU_LIMIT"),
		ExtraRedirectURLs:  os.Getenv("AUTH_EXTRA_REDIRECT_URLS"),
		WebAuthnEnabled:    getEnvBool("AUTH_WEBAUTHN_ENABLED", false),
		LogLevel:           os.Getenv("AUTH_LOG_LEVEL"),
	}

	// ── Nginx ────────────────────────────────────────────────────────
	cfg.Nginx = NginxConfig{
		Version:       os.Getenv("NGINX_VERSION"),
		HTTPPort:      getEnvInt("NGINX_HTTP_PORT", getEnvInt("NGINX_PORT", 0)),
		SSLPort:       getEnvInt("NGINX_HTTPS_PORT", getEnvInt("NGINX_SSL_PORT", 0)),
		MaxBody:       os.Getenv("NGINX_CLIENT_MAX_BODY_SIZE"),
		BindIP:        os.Getenv("NGINX_BIND_IP"),
		AuthRateLimit: os.Getenv("AUTH_RATE_LIMIT"),
		RateLimitAPI:  os.Getenv("RATE_LIMIT_API_RPS"),
		RateLimitAuth: os.Getenv("RATE_LIMIT_AUTH_RPS"),
		RateLimitAI:   os.Getenv("RATE_LIMIT_AI_RPS"),
		FrontedBy:     os.Getenv("NGINX_FRONTED_BY"),
	}

	// ── SSL ──────────────────────────────────────────────────────────
	cfg.SSLMode = os.Getenv("SSL_MODE")
	cfg.SSLProvider = os.Getenv("SSL_PROVIDER")
	cfg.SSLWildcardDomain = os.Getenv("SSL_WILDCARD_DOMAIN")
	cfg.ExtraSSLDomains = os.Getenv("EXTRA_SSL_DOMAINS")
	cfg.CloudflareAPIKey = os.Getenv("CLOUDFLARE_API_KEY")

	// ── WAF ──────────────────────────────────────────────────────────
	cfg.WAFMode = os.Getenv("WAF_MODE")

	// ── Redis ────────────────────────────────────────────────────────
	cfg.Redis = RedisConfig{
		Enabled:  getEnvBool("REDIS_ENABLED", false),
		Version:  os.Getenv("REDIS_VERSION"),
		Port:     getEnvInt("REDIS_PORT", 0),
		Password: os.Getenv("REDIS_PASSWORD"),
		Memory:   os.Getenv("REDIS_MEMORY"),
		CPU:      os.Getenv("REDIS_CPU"),
	}

}
