package nginx

// routes_core.go — core and optional-service route generation.
//
// Purpose: build the Nginx route set for the always-on core services and the enabled optional services, used by generateAllRoutes in routes.go, split out for file size.
// Inputs: the loaded Config identifying enabled services and their ports/domains.
// Outputs: []NginxRoute entries for the core and optional services.
// Constraints: pure move from routes.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
)

// coreRoutes returns routes for always-present services: hasura, auth, storage, storage-console.
func (g *Generator) coreRoutes(baseDomain, sslDir string) []routeEntry {
	var entries []routeEntry

	// Hasura GraphQL
	hasuraRoute := g.cfg.Hasura.Route
	if hasuraRoute == "" {
		hasuraRoute = "api"
	}
	hasuraRoute = g.appPrefixedRoute(hasuraRoute)
	// Gap #7: the nginx upstream must always target the container-internal
	// listen port (hasuraContainerPort, always 8080 — see buildHasuraService in
	// internal/compose/core_services.go), never cfg.Hasura.Port. cfg.Hasura.Port
	// is the HOST-exposed port ("127.0.0.1:<port>:8080" in the compose Ports
	// mapping) and is meant for reaching Hasura from outside Docker. nginx and
	// Hasura share the same Docker network, so the upstream must use the
	// in-network port regardless of what host port the operator chose (e.g.
	// HASURA_PORT=8181 to avoid a host port collision must NOT change the
	// upstream, or nginx proxies to a port Hasura isn't listening on inside
	// the container).
	entries = append(entries, routeEntry{
		filename: "hasura.conf",
		data: ServiceRouteData{
			Route:      hasuraRoute,
			BaseDomain: baseDomain,
			Upstream:   fmt.Sprintf("hasura:%d", hasuraContainerPort),
			SSLDir:     sslDir,
			RateZone:   "graphql_api",
			Burst:      20,
			ConnLimit:  10,
			WebSocket:  true,
		},
	})

	// Auth
	authRoute := g.cfg.Auth.Route
	if authRoute == "" {
		authRoute = "auth"
	}
	authRoute = g.appPrefixedRoute(authRoute)
	authPort := g.cfg.Auth.Port
	if authPort == 0 {
		authPort = 4000
	}
	entries = append(entries, routeEntry{
		filename: "auth.conf",
		data: ServiceRouteData{
			Route:      authRoute,
			BaseDomain: baseDomain,
			Upstream:   fmt.Sprintf("auth:%d", authPort),
			SSLDir:     sslDir,
			RateZone:   "auth",
			Burst:      5,
			ConnLimit:  5,
		},
	})

	// Storage
	if g.cfg.Minio.Enabled {
		storageRoute := g.cfg.Minio.StorageRoute
		if storageRoute == "" {
			storageRoute = "storage"
		}
		storageRoute = g.appPrefixedRoute(storageRoute)
		entries = append(entries, routeEntry{
			filename: "storage.conf",
			data: ServiceRouteData{
				Route:      storageRoute,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("minio:%d", g.cfg.Minio.Port),
				SSLDir:     sslDir,
				RateZone:   "uploads",
				Burst:      2,
				ConnLimit:  5,
			},
		})

		// Storage Console
		consoleRoute := g.cfg.Minio.ConsoleRoute
		if consoleRoute == "" {
			consoleRoute = "storage-console"
		}
		consoleRoute = g.appPrefixedRoute(consoleRoute)
		entries = append(entries, routeEntry{
			filename: "storage-console.conf",
			data: ServiceRouteData{
				Route:      consoleRoute,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("minio:%d", minioConsolePort(g.cfg)),
				SSLDir:     sslDir,
			},
		})
	}

	return entries
}

// optionalRoutes returns routes for optional services that are conditionally enabled.
func (g *Generator) optionalRoutes(baseDomain, sslDir string) []routeEntry {
	var entries []routeEntry

	// Admin (may not be running)
	if g.cfg.Admin.Enabled {
		entries = append(entries, g.adminRoute(baseDomain, sslDir))
	}

	// Search
	if g.cfg.Search.Enabled {
		searchRoute := g.cfg.Search.Route
		if searchRoute == "" {
			searchRoute = "search"
		}
		searchPort := g.cfg.Search.Port
		if searchPort == 0 {
			searchPort = 7700
		}
		// Use actual Docker service name as upstream (meilisearch, typesense, etc.)
		searchUpstream := g.cfg.Search.Engine
		if searchUpstream == "" {
			searchUpstream = "meilisearch"
		}
		entries = append(entries, routeEntry{
			filename: "search.conf",
			data: ServiceRouteData{
				Route:      searchRoute,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("%s:%d", searchUpstream, searchPort),
				SSLDir:     sslDir,
				RateZone:   "general",
				Burst:      10,
			},
		})
	}

	// Mail (Mailpit)
	if g.cfg.Mailpit.Enabled {
		mailRoute := g.cfg.Mailpit.Route
		if mailRoute == "" {
			mailRoute = "mail"
		}
		mailPort := g.cfg.Mailpit.UIPort
		if mailPort == 0 {
			mailPort = 8025
		}
		entries = append(entries, routeEntry{
			filename: "mail.conf",
			data: ServiceRouteData{
				Route:      mailRoute,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("mailpit:%d", mailPort),
				SSLDir:     sslDir,
				RateZone:   "general",
				Burst:      10,
			},
		})
	}

	// MLflow: routes handled by nself-mlflow free plugin

	// Functions
	if g.cfg.Functions.Enabled {
		fnRoute := g.cfg.Functions.Route
		if fnRoute == "" {
			fnRoute = "functions"
		}
		fnPort := g.cfg.Functions.Port
		if fnPort == 0 {
			fnPort = 3008
		}
		entries = append(entries, routeEntry{
			filename: "functions.conf",
			data: ServiceRouteData{
				Route:      fnRoute,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("functions:%d", fnPort),
				SSLDir:     sslDir,
				RateZone:   "functions",
				Burst:      15,
				ConnLimit:  10,
			},
		})
	}

	// Monitoring: routes handled by nself-monitoring free plugin

	return entries
}
