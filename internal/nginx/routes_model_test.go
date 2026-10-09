package nginx

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
)

func modelFixture(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.ApplyDefaults(&config.Config{BaseDomain: "example.test", SSLMode: "none"})
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// TestRouteModelProperty compares 500 seeded configurations with the old path.
func TestRouteModelProperty(t *testing.T) {
	rng := rand.New(rand.NewSource(701))
	for i := 0; i < 500; i++ {
		cfg := modelFixture(t)
		cfg.Hasura.Route = "api" + string(rune('a'+rng.Intn(26)))
		cfg.Auth.Route = "auth" + string(rune('a'+rng.Intn(26)))
		cfg.Minio.Enabled = rng.Intn(2) == 0
		cfg.Admin.Enabled = rng.Intn(2) == 0
		cfg.Search.Enabled = rng.Intn(2) == 0
		cfg.Mailpit.Enabled = rng.Intn(2) == 0
		cfg.Functions.Enabled = rng.Intn(2) == 0
		cfg.CustomServices = []config.CustomService{{Index: 1, Name: "worker", Route: "worker", Port: 9000 + rng.Intn(100)}}
		g := NewGenerator(cfg, t.TempDir())
		if err := g.parseTemplates(); err != nil {
			t.Fatal(err)
		}
		old, err := g.legacyGenerateAllRoutes()
		if err != nil {
			t.Fatal(err)
		}
		g.seenRoutes = nil
		current, err := g.generateAllRoutes()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(old, current) {
			t.Fatalf("case %d changed nginx site bytes", i)
		}
		b, err := routemodel.Marshal(g.model)
		if err != nil {
			t.Fatal(err)
		}
		var decoded routemodel.Model
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
		b2, err := routemodel.Marshal(&decoded)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != string(b2) {
			t.Fatalf("case %d model not stable", i)
		}
	}
}

func TestShadowedRoute(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "nginx", "conf.d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nginx", "conf.d", "manual.conf"), []byte("server { server_name api.example.test; }"), 0644); err != nil {
		t.Fatal(err)
	}
	g := NewGenerator(modelFixture(t), dir)
	files, err := g.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["nginx/sites/hasura.conf"]; ok {
		t.Fatal("shadowed route rendered")
	}
	b, err := routemodel.Marshal(g.model)
	if err != nil {
		t.Fatal(err)
	}
	var decoded routemodel.Model
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range decoded.Routes {
		if r.ID == "core:hasura" && r.ShadowedBy != nil && strings.Contains(*r.ShadowedBy, "manual.conf") {
			found = true
		}
	}
	if !found {
		t.Fatal("routes.json lost the shadowed route and its source file")
	}
}

func TestGoldenFrontendRoutes(t *testing.T) {
	old := isLinuxServer
	isLinuxServer = func() bool { return false }
	t.Cleanup(func() { isLinuxServer = old })
	cfg := modelFixture(t)
	cfg.FrontendApps = []config.FrontendApp{{Index: 1, SystemName: "web", Route: "web", Port: 3000}}
	g := NewGenerator(cfg, t.TempDir())
	if err := g.parseTemplates(); err != nil {
		t.Fatal(err)
	}
	want, err := g.legacyGenerateAllRoutes()
	if err != nil {
		t.Fatal(err)
	}
	got, err := g.generateAllRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatal("frontend route bytes differ")
	}
	if _, ok := got["nginx/sites/frontend-web.conf"]; !ok {
		t.Fatal("frontend fixture was not rendered")
	}
}

