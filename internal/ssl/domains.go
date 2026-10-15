package ssl

import (
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// CollectDomains gathers all Subject Alternative Names (SANs) needed for the
// project's SSL certificate. The list is deduplicated. It is exported for use
// by the dns-setup command.
func (g *Generator) CollectDomains() []string {
	return g.collectDomains()
}

// collectDomains gathers all Subject Alternative Names (SANs) needed for the
// project's SSL certificate. The list is deduplicated.
func (g *Generator) collectDomains() []string {
	seen := make(map[string]bool)
	var domains []string

	add := func(d string) {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			return
		}
		seen[d] = true
		domains = append(domains, d)
	}

	// resolveRoute returns the FQDN for a route value. If the route already
	// ends with ".{baseDomain}" it is returned as-is. Otherwise baseDomain is
	// appended so that both bare values like "api" and namespace subdomains
	// like "www.pro" become "api.ummat.local" / "www.pro.ummat.local".
	resolveRoute := func(route, baseDomain string) string {
		if strings.HasSuffix(route, "."+baseDomain) {
			return route
		}
		return route + "." + baseDomain
	}

	// Always included: localhost, loopback addresses.
	add("localhost")
	add("127.0.0.1")
	add("::1")

	bd := g.cfg.BaseDomain

	// BASE_DOMAIN and wildcard.
	if bd != "" {
		add(bd)
		add("*." + bd)
	}

	// local.nself.org and wildcard (always, for local dev tooling).
	add("local.nself.org")
	add("*.local.nself.org")

	// Core services (always enabled).
	if bd != "" {
		// Hasura
		if route := g.cfg.Hasura.Route; route != "" {
			add(resolveRoute(route, bd))
		} else {
			add(strings.TrimPrefix(config.BuildServiceURL("api", bd), "https://"))
		}

		// Auth
		if route := g.cfg.Auth.Route; route != "" {
			add(resolveRoute(route, bd))
		} else {
			add(strings.TrimPrefix(config.BuildServiceURL("auth", bd), "https://"))
		}

		// Storage (MinIO + nhost-storage, or SeaweedFS in v1.5 with no console). D-0683.
		store := compose.CurrentStore(g.cfg)
		if store != "none" {
			if route := g.cfg.Minio.StorageRoute; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("storage", bd), "https://"))
			}
		}
		if store == "minio-core" || store == "minio-plugin" {
			if route := g.cfg.Minio.ConsoleRoute; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("storage-console", bd), "https://"))
			}
		}

		// Admin
		if g.cfg.Admin.Enabled {
			if route := g.cfg.Admin.Route; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("admin", bd), "https://"))
			}
		}

		// Search
		if g.cfg.Search.Enabled {
			if route := g.cfg.Search.Route; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("search", bd), "https://"))
			}
		}

		// Mailpit
		if g.cfg.Mailpit.Enabled {
			if route := g.cfg.Mailpit.Route; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("mail", bd), "https://"))
			}
		}

		// Functions
		if g.cfg.Functions.Enabled {
			if route := g.cfg.Functions.Route; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("functions", bd), "https://"))
			}
		}

		// MLflow
		if g.cfg.MLflow.Enabled {
			if route := g.cfg.MLflow.Route; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("mlflow", bd), "https://"))
			}
		}

		// Monitoring (Grafana)
		if g.cfg.Monitoring.Enabled && g.cfg.Monitoring.GrafanaEnabled {
			if route := g.cfg.Monitoring.GrafanaRoute; route != "" {
				add(resolveRoute(route, bd))
			} else {
				add(strings.TrimPrefix(config.BuildServiceURL("grafana", bd), "https://"))
			}
		}

		// Custom services (only if they have a public route).
		for _, cs := range g.cfg.CustomServices {
			if cs.Route != "" && cs.Public {
				// Route may be a full domain or a subdomain prefix (possibly with dots).
				if strings.HasSuffix(cs.Route, "."+bd) {
					add(cs.Route)
				} else {
					add(cs.Route + "." + bd)
				}
			}
		}

		// Frontend apps.
		for _, fa := range g.cfg.FrontendApps {
			if fa.Route != "" {
				if strings.HasSuffix(fa.Route, "."+bd) {
					add(fa.Route)
				} else {
					add(fa.Route + "." + bd)
				}
			}
		}
	}

	// EXTRA_SSL_DOMAINS (comma-separated).
	if extra := g.cfg.ExtraSSLDomains; extra != "" {
		for _, d := range strings.Split(extra, ",") {
			add(d)
		}
	}

	return domains
}
