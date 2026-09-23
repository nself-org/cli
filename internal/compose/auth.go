package compose

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// authPGAliasVars returns AUTH_DB_* alias vars that Nhost Auth uses
// to connect to PostgreSQL. These are required alongside POSTGRES_*.
// The internal container port is always 5432.
func authPGAliasVars(pg config.PostgresConfig) map[string]string {
	dsn := fmt.Sprintf("postgresql://%s@postgres:5432/%s", config.URLUserInfo(pg.User, pg.Password), pg.DB)
	return map[string]string{
		"AUTH_DB_HOST":     "postgres",
		"AUTH_DB_PORT":     "5432",
		"AUTH_DB_NAME":     pg.DB,
		"AUTH_DB_USER":     pg.User,
		"AUTH_DB_PASSWORD": pg.Password,
		"AUTH_DB_URL":      dsn,
	}
}

// computeAllowedRedirectURLs builds the AUTH_ALLOWED_REDIRECT_URLS env var value.
// Includes base domain, wildcard subdomain, auth client URL, frontend app routes,
// and any user-supplied extra URLs (AUTH_EXTRA_REDIRECT_URLS).
func computeAllowedRedirectURLs(cfg *config.Config) string {
	seen := map[string]bool{}
	var urls []string
	add := func(u string) {
		if u != "" && !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	add(fmt.Sprintf("https://%s", cfg.BaseDomain))
	add(fmt.Sprintf("https://*.%s", cfg.BaseDomain))
	add(cfg.Auth.ClientURL)
	for _, app := range cfg.FrontendApps {
		if app.Route != "" {
			add(fmt.Sprintf("https://%s.%s", app.Route, cfg.BaseDomain))
		}
	}
	if cfg.Auth.ExtraRedirectURLs != "" {
		for _, u := range strings.Split(cfg.Auth.ExtraRedirectURLs, ",") {
			add(strings.TrimSpace(u))
		}
	}
	return strings.Join(urls, ",")
}
