package nginx

import (
	"fmt"
	"strings"

	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
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

// generateAllRoutes projects the shared model into the existing template data.
func (g *Generator) generateAllRoutes() (map[string]string, error) {
	if err := g.ensureModel(); err != nil {
		return nil, err
	}
	files := make(map[string]string)
	g.seenRoutes = make(map[string]bool)
	for _, r := range g.model.Routes {
		if r.Source == "frontend" && isLinuxServer() {
			continue
		}
		if len(r.ServerNames) == 0 || r.ShadowedBy != nil {
			continue
		}
		fqdn := r.ServerNames[0]
		if g.seenRoutes[fqdn] {
			continue
		}
		g.seenRoutes[fqdn] = true
		route := r.RouteName
		if route == "" {
			route = strings.TrimSuffix(fqdn, "."+g.model.BaseDomain)
		}
		upstream := r.RawUpstream
		if upstream == "" {
			for _, loc := range r.Locations {
				if loc.Path == "/" && loc.Upstream != nil {
					upstream = fmt.Sprintf("%s://%s:%d", loc.Upstream.Scheme, loc.Upstream.Host, loc.Upstream.Port)
					break
				}
			}
		}
		data := ServiceRouteData{Route: route, BaseDomain: g.model.BaseDomain, Upstream: upstream}
		if r.TLS != nil {
			data.SSLDir = r.TLS.SSLDir
		}
		if r.RateLimit != nil {
			data.RateZone, data.Burst, data.ConnLimit = r.RateLimit.Zone, r.RateLimit.Burst, r.RateLimit.ConnLimit
		}
		for _, loc := range r.Locations {
			if loc.Path == "/" {
				data.WebSocket = loc.WebSocket
			}
			if loc.RateLimit != nil && loc.Path != "/" {
				data.PathZones = append(data.PathZones, PathZone{Path: loc.Path, Exact: loc.Match == "exact", Zone: loc.RateLimit.Zone, Burst: loc.RateLimit.Burst})
			}
		}
		g.finalizeServiceRoute(&data)
		content, err := g.render("service.conf.tmpl", data)
		if err != nil {
			return nil, fmt.Errorf("rendering route %s: %w", r.File, err)
		}
		files[r.File] = content
	}
	return files, nil
}

func (g *Generator) ensureModel() error {
	if g.model != nil {
		return nil
	}
	m, err := routemodel.Build(g.cfg, g.workdir, g.hasSSL, g.hasTrustedChain)
	if err != nil {
		return err
	}
	g.model = m
	return nil
}

// finalizeServiceRoute fills in every ServiceRouteData field the generator
// (not the caller) is responsible for computing: HasSSL, HasTrustedChain,
// SSLBasePath, UpstreamName, ProxyTarget, and a default PathZones.
//
// This is the single completion point for ServiceRouteData, called from both
// RenderServiceRoute (generator.go) and the generateAllRoutes loop above. It
// exists because the two used to duplicate this logic, and the loop's copy
// silently omitted SSLBasePath: every nginx/sites/*.conf written by `nself
// build` rendered "ssl_certificate /certificates/<dir>/fullchain.pem"
// (missing the "/etc/nginx/ssl" mount-path prefix service.conf.tmpl
// expects), so nginx refused to start with "cannot load certificate ...:
// BIO_new_file() failed" on every fresh SSL-enabled build. Verified on
// production 2026-09-21. Both call sites must go through this helper so the
// two paths cannot drift apart again.
func (g *Generator) finalizeServiceRoute(data *ServiceRouteData) {
	data.HasSSL = g.hasSSL
	data.HasTrustedChain = g.hasTrustedChain(data.SSLDir)
	data.SSLBasePath = nginxtopo.NginxSSLContainerPath
	data.UpstreamName = upstreamName(data.Route)
	data.ProxyTarget = proxyTarget(data.Upstream)
	if data.PathZones == nil {
		data.PathZones = defaultSecurityPathZones()
	}
}

// proxyTarget normalizes an upstream address into a proxy_pass URL with
// exactly one scheme. Compose-service upstreams are bare "host:port" and get
// "http://" prepended; internal-route targets already carry their own scheme
// (validateInternalRouteTarget rejects a target without one) and are passed
// through unchanged, including "https://".
func proxyTarget(upstream string) string {
	if strings.HasPrefix(upstream, "http://") || strings.HasPrefix(upstream, "https://") {
		return upstream
	}
	return "http://" + upstream
}

// upstreamName derives a unique, nginx-safe proxy_pass variable name suffix from a route.
// nginx upstream names allow [A-Za-z0-9_], so non-conforming chars become '_'.
func upstreamName(route string) string {
	var b strings.Builder
	b.WriteString("up_")
	for _, r := range route {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