// generateAllRoutes generates per-service nginx site configs.
// Returns map of filepath (e.g. "nginx/sites/hasura.conf") to nginx config content.
func (g *Generator) legacyGenerateAllRoutes() (map[string]string, error) {
	files := make(map[string]string)

	baseDomain := g.cfg.BaseDomain
	sslDir := sslDirName(baseDomain)

	// Core routes (always generated)
	coreRoutes := g.coreRoutes(baseDomain, sslDir)

	// Optional service routes
	optionalRoutes := g.optionalRoutes(baseDomain, sslDir)

	// Custom service routes (CS_1..CS_10)
	csRoutes := g.customServiceRoutes(baseDomain, sslDir)

	// Frontend app routes (skipped on Linux servers)
	var feRoutes []routeEntry
	if !isLinuxServer() {
		feRoutes = g.frontendRoutes(baseDomain, sslDir)
	}

	// Internal routes (INTERNAL_ROUTE_1..INTERNAL_ROUTE_20)
	irRoutes, err := g.internalRoutes(baseDomain, sslDir)
	if err != nil {
		return nil, err
	}

	// Combine all route entries
	allEntries := make([]routeEntry, 0, len(coreRoutes)+len(optionalRoutes)+len(csRoutes)+len(feRoutes)+len(irRoutes))
	allEntries = append(allEntries, coreRoutes...)
	allEntries = append(allEntries, optionalRoutes...)
	allEntries = append(allEntries, csRoutes...)
	allEntries = append(allEntries, feRoutes...)
	allEntries = append(allEntries, irRoutes...)

	// Initialize seen map for this generation batch.
	g.seenRoutes = make(map[string]bool)

	// Complete every route entry's ServiceRouteData through finalizeServiceRoute
	// below, the single completion point it shares with RenderServiceRoute.
	// This loop is the code path `nself build` actually uses to render every
	// nginx/sites/*.conf file, so any field the two call sites set
	// independently is prone to drift — see finalizeServiceRoute's doc
	// comment for the SSLBasePath incident this caused (prod, 2026-09-21) and
	// the earlier HasTrustedChain incident it also once caused (2026-09-03).
	for i := range allEntries {
		g.finalizeServiceRoute(&allEntries[i].data)
	}

	for _, entry := range allEntries {
		// Domain conflict prevention: skip if domain already exists in conf.d/
		if g.hasDomainConflict(entry.data.Route, entry.data.BaseDomain) {
			continue
		}

		content, err := g.render("service.conf.tmpl", entry.data)
		if err != nil {
			return nil, fmt.Errorf("rendering route %s: %w", entry.filename, err)
		}
		files["nginx/sites/"+entry.filename] = content
	}

	return files, nil
}

// routeEntry pairs a filename with its template data.
type routeEntry struct {
	filename string
	data     ServiceRouteData
}

// appPrefixedRoute returns route prefixed with the configured APP_NAME
// (gap #5), producing e.g. "api.task" instead of "api" so the resulting
// server_name is "api.task.{BASE_DOMAIN}". When APP_NAME is unset (the
// default), route is returned unchanged, preserving the bare
// "api.{BASE_DOMAIN}" scheme every existing single-app deployment
// (ummat, unity) already relies on.
func (g *Generator) appPrefixedRoute(route string) string {
	if g.cfg.AppName == "" {
		return route
	}
	return route + "." + g.cfg.AppName
}

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

// adminRoute builds the admin route entry with dev mode support. The
// route it returns is resolved at request time by the generated template
// (see service.conf.tmpl) like every other service route.
func (g *Generator) adminRoute(baseDomain, sslDir string) routeEntry {
	adminRoute := g.cfg.Admin.Route
	if adminRoute == "" {
		adminRoute = "admin"
	}

	var upstream string
	if g.cfg.Admin.DevMode {
		// Dev mode: proxy to host machine for hot-reload
		devPort := g.cfg.Admin.DevPort
		if devPort == 0 {
			devPort = 3000
		}
		if runtime.GOOS == "linux" {
			upstream = fmt.Sprintf("172.17.0.1:%d", devPort)
		} else {
			upstream = fmt.Sprintf("host.docker.internal:%d", devPort)
		}
	} else {
		adminPort := g.cfg.Admin.Port
		if adminPort == 0 {
			adminPort = 3021
		}
		upstream = fmt.Sprintf("nself-admin:%d", adminPort)
	}

	return routeEntry{
		filename: "admin.conf",
		data: ServiceRouteData{
			Route:      adminRoute,
			BaseDomain: baseDomain,
			Upstream:   upstream,
			SSLDir:     sslDir,
			RateZone:   "general",
			Burst:      10,
		},
	}
}

