package nginx

// routes_app.go — admin, custom-service, frontend and internal app routes.
//
// Purpose: build the Nginx route set for the admin UI, custom services, frontend apps and internal-only routes, used by generateAllRoutes in routes.go, split out for file size.
// Inputs: the loaded Config identifying custom services, frontend apps and internal routes.
// Outputs: []NginxRoute entries for these app-level routes.
// Constraints: pure move from routes.go (CLI-R12 Batch E); no behaviour change.

import (
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

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
