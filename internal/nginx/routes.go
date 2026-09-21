package nginx

import (
	"fmt"
)

// NginxRoute represents a single nginx server block route for conflict detection.
type NginxRoute struct {
	ServerName string // e.g. "auth.example.com"
	Location   string // e.g. "/" (always "/" in current templates)
	PluginName string // owning plugin name — used in conflict error messages
}

// HasDomainConflict detects when two or more routes share the same
// server_name + location combination. Returns true and a list of
// conflict description strings if conflicts are found.
//
// When NginxRoute.PluginName is set on both the current and previously-seen
// route, the conflict message uses the format:
//
//	route conflict: /api claimed by <pluginA> and <pluginB>
func HasDomainConflict(routes []NginxRoute) (bool, []string) {
	// seen maps (serverName+location) -> the NginxRoute that first claimed it,
	// so we can surface both plugin names in conflict messages.
	seen := make(map[string]NginxRoute)
	var conflicts []string
	for _, r := range routes {
		key := r.ServerName + r.Location
		if prev, exists := seen[key]; exists {
			// Produce a named-plugin conflict message when plugin names are available.
			if r.PluginName != "" && prev.PluginName != "" {
				conflicts = append(conflicts, fmt.Sprintf("route conflict: %s%s claimed by %s and %s", r.ServerName, r.Location, prev.PluginName, r.PluginName))
			} else {
				conflicts = append(conflicts, fmt.Sprintf("%s conflicts with %s (same server_name+location: %s%s)", r.ServerName, prev.ServerName, r.ServerName, r.Location))
			}
		} else {
			seen[key] = r
		}
	}
	return len(conflicts) > 0, conflicts
}

// generateAllRoutes generates per-service nginx site configs.
// Returns map of filepath (e.g. "nginx/sites/hasura.conf") to nginx config content.
func (g *Generator) generateAllRoutes() (map[string]string, error) {
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

	// Complete every route entry's ServiceRouteData through the same
	// generator-owned helper RenderServiceRoute uses (finalizeServiceRoute in
	// generator.go): HasSSL, HasTrustedChain, SSLBasePath, UpstreamName,
	// ProxyTarget, and a default PathZones. This loop (not RenderServiceRoute)
	// is the code path `nself build` actually uses to render every
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
