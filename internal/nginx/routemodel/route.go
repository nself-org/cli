package routemodel

import (
	"fmt"
	"net/url"
	"runtime"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

func isLinux() bool { return runtime.GOOS == "linux" }

func makeRoute(cfg *config.Config, workdir, sslDir string, hasSSL bool, s routeSpec) Route {
	name, err := config.RouteToFQDN(s.name, cfg.BaseDomain)
	if err != nil {
		name = s.name + "." + cfg.BaseDomain
	}
	r := Route{ID: s.id, Source: s.source, File: "nginx/sites/" + s.file,
		ServerNames: []string{name}, Listen: Listen{HTTP: !hasSSL, HTTPS: hasSSL},
		SecurityHeaders: headers(hasSSL), BlockedPaths: []BlockedPath{
			{Regex: `/\.(env|git|svn|htaccess|htpasswd|dockerignore)`, Status: 404},
			{Regex: `^/(docker-compose|Dockerfile)`, Status: 404}},
		Locations: []Location{}, Unmodelled: []string{}, LegacyServerName: s.legacy + "." + cfg.BaseDomain,
		RouteName: s.name, RawUpstream: s.upstream,
		ShadowedBy: shadowFile(cfg, workdir, name)}
	if s.owner != "" {
		r.Owner = ptr(s.owner)
	}
	if hasSSL {
		r.TLS = &RouteTLS{SSLDir: sslDir, Protocols: []string{"TLSv1.2", "TLSv1.3"}, Ciphers: ptr(ciphers)}
	}
	if s.zone != "" {
		r.RateLimit = &RateLimit{Zone: s.zone, Burst: s.burst, ConnLimit: s.conn}
	}
	up := parseUpstream(s.upstream)
	forwarded := map[string]string{"Host": "$host", "X-Real-IP": "$remote_addr", "X-Forwarded-For": "$proxy_add_x_forwarded_for", "X-Forwarded-Proto": "$scheme"}
	r.Locations = append(r.Locations, Location{Path: "/", Match: "prefix", Upstream: &up,
		WebSocket: s.websocket, ForwardedHeaders: true, HeadersSet: forwarded, Timeouts: Timeouts{}, AccessLog: true})
	for _, p := range []struct {
		path, match, zone string
		burst             int
	}{
		{"/auth/login", "exact", "auth_strict", 5}, {"/api/", "prefix", "api", 20}} {
		copyHeaders := map[string]string{}
		for k, v := range forwarded {
			copyHeaders[k] = v
		}
		r.Locations = append(r.Locations, Location{Path: p.path, Match: p.match, Upstream: &up,
			RateLimit:        &LocationRate{Zone: p.zone, Burst: p.burst, Nodelay: true},
			ForwardedHeaders: true, HeadersSet: copyHeaders, Timeouts: Timeouts{}, AccessLog: true})
	}
	r.Locations = append(r.Locations, Location{Path: "/healthz", Match: "exact", Upstream: &up,
		HealthProbe: true, ForwardedHeaders: true, HeadersSet: forwarded, Timeouts: Timeouts{}, AccessLog: true,
		Methods: []string{"GET"}})
	if !hasSSL {
		r.Locations = append(r.Locations, acmeLocation())
	}
	return r
}

func headers(ssl bool) map[string]string {
	h := map[string]string{
		"X-Frame-Options": "SAMEORIGIN", "X-Content-Type-Options": "nosniff", "X-XSS-Protection": "0",
		"Referrer-Policy": "strict-origin-when-cross-origin", "Permissions-Policy": "camera=(), microphone=(), geolocation=()",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; frame-ancestors 'none'",
	}
	if ssl {
		h["Strict-Transport-Security"] = "max-age=63072000; includeSubDomains; preload"
	}
	return h
}

func acmeLocation() Location {
	root := "/etc/nginx/ssl/.acme-webroot"
	return Location{Path: "/.well-known/acme-challenge/", Match: "prefix", StaticRoot: &root,
		Methods: []string{"GET"}, DenyAll: false, HeadersSet: map[string]string{}, Timeouts: Timeouts{}, AccessLog: true,
	}
}

func parseUpstream(raw string) Upstream {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return Upstream{Scheme: "http", Host: raw, Port: 80}
	}
	// A portless target means the scheme's default port; the model never
	// carries port 0 (routes.json and the model-rendered proxy_pass use it).
	port, err := strconv.Atoi(u.Port())
	if err != nil || port == 0 {
		port = 80
		if u.Scheme == "https" {
			port = 443
		}
	}
	return Upstream{Scheme: u.Scheme, Host: u.Hostname(), Port: port}
}

func buildZones(cfg *config.Config) []Zone {
	api, auth, ai := cfg.Nginx.RateLimitAPI, cfg.Nginx.RateLimitAuth, cfg.Nginx.RateLimitAI
	if api == "" {
		api = "30"
	}
	if auth == "" {
		auth = "5"
	}
	if ai == "" {
		ai = "10"
	}
	request := []struct{ name, key, rate string }{
		{"general", "$binary_remote_addr", "2r/s"}, {"graphql_api", "$binary_remote_addr", "100r/m"},
		{"auth", "$binary_remote_addr", cfg.Nginx.AuthRateLimit}, {"uploads", "$binary_remote_addr", "5r/m"},
		{"user_api", "$http_authorization", "1000r/m"}, {"static", "$binary_remote_addr", "1000r/m"},
		{"webhooks", "$binary_remote_addr", fmt.Sprintf("%sr/s", api)}, {"functions", "$binary_remote_addr", "50r/m"},
		{"api", "$binary_remote_addr", fmt.Sprintf("%sr/s", api)}, {"auth_strict", "$binary_remote_addr", fmt.Sprintf("%sr/s", auth)},
		{"ai", "$binary_remote_addr", fmt.Sprintf("%sr/s", ai)}, {"webhook", "$binary_remote_addr", "100r/s"},
		{"tenant_api", "$http_x_tenant_id", fmt.Sprintf("%sr/s", api)}, {"tenant_ai", "$http_x_tenant_id", fmt.Sprintf("%sr/s", ai)},
	}
	zones := make([]Zone, 0, len(request)+2)
	for _, z := range request {
		zones = append(zones, Zone{Name: z.name, Kind: "req", Key: z.key, Rate: ptr(z.rate)})
	}
	zones = append(zones, Zone{Name: "conn_limit_per_ip", Kind: "conn", Key: "$binary_remote_addr"}, Zone{Name: "conn_limit_server", Kind: "conn", Key: "$server_name"})
	return zones
}