// customServiceRoutes returns routes for CS_1..CS_10 custom services.
// All custom services resolve their upstream at request time (see
// service.conf.tmpl) since they may be started, stopped, or recreated
// independently of nginx.
func (g *Generator) customServiceRoutes(baseDomain, sslDir string) []routeEntry {
	var entries []routeEntry

	for _, cs := range g.cfg.CustomServices {
		if cs.Route == "" {
			continue // internal only, no nginx route
		}
		if cs.Port == 0 {
			continue
		}

		entries = append(entries, routeEntry{
			filename: fmt.Sprintf("cs-%s.conf", cs.Name),
			data: ServiceRouteData{
				Route:      cs.Route,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("%s:%d", cs.Name, cs.Port),
				SSLDir:     sslDir,
				RateZone:   "general",
				Burst:      10,
			},
		})
	}

	return entries
}

// frontendRoutes returns routes for frontend apps.
// These are SKIPPED on Linux servers (caller checks isLinuxServer).
// Uses host.docker.internal since frontend dev servers run on the host.
func (g *Generator) frontendRoutes(baseDomain, sslDir string) []routeEntry {
	var entries []routeEntry

	for _, fe := range g.cfg.FrontendApps {
		if fe.Route == "" || fe.Port == 0 || fe.SystemName == "" {
			continue
		}

		entries = append(entries, routeEntry{
			filename: fmt.Sprintf("frontend-%s.conf", fe.SystemName),
			data: ServiceRouteData{
				Route:      fe.Route,
				BaseDomain: baseDomain,
				Upstream:   fmt.Sprintf("host.docker.internal:%d", fe.Port),
				SSLDir:     sslDir,
				RateZone:   "static",
				Burst:      10,
				WebSocket:  true, // HMR needs WebSocket
			},
		})
	}

	return entries
}

// validateInternalRouteTarget checks that an InternalRoute target is a valid
// http://host:port or https://host:port URL with no nginx-unsafe characters.
func validateInternalRouteTarget(target string) error {
	// Reject injection characters before URL parsing.
	for _, ch := range []rune{'\n', '\r', ';', '{', '}'} {
		if strings.ContainsRune(target, ch) {
			return fmt.Errorf("target contains forbidden character %q", ch)
		}
	}
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("target is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("target scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("target is missing host")
	}
	return nil
}

// internalRoutes returns routes for INTERNAL_ROUTE_1..INTERNAL_ROUTE_20.
// These resolve their upstream at request time (see service.conf.tmpl)
// since the target may live in another stack entirely and be recreated
// independently of this nginx.
func (g *Generator) internalRoutes(baseDomain, sslDir string) ([]routeEntry, error) {
	var entries []routeEntry

	for _, ir := range g.cfg.InternalRoutes {
		if ir.Subdomain == "" || ir.Target == "" {
			continue
		}
		if err := validateInternalRouteTarget(ir.Target); err != nil {
			return nil, fmt.Errorf("internal route %q: invalid target: %w", ir.Name, err)
		}

		rateZone := ir.RateZone
		if rateZone == "" {
			rateZone = "general"
		}

		entries = append(entries, routeEntry{
			filename: fmt.Sprintf("ir-%s.conf", ir.Name),
			data: ServiceRouteData{
				Route:      ir.Subdomain,
				BaseDomain: baseDomain,
				Upstream:   ir.Target,
				SSLDir:     sslDir,
				RateZone:   rateZone,
				Burst:      10,
				ConnLimit:  10,
				WebSocket:  ir.WebSocket,
			},
		})
	}

	return entries, nil
}
